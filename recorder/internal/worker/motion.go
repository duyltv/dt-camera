package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var motionMetadataPattern = regexp.MustCompile(`lavfi\.scene_score=([0-9.]+)`)
var motionTimePattern = regexp.MustCompile(`pts_time:([0-9.]+)`)

func (j *recordingJob) processMotion(ctx context.Context, segmentID string, segment SegmentMetadata) {
	if !j.camera.MotionDetectionEnabled {
		return
	}
	threshold := j.camera.MotionSensitivity
	if threshold <= 0 {
		threshold = 0.35
	}
	detection, ok, err := detectMotionInSegment(ctx, segment.FilePath, threshold, j.camera.MotionMinDurationSeconds)
	if err != nil {
		log.Printf("motion detection failed camera_id=%s camera_name=%q error=%v", j.camera.ID, j.camera.Name, err)
		_ = insertSystemEvent(ctx, j.db, "motion.detect_failure", "camera", &j.camera.ID, "warning", "motion detection failed", map[string]any{"camera_name": j.camera.Name, "error": sanitizeLog(err.Error())})
		return
	}
	if !ok {
		return
	}

	occurredAt := segment.StartTime.Add(time.Duration(detection.OffsetSeconds * float64(time.Second)))
	event := MotionEvent{
		CameraID:           j.camera.ID,
		RecordingSegmentID: segmentID,
		OccurredAt:         occurredAt,
		Score:              detection.Score,
	}

	evidenceDir := filepath.Join(cameraRoot(j.camera.StoragePath, j.camera.ID), "events", occurredAt.UTC().Format("2006/01/02"))
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		log.Printf("motion evidence directory failed camera_id=%s camera_name=%q error=%v", j.camera.ID, j.camera.Name, err)
		return
	}
	base := fmt.Sprintf("motion_%s_%s", j.camera.ID, occurredAt.UTC().Format("20060102T150405Z"))
	imagePath := filepath.Join(evidenceDir, base+".jpg")
	videoPath := filepath.Join(evidenceDir, base+".mp4")

	if err := createMotionSnapshot(ctx, segment.FilePath, detection.OffsetSeconds, imagePath); err != nil {
		log.Printf("motion snapshot failed camera_id=%s camera_name=%q error=%v", j.camera.ID, j.camera.Name, err)
	} else {
		event.ImagePath = imagePath
	}
	if err := createMotionClip(ctx, segment.FilePath, detection.OffsetSeconds, 7, 3, 4, videoPath); err != nil {
		log.Printf("motion clip failed camera_id=%s camera_name=%q error=%v", j.camera.ID, j.camera.Name, err)
	} else {
		event.VideoPath = videoPath
	}

	eventID, err := insertMotionEvent(ctx, j.db, event)
	if err != nil {
		log.Printf("motion event insert failed camera_id=%s camera_name=%q error=%v", j.camera.ID, j.camera.Name, err)
		return
	}
	_ = insertSystemEvent(ctx, j.db, "motion.detected", "camera", &j.camera.ID, "info", "motion detected", map[string]any{"camera_name": j.camera.Name, "score": detection.Score, "motion_event_id": eventID})
	j.sendMotionNotifications(ctx, eventID, event)
}

func (j *recordingJob) sendMotionNotifications(ctx context.Context, eventID string, event MotionEvent) {
	rules, err := fetchNotificationRules(ctx, j.db, "motion_detected", j.camera.ID)
	if err != nil {
		log.Printf("motion notification rules failed camera_id=%s camera_name=%q error=%v", j.camera.ID, j.camera.Name, err)
		return
	}
	if len(rules) == 0 {
		_ = updateMotionEventStatus(ctx, j.db, eventID, "detected")
		return
	}
	sentAny := false
	for _, rule := range rules {
		now := time.Now().UTC()
		allowed, err := shouldSendNotification(ctx, j.db, rule, j.camera.ID, now)
		if err != nil {
			log.Printf("notification cooldown check failed camera_id=%s camera_name=%q rule=%q error=%v", j.camera.ID, j.camera.Name, rule.Name, err)
			continue
		}
		if !allowed {
			_ = insertNotificationDelivery(ctx, j.db, rule, "motion_detected", "motion_event", eventID, j.camera.ID, "suppressed", "", nil)
			continue
		}
		evidence := event
		if !rule.AttachImage {
			evidence.ImagePath = ""
		}
		if !rule.AttachVideo {
			evidence.VideoPath = ""
		} else if event.RecordingSegmentID != "" && (rule.PreEventSeconds != 7 || rule.PostEventSeconds != 3 || rule.VideoFPS != 4) {
			videoPath, err := j.renderRuleSpecificClip(ctx, event, rule)
			if err != nil {
				log.Printf("rule-specific motion clip failed camera_id=%s camera_name=%q rule=%q error=%v", j.camera.ID, j.camera.Name, rule.Name, err)
			} else {
				evidence.VideoPath = videoPath
			}
		}
		if err := sendNotification(ctx, rule, j.camera, evidence); err != nil {
			log.Printf("notification send failed camera_id=%s camera_name=%q rule=%q method=%s error=%v", j.camera.ID, j.camera.Name, rule.Name, rule.Channel.Method, sanitizeLog(err.Error()))
			_ = insertNotificationDelivery(ctx, j.db, rule, "motion_detected", "motion_event", eventID, j.camera.ID, "failed", sanitizeLog(err.Error()), nil)
			_ = insertSystemEvent(ctx, j.db, "notification.send_failure", "motion_event", &eventID, "warning", "motion notification failed", map[string]any{"camera_name": j.camera.Name, "rule": rule.Name, "method": rule.Channel.Method, "error": sanitizeLog(err.Error())})
			continue
		}
		sentAt := time.Now().UTC()
		_ = insertNotificationDelivery(ctx, j.db, rule, "motion_detected", "motion_event", eventID, j.camera.ID, "sent", "", &sentAt)
		_ = insertSystemEvent(ctx, j.db, "notification.sent", "motion_event", &eventID, "info", "motion notification sent", map[string]any{"camera_name": j.camera.Name, "rule": rule.Name, "method": rule.Channel.Method})
		sentAny = true
	}
	if sentAny {
		_ = updateMotionEventStatus(ctx, j.db, eventID, "notified")
	}
}

func (j *recordingJob) renderRuleSpecificClip(ctx context.Context, event MotionEvent, rule NotificationRule) (string, error) {
	rows, err := j.db.QueryContext(ctx, `SELECT file_path, start_time FROM recording_segments WHERE id = $1`, event.RecordingSegmentID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var filePath string
	var start time.Time
	if !rows.Next() {
		return "", fmt.Errorf("recording segment not found")
	}
	if err := rows.Scan(&filePath, &start); err != nil {
		return "", err
	}
	offset := event.OccurredAt.Sub(start).Seconds()
	output := strings.TrimSuffix(event.VideoPath, ".mp4") + fmt.Sprintf("_%ds_%ds_%dfps.mp4", rule.PreEventSeconds, rule.PostEventSeconds, rule.VideoFPS)
	return output, createMotionClip(ctx, filePath, offset, rule.PreEventSeconds, rule.PostEventSeconds, rule.VideoFPS, output)
}

type motionDetection struct {
	OffsetSeconds float64
	Score         float64
}

func detectMotionInSegment(ctx context.Context, path string, threshold float64, minDurationSeconds int) (motionDetection, bool, error) {
	detectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	filter := fmt.Sprintf("fps=2,select='gt(scene,%.3f)',metadata=print:file=-", threshold)
	cmd := exec.CommandContext(detectCtx, "ffmpeg", "-hide_banner", "-v", "info", "-i", path, "-an", "-vf", filter, "-f", "null", "-")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return motionDetection{}, false, fmt.Errorf("ffmpeg scene detection: %s", sanitizeLog(truncate(string(output), 700)))
	}
	lines := strings.Split(string(output), "\n")
	var best motionDetection
	var lastOffset float64
	firstMotionOffset := -1.0
	lastMotionOffset := -1.0
	for _, line := range lines {
		timeMatch := motionTimePattern.FindStringSubmatch(line)
		if len(timeMatch) == 2 {
			lastOffset, _ = strconv.ParseFloat(timeMatch[1], 64)
		}
		scoreMatch := motionMetadataPattern.FindStringSubmatch(line)
		if len(scoreMatch) != 2 {
			continue
		}
		score, _ := strconv.ParseFloat(scoreMatch[1], 64)
		if score < threshold || score <= best.Score {
			if score >= threshold {
				if firstMotionOffset < 0 {
					firstMotionOffset = lastOffset
				}
				lastMotionOffset = lastOffset
			}
			continue
		}
		if firstMotionOffset < 0 {
			firstMotionOffset = lastOffset
		}
		lastMotionOffset = lastOffset
		best.OffsetSeconds = lastOffset
		best.Score = score
	}
	if minDurationSeconds > 0 && firstMotionOffset >= 0 && lastMotionOffset-firstMotionOffset < float64(minDurationSeconds) {
		return motionDetection{}, false, nil
	}
	return best, best.Score >= threshold, nil
}

func createMotionSnapshot(ctx context.Context, input string, offset float64, output string) error {
	cmdCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	args := []string{"-hide_banner", "-loglevel", "error", "-ss", fmt.Sprintf("%.3f", maxFloat(offset, 0)), "-i", input, "-frames:v", "1", "-q:v", "3", "-y", output}
	outputBytes, err := exec.CommandContext(cmdCtx, "ffmpeg", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg snapshot: %s", sanitizeLog(truncate(string(outputBytes), 700)))
	}
	return nil
}

func createMotionClip(ctx context.Context, input string, offset float64, preSeconds, postSeconds, fps int, output string) error {
	if fps <= 0 {
		fps = 4
	}
	start := maxFloat(offset-float64(preSeconds), 0)
	duration := preSeconds + postSeconds
	if duration <= 0 {
		duration = 10
	}
	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(duration+30)*time.Second)
	defer cancel()
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-ss", fmt.Sprintf("%.3f", start),
		"-i", input,
		"-t", fmt.Sprintf("%d", duration),
		"-vf", fmt.Sprintf("fps=%d,scale='min(960,iw)':-2", fps),
		"-an",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "30",
		"-movflags", "+faststart",
		"-y", output,
	}
	outputBytes, err := exec.CommandContext(cmdCtx, "ffmpeg", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg clip: %s", sanitizeLog(truncate(string(outputBytes), 700)))
	}
	return nil
}

func sendNotification(ctx context.Context, rule NotificationRule, camera Camera, event MotionEvent) error {
	switch rule.Channel.Method {
	case "telegram":
		return sendTelegramNotification(ctx, rule, camera, event)
	default:
		return fmt.Errorf("unsupported notification method %q", rule.Channel.Method)
	}
}

func sendTelegramNotification(ctx context.Context, rule NotificationRule, camera Camera, event MotionEvent) error {
	token := strings.TrimSpace(rule.Channel.Config["bot_token"])
	chatID := strings.TrimSpace(rule.Channel.Config["chat_id"])
	if token == "" || chatID == "" {
		return fmt.Errorf("telegram bot_token and chat_id are required")
	}
	text := renderNotificationMessage(rule.MessageTemplate, camera, event)
	if event.VideoPath != "" {
		if err := telegramUpload(ctx, token, "sendVideo", chatID, text, "video", event.VideoPath); err == nil {
			return nil
		} else {
			log.Printf("telegram video send failed camera_id=%s camera_name=%q error=%v", camera.ID, camera.Name, sanitizeLog(err.Error()))
		}
	}
	if event.ImagePath != "" {
		if err := telegramUpload(ctx, token, "sendPhoto", chatID, text, "photo", event.ImagePath); err == nil {
			return nil
		} else {
			log.Printf("telegram photo send failed camera_id=%s camera_name=%q error=%v", camera.ID, camera.Name, sanitizeLog(err.Error()))
		}
	}
	return telegramJSON(ctx, token, "sendMessage", map[string]string{"chat_id": chatID, "text": text})
}

func telegramJSON(ctx context.Context, token, method string, payload map[string]string) error {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doTelegramRequest(req)
}

func telegramUpload(ctx context.Context, token, method, chatID, caption, fieldName, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("chat_id", chatID)
	_ = writer.WriteField("caption", caption)
	part, err := writer.CreateFormFile(fieldName, filepath.Base(filePath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+token+"/"+method, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return doTelegramRequest(req)
}

func doTelegramRequest(req *http.Request) error {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return fmt.Errorf("telegram returned %s: %s", resp.Status, sanitizeLog(string(body)))
	}
	return nil
}

func renderNotificationMessage(template string, camera Camera, event MotionEvent) string {
	if strings.TrimSpace(template) == "" {
		template = "Motion detected on {{camera_name}}\nTime: {{event_time}}\nScore: {{score}}"
	}
	replacements := map[string]string{
		"{{camera_name}}": camera.Name,
		"{{camera_id}}":   camera.ID,
		"{{event_time}}":  event.OccurredAt.UTC().Format(time.RFC3339),
		"{{score}}":       fmt.Sprintf("%.3f", event.Score),
	}
	out := template
	for key, value := range replacements {
		out = strings.ReplaceAll(out, key, value)
	}
	return out
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
