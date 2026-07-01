package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeSegmentRowDeleter struct {
	deleted []string
}

func (f *fakeSegmentRowDeleter) DeleteSegmentRow(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func TestSelectRetentionExpiredSegments(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	segments := []retentionSegment{
		{ID: "old", StartTime: now.AddDate(0, 0, -8), RetentionDays: 7},
		{ID: "fresh", StartTime: now.AddDate(0, 0, -6), RetentionDays: 7},
	}

	selected := selectRetentionExpiredSegments(segments, now)
	if len(selected) != 1 || selected[0].ID != "old" {
		t.Fatalf("selected = %+v, want old segment only", selected)
	}
}

func TestSelectMaxStorageSegments(t *testing.T) {
	limit := int64(100)
	start := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	segments := []retentionSegment{
		{ID: "old", CameraID: "camera", StartTime: start, SizeBytes: 60, MaxStorageBytes: &limit},
		{ID: "middle", CameraID: "camera", StartTime: start.Add(time.Minute), SizeBytes: 60, MaxStorageBytes: &limit},
		{ID: "new", CameraID: "camera", StartTime: start.Add(2 * time.Minute), SizeBytes: 30, MaxStorageBytes: &limit},
	}

	selected := selectMaxStorageSegments(segments)
	if len(selected) != 1 || selected[0].ID != "old" {
		t.Fatalf("selected = %+v, want oldest segment to fit max storage", selected)
	}
}

func TestSelectLowDiskSegmentsSkipsFreshSegments(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	segments := []retentionSegment{
		{ID: "old", StorageLocationID: "storage-1", StartTime: now.Add(-2 * time.Hour)},
		{ID: "fresh", StorageLocationID: "storage-1", StartTime: now.Add(-10 * time.Minute)},
		{ID: "other-storage", StorageLocationID: "storage-2", StartTime: now.Add(-3 * time.Hour)},
	}

	selected := selectLowDiskSegments(segments, "storage-1", now, time.Hour)
	if len(selected) != 1 || selected[0].ID != "old" {
		t.Fatalf("selected = %+v, want old storage-1 segment only", selected)
	}
}

func TestSafeSegmentPathRejectsOutsideStorageRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.mp4")
	if err := os.WriteFile(outside, []byte("video"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	if _, err := safeSegmentPath(root, outside); err == nil {
		t.Fatalf("expected outside path to be rejected")
	}
	if _, err := safeSegmentPath(root, filepath.Join(root, "..", filepath.Base(outside))); err == nil {
		t.Fatalf("expected traversal path to be rejected")
	}
}

func TestCleanupSegmentDeletesFileAndRow(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "camera", "segment.mp4")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("video"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	deleter := &fakeSegmentRowDeleter{}
	err := cleanupSegment(context.Background(), deleter, retentionSegment{
		ID:          "segment-1",
		StorageRoot: root,
		FilePath:    filePath,
	})
	if err != nil {
		t.Fatalf("cleanupSegment() error = %v", err)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected file to be deleted, stat err=%v", err)
	}
	if len(deleter.deleted) != 1 || deleter.deleted[0] != "segment-1" {
		t.Fatalf("deleted rows = %+v, want segment-1", deleter.deleted)
	}
}

func TestCleanupSegmentDoesNotDeleteOutsideStorageRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.mp4")
	if err := os.WriteFile(outside, []byte("video"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	deleter := &fakeSegmentRowDeleter{}
	err := cleanupSegment(context.Background(), deleter, retentionSegment{
		ID:          "segment-1",
		StorageRoot: root,
		FilePath:    outside,
	})
	if err == nil {
		t.Fatalf("expected cleanupSegment to reject outside file")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file should remain, stat err=%v", err)
	}
	if len(deleter.deleted) != 0 {
		t.Fatalf("row should not be deleted for outside file, got %+v", deleter.deleted)
	}
}
