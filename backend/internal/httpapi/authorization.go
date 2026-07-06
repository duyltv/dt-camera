package httpapi

import "net/http"

func (s *Server) canViewPlayback(r *http.Request, user userResponse, cameraID string) bool {
	if user.Role == "admin" {
		return true
	}
	var allowed bool
	if err := s.db.QueryRowContext(r.Context(), `
		SELECT COALESCE((
			SELECT can_view_playback
			FROM user_camera_permissions
			WHERE user_id = $1 AND camera_id = $2
		), FALSE)
	`, user.ID, cameraID).Scan(&allowed); err != nil {
		return false
	}
	return allowed
}

func (s *Server) canViewLive(r *http.Request, user userResponse, cameraID string) bool {
	if user.Role == "admin" {
		return true
	}
	var allowed bool
	if err := s.db.QueryRowContext(r.Context(), `
		SELECT COALESCE((
			SELECT can_view_live
			FROM user_camera_permissions
			WHERE user_id = $1 AND camera_id = $2
		), FALSE)
	`, user.ID, cameraID).Scan(&allowed); err != nil {
		return false
	}
	return allowed
}

func (s *Server) filterPlaybackCameraIDs(r *http.Request, user userResponse, cameraIDs []string) []string {
	if user.Role == "admin" {
		return cameraIDs
	}
	allowed := make([]string, 0, len(cameraIDs))
	for _, cameraID := range cameraIDs {
		if s.canViewPlayback(r, user, cameraID) {
			allowed = append(allowed, cameraID)
		}
	}
	return allowed
}

func (s *Server) playbackCameraIDsForUser(r *http.Request, userID string) ([]string, error) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT camera_id
		FROM user_camera_permissions
		WHERE user_id = $1 AND can_view_playback = TRUE
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
