package httpapi

import (
	"database/sql"
	"net/http"
	"strings"
	"time"
)

type storageLocationResponse struct {
	ID                    string     `json:"id"`
	Name                  string     `json:"name"`
	ContainerPath         string     `json:"container_path"`
	Enabled               bool       `json:"enabled"`
	HealthStatus          string     `json:"health_status"`
	Exists                bool       `json:"exists"`
	Writable              bool       `json:"writable"`
	TotalBytes            int64      `json:"total_bytes"`
	FreeBytes             int64      `json:"free_bytes"`
	UsedBytes             int64      `json:"used_bytes"`
	UsedPercent           float64    `json:"used_percent"`
	LatestValidationError *string    `json:"latest_validation_error,omitempty"`
	LastCheckedAt         *time.Time `json:"last_checked_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type createStorageLocationRequest struct {
	Name          string `json:"name"`
	ContainerPath string `json:"container_path"`
	Enabled       *bool  `json:"enabled,omitempty"`
}

type updateStorageLocationRequest struct {
	Name          *string `json:"name,omitempty"`
	ContainerPath *string `json:"container_path,omitempty"`
	Enabled       *bool   `json:"enabled,omitempty"`
}

type setEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

func (s *Server) handleStorageLocations(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listStorageLocations(w, r)
	case http.MethodPost:
		s.createStorageLocation(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleStorageLocationByID(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	id, action, ok := splitIDAction(r.URL.Path, "/api/storage-locations/")
	if !ok || !isUUID(id) {
		writeError(w, http.StatusNotFound, "not_found", "storage location not found", nil)
		return
	}

	if action == "enabled" {
		if r.Method != http.MethodPatch {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		s.setStorageLocationEnabled(w, r, id)
		return
	}

	if action == "health" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		s.getStorageLocationHealth(w, r, id)
		return
	}

	if action != "" {
		writeError(w, http.StatusNotFound, "not_found", "storage location endpoint not found", nil)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getStorageLocation(w, r, id)
	case http.MethodPatch:
		s.updateStorageLocation(w, r, id)
	case http.MethodDelete:
		s.deleteStorageLocation(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) createStorageLocation(w http.ResponseWriter, r *http.Request) {
	var req createStorageLocationRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}

	name := strings.TrimSpace(req.Name)
	containerPath := cleanStoragePath(strings.TrimSpace(req.ContainerPath))
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if name == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "name is required", nil)
		return
	}

	validation := validateStoragePath(containerPath)
	if !validation.Valid {
		writeError(w, http.StatusBadRequest, "storage_validation_failed", validation.Message, validation)
		return
	}

	row := s.db.QueryRowContext(r.Context(), `
		INSERT INTO storage_locations (
			name, container_path, is_enabled, health_status, exists, writable,
			total_bytes, free_bytes, used_bytes, used_percent, latest_validation_error, last_checked_at
		)
		VALUES ($1, $2, $3, 'ok', $4, $5, $6, $7, $8, $9, NULL, now())
		RETURNING id, name, container_path, is_enabled, health_status, exists, writable,
			total_bytes, free_bytes, used_bytes, used_percent, latest_validation_error,
			last_checked_at, created_at, updated_at
	`, name, containerPath, enabled, validation.Exists, validation.Writable, int64(validation.TotalBytes), int64(validation.FreeBytes), int64(validation.UsedBytes), validation.UsedPercent)

	location, err := scanStorageLocation(row)
	if err != nil {
		writeDBError(w, err)
		return
	}

	s.recordEvent(r, eventRecord{EventType: "storage.create", EntityType: "storage", EntityID: &location.ID, Message: "storage location created"})
	writeJSON(w, http.StatusCreated, location)
}

func (s *Server) listStorageLocations(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, name, container_path, is_enabled, health_status, exists, writable,
			total_bytes, free_bytes, used_bytes, used_percent, latest_validation_error,
			last_checked_at, created_at, updated_at
		FROM storage_locations
		ORDER BY name
	`)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer rows.Close()

	locations := []storageLocationResponse{}
	for rows.Next() {
		location, err := scanStorageLocation(rows)
		if err != nil {
			writeDBError(w, err)
			return
		}
		locations = append(locations, location)
	}
	if err := rows.Err(); err != nil {
		writeDBError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"storage_locations": locations})
}

func (s *Server) getStorageLocation(w http.ResponseWriter, r *http.Request, id string) {
	location, err := s.findStorageLocation(r, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, location)
}

func (s *Server) updateStorageLocation(w http.ResponseWriter, r *http.Request, id string) {
	var req updateStorageLocationRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}

	current, err := s.findStorageLocation(r, id)
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

	containerPath := current.ContainerPath
	shouldValidate := false
	if req.ContainerPath != nil {
		containerPath = cleanStoragePath(strings.TrimSpace(*req.ContainerPath))
		shouldValidate = true
	}

	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
		if enabled {
			shouldValidate = true
		}
	}

	healthStatus := current.HealthStatus
	lastCheckedAt := current.LastCheckedAt
	exists := current.Exists
	writable := current.Writable
	totalBytes := current.TotalBytes
	freeBytes := current.FreeBytes
	usedBytes := current.UsedBytes
	usedPercent := current.UsedPercent
	if shouldValidate {
		validation := validateStoragePath(containerPath)
		now := time.Now().UTC()
		lastCheckedAt = &now
		exists = validation.Exists
		writable = validation.Writable
		totalBytes = int64(validation.TotalBytes)
		freeBytes = int64(validation.FreeBytes)
		usedBytes = int64(validation.UsedBytes)
		usedPercent = validation.UsedPercent
		if !validation.Valid {
			healthStatus = "unhealthy"
			_, _ = s.updateStorageHealth(r, id, containerPath, validation, healthStatus, lastCheckedAt)
			writeError(w, http.StatusBadRequest, "storage_validation_failed", validation.Message, validation)
			return
		}
		healthStatus = "ok"
	}

	row := s.db.QueryRowContext(r.Context(), `
		UPDATE storage_locations
		SET name = $2, container_path = $3, is_enabled = $4, health_status = $5,
			exists = $6, writable = $7, total_bytes = $8, free_bytes = $9,
			used_bytes = $10, used_percent = $11, latest_validation_error = NULL,
			last_checked_at = $12
		WHERE id = $1
		RETURNING id, name, container_path, is_enabled, health_status, exists, writable,
			total_bytes, free_bytes, used_bytes, used_percent, latest_validation_error,
			last_checked_at, created_at, updated_at
	`, id, name, containerPath, enabled, healthStatus, exists, writable, totalBytes, freeBytes, usedBytes, usedPercent, lastCheckedAt)

	location, err := scanStorageLocation(row)
	if err != nil {
		writeDBError(w, err)
		return
	}

	s.recordEvent(r, eventRecord{EventType: "storage.update", EntityType: "storage", EntityID: &location.ID, Message: "storage location updated"})
	writeJSON(w, http.StatusOK, location)
}

func (s *Server) setStorageLocationEnabled(w http.ResponseWriter, r *http.Request, id string) {
	var req setEnabledRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}

	current, err := s.findStorageLocation(r, id)
	if err != nil {
		writeDBError(w, err)
		return
	}

	healthStatus := current.HealthStatus
	lastCheckedAt := current.LastCheckedAt
	exists := current.Exists
	writable := current.Writable
	totalBytes := current.TotalBytes
	freeBytes := current.FreeBytes
	usedBytes := current.UsedBytes
	usedPercent := current.UsedPercent
	if req.Enabled {
		validation := validateStoragePath(current.ContainerPath)
		now := time.Now().UTC()
		lastCheckedAt = &now
		exists = validation.Exists
		writable = validation.Writable
		totalBytes = int64(validation.TotalBytes)
		freeBytes = int64(validation.FreeBytes)
		usedBytes = int64(validation.UsedBytes)
		usedPercent = validation.UsedPercent
		if !validation.Valid {
			_, _ = s.db.ExecContext(r.Context(), `
				UPDATE storage_locations
				SET health_status = 'unhealthy', exists = $2, writable = $3,
					total_bytes = $4, free_bytes = $5, used_bytes = $6,
					used_percent = $7, latest_validation_error = $8,
					last_checked_at = $9
				WHERE id = $1
			`, id, validation.Exists, validation.Writable, int64(validation.TotalBytes), int64(validation.FreeBytes), int64(validation.UsedBytes), validation.UsedPercent, validation.Message, now)
			writeError(w, http.StatusBadRequest, "storage_validation_failed", validation.Message, validation)
			return
		}
		healthStatus = "ok"
	}

	row := s.db.QueryRowContext(r.Context(), `
		UPDATE storage_locations
		SET is_enabled = $2, health_status = $3, exists = $4, writable = $5,
			total_bytes = $6, free_bytes = $7, used_bytes = $8, used_percent = $9,
			latest_validation_error = NULL, last_checked_at = $10
		WHERE id = $1
		RETURNING id, name, container_path, is_enabled, health_status, exists, writable,
			total_bytes, free_bytes, used_bytes, used_percent, latest_validation_error,
			last_checked_at, created_at, updated_at
	`, id, req.Enabled, healthStatus, exists, writable, totalBytes, freeBytes, usedBytes, usedPercent, lastCheckedAt)

	location, err := scanStorageLocation(row)
	if err != nil {
		writeDBError(w, err)
		return
	}

	eventType := "storage.disable"
	if location.Enabled {
		eventType = "storage.enable"
	}
	s.recordEvent(r, eventRecord{EventType: eventType, EntityType: "storage", EntityID: &location.ID, Message: "storage location enabled state changed"})
	writeJSON(w, http.StatusOK, location)
}

func (s *Server) deleteStorageLocation(w http.ResponseWriter, r *http.Request, id string) {
	var cameraCount int
	if err := s.db.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM cameras WHERE storage_location_id = $1
	`, id).Scan(&cameraCount); err != nil {
		writeDBError(w, err)
		return
	}

	var segmentCount int
	if err := s.db.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM recording_segments WHERE storage_location_id = $1
	`, id).Scan(&segmentCount); err != nil {
		writeDBError(w, err)
		return
	}

	if cameraCount > 0 || segmentCount > 0 {
		writeError(w, http.StatusConflict, "storage_location_in_use", "storage location cannot be deleted while cameras or recordings depend on it", map[string]int{
			"cameras":            cameraCount,
			"recording_segments": segmentCount,
		})
		return
	}

	result, err := s.db.ExecContext(r.Context(), `DELETE FROM storage_locations WHERE id = $1`, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeDBError(w, sql.ErrNoRows)
		return
	}

	s.recordEvent(r, eventRecord{EventType: "storage.delete", EntityType: "storage", EntityID: &id, Message: "storage location deleted"})
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) findStorageLocation(r *http.Request, id string) (storageLocationResponse, error) {
	row := s.db.QueryRowContext(r.Context(), `
		SELECT id, name, container_path, is_enabled, health_status, exists, writable,
			total_bytes, free_bytes, used_bytes, used_percent, latest_validation_error,
			last_checked_at, created_at, updated_at
		FROM storage_locations
		WHERE id = $1
	`, id)
	return scanStorageLocation(row)
}

func (s *Server) getStorageLocationHealth(w http.ResponseWriter, r *http.Request, id string) {
	current, err := s.findStorageLocation(r, id)
	if err != nil {
		writeDBError(w, err)
		return
	}

	validation := validateStoragePath(current.ContainerPath)
	status := "ok"
	if !validation.Valid {
		status = "unhealthy"
	}
	now := time.Now().UTC()
	location, err := s.updateStorageHealth(r, id, current.ContainerPath, validation, status, &now)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, location)
}

func (s *Server) updateStorageHealth(r *http.Request, id, containerPath string, validation storageValidationResult, status string, checkedAt *time.Time) (storageLocationResponse, error) {
	row := s.db.QueryRowContext(r.Context(), `
		UPDATE storage_locations
		SET health_status = $2, exists = $3, writable = $4, total_bytes = $5,
			free_bytes = $6, used_bytes = $7, used_percent = $8,
			latest_validation_error = NULLIF($9, ''), last_checked_at = $10
		WHERE id = $1
		RETURNING id, name, container_path, is_enabled, health_status, exists, writable,
			total_bytes, free_bytes, used_bytes, used_percent, latest_validation_error,
			last_checked_at, created_at, updated_at
	`, id, status, validation.Exists, validation.Writable, int64(validation.TotalBytes), int64(validation.FreeBytes), int64(validation.UsedBytes), validation.UsedPercent, validation.LatestValidationError, checkedAt)
	return scanStorageLocation(row)
}

type storageLocationScanner interface {
	Scan(dest ...any) error
}

func scanStorageLocation(scanner storageLocationScanner) (storageLocationResponse, error) {
	var location storageLocationResponse
	var lastCheckedAt sql.NullTime
	var latestValidationError sql.NullString
	if err := scanner.Scan(
		&location.ID,
		&location.Name,
		&location.ContainerPath,
		&location.Enabled,
		&location.HealthStatus,
		&location.Exists,
		&location.Writable,
		&location.TotalBytes,
		&location.FreeBytes,
		&location.UsedBytes,
		&location.UsedPercent,
		&latestValidationError,
		&lastCheckedAt,
		&location.CreatedAt,
		&location.UpdatedAt,
	); err != nil {
		return storageLocationResponse{}, err
	}
	if lastCheckedAt.Valid {
		location.LastCheckedAt = &lastCheckedAt.Time
	}
	if latestValidationError.Valid {
		location.LatestValidationError = &latestValidationError.String
	}
	return location, nil
}
