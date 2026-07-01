package worker

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type retentionSegment struct {
	ID                string
	CameraID          string
	StorageLocationID string
	StorageRoot       string
	FilePath          string
	StartTime         time.Time
	SizeBytes         int64
	RetentionDays     int
	MaxStorageBytes   *int64
}

type storageDiskHealth struct {
	StorageLocationID string
	Root              string
	Exists            bool
	Writable          bool
	TotalBytes        int64
	FreeBytes         int64
	UsedBytes         int64
	UsedPercent       float64
	Error             string
}

type cleanupRunner struct {
	db                 *sql.DB
	now                time.Time
	lowDiskFreePercent float64
	lowDiskMinFileAge  time.Duration
}

type segmentRowDeleter interface {
	DeleteSegmentRow(ctx context.Context, id string) error
}

type dbSegmentRowDeleter struct {
	db *sql.DB
}

func (d dbSegmentRowDeleter) DeleteSegmentRow(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM recording_segments WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete segment row: %w", err)
	}
	return nil
}

func (r cleanupRunner) run(ctx context.Context) error {
	segments, err := fetchRetentionSegments(ctx, r.db)
	if err != nil {
		return err
	}

	expired := selectRetentionExpiredSegments(segments, r.now)
	deleter := dbSegmentRowDeleter{db: r.db}
	for _, segment := range expired {
		if err := cleanupSegment(ctx, deleter, segment); err != nil {
			log.Printf("retention cleanup skipped segment_id=%s camera_id=%s error=%v", segment.ID, segment.CameraID, err)
			_ = insertSystemEvent(ctx, r.db, "cleanup.failure", "recording_segment", &segment.ID, "error", "retention cleanup failed", map[string]any{"camera_id": segment.CameraID, "error": err.Error()})
			continue
		}
		log.Printf("retention cleanup deleted segment_id=%s camera_id=%s file=%s", segment.ID, segment.CameraID, segment.FilePath)
		_ = insertSystemEvent(ctx, r.db, "cleanup.delete", "recording_segment", &segment.ID, "info", "retention cleanup deleted segment", map[string]any{"camera_id": segment.CameraID, "storage_location_id": segment.StorageLocationID, "size_bytes": segment.SizeBytes})
	}

	remaining, err := fetchRetentionSegments(ctx, r.db)
	if err != nil {
		return err
	}
	overLimit := selectMaxStorageSegments(remaining)
	for _, segment := range overLimit {
		if err := cleanupSegment(ctx, deleter, segment); err != nil {
			log.Printf("max storage cleanup skipped segment_id=%s camera_id=%s error=%v", segment.ID, segment.CameraID, err)
			_ = insertSystemEvent(ctx, r.db, "cleanup.failure", "recording_segment", &segment.ID, "error", "max storage cleanup failed", map[string]any{"camera_id": segment.CameraID, "error": err.Error()})
			continue
		}
		log.Printf("max storage cleanup deleted segment_id=%s camera_id=%s file=%s", segment.ID, segment.CameraID, segment.FilePath)
		_ = insertSystemEvent(ctx, r.db, "cleanup.delete", "recording_segment", &segment.ID, "info", "max storage cleanup deleted segment", map[string]any{"camera_id": segment.CameraID, "storage_location_id": segment.StorageLocationID, "size_bytes": segment.SizeBytes})
	}

	remaining, err = fetchRetentionSegments(ctx, r.db)
	if err != nil {
		return err
	}
	for _, health := range collectStorageHealth(remaining) {
		if err := updateStorageHealth(ctx, r.db, healthStatusFromDisk(health, r.lowDiskFreePercent), health); err != nil {
			log.Printf("storage health update failed storage_location_id=%s error=%v", health.StorageLocationID, err)
		}
		if health.Error != "" || health.TotalBytes == 0 {
			continue
		}
		if freePercent(health) >= r.lowDiskFreePercent {
			continue
		}

		candidates := selectLowDiskSegments(remaining, health.StorageLocationID, r.now, r.lowDiskMinFileAge)
		for _, segment := range candidates {
			if err := cleanupSegment(ctx, deleter, segment); err != nil {
				log.Printf("low disk cleanup skipped segment_id=%s camera_id=%s error=%v", segment.ID, segment.CameraID, err)
				_ = insertSystemEvent(ctx, r.db, "cleanup.failure", "recording_segment", &segment.ID, "error", "low disk cleanup failed", map[string]any{"camera_id": segment.CameraID, "error": err.Error()})
				continue
			}
			log.Printf("low disk cleanup deleted segment_id=%s camera_id=%s file=%s", segment.ID, segment.CameraID, segment.FilePath)
			_ = insertSystemEvent(ctx, r.db, "cleanup.delete", "recording_segment", &segment.ID, "info", "low disk cleanup deleted segment", map[string]any{"camera_id": segment.CameraID, "storage_location_id": segment.StorageLocationID, "size_bytes": segment.SizeBytes})
			health.FreeBytes += segment.SizeBytes
			health.UsedBytes -= segment.SizeBytes
			if health.UsedBytes < 0 {
				health.UsedBytes = 0
			}
			if health.TotalBytes > 0 {
				health.UsedPercent = (float64(health.UsedBytes) / float64(health.TotalBytes)) * 100
			}
			if freePercent(health) >= r.lowDiskFreePercent {
				break
			}
		}
		if err := updateStorageHealth(ctx, r.db, healthStatusFromDisk(health, r.lowDiskFreePercent), health); err != nil {
			log.Printf("storage health update failed storage_location_id=%s error=%v", health.StorageLocationID, err)
		}
	}

	return nil
}

func selectRetentionExpiredSegments(segments []retentionSegment, now time.Time) []retentionSegment {
	selected := make([]retentionSegment, 0)
	for _, segment := range segments {
		if segment.RetentionDays <= 0 {
			continue
		}
		cutoff := now.AddDate(0, 0, -segment.RetentionDays)
		if segment.StartTime.Before(cutoff) {
			selected = append(selected, segment)
		}
	}
	sortSegmentsOldestFirst(selected)
	return selected
}

func selectLowDiskSegments(segments []retentionSegment, storageLocationID string, now time.Time, minAge time.Duration) []retentionSegment {
	selected := make([]retentionSegment, 0)
	for _, segment := range segments {
		if segment.StorageLocationID != storageLocationID {
			continue
		}
		if minAge > 0 && now.Sub(segment.StartTime) < minAge {
			continue
		}
		selected = append(selected, segment)
	}
	sortSegmentsOldestFirst(selected)
	return selected
}

func selectMaxStorageSegments(segments []retentionSegment) []retentionSegment {
	byCamera := map[string][]retentionSegment{}
	for _, segment := range segments {
		if segment.MaxStorageBytes == nil {
			continue
		}
		byCamera[segment.CameraID] = append(byCamera[segment.CameraID], segment)
	}

	selected := []retentionSegment{}
	for _, cameraSegments := range byCamera {
		sortSegmentsOldestFirst(cameraSegments)
		var total int64
		for _, segment := range cameraSegments {
			total += segment.SizeBytes
		}
		limit := *cameraSegments[0].MaxStorageBytes
		for _, segment := range cameraSegments {
			if total <= limit {
				break
			}
			selected = append(selected, segment)
			total -= segment.SizeBytes
		}
	}
	sortSegmentsOldestFirst(selected)
	return selected
}

func sortSegmentsOldestFirst(segments []retentionSegment) {
	sort.Slice(segments, func(i, j int) bool {
		return segments[i].StartTime.Before(segments[j].StartTime)
	})
}

func cleanupSegment(ctx context.Context, deleter segmentRowDeleter, segment retentionSegment) error {
	safePath, err := safeSegmentPath(segment.StorageRoot, segment.FilePath)
	if err != nil {
		return err
	}
	if err := os.Remove(safePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete file: %w", err)
	}
	return deleter.DeleteSegmentRow(ctx, segment.ID)
}

func safeSegmentPath(storageRoot, filePath string) (string, error) {
	if storageRoot == "" || filePath == "" {
		return "", fmt.Errorf("storage root and file path are required")
	}
	if !filepath.IsAbs(storageRoot) || !filepath.IsAbs(filePath) {
		return "", fmt.Errorf("recording paths must be absolute container paths")
	}
	cleanRoot, err := filepath.Abs(filepath.Clean(storageRoot))
	if err != nil {
		return "", fmt.Errorf("storage root cannot be resolved")
	}
	cleanPath, err := filepath.Abs(filepath.Clean(filePath))
	if err != nil {
		return "", fmt.Errorf("recording path cannot be resolved")
	}
	rootEval, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil {
		return "", fmt.Errorf("storage root cannot be verified")
	}
	pathEval := cleanPath
	if _, err := os.Stat(cleanPath); err == nil {
		if evaluated, err := filepath.EvalSymlinks(cleanPath); err == nil {
			pathEval = evaluated
		}
	}
	rel, err := filepath.Rel(rootEval, pathEval)
	if err != nil {
		return "", fmt.Errorf("recording path cannot be compared with storage root")
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("recording file is outside its storage location")
	}
	return pathEval, nil
}

func collectStorageHealth(segments []retentionSegment) []storageDiskHealth {
	byStorage := map[string]storageDiskHealth{}
	for _, segment := range segments {
		if _, ok := byStorage[segment.StorageLocationID]; ok {
			continue
		}
		byStorage[segment.StorageLocationID] = inspectStorageDisk(segment.StorageLocationID, segment.StorageRoot)
	}
	result := make([]storageDiskHealth, 0, len(byStorage))
	for _, health := range byStorage {
		result = append(result, health)
	}
	return result
}

func inspectStorageDisk(storageLocationID, root string) storageDiskHealth {
	health := storageDiskHealth{StorageLocationID: storageLocationID, Root: root}
	info, err := os.Stat(root)
	if err != nil {
		health.Error = fmt.Sprintf("storage path cannot be inspected: %v", err)
		return health
	}
	health.Exists = true
	if !info.IsDir() {
		health.Error = "storage path is not a directory"
		return health
	}
	if err := ensureWritableDirectory(root); err != nil {
		health.Error = err.Error()
	} else {
		health.Writable = true
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(root, &stat); err != nil {
		health.Error = fmt.Sprintf("storage disk cannot be inspected: %v", err)
		return health
	}
	blockSize := int64(stat.Bsize)
	health.TotalBytes = int64(stat.Blocks) * blockSize
	health.FreeBytes = int64(stat.Bavail) * blockSize
	health.UsedBytes = health.TotalBytes - health.FreeBytes
	if health.TotalBytes > 0 {
		health.UsedPercent = (float64(health.UsedBytes) / float64(health.TotalBytes)) * 100
	}
	return health
}

func freePercent(health storageDiskHealth) float64 {
	if health.TotalBytes <= 0 {
		return 0
	}
	return (float64(health.FreeBytes) / float64(health.TotalBytes)) * 100
}

func healthStatusFromDisk(health storageDiskHealth, lowDiskFreePercent float64) string {
	if health.Error != "" || !health.Exists || !health.Writable {
		return "error"
	}
	if freePercent(health) < lowDiskFreePercent {
		return "warning"
	}
	return "ok"
}

func fetchRetentionSegments(ctx context.Context, db *sql.DB) ([]retentionSegment, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			rs.id,
			rs.camera_id,
			rs.storage_location_id,
			sl.container_path,
			rs.file_path,
			rs.start_time,
			rs.size_bytes,
			c.retention_days,
			c.max_storage_bytes
		FROM recording_segments rs
		JOIN cameras c ON c.id = rs.camera_id
		JOIN storage_locations sl ON sl.id = rs.storage_location_id
		WHERE rs.status = 'completed'
			AND rs.storage_location_id IS NOT NULL
		ORDER BY rs.start_time
	`)
	if err != nil {
		return nil, fmt.Errorf("query retention segments: %w", err)
	}
	defer rows.Close()

	segments := []retentionSegment{}
	for rows.Next() {
		var segment retentionSegment
		if err := rows.Scan(
			&segment.ID,
			&segment.CameraID,
			&segment.StorageLocationID,
			&segment.StorageRoot,
			&segment.FilePath,
			&segment.StartTime,
			&segment.SizeBytes,
			&segment.RetentionDays,
			&segment.MaxStorageBytes,
		); err != nil {
			return nil, fmt.Errorf("scan retention segment: %w", err)
		}
		segments = append(segments, segment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate retention segments: %w", err)
	}
	return segments, nil
}

func updateStorageHealth(ctx context.Context, db *sql.DB, status string, health storageDiskHealth) error {
	_, err := db.ExecContext(ctx, `
		UPDATE storage_locations
		SET health_status = $2,
			exists = $3,
			writable = $4,
			total_bytes = $5,
			free_bytes = $6,
			used_bytes = $7,
			used_percent = $8,
			latest_validation_error = NULLIF($9, ''),
			last_checked_at = now()
		WHERE id = $1
	`, health.StorageLocationID, status, health.Exists, health.Writable, health.TotalBytes, health.FreeBytes, health.UsedBytes, health.UsedPercent, health.Error)
	if err != nil {
		return fmt.Errorf("update storage health: %w", err)
	}
	return nil
}
