package httpapi

import (
	"database/sql"
	"net/http"
	"strings"
	"time"
)

type cameraResponse struct {
	ID                string    `json:"id"`
	StorageLocationID *string   `json:"storage_location_id,omitempty"`
	Name              string    `json:"name"`
	Location          *string   `json:"location,omitempty"`
	CameraGroup       *string   `json:"camera_group,omitempty"`
	Enabled           bool      `json:"enabled"`
	RecordingEnabled  bool      `json:"recording_enabled"`
	RetentionDays     int       `json:"retention_days"`
	MaxStorageBytes   *int64    `json:"max_storage_bytes,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type createCameraRequest struct {
	Name              string  `json:"name"`
	RTSPURL           string  `json:"rtsp_url"`
	StorageLocationID *string `json:"storage_location_id,omitempty"`
	Location          *string `json:"location,omitempty"`
	CameraGroup       *string `json:"camera_group,omitempty"`
	Enabled           *bool   `json:"enabled,omitempty"`
	RecordingEnabled  *bool   `json:"recording_enabled,omitempty"`
	RetentionDays     *int    `json:"retention_days,omitempty"`
	MaxStorageBytes   *int64  `json:"max_storage_bytes,omitempty"`
}

type updateCameraRequest struct {
	Name              *string `json:"name,omitempty"`
	RTSPURL           *string `json:"rtsp_url,omitempty"`
	StorageLocationID *string `json:"storage_location_id,omitempty"`
	Location          *string `json:"location,omitempty"`
	CameraGroup       *string `json:"camera_group,omitempty"`
	Enabled           *bool   `json:"enabled,omitempty"`
	RecordingEnabled  *bool   `json:"recording_enabled,omitempty"`
	RetentionDays     *int    `json:"retention_days,omitempty"`
	MaxStorageBytes   *int64  `json:"max_storage_bytes,omitempty"`
}

func (s *Server) handleCameras(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listCameras(w, r)
	case http.MethodPost:
		s.createCamera(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleCameraByID(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	id, action, ok := splitIDAction(r.URL.Path, "/api/cameras/")
	if !ok || !isUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "camera not found", nil)
		return
	}

	if action == "enabled" {
		if r.Method != http.MethodPatch {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		s.setCameraEnabled(w, r, id)
		return
	}

	if action == "preview" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		s.previewCamera(w, r, id)
		return
	}

	if action != "" {
		writeError(w, http.StatusNotFound, "not_found", "camera endpoint not found", nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getCamera(w, r, id)
	case http.MethodPatch:
		s.updateCamera(w, r, id)
	case http.MethodDelete:
		s.deleteCamera(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) createCamera(w http.ResponseWriter, r *http.Request) {
	var req createCameraRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}

	name := strings.TrimSpace(req.Name)
	rtspURL := strings.TrimSpace(req.RTSPURL)
	if name == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "name is required", nil)
		return
	}
	if rtspURL == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "rtsp_url is required", nil)
		return
	}
	if !strings.HasPrefix(strings.ToLower(rtspURL), "rtsp://") {
		writeError(w, http.StatusBadRequest, "validation_error", "rtsp_url must start with rtsp://", nil)
		return
	}

	if err := s.validateStorageLocationForCamera(r, req.StorageLocationID); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	recordingEnabled := false
	if req.RecordingEnabled != nil {
		recordingEnabled = *req.RecordingEnabled
	}
	retentionDays := 30
	if req.RetentionDays != nil {
		retentionDays = *req.RetentionDays
	}
	if retentionDays <= 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "retention_days must be greater than zero", nil)
		return
	}
	if req.MaxStorageBytes != nil && *req.MaxStorageBytes <= 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "max_storage_bytes must be greater than zero", nil)
		return
	}

	row := s.db.QueryRowContext(r.Context(), `
		INSERT INTO cameras (name, rtsp_url, storage_location_id, location, camera_group, is_enabled, recording_enabled, retention_days, max_storage_bytes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, storage_location_id, name, location, camera_group, is_enabled, recording_enabled, retention_days, max_storage_bytes, created_at, updated_at
	`, name, rtspURL, nullableString(req.StorageLocationID), nullableCleanString(req.Location), nullableCleanString(req.CameraGroup), enabled, recordingEnabled, retentionDays, nullableInt64(req.MaxStorageBytes))

	camera, err := scanCamera(row)
	if err != nil {
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "camera.create", EntityType: "camera", EntityID: &camera.ID, Message: "camera created"})
	writeJSON(w, http.StatusCreated, camera)
}

func (s *Server) listCameras(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, storage_location_id, name, location, camera_group, is_enabled, recording_enabled, retention_days, max_storage_bytes, created_at, updated_at
		FROM cameras
		ORDER BY name
	`)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer rows.Close()

	cameras := []cameraResponse{}
	for rows.Next() {
		camera, err := scanCamera(rows)
		if err != nil {
			writeDBError(w, err)
			return
		}
		cameras = append(cameras, camera)
	}
	if err := rows.Err(); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cameras": cameras})
}

func (s *Server) getCamera(w http.ResponseWriter, r *http.Request, id string) {
	camera, err := s.findCamera(r, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "camera.update", EntityType: "camera", EntityID: &camera.ID, Message: "camera updated"})
	writeJSON(w, http.StatusOK, camera)
}

func (s *Server) previewCamera(w http.ResponseWriter, r *http.Request, id string) {
	var rtspURL string
	var enabled bool
	err := s.db.QueryRowContext(r.Context(), `
		SELECT rtsp_url, is_enabled
		FROM cameras
		WHERE id = $1
	`, id).Scan(&rtspURL, &enabled)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "not_found", "camera not found", nil)
			return
		}
		writeDBError(w, err)
		return
	}
	if !enabled {
		if cached, ok := s.readCameraPreviewCache(id); ok {
			w.Header().Set("X-Preview-Source", "cache")
			writeJPEG(w, cached)
			return
		}
		writeError(w, http.StatusBadRequest, "camera_disabled", "camera is disabled", nil)
		return
	}
	image, err := captureRTSPPreview(r.Context(), rtspURL)
	if err != nil {
		if cached, ok := s.readCameraPreviewCache(id); ok {
			w.Header().Set("X-Preview-Source", "cache")
			writeJPEG(w, cached)
			return
		}
		writeError(w, http.StatusBadRequest, "camera_preview_failed", "could not capture camera preview image", nil)
		return
	}
	s.saveCameraPreviewCache(id, image)
	w.Header().Set("X-Preview-Source", "live")
	writeJPEG(w, image)
}

func (s *Server) updateCamera(w http.ResponseWriter, r *http.Request, id string) {
	var req updateCameraRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}

	current, err := s.findCameraInternal(r, id)
	if err != nil {
		writeDBError(w, err)
		return
	}

	name := current.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "name is required", nil)
		return
	}

	rtspURL := current.RTSPURL
	if req.RTSPURL != nil {
		rtspURL = strings.TrimSpace(*req.RTSPURL)
	}
	if rtspURL == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "rtsp_url is required", nil)
		return
	}
	if !strings.HasPrefix(strings.ToLower(rtspURL), "rtsp://") {
		writeError(w, http.StatusBadRequest, "validation_error", "rtsp_url must start with rtsp://", nil)
		return
	}

	storageLocationID := current.StorageLocationID
	if req.StorageLocationID != nil {
		value := strings.TrimSpace(*req.StorageLocationID)
		storageLocationID = nil
		if value != "" {
			storageLocationID = &value
		}
	}
	if err := s.validateStorageLocationForCamera(r, storageLocationID); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}

	location := current.Location
	if req.Location != nil {
		location = cleanedOptionalString(req.Location)
	}
	cameraGroup := current.CameraGroup
	if req.CameraGroup != nil {
		cameraGroup = cleanedOptionalString(req.CameraGroup)
	}
	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	recordingEnabled := current.RecordingEnabled
	if req.RecordingEnabled != nil {
		recordingEnabled = *req.RecordingEnabled
	}
	retentionDays := current.RetentionDays
	if req.RetentionDays != nil {
		retentionDays = *req.RetentionDays
	}
	if retentionDays <= 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "retention_days must be greater than zero", nil)
		return
	}
	maxStorageBytes := current.MaxStorageBytes
	if req.MaxStorageBytes != nil {
		if *req.MaxStorageBytes <= 0 {
			writeError(w, http.StatusBadRequest, "validation_error", "max_storage_bytes must be greater than zero", nil)
			return
		}
		value := *req.MaxStorageBytes
		maxStorageBytes = &value
	}

	row := s.db.QueryRowContext(r.Context(), `
		UPDATE cameras
		SET name = $2, rtsp_url = $3, storage_location_id = $4, location = $5, camera_group = $6,
			is_enabled = $7, recording_enabled = $8, retention_days = $9, max_storage_bytes = $10
		WHERE id = $1
		RETURNING id, storage_location_id, name, location, camera_group, is_enabled, recording_enabled, retention_days, max_storage_bytes, created_at, updated_at
	`, id, name, rtspURL, nullableString(storageLocationID), nullableString(location), nullableString(cameraGroup), enabled, recordingEnabled, retentionDays, nullableInt64(maxStorageBytes))

	camera, err := scanCamera(row)
	if err != nil {
		writeDBError(w, err)
		return
	}
	eventType := "camera.disable"
	if camera.Enabled {
		eventType = "camera.enable"
	}
	s.recordEvent(r, eventRecord{EventType: eventType, EntityType: "camera", EntityID: &camera.ID, Message: "camera enabled state changed"})
	writeJSON(w, http.StatusOK, camera)
}

func (s *Server) setCameraEnabled(w http.ResponseWriter, r *http.Request, id string) {
	var req setEnabledRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}

	row := s.db.QueryRowContext(r.Context(), `
		UPDATE cameras
		SET is_enabled = $2
		WHERE id = $1
		RETURNING id, storage_location_id, name, location, camera_group, is_enabled, recording_enabled, retention_days, max_storage_bytes, created_at, updated_at
	`, id, req.Enabled)

	camera, err := scanCamera(row)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, camera)
}

func (s *Server) deleteCamera(w http.ResponseWriter, r *http.Request, id string) {
	var segmentCount int
	if err := s.db.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM recording_segments WHERE camera_id = $1
	`, id).Scan(&segmentCount); err != nil {
		writeDBError(w, err)
		return
	}

	if segmentCount > 0 {
		row := s.db.QueryRowContext(r.Context(), `
			UPDATE cameras
			SET is_enabled = FALSE, recording_enabled = FALSE
			WHERE id = $1
			RETURNING id, storage_location_id, name, location, camera_group, is_enabled, recording_enabled, retention_days, max_storage_bytes, created_at, updated_at
		`, id)
		camera, err := scanCamera(row)
		if err != nil {
			writeDBError(w, err)
			return
		}
		s.recordEvent(r, eventRecord{EventType: "camera.disable", EntityType: "camera", EntityID: &camera.ID, Message: "camera disabled because recordings exist"})
		writeJSON(w, http.StatusOK, map[string]any{
			"deleted": false,
			"reason":  "camera has recording segments, so it was disabled instead of deleted",
			"camera":  camera,
		})
		return
	}

	result, err := s.db.ExecContext(r.Context(), `DELETE FROM cameras WHERE id = $1`, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeDBError(w, sql.ErrNoRows)
		return
	}

	s.recordEvent(r, eventRecord{EventType: "camera.delete", EntityType: "camera", EntityID: &id, Message: "camera deleted"})
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) validateStorageLocationForCamera(r *http.Request, storageLocationID *string) error {
	if storageLocationID == nil {
		return nil
	}
	if !isUUID(*storageLocationID) {
		return errValidation("storage_location_id must be a UUID")
	}

	var enabled bool
	if err := s.db.QueryRowContext(r.Context(), `
		SELECT is_enabled FROM storage_locations WHERE id = $1
	`, *storageLocationID).Scan(&enabled); err != nil {
		if err == sql.ErrNoRows {
			return errValidation("storage_location_id does not exist")
		}
		return err
	}
	if !enabled {
		return errValidation("storage_location_id is disabled")
	}
	return nil
}

func (s *Server) findCamera(r *http.Request, id string) (cameraResponse, error) {
	row := s.db.QueryRowContext(r.Context(), `
		SELECT id, storage_location_id, name, location, camera_group, is_enabled, recording_enabled, retention_days, max_storage_bytes, created_at, updated_at
		FROM cameras
		WHERE id = $1
	`, id)
	return scanCamera(row)
}

type cameraInternal struct {
	cameraResponse
	RTSPURL string
}

func (s *Server) findCameraInternal(r *http.Request, id string) (cameraInternal, error) {
	row := s.db.QueryRowContext(r.Context(), `
		SELECT id, storage_location_id, name, location, camera_group, is_enabled, recording_enabled, retention_days, max_storage_bytes, created_at, updated_at, rtsp_url
		FROM cameras
		WHERE id = $1
	`, id)
	return scanCameraInternal(row)
}

type cameraScanner interface {
	Scan(dest ...any) error
}

func scanCamera(scanner cameraScanner) (cameraResponse, error) {
	var camera cameraResponse
	var storageLocationID sql.NullString
	var location sql.NullString
	var cameraGroup sql.NullString
	var maxStorageBytes sql.NullInt64
	if err := scanner.Scan(
		&camera.ID,
		&storageLocationID,
		&camera.Name,
		&location,
		&cameraGroup,
		&camera.Enabled,
		&camera.RecordingEnabled,
		&camera.RetentionDays,
		&maxStorageBytes,
		&camera.CreatedAt,
		&camera.UpdatedAt,
	); err != nil {
		return cameraResponse{}, err
	}
	if storageLocationID.Valid {
		camera.StorageLocationID = &storageLocationID.String
	}
	if location.Valid {
		camera.Location = &location.String
	}
	if cameraGroup.Valid {
		camera.CameraGroup = &cameraGroup.String
	}
	if maxStorageBytes.Valid {
		camera.MaxStorageBytes = &maxStorageBytes.Int64
	}
	return camera, nil
}

func scanCameraInternal(scanner cameraScanner) (cameraInternal, error) {
	var camera cameraInternal
	var storageLocationID sql.NullString
	var location sql.NullString
	var cameraGroup sql.NullString
	var maxStorageBytes sql.NullInt64
	if err := scanner.Scan(
		&camera.ID,
		&storageLocationID,
		&camera.Name,
		&location,
		&cameraGroup,
		&camera.Enabled,
		&camera.RecordingEnabled,
		&camera.RetentionDays,
		&maxStorageBytes,
		&camera.CreatedAt,
		&camera.UpdatedAt,
		&camera.RTSPURL,
	); err != nil {
		return cameraInternal{}, err
	}
	if storageLocationID.Valid {
		camera.StorageLocationID = &storageLocationID.String
	}
	if location.Valid {
		camera.Location = &location.String
	}
	if cameraGroup.Valid {
		camera.CameraGroup = &cameraGroup.String
	}
	if maxStorageBytes.Valid {
		camera.MaxStorageBytes = &maxStorageBytes.Int64
	}
	return camera, nil
}

type validationError string

func (e validationError) Error() string {
	return string(e)
}

func errValidation(message string) error {
	return validationError(message)
}

func cleanedOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	cleaned := strings.TrimSpace(*value)
	if cleaned == "" {
		return nil
	}
	return &cleaned
}

func nullableCleanString(value *string) any {
	return nullableString(cleanedOptionalString(value))
}

func nullableString(value *string) any {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return strings.TrimSpace(*value)
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}
