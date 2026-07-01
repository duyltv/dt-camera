package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var segmentNamePattern = regexp.MustCompile(`^(.+)_([0-9]{8}T[0-9]{6})\.([A-Za-z0-9]+)$`)

type trackedFile struct {
	Size         int64
	ModTime      time.Time
	StableChecks int
	Inserted     bool
}

func cameraRoot(storagePath, cameraID string) string {
	return filepath.Join(storagePath, cameraID)
}

func segmentPattern(storagePath, cameraID string, now time.Time) string {
	year, month, day := now.Date()
	return filepath.Join(
		cameraRoot(storagePath, cameraID),
		fmt.Sprintf("%04d", year),
		fmt.Sprintf("%02d", int(month)),
		fmt.Sprintf("%02d", day),
		cameraID+"_%Y%m%dT%H%M%S.mp4",
	)
}

func untilNextDay(now time.Time) time.Duration {
	nextDay := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 1, 0, now.Location())
	return nextDay.Sub(now)
}

func parseSegmentMetadata(path string, camera Camera, segmentDuration time.Duration) (SegmentMetadata, error) {
	base := filepath.Base(path)
	matches := segmentNamePattern.FindStringSubmatch(base)
	if len(matches) != 4 {
		return SegmentMetadata{}, fmt.Errorf("segment filename does not match expected pattern: %s", base)
	}
	if matches[1] != camera.ID {
		return SegmentMetadata{}, fmt.Errorf("segment camera ID mismatch in filename: %s", base)
	}

	start, err := time.ParseInLocation("20060102T150405", matches[2], time.Local)
	if err != nil {
		return SegmentMetadata{}, fmt.Errorf("parse segment timestamp: %w", err)
	}
	start = start.UTC()

	info, err := os.Stat(path)
	if err != nil {
		return SegmentMetadata{}, fmt.Errorf("stat segment: %w", err)
	}

	durationSeconds := segmentDuration.Seconds()
	end := start.Add(segmentDuration)
	if durationSeconds <= 0 {
		durationSeconds = 0
		end = start
	}

	return SegmentMetadata{
		CameraID:          camera.ID,
		StorageLocationID: camera.StorageLocationID,
		FilePath:          path,
		StartTime:         start,
		EndTime:           end,
		DurationSeconds:   durationSeconds,
		SizeBytes:         info.Size(),
		Format:            strings.ToLower(matches[3]),
		Status:            "completed",
	}, nil
}

func listSegmentFiles(root string) ([]string, error) {
	var files []string
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return files, nil
	}

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".mp4") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk segment files: %w", err)
	}
	return files, nil
}
