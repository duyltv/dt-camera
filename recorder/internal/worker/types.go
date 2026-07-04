package worker

import (
	"fmt"
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
	ID                       string
	Name                     string
	RTSPURL                  string
	StorageLocationID        string
	StoragePath              string
	RecordAudio              bool
	MotionDetectionEnabled   bool
	MotionSensitivity        float64
	MotionMinDurationSeconds int
	RetentionDays            int
	MaxStorageBytes          *int64
	UpdatedAt                time.Time
}

func (c Camera) ConfigKey() string {
	return c.ID + "|" + c.RTSPURL + "|" + c.StorageLocationID + "|" + c.StoragePath + "|" + fmtBool(c.RecordAudio) + "|" + fmtBool(c.MotionDetectionEnabled) + "|" + fmt.Sprintf("%.3f", c.MotionSensitivity) + "|" + fmt.Sprintf("%d", c.MotionMinDurationSeconds) + "|" + c.UpdatedAt.Format(time.RFC3339Nano)
}

func fmtBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

type MotionEvent struct {
	ID                 string
	CameraID           string
	RecordingSegmentID string
	OccurredAt         time.Time
	Score              float64
	ImagePath          string
	VideoPath          string
}

type NotificationRule struct {
	ID                    string
	Name                  string
	EventType             string
	NotificationChannelID string
	CooldownSeconds       int
	AttachImage           bool
	AttachVideo           bool
	PreEventSeconds       int
	PostEventSeconds      int
	VideoFPS              int
	Channel               NotificationChannel
}

type NotificationChannel struct {
	ID      string
	Name    string
	Method  string
	Enabled bool
	Config  map[string]string
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
