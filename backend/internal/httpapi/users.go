package httpapi

import (
	"database/sql"
	"net/http"
	"strings"
)

type createUserRequest struct {
	Email       string  `json:"email"`
	Username    *string `json:"username,omitempty"`
	DisplayName string  `json:"display_name"`
	Password    string  `json:"password"`
	Role        string  `json:"role"`
	Active      *bool   `json:"active,omitempty"`
}

type cameraPermissionRequest struct {
	CanViewLive     bool `json:"can_view_live"`
	CanViewPlayback bool `json:"can_view_playback"`
}

type cameraPermissionResponse struct {
	UserID          string `json:"user_id"`
	CameraID        string `json:"camera_id"`
	CanViewLive     bool   `json:"can_view_live"`
	CanViewPlayback bool   `json:"can_view_playback"`
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listUsers(w, r)
	case http.MethodPost:
		s.createUser(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleUserByID(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	parts, ok := splitPathParts(r.URL.Path, "/api/users/")
	if !ok || len(parts) == 0 || !isUUID(parts[0]) {
		writeError(w, http.StatusNotFound, "not_found", "user endpoint not found", nil)
		return
	}
	userID := parts[0]
	if len(parts) == 2 && parts[1] == "camera-permissions" && r.Method == http.MethodGet {
		s.listCameraPermissions(w, r, userID)
		return
	}
	if len(parts) == 3 && parts[1] == "camera-permissions" && isUUID(parts[2]) {
		switch r.Method {
		case http.MethodPut:
			s.upsertCameraPermission(w, r, userID, parts[2])
		case http.MethodDelete:
			s.deleteCameraPermission(w, r, userID, parts[2])
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		}
		return
	}
	writeError(w, http.StatusNotFound, "not_found", "user endpoint not found", nil)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}
	email := strings.TrimSpace(req.Email)
	displayName := strings.TrimSpace(req.DisplayName)
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "user"
	}
	if email == "" || displayName == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "email, display_name, and password are required", nil)
		return
	}
	if role != "admin" && role != "user" {
		writeError(w, http.StatusBadRequest, "validation_error", "role must be admin or user", nil)
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	row := s.db.QueryRowContext(r.Context(), `
		INSERT INTO users (email, username, display_name, password_hash, role, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, email, username, display_name, role, is_active, created_at, updated_at
	`, email, nullableString(req.Username), displayName, hash, role, active)
	user, err := scanUser(row)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, email, username, display_name, role, is_active, created_at, updated_at
		FROM users
		ORDER BY email
	`)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer rows.Close()
	users := []userResponse{}
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			writeDBError(w, err)
			return
		}
		users = append(users, user)
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (s *Server) listCameraPermissions(w http.ResponseWriter, r *http.Request, userID string) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT user_id, camera_id, can_view_live, can_view_playback
		FROM user_camera_permissions
		WHERE user_id = $1
		ORDER BY camera_id
	`, userID)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer rows.Close()
	permissions := []cameraPermissionResponse{}
	for rows.Next() {
		var permission cameraPermissionResponse
		if err := rows.Scan(&permission.UserID, &permission.CameraID, &permission.CanViewLive, &permission.CanViewPlayback); err != nil {
			writeDBError(w, err)
			return
		}
		permissions = append(permissions, permission)
	}
	writeJSON(w, http.StatusOK, map[string]any{"camera_permissions": permissions})
}

func (s *Server) upsertCameraPermission(w http.ResponseWriter, r *http.Request, userID, cameraID string) {
	var req cameraPermissionRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}
	if err := s.ensureUserExists(r, userID); err != nil {
		writeDBError(w, err)
		return
	}
	if err := s.ensureCameraExists(r, cameraID); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	row := s.db.QueryRowContext(r.Context(), `
		INSERT INTO user_camera_permissions (user_id, camera_id, can_view_live, can_view_playback)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, camera_id)
		DO UPDATE SET can_view_live = EXCLUDED.can_view_live, can_view_playback = EXCLUDED.can_view_playback
		RETURNING user_id, camera_id, can_view_live, can_view_playback
	`, userID, cameraID, req.CanViewLive, req.CanViewPlayback)
	var permission cameraPermissionResponse
	if err := row.Scan(&permission.UserID, &permission.CameraID, &permission.CanViewLive, &permission.CanViewPlayback); err != nil {
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{
		EventType:  "permission.grant",
		EntityType: "camera_permission",
		EntityID:   &cameraID,
		Message:    "camera permission granted",
		Metadata:   map[string]any{"user_id": userID, "can_view_live": permission.CanViewLive, "can_view_playback": permission.CanViewPlayback},
	})
	writeJSON(w, http.StatusOK, permission)
}

func (s *Server) deleteCameraPermission(w http.ResponseWriter, r *http.Request, userID, cameraID string) {
	result, err := s.db.ExecContext(r.Context(), `DELETE FROM user_camera_permissions WHERE user_id = $1 AND camera_id = $2`, userID, cameraID)
	if err != nil {
		writeDBError(w, err)
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeDBError(w, sql.ErrNoRows)
		return
	}
	s.recordEvent(r, eventRecord{
		EventType:  "permission.revoke",
		EntityType: "camera_permission",
		EntityID:   &cameraID,
		Message:    "camera permission revoked",
		Metadata:   map[string]any{"user_id": userID},
	})
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) ensureUserExists(r *http.Request, id string) error {
	var exists bool
	if err := s.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return sql.ErrNoRows
	}
	return nil
}
