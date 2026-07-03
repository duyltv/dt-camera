package httpapi

import (
	"database/sql"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type liveStreamResponse struct {
	CameraID   string `json:"camera_id"`
	CameraName string `json:"camera_name,omitempty"`
	Status     string `json:"status"`
	HLSURL     string `json:"hls_url,omitempty"`
}

type layoutLiveCameraResponse struct {
	CameraID   string                      `json:"camera_id"`
	CameraName string                      `json:"camera_name,omitempty"`
	Status     string                      `json:"status"`
	HLSURL     string                      `json:"hls_url,omitempty"`
	LayoutItem *layoutItemPositionResponse `json:"layout_item,omitempty"`
}

func (s *Server) handleLiveCameraByID(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	parts, ok := splitPathParts(r.URL.Path, "/api/live/cameras/")
	if !ok || len(parts) != 1 || !isUUID(parts[0]) {
		writeError(w, http.StatusNotFound, "not_found", "live camera endpoint not found", nil)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	response := s.liveInfoForCamera(r, user, parts[0])
	if response.Status == "no_permission" {
		writeError(w, http.StatusForbidden, "forbidden", "live permission required", response)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleLiveLayoutByID(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	parts, ok := splitPathParts(r.URL.Path, "/api/live/layouts/")
	if !ok || len(parts) != 1 || !isUUID(parts[0]) {
		writeError(w, http.StatusNotFound, "not_found", "live layout endpoint not found", nil)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	targets, err := s.resolvePlaybackCameraTargets(r, playbackPrepareRequest{LayoutID: &parts[0], SelectedTimestamp: time.Now()})
	if err != nil {
		writeDBError(w, err)
		return
	}
	cameras := make([]layoutLiveCameraResponse, 0, len(targets))
	for _, target := range targets {
		info := s.liveInfoForCamera(r, user, target.CameraID)
		if info.Status == "no_permission" {
			continue
		}
		cameras = append(cameras, layoutLiveCameraResponse{
			CameraID:   target.CameraID,
			CameraName: info.CameraName,
			Status:     info.Status,
			HLSURL:     info.HLSURL,
			LayoutItem: target.LayoutItem,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"layout_id": parts[0], "cameras": cameras})
}

func (s *Server) liveInfoForCamera(r *http.Request, user userResponse, cameraID string) liveStreamResponse {
	if !s.canViewLive(r, user, cameraID) {
		return liveStreamResponse{CameraID: cameraID, Status: "no_permission"}
	}
	camera, err := s.findLiveCamera(r, cameraID)
	if err != nil {
		return liveStreamResponse{CameraID: cameraID, Status: "stream_unavailable"}
	}
	if !camera.Enabled {
		if s.streams != nil {
			s.streams.stop(cameraID)
		}
		return liveStreamResponse{CameraID: cameraID, CameraName: camera.Name, Status: "camera_disabled"}
	}
	if !camera.StreamEnabled {
		if s.streams != nil {
			s.streams.stop(cameraID)
		}
		return liveStreamResponse{CameraID: cameraID, CameraName: camera.Name, Status: "stream_disabled"}
	}
	hlsURL, err := s.streams.ensureStream(r.Context(), camera)
	if err != nil {
		return liveStreamResponse{CameraID: cameraID, CameraName: camera.Name, Status: "stream_unavailable"}
	}
	if !s.streams.streamReady(cameraID) {
		if s.streams.waitReady(r.Context(), cameraID, 2*time.Second) {
			return liveStreamResponse{CameraID: cameraID, CameraName: camera.Name, Status: "ok", HLSURL: hlsURL}
		}
		return liveStreamResponse{CameraID: cameraID, CameraName: camera.Name, Status: "starting", HLSURL: hlsURL}
	}
	return liveStreamResponse{CameraID: cameraID, CameraName: camera.Name, Status: "ok", HLSURL: hlsURL}
}

func (s *Server) findLiveCamera(r *http.Request, cameraID string) (liveCamera, error) {
	row := s.db.QueryRowContext(r.Context(), `SELECT id, name, rtsp_url, is_enabled, stream_enabled, stream_audio FROM cameras WHERE id = $1`, cameraID)
	var camera liveCamera
	if err := row.Scan(&camera.ID, &camera.Name, &camera.RTSPURL, &camera.Enabled, &camera.StreamEnabled, &camera.StreamAudio); err != nil {
		if err == sql.ErrNoRows {
			return liveCamera{}, err
		}
		return liveCamera{}, err
	}
	return camera, nil
}

func (s *Server) handleHLSFile(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	parts, ok := splitPathParts(r.URL.Path, "/hls/")
	if !ok || len(parts) != 2 || !isUUID(parts[0]) {
		writeError(w, http.StatusNotFound, "not_found", "hls file not found", nil)
		return
	}
	cameraID := parts[0]
	filename := parts[1]
	if !s.canViewLive(r, user, cameraID) {
		writeError(w, http.StatusForbidden, "forbidden", "live permission required", nil)
		return
	}
	path, err := safeHLSFilePath(s.cfg.HLSRoot, cameraID, filename)
	if err != nil {
		writeError(w, http.StatusForbidden, "invalid_hls_path", err.Error(), nil)
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "not_found", "hls file not found", nil)
		return
	}
	if contentType := hlsContentType(path); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if s.streams != nil {
		s.streams.touch(cameraID)
	}
	http.ServeFile(w, r, path)
}

func hlsContentType(path string) string {
	switch filepath.Ext(path) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".ts":
		return "video/mp2t"
	default:
		return mime.TypeByExtension(filepath.Ext(path))
	}
}

func safeHLSFilePath(root, cameraID, filename string) (string, error) {
	if !isUUID(cameraID) {
		return "", fmt.Errorf("camera_id must be a UUID")
	}
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || filename == "." || filename == ".." {
		return "", fmt.Errorf("invalid hls filename")
	}
	if filepath.Ext(filename) != ".m3u8" && filepath.Ext(filename) != ".ts" {
		return "", fmt.Errorf("invalid hls file type")
	}
	cleanRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", fmt.Errorf("hls root cannot be resolved")
	}
	cleanPath, err := filepath.Abs(filepath.Join(cleanRoot, cameraID, filename))
	if err != nil {
		return "", fmt.Errorf("hls path cannot be resolved")
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("hls path escapes root")
	}
	return cleanPath, nil
}
