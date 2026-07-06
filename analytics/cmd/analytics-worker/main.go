package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	_ "github.com/lib/pq"
)

const maxJobAttempts = 3

type config struct {
	databaseURL       string
	workerID          string
	pollInterval      time.Duration
	maxConcurrentJobs int
}

type aiJob struct {
	ID            string
	CameraID      string
	SourceEventID sql.NullString
	JobType       string
	Priority      int
	FramePath     sql.NullString
	MetadataJSON  string
	Attempts      int
}

type jobHandler interface {
	Handle(ctx context.Context, job aiJob) error
}

type handlerFunc func(context.Context, aiJob) error

func (fn handlerFunc) Handle(ctx context.Context, job aiJob) error {
	return fn(ctx, job)
}

type worker struct {
	db       *sql.DB
	cfg      config
	handlers map[string]jobHandler
	slots    chan struct{}
	wg       sync.WaitGroup
}

func main() {
	cfg := loadConfig()

	db, err := sql.Open("postgres", cfg.databaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(cfg.maxConcurrentJobs + 2)
	db.SetMaxIdleConns(cfg.maxConcurrentJobs + 1)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ping database: %v", err)
	}

	w := newWorker(db, cfg)
	log.Printf("analytics worker running worker_id=%s poll_interval=%s max_concurrent_jobs=%d inference=disabled",
		cfg.workerID, cfg.pollInterval, cfg.maxConcurrentJobs)

	if err := w.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("analytics worker stopped with error: %v", err)
	}
	log.Printf("analytics worker stopped worker_id=%s", cfg.workerID)
}

func newWorker(db *sql.DB, cfg config) *worker {
	return &worker{
		db:    db,
		cfg:   cfg,
		slots: make(chan struct{}, cfg.maxConcurrentJobs),
		handlers: map[string]jobHandler{
			"human_detect":   placeholderHandler("human_detect"),
			"face_detect":    placeholderHandler("face_detect"),
			"identity_match": placeholderHandler("identity_match"),
		},
	}
}

func (w *worker) run(ctx context.Context) error {
	ticker := time.NewTicker(w.cfg.pollInterval)
	defer ticker.Stop()

	for {
		if err := w.dispatchAvailable(ctx); err != nil {
			log.Printf("analytics poll failed: %v", err)
		}

		select {
		case <-ctx.Done():
			w.wg.Wait()
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *worker) dispatchAvailable(ctx context.Context) error {
	available := cap(w.slots) - len(w.slots)
	if available <= 0 {
		return nil
	}

	jobs, err := w.claimPendingJobs(ctx, available)
	if err != nil {
		return err
	}

	for _, job := range jobs {
		w.slots <- struct{}{}
		w.wg.Add(1)
		go func(job aiJob) {
			defer func() {
				<-w.slots
				w.wg.Done()
			}()
			w.processJob(ctx, job)
		}(job)
	}
	return nil
}

func (w *worker) claimPendingJobs(ctx context.Context, limit int) ([]aiJob, error) {
	tx, err := w.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		WITH picked AS (
			SELECT id
			FROM ai_jobs
			WHERE status = 'pending'
			ORDER BY priority ASC, created_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE ai_jobs AS jobs
		SET status = 'processing',
			started_at = now(),
			error_message = NULL
		FROM picked
		WHERE jobs.id = picked.id
		RETURNING jobs.id, jobs.camera_id, jobs.source_event_id, jobs.job_type,
			jobs.priority, jobs.frame_path, jobs.metadata_json::text, jobs.attempts
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]aiJob, 0, limit)
	for rows.Next() {
		var job aiJob
		if err := rows.Scan(
			&job.ID,
			&job.CameraID,
			&job.SourceEventID,
			&job.JobType,
			&job.Priority,
			&job.FramePath,
			&job.MetadataJSON,
			&job.Attempts,
		); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (w *worker) processJob(ctx context.Context, job aiJob) {
	handler, ok := w.handlers[job.JobType]
	if !ok {
		w.failOrRetryJob(ctx, job, fmt.Errorf("unsupported analytics job type %q", job.JobType))
		return
	}

	log.Printf("analytics job start worker_id=%s job_id=%s camera_id=%s job_type=%s attempt=%d",
		w.cfg.workerID, job.ID, job.CameraID, job.JobType, job.Attempts+1)

	if err := handler.Handle(ctx, job); err != nil {
		w.failOrRetryJob(ctx, job, err)
		return
	}

	dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := w.db.ExecContext(dbCtx, `
		UPDATE ai_jobs
		SET status = 'completed',
			completed_at = now(),
			error_message = NULL
		WHERE id = $1
	`, job.ID); err != nil {
		log.Printf("analytics job completion update failed job_id=%s camera_id=%s job_type=%s error=%v",
			job.ID, job.CameraID, job.JobType, err)
		return
	}
	log.Printf("analytics job completed worker_id=%s job_id=%s camera_id=%s job_type=%s",
		w.cfg.workerID, job.ID, job.CameraID, job.JobType)
}

func (w *worker) failOrRetryJob(ctx context.Context, job aiJob, jobErr error) {
	nextAttempts := job.Attempts + 1
	status := "pending"
	if nextAttempts >= maxJobAttempts {
		status = "failed"
	}

	dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := w.db.ExecContext(dbCtx, `
		UPDATE ai_jobs
		SET status = $2,
			attempts = attempts + 1,
			error_message = $3,
			completed_at = CASE WHEN $2 = 'failed' THEN now() ELSE completed_at END
		WHERE id = $1
	`, job.ID, status, jobErr.Error()); err != nil {
		log.Printf("analytics job failure update failed job_id=%s camera_id=%s job_type=%s error=%v",
			job.ID, job.CameraID, job.JobType, err)
		return
	}

	log.Printf("analytics job %s worker_id=%s job_id=%s camera_id=%s job_type=%s attempts=%d error=%v",
		status, w.cfg.workerID, job.ID, job.CameraID, job.JobType, nextAttempts, jobErr)
}

func placeholderHandler(jobType string) jobHandler {
	return handlerFunc(func(ctx context.Context, job aiJob) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		log.Printf("analytics placeholder handler job_id=%s camera_id=%s job_type=%s inference=disabled",
			job.ID, job.CameraID, jobType)
		return nil
	})
}

func loadConfig() config {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatalf("DATABASE_URL is required")
	}

	return config{
		databaseURL:       databaseURL,
		workerID:          getenv("ANALYTICS_WORKER_ID", "analytics-1"),
		pollInterval:      getenvDurationMilliseconds("AI_POLL_INTERVAL_MS", 1000*time.Millisecond),
		maxConcurrentJobs: getenvPositiveInt("AI_MAX_CONCURRENT_JOBS", 1),
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvPositiveInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func getenvDurationMilliseconds(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	milliseconds, err := strconv.Atoi(value)
	if err != nil || milliseconds <= 0 {
		return fallback
	}
	return time.Duration(milliseconds) * time.Millisecond
}
