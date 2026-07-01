package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dt-camera/backend/internal/version"
)

func TestRetentionConfigDefaultsAreConservative(t *testing.T) {
	// Make sure no env override is set.
	for _, k := range []string{"EVENT_RETENTION_HOURS", "ALERT_RETENTION_HOURS", "HEARTBEAT_RETENTION_HOURS", "REVOKED_SESSION_HOURS", "RETENTION_INTERVAL_SECONDS"} {
		t.Setenv(k, "")
	}
	server := &Server{cfg: Config{RetentionInterval: 6 * time.Hour}}
	cfg := server.retentionConfigFromEnv()
	if cfg.EventRetention < 30*24*time.Hour {
		t.Fatalf("default event retention should be >= 30 days, got %s", cfg.EventRetention)
	}
	if cfg.AlertRetention < 90*24*time.Hour {
		t.Fatalf("default alert retention should be >= 90 days, got %s", cfg.AlertRetention)
	}
	if cfg.HeartbeatRetention < 7*24*time.Hour {
		t.Fatalf("default heartbeat retention should be >= 7 days, got %s", cfg.HeartbeatRetention)
	}
	if cfg.RevokedSessionCutoff < 24*time.Hour {
		t.Fatalf("default revoked-session cutoff should be >= 24h, got %s", cfg.RevokedSessionCutoff)
	}
	if cfg.Interval < 6*time.Hour {
		t.Fatalf("default interval should be >= 6h, got %s", cfg.Interval)
	}
}

func TestRetentionConfigEnvOverridesApply(t *testing.T) {
	t.Setenv("EVENT_RETENTION_HOURS", "12h")
	t.Setenv("ALERT_RETENTION_HOURS", "48h")
	t.Setenv("HEARTBEAT_RETENTION_HOURS", "6h")
	t.Setenv("REVOKED_SESSION_HOURS", "2h")
	t.Setenv("RETENTION_INTERVAL_SECONDS", "30m")
	server := &Server{}
	cfg := server.retentionConfigFromEnv()
	if cfg.EventRetention != 12*time.Hour {
		t.Fatalf("event retention = %s, want 12h", cfg.EventRetention)
	}
	if cfg.AlertRetention != 48*time.Hour {
		t.Fatalf("alert retention = %s, want 48h", cfg.AlertRetention)
	}
	if cfg.HeartbeatRetention != 6*time.Hour {
		t.Fatalf("heartbeat retention = %s, want 6h", cfg.HeartbeatRetention)
	}
	if cfg.RevokedSessionCutoff != 2*time.Hour {
		t.Fatalf("revoked cutoff = %s, want 2h", cfg.RevokedSessionCutoff)
	}
	if cfg.Interval != 30*time.Minute {
		t.Fatalf("interval = %s, want 30m", cfg.Interval)
	}
}

func TestRetentionConfigIgnoresGarbageEnv(t *testing.T) {
	t.Setenv("EVENT_RETENTION_HOURS", "not-a-duration")
	t.Setenv("ALERT_RETENTION_HOURS", "")
	server := &Server{}
	cfg := server.retentionConfigFromEnv()
	// Garbage or empty values should fall back to the conservative defaults.
	if cfg.EventRetention < 30*24*time.Hour {
		t.Fatalf("garbage event retention should fall back, got %s", cfg.EventRetention)
	}
	if cfg.AlertRetention < 90*24*time.Hour {
		t.Fatalf("empty alert retention should fall back, got %s", cfg.AlertRetention)
	}
}

func TestAgeHoursFloorsToWholeHours(t *testing.T) {
	// Each case lists the input duration and the expected integer number of
	// whole hours, computed by `int(d / time.Hour)`. Sub-hour durations
	// truncate to 0; non-positive durations are clamped to 1 to keep the
	// downstream `($N || ' hours')::interval` safe (0 hours would equal
	// "now", which would silently purge everything).
	cases := map[time.Duration]int{
		0:                       1, // clamp: <= 0 -> 1
		-time.Hour:              1, // clamp: negative -> 1
		15 * time.Minute:        0,
		45 * time.Minute:        0,
		59 * time.Minute:        0,
		1 * time.Hour:          1,
		90 * time.Minute:        1, // 1h30m / 1h = 1
		2 * time.Hour:          2,
		23*time.Hour + 59*time.Minute: 23,
		24 * time.Hour:        24,
		48 * time.Hour:        48,
	}
	for in, want := range cases {
		if got := ageHours(in); got != want {
			t.Errorf("ageHours(%s) = %d, want %d", in, got, want)
		}
	}
}

func TestVersionSnapshotReturnsValues(t *testing.T) {
	// Even without ldflags the snapshot must be a valid Info with the dev
	// fallback so that `/api/version` and the frontend Health page can
	// always render a row.
	info := version.Snapshot()
	if info.AppVersion == "" {
		t.Fatalf("AppVersion should never be empty")
	}
	if info.BuildTime == "" {
		t.Fatalf("BuildTime should never be empty")
	}
}

func TestVersionHandleRequiresGET(t *testing.T) {
	server := &Server{}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/version", nil)
		rec := httptest.NewRecorder()
		server.handleVersion(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("method %s: status = %d, want 405", method, rec.Code)
		}
	}
}

func TestRetentionStatusHandleRequiresAdmin(t *testing.T) {
	server := &Server{cfg: Config{RetentionInterval: time.Hour}}
	req := httptest.NewRequest(http.MethodGet, "/api/retention/status", nil)
	rec := httptest.NewRecorder()
	server.handleRetentionStatus(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("retention status without admin: status = %d, want 401", rec.Code)
	}
}

func TestMigrationsHandleRequiresAdmin(t *testing.T) {
	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/migrations", nil)
	rec := httptest.NewRecorder()
	server.handleMigrations(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("migrations without admin: status = %d, want 401", rec.Code)
	}
}

func TestVersionHandleIsPublic(t *testing.T) {
	// /api/version must NOT require admin — load balancers and the frontend
	// pull it without an auth cookie.
	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	rec := httptest.NewRecorder()
	server.handleVersion(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("version endpoint should be public, got 401")
	}
}

func TestRetentionSummaryShapeJSON(t *testing.T) {
	// Pin the JSON shape so a future struct refactor that renames a field
	// breaks loudly here.
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	s := retentionSummary{
		LastRunAt:          &now,
		EventsDeleted:      12,
		AlertsDeleted:      3,
		HeartbeatsDeleted:  100,
		SessionsDeleted:    1,
		IntervalSeconds:    21600,
		EventDays:          30,
		AlertDays:          90,
		HeartbeatDays:      7,
		RevokedSessionDays: 1,
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, k := range []string{
		`"last_run_at"`, `"events_deleted"`, `"alerts_deleted"`,
		`"heartbeats_deleted"`, `"sessions_deleted"`, `"interval_seconds"`,
		`"event_days"`, `"alert_days"`, `"heartbeat_days"`, `"revoked_session_days"`,
	} {
		if !contains(string(raw), k) {
			t.Errorf("payload missing %s: %s", k, raw)
		}
	}
}

// contains is a tiny helper to avoid pulling in strings just for one Contains.
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}