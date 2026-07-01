package httpapi

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
	"strings"
	"sync"
	"syscall"
	"time"
)

var hlsURLLogPattern = regexp.MustCompile(`(?i)(rtsp|tcp)://[^\s"']+`)

type liveCamera struct {
	ID      string
	Name    string
	RTSPURL string
	Enabled bool
}

type hlsManager struct {
	db         *sql.DB
	root       string
	inactivity time.Duration
	mu         sync.Mutex
	streams    map[string]*hlsStream
}

type hlsStream struct {
	camera   liveCamera
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	started  time.Time
	lastUsed time.Time
}

func newHLSManager(db *sql.DB, root string, inactivity time.Duration) *hlsManager {
	return &hlsManager{
		db:         db,
		root:       root,
		inactivity: inactivity,
		streams:    make(map[string]*hlsStream),
	}
}

func (m *hlsManager) ensureStream(ctx context.Context, camera liveCamera) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if stream, ok := m.streams[camera.ID]; ok && stream.cmd.Process != nil {
		stream.lastUsed = time.Now()
		return "/hls/" + camera.ID + "/index.m3u8", nil
	}

	outputDir := filepath.Join(m.root, camera.ID)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create hls directory: %w", err)
	}
	_ = cleanupDirectory(outputDir)

	childCtx, cancel := context.WithCancel(context.Background())
	playlist := filepath.Join(outputDir, "index.m3u8")
	args := []string{
		"-hide_banner",
		"-loglevel", "warning",
		"-rtsp_transport", "tcp",
		"-i", camera.RTSPURL,
		"-an",
		"-c:v", "copy",
		"-f", "hls",
		"-hls_time", "2",
		"-hls_list_size", "6",
		"-hls_flags", "delete_segments+append_list+omit_endlist",
		"-hls_segment_filename", filepath.Join(outputDir, "segment_%05d.ts"),
		playlist,
	}
	cmd := exec.CommandContext(childCtx, "ffmpeg", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stderr := &limitedHLSBuffer{max: 8192}
	cmd.Stderr = stderr

	log.Printf("hls ffmpeg starting camera_id=%s camera_name=%q output=%s", camera.ID, camera.Name, playlist)
	if err := cmd.Start(); err != nil {
		cancel()
		_ = insertSystemEvent(ctx, m.db, eventRecord{EventType: "live.failure", EntityType: "camera", EntityID: &camera.ID, Severity: "error", Message: "live stream failed to start", Metadata: map[string]any{"camera_name": camera.Name}})
		return "", fmt.Errorf("start hls ffmpeg: %w", err)
	}

	stream := &hlsStream{camera: camera, cmd: cmd, cancel: cancel, started: time.Now(), lastUsed: time.Now()}
	m.streams[camera.ID] = stream
	_ = insertSystemEvent(ctx, m.db, eventRecord{EventType: "live.start", EntityType: "camera", EntityID: &camera.ID, Message: "live stream started", Metadata: map[string]any{"camera_name": camera.Name}})
	go func() {
		err := cmd.Wait()
		output := sanitizeHLSLog(stderr.String())
		m.mu.Lock()
		if current, ok := m.streams[camera.ID]; ok && current == stream {
			delete(m.streams, camera.ID)
		}
		m.mu.Unlock()
		if err != nil && childCtx.Err() == nil {
			log.Printf("hls ffmpeg exited camera_id=%s camera_name=%q error=%v stderr=%q", camera.ID, camera.Name, err, output)
			_ = insertSystemEvent(context.Background(), m.db, eventRecord{EventType: "live.failure", EntityType: "camera", EntityID: &camera.ID, Severity: "error", Message: "live stream ffmpeg exited unexpectedly", Metadata: map[string]any{"camera_name": camera.Name, "error": err.Error(), "stderr": output}})
		} else {
			log.Printf("hls ffmpeg stopped camera_id=%s camera_name=%q stderr=%q", camera.ID, camera.Name, output)
			_ = insertSystemEvent(context.Background(), m.db, eventRecord{EventType: "live.stop", EntityType: "camera", EntityID: &camera.ID, Message: "live stream stopped", Metadata: map[string]any{"camera_name": camera.Name}})
		}
	}()

	if err := waitForHLSReady(ctx, playlist, 12*time.Second); err != nil {
		delete(m.streams, camera.ID)
		stopHLSProcess(stream)
		_ = insertSystemEvent(ctx, m.db, eventRecord{EventType: "live.failure", EntityType: "camera", EntityID: &camera.ID, Severity: "error", Message: "live stream playlist was not ready", Metadata: map[string]any{"camera_name": camera.Name, "error": err.Error()}})
		return "", err
	}

	return "/hls/" + camera.ID + "/index.m3u8", nil
}

func waitForHLSReady(ctx context.Context, playlist string, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		if hlsPlaylistReady(playlist) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("hls playlist was not ready before timeout")
		case <-ticker.C:
		}
	}
}

func hlsPlaylistReady(playlist string) bool {
	data, err := os.ReadFile(playlist)
	if err != nil {
		return false
	}
	body := string(data)
	if !strings.Contains(body, "#EXTM3U") || !strings.Contains(body, ".ts") {
		return false
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, ".ts") {
			if _, err := os.Stat(filepath.Join(filepath.Dir(playlist), line)); err == nil {
				return true
			}
		}
	}
	return false
}

func (m *hlsManager) touch(cameraID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if stream, ok := m.streams[cameraID]; ok {
		stream.lastUsed = time.Now()
	}
}

func (m *hlsManager) stop(cameraID string) {
	m.mu.Lock()
	stream := m.streams[cameraID]
	delete(m.streams, cameraID)
	m.mu.Unlock()
	if stream != nil {
		stopHLSProcess(stream)
	}
}

func (m *hlsManager) stopAll() {
	m.mu.Lock()
	streams := m.streams
	m.streams = make(map[string]*hlsStream)
	m.mu.Unlock()
	for _, stream := range streams {
		stopHLSProcess(stream)
	}
}

func (m *hlsManager) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.cleanupStale()
		}
	}
}

func (m *hlsManager) cleanupStale() {
	var stale []string
	now := time.Now()
	m.mu.Lock()
	for cameraID, stream := range m.streams {
		if now.Sub(stream.lastUsed) > m.inactivity || !m.cameraStillEnabled(cameraID) {
			stale = append(stale, cameraID)
		}
	}
	m.mu.Unlock()
	for _, cameraID := range stale {
		log.Printf("hls stream stopping camera_id=%s reason=inactive_or_disabled", cameraID)
		m.stop(cameraID)
		_ = os.RemoveAll(filepath.Join(m.root, cameraID))
	}
}

func (m *hlsManager) cameraStillEnabled(cameraID string) bool {
	var enabled bool
	if err := m.db.QueryRow(`SELECT is_enabled FROM cameras WHERE id = $1`, cameraID).Scan(&enabled); err != nil {
		return false
	}
	return enabled
}

func stopHLSProcess(stream *hlsStream) {
	if stream.cancel != nil {
		stream.cancel()
	}
	if stream.cmd != nil && stream.cmd.Process != nil {
		_ = syscall.Kill(-stream.cmd.Process.Pid, syscall.SIGTERM)
	}
}

func cleanupDirectory(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(path, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func sanitizeHLSLog(value string) string {
	return hlsURLLogPattern.ReplaceAllString(value, "$1://<redacted>")
}

type limitedHLSBuffer struct {
	bytes.Buffer
	max int
}

func (b *limitedHLSBuffer) Write(p []byte) (int, error) {
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
