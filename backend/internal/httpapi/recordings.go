package httpapi

import (
	"database/sql"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

type recordingSegmentResponse struct {
	ID                string    `json:"id"`
	CameraID          string    `json:"camera_id"`
	StorageLocationID *string   `json:"storage_location_id,omitempty"`
	StartTime         time.Time `json:"start_time"`
	EndTime           time.Time `json:"end_time"`
	DurationSeconds   float64   `json:"duration_seconds"`
	SizeBytes         int64     `json:"size_bytes"`
	Format            string    `json:"format"`
	Status            string    `json:"status"`
	PlaybackURL       string    `json:"playback_url"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type recordingSegmentInternal struct {
	recordingSegmentResponse
	FilePath             string
	StorageContainerPath *string
}

type availabilityRange struct {
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time"`
	DurationSeconds float64   `json:"duration_seconds"`
	SegmentCount    int       `json:"segment_count"`
}

type availabilityGap struct {
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time"`
	DurationSeconds float64   `json:"duration_seconds"`
}

type cameraAvailabilityResponse struct {
	CameraID string              `json:"camera_id"`
	Ranges   []availabilityRange `json:"ranges"`
	Gaps     []availabilityGap   `json:"gaps"`
}

type playbackPrepareRequest struct {
	LayoutID          *string   `json:"layout_id,omitempty"`
	CameraIDs         []string  `json:"camera_ids,omitempty"`
	SelectedTimestamp time.Time `json:"selected_timestamp"`
}

type playbackPrepareCameraResponse struct {
	CameraID          string                      `json:"camera_id"`
	CameraName        string                      `json:"camera_name,omitempty"`
	Status            string                      `json:"status"`
	SelectedTimestamp time.Time                   `json:"selected_timestamp"`
	SegmentStartTime  *time.Time                  `json:"segment_start_time,omitempty"`
	OffsetSeconds     *float64                    `json:"offset_seconds,omitempty"`
	LayoutItem        *layoutItemPositionResponse `json:"layout_item,omitempty"`
	Segment           *recordingSegmentResponse   `json:"segment,omitempty"`
}

type playbackCameraTarget struct {
	CameraID   string
	CameraName string
	LayoutItem *layoutItemPositionResponse
}

func (s *Server) handleRecordingSearch(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}

	filter, ok := parseRecordingFilter(w, r)
	if !ok {
		return
	}
	filter.CameraIDs = s.filterPlaybackCameraIDs(r, user, filter.CameraIDs)
	if len(filter.CameraIDs) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"segments": []recordingSegmentResponse{}})
		return
	}

	segments, err := s.searchRecordingSegments(r, filter)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"segments": segments})
}

func (s *Server) handleRecordingTimeline(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}

	filter, ok := parseRecordingFilter(w, r)
	if !ok {
		return
	}
	filter.CameraIDs = s.filterPlaybackCameraIDs(r, user, filter.CameraIDs)
	gapThreshold := parseOptionalDurationSeconds(r.URL.Query().Get("gap_threshold_seconds"), 3*time.Second)

	segments, err := s.searchRecordingSegments(r, filter)
	if err != nil {
		writeDBError(w, err)
		return
	}

	availability := buildAvailability(filter.CameraIDs, segments, gapThreshold)
	writeJSON(w, http.StatusOK, map[string]any{
		"camera_availability":   availability,
		"gap_threshold_seconds": gapThreshold.Seconds(),
	})
}

func (s *Server) handleRecordingByID(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	parts, ok := splitPathParts(r.URL.Path, "/api/recordings/")
	if !ok || len(parts) != 2 || parts[1] != "file" || !isUUID(parts[0]) {
		writeError(w, http.StatusNotFound, "not_found", "recording endpoint not found", nil)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	s.serveRecordingFile(w, r, user, parts[0])
}

func (s *Server) handlePlaybackPrepare(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}

	var req playbackPrepareRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}
	if req.SelectedTimestamp.IsZero() {
		writeError(w, http.StatusBadRequest, "validation_error", "selected_timestamp is required", nil)
		return
	}

	targets, err := s.resolvePlaybackCameraTargets(r, req)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "layout not found", nil)
			return
		}
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}

	results := make([]playbackPrepareCameraResponse, 0, len(targets))
	for _, target := range targets {
		if !s.canViewPlayback(r, user, target.CameraID) {
			continue
		}
		segment, err := s.findBestSegmentAround(r, target.CameraID, req.SelectedTimestamp)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				results = append(results, playbackPrepareCameraResponse{
					CameraID:          target.CameraID,
					CameraName:        target.CameraName,
					Status:            "no_recording",
					SelectedTimestamp: req.SelectedTimestamp,
					LayoutItem:        target.LayoutItem,
				})
				continue
			}
			writeDBError(w, err)
			return
		}
		offsetSeconds := 0.0
		if req.SelectedTimestamp.After(segment.StartTime) {
			offsetSeconds = req.SelectedTimestamp.Sub(segment.StartTime).Seconds()
		}
		if segment.DurationSeconds > 0 && offsetSeconds > segment.DurationSeconds {
			offsetSeconds = segment.DurationSeconds
		}
		segmentStartTime := segment.StartTime
		results = append(results, playbackPrepareCameraResponse{
			CameraID:          target.CameraID,
			CameraName:        target.CameraName,
			Status:            "ok",
			SelectedTimestamp: req.SelectedTimestamp,
			SegmentStartTime:  &segmentStartTime,
			OffsetSeconds:     &offsetSeconds,
			LayoutItem:        target.LayoutItem,
			Segment:           &segment.recordingSegmentResponse,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"selected_timestamp": req.SelectedTimestamp,
		"cameras":            results,
	})
}

type recordingFilter struct {
	CameraIDs []string
	StartTime time.Time
	EndTime   time.Time
}

func parseRecordingFilter(w http.ResponseWriter, r *http.Request) (recordingFilter, bool) {
	query := r.URL.Query()
	cameraIDs := parseCameraIDs(query["camera_id"])
	if len(cameraIDs) == 0 {
		cameraIDs = parseCameraIDs(query["camera_ids"])
	}
	if len(cameraIDs) == 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "at least one camera_id is required", nil)
		return recordingFilter{}, false
	}
	for _, id := range cameraIDs {
		if !isUUID(id) {
			writeError(w, http.StatusBadRequest, "validation_error", "camera_id must be a UUID", map[string]string{"camera_id": id})
			return recordingFilter{}, false
		}
	}

	startTime, err := parseRequiredTime(query.Get("start_time"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "start_time must be RFC3339", nil)
		return recordingFilter{}, false
	}
	endTime, err := parseRequiredTime(query.Get("end_time"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "end_time must be RFC3339", nil)
		return recordingFilter{}, false
	}
	if !endTime.After(startTime) {
		writeError(w, http.StatusBadRequest, "validation_error", "end_time must be after start_time", nil)
		return recordingFilter{}, false
	}

	return recordingFilter{CameraIDs: cameraIDs, StartTime: startTime, EndTime: endTime}, true
}

func (s *Server) searchRecordingSegments(r *http.Request, filter recordingFilter) ([]recordingSegmentResponse, error) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT
			id,
			camera_id,
			storage_location_id,
			start_time,
			COALESCE(end_time, start_time),
			COALESCE(duration_seconds, 0),
			size_bytes,
			format,
			status,
			created_at,
			updated_at
		FROM recording_segments
		WHERE camera_id = ANY($1::uuid[])
			AND start_time < $3
			AND COALESCE(end_time, start_time) > $2
			AND status <> 'deleted'
		ORDER BY camera_id, start_time
	`, pq.Array(filter.CameraIDs), filter.StartTime, filter.EndTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	segments := []recordingSegmentResponse{}
	for rows.Next() {
		segment, err := scanRecordingSegment(rows)
		if err != nil {
			return nil, err
		}
		segments = append(segments, segment)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return segments, nil
}

func (s *Server) serveRecordingFile(w http.ResponseWriter, r *http.Request, user userResponse, id string) {
	segment, err := s.findRecordingSegmentForFile(r, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	if !s.canViewPlayback(r, user, segment.CameraID) {
		writeError(w, http.StatusForbidden, "forbidden", "playback permission required", nil)
		return
	}

	if segment.StorageContainerPath == nil {
		writeError(w, http.StatusConflict, "invalid_storage", "recording segment has no storage location", nil)
		return
	}

	safePath, err := safeRecordingFilePath(*segment.StorageContainerPath, segment.FilePath)
	if err != nil {
		writeError(w, http.StatusForbidden, "invalid_recording_path", err.Error(), nil)
		return
	}
	info, err := os.Stat(safePath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "file_not_found", "recording file not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "file_error", "recording file cannot be inspected", nil)
		return
	}
	if info.IsDir() {
		writeError(w, http.StatusForbidden, "invalid_recording_path", "recording path is a directory", nil)
		return
	}

	if contentType := mime.TypeByExtension(filepath.Ext(safePath)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filepath.Base(safePath)))
	http.ServeFile(w, r, safePath)
}

func (s *Server) findRecordingSegmentForFile(r *http.Request, id string) (recordingSegmentInternal, error) {
	row := s.db.QueryRowContext(r.Context(), `
		SELECT
			rs.id,
			rs.camera_id,
			rs.storage_location_id,
			rs.start_time,
			COALESCE(rs.end_time, rs.start_time),
			COALESCE(rs.duration_seconds, 0),
			rs.file_path,
			rs.size_bytes,
			rs.format,
			rs.status,
			rs.created_at,
			rs.updated_at,
			sl.container_path
		FROM recording_segments rs
		LEFT JOIN storage_locations sl ON sl.id = rs.storage_location_id
		WHERE rs.id = $1
	`, id)
	return scanRecordingSegmentInternal(row)
}

func (s *Server) findBestSegmentAround(r *http.Request, cameraID string, selected time.Time) (recordingSegmentInternal, error) {
	row := s.db.QueryRowContext(r.Context(), `
		SELECT
			id,
			camera_id,
			storage_location_id,
			start_time,
			COALESCE(end_time, start_time),
			COALESCE(duration_seconds, 0),
			file_path,
			size_bytes,
			format,
			status,
			created_at,
			updated_at,
			NULL::text
		FROM recording_segments
		WHERE camera_id = $1
			AND status = 'completed'
			AND start_time <= $2
			AND COALESCE(end_time, start_time) >= $2
		ORDER BY start_time DESC
		LIMIT 1
	`, cameraID, selected)
	return scanRecordingSegmentInternal(row)
}

func (s *Server) resolvePlaybackCameraTargets(r *http.Request, req playbackPrepareRequest) ([]playbackCameraTarget, error) {
	if req.LayoutID != nil && strings.TrimSpace(*req.LayoutID) != "" {
		layoutID := strings.TrimSpace(*req.LayoutID)
		if !isUUID(layoutID) {
			return nil, fmt.Errorf("layout_id must be a UUID")
		}
		rows, err := s.db.QueryContext(r.Context(), `
			SELECT li.id, li.layout_id, li.camera_id, c.name, li.position_x, li.position_y, li.width, li.height, li.display_order, li.tile_type
			FROM layout_items li
			JOIN cameras c ON c.id = li.camera_id
			WHERE li.layout_id = $1 AND li.camera_id IS NOT NULL
			ORDER BY li.display_order, li.position_y, li.position_x
		`, layoutID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		targets := []playbackCameraTarget{}
		for rows.Next() {
			var item layoutItemPositionResponse
			var cameraID string
			var cameraName string
			if err := rows.Scan(
				&item.ItemID,
				&item.LayoutID,
				&cameraID,
				&cameraName,
				&item.X,
				&item.Y,
				&item.Width,
				&item.Height,
				&item.DisplayOrder,
				&item.TileType,
			); err != nil {
				return nil, err
			}
			targets = append(targets, playbackCameraTarget{CameraID: cameraID, CameraName: cameraName, LayoutItem: &item})
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if len(targets) == 0 {
			var exists bool
			if err := s.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM layouts WHERE id = $1)`, layoutID).Scan(&exists); err != nil {
				return nil, err
			}
			if !exists {
				return nil, sql.ErrNoRows
			}
		}
		return targets, nil
	}

	cameraIDs := dedupeStrings(req.CameraIDs)
	if len(cameraIDs) == 0 {
		return nil, fmt.Errorf("layout_id or camera_ids is required")
	}
	for _, cameraID := range cameraIDs {
		if !isUUID(cameraID) {
			return nil, fmt.Errorf("camera_ids must contain only UUID values")
		}
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id, name FROM cameras WHERE id = ANY($1::uuid[])`, pq.Array(cameraIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cameraNames := map[string]string{}
	for rows.Next() {
		var cameraID string
		var cameraName string
		if err := rows.Scan(&cameraID, &cameraName); err != nil {
			return nil, err
		}
		cameraNames[cameraID] = cameraName
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	targets := make([]playbackCameraTarget, 0, len(cameraIDs))
	for _, cameraID := range cameraIDs {
		targets = append(targets, playbackCameraTarget{CameraID: cameraID, CameraName: cameraNames[cameraID]})
	}
	return targets, nil
}

func scanRecordingSegment(scanner interface{ Scan(dest ...any) error }) (recordingSegmentResponse, error) {
	var segment recordingSegmentResponse
	var storageLocationID sql.NullString
	if err := scanner.Scan(
		&segment.ID,
		&segment.CameraID,
		&storageLocationID,
		&segment.StartTime,
		&segment.EndTime,
		&segment.DurationSeconds,
		&segment.SizeBytes,
		&segment.Format,
		&segment.Status,
		&segment.CreatedAt,
		&segment.UpdatedAt,
	); err != nil {
		return recordingSegmentResponse{}, err
	}
	if storageLocationID.Valid {
		segment.StorageLocationID = &storageLocationID.String
	}
	segment.PlaybackURL = "/api/recordings/" + segment.ID + "/file"
	return segment, nil
}

func scanRecordingSegmentInternal(scanner interface{ Scan(dest ...any) error }) (recordingSegmentInternal, error) {
	var segment recordingSegmentInternal
	var storageLocationID sql.NullString
	var storageContainerPath sql.NullString
	if err := scanner.Scan(
		&segment.ID,
		&segment.CameraID,
		&storageLocationID,
		&segment.StartTime,
		&segment.EndTime,
		&segment.DurationSeconds,
		&segment.FilePath,
		&segment.SizeBytes,
		&segment.Format,
		&segment.Status,
		&segment.CreatedAt,
		&segment.UpdatedAt,
		&storageContainerPath,
	); err != nil {
		return recordingSegmentInternal{}, err
	}
	if storageLocationID.Valid {
		segment.StorageLocationID = &storageLocationID.String
	}
	if storageContainerPath.Valid {
		segment.StorageContainerPath = &storageContainerPath.String
	}
	segment.PlaybackURL = "/api/recordings/" + segment.ID + "/file"
	return segment, nil
}

func buildAvailability(cameraIDs []string, segments []recordingSegmentResponse, gapThreshold time.Duration) []cameraAvailabilityResponse {
	byCamera := make(map[string][]recordingSegmentResponse)
	for _, segment := range segments {
		byCamera[segment.CameraID] = append(byCamera[segment.CameraID], segment)
	}

	result := make([]cameraAvailabilityResponse, 0, len(cameraIDs))
	for _, cameraID := range cameraIDs {
		cameraSegments := byCamera[cameraID]
		ranges := []availabilityRange{}
		gaps := []availabilityGap{}
		for _, segment := range cameraSegments {
			if len(ranges) == 0 {
				ranges = append(ranges, availabilityRange{
					StartTime:       segment.StartTime,
					EndTime:         segment.EndTime,
					DurationSeconds: segment.EndTime.Sub(segment.StartTime).Seconds(),
					SegmentCount:    1,
				})
				continue
			}

			current := &ranges[len(ranges)-1]
			gap := segment.StartTime.Sub(current.EndTime)
			if gap <= gapThreshold {
				if segment.EndTime.After(current.EndTime) {
					current.EndTime = segment.EndTime
				}
				current.DurationSeconds = current.EndTime.Sub(current.StartTime).Seconds()
				current.SegmentCount++
				continue
			}

			gaps = append(gaps, availabilityGap{
				StartTime:       current.EndTime,
				EndTime:         segment.StartTime,
				DurationSeconds: gap.Seconds(),
			})
			ranges = append(ranges, availabilityRange{
				StartTime:       segment.StartTime,
				EndTime:         segment.EndTime,
				DurationSeconds: segment.EndTime.Sub(segment.StartTime).Seconds(),
				SegmentCount:    1,
			})
		}

		result = append(result, cameraAvailabilityResponse{
			CameraID: cameraID,
			Ranges:   ranges,
			Gaps:     gaps,
		})
	}
	return result
}

func safeRecordingFilePath(storageRoot, filePath string) (string, error) {
	if storageRoot == "" || filePath == "" {
		return "", fmt.Errorf("storage root and file path are required")
	}
	if !filepath.IsAbs(storageRoot) || !filepath.IsAbs(filePath) {
		return "", fmt.Errorf("recording paths must be absolute container paths")
	}

	cleanRoot, err := filepath.Abs(filepath.Clean(storageRoot))
	if err != nil {
		return "", fmt.Errorf("storage root cannot be resolved")
	}
	cleanPath, err := filepath.Abs(filepath.Clean(filePath))
	if err != nil {
		return "", fmt.Errorf("recording path cannot be resolved")
	}

	rootEval, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil {
		return "", fmt.Errorf("storage root cannot be verified")
	}
	pathEval := cleanPath
	if _, err := os.Stat(cleanPath); err == nil {
		if evaluated, err := filepath.EvalSymlinks(cleanPath); err == nil {
			pathEval = evaluated
		}
	}

	rel, err := filepath.Rel(rootEval, pathEval)
	if err != nil {
		return "", fmt.Errorf("recording path cannot be compared with storage root")
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("recording file is outside its storage location")
	}

	return pathEval, nil
}

func parseCameraIDs(values []string) []string {
	var ids []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				ids = append(ids, part)
			}
		}
	}
	return dedupeStrings(ids)
}

func parseRequiredTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, fmt.Errorf("time is required")
	}
	return time.Parse(time.RFC3339, value)
}

func parseOptionalDurationSeconds(value string, fallback time.Duration) time.Duration {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
