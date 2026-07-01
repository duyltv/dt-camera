package worker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSegmentPattern(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	got := segmentPattern("/recordings", "camera-id", now)
	want := filepath.Join("/recordings", "camera-id", "2026", "07", "01", "camera-id_%Y%m%dT%H%M%S.mp4")
	if got != want {
		t.Fatalf("segmentPattern() = %q, want %q", got, want)
	}
}

func TestParseSegmentMetadata(t *testing.T) {
	tempDir := t.TempDir()
	camera := Camera{
		ID:                "018f5d67-89ab-7def-8123-456789abcdea",
		StorageLocationID: "018f5d67-89ab-7def-8123-456789abcdef",
	}
	path := filepath.Join(tempDir, "018f5d67-89ab-7def-8123-456789abcdea_20260701T123456.mp4")
	if err := os.WriteFile(path, []byte("segment"), 0o600); err != nil {
		t.Fatalf("write segment file: %v", err)
	}

	metadata, err := parseSegmentMetadata(path, camera, 60*time.Second)
	if err != nil {
		t.Fatalf("parseSegmentMetadata() error = %v", err)
	}

	if metadata.CameraID != camera.ID {
		t.Fatalf("CameraID = %q, want %q", metadata.CameraID, camera.ID)
	}
	if metadata.Format != "mp4" {
		t.Fatalf("Format = %q, want mp4", metadata.Format)
	}
	if metadata.DurationSeconds != 60 {
		t.Fatalf("DurationSeconds = %v, want 60", metadata.DurationSeconds)
	}
	if metadata.SizeBytes == 0 {
		t.Fatalf("SizeBytes should be set")
	}
}
