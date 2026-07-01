package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"github.com/dt-camera/recorder/internal/worker"
)

func main() {
	cfg := worker.Config{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		RecordingsPath:     getenv("RECORDINGS_PATH", "/recordings"),
		WorkerID:           getenv("RECORDER_WORKER_ID", "recorder-1"),
		PollInterval:       getenvDurationSeconds("RECORDER_POLL_INTERVAL_SECONDS", 5*time.Second),
		SegmentDuration:    getenvDurationSeconds("RECORDER_SEGMENT_SECONDS", 60*time.Second),
		MaxBackoff:         getenvDurationSeconds("RECORDER_MAX_BACKOFF_SECONDS", 60*time.Second),
		StableFileAge:      getenvDurationSeconds("RECORDER_STABLE_FILE_AGE_SECONDS", 8*time.Second),
		StableFileChecks:   2,
		HeartbeatInterval:  10 * time.Second,
		CleanupInterval:    getenvDurationSeconds("RECORDER_CLEANUP_INTERVAL_SECONDS", 60*time.Second),
		LowDiskFreePercent: getenvFloat("RECORDER_LOW_DISK_FREE_PERCENT", 10),
		LowDiskMinFileAge:  getenvDurationSeconds("RECORDER_LOW_DISK_MIN_FILE_AGE_SECONDS", time.Hour),
	}

	if cfg.DatabaseURL == "" {
		log.Fatalf("DATABASE_URL is required")
	}

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	recorder := worker.New(db, cfg)
	if err := recorder.Run(ctx); err != nil {
		log.Fatalf("recorder stopped: %v", err)
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvDurationSeconds(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func getenvFloat(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}
