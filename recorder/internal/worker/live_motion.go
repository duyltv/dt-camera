package worker

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const (
	liveMotionFPS           = 4
	liveMotionBufferSeconds = 130
	liveMotionPostSeconds   = 3
	liveMotionPreSeconds    = 7
	liveMotionWidth         = 640
)

type liveMotionFrame struct {
	At          time.Time
	JPEG        []byte
	Fingerprint []uint8
}

type liveMotionDetector struct {
	job       *recordingJob
	frames    []liveMotionFrame
	lastFrame *liveMotionFrame

	motionStartedAt *time.Time
	nextEventAfter  time.Time

	mu sync.Mutex
}

func (j *recordingJob) runLiveMotionDetector(ctx context.Context) {
	detector := &liveMotionDetector{job: j}
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		if err := detector.runFFmpeg(ctx); err != nil && ctx.Err() == nil {
			log.Printf("live motion detector stopped camera_id=%s camera_name=%q backoff=%s error=%v", j.camera.ID, j.camera.Name, backoff, sanitizeLog(err.Error()))
			_ = insertSystemEvent(ctx, j.db, "motion.live_failure", "camera", &j.camera.ID, "warning", "live motion detector failed", map[string]any{"camera_name": j.camera.Name, "error": sanitizeLog(err.Error())})
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > j.maxBackoff {
				backoff = j.maxBackoff
			}
			continue
		}
		return
	}
}

func (d *liveMotionDetector) runFFmpeg(ctx context.Context) error {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-rtsp_transport", "tcp",
		"-i", d.job.camera.RTSPURL,
		"-an",
		"-vf", fmt.Sprintf("fps=%d,scale='min(%d,iw)':-2", liveMotionFPS, liveMotionWidth),
		"-q:v", "6",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"pipe:1",
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("motion stdout pipe: %w", err)
	}
	stderr := &limitedBuffer{max: 4096}
	cmd.Stderr = stderr
	log.Printf("live motion detector starting camera_id=%s camera_name=%q fps=%d", d.job.camera.ID, d.job.camera.Name, liveMotionFPS)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start live motion ffmpeg: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	reader := bufio.NewReader(stdout)
	for {
		img, err := jpeg.Decode(reader)
		if err != nil {
			stopProcessGroup(cmd.Process.Pid)
			select {
			case waitErr := <-done:
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if waitErr != nil {
					return fmt.Errorf("live motion ffmpeg exited: %w stderr=%s", waitErr, sanitizeLog(truncate(stderr.String(), 800)))
				}
				return fmt.Errorf("live motion stream ended: %w", err)
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				<-done
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("live motion decode failed: %w", err)
			}
		}
		if ctx.Err() != nil {
			stopProcessGroup(cmd.Process.Pid)
			<-done
			return ctx.Err()
		}
		if err := d.ingestFrame(ctx, img, time.Now().UTC()); err != nil {
			log.Printf("live motion frame skipped camera_id=%s camera_name=%q error=%v", d.job.camera.ID, d.job.camera.Name, err)
		}
	}
}

func (d *liveMotionDetector) ingestFrame(ctx context.Context, img image.Image, at time.Time) error {
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 72}); err != nil {
		return err
	}
	frame := liveMotionFrame{
		At:          at,
		JPEG:        encoded.Bytes(),
		Fingerprint: frameFingerprint(img),
	}

	var trigger bool
	var score float64
	d.mu.Lock()
	if d.lastFrame != nil {
		score = frameDifference(d.lastFrame.Fingerprint, frame.Fingerprint)
	}
	d.frames = append(d.frames, frame)
	d.trimLocked(at)

	threshold := d.job.camera.MotionSensitivity
	if threshold <= 0 {
		threshold = 0.35
	}
	minDuration := time.Duration(d.job.camera.MotionMinDurationSeconds) * time.Second
	if score >= threshold && at.After(d.nextEventAfter) {
		if d.motionStartedAt == nil {
			started := at
			d.motionStartedAt = &started
		}
		if minDuration <= 0 || at.Sub(*d.motionStartedAt) >= minDuration {
			trigger = true
			d.nextEventAfter = at.Add(5 * time.Second)
			d.motionStartedAt = nil
		}
	} else if score < threshold {
		d.motionStartedAt = nil
	}
	d.lastFrame = &frame
	d.mu.Unlock()

	if trigger {
		log.Printf("live motion detected camera_id=%s camera_name=%q score=%.3f", d.job.camera.ID, d.job.camera.Name, score)
		activeUntil := at.Add(liveMotionPostSeconds * time.Second)
		event := MotionEvent{
			CameraID:   d.job.camera.ID,
			OccurredAt: at,
			Score:      score,
		}
		eventID, err := insertMotionEventWithMetadata(ctx, d.job.db, event, map[string]any{
			"active_until": activeUntil.Format(time.RFC3339Nano),
			"source":       "live_motion_detector",
		})
		if err != nil {
			log.Printf("live motion event insert failed camera_id=%s camera_name=%q error=%v", d.job.camera.ID, d.job.camera.Name, err)
			return nil
		}
		event.ID = eventID
		_ = insertSystemEvent(ctx, d.job.db, "motion.detected", "camera", &d.job.camera.ID, "info", "live motion detected", map[string]any{"camera_name": d.job.camera.Name, "score": score, "motion_event_id": eventID})
		go d.finalizeLiveMotionEvent(context.Background(), eventID, event, activeUntil)
	}
	return nil
}

func (d *liveMotionDetector) finalizeLiveMotionEvent(ctx context.Context, eventID string, event MotionEvent, activeUntil time.Time) {
	time.Sleep(time.Until(activeUntil))
	frames := d.framesAround(event.OccurredAt, liveMotionPreSeconds*time.Second, liveMotionPostSeconds*time.Second)
	if len(frames) == 0 {
		log.Printf("live motion evidence missing camera_id=%s camera_name=%q", d.job.camera.ID, d.job.camera.Name)
		return
	}

	evidenceDir := filepath.Join(cameraRoot(d.job.camera.StoragePath, d.job.camera.ID), "events", event.OccurredAt.UTC().Format("2006/01/02"))
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		log.Printf("live motion evidence directory failed camera_id=%s camera_name=%q error=%v", d.job.camera.ID, d.job.camera.Name, err)
		return
	}
	base := fmt.Sprintf("motion_%s_%s", d.job.camera.ID, event.OccurredAt.UTC().Format("20060102T150405Z"))
	imagePath := filepath.Join(evidenceDir, base+".jpg")
	videoPath := filepath.Join(evidenceDir, base+".mp4")

	snapshot := closestFrame(frames, event.OccurredAt)
	if snapshot != nil {
		if err := os.WriteFile(imagePath, snapshot.JPEG, 0o644); err != nil {
			log.Printf("live motion snapshot failed camera_id=%s camera_name=%q error=%v", d.job.camera.ID, d.job.camera.Name, err)
			imagePath = ""
		}
	}
	if err := renderFrameClip(ctx, frames, liveMotionFPS, videoPath); err != nil {
		log.Printf("live motion clip failed camera_id=%s camera_name=%q error=%v", d.job.camera.ID, d.job.camera.Name, err)
		videoPath = ""
	}

	event.ImagePath = imagePath
	event.VideoPath = videoPath
	if err := updateMotionEventEvidence(ctx, d.job.db, eventID, imagePath, videoPath); err != nil {
		log.Printf("live motion evidence update failed camera_id=%s camera_name=%q error=%v", d.job.camera.ID, d.job.camera.Name, err)
		return
	}
	event.ID = eventID
	d.job.enqueueMotionAnalytics(ctx, event)
}

func (d *liveMotionDetector) framesAround(occurredAt time.Time, pre, post time.Duration) []liveMotionFrame {
	start := occurredAt.Add(-pre)
	end := occurredAt.Add(post)
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]liveMotionFrame, 0, len(d.frames))
	for _, frame := range d.frames {
		if !frame.At.Before(start) && !frame.At.After(end) {
			out = append(out, frame)
		}
	}
	return out
}

func (d *liveMotionDetector) trimLocked(now time.Time) {
	cutoff := now.Add(-liveMotionBufferSeconds * time.Second)
	keepFrom := 0
	for keepFrom < len(d.frames) && d.frames[keepFrom].At.Before(cutoff) {
		keepFrom++
	}
	if keepFrom > 0 {
		d.frames = append([]liveMotionFrame(nil), d.frames[keepFrom:]...)
	}
}

func frameFingerprint(img image.Image) []uint8 {
	const cols = 16
	const rows = 12
	bounds := img.Bounds()
	out := make([]uint8, cols*rows)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			px := bounds.Min.X + (x*bounds.Dx()+bounds.Dx()/2)/cols
			py := bounds.Min.Y + (y*bounds.Dy()+bounds.Dy()/2)/rows
			gray := color.GrayModel.Convert(img.At(px, py)).(color.Gray)
			out[y*cols+x] = gray.Y
		}
	}
	return out
}

func frameDifference(a, b []uint8) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var total int
	for i := range a {
		delta := int(a[i]) - int(b[i])
		if delta < 0 {
			delta = -delta
		}
		total += delta
	}
	normalized := (float64(total) / float64(len(a)*255)) * 4
	if normalized > 1 {
		return 1
	}
	return normalized
}

func closestFrame(frames []liveMotionFrame, target time.Time) *liveMotionFrame {
	if len(frames) == 0 {
		return nil
	}
	best := &frames[0]
	bestDistance := absDuration(frames[0].At.Sub(target))
	for i := 1; i < len(frames); i++ {
		if distance := absDuration(frames[i].At.Sub(target)); distance < bestDistance {
			best = &frames[i]
			bestDistance = distance
		}
	}
	return best
}

func renderFrameClip(ctx context.Context, frames []liveMotionFrame, fps int, output string) error {
	if fps <= 0 {
		fps = liveMotionFPS
	}
	tempDir, err := os.MkdirTemp(filepath.Dir(output), ".motion-frames-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	for i, frame := range frames {
		name := filepath.Join(tempDir, fmt.Sprintf("frame_%06d.jpg", i+1))
		if err := os.WriteFile(name, frame.JPEG, 0o644); err != nil {
			return err
		}
	}
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-framerate", fmt.Sprintf("%d", fps),
		"-i", filepath.Join(tempDir, "frame_%06d.jpg"),
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "30",
		"-pix_fmt", "yuv420p",
		"-movflags", "+faststart",
		"-y", output,
	}
	outputBytes, err := exec.CommandContext(cmdCtx, "ffmpeg", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg frame clip: %s", sanitizeLog(truncate(string(outputBytes), 700)))
	}
	return nil
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
