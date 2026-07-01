package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type systemEventResponse struct {
	ID          string          `json:"id"`
	EventType   string          `json:"event_type"`
	EntityType  string          `json:"entity_type"`
	EntityID    *string         `json:"entity_id,omitempty"`
	Severity    string          `json:"severity"`
	Message     string          `json:"message"`
	ActorUserID *string         `json:"actor_user_id,omitempty"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedAt   time.Time       `json:"created_at"`
}

type eventRecord struct {
	EventType   string
	EntityType  string
	EntityID    *string
	Severity    string
	Message     string
	ActorUserID *string
	Metadata    map[string]any
}

func normalizeEventForInsert(event eventRecord) (eventRecord, string) {
	if event.Severity == "" {
		event.Severity = "info"
	}
	if event.Metadata == nil {
		event.Metadata = map[string]any{}
	}
	payload, err := json.Marshal(event.Metadata)
	if err != nil {
		payload = []byte(`{}`)
	}
	return event, string(payload)
}

func (s *Server) recordEvent(r *http.Request, event eventRecord) {
	_ = insertSystemEvent(r.Context(), s.db, event)
}

func insertSystemEvent(ctx context.Context, db *sql.DB, event eventRecord) error {
	if db == nil {
		return nil
	}
	normalized, payload := normalizeEventForInsert(event)
	_, err := db.ExecContext(ctx, `
		INSERT INTO system_events (event_type, entity_type, entity_id, severity, message, actor_user_id, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
	`, normalized.EventType, normalized.EntityType, nullableString(normalized.EntityID), normalized.Severity, normalized.Message, nullableString(normalized.ActorUserID), payload)
	return err
}

type eventFilter struct {
	EventType  string
	EntityType string
	EntityID   string
	StartTime  *time.Time
	EndTime    *time.Time
	Severity   string
	Limit      int
}

func parseEventFilter(r *http.Request) (eventFilter, error) {
	query := r.URL.Query()
	filter := eventFilter{
		EventType:  strings.TrimSpace(query.Get("event_type")),
		EntityType: strings.TrimSpace(query.Get("entity_type")),
		EntityID:   strings.TrimSpace(query.Get("entity_id")),
		Severity:   strings.TrimSpace(query.Get("severity")),
		Limit:      100,
	}
	if filter.EntityID != "" && !isUUID(filter.EntityID) {
		return eventFilter{}, validationError("entity_id must be a UUID")
	}
	if start := strings.TrimSpace(query.Get("start_time")); start != "" {
		parsed, err := time.Parse(time.RFC3339, start)
		if err != nil {
			return eventFilter{}, validationError("start_time must be RFC3339")
		}
		filter.StartTime = &parsed
	}
	if end := strings.TrimSpace(query.Get("end_time")); end != "" {
		parsed, err := time.Parse(time.RFC3339, end)
		if err != nil {
			return eventFilter{}, validationError("end_time must be RFC3339")
		}
		filter.EndTime = &parsed
	}
	if rawLimit := strings.TrimSpace(query.Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err == nil && parsed > 0 && parsed <= 500 {
			filter.Limit = parsed
		}
	}
	return filter, nil
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}

	filter, err := parseEventFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	args := []any{}
	where := []string{"1=1"}
	addTextFilter := func(column, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		args = append(args, value)
		where = append(where, column+" = $"+strconv.Itoa(len(args)))
	}
	addTextFilter("event_type", filter.EventType)
	addTextFilter("entity_type", filter.EntityType)
	addTextFilter("severity", filter.Severity)
	if filter.EntityID != "" {
		args = append(args, filter.EntityID)
		where = append(where, "entity_id = $"+strconv.Itoa(len(args)))
	}
	if filter.StartTime != nil {
		args = append(args, *filter.StartTime)
		where = append(where, "created_at >= $"+strconv.Itoa(len(args)))
	}
	if filter.EndTime != nil {
		args = append(args, *filter.EndTime)
		where = append(where, "created_at <= $"+strconv.Itoa(len(args)))
	}
	args = append(args, filter.Limit)

	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, event_type, entity_type, entity_id, severity, message, actor_user_id, metadata, created_at
		FROM system_events
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY created_at DESC
		LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer rows.Close()
	events := []systemEventResponse{}
	for rows.Next() {
		event, err := scanSystemEvent(rows)
		if err != nil {
			writeDBError(w, err)
			return
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func scanSystemEvent(scanner interface{ Scan(dest ...any) error }) (systemEventResponse, error) {
	var event systemEventResponse
	var entityID sql.NullString
	var actorUserID sql.NullString
	if err := scanner.Scan(&event.ID, &event.EventType, &event.EntityType, &entityID, &event.Severity, &event.Message, &actorUserID, &event.Metadata, &event.CreatedAt); err != nil {
		return systemEventResponse{}, err
	}
	if entityID.Valid {
		event.EntityID = &entityID.String
	}
	if actorUserID.Valid {
		event.ActorUserID = &actorUserID.String
	}
	if len(event.Metadata) == 0 {
		event.Metadata = json.RawMessage(`{}`)
	}
	return event, nil
}

func (s *Server) handleRecorderStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}

	heartbeats, err := s.fetchRecorderHeartbeats(r)
	if err != nil {
		writeDBError(w, err)
		return
	}
	jobs, err := s.fetchRecorderJobs(r)
	if err != nil {
		writeDBError(w, err)
		return
	}
	lastSegments, err := s.fetchLastSegments(r)
	if err != nil {
		writeDBError(w, err)
		return
	}
	errors, err := s.fetchLatestRecorderErrors(r)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"heartbeats":      heartbeats,
		"active_jobs":     jobs,
		"last_segments":   lastSegments,
		"recorder_errors": errors,
	})
}

func (s *Server) fetchRecorderHeartbeats(r *http.Request) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT worker_id, version, status, active_job_count, last_seen_at FROM recorder_heartbeats ORDER BY last_seen_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var workerID, status string
		var version sql.NullString
		var active int
		var lastSeen time.Time
		if err := rows.Scan(&workerID, &version, &status, &active, &lastSeen); err != nil {
			return nil, err
		}
		result = append(result, map[string]any{"worker_id": workerID, "version": version.String, "status": status, "active_job_count": active, "last_seen_at": lastSeen})
	}
	return result, rows.Err()
}

func (s *Server) fetchRecorderJobs(r *http.Request) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT camera_id, worker_id, camera_name, status, started_at, stopped_at, last_error, updated_at FROM recorder_jobs ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var cameraID, workerID, cameraName, status string
		var started, stopped sql.NullTime
		var lastError sql.NullString
		var updated time.Time
		if err := rows.Scan(&cameraID, &workerID, &cameraName, &status, &started, &stopped, &lastError, &updated); err != nil {
			return nil, err
		}
		result = append(result, map[string]any{"camera_id": cameraID, "worker_id": workerID, "camera_name": cameraName, "status": status, "started_at": nullableTimeValue(started), "stopped_at": nullableTimeValue(stopped), "last_error": lastError.String, "updated_at": updated})
	}
	return result, rows.Err()
}

func (s *Server) fetchLastSegments(r *http.Request) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT DISTINCT ON (c.id) c.id, c.name, rs.id, rs.start_time, rs.end_time, rs.status
		FROM cameras c
		LEFT JOIN recording_segments rs ON rs.camera_id = c.id
		ORDER BY c.id, rs.start_time DESC NULLS LAST
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var cameraID, cameraName string
		var segmentID, status sql.NullString
		var start, end sql.NullTime
		if err := rows.Scan(&cameraID, &cameraName, &segmentID, &start, &end, &status); err != nil {
			return nil, err
		}
		result = append(result, map[string]any{"camera_id": cameraID, "camera_name": cameraName, "segment_id": segmentID.String, "start_time": nullableTimeValue(start), "end_time": nullableTimeValue(end), "status": status.String})
	}
	return result, rows.Err()
}

func (s *Server) fetchLatestRecorderErrors(r *http.Request) ([]systemEventResponse, error) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, event_type, entity_type, entity_id, severity, message, actor_user_id, metadata, created_at
		FROM system_events
		WHERE entity_type = 'recorder' AND severity IN ('warning', 'error')
		ORDER BY created_at DESC
		LIMIT 25
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []systemEventResponse{}
	for rows.Next() {
		event, err := scanSystemEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func nullableTimeValue(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time
}
