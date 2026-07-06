package main

import (
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("ANALYTICS_WORKER_ID", "")
	t.Setenv("AI_MAX_CONCURRENT_JOBS", "")
	t.Setenv("AI_POLL_INTERVAL_MS", "")

	cfg := loadConfig()
	if cfg.workerID != "analytics-1" {
		t.Fatalf("workerID = %q, want analytics-1", cfg.workerID)
	}
	if cfg.maxConcurrentJobs != 1 {
		t.Fatalf("maxConcurrentJobs = %d, want 1", cfg.maxConcurrentJobs)
	}
	if cfg.pollInterval != time.Second {
		t.Fatalf("pollInterval = %s, want 1s", cfg.pollInterval)
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("ANALYTICS_WORKER_ID", "ai-2")
	t.Setenv("AI_MAX_CONCURRENT_JOBS", "3")
	t.Setenv("AI_POLL_INTERVAL_MS", "250")

	cfg := loadConfig()
	if cfg.workerID != "ai-2" {
		t.Fatalf("workerID = %q, want ai-2", cfg.workerID)
	}
	if cfg.maxConcurrentJobs != 3 {
		t.Fatalf("maxConcurrentJobs = %d, want 3", cfg.maxConcurrentJobs)
	}
	if cfg.pollInterval != 250*time.Millisecond {
		t.Fatalf("pollInterval = %s, want 250ms", cfg.pollInterval)
	}
}

func TestLoadConfigRejectsInvalidNumbers(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("AI_MAX_CONCURRENT_JOBS", "0")
	t.Setenv("AI_POLL_INTERVAL_MS", "-10")

	cfg := loadConfig()
	if cfg.maxConcurrentJobs != 1 {
		t.Fatalf("maxConcurrentJobs = %d, want fallback 1", cfg.maxConcurrentJobs)
	}
	if cfg.pollInterval != time.Second {
		t.Fatalf("pollInterval = %s, want fallback 1s", cfg.pollInterval)
	}
}
