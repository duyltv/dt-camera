package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	alertRuleTypeRecorderStale         = "recorder_stale"
	alertRuleTypeCameraRecordingFailed = "camera_recording_failed"
	alertRuleTypeStorageLowDisk        = "storage_low_disk"
	alertRuleTypeLiveStreamFailed      = "live_stream_failed"

	alertStatusOpen         = "open"
	alertStatusAcknowledged = "acknowledged"
	alertStatusResolved     = "resolved"
)

var (
	alertRuleTypes = map[string]struct{}{
		alertRuleTypeRecorderStale:         {},
		alertRuleTypeCameraRecordingFailed: {},
		alertRuleTypeStorageLowDisk:        {},
		alertRuleTypeLiveStreamFailed:      {},
	}
	alertSeverities = map[string]struct{}{
		"debug": {}, "info": {}, "warning": {}, "error": {},
	}
	alertStatuses = map[string]struct{}{
		alertStatusOpen: {}, alertStatusAcknowledged: {}, alertStatusResolved: {},
	}
)

// Patterns that must never appear in alert metadata.
var (
	alertSensitiveRTSP   = regexp.MustCompile(`(?i)\brtsp://[^\s"'<>]+`)
	alertSensitiveToken  = regexp.MustCompile(`(?i)\b(token|session|password|passwd|secret|api[_-]?key)\s*[:=]\s*"?[^\s"',;<>]+"?`)
	alertSensitiveBearer = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._-]+`)
)

type alertRule struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Type           string         `json:"type"`
	Enabled        bool           `json:"enabled"`
	Severity       string         `json:"severity"`
	Threshold      map[string]any `json:"threshold"`
	CooldownSeconds int           `json:"cooldown_seconds"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type alertRuleRequest struct {
	Name            string         `json:"name"`
	Type            string         `json:"type"`
	Enabled         *bool          `json:"enabled,omitempty"`
	Severity        string         `json:"severity,omitempty"`
	Threshold       map[string]any `json:"threshold,omitempty"`
	CooldownSeconds *int           `json:"cooldown_seconds,omitempty"`
}

type alertRulePatch struct {
	Name            *string        `json:"name,omitempty"`
	Type            *string        `json:"type,omitempty"`
	Enabled         *bool          `json:"enabled,omitempty"`
	Severity        *string        `json:"severity,omitempty"`
	Threshold       map[string]any `json:"threshold,omitempty"`
	CooldownSeconds *int           `json:"cooldown_seconds,omitempty"`
}

type alert struct {
	ID             string          `json:"id"`
	AlertRuleID    string          `json:"alert_rule_id"`
	EventType      string          `json:"event_type"`
	EntityType     string          `json:"entity_type"`
	EntityID       *string         `json:"entity_id,omitempty"`
	Severity       string          `json:"severity"`
	Status         string          `json:"status"`
	Message        string          `json:"message"`
	Metadata       json.RawMessage `json:"metadata"`
	OpenedAt       time.Time       `json:"opened_at"`
	AcknowledgedAt *time.Time      `json:"acknowledged_at,omitempty"`
	ResolvedAt     *time.Time      `json:"resolved_at,omitempty"`
	RuleName       string          `json:"rule_name,omitempty"`
	RuleType       string          `json:"rule_type,omitempty"`
}

type alertListFilter struct {
	Status     string
	Severity   string
	EntityType string
	EntityID   string
	Limit      int
}

// sanitizeAlertMetadata scrubs RTSP URLs, bearer tokens, and password-shaped
// fields from a metadata map. Returns a new map; never mutates the input.
func sanitizeAlertMetadata(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		key := strings.ToLower(k)
		if key == "password" || key == "passwd" || key == "secret" || key == "token" ||
			key == "session" || key == "api_key" || key == "apikey" {
			out[k] = "[redacted]"
			continue
		}
		switch val := v.(type) {
		case string:
			out[k] = sanitizeAlertString(val)
		case map[string]any:
			out[k] = sanitizeAlertMetadata(val)
		default:
			out[k] = v
		}
	}
	return out
}

func sanitizeAlertString(s string) string {
	if s == "" {
		return s
	}
	out := alertSensitiveRTSP.ReplaceAllString(s, "rtsp://[redacted]")
	out = alertSensitiveBearer.ReplaceAllString(out, "Bearer [redacted]")
	out = alertSensitiveToken.ReplaceAllString(out, "$1=[redacted]")
	return out
}

func normalizeAlertRuleInput(req alertRuleRequest) (alertRule, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return alertRule{}, validationError("name is required")
	}
	typ := strings.TrimSpace(req.Type)
	if _, ok := alertRuleTypes[typ]; !ok {
		return alertRule{}, validationError("type is invalid")
	}
	severity := strings.TrimSpace(req.Severity)
	if severity == "" {
		severity = "warning"
	}
	if _, ok := alertSeverities[severity]; !ok {
		return alertRule{}, validationError("severity is invalid")
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	cooldown := 300
	if req.CooldownSeconds != nil {
		if *req.CooldownSeconds < 0 {
			return alertRule{}, validationError("cooldown_seconds must be >= 0")
		}
		cooldown = *req.CooldownSeconds
	}
	threshold := req.Threshold
	if threshold == nil {
		threshold = map[string]any{}
	}
	return alertRule{
		Name:            name,
		Type:            typ,
		Enabled:         enabled,
		Severity:        severity,
		Threshold:       threshold,
		CooldownSeconds: cooldown,
	}, nil
}

func normalizeAlertRulePatch(p alertRulePatch) (map[string]any, error) {
	updates := map[string]any{}
	if p.Name != nil {
		name := strings.TrimSpace(*p.Name)
		if name == "" {
			return nil, validationError("name is required")
		}
		updates["name"] = name
	}
	if p.Type != nil {
		typ := strings.TrimSpace(*p.Type)
		if _, ok := alertRuleTypes[typ]; !ok {
			return nil, validationError("type is invalid")
		}
		updates["type"] = typ
	}
	if p.Enabled != nil {
		updates["enabled"] = *p.Enabled
	}
	if p.Severity != nil {
		severity := strings.TrimSpace(*p.Severity)
		if _, ok := alertSeverities[severity]; !ok {
			return nil, validationError("severity is invalid")
		}
		updates["severity"] = severity
	}
	if p.Threshold != nil {
		updates["threshold"] = p.Threshold
	}
	if p.CooldownSeconds != nil {
		if *p.CooldownSeconds < 0 {
			return nil, validationError("cooldown_seconds must be >= 0")
		}
		updates["cooldown_seconds"] = *p.CooldownSeconds
	}
	if len(updates) == 0 {
		return nil, validationError("no fields to update")
	}
	return updates, nil
}

func parseAlertListFilter(r *http.Request) (alertListFilter, error) {
	q := r.URL.Query()
	filter := alertListFilter{
		Status:     strings.TrimSpace(q.Get("status")),
		Severity:   strings.TrimSpace(q.Get("severity")),
		EntityType: strings.TrimSpace(q.Get("entity_type")),
		EntityID:   strings.TrimSpace(q.Get("entity_id")),
		Limit:      100,
	}
	if filter.Status != "" {
		if _, ok := alertStatuses[filter.Status]; !ok {
			return filter, validationError("status is invalid")
		}
	}
	if filter.Severity != "" {
		if _, ok := alertSeverities[filter.Severity]; !ok {
			return filter, validationError("severity is invalid")
		}
	}
	if filter.EntityID != "" && !isUUID(filter.EntityID) {
		return filter, validationError("entity_id must be a UUID")
	}
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err == nil && parsed > 0 && parsed <= 500 {
			filter.Limit = parsed
		}
	}
	return filter, nil
}

// ---------- alert_rules CRUD ----------

func (s *Server) handleAlertRules(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listAlertRules(w, r)
	case http.MethodPost:
		s.createAlertRule(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) listAlertRules(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, name, type, enabled, severity, threshold, cooldown_seconds, created_at, updated_at
		FROM alert_rules
		ORDER BY lower(name)
	`)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer rows.Close()
	rules := []alertRule{}
	for rows.Next() {
		rule, err := scanAlertRule(rows)
		if err != nil {
			writeDBError(w, err)
			return
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"alert_rules": rules})
}

func (s *Server) createAlertRule(w http.ResponseWriter, r *http.Request) {
	var req alertRuleRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}
	rule, err := normalizeAlertRuleInput(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	threshold, _ := json.Marshal(rule.Threshold)
	row := s.db.QueryRowContext(r.Context(), `
		INSERT INTO alert_rules (name, type, enabled, severity, threshold, cooldown_seconds)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)
		RETURNING id, name, type, enabled, severity, threshold, cooldown_seconds, created_at, updated_at
	`, rule.Name, rule.Type, rule.Enabled, rule.Severity, string(threshold), rule.CooldownSeconds)
	created, err := scanAlertRule(row)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "duplicate_name", "an alert rule with that name already exists", nil)
			return
		}
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleAlertRuleByID(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	id, action, ok := splitIDAction(r.URL.Path, "/api/alert-rules/")
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "route not found", nil)
		return
	}
	if action != "" {
		writeError(w, http.StatusNotFound, "not_found", "route not found", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.getAlertRule(w, r, id)
	case http.MethodPatch:
		s.patchAlertRule(w, r, id)
	case http.MethodDelete:
		s.deleteAlertRule(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) getAlertRule(w http.ResponseWriter, r *http.Request, id string) {
	row := s.db.QueryRowContext(r.Context(), `
		SELECT id, name, type, enabled, severity, threshold, cooldown_seconds, created_at, updated_at
		FROM alert_rules WHERE id = $1
	`, id)
	rule, err := scanAlertRule(row)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) patchAlertRule(w http.ResponseWriter, r *http.Request, id string) {
	var p alertRulePatch
	if err := readJSON(r, &p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}
	updates, err := normalizeAlertRulePatch(p)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	setClauses := []string{}
	args := []any{}
	for col, val := range updates {
		args = append(args, val)
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	args = append(args, id)
	query := fmt.Sprintf(`UPDATE alert_rules SET %s WHERE id = $%d
		RETURNING id, name, type, enabled, severity, threshold, cooldown_seconds, created_at, updated_at`,
		strings.Join(setClauses, ", "), len(args))
	row := s.db.QueryRowContext(r.Context(), query, args...)
	rule, err := scanAlertRule(row)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "duplicate_name", "an alert rule with that name already exists", nil)
			return
		}
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) deleteAlertRule(w http.ResponseWriter, r *http.Request, id string) {
	result, err := s.db.ExecContext(r.Context(), `DELETE FROM alert_rules WHERE id = $1`, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		writeError(w, http.StatusNotFound, "not_found", "alert rule not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
}

func scanAlertRule(row interface{ Scan(dest ...any) error }) (alertRule, error) {
	var rule alertRule
	var thresholdJSON []byte
	if err := row.Scan(&rule.ID, &rule.Name, &rule.Type, &rule.Enabled, &rule.Severity, &thresholdJSON, &rule.CooldownSeconds, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
		return alertRule{}, err
	}
	if len(thresholdJSON) > 0 {
		_ = json.Unmarshal(thresholdJSON, &rule.Threshold)
	}
	if rule.Threshold == nil {
		rule.Threshold = map[string]any{}
	}
	return rule, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate key value")
}

// ---------- alerts lifecycle ----------

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	filter, err := parseAlertListFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	args := []any{}
	where := []string{"1=1"}
	add := func(col, val string) {
		if val == "" {
			return
		}
		args = append(args, val)
		where = append(where, col+" = $"+strconv.Itoa(len(args)))
	}
	add("a.status", filter.Status)
	add("a.severity", filter.Severity)
	add("a.entity_type", filter.EntityType)
	if filter.EntityID != "" {
		args = append(args, filter.EntityID)
		where = append(where, "a.entity_id = $"+strconv.Itoa(len(args)))
	}
	args = append(args, filter.Limit)
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT a.id, a.alert_rule_id, a.event_type, a.entity_type, a.entity_id,
		       a.severity, a.status, a.message, a.metadata,
		       a.opened_at, a.acknowledged_at, a.resolved_at,
		       r.name, r.type
		FROM alerts a
		JOIN alert_rules r ON r.id = a.alert_rule_id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY a.opened_at DESC
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer rows.Close()
	alerts := []alert{}
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			writeDBError(w, err)
			return
		}
		alerts = append(alerts, a)
	}
	if err := rows.Err(); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": alerts})
}

func (s *Server) handleAlertByID(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	id, action, ok := splitIDAction(r.URL.Path, "/api/alerts/")
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "route not found", nil)
		return
	}
	if action == "" {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	switch r.Method {
	case http.MethodPost:
		switch action {
		case "acknowledge":
			s.acknowledgeAlert(w, r, id)
		case "resolve":
			s.resolveAlert(w, r, id)
		default:
			writeError(w, http.StatusNotFound, "not_found", "route not found", nil)
		}
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) acknowledgeAlert(w http.ResponseWriter, r *http.Request, id string) {
	now := time.Now().UTC()
	row := s.db.QueryRowContext(r.Context(), `
		UPDATE alerts
		SET status = 'acknowledged', acknowledged_at = $2
		WHERE id = $1 AND status = 'open'
		RETURNING id, alert_rule_id, event_type, entity_type, entity_id,
		          severity, status, message, metadata,
		          opened_at, acknowledged_at, resolved_at
	`, id, now)
	a, err := scanAlertJoined(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusConflict, "not_open", "alert is not open", nil)
			return
		}
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) resolveAlert(w http.ResponseWriter, r *http.Request, id string) {
	now := time.Now().UTC()
	row := s.db.QueryRowContext(r.Context(), `
		UPDATE alerts
		SET status = 'resolved', resolved_at = $2,
		    acknowledged_at = COALESCE(acknowledged_at, $2)
		WHERE id = $1 AND status IN ('open', 'acknowledged')
		RETURNING id, alert_rule_id, event_type, entity_type, entity_id,
		          severity, status, message, metadata,
		          opened_at, acknowledged_at, resolved_at
	`, id, now)
	a, err := scanAlertJoined(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusConflict, "already_resolved", "alert is already resolved", nil)
			return
		}
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func scanAlert(row interface{ Scan(dest ...any) error }) (alert, error) {
	var a alert
	var entityID sql.NullString
	var ack, resolved sql.NullTime
	if err := row.Scan(&a.ID, &a.AlertRuleID, &a.EventType, &a.EntityType, &entityID,
		&a.Severity, &a.Status, &a.Message, &a.Metadata,
		&a.OpenedAt, &ack, &resolved, &a.RuleName, &a.RuleType); err != nil {
		return alert{}, err
	}
	if entityID.Valid {
		v := entityID.String
		a.EntityID = &v
	}
	if ack.Valid {
		t := ack.Time
		a.AcknowledgedAt = &t
	}
	if resolved.Valid {
		t := resolved.Time
		a.ResolvedAt = &t
	}
	if len(a.Metadata) == 0 {
		a.Metadata = json.RawMessage(`{}`)
	}
	return a, nil
}

func scanAlertJoined(row interface{ Scan(dest ...any) error }) (alert, error) {
	var a alert
	var entityID sql.NullString
	var ack, resolved sql.NullTime
	if err := row.Scan(&a.ID, &a.AlertRuleID, &a.EventType, &a.EntityType, &entityID,
		&a.Severity, &a.Status, &a.Message, &a.Metadata,
		&a.OpenedAt, &ack, &resolved); err != nil {
		return alert{}, err
	}
	if entityID.Valid {
		v := entityID.String
		a.EntityID = &v
	}
	if ack.Valid {
		t := ack.Time
		a.AcknowledgedAt = &t
	}
	if resolved.Valid {
		t := resolved.Time
		a.ResolvedAt = &t
	}
	if len(a.Metadata) == 0 {
		a.Metadata = json.RawMessage(`{}`)
	}
	return a, nil
}

// ---------- evaluator ----------

type evaluator interface {
	evaluate(ctx context.Context, rule alertRule) ([]alertFinding, error)
}

type alertFinding struct {
	EventType  string
	EntityType string
	EntityID   *string
	Message    string
	Metadata   map[string]any
}

func (s *Server) alertEvaluatorFor(rule alertRule) evaluator {
	switch rule.Type {
	case alertRuleTypeRecorderStale:
		return recorderStaleEvaluator{db: s.db}
	case alertRuleTypeCameraRecordingFailed:
		return cameraRecordingFailedEvaluator{db: s.db}
	case alertRuleTypeStorageLowDisk:
		return storageLowDiskEvaluator{db: s.db}
	case alertRuleTypeLiveStreamFailed:
		return liveStreamFailedEvaluator{db: s.db}
	default:
		return nil
	}
}

type recorderStaleEvaluator struct{ db *sql.DB }

func (e recorderStaleEvaluator) evaluate(ctx context.Context, rule alertRule) ([]alertFinding, error) {
	thresholdSeconds := intThreshold(rule.Threshold, "max_age_seconds", 60)
	if thresholdSeconds < 1 {
		thresholdSeconds = 60
	}
	rows, err := e.db.QueryContext(ctx, `
		SELECT worker_id
		FROM recorder_heartbeats
		WHERE last_seen_at < now() - ($1 || ' seconds')::interval
	`, thresholdSeconds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var findings []alertFinding
	for rows.Next() {
		var workerID string
		if err := rows.Scan(&workerID); err != nil {
			return nil, err
		}
		findings = append(findings, alertFinding{
			EventType:  "alert.recorder_stale",
			EntityType: "recorder",
			EntityID:   nil,
			Message:    fmt.Sprintf("recorder worker %s has not checked in within %ds", workerID, thresholdSeconds),
			Metadata: map[string]any{
				"worker_id":        workerID,
				"max_age_seconds":  thresholdSeconds,
			},
		})
	}
	return findings, rows.Err()
}

type cameraRecordingFailedEvaluator struct{ db *sql.DB }

func (e cameraRecordingFailedEvaluator) evaluate(ctx context.Context, rule alertRule) ([]alertFinding, error) {
	minFailures := intThreshold(rule.Threshold, "min_failures", 3)
	windowSeconds := intThreshold(rule.Threshold, "window_seconds", 900)
	if minFailures < 1 {
		minFailures = 1
	}
	if windowSeconds < 60 {
		windowSeconds = 60
	}
	rows, err := e.db.QueryContext(ctx, `
		SELECT entity_id, COUNT(*)
		FROM system_events
		WHERE event_type = 'recorder.job_failure'
		  AND created_at > now() - ($1 || ' seconds')::interval
		  AND entity_id IS NOT NULL
		GROUP BY entity_id
		HAVING COUNT(*) >= $2
	`, windowSeconds, minFailures)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var findings []alertFinding
	for rows.Next() {
		var cameraID string
		var count int
		if err := rows.Scan(&cameraID, &count); err != nil {
			return nil, err
		}
		id := cameraID
		findings = append(findings, alertFinding{
			EventType:  "alert.camera_recording_failed",
			EntityType: "camera",
			EntityID:   &id,
			Message:    fmt.Sprintf("camera %s has had %d recording failures in the last %ds", short(cameraID), count, windowSeconds),
			Metadata: map[string]any{
				"camera_id":      cameraID,
				"failure_count":  count,
				"window_seconds": windowSeconds,
			},
		})
	}
	return findings, rows.Err()
}

type storageLowDiskEvaluator struct{ db *sql.DB }

func (e storageLowDiskEvaluator) evaluate(ctx context.Context, rule alertRule) ([]alertFinding, error) {
	minUsedPercent := floatThreshold(rule.Threshold, "min_used_percent", 90)
	if minUsedPercent <= 0 || minUsedPercent > 100 {
		minUsedPercent = 90
	}
	rows, err := e.db.QueryContext(ctx, `
		SELECT id, name, COALESCE(used_percent, 0), COALESCE(health_status, 'unknown')
		FROM storage_locations
		WHERE is_enabled = TRUE
		  AND COALESCE(used_percent, 0) >= $1
	`, minUsedPercent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var findings []alertFinding
	for rows.Next() {
		var id, name, health string
		var used float64
		if err := rows.Scan(&id, &name, &used, &health); err != nil {
			return nil, err
		}
		storageID := id
		findings = append(findings, alertFinding{
			EventType:  "alert.storage_low_disk",
			EntityType: "storage_location",
			EntityID:   &storageID,
			Message:    fmt.Sprintf("storage %q is %.1f%% full", name, used),
			Metadata: map[string]any{
				"storage_location_id": id,
				"storage_name":        name,
				"used_percent":        used,
				"health_status":       health,
				"min_used_percent":    minUsedPercent,
			},
		})
	}
	return findings, rows.Err()
}

type liveStreamFailedEvaluator struct{ db *sql.DB }

func (e liveStreamFailedEvaluator) evaluate(ctx context.Context, rule alertRule) ([]alertFinding, error) {
	minFailures := intThreshold(rule.Threshold, "min_failures", 3)
	windowSeconds := intThreshold(rule.Threshold, "window_seconds", 900)
	if minFailures < 1 {
		minFailures = 1
	}
	if windowSeconds < 60 {
		windowSeconds = 60
	}
	rows, err := e.db.QueryContext(ctx, `
		SELECT entity_id, COUNT(*)
		FROM system_events
		WHERE event_type = 'live.failure'
		  AND created_at > now() - ($1 || ' seconds')::interval
		  AND entity_id IS NOT NULL
		GROUP BY entity_id
		HAVING COUNT(*) >= $2
	`, windowSeconds, minFailures)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var findings []alertFinding
	for rows.Next() {
		var cameraID string
		var count int
		if err := rows.Scan(&cameraID, &count); err != nil {
			return nil, err
		}
		id := cameraID
		findings = append(findings, alertFinding{
			EventType:  "alert.live_stream_failed",
			EntityType: "camera",
			EntityID:   &id,
			Message:    fmt.Sprintf("camera %s had %d live stream failures in the last %ds", short(cameraID), count, windowSeconds),
			Metadata: map[string]any{
				"camera_id":      cameraID,
				"failure_count":  count,
				"window_seconds": windowSeconds,
			},
		})
	}
	return findings, rows.Err()
}

func intThreshold(m map[string]any, key string, def int) int {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok {
		return def
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case string:
		if parsed, err := strconv.Atoi(t); err == nil {
			return parsed
		}
	}
	return def
}

func floatThreshold(m map[string]any, key string, def float64) float64 {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok {
		return def
	}
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		if parsed, err := strconv.ParseFloat(t, 64); err == nil {
			return parsed
		}
	}
	return def
}

func short(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

// evaluateAlerts runs all enabled rules once. Used by both the periodic loop
// and unit tests.
func (s *Server) evaluateAlerts(ctx context.Context) error {
	if s.db == nil {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, type, enabled, severity, threshold, cooldown_seconds, created_at, updated_at
		FROM alert_rules WHERE enabled = TRUE
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var rules []alertRule
	for rows.Next() {
		rule, err := scanAlertRule(rows)
		if err != nil {
			return err
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, rule := range rules {
		ev := s.alertEvaluatorFor(rule)
		if ev == nil {
			continue
		}
		findings, err := ev.evaluate(ctx, rule)
		if err != nil {
			return err
		}
		for _, f := range findings {
			if err := s.openAlertIfAllowed(ctx, rule, f); err != nil {
				return err
			}
		}
	}
	return nil
}

// openAlertIfAllowed opens a new alert unless one is already open for the same
// (rule, entity), or unless the most recent alert for that pair was opened
// within the rule's cooldown window.
func (s *Server) openAlertIfAllowed(ctx context.Context, rule alertRule, f alertFinding) error {
	// Has an open alert already?
	var openCount int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM alerts
		WHERE alert_rule_id = $1
		  AND entity_type = $2
		  AND status = 'open'
		  AND (
		    (entity_id IS NOT NULL AND entity_id = $3) OR
		    (entity_id IS NULL AND $3::uuid IS NULL)
		  )
	`, rule.ID, f.EntityType, nullableString(f.EntityID)).Scan(&openCount); err != nil {
		return err
	}
	if openCount > 0 {
		return nil
	}
	// Cooldown check: was the most recent alert for this (rule, entity) opened
	// within the cooldown window?
	if rule.CooldownSeconds > 0 {
		var lastOpened sql.NullTime
		row := s.db.QueryRowContext(ctx, `
			SELECT MAX(opened_at)
			FROM alerts
			WHERE alert_rule_id = $1 AND entity_type = $2
			  AND (
			    (entity_id IS NOT NULL AND entity_id = $3) OR
			    (entity_id IS NULL AND $3::uuid IS NULL)
			  )
		`, rule.ID, f.EntityType, nullableString(f.EntityID))
		if err := row.Scan(&lastOpened); err != nil {
			return err
		}
		if lastOpened.Valid && time.Since(lastOpened.Time) < time.Duration(rule.CooldownSeconds)*time.Second {
			return nil
		}
	}
	metadata := sanitizeAlertMetadata(f.Metadata)
	payload, _ := json.Marshal(metadata)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO alerts (alert_rule_id, event_type, entity_type, entity_id,
		                    severity, status, message, metadata)
		VALUES ($1, $2, $3, $4, $5, 'open', $6, $7::jsonb)
	`, rule.ID, f.EventType, f.EntityType, nullableString(f.EntityID),
		rule.Severity, f.Message, string(payload))
	return err
}

// alertEvaluationLoop runs evaluateAlerts on a ticker until ctx is canceled.
func (s *Server) alertEvaluationLoop(ctx context.Context, interval time.Duration, stop <-chan struct{}) {
	if interval <= 0 || s.db == nil {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	s.evaluateAlerts(ctx)
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.evaluateAlerts(ctx)
		}
	}
}