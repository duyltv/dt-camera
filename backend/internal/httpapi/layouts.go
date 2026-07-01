package httpapi

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var allowedTileTypes = map[string]struct{}{
	"small":     {},
	"large":     {},
	"portrait":  {},
	"landscape": {},
	"custom":    {},
}

type layoutResponse struct {
	ID        string               `json:"id"`
	Name      string               `json:"name"`
	Settings  json.RawMessage      `json:"settings"`
	IsDefault bool                 `json:"is_default"`
	Items     []layoutItemResponse `json:"layout_items"`
	CreatedAt time.Time            `json:"created_at"`
	UpdatedAt time.Time            `json:"updated_at"`
}

type layoutItemResponse struct {
	ID           string    `json:"id"`
	LayoutID     string    `json:"layout_id"`
	CameraID     string    `json:"camera_id"`
	X            int       `json:"x"`
	Y            int       `json:"y"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	DisplayOrder int       `json:"display_order"`
	TileType     string    `json:"tile_type"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type layoutItemPositionResponse struct {
	ItemID       string `json:"item_id"`
	LayoutID     string `json:"layout_id"`
	X            int    `json:"x"`
	Y            int    `json:"y"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	DisplayOrder int    `json:"display_order"`
	TileType     string `json:"tile_type"`
}

type createLayoutRequest struct {
	Name      string              `json:"name"`
	Settings  json.RawMessage     `json:"settings,omitempty"`
	IsDefault bool                `json:"is_default,omitempty"`
	Items     []layoutItemRequest `json:"layout_items,omitempty"`
}

type updateLayoutRequest struct {
	Name      *string          `json:"name,omitempty"`
	Settings  *json.RawMessage `json:"settings,omitempty"`
	IsDefault *bool            `json:"is_default,omitempty"`
}

type layoutItemRequest struct {
	CameraID     string  `json:"camera_id"`
	X            int     `json:"x"`
	Y            int     `json:"y"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	DisplayOrder int     `json:"display_order,omitempty"`
	TileType     *string `json:"tile_type,omitempty"`
}

func (s *Server) handleLayouts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if _, ok := s.requireUser(w, r); !ok {
			return
		}
		s.listLayouts(w, r)
	case http.MethodPost:
		if _, ok := s.requireAdmin(w, r); !ok {
			return
		}
		s.createLayout(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

func (s *Server) handleLayoutByID(w http.ResponseWriter, r *http.Request) {
	parts, ok := splitPathParts(r.URL.Path, "/api/layouts/")
	if !ok || len(parts) == 0 || !isUUID(parts[0]) {
		writeError(w, http.StatusNotFound, "not_found", "layout not found", nil)
		return
	}

	layoutID := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			if _, ok := s.requireUser(w, r); !ok {
				return
			}
			s.getLayout(w, r, layoutID)
		case http.MethodPatch:
			if _, ok := s.requireAdmin(w, r); !ok {
				return
			}
			s.updateLayout(w, r, layoutID)
		case http.MethodDelete:
			if _, ok := s.requireAdmin(w, r); !ok {
				return
			}
			s.deleteLayout(w, r, layoutID)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		}
		return
	}

	if len(parts) == 2 && parts[1] == "default" {
		if r.Method != http.MethodPatch {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		if _, ok := s.requireAdmin(w, r); !ok {
			return
		}
		s.setDefaultLayout(w, r, layoutID)
		return
	}

	if len(parts) == 2 && parts[1] == "items" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
			return
		}
		if _, ok := s.requireAdmin(w, r); !ok {
			return
		}
		s.createLayoutItem(w, r, layoutID)
		return
	}

	if len(parts) == 3 && parts[1] == "items" && isUUID(parts[2]) {
		switch r.Method {
		case http.MethodPatch:
			if _, ok := s.requireAdmin(w, r); !ok {
				return
			}
			s.updateLayoutItem(w, r, layoutID, parts[2])
		case http.MethodDelete:
			if _, ok := s.requireAdmin(w, r); !ok {
				return
			}
			s.deleteLayoutItem(w, r, layoutID, parts[2])
		default:
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		}
		return
	}

	writeError(w, http.StatusNotFound, "not_found", "layout endpoint not found", nil)
}

func (s *Server) createLayout(w http.ResponseWriter, r *http.Request) {
	var req createLayoutRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "name is required", nil)
		return
	}
	settings, ok := normalizeJSON(req.Settings)
	if !ok {
		writeError(w, http.StatusBadRequest, "validation_error", "settings must be valid JSON", nil)
		return
	}
	if err := validateLayoutItems(req.Items); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer tx.Rollback()

	if req.IsDefault {
		if err := clearGlobalDefaultLayout(r, tx); err != nil {
			writeDBError(w, err)
			return
		}
	}

	var layoutID string
	if err := tx.QueryRowContext(r.Context(), `
		INSERT INTO layouts (name, settings, is_default)
		VALUES ($1, $2::jsonb, $3)
		RETURNING id
	`, name, string(settings), req.IsDefault).Scan(&layoutID); err != nil {
		writeDBError(w, err)
		return
	}

	for _, item := range req.Items {
		if err := ensureCameraExistsTx(r, tx, item.CameraID); err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
			return
		}
		if _, err := insertLayoutItemTx(r, tx, layoutID, item); err != nil {
			writeDBError(w, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeDBError(w, err)
		return
	}

	layout, err := s.findLayout(r, layoutID)
	if err != nil {
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "layout.create", EntityType: "layout", EntityID: &layout.ID, Message: "layout created"})
	writeJSON(w, http.StatusCreated, layout)
}

func (s *Server) listLayouts(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, name, settings, is_default, created_at, updated_at
		FROM layouts
		ORDER BY is_default DESC, name
	`)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer rows.Close()

	layouts := []layoutResponse{}
	for rows.Next() {
		layout, err := scanLayout(rows)
		if err != nil {
			writeDBError(w, err)
			return
		}
		items, err := s.findLayoutItems(r, layout.ID)
		if err != nil {
			writeDBError(w, err)
			return
		}
		layout.Items = items
		layouts = append(layouts, layout)
	}
	if err := rows.Err(); err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"layouts": layouts})
}

func (s *Server) getLayout(w http.ResponseWriter, r *http.Request, id string) {
	layout, err := s.findLayout(r, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "layout.update", EntityType: "layout", EntityID: &layout.ID, Message: "layout updated"})
	writeJSON(w, http.StatusOK, layout)
}

func (s *Server) updateLayout(w http.ResponseWriter, r *http.Request, id string) {
	var req updateLayoutRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}

	current, err := s.findLayout(r, id)
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
	settings := current.Settings
	if req.Settings != nil {
		normalized, ok := normalizeJSON(*req.Settings)
		if !ok {
			writeError(w, http.StatusBadRequest, "validation_error", "settings must be valid JSON", nil)
			return
		}
		settings = normalized
	}
	isDefault := current.IsDefault
	if req.IsDefault != nil {
		isDefault = *req.IsDefault
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer tx.Rollback()

	if isDefault {
		if err := clearGlobalDefaultLayout(r, tx); err != nil {
			writeDBError(w, err)
			return
		}
	}
	result, err := tx.ExecContext(r.Context(), `
		UPDATE layouts
		SET name = $2, settings = $3::jsonb, is_default = $4
		WHERE id = $1
	`, id, name, string(settings), isDefault)
	if err != nil {
		writeDBError(w, err)
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeDBError(w, sql.ErrNoRows)
		return
	}
	if err := tx.Commit(); err != nil {
		writeDBError(w, err)
		return
	}

	layout, err := s.findLayout(r, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, layout)
}

func (s *Server) deleteLayout(w http.ResponseWriter, r *http.Request, id string) {
	result, err := s.db.ExecContext(r.Context(), `DELETE FROM layouts WHERE id = $1`, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeDBError(w, sql.ErrNoRows)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "layout.delete", EntityType: "layout", EntityID: &id, Message: "layout deleted"})
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) setDefaultLayout(w http.ResponseWriter, r *http.Request, id string) {
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeDBError(w, err)
		return
	}
	defer tx.Rollback()

	if err := clearGlobalDefaultLayout(r, tx); err != nil {
		writeDBError(w, err)
		return
	}
	result, err := tx.ExecContext(r.Context(), `UPDATE layouts SET is_default = TRUE WHERE id = $1`, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeDBError(w, sql.ErrNoRows)
		return
	}
	if err := tx.Commit(); err != nil {
		writeDBError(w, err)
		return
	}

	layout, err := s.findLayout(r, id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "layout.default", EntityType: "layout", EntityID: &layout.ID, Message: "default layout changed"})
	writeJSON(w, http.StatusOK, layout)
}

func (s *Server) createLayoutItem(w http.ResponseWriter, r *http.Request, layoutID string) {
	var req layoutItemRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}
	if err := validateLayoutItem(req); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	if err := s.ensureLayoutExists(r, layoutID); err != nil {
		writeDBError(w, err)
		return
	}
	if err := s.ensureCameraExists(r, req.CameraID); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	if err := s.ensureCameraNotInLayout(r, layoutID, req.CameraID, ""); err != nil {
		writeError(w, http.StatusConflict, "duplicate_layout_camera", err.Error(), nil)
		return
	}

	item, err := s.insertLayoutItem(r, layoutID, req)
	if err != nil {
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "layout_item.create", EntityType: "layout", EntityID: &layoutID, Message: "layout item created", Metadata: map[string]any{"item_id": item.ID, "camera_id": item.CameraID}})
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) updateLayoutItem(w http.ResponseWriter, r *http.Request, layoutID, itemID string) {
	var req layoutItemRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}
	if err := validateLayoutItem(req); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	if err := s.ensureCameraExists(r, req.CameraID); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	if err := s.ensureCameraNotInLayout(r, layoutID, req.CameraID, itemID); err != nil {
		writeError(w, http.StatusConflict, "duplicate_layout_camera", err.Error(), nil)
		return
	}

	item, err := s.updateLayoutItemRow(r, layoutID, itemID, req)
	if err != nil {
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "layout_item.update", EntityType: "layout", EntityID: &layoutID, Message: "layout item updated", Metadata: map[string]any{"item_id": item.ID, "camera_id": item.CameraID}})
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) deleteLayoutItem(w http.ResponseWriter, r *http.Request, layoutID, itemID string) {
	result, err := s.db.ExecContext(r.Context(), `DELETE FROM layout_items WHERE layout_id = $1 AND id = $2`, layoutID, itemID)
	if err != nil {
		writeDBError(w, err)
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeDBError(w, sql.ErrNoRows)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "layout_item.delete", EntityType: "layout", EntityID: &layoutID, Message: "layout item deleted", Metadata: map[string]any{"item_id": itemID}})
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) findLayout(r *http.Request, id string) (layoutResponse, error) {
	row := s.db.QueryRowContext(r.Context(), `
		SELECT id, name, settings, is_default, created_at, updated_at
		FROM layouts
		WHERE id = $1
	`, id)
	layout, err := scanLayout(row)
	if err != nil {
		return layoutResponse{}, err
	}
	items, err := s.findLayoutItems(r, id)
	if err != nil {
		return layoutResponse{}, err
	}
	layout.Items = items
	return layout, nil
}

func (s *Server) findLayoutItems(r *http.Request, layoutID string) ([]layoutItemResponse, error) {
	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, layout_id, camera_id, position_x, position_y, width, height, display_order, tile_type, created_at, updated_at
		FROM layout_items
		WHERE layout_id = $1
		ORDER BY display_order, position_y, position_x, created_at
	`, layoutID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []layoutItemResponse{}
	for rows.Next() {
		item, err := scanLayoutItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Server) ensureLayoutExists(r *http.Request, id string) error {
	var exists bool
	if err := s.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM layouts WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Server) ensureCameraExists(r *http.Request, id string) error {
	var exists bool
	if err := s.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cameras WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("camera_id does not exist")
	}
	return nil
}

func ensureCameraExistsTx(r *http.Request, tx *sql.Tx, id string) error {
	var exists bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cameras WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("camera_id does not exist")
	}
	return nil
}

func (s *Server) ensureCameraNotInLayout(r *http.Request, layoutID, cameraID, exceptItemID string) error {
	var exists bool
	if exceptItemID == "" {
		if err := s.db.QueryRowContext(r.Context(), `
			SELECT EXISTS(SELECT 1 FROM layout_items WHERE layout_id = $1 AND camera_id = $2)
		`, layoutID, cameraID).Scan(&exists); err != nil {
			return err
		}
	} else {
		if err := s.db.QueryRowContext(r.Context(), `
			SELECT EXISTS(SELECT 1 FROM layout_items WHERE layout_id = $1 AND camera_id = $2 AND id <> $3)
		`, layoutID, cameraID, exceptItemID).Scan(&exists); err != nil {
			return err
		}
	}
	if exists {
		return fmt.Errorf("layout already contains this camera")
	}
	return nil
}

func (s *Server) insertLayoutItem(r *http.Request, layoutID string, req layoutItemRequest) (layoutItemResponse, error) {
	row := s.db.QueryRowContext(r.Context(), `
		INSERT INTO layout_items (layout_id, camera_id, position_x, position_y, width, height, display_order, tile_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, layout_id, camera_id, position_x, position_y, width, height, display_order, tile_type, created_at, updated_at
	`, layoutID, req.CameraID, req.X, req.Y, req.Width, req.Height, req.DisplayOrder, normalizeTileType(req.TileType))
	return scanLayoutItem(row)
}

func insertLayoutItemTx(r *http.Request, tx *sql.Tx, layoutID string, req layoutItemRequest) (layoutItemResponse, error) {
	row := tx.QueryRowContext(r.Context(), `
		INSERT INTO layout_items (layout_id, camera_id, position_x, position_y, width, height, display_order, tile_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, layout_id, camera_id, position_x, position_y, width, height, display_order, tile_type, created_at, updated_at
	`, layoutID, req.CameraID, req.X, req.Y, req.Width, req.Height, req.DisplayOrder, normalizeTileType(req.TileType))
	return scanLayoutItem(row)
}

func (s *Server) updateLayoutItemRow(r *http.Request, layoutID, itemID string, req layoutItemRequest) (layoutItemResponse, error) {
	row := s.db.QueryRowContext(r.Context(), `
		UPDATE layout_items
		SET camera_id = $3, position_x = $4, position_y = $5, width = $6, height = $7, display_order = $8, tile_type = $9
		WHERE layout_id = $1 AND id = $2
		RETURNING id, layout_id, camera_id, position_x, position_y, width, height, display_order, tile_type, created_at, updated_at
	`, layoutID, itemID, req.CameraID, req.X, req.Y, req.Width, req.Height, req.DisplayOrder, normalizeTileType(req.TileType))
	return scanLayoutItem(row)
}

func scanLayout(scanner interface{ Scan(dest ...any) error }) (layoutResponse, error) {
	var layout layoutResponse
	var settings []byte
	if err := scanner.Scan(&layout.ID, &layout.Name, &settings, &layout.IsDefault, &layout.CreatedAt, &layout.UpdatedAt); err != nil {
		return layoutResponse{}, err
	}
	layout.Settings = json.RawMessage(settings)
	layout.Items = []layoutItemResponse{}
	return layout, nil
}

func scanLayoutItem(scanner interface{ Scan(dest ...any) error }) (layoutItemResponse, error) {
	var item layoutItemResponse
	if err := scanner.Scan(
		&item.ID,
		&item.LayoutID,
		&item.CameraID,
		&item.X,
		&item.Y,
		&item.Width,
		&item.Height,
		&item.DisplayOrder,
		&item.TileType,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return layoutItemResponse{}, err
	}
	return item, nil
}

func clearGlobalDefaultLayout(r *http.Request, tx *sql.Tx) error {
	_, err := tx.ExecContext(r.Context(), `UPDATE layouts SET is_default = FALSE WHERE user_id IS NULL AND is_default = TRUE`)
	return err
}

func validateLayoutItems(items []layoutItemRequest) error {
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if err := validateLayoutItem(item); err != nil {
			return err
		}
		if _, ok := seen[item.CameraID]; ok {
			return fmt.Errorf("layout must not contain duplicate camera items")
		}
		seen[item.CameraID] = struct{}{}
	}
	return nil
}

func validateLayoutItem(item layoutItemRequest) error {
	if !isUUID(item.CameraID) {
		return fmt.Errorf("camera_id must be a UUID")
	}
	if item.Width <= 0 || item.Height <= 0 {
		return fmt.Errorf("layout item width and height must be positive")
	}
	if item.X < 0 || item.Y < 0 {
		return fmt.Errorf("layout item x and y must be zero or positive")
	}
	tileType := normalizeTileType(item.TileType)
	if _, ok := allowedTileTypes[tileType]; !ok {
		return fmt.Errorf("tile_type must be one of small, large, portrait, landscape, custom")
	}
	return nil
}

func normalizeTileType(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "custom"
	}
	return strings.TrimSpace(*value)
}

func normalizeJSON(value json.RawMessage) (json.RawMessage, bool) {
	if len(value) == 0 {
		return json.RawMessage(`{}`), true
	}
	if !json.Valid(value) {
		return nil, false
	}
	return value, true
}
