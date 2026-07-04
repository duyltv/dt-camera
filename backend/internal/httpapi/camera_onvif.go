package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"database/sql"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type onvifCameraRequest struct {
	XAddr             string  `json:"xaddr"`
	Name              string  `json:"name,omitempty"`
	Username          string  `json:"username,omitempty"`
	Password          string  `json:"password,omitempty"`
	StorageLocationID *string `json:"storage_location_id,omitempty"`
	Enabled           *bool   `json:"enabled,omitempty"`
	RecordingEnabled  *bool   `json:"recording_enabled,omitempty"`
	RecordAudio       *bool   `json:"record_audio,omitempty"`
	StreamEnabled     *bool   `json:"stream_enabled,omitempty"`
	StreamAudio       *bool   `json:"stream_audio,omitempty"`
	RetentionDays     *int    `json:"retention_days,omitempty"`
	MaxStorageBytes   *int64  `json:"max_storage_bytes,omitempty"`
}

type onvifStreamInfo struct {
	DeviceXAddr  string
	MediaXAddr   string
	ProfileToken string
	RTSPURL      string
}

func (s *Server) handleCameraONVIFTest(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	var req onvifCameraRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}
	stream, err := discoverONVIFStream(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "onvif_test_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"stream_found":  stream.RTSPURL != "",
		"media_service": stream.MediaXAddr != "",
		"profile_token": stream.ProfileToken,
	})
}

func (s *Server) handleCameraONVIFImport(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	var req onvifCameraRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "name is required", nil)
		return
	}
	if err := s.validateStorageLocationForCamera(r, req.StorageLocationID); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
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

	stream, err := discoverONVIFStream(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "onvif_test_failed", err.Error(), nil)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	recordingEnabled := defaultRecordingEnabled(req.RecordingEnabled, req.StorageLocationID)
	if recordingEnabled && req.StorageLocationID == nil {
		writeError(w, http.StatusBadRequest, "validation_error", "storage_location_id is required when recording is enabled", nil)
		return
	}
	recordAudio := boolValue(req.RecordAudio, false)
	streamEnabled := boolValue(req.StreamEnabled, true)
	streamAudio := boolValue(req.StreamAudio, false)
	location := hostFromHTTPURL(req.XAddr)
	cameraGroup := "Discovered"
	row := s.db.QueryRowContext(r.Context(), `
		INSERT INTO cameras (name, rtsp_url, storage_location_id, location, camera_group, is_enabled, recording_enabled, record_audio, stream_enabled, stream_audio, retention_days, max_storage_bytes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, storage_location_id, name, location, camera_group, is_enabled, recording_enabled, record_audio, stream_enabled, stream_audio, retention_days, max_storage_bytes, created_at, updated_at
	`, name, stream.RTSPURL, nullableString(req.StorageLocationID), nullableString(&location), nullableString(&cameraGroup), enabled, recordingEnabled, recordAudio, streamEnabled, streamAudio, retentionDays, nullableInt64(req.MaxStorageBytes))
	camera, err := scanCamera(row)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusInternalServerError, "camera_create_failed", "camera was not created", nil)
			return
		}
		writeDBError(w, err)
		return
	}
	s.recordEvent(r, eventRecord{EventType: "camera.create", EntityType: "camera", EntityID: &camera.ID, Message: "camera created from ONVIF discovery"})
	writeJSON(w, http.StatusCreated, camera)
}

func (s *Server) handleCameraONVIFPreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdmin(w, r); !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	var req onvifCameraRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", err.Error())
		return
	}
	stream, err := discoverONVIFStream(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "onvif_preview_failed", err.Error(), nil)
		return
	}
	image, err := captureRTSPPreview(r.Context(), stream.RTSPURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "onvif_preview_failed", "could not capture camera preview image", nil)
		return
	}
	writeJPEG(w, image)
}

func captureRTSPPreview(ctx context.Context, rtspURL string) ([]byte, error) {
	previewCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(previewCtx, "ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-rtsp_transport", "tcp",
		"-i", rtspURL,
		"-frames:v", "1",
		"-q:v", "4",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"pipe:1",
	)
	var stderr limitedHLSBuffer
	stderr.max = 4096
	cmd.Stderr = &stderr
	image, err := cmd.Output()
	if err != nil || len(image) == 0 {
		return nil, fmt.Errorf("could not capture camera preview image")
	}
	return image, nil
}

func writeJPEG(w http.ResponseWriter, image []byte) {
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(image)
}

func (s *Server) cameraPreviewCachePath(cameraID string) (string, error) {
	if !isUUID(cameraID) {
		return "", fmt.Errorf("invalid camera id")
	}
	root, err := filepath.Abs(s.cfg.PreviewCacheRoot)
	if err != nil {
		return "", fmt.Errorf("preview cache root cannot be resolved")
	}
	path, err := filepath.Abs(filepath.Join(root, cameraID+".jpg"))
	if err != nil {
		return "", fmt.Errorf("preview cache path cannot be resolved")
	}
	if path != filepath.Join(root, cameraID+".jpg") {
		return "", fmt.Errorf("preview cache path escapes root")
	}
	return path, nil
}

func (s *Server) saveCameraPreviewCache(cameraID string, image []byte) {
	path, err := s.cameraPreviewCachePath(cameraID)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, image, 0o644)
}

func (s *Server) readCameraPreviewCache(cameraID string) ([]byte, bool) {
	path, err := s.cameraPreviewCachePath(cameraID)
	if err != nil {
		return nil, false
	}
	image, err := os.ReadFile(path)
	if err != nil || len(image) == 0 {
		return nil, false
	}
	return image, true
}

func discoverONVIFStream(ctx context.Context, req onvifCameraRequest) (onvifStreamInfo, error) {
	xaddr, err := validateONVIFXAddr(req.XAddr)
	if err != nil {
		return onvifStreamInfo{}, err
	}
	client := &http.Client{Timeout: 7 * time.Second}
	username := strings.TrimSpace(req.Username)
	password := req.Password
	if (username == "") != (password == "") {
		return onvifStreamInfo{}, fmt.Errorf("username and password must be provided together")
	}

	mediaXAddr := discoverONVIFMediaXAddr(ctx, client, xaddr, username, password)
	if mediaXAddr == "" {
		mediaXAddr = xaddr
	}
	token, err := discoverONVIFProfileToken(ctx, client, mediaXAddr, username, password)
	if err != nil {
		return onvifStreamInfo{}, err
	}
	rtspURL, err := discoverONVIFStreamURI(ctx, client, mediaXAddr, token, username, password)
	if err != nil {
		return onvifStreamInfo{}, err
	}
	return onvifStreamInfo{
		DeviceXAddr:  xaddr,
		MediaXAddr:   mediaXAddr,
		ProfileToken: token,
		RTSPURL:      addCredentialsToRTSPURL(rtspURL, username, password),
	}, nil
}

func validateONVIFXAddr(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("xaddr must be a valid ONVIF HTTP URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("xaddr must use http or https")
	}
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	if ip == nil {
		ips, err := net.LookupIP(host)
		if err != nil || len(ips) == 0 {
			return "", fmt.Errorf("xaddr host could not be resolved")
		}
		ip = ips[0]
	}
	if !isAllowedCameraScanIP(ip) {
		return "", fmt.Errorf("xaddr must point to a private or local IP address")
	}
	return value, nil
}

func discoverONVIFMediaXAddr(ctx context.Context, client *http.Client, xaddr, username, password string) string {
	body, err := postONVIFSOAP(ctx, client, xaddr, "http://www.onvif.org/ver10/device/wsdl/GetCapabilities", getCapabilitiesEnvelope, username, password)
	if err != nil {
		return ""
	}
	if mediaXAddr := firstNestedElementText(body, "Media", "XAddr"); mediaXAddr != "" {
		return mediaXAddr
	}
	return firstElementText(body, "XAddr")
}

func discoverONVIFProfileToken(ctx context.Context, client *http.Client, mediaXAddr, username, password string) (string, error) {
	body, err := postONVIFSOAP(ctx, client, mediaXAddr, "http://www.onvif.org/ver10/media/wsdl/GetProfiles", getProfilesEnvelope, username, password)
	if err != nil {
		return "", fmt.Errorf("get ONVIF media profiles: %w", err)
	}
	token := firstProfileToken(body)
	if token == "" {
		return "", fmt.Errorf("ONVIF media profile was not found")
	}
	return token, nil
}

func discoverONVIFStreamURI(ctx context.Context, client *http.Client, mediaXAddr, profileToken, username, password string) (string, error) {
	envelope := fmt.Sprintf(getStreamURIEnvelope, xmlEscape(profileToken))
	body, err := postONVIFSOAP(ctx, client, mediaXAddr, "http://www.onvif.org/ver10/media/wsdl/GetStreamUri", envelope, username, password)
	if err != nil {
		return "", fmt.Errorf("get ONVIF stream URI: %w", err)
	}
	uri := firstElementText(body, "Uri")
	if !strings.HasPrefix(strings.ToLower(uri), "rtsp://") {
		return "", fmt.Errorf("ONVIF stream URI was not an RTSP URL")
	}
	return uri, nil
}

func postONVIFSOAP(ctx context.Context, client *http.Client, endpoint, action, body, username, password string) ([]byte, error) {
	envelope := withONVIFSecurity(body, username, password)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(envelope))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	httpReq.Header.Set("SOAPAction", action)
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("ONVIF authentication failed")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ONVIF service returned HTTP %d", resp.StatusCode)
	}
	return data, nil
}

func withONVIFSecurity(body, username, password string) string {
	header := ""
	if username != "" {
		header = buildONVIFSecurityHeader(username, password)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tds="http://www.onvif.org/ver10/device/wsdl" xmlns:trt="http://www.onvif.org/ver10/media/wsdl" xmlns:tt="http://www.onvif.org/ver10/schema">%s<s:Body>%s</s:Body></s:Envelope>`, header, body)
}

func buildONVIFSecurityHeader(username, password string) string {
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	created := time.Now().UTC().Format(time.RFC3339)
	sum := sha1.Sum(append(append(nonce, []byte(created)...), []byte(password)...))
	return fmt.Sprintf(`<s:Header><wsse:Security s:mustUnderstand="1" xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd" xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd"><wsse:UsernameToken><wsse:Username>%s</wsse:Username><wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">%s</wsse:Password><wsse:Nonce>%s</wsse:Nonce><wsu:Created>%s</wsu:Created></wsse:UsernameToken></wsse:Security></s:Header>`,
		xmlEscape(username), base64.StdEncoding.EncodeToString(sum[:]), base64.StdEncoding.EncodeToString(nonce), created)
}

func firstProfileToken(body []byte) string {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	for {
		token, err := decoder.Token()
		if err != nil {
			return ""
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Profiles" {
			continue
		}
		for _, attr := range start.Attr {
			if attr.Name.Local == "token" {
				return strings.TrimSpace(attr.Value)
			}
		}
	}
}

func firstElementText(body []byte, localName string) string {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	for {
		token, err := decoder.Token()
		if err != nil {
			return ""
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != localName {
			continue
		}
		var value string
		if err := decoder.DecodeElement(&value, &start); err != nil {
			return ""
		}
		return strings.TrimSpace(value)
	}
}

func firstNestedElementText(body []byte, parentName, childName string) string {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	depth := 0
	inParent := false
	for {
		token, err := decoder.Token()
		if err != nil {
			return ""
		}
		switch node := token.(type) {
		case xml.StartElement:
			if node.Name.Local == parentName {
				inParent = true
				depth = 1
				continue
			}
			if inParent {
				depth++
				if node.Name.Local == childName {
					var value string
					if err := decoder.DecodeElement(&value, &node); err != nil {
						return ""
					}
					return strings.TrimSpace(value)
				}
			}
		case xml.EndElement:
			if inParent {
				depth--
				if depth <= 0 {
					inParent = false
				}
			}
		}
	}
}

func addCredentialsToRTSPURL(value, username, password string) string {
	if username == "" {
		return value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.Scheme != "rtsp" {
		return value
	}
	parsed.User = url.UserPassword(username, password)
	return parsed.String()
}

func hostFromHTTPURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func xmlEscape(value string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(value))
	return buf.String()
}

const getCapabilitiesEnvelope = `<tds:GetCapabilities><tds:Category>All</tds:Category></tds:GetCapabilities>`

const getProfilesEnvelope = `<trt:GetProfiles/>`

const getStreamURIEnvelope = `<trt:GetStreamUri><trt:StreamSetup><tt:Stream>RTP-Unicast</tt:Stream><tt:Transport><tt:Protocol>RTSP</tt:Protocol></tt:Transport></trt:StreamSetup><trt:ProfileToken>%s</trt:ProfileToken></trt:GetStreamUri>`
