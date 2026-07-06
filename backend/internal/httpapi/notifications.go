package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maxNotificationDeliveries = 1000

type notificationChannelResponse struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Method    string          `json:"method"`
	Enabled   bool            `json:"enabled"`
	Config    json.RawMessage `json:"config"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type notificationRuleResponse struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	EventType             string    `json:"event_type"`
	Enabled               bool      `json:"enabled"`
	NotificationChannelID string    `json:"notification_channel_id"`
	CameraID              *string   `json:"camera_id,omitempty"`
	CooldownSeconds       int       `json:"cooldown_seconds"`
	MessageTemplate       string    `json:"message_template"`
	AttachImage           bool      `json:"attach_image"`
	AttachVideo           bool      `json:"attach_video"`
	PreEventSeconds       int       `json:"pre_event_seconds"`
	PostEventSeconds      int       `json:"post_event_seconds"`
	VideoFPS              int       `json:"video_fps"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type notificationChannelRequest struct {
	Name    string         `json:"name"`
	Method  string         `json:"method"`
	Enabled *bool          `json:"enabled,omitempty"`
	Config  map[string]any `json:"config,omitempty"`
}

type notificationRuleRequest struct {
	Name                  string  `json:"name"`
	EventType             string  `json:"event_type"`
	Enabled               *bool   `json:"enabled,omitempty"`
	NotificationChannelID string  `json:"notification_channel_id"`
	CameraID              *string `json:"camera_id,omitempty"`
	CooldownSeconds       *int    `json:"cooldown_seconds,omitempty"`
	MessageTemplate       string  `json:"message_template,omitempty"`
	AttachImage           *bool   `json:"attach_image,omitempty"`
	AttachVideo           *bool   `json:"attach_video,omitempty"`
	PreEventSeconds       *int    `json:"pre_event_seconds,omitempty"`
	PostEventSeconds      *int    `json:"post_event_seconds,omitempty"`
	VideoFPS              *int    `json:"video_fps,omitempty"`
}

var notificationRuleEventTypes = map[string]struct{}{
	"motion_detected":         {},
	"human_detected":          {},
	"person_detected":         {},
	"person_recognized":       {},
	"unknown_person_detected": {},
	"classification":          {},
	"observation_created":     {},
}

type notificationDeliveryResponse struct {
	ID                    string     `json:"id"`
	NotificationRuleID    string     `json:"notification_rule_id"`
	NotificationChannelID string     `json:"notification_channel_id"`
	RuleName              string     `json:"rule_name"`
	ChannelName           string     `json:"channel_name"`
	ChannelMethod         string     `json:"channel_method"`
	EventType             string     `json:"event_type"`
	EntityType            string     `json:"entity_type"`
	EntityID              *string    `json:"entity_id,omitempty"`
	CameraID              *string    `json:"camera_id,omitempty"`
	CameraName            *string    `json:"camera_name,omitempty"`
	Status                string     `json:"status"`
	Error                 *string    `json:"error,omitempty"`
	SentAt                *time.Time `json:"sent_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

func (s *Server) handleNotificationChannels(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listNotificationChannels(w, r)
	case http.MethodPost:
		s.createNotificationChannel(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleNotificationChannelByID(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	id, action, ok := splitIDAction(r.URL.Path, "/api/notification-channels/")
	if !ok || !isUUID(id) || action != "" {
		writeError(w, http.StatusNotFound, "not_found", "notification channel not found", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.getNotificationChannel(w, r, id)
	case http.MethodPatch:
		s.updateNotificationChannel(w, r, id)
	case http.MethodDelete:
		s.deleteNotificationChannel(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleNotificationRules(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listNotificationRules(w, r)
	case http.MethodPost:
		s.createNotificationRule(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleNotificationRuleByID(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	id, action, ok := splitIDAction(r.URL.Path, "/api/notification-rules/")
	if !ok || !isUUID(id) || action != "" {
		writeError(w, http.StatusNotFound, "not_found", "notification rule not found", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.getNotificationRule(w, r, id)
	case http.MethodPatch:
		s.updateNotificationRule(w, r, id)
	case http.MethodDelete:
		s.deleteNotificationRule(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleNotificationDeliveries(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	s.listNotificationDeliveries(w, r)
}

func (s *Server) listNotificationChannels(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, name, method, enabled, config, created_at, updated_at
		FROM notification_channels
		ORDER BY name
	`)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer rows.Close()
	items := []notificationChannelResponse{}
	for rows.Next() {
		item, err := scanNotificationChannel(rows)
		if err != nil {
			writeDBError(w, err)
			return
		}
		items = append(items, redactNotificationChannel(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"notification_channels": items})
}

func (s *Server) createNotificationChannel(w http.ResponseWriter, r *http.Request) {
	var req notificationChannelRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	method := strings.TrimSpace(req.Method)
	if name == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "name is required", nil)
		return
	}
	if method != "telegram" {
		writeError(w, http.StatusBadRequest, "validation_error", "method must be telegram", nil)
		return
	}
	if err := validateNotificationChannelConfig(method, req.Config); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	payload, _ := json.Marshal(req.Config)
	row := s.db.QueryRowContext(r.Context(), `
		INSERT INTO notification_channels (name, method, enabled, config)
		VALUES ($1, $2, $3, $4::jsonb)
		RETURNING id, name, method, enabled, config, created_at, updated_at
	`, name, method, enabled, string(payload))
	item, err := scanNotificationChannel(row)
	if err != nil {
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "notification.channel_create", EntityType: "notification_channel", EntityID: &item.ID, Message: "notification channel created"})
	writeJSON(w, http.StatusCreated, redactNotificationChannel(item))
}

func (s *Server) getNotificationChannel(w http.ResponseWriter, r *http.Request, id string) {
	item, err := s.findNotificationChannel(r, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, redactNotificationChannel(item))
}

func (s *Server) updateNotificationChannel(w http.ResponseWriter, r *http.Request, id string) {
	var req notificationChannelRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}
	current, err := s.findNotificationChannelRaw(r, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = current.Name
	}
	method := strings.TrimSpace(req.Method)
	if method == "" {
		method = current.Method
	}
	if method != "telegram" {
		writeError(w, http.StatusBadRequest, "validation_error", "method must be telegram", nil)
		return
	}
	config := req.Config
	if config == nil {
		config = rawJSONToMap(current.Config)
	} else {
		currentConfig := rawJSONToMap(current.Config)
		if strings.TrimSpace(stringMapValue(config, "bot_token")) == "" || stringMapValue(config, "bot_token") == "[redacted]" {
			if token := strings.TrimSpace(stringMapValue(currentConfig, "bot_token")); token != "" {
				config["bot_token"] = token
			}
		}
		if strings.TrimSpace(stringMapValue(config, "chat_id")) == "" {
			if chatID := strings.TrimSpace(stringMapValue(currentConfig, "chat_id")); chatID != "" {
				config["chat_id"] = chatID
			}
		}
	}
	if err := validateNotificationChannelConfig(method, config); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	payload, _ := json.Marshal(config)
	row := s.db.QueryRowContext(r.Context(), `
		UPDATE notification_channels
		SET name = $2, method = $3, enabled = $4, config = $5::jsonb
		WHERE id = $1
		RETURNING id, name, method, enabled, config, created_at, updated_at
	`, id, name, method, enabled, string(payload))
	item, err := scanNotificationChannel(row)
	if err != nil {
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "notification.channel_update", EntityType: "notification_channel", EntityID: &item.ID, Message: "notification channel updated"})
	writeJSON(w, http.StatusOK, redactNotificationChannel(item))
}

func (s *Server) deleteNotificationChannel(w http.ResponseWriter, r *http.Request, id string) {
	result, err := s.db.ExecContext(r.Context(), `DELETE FROM notification_channels WHERE id = $1`, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		writeDBError(w, sql.ErrNoRows)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "notification.channel_delete", EntityType: "notification_channel", EntityID: &id, Message: "notification channel deleted"})
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) listNotificationRules(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, name, event_type, enabled, notification_channel_id, camera_id,
			cooldown_seconds, message_template, attach_image, attach_video, pre_event_seconds,
			post_event_seconds, video_fps, created_at, updated_at
		FROM notification_rules
		ORDER BY name
	`)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer rows.Close()
	items := []notificationRuleResponse{}
	for rows.Next() {
		item, err := scanNotificationRule(rows)
		if err != nil {
			writeDBError(w, err)
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"notification_rules": items})
}

func (s *Server) listNotificationDeliveries(w http.ResponseWriter, r *http.Request) {
	limit := 50
	offset := 0
	const maxWindow = maxNotificationDeliveries
	query := r.URL.Query()
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}
	if raw := strings.TrimSpace(query.Get("offset")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err == nil && parsed > 0 {
			offset = parsed
		}
	}
	if offset >= maxWindow {
		offset = maxWindow - limit
	}
	if offset < 0 {
		offset = 0
	}
	if offset+limit > maxWindow {
		limit = maxWindow - offset
	}
	if err := s.pruneNotificationDeliveries(r); err != nil {
		writeDBError(w, err)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT d.id, d.notification_rule_id, d.notification_channel_id,
			COALESCE(r.name, 'Deleted rule') AS rule_name,
			COALESCE(ch.name, 'Deleted channel') AS channel_name,
			COALESCE(ch.method, 'telegram') AS channel_method,
			d.event_type, d.entity_type, d.entity_id, d.camera_id, c.name AS camera_name,
			d.status, d.error, d.sent_at, d.created_at, d.updated_at
		FROM notification_deliveries d
		LEFT JOIN notification_rules r ON r.id = d.notification_rule_id
		LEFT JOIN notification_channels ch ON ch.id = d.notification_channel_id
		LEFT JOIN cameras c ON c.id = d.camera_id
		ORDER BY d.created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer rows.Close()
	items := []notificationDeliveryResponse{}
	for rows.Next() {
		item, err := scanNotificationDelivery(rows)
		if err != nil {
			writeDBError(w, err)
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeDBError(w, err)
		return
	}

	var totalLatest int
	if err := s.db.QueryRowContext(r.Context(), `
		SELECT LEAST(COUNT(*), $1)::int FROM notification_deliveries
	`, maxWindow).Scan(&totalLatest); err != nil {
		writeDBError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"notification_deliveries": items,
		"limit":                   limit,
		"offset":                  offset,
		"total_latest":            totalLatest,
		"max_window":              maxWindow,
	})
}

func (s *Server) pruneNotificationDeliveries(r *http.Request) error {
	_, err := s.db.ExecContext(r.Context(), `
		DELETE FROM notification_deliveries
		WHERE id NOT IN (
			SELECT id
			FROM notification_deliveries
			ORDER BY created_at DESC, id DESC
			LIMIT $1
		)
	`, maxNotificationDeliveries)
	return err
}

func (s *Server) createNotificationRule(w http.ResponseWriter, r *http.Request) {
	req, ok := s.readNotificationRuleRequest(w, r)
	if !ok {
		return
	}
	row := s.db.QueryRowContext(r.Context(), `
		INSERT INTO notification_rules (
			name, event_type, enabled, notification_channel_id, camera_id, cooldown_seconds,
			message_template, attach_image, attach_video, pre_event_seconds, post_event_seconds, video_fps
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, name, event_type, enabled, notification_channel_id, camera_id,
			cooldown_seconds, message_template, attach_image, attach_video, pre_event_seconds,
			post_event_seconds, video_fps, created_at, updated_at
	`, req.Name, req.EventType, boolPtrValue(req.Enabled, true), req.NotificationChannelID, nullableString(req.CameraID), intPtrValue(req.CooldownSeconds, 300), normalizeMessageTemplate(req.MessageTemplate), boolPtrValue(req.AttachImage, true), boolPtrValue(req.AttachVideo, true), intPtrValue(req.PreEventSeconds, 7), intPtrValue(req.PostEventSeconds, 3), intPtrValue(req.VideoFPS, 4))
	item, err := scanNotificationRule(row)
	if err != nil {
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "notification.rule_create", EntityType: "notification_rule", EntityID: &item.ID, Message: "notification rule created"})
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) getNotificationRule(w http.ResponseWriter, r *http.Request, id string) {
	item, err := s.findNotificationRule(r, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) updateNotificationRule(w http.ResponseWriter, r *http.Request, id string) {
	req, ok := s.readNotificationRuleRequest(w, r)
	if !ok {
		return
	}
	current, err := s.findNotificationRule(r, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	attachImage := current.AttachImage
	if req.AttachImage != nil {
		attachImage = *req.AttachImage
	}
	attachVideo := current.AttachVideo
	if req.AttachVideo != nil {
		attachVideo = *req.AttachVideo
	}
	cooldownSeconds := current.CooldownSeconds
	if req.CooldownSeconds != nil {
		cooldownSeconds = *req.CooldownSeconds
	}
	preEventSeconds := current.PreEventSeconds
	if req.PreEventSeconds != nil {
		preEventSeconds = *req.PreEventSeconds
	}
	postEventSeconds := current.PostEventSeconds
	if req.PostEventSeconds != nil {
		postEventSeconds = *req.PostEventSeconds
	}
	videoFPS := current.VideoFPS
	if req.VideoFPS != nil {
		videoFPS = *req.VideoFPS
	}
	messageTemplate := current.MessageTemplate
	if strings.TrimSpace(req.MessageTemplate) != "" {
		messageTemplate = normalizeMessageTemplate(req.MessageTemplate)
	}
	row := s.db.QueryRowContext(r.Context(), `
		UPDATE notification_rules
		SET name = $2, event_type = $3, enabled = $4, notification_channel_id = $5,
			camera_id = $6, cooldown_seconds = $7, message_template = $8, attach_image = $9, attach_video = $10,
			pre_event_seconds = $11, post_event_seconds = $12, video_fps = $13
		WHERE id = $1
		RETURNING id, name, event_type, enabled, notification_channel_id, camera_id,
			cooldown_seconds, message_template, attach_image, attach_video, pre_event_seconds,
			post_event_seconds, video_fps, created_at, updated_at
	`, id, req.Name, req.EventType, enabled, req.NotificationChannelID, nullableString(req.CameraID), cooldownSeconds, messageTemplate, attachImage, attachVideo, preEventSeconds, postEventSeconds, videoFPS)
	item, err := scanNotificationRule(row)
	if err != nil {
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "notification.rule_update", EntityType: "notification_rule", EntityID: &item.ID, Message: "notification rule updated"})
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) deleteNotificationRule(w http.ResponseWriter, r *http.Request, id string) {
	result, err := s.db.ExecContext(r.Context(), `DELETE FROM notification_rules WHERE id = $1`, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		writeDBError(w, sql.ErrNoRows)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "notification.rule_delete", EntityType: "notification_rule", EntityID: &id, Message: "notification rule deleted"})
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) readNotificationRuleRequest(w http.ResponseWriter, r *http.Request) (notificationRuleRequest, bool) {
	var req notificationRuleRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return req, false
	}
	req.Name = strings.TrimSpace(req.Name)
	req.EventType = strings.TrimSpace(req.EventType)
	req.NotificationChannelID = strings.TrimSpace(req.NotificationChannelID)
	if req.Name == "" || req.EventType == "" || req.NotificationChannelID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "name, event_type, and notification_channel_id are required", nil)
		return req, false
	}
	if _, ok := notificationRuleEventTypes[req.EventType]; !ok {
		writeError(w, http.StatusBadRequest, "validation_error", "event_type is invalid", nil)
		return req, false
	}
	if !isUUID(req.NotificationChannelID) {
		writeError(w, http.StatusBadRequest, "validation_error", "notification_channel_id must be a UUID", nil)
		return req, false
	}
	if req.CameraID != nil {
		cleaned := strings.TrimSpace(*req.CameraID)
		if cleaned == "" {
			req.CameraID = nil
		} else if !isUUID(cleaned) {
			writeError(w, http.StatusBadRequest, "validation_error", "camera_id must be a UUID", nil)
			return req, false
		} else {
			req.CameraID = &cleaned
		}
	}
	if err := s.ensureNotificationChannelExists(r, req.NotificationChannelID); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return req, false
	}
	if req.CameraID != nil {
		if err := s.ensureCameraExists(r, *req.CameraID); err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
			return req, false
		}
	}
	if intPtrValue(req.CooldownSeconds, 300) < 0 || intPtrValue(req.PreEventSeconds, 7) < 0 || intPtrValue(req.PostEventSeconds, 3) < 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "cooldown and evidence window values must be greater than or equal to zero", nil)
		return req, false
	}
	if fps := intPtrValue(req.VideoFPS, 4); fps < 1 || fps > 15 {
		writeError(w, http.StatusBadRequest, "validation_error", "video_fps must be between 1 and 15", nil)
		return req, false
	}
	return req, true
}

func (s *Server) findNotificationChannel(r *http.Request, id string) (notificationChannelResponse, error) {
	item, err := s.findNotificationChannelRaw(r, id)
	if err != nil {
		return notificationChannelResponse{}, err
	}
	return redactNotificationChannel(item), nil
}

func (s *Server) findNotificationChannelRaw(r *http.Request, id string) (notificationChannelResponse, error) {
	row := s.db.QueryRowContext(r.Context(), `
		SELECT id, name, method, enabled, config, created_at, updated_at
		FROM notification_channels
		WHERE id = $1
	`, id)
	return scanNotificationChannel(row)
}

func (s *Server) findNotificationRule(r *http.Request, id string) (notificationRuleResponse, error) {
	row := s.db.QueryRowContext(r.Context(), `
		SELECT id, name, event_type, enabled, notification_channel_id, camera_id,
			cooldown_seconds, message_template, attach_image, attach_video, pre_event_seconds,
			post_event_seconds, video_fps, created_at, updated_at
		FROM notification_rules
		WHERE id = $1
	`, id)
	return scanNotificationRule(row)
}

func scanNotificationChannel(scanner interface{ Scan(dest ...any) error }) (notificationChannelResponse, error) {
	var item notificationChannelResponse
	if err := scanner.Scan(&item.ID, &item.Name, &item.Method, &item.Enabled, &item.Config, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return notificationChannelResponse{}, err
	}
	if len(item.Config) == 0 {
		item.Config = json.RawMessage(`{}`)
	}
	return item, nil
}

func scanNotificationRule(scanner interface{ Scan(dest ...any) error }) (notificationRuleResponse, error) {
	var item notificationRuleResponse
	var cameraID sql.NullString
	if err := scanner.Scan(
		&item.ID,
		&item.Name,
		&item.EventType,
		&item.Enabled,
		&item.NotificationChannelID,
		&cameraID,
		&item.CooldownSeconds,
		&item.MessageTemplate,
		&item.AttachImage,
		&item.AttachVideo,
		&item.PreEventSeconds,
		&item.PostEventSeconds,
		&item.VideoFPS,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return notificationRuleResponse{}, err
	}
	if cameraID.Valid {
		item.CameraID = &cameraID.String
	}
	return item, nil
}

func scanNotificationDelivery(scanner interface{ Scan(dest ...any) error }) (notificationDeliveryResponse, error) {
	var item notificationDeliveryResponse
	var entityID sql.NullString
	var cameraID sql.NullString
	var cameraName sql.NullString
	var errText sql.NullString
	var sentAt sql.NullTime
	if err := scanner.Scan(
		&item.ID,
		&item.NotificationRuleID,
		&item.NotificationChannelID,
		&item.RuleName,
		&item.ChannelName,
		&item.ChannelMethod,
		&item.EventType,
		&item.EntityType,
		&entityID,
		&cameraID,
		&cameraName,
		&item.Status,
		&errText,
		&sentAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return notificationDeliveryResponse{}, err
	}
	if entityID.Valid {
		item.EntityID = &entityID.String
	}
	if cameraID.Valid {
		item.CameraID = &cameraID.String
	}
	if cameraName.Valid {
		item.CameraName = &cameraName.String
	}
	if errText.Valid {
		cleaned := sanitizeAlertString(errText.String)
		item.Error = &cleaned
	}
	if sentAt.Valid {
		item.SentAt = &sentAt.Time
	}
	return item, nil
}

func normalizeMessageTemplate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Motion detected on {{camera_name}}\nTime: {{event_time}}\nScore: {{score}}"
	}
	return value
}

func validateNotificationChannelConfig(method string, config map[string]any) error {
	if method != "telegram" {
		return errValidation("unsupported notification method")
	}
	if strings.TrimSpace(stringMapValue(config, "bot_token")) == "" {
		return errValidation("config.bot_token is required")
	}
	if strings.TrimSpace(stringMapValue(config, "chat_id")) == "" {
		return errValidation("config.chat_id is required")
	}
	return nil
}

func redactNotificationChannel(item notificationChannelResponse) notificationChannelResponse {
	config := rawJSONToMap(item.Config)
	if _, ok := config["bot_token"]; ok {
		config["bot_token"] = "[redacted]"
	}
	payload, _ := json.Marshal(config)
	item.Config = payload
	return item
}

func rawJSONToMap(raw json.RawMessage) map[string]any {
	out := map[string]any{}
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func stringMapValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	switch value := values[key].(type) {
	case string:
		return value
	default:
		return ""
	}
}

func boolPtrValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func intPtrValue(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func (s *Server) ensureNotificationChannelExists(r *http.Request, id string) error {
	var exists bool
	if err := s.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM notification_channels WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errValidation("notification_channel_id does not exist")
	}
	return nil
}
