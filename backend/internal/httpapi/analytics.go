package httpapi

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

type identityResponse struct {
	ID                  string     `json:"id"`
	DisplayName         string     `json:"display_name"`
	Type                string     `json:"type"`
	Known               bool       `json:"known"`
	Notes               *string    `json:"notes,omitempty"`
	ReferenceImageCount int        `json:"reference_image_count"`
	LastSeenAt          *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type identityRequest struct {
	DisplayName string  `json:"display_name"`
	Type        string  `json:"type,omitempty"`
	Known       *bool   `json:"known,omitempty"`
	Notes       *string `json:"notes,omitempty"`
}

type referenceImageResponse struct {
	ID            string          `json:"id"`
	IdentityID    string          `json:"identity_id"`
	ImageURL      string          `json:"image_url"`
	FaceCropPath  *string         `json:"face_crop_path,omitempty"`
	EmbeddingJSON json.RawMessage `json:"embedding_json,omitempty"`
	QualityScore  *float64        `json:"quality_score,omitempty"`
	Status        string          `json:"status"`
	CreatedAt     time.Time       `json:"created_at"`
}

type referenceImageRequest struct {
	ImagePath     string          `json:"image_path"`
	FaceCropPath  *string         `json:"face_crop_path,omitempty"`
	EmbeddingJSON json.RawMessage `json:"embedding_json,omitempty"`
	QualityScore  *float64        `json:"quality_score,omitempty"`
	Status        string          `json:"status,omitempty"`
}

type observationResponse struct {
	ID              string          `json:"id"`
	CameraID        string          `json:"camera_id"`
	EventID         *string         `json:"event_id,omitempty"`
	ObservedAt      time.Time       `json:"observed_at"`
	ObservationType string          `json:"observation_type"`
	BBoxJSON        json.RawMessage `json:"bbox_json,omitempty"`
	Confidence      *float64        `json:"confidence,omitempty"`
	FramePath       *string         `json:"frame_path,omitempty"`
	CropPath        *string         `json:"crop_path,omitempty"`
	AttributesJSON  json.RawMessage `json:"attributes_json"`
	EmbeddingJSON   json.RawMessage `json:"embedding_json,omitempty"`
	TrackID         *string         `json:"track_id,omitempty"`
	IdentityID      *string         `json:"identity_id,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
}

type observationRequest struct {
	CameraID        string          `json:"camera_id"`
	EventID         *string         `json:"event_id,omitempty"`
	ObservedAt      time.Time       `json:"observed_at"`
	ObservationType string          `json:"observation_type"`
	BBoxJSON        json.RawMessage `json:"bbox_json,omitempty"`
	Confidence      *float64        `json:"confidence,omitempty"`
	FramePath       *string         `json:"frame_path,omitempty"`
	CropPath        *string         `json:"crop_path,omitempty"`
	AttributesJSON  json.RawMessage `json:"attributes_json,omitempty"`
	EmbeddingJSON   json.RawMessage `json:"embedding_json,omitempty"`
	TrackID         *string         `json:"track_id,omitempty"`
	IdentityID      *string         `json:"identity_id,omitempty"`
}

type identityMatchAttemptResponse struct {
	ID              string    `json:"id"`
	ObservationID   string    `json:"observation_id"`
	IdentityID      *string   `json:"identity_id,omitempty"`
	Method          string    `json:"method"`
	SimilarityScore *float64  `json:"similarity_score,omitempty"`
	Threshold       *float64  `json:"threshold,omitempty"`
	Decision        string    `json:"decision"`
	CreatedAt       time.Time `json:"created_at"`
}

type identityMatchAttemptRequest struct {
	ObservationID   string   `json:"observation_id"`
	IdentityID      *string  `json:"identity_id,omitempty"`
	Method          string   `json:"method"`
	SimilarityScore *float64 `json:"similarity_score,omitempty"`
	Threshold       *float64 `json:"threshold,omitempty"`
	Decision        string   `json:"decision"`
}

type aiJobResponse struct {
	ID            string          `json:"id"`
	CameraID      string          `json:"camera_id"`
	SourceEventID *string         `json:"source_event_id,omitempty"`
	JobType       string          `json:"job_type"`
	Status        string          `json:"status"`
	Priority      int             `json:"priority"`
	FramePath     *string         `json:"frame_path,omitempty"`
	MetadataJSON  json.RawMessage `json:"metadata_json"`
	Attempts      int             `json:"attempts"`
	ErrorMessage  *string         `json:"error_message,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	StartedAt     *time.Time      `json:"started_at,omitempty"`
	CompletedAt   *time.Time      `json:"completed_at,omitempty"`
}

type aiJobRequest struct {
	CameraID      string          `json:"camera_id"`
	SourceEventID *string         `json:"source_event_id,omitempty"`
	JobType       string          `json:"job_type"`
	Status        string          `json:"status,omitempty"`
	Priority      *int            `json:"priority,omitempty"`
	FramePath     *string         `json:"frame_path,omitempty"`
	MetadataJSON  json.RawMessage `json:"metadata_json,omitempty"`
}

type aiJobPatchRequest struct {
	Status       *string `json:"status,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
}

func (s *Server) handleIdentities(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listIdentities(w, r)
	case http.MethodPost:
		s.createIdentity(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleIdentityByID(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	parts, ok := splitPathParts(r.URL.Path, "/api/identities/")
	if !ok || len(parts) == 0 || !isUUID(parts[0]) {
		writeError(w, http.StatusNotFound, "not_found", "identity not found", nil)
		return
	}
	id := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			s.getIdentity(w, r, id)
		case http.MethodPatch, http.MethodPut:
			s.updateIdentity(w, r, id)
		case http.MethodDelete:
			s.deleteIdentity(w, r, id)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		}
		return
	}
	if len(parts) == 2 && parts[1] == "reference-images" {
		switch r.Method {
		case http.MethodGet:
			s.listReferenceImages(w, r, id)
		case http.MethodPost:
			s.createReferenceImage(w, r, id)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		}
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "identity endpoint not found", nil)
}

func (s *Server) handleReferenceImageByID(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	id, action, ok := splitIDAction(r.URL.Path, "/api/identity-reference-images/")
	if !ok || !isUUID(id) || action != "" {
		if ok && isUUID(id) && action == "file" && r.Method == http.MethodGet {
			s.serveReferenceImage(w, r, id)
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "reference image not found", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.serveReferenceImage(w, r, id)
	case http.MethodDelete:
		s.deleteReferenceImage(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleObservations(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listObservations(w, r, user)
	case http.MethodPost:
		if user.Role != "admin" {
			writeError(w, http.StatusForbidden, "forbidden", "admin role required", nil)
			return
		}
		s.createObservation(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleIdentityMatchAttempts(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listIdentityMatchAttempts(w, r)
	case http.MethodPost:
		s.createIdentityMatchAttempt(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleAIJobs(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listAIJobs(w, r)
	case http.MethodPost:
		s.createAIJob(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleAIJobByID(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	id, action, ok := splitIDAction(r.URL.Path, "/api/ai-jobs/")
	if !ok || !isUUID(id) || action != "" {
		writeError(w, http.StatusNotFound, "not_found", "ai job not found", nil)
		return
	}
	if r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	s.updateAIJob(w, r, id)
}

func (s *Server) listIdentities(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT i.id, i.display_name, i.type, i.known, i.notes,
			COUNT(DISTINCT iri.id) AS reference_image_count,
			MAX(o.observed_at) AS last_seen_at,
			i.created_at, i.updated_at
		FROM identities i
		LEFT JOIN identity_reference_images iri ON iri.identity_id = i.id
		LEFT JOIN observations o ON o.identity_id = i.id
		GROUP BY i.id
		ORDER BY i.known DESC, i.display_name
	`)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer rows.Close()
	items := []identityResponse{}
	for rows.Next() {
		item, err := scanIdentity(rows)
		if err != nil {
			writeDBError(w, err)
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"identities": items})
}

func (s *Server) createIdentity(w http.ResponseWriter, r *http.Request) {
	req, ok := readIdentityRequest(w, r)
	if !ok {
		return
	}
	row := s.db.QueryRowContext(r.Context(), `
		INSERT INTO identities (display_name, type, known, notes)
		VALUES ($1, $2, $3, $4)
		RETURNING id, display_name, type, known, notes, 0, NULL::timestamptz, created_at, updated_at
	`, req.DisplayName, req.Type, boolPtrValue(req.Known, true), nullableCleanString(req.Notes))
	item, err := scanIdentity(row)
	if err != nil {
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "identity.create", EntityType: "identity", EntityID: &item.ID, Message: "identity created"})
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) getIdentity(w http.ResponseWriter, r *http.Request, id string) {
	item, err := s.findIdentity(r, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) updateIdentity(w http.ResponseWriter, r *http.Request, id string) {
	current, err := s.findIdentity(r, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	req, ok := readIdentityRequest(w, r)
	if !ok {
		return
	}
	known := current.Known
	if req.Known != nil {
		known = *req.Known
	}
	row := s.db.QueryRowContext(r.Context(), `
		UPDATE identities
		SET display_name = $2, type = $3, known = $4, notes = $5
		WHERE id = $1
		RETURNING id, display_name, type, known, notes,
			(SELECT COUNT(*) FROM identity_reference_images WHERE identity_id = identities.id),
			(SELECT MAX(observed_at) FROM observations WHERE identity_id = identities.id),
			created_at, updated_at
	`, id, req.DisplayName, req.Type, known, nullableCleanString(req.Notes))
	item, err := scanIdentity(row)
	if err != nil {
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "identity.update", EntityType: "identity", EntityID: &item.ID, Message: "identity updated"})
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) deleteIdentity(w http.ResponseWriter, r *http.Request, id string) {
	result, err := s.db.ExecContext(r.Context(), `DELETE FROM identities WHERE id = $1`, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		writeDBError(w, sql.ErrNoRows)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "identity.delete", EntityType: "identity", EntityID: &id, Message: "identity deleted"})
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) listReferenceImages(w http.ResponseWriter, r *http.Request, identityID string) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, identity_id, image_path, face_crop_path, embedding_json, quality_score, status, created_at
		FROM identity_reference_images
		WHERE identity_id = $1
		ORDER BY created_at DESC
	`, identityID)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer rows.Close()
	items := []referenceImageResponse{}
	for rows.Next() {
		item, err := scanReferenceImage(rows)
		if err != nil {
			writeDBError(w, err)
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"reference_images": items})
}

func (s *Server) createReferenceImage(w http.ResponseWriter, r *http.Request, identityID string) {
	if _, err := s.findIdentity(r, identityID); err != nil {
		writeDBError(w, err)
		return
	}
	imagePath, err := s.storeReferenceImageUpload(w, r, identityID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	row := s.db.QueryRowContext(r.Context(), `
		INSERT INTO identity_reference_images (identity_id, image_path, face_crop_path, embedding_json, quality_score, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, identity_id, image_path, face_crop_path, embedding_json, quality_score, status, created_at
	`, identityID, imagePath, nil, nil, nil, "active")
	item, err := scanReferenceImage(row)
	if err != nil {
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "identity.reference_image_create", EntityType: "identity", EntityID: &identityID, Message: "identity reference image created"})
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) deleteReferenceImage(w http.ResponseWriter, r *http.Request, id string) {
	var imagePath string
	if err := s.db.QueryRowContext(r.Context(), `SELECT image_path FROM identity_reference_images WHERE id = $1`, id).Scan(&imagePath); err != nil {
		writeDBError(w, err)
		return
	}
	result, err := s.db.ExecContext(r.Context(), `DELETE FROM identity_reference_images WHERE id = $1`, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		writeDBError(w, sql.ErrNoRows)
		return
	}
	if safePath, err := s.safeReferenceImagePath(imagePath); err == nil {
		_ = os.Remove(safePath)
	}
	s.recordEvent(r, eventRecord{EventType: "identity.reference_image_delete", EntityType: "identity_reference_image", EntityID: &id, Message: "identity reference image deleted"})
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) serveReferenceImage(w http.ResponseWriter, r *http.Request, id string) {
	var imagePath string
	if err := s.db.QueryRowContext(r.Context(), `SELECT image_path FROM identity_reference_images WHERE id = $1`, id).Scan(&imagePath); err != nil {
		writeDBError(w, err)
		return
	}
	safePath, err := s.safeReferenceImagePath(imagePath)
	if err != nil {
		writeError(w, http.StatusForbidden, "invalid_reference_image_path", err.Error(), nil)
		return
	}
	if contentType := mime.TypeByExtension(filepath.Ext(safePath)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, safePath)
}

func (s *Server) listObservations(w http.ResponseWriter, r *http.Request, user userResponse) {
	query := r.URL.Query()
	cameraIDs := parseCameraIDs(query["camera_id"])
	if user.Role != "admin" {
		if len(cameraIDs) == 0 {
			allowed, err := s.playbackCameraIDsForUser(r, user.ID)
			if err != nil {
				writeDBError(w, err)
				return
			}
			cameraIDs = allowed
		} else {
			cameraIDs = s.filterPlaybackCameraIDs(r, user, cameraIDs)
		}
		if len(cameraIDs) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{"observations": []observationResponse{}})
			return
		}
	}
	limit := boundedLimit(query.Get("limit"), 100, 500)
	args := []any{}
	clauses := []string{"TRUE"}
	if len(cameraIDs) > 0 {
		args = append(args, pq.Array(cameraIDs))
		clauses = append(clauses, "camera_id = ANY($"+strconv.Itoa(len(args))+"::uuid[])")
	}
	if typ := strings.TrimSpace(query.Get("observation_type")); typ != "" {
		args = append(args, typ)
		clauses = append(clauses, "observation_type = $"+strconv.Itoa(len(args)))
	}
	if identityID := strings.TrimSpace(query.Get("identity_id")); identityID != "" {
		if !isUUID(identityID) {
			writeError(w, http.StatusBadRequest, "validation_error", "identity_id must be a UUID", nil)
			return
		}
		args = append(args, identityID)
		clauses = append(clauses, "identity_id = $"+strconv.Itoa(len(args)))
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, camera_id, event_id, observed_at, observation_type, bbox_json, confidence,
			frame_path, crop_path, attributes_json, embedding_json, track_id, identity_id, created_at
		FROM observations
		WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY observed_at DESC
		LIMIT $`+strconv.Itoa(len(args))+`
	`, args...)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer rows.Close()
	items := []observationResponse{}
	for rows.Next() {
		item, err := scanObservation(rows)
		if err != nil {
			writeDBError(w, err)
			return
		}
		items = append(items, redactObservationPaths(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"observations": items})
}

func (s *Server) createObservation(w http.ResponseWriter, r *http.Request) {
	var req observationRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}
	if !isUUID(req.CameraID) || req.ObservationType == "" || req.ObservedAt.IsZero() {
		writeError(w, http.StatusBadRequest, "validation_error", "camera_id, observation_type, and observed_at are required", nil)
		return
	}
	row := s.db.QueryRowContext(r.Context(), `
		INSERT INTO observations (
			camera_id, event_id, observed_at, observation_type, bbox_json, confidence, frame_path,
			crop_path, attributes_json, embedding_json, track_id, identity_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, COALESCE($9::jsonb, '{}'::jsonb), $10, $11, $12)
		RETURNING id, camera_id, event_id, observed_at, observation_type, bbox_json, confidence,
			frame_path, crop_path, attributes_json, embedding_json, track_id, identity_id, created_at
	`, req.CameraID, nullableString(req.EventID), req.ObservedAt, req.ObservationType, nullableJSON(req.BBoxJSON), nullableFloat(req.Confidence), nullableCleanString(req.FramePath), nullableCleanString(req.CropPath), nullableJSON(req.AttributesJSON), nullableJSON(req.EmbeddingJSON), nullableString(req.TrackID), nullableString(req.IdentityID))
	item, err := scanObservation(row)
	if err != nil {
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "observation.create", EntityType: "observation", EntityID: &item.ID, Message: "observation created"})
	writeJSON(w, http.StatusCreated, redactObservationPaths(item))
}

func (s *Server) listIdentityMatchAttempts(w http.ResponseWriter, r *http.Request) {
	observationID := strings.TrimSpace(r.URL.Query().Get("observation_id"))
	if observationID == "" || !isUUID(observationID) {
		writeError(w, http.StatusBadRequest, "validation_error", "observation_id must be a UUID", nil)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, observation_id, identity_id, method, similarity_score, threshold, decision, created_at
		FROM identity_match_attempts
		WHERE observation_id = $1
		ORDER BY created_at DESC
	`, observationID)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer rows.Close()
	items := []identityMatchAttemptResponse{}
	for rows.Next() {
		item, err := scanIdentityMatchAttempt(rows)
		if err != nil {
			writeDBError(w, err)
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"identity_match_attempts": items})
}

func (s *Server) createIdentityMatchAttempt(w http.ResponseWriter, r *http.Request) {
	var req identityMatchAttemptRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}
	if !isUUID(req.ObservationID) || strings.TrimSpace(req.Method) == "" || strings.TrimSpace(req.Decision) == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "observation_id, method, and decision are required", nil)
		return
	}
	row := s.db.QueryRowContext(r.Context(), `
		INSERT INTO identity_match_attempts (observation_id, identity_id, method, similarity_score, threshold, decision)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, observation_id, identity_id, method, similarity_score, threshold, decision, created_at
	`, req.ObservationID, nullableString(req.IdentityID), strings.TrimSpace(req.Method), nullableFloat(req.SimilarityScore), nullableFloat(req.Threshold), strings.TrimSpace(req.Decision))
	item, err := scanIdentityMatchAttempt(row)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) listAIJobs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit := boundedLimit(query.Get("limit"), 100, 500)
	status := strings.TrimSpace(query.Get("status"))
	args := []any{limit}
	where := "TRUE"
	if status != "" {
		args = append([]any{status}, args...)
		where = "status = $1"
	}
	limitParam := "$" + strconv.Itoa(len(args))
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, camera_id, source_event_id, job_type, status, priority, frame_path, metadata_json,
			attempts, error_message, created_at, started_at, completed_at
		FROM ai_jobs
		WHERE `+where+`
		ORDER BY priority ASC, created_at ASC
		LIMIT `+limitParam+`
	`, args...)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer rows.Close()
	items := []aiJobResponse{}
	for rows.Next() {
		item, err := scanAIJob(rows)
		if err != nil {
			writeDBError(w, err)
			return
		}
		items = append(items, redactAIJobPaths(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"ai_jobs": items})
}

func (s *Server) createAIJob(w http.ResponseWriter, r *http.Request) {
	var req aiJobRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}
	if !isUUID(req.CameraID) || strings.TrimSpace(req.JobType) == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "camera_id and job_type are required", nil)
		return
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "pending"
	}
	priority := 100
	if req.Priority != nil {
		priority = *req.Priority
	}
	row := s.db.QueryRowContext(r.Context(), `
		INSERT INTO ai_jobs (camera_id, source_event_id, job_type, status, priority, frame_path, metadata_json)
		VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7::jsonb, '{}'::jsonb))
		RETURNING id, camera_id, source_event_id, job_type, status, priority, frame_path, metadata_json,
			attempts, error_message, created_at, started_at, completed_at
	`, req.CameraID, nullableString(req.SourceEventID), strings.TrimSpace(req.JobType), status, priority, nullableCleanString(req.FramePath), nullableJSON(req.MetadataJSON))
	item, err := scanAIJob(row)
	if err != nil {
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "ai.job_create", EntityType: "ai_job", EntityID: &item.ID, Message: "AI job queued"})
	writeJSON(w, http.StatusCreated, redactAIJobPaths(item))
}

func (s *Server) updateAIJob(w http.ResponseWriter, r *http.Request, id string) {
	var req aiJobPatchRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}
	status := nullableCleanString(req.Status)
	errorMessage := nullableCleanString(req.ErrorMessage)
	row := s.db.QueryRowContext(r.Context(), `
		UPDATE ai_jobs
		SET status = COALESCE($2, status),
			error_message = $3,
			started_at = CASE WHEN $2 = 'running' AND started_at IS NULL THEN now() ELSE started_at END,
			completed_at = CASE WHEN $2 IN ('completed', 'failed', 'cancelled') THEN now() ELSE completed_at END,
			attempts = CASE WHEN $2 = 'running' THEN attempts + 1 ELSE attempts END
		WHERE id = $1
		RETURNING id, camera_id, source_event_id, job_type, status, priority, frame_path, metadata_json,
			attempts, error_message, created_at, started_at, completed_at
	`, id, status, errorMessage)
	item, err := scanAIJob(row)
	if err != nil {
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "ai.job_update", EntityType: "ai_job", EntityID: &item.ID, Message: "AI job updated"})
	writeJSON(w, http.StatusOK, redactAIJobPaths(item))
}

func readIdentityRequest(w http.ResponseWriter, r *http.Request) (identityRequest, bool) {
	var req identityRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return req, false
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.DisplayName == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "display_name is required", nil)
		return req, false
	}
	req.Type = strings.TrimSpace(req.Type)
	if req.Type == "" {
		req.Type = "person"
	}
	return req, true
}

func (s *Server) findIdentity(r *http.Request, id string) (identityResponse, error) {
	row := s.db.QueryRowContext(r.Context(), `
		SELECT i.id, i.display_name, i.type, i.known, i.notes,
			COUNT(DISTINCT iri.id) AS reference_image_count,
			MAX(o.observed_at) AS last_seen_at,
			i.created_at, i.updated_at
		FROM identities i
		LEFT JOIN identity_reference_images iri ON iri.identity_id = i.id
		LEFT JOIN observations o ON o.identity_id = i.id
		WHERE i.id = $1
		GROUP BY i.id
	`, id)
	return scanIdentity(row)
}

func scanIdentity(scanner interface{ Scan(dest ...any) error }) (identityResponse, error) {
	var item identityResponse
	var notes sql.NullString
	var lastSeen sql.NullTime
	if err := scanner.Scan(&item.ID, &item.DisplayName, &item.Type, &item.Known, &notes, &item.ReferenceImageCount, &lastSeen, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return identityResponse{}, err
	}
	if notes.Valid {
		item.Notes = &notes.String
	}
	if lastSeen.Valid {
		item.LastSeenAt = &lastSeen.Time
	}
	return item, nil
}

func scanReferenceImage(scanner interface{ Scan(dest ...any) error }) (referenceImageResponse, error) {
	var item referenceImageResponse
	var faceCropPath sql.NullString
	var embeddingJSON []byte
	var qualityScore sql.NullFloat64
	var imagePath string
	if err := scanner.Scan(&item.ID, &item.IdentityID, &imagePath, &faceCropPath, &embeddingJSON, &qualityScore, &item.Status, &item.CreatedAt); err != nil {
		return referenceImageResponse{}, err
	}
	item.ImageURL = "/api/identity-reference-images/" + item.ID + "/file"
	if faceCropPath.Valid {
		item.FaceCropPath = &faceCropPath.String
	}
	if len(embeddingJSON) > 0 {
		item.EmbeddingJSON = json.RawMessage(embeddingJSON)
	}
	if qualityScore.Valid {
		item.QualityScore = &qualityScore.Float64
	}
	return item, nil
}

func scanObservation(scanner interface{ Scan(dest ...any) error }) (observationResponse, error) {
	var item observationResponse
	var eventID, framePath, cropPath, trackID, identityID sql.NullString
	var bboxJSON, attributesJSON, embeddingJSON []byte
	var confidence sql.NullFloat64
	if err := scanner.Scan(
		&item.ID, &item.CameraID, &eventID, &item.ObservedAt, &item.ObservationType, &bboxJSON, &confidence,
		&framePath, &cropPath, &attributesJSON, &embeddingJSON, &trackID, &identityID, &item.CreatedAt,
	); err != nil {
		return observationResponse{}, err
	}
	if eventID.Valid {
		item.EventID = &eventID.String
	}
	if len(bboxJSON) > 0 {
		item.BBoxJSON = json.RawMessage(bboxJSON)
	}
	if confidence.Valid {
		item.Confidence = &confidence.Float64
	}
	if framePath.Valid {
		item.FramePath = &framePath.String
	}
	if cropPath.Valid {
		item.CropPath = &cropPath.String
	}
	if len(attributesJSON) > 0 {
		item.AttributesJSON = json.RawMessage(attributesJSON)
	} else {
		item.AttributesJSON = json.RawMessage(`{}`)
	}
	if len(embeddingJSON) > 0 {
		item.EmbeddingJSON = json.RawMessage(embeddingJSON)
	}
	if trackID.Valid {
		item.TrackID = &trackID.String
	}
	if identityID.Valid {
		item.IdentityID = &identityID.String
	}
	return item, nil
}

func scanIdentityMatchAttempt(scanner interface{ Scan(dest ...any) error }) (identityMatchAttemptResponse, error) {
	var item identityMatchAttemptResponse
	var identityID sql.NullString
	var similarity, threshold sql.NullFloat64
	if err := scanner.Scan(&item.ID, &item.ObservationID, &identityID, &item.Method, &similarity, &threshold, &item.Decision, &item.CreatedAt); err != nil {
		return identityMatchAttemptResponse{}, err
	}
	if identityID.Valid {
		item.IdentityID = &identityID.String
	}
	if similarity.Valid {
		item.SimilarityScore = &similarity.Float64
	}
	if threshold.Valid {
		item.Threshold = &threshold.Float64
	}
	return item, nil
}

func scanAIJob(scanner interface{ Scan(dest ...any) error }) (aiJobResponse, error) {
	var item aiJobResponse
	var sourceEventID, framePath, errorMessage sql.NullString
	var startedAt, completedAt sql.NullTime
	var metadataJSON []byte
	if err := scanner.Scan(
		&item.ID, &item.CameraID, &sourceEventID, &item.JobType, &item.Status, &item.Priority,
		&framePath, &metadataJSON, &item.Attempts, &errorMessage, &item.CreatedAt, &startedAt, &completedAt,
	); err != nil {
		return aiJobResponse{}, err
	}
	if sourceEventID.Valid {
		item.SourceEventID = &sourceEventID.String
	}
	if framePath.Valid {
		item.FramePath = &framePath.String
	}
	if len(metadataJSON) > 0 {
		item.MetadataJSON = json.RawMessage(metadataJSON)
	} else {
		item.MetadataJSON = json.RawMessage(`{}`)
	}
	if errorMessage.Valid {
		item.ErrorMessage = &errorMessage.String
	}
	if startedAt.Valid {
		item.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		item.CompletedAt = &completedAt.Time
	}
	return item, nil
}

func boundedLimit(raw string, fallback, max int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func nullableJSON(raw json.RawMessage) any {
	normalized, ok := normalizeJSON(raw)
	if !ok || len(normalized) == 0 {
		return nil
	}
	return string(normalized)
}

func nullableFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func (s *Server) referenceImageRoot() string {
	return filepath.Join(s.recordingsPath, "identity-reference-images")
}

func (s *Server) storeReferenceImageUpload(w http.ResponseWriter, r *http.Request, identityID string) (string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		return "", fmt.Errorf("reference image upload must be multipart/form-data and at most 12 MB")
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		return "", fmt.Errorf("image file is required")
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
	default:
		return "", fmt.Errorf("image must be jpg, png, or webp")
	}
	dir := filepath.Join(s.referenceImageRoot(), identityID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("reference image folder cannot be created")
	}
	filename := fmt.Sprintf("%s%s", time.Now().UTC().Format("20060102T150405.000000000"), ext)
	target := filepath.Join(dir, filename)
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return "", fmt.Errorf("reference image cannot be stored")
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		_ = os.Remove(target)
		return "", fmt.Errorf("reference image cannot be written")
	}
	return target, nil
}

func (s *Server) safeReferenceImagePath(imagePath string) (string, error) {
	if imagePath == "" {
		return "", fmt.Errorf("reference image path is required")
	}
	return safeRecordingFilePath(s.referenceImageRoot(), imagePath)
}

func redactObservationPaths(item observationResponse) observationResponse {
	item.FramePath = nil
	item.CropPath = nil
	item.EmbeddingJSON = nil
	return item
}

func redactAIJobPaths(item aiJobResponse) aiJobResponse {
	item.FramePath = nil
	return item
}
