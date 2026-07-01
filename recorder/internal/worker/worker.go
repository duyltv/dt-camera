package worker

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"
)

const recorderVersion = "phase-4"

type Worker struct {
	db   *sql.DB
	cfg  Config
	jobs map[string]*recordingJob
}

func New(db *sql.DB, cfg Config) *Worker {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.SegmentDuration <= 0 {
		cfg.SegmentDuration = 60 * time.Second
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 60 * time.Second
	}
	if cfg.StableFileAge <= 0 {
		cfg.StableFileAge = 8 * time.Second
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 10 * time.Second
	}
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = 60 * time.Second
	}
	if cfg.LowDiskFreePercent <= 0 {
		cfg.LowDiskFreePercent = 10
	}
	if cfg.LowDiskMinFileAge <= 0 {
		cfg.LowDiskMinFileAge = time.Hour
	}
	if cfg.WorkerID == "" {
		cfg.WorkerID = "recorder-1"
	}

	return &Worker{
		db:   db,
		cfg:  cfg,
		jobs: make(map[string]*recordingJob),
	}
}

func (w *Worker) Run(ctx context.Context) error {
	log.Printf("recorder running worker_id=%s", w.cfg.WorkerID)
	log.Printf("recordings path mounted at %s", w.cfg.RecordingsPath)
	log.Printf("poll_interval=%s segment_duration=%s max_backoff=%s", w.cfg.PollInterval, w.cfg.SegmentDuration, w.cfg.MaxBackoff)

	if err := pingDatabase(ctx, w.db); err != nil {
		return err
	}
	if err := ensureWritableDirectory(w.cfg.RecordingsPath); err != nil {
		return err
	}

	if err := w.reconcile(ctx); err != nil {
		log.Printf("initial reconcile failed: %v", err)
	}
	_ = w.writeHeartbeat(ctx, "running")

	pollTicker := time.NewTicker(w.cfg.PollInterval)
	defer pollTicker.Stop()
	heartbeatTicker := time.NewTicker(w.cfg.HeartbeatInterval)
	defer heartbeatTicker.Stop()
	cleanupTicker := time.NewTicker(w.cfg.CleanupInterval)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("recorder shutting down")
			w.stopAll()
			_ = w.writeHeartbeat(context.Background(), "stopped")
			return nil
		case <-pollTicker.C:
			if err := w.reconcile(ctx); err != nil {
				log.Printf("reconcile failed: %v", err)
			}
		case <-heartbeatTicker.C:
			if err := w.writeHeartbeat(ctx, "running"); err != nil {
				log.Printf("heartbeat failed: %v", err)
			}
		case <-cleanupTicker.C:
			if err := w.runCleanup(ctx); err != nil {
				log.Printf("retention cleanup failed: %v", err)
			}
		}
	}
}

func (w *Worker) reconcile(ctx context.Context) error {
	cameras, err := fetchEnabledCameras(ctx, w.db)
	if err != nil {
		return err
	}

	desired := make(map[string]Camera, len(cameras))
	for _, camera := range cameras {
		if camera.StoragePath == "" {
			log.Printf("camera skipped without storage path camera_id=%s camera_name=%q", camera.ID, camera.Name)
			continue
		}
		if err := ensureWritableDirectory(camera.StoragePath); err != nil {
			log.Printf("camera skipped with invalid storage camera_id=%s camera_name=%q storage=%s error=%v", camera.ID, camera.Name, camera.StoragePath, err)
			continue
		}
		desired[camera.ID] = camera
	}

	for cameraID, job := range w.jobs {
		camera, ok := desired[cameraID]
		if !ok {
			log.Printf("stopping recorder job camera_id=%s camera_name=%q reason=disabled_or_deleted", job.camera.ID, job.camera.Name)
			job.stop()
			delete(w.jobs, cameraID)
			continue
		}
		if job.configKey != camera.ConfigKey() {
			log.Printf("restarting recorder job camera_id=%s camera_name=%q reason=config_changed", job.camera.ID, job.camera.Name)
			_ = insertSystemEvent(ctx, w.db, "recorder.job_restart", "recorder", &cameraID, "info", "recorder job restarting because configuration changed", map[string]any{"camera_name": job.camera.Name, "worker_id": w.cfg.WorkerID})
			job.stop()
			delete(w.jobs, cameraID)
		}
	}

	for cameraID, camera := range desired {
		if _, ok := w.jobs[cameraID]; ok {
			continue
		}
		job := newRecordingJob(w.db, camera, w.cfg)
		w.jobs[cameraID] = job
		job.start(ctx)
	}

	return w.writeHeartbeat(ctx, "running")
}

func (w *Worker) stopAll() {
	for cameraID, job := range w.jobs {
		job.stop()
		delete(w.jobs, cameraID)
	}
}

func (w *Worker) writeHeartbeat(ctx context.Context, status string) error {
	return upsertHeartbeat(ctx, w.db, w.cfg.WorkerID, recorderVersion, status, len(w.jobs))
}

func (w *Worker) runCleanup(ctx context.Context) error {
	cleaner := cleanupRunner{
		db:                 w.db,
		now:                time.Now().UTC(),
		lowDiskFreePercent: w.cfg.LowDiskFreePercent,
		lowDiskMinFileAge:  w.cfg.LowDiskMinFileAge,
	}
	return cleaner.run(ctx)
}

func ensureWritableDirectory(path string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory")
	}

	temp, err := os.CreateTemp(path, ".dt-camera-recorder-write-test-*")
	if err != nil {
		return fmt.Errorf("path is not writable: %w", err)
	}
	name := temp.Name()
	if err := temp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("close write test file: %w", err)
	}
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("remove write test file: %w", err)
	}
	return nil
}
