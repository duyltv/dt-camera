package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildAvailabilityMergesRangesAndReportsGaps(t *testing.T) {
	cameraID := "018f5d67-89ab-7def-8123-456789abcdea"
	start := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	segments := []recordingSegmentResponse{
		{CameraID: cameraID, StartTime: start, EndTime: start.Add(60 * time.Second)},
		{CameraID: cameraID, StartTime: start.Add(61 * time.Second), EndTime: start.Add(120 * time.Second)},
		{CameraID: cameraID, StartTime: start.Add(180 * time.Second), EndTime: start.Add(240 * time.Second)},
	}

	availability := buildAvailability([]string{cameraID}, segments, 2*time.Second)
	if len(availability) != 1 {
		t.Fatalf("expected one camera availability, got %d", len(availability))
	}
	if len(availability[0].Ranges) != 2 {
		t.Fatalf("expected two ranges, got %d", len(availability[0].Ranges))
	}
	if availability[0].Ranges[0].SegmentCount != 2 {
		t.Fatalf("expected first range to include two segments")
	}
	if len(availability[0].Gaps) != 1 {
		t.Fatalf("expected one gap, got %d", len(availability[0].Gaps))
	}
}

func TestSafeRecordingFilePath(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "camera", "segment.mp4")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("video"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	safePath, err := safeRecordingFilePath(root, filePath)
	if err != nil {
		t.Fatalf("safeRecordingFilePath() error = %v", err)
	}
	rootEval, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("eval root: %v", err)
	}
	if rel, err := filepath.Rel(rootEval, safePath); err != nil || rel != filepath.Join("camera", "segment.mp4") {
		t.Fatalf("safe path = %q, rel = %q, err = %v", safePath, rel, err)
	}
}

func TestSafeRecordingFilePathRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.mp4")
	if err := os.WriteFile(outside, []byte("video"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	if _, err := safeRecordingFilePath(root, outside); err == nil {
		t.Fatalf("expected outside file to be rejected")
	}
	if _, err := safeRecordingFilePath(root, filepath.Join(root, "..", filepath.Base(outside))); err == nil {
		t.Fatalf("expected traversal path to be rejected")
	}
}

func TestRecordingSegmentResponseShape(t *testing.T) {
	response := recordingSegmentResponse{
		ID:              "018f5d67-89ab-7def-8123-456789abcdea",
		CameraID:        "018f5d67-89ab-7def-8123-456789abcdef",
		StartTime:       time.Now().UTC(),
		EndTime:         time.Now().UTC().Add(time.Minute),
		DurationSeconds: 60,
		SizeBytes:       1234,
		Format:          "mp4",
		Status:          "completed",
		PlaybackURL:     "/api/recordings/018f5d67-89ab-7def-8123-456789abcdea/file",
	}

	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	body := string(payload)
	if strings.Contains(body, "file_path") || strings.Contains(body, "/recordings/camera") {
		t.Fatalf("response leaked raw file path: %s", body)
	}
	if !strings.Contains(body, "playback_url") {
		t.Fatalf("response should include playback_url: %s", body)
	}
}

func TestPlaybackPrepareForbiddenResponseKeepsLayoutPosition(t *testing.T) {
	response := playbackPrepareCameraResponse{
		CameraID:          "018f5d67-89ab-4def-8123-456789abcdef",
		Status:            "forbidden",
		SelectedTimestamp: time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
		LayoutItem: &layoutItemPositionResponse{
			ItemID:   "018f5d67-89ab-4def-8123-456789abcdea",
			LayoutID: "018f5d67-89ab-4def-8123-456789abcdeb",
			X:        1,
			Y:        2,
			Width:    3,
			Height:   4,
			TileType: "custom",
		},
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	body := string(payload)
	if !strings.Contains(body, `"status":"forbidden"`) || !strings.Contains(body, `"layout_item"`) {
		t.Fatalf("expected forbidden response with layout item, got %s", body)
	}
}

func TestPlaybackPrepareResponseIncludesOffset(t *testing.T) {
	segmentStart := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	offset := 25.0
	response := playbackPrepareCameraResponse{
		CameraID:          "018f5d67-89ab-4def-8123-456789abcdef",
		Status:            "ok",
		SelectedTimestamp: segmentStart.Add(25 * time.Second),
		SegmentStartTime:  &segmentStart,
		OffsetSeconds:     &offset,
		Segment: &recordingSegmentResponse{
			ID:          "018f5d67-89ab-4def-8123-456789abcdea",
			CameraID:    "018f5d67-89ab-4def-8123-456789abcdef",
			StartTime:   segmentStart,
			EndTime:     segmentStart.Add(time.Minute),
			PlaybackURL: "/api/recordings/018f5d67-89ab-4def-8123-456789abcdea/file",
		},
	}

	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	body := string(payload)
	if !strings.Contains(body, `"segment_start_time"`) || !strings.Contains(body, `"offset_seconds":25`) {
		t.Fatalf("expected offset fields in response, got %s", body)
	}
	if strings.Contains(body, "file_path") {
		t.Fatalf("response leaked file path: %s", body)
	}
}

func TestParseRecordingFilterSupportsMultipleCameraIDsAndTimeRange(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/recordings/search?camera_id=018f5d67-89ab-4def-8123-456789abcdea,018f5d67-89ab-4def-8123-456789abcdef&start_time=2026-07-01T10:00:00Z&end_time=2026-07-01T11:00:00Z", nil)
	recorder := httptest.NewRecorder()

	filter, ok := parseRecordingFilter(recorder, req)
	if !ok {
		t.Fatalf("expected filter to parse, status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if len(filter.CameraIDs) != 2 {
		t.Fatalf("expected two camera IDs, got %d", len(filter.CameraIDs))
	}
	if !filter.EndTime.After(filter.StartTime) {
		t.Fatalf("expected end time after start time")
	}
}
