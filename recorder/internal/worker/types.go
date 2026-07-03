package worker

import (
	"time"
)

type Config struct {
	DatabaseURL        string
	RecordingsPath     string
	WorkerID           string
	PollInterval       time.Duration
	SegmentDuration    time.Duration
	MaxBackoff         time.Duration
	StableFileAge      time.Duration
	StableFileChecks   int
	HeartbeatInterval  time.Duration
	CleanupInterval    time.Duration
	LowDiskFreePercent float64
	LowDiskMinFileAge  time.Duration
}

type Camera struct {
	ID                string
	Name              string
	RTSPURL           string
	StorageLocationID string
	StoragePath       string
	RecordAudio       bool
	RetentionDays     int
	MaxStorageBytes   *int64
	UpdatedAt         time.Time
}

func (c Camera) ConfigKey() string {
	return c.ID + "|" + c.RTSPURL + "|" + c.StorageLocationID + "|" + c.StoragePath + "|" + fmtBool(c.RecordAudio) + "|" + c.UpdatedAt.Format(time.RFC3339Nano)
}

func fmtBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

type SegmentMetadata struct {
	CameraID          string
	StorageLocationID string
	FilePath          string
	StartTime         time.Time
	EndTime           time.Time
	DurationSeconds   float64
	SizeBytes         int64
	Format            string
	Status            string
}
