package worker

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"
	"time"
)

var streamURLLogPattern = regexp.MustCompile(`(?i)(rtsp|tcp)://[^\s"']+`)

type recordingJob struct {
	db             *sql.DB
	workerID       string
	camera         Camera
	configKey      string
	segmentSeconds int
	stableAge      time.Duration
	stableChecks   int
	maxBackoff     time.Duration

	cancel context.CancelFunc
	done   chan struct{}

	mu           sync.Mutex
	knownFiles   map[string]trackedFile
	retryAttempt int
	nextRetry    time.Time
	lastErr      string
}

func newRecordingJob(db *sql.DB, camera Camera, cfg Config) *recordingJob {
	segmentSeconds := int(cfg.SegmentDuration.Seconds())
	if segmentSeconds <= 0 {
		segmentSeconds = 60
	}
	stableChecks := cfg.StableFileChecks
	if stableChecks <= 0 {
		stableChecks = 2
	}

	return &recordingJob{
		db:             db,
		workerID:       cfg.WorkerID,
		camera:         camera,
		configKey:      camera.ConfigKey(),
		segmentSeconds: segmentSeconds,
		stableAge:      cfg.StableFileAge,
		stableChecks:   stableChecks,
		maxBackoff:     cfg.MaxBackoff,
		done:           make(chan struct{}),
		knownFiles:     make(map[string]trackedFile),
	}
}

func (j *recordingJob) start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	j.cancel = cancel
	go j.run(ctx)
}

func (j *recordingJob) stop() {
	if j.cancel != nil {
		j.cancel()
	}
	<-j.done
}

func (j *recordingJob) run(ctx context.Context) {
	defer close(j.done)

	log.Printf("recorder job starting camera_id=%s camera_name=%q", j.camera.ID, j.camera.Name)
	started := time.Now().UTC()
	_ = upsertRecorderJob(ctx, j.db, j.workerID, j.camera, "running", &started, nil, "")
	_ = insertSystemEvent(ctx, j.db, "recorder.job_start", "recorder", &j.camera.ID, "info", "recorder job started", map[string]any{"camera_name": j.camera.Name, "worker_id": j.workerID})
	if j.camera.MotionDetectionEnabled {
		go j.runLiveMotionDetector(ctx)
	}

	scanTicker := time.NewTicker(5 * time.Second)
	defer scanTicker.Stop()

	for {
		if err := j.scanSegments(ctx); err != nil {
			log.Printf("segment scan failed camera_id=%s camera_name=%q error=%v", j.camera.ID, j.camera.Name, err)
		}

		wait := time.Until(j.nextRetryTime())
		if wait > 0 {
			select {
			case <-ctx.Done():
				log.Printf("recorder job stopping camera_id=%s camera_name=%q", j.camera.ID, j.camera.Name)
				stopped := time.Now().UTC()
				_ = upsertRecorderJob(context.Background(), j.db, j.workerID, j.camera, "stopped", nil, &stopped, "")
				_ = insertSystemEvent(context.Background(), j.db, "recorder.job_stop", "recorder", &j.camera.ID, "info", "recorder job stopped", map[string]any{"camera_name": j.camera.Name, "worker_id": j.workerID})
				return
			case <-time.After(wait):
			case <-scanTicker.C:
				continue
			}
		}

		if err := j.runFFmpeg(ctx); err != nil {
			if ctx.Err() != nil {
				_ = j.scanSegments(context.Background())
				log.Printf("recorder job stopped camera_id=%s camera_name=%q", j.camera.ID, j.camera.Name)
				stopped := time.Now().UTC()
				_ = upsertRecorderJob(context.Background(), j.db, j.workerID, j.camera, "stopped", nil, &stopped, "")
				_ = insertSystemEvent(context.Background(), j.db, "recorder.job_stop", "recorder", &j.camera.ID, "info", "recorder job stopped", map[string]any{"camera_name": j.camera.Name, "worker_id": j.workerID})
				return
			}
			j.scheduleRetry(err)
			continue
		}
	}
}

func (j *recordingJob) runFFmpeg(ctx context.Context) error {
	outputPattern := segmentPattern(j.camera.StoragePath, j.camera.ID, time.Now())
	if err := os.MkdirAll(filepath.Dir(outputPattern), 0o755); err != nil {
		return fmt.Errorf("create recording directory: %w", err)
	}

	args := []string{
		"-hide_banner",
		"-loglevel", "warning",
		"-fflags", "+genpts",
		"-use_wallclock_as_timestamps", "1",
		"-rtsp_transport", "tcp",
		"-i", j.camera.RTSPURL,
		"-map", "0:v:0",
		"-c:v", "copy",
	}
	if j.camera.RecordAudio {
		args = append(args, "-map", "0:a:0?", "-c:a", "aac", "-b:a", "96k", "-af", "aresample=async=1:first_pts=0")
	} else {
		args = append(args, "-an")
	}
	args = append(args,
		"-max_interleave_delta", "0",
		"-muxdelay", "0",
		"-avoid_negative_ts", "make_zero",
		"-f", "segment",
		"-segment_time", fmt.Sprintf("%d", j.segmentSeconds),
		"-reset_timestamps", "1",
		"-strftime", "1",
		outputPattern,
	)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stderr := &limitedBuffer{max: 8192}
	cmd.Stderr = stderr

	log.Printf("ffmpeg starting camera_id=%s camera_name=%q segment_seconds=%d output=%s", j.camera.ID, j.camera.Name, j.segmentSeconds, outputPattern)
	if err := cmd.Start(); err != nil {
		_ = upsertRecorderJob(ctx, j.db, j.workerID, j.camera, "failed", nil, nil, sanitizeLog(err.Error()))
		_ = insertSystemEvent(ctx, j.db, "recorder.job_failure", "recorder", &j.camera.ID, "error", "recorder ffmpeg failed to start", map[string]any{"camera_name": j.camera.Name, "worker_id": j.workerID, "error": sanitizeLog(err.Error())})
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	scanTicker := time.NewTicker(5 * time.Second)
	defer scanTicker.Stop()
	dayRollover := time.NewTimer(untilNextDay(time.Now()))
	defer dayRollover.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("ffmpeg stopping camera_id=%s camera_name=%q", j.camera.ID, j.camera.Name)
			stopProcessGroup(cmd.Process.Pid)
			select {
			case <-done:
			case <-time.After(8 * time.Second):
				_ = cmd.Process.Kill()
				<-done
			}
			return ctx.Err()
		case err := <-done:
			output := sanitizeLog(truncate(stderr.String(), 2000))
			if err != nil {
				log.Printf("ffmpeg exited camera_id=%s camera_name=%q error=%v stderr=%q", j.camera.ID, j.camera.Name, err, output)
				_ = upsertRecorderJob(context.Background(), j.db, j.workerID, j.camera, "failed", nil, nil, sanitizeLog(err.Error()))
				_ = insertSystemEvent(context.Background(), j.db, "recorder.job_failure", "recorder", &j.camera.ID, "error", "recorder ffmpeg exited unexpectedly", map[string]any{"camera_name": j.camera.Name, "worker_id": j.workerID, "error": sanitizeLog(err.Error()), "stderr": output})
				return fmt.Errorf("ffmpeg exited: %w", err)
			}
			log.Printf("ffmpeg exited camera_id=%s camera_name=%q stderr=%q", j.camera.ID, j.camera.Name, output)
			return nil
		case <-scanTicker.C:
			if err := j.scanSegments(ctx); err != nil {
				log.Printf("segment scan failed camera_id=%s camera_name=%q error=%v", j.camera.ID, j.camera.Name, err)
			}
		case <-dayRollover.C:
			log.Printf("ffmpeg restarting for daily folder rollover camera_id=%s camera_name=%q", j.camera.ID, j.camera.Name)
			stopProcessGroup(cmd.Process.Pid)
			select {
			case <-done:
			case <-time.After(8 * time.Second):
				_ = cmd.Process.Kill()
				<-done
			}
			_ = j.scanSegments(ctx)
			return nil
		}
	}
}

func (j *recordingJob) scanSegments(ctx context.Context) error {
	root := cameraRoot(j.camera.StoragePath, j.camera.ID)
	files, err := listSegmentFiles(root)
	if err != nil {
		return err
	}

	now := time.Now()
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		j.mu.Lock()
		state := j.knownFiles[path]
		if state.Inserted {
			j.mu.Unlock()
			continue
		}
		if state.Size == info.Size() && state.ModTime.Equal(info.ModTime()) {
			state.StableChecks++
		} else {
			state.Size = info.Size()
			state.ModTime = info.ModTime()
			state.StableChecks = 1
		}
		j.knownFiles[path] = state
		ready := state.StableChecks >= j.stableChecks && now.Sub(info.ModTime()) >= j.stableAge
		j.mu.Unlock()

		if !ready {
			continue
		}

		metadata, err := parseSegmentMetadata(path, j.camera, time.Duration(j.segmentSeconds)*time.Second)
		if err != nil {
			log.Printf("segment metadata skipped camera_id=%s camera_name=%q file=%s error=%v", j.camera.ID, j.camera.Name, path, err)
			continue
		}
		if err := validateSegmentFile(ctx, path); err != nil {
			log.Printf("segment file invalid camera_id=%s camera_name=%q file=%s error=%v", j.camera.ID, j.camera.Name, path, err)
			_ = os.Remove(path)
			j.mu.Lock()
			state.Inserted = true
			j.knownFiles[path] = state
			j.mu.Unlock()
			continue
		}

		segmentID, err := insertSegmentWithID(ctx, j.db, metadata)
		if err != nil {
			return err
		}

		j.mu.Lock()
		state.Inserted = true
		j.knownFiles[path] = state
		j.mu.Unlock()

		log.Printf("segment metadata inserted camera_id=%s camera_name=%q file=%s size_bytes=%d", j.camera.ID, j.camera.Name, metadata.FilePath, metadata.SizeBytes)
		_ = segmentID
	}
	return nil
}

func validateSegmentFile(ctx context.Context, path string) error {
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, "ffprobe", "-hide_banner", "-v", "error", "-show_format", "-show_streams", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffprobe failed: %s", sanitizeLog(truncate(string(output), 500)))
	}
	return nil
}

func (j *recordingJob) scheduleRetry(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.lastErr = err.Error()
	j.retryAttempt++
	backoff := time.Second * time.Duration(1<<min(j.retryAttempt-1, 5))
	if backoff > j.maxBackoff {
		backoff = j.maxBackoff
	}
	j.nextRetry = time.Now().Add(backoff)

	log.Printf("recording retry scheduled camera_id=%s camera_name=%q backoff=%s error=%v", j.camera.ID, j.camera.Name, backoff, err)
	_ = upsertRecorderJob(context.Background(), j.db, j.workerID, j.camera, "retrying", nil, nil, sanitizeLog(err.Error()))
}

func (j *recordingJob) nextRetryTime() time.Time {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.nextRetry
}

func stopProcessGroup(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func sanitizeLog(value string) string {
	return streamURLLogPattern.ReplaceAllString(value, "$1://<redacted>")
}

type limitedBuffer struct {
	bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.max <= 0 {
		return len(p), nil
	}
	remaining := b.max - b.Buffer.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		return len(p), nil
	}
	_, _ = b.Buffer.Write(p)
	return len(p), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
