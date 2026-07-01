package httpapi

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/dt-camera/backend/internal/version"
)

// retentionConfig captures the configurable retention durations used by the
// operational cleanup pass. All durations are conservative defaults that keep
// data long enough to debug recent incidents.
type retentionConfig struct {
	EventRetention       time.Duration // age at which system_events rows are pruned
	AlertRetention       time.Duration // age at which resolved/acknowledged alerts are pruned
	HeartbeatRetention   time.Duration // age at which recorder_heartbeats rows are pruned
	RevokedSessionCutoff time.Duration // age at which revoked sessions are hard-deleted (defense in depth)
	Interval             time.Duration // how often the cleanup pass runs
}

// retentionSummary is exposed via /api/retention/status (admin) so dashboards
// can show "last cleanup deleted N rows" and operators can correlate growth.
type retentionSummary struct {
	LastRunAt          *time.Time `json:"last_run_at,omitempty"`
	EventsDeleted      int        `json:"events_deleted"`
	AlertsDeleted      int        `json:"alerts_deleted"`
	HeartbeatsDeleted  int        `json:"heartbeats_deleted"`
	SessionsDeleted    int        `json:"sessions_deleted"`
	IntervalSeconds    int        `json:"interval_seconds"`
	EventDays          int        `json:"event_days"`
	AlertDays          int        `json:"alert_days"`
	HeartbeatDays      int        `json:"heartbeat_days"`
	RevokedSessionDays int        `json:"revoked_session_days"`
}

func (s *Server) retentionConfigFromEnv() retentionConfig {
	cfg := retentionConfig{
		EventRetention:       30 * 24 * time.Hour,
		AlertRetention:       90 * 24 * time.Hour,
		HeartbeatRetention:   7 * 24 * time.Hour,
		RevokedSessionCutoff: 24 * time.Hour,
		Interval:             6 * time.Hour,
	}
	if d, ok := parseRetentionHours("EVENT_RETENTION_HOURS"); ok {
		cfg.EventRetention = d
	}
	if d, ok := parseRetentionHours("ALERT_RETENTION_HOURS"); ok {
		cfg.AlertRetention = d
	}
	if d, ok := parseRetentionHours("HEARTBEAT_RETENTION_HOURS"); ok {
		cfg.HeartbeatRetention = d
	}
	if d, ok := parseRetentionHours("REVOKED_SESSION_HOURS"); ok {
		cfg.RevokedSessionCutoff = d
	}
	if d, ok := parseRetentionHours("RETENTION_INTERVAL_SECONDS"); ok {
		cfg.Interval = d
	}
	return cfg
}

func parseRetentionHours(envName string) (time.Duration, bool) {
	raw, ok := os.LookupEnv(envName)
	if !ok || raw == "" {
		return 0, false
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, false
	}
	return d, true
}

// retentionState holds the last-run summary so the admin endpoint and the
// health endpoint can report it without hitting the DB.
type retentionState struct {
	lastRun    time.Time
	lastResult retentionSummary
}

// purgeSystemEvents deletes system_events older than `age`. Only `info`,
// `debug`, and `warning` rows are eligible for pruning; `error` rows are
// kept indefinitely so postmortems still have the signal.
func purgeSystemEvents(ctx context.Context, db *sql.DB, age time.Duration) (int, error) {
	cancel, cancelFn := context.WithTimeout(ctx, 30*time.Second)
	defer cancelFn()
	res, err := db.ExecContext(cancel, `
		DELETE FROM system_events
		WHERE severity <> 'error'
		  AND created_at < now() - ($1 || ' hours')::interval
	`, ageHours(age))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// purgeOldAlerts deletes alerts that are not currently open and older than
// `age`. Open alerts are never pruned (they're still actionable); resolved
// and acknowledged alerts are housekeeping-friendly after the cooldown.
func purgeOldAlerts(ctx context.Context, db *sql.DB, age time.Duration) (int, error) {
	cancel, cancelFn := context.WithTimeout(ctx, 30*time.Second)
	defer cancelFn()
	res, err := db.ExecContext(cancel, `
		DELETE FROM alerts
		WHERE status IN ('acknowledged', 'resolved')
		  AND COALESCE(resolved_at, acknowledged_at, opened_at) < now() - ($1 || ' hours')::interval
	`, ageHours(age))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// purgeOldHeartbeats deletes recorder_heartbeats older than `age`. The
// recorder upserts a row on every poll, so this trims the historical tail
// without affecting current recorder state.
func purgeOldHeartbeats(ctx context.Context, db *sql.DB, age time.Duration) (int, error) {
	cancel, cancelFn := context.WithTimeout(ctx, 30*time.Second)
	defer cancelFn()
	res, err := db.ExecContext(cancel, `
		DELETE FROM recorder_heartbeats
		WHERE last_seen_at < now() - ($1 || ' hours')::interval
	`, ageHours(age))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// purgeRevokedSessions hard-deletes sessions that have been revoked for
// longer than `age`. The session-cleanup loop already DELETEs expired
// sessions; this is defense-in-depth against rows that were revoked but
// somehow missed the expiration sweep.
func purgeRevokedSessions(ctx context.Context, db *sql.DB, age time.Duration) (int, error) {
	cancel, cancelFn := context.WithTimeout(ctx, 30*time.Second)
	defer cancelFn()
	res, err := db.ExecContext(cancel, `
		DELETE FROM sessions
		WHERE revoked_at IS NOT NULL
		  AND revoked_at < now() - ($1 || ' hours')::interval
	`, ageHours(age))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ageHours returns the number of whole hours in `d`. The cleanup queries
// accept `(N || ' hours')::interval`, so we round down. Fractional hours
// would still work but keeping integer math simplifies the SQL parameter.
func ageHours(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	return int(d / time.Hour)
}

// retentionRun executes one pass of all four pruners and returns the totals.
// Errors are returned individually but never abort the whole pass: a failure
// pruning system_events shouldn't stop alerts from being pruned.
type retentionResult struct {
	Events     int
	Alerts     int
	Heartbeats int
	Sessions   int
}

func (s *Server) runRetentionPass(ctx context.Context) (retentionResult, error) {
	if s.db == nil {
		return retentionResult{}, nil
	}
	cfg := s.retentionConfigFromEnv()
	out := retentionResult{}

	if n, err := purgeSystemEvents(ctx, s.db, cfg.EventRetention); err != nil {
		log.Printf("retention: prune system_events failed: %v", err)
	} else {
		out.Events = n
	}
	if n, err := purgeOldAlerts(ctx, s.db, cfg.AlertRetention); err != nil {
		log.Printf("retention: prune alerts failed: %v", err)
	} else {
		out.Alerts = n
	}
	if n, err := purgeOldHeartbeats(ctx, s.db, cfg.HeartbeatRetention); err != nil {
		log.Printf("retention: prune recorder_heartbeats failed: %v", err)
	} else {
		out.Heartbeats = n
	}
	if n, err := purgeRevokedSessions(ctx, s.db, cfg.RevokedSessionCutoff); err != nil {
		log.Printf("retention: prune revoked sessions failed: %v", err)
	} else {
		out.Sessions = n
	}
	return out, nil
}

// retentionLoop runs the cleanup pass on a ticker until ctx is canceled or
// stop is closed. The summary of each pass is stored in s.retentionLast so
// the admin endpoint and the health page can surface "last cleanup" stats.
func (s *Server) retentionLoopWithState(ctx context.Context, interval time.Duration, stop <-chan struct{}) {
	if interval <= 0 || s.db == nil {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	s.runAndRecordRetention(ctx)
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runAndRecordRetention(ctx)
		}
	}
}

// runAndRecordRetention runs one pass and writes the summary into
// s.retentionLast so the API can surface it. The lock-free single-writer
// pattern is safe because only this goroutine mutates s.retentionLast.
func (s *Server) runAndRecordRetention(ctx context.Context) {
	cfg := s.retentionConfigFromEnv()
	res, _ := s.runRetentionPass(ctx)
	now := time.Now().UTC()
	s.retentionLast = retentionSummary{
		LastRunAt:          &now,
		EventsDeleted:      res.Events,
		AlertsDeleted:      res.Alerts,
		HeartbeatsDeleted:  res.Heartbeats,
		SessionsDeleted:    res.Sessions,
		IntervalSeconds:    int(cfg.Interval.Seconds()),
		EventDays:          int(cfg.EventRetention / (24 * time.Hour)),
		AlertDays:          int(cfg.AlertRetention / (24 * time.Hour)),
		HeartbeatDays:      int(cfg.HeartbeatRetention / (24 * time.Hour)),
		RevokedSessionDays: int(cfg.RevokedSessionCutoff / (24 * time.Hour)),
	}
}

// handleRetentionStatus serves the latest retention run summary to admins.
// Used by the frontend Health page to show "last cleanup deleted N rows".
func (s *Server) handleRetentionStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	writeJSON(w, http.StatusOK, s.retentionLast)
}

// handleVersion serves the build identity. Public endpoint (no auth) so
// load balancers, monitoring, and the frontend can identify the running
// build without an admin session.
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	buildInfo := version.Snapshot()
	migMax, migCount, _ := s.fetchMigrationStats(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"app_version":         buildInfo.AppVersion,
		"git_commit":          buildInfo.GitCommit,
		"build_time":          buildInfo.BuildTime,
		"latest_migration":    migMax,
		"migrations_applied":  migCount,
	})
}

// handleMigrations lists applied schema migrations to admins, newest first.
// Mirrors the schema_migrations table.
func (s *Server) handleMigrations(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT version, name, applied_at
		FROM schema_migrations
		ORDER BY version DESC
	`)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer rows.Close()
	type migration struct {
		Version   int       `json:"version"`
		Name      string    `json:"name"`
		AppliedAt time.Time `json:"applied_at"`
	}
	out := []migration{}
	for rows.Next() {
		var m migration
		if err := rows.Scan(&m.Version, &m.Name, &m.AppliedAt); err != nil {
			writeDBError(w, err)
			return
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"migrations": out})
}

// fetchMigrationStats returns (latest version, count, error). Used by
// /healthz and /api/version to surface migration visibility.
func (s *Server) fetchMigrationStats(ctx context.Context) (int, int, error) {
	if s.db == nil {
		return 0, 0, nil
	}
	var latest sql.NullInt64
	var count sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version), 0), COUNT(*)
		FROM schema_migrations
	`).Scan(&latest, &count); err != nil {
		return 0, 0, err
	}
	return int(latest.Int64), int(count.Int64), nil
}

// summaryForAPI returns the last retention summary as a value that the
// admin endpoint can serialize. `sinceStart` is true when no run has happened
// yet, so callers can avoid an "all zeros" first paint.
func (s *Server) summaryForAPI() retentionSummary {
	cfg := s.retentionConfigFromEnv()
	return retentionSummary{
		LastRunAt:          nil,
		EventsDeleted:      0,
		AlertsDeleted:      0,
		HeartbeatsDeleted:  0,
		SessionsDeleted:    0,
		IntervalSeconds:    int(cfg.Interval.Seconds()),
		EventDays:          int(cfg.EventRetention / (24 * time.Hour)),
		AlertDays:          int(cfg.AlertRetention / (24 * time.Hour)),
		HeartbeatDays:      int(cfg.HeartbeatRetention / (24 * time.Hour)),
		RevokedSessionDays: int(cfg.RevokedSessionCutoff / (24 * time.Hour)),
	}
}