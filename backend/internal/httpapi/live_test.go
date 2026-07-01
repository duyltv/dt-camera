package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdminCanViewLiveBypass(t *testing.T) {
	server := &Server{}
	admin := userResponse{ID: "admin", Role: "admin"}
	if !server.canViewLive(nil, admin, "camera") {
		t.Fatalf("admin should bypass live permission checks")
	}
}

func TestHLSLogSanitizationRedactsStreamURLs(t *testing.T) {
	output := sanitizeHLSLog("open rtsp://user:pass@192.168.1.57/path via tcp://192.168.1.57:554")
	if strings.Contains(output, "pass") || strings.Contains(output, "192.168.1.57") {
		t.Fatalf("stream URL leaked: %s", output)
	}
	if !strings.Contains(output, "rtsp://<redacted>") || !strings.Contains(output, "tcp://<redacted>") {
		t.Fatalf("expected redaction markers, got %s", output)
	}
}

func TestSafeHLSFilePathRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	cameraID := "018f5d67-89ab-4def-8123-456789abcdef"
	if _, err := safeHLSFilePath(root, cameraID, "../secret.m3u8"); err == nil {
		t.Fatalf("expected traversal to be rejected")
	}
	if _, err := safeHLSFilePath(root, cameraID, "segment_00001.mp4"); err == nil {
		t.Fatalf("expected unsupported extension to be rejected")
	}
	if _, err := safeHLSFilePath(root, cameraID, "index.m3u8"); err != nil {
		t.Fatalf("expected playlist path to be accepted: %v", err)
	}
}

func TestHLSContentType(t *testing.T) {
	tests := map[string]string{
		"index.m3u8":        "application/vnd.apple.mpegurl",
		"segment_00001.ts":  "video/mp2t",
		"segment_00001.mp4": "video/mp4",
	}
	for path, expected := range tests {
		if got := hlsContentType(path); got != expected {
			t.Fatalf("hlsContentType(%q) = %q, want %q", path, got, expected)
		}
	}
}

func TestHLSPlaylistReadyRequiresPlaylistAndSegment(t *testing.T) {
	root := t.TempDir()
	playlist := filepath.Join(root, "index.m3u8")
	if hlsPlaylistReady(playlist) {
		t.Fatalf("missing playlist should not be ready")
	}
	if err := os.WriteFile(playlist, []byte("#EXTM3U\n#EXTINF:2.0,\nsegment_00001.ts\n"), 0o600); err != nil {
		t.Fatalf("write playlist: %v", err)
	}
	if hlsPlaylistReady(playlist) {
		t.Fatalf("playlist without segment should not be ready")
	}
	if err := os.WriteFile(filepath.Join(root, "segment_00001.ts"), []byte("video"), 0o600); err != nil {
		t.Fatalf("write segment: %v", err)
	}
	if !hlsPlaylistReady(playlist) {
		t.Fatalf("playlist with referenced segment should be ready")
	}
}

func TestLayoutLiveResponseShape(t *testing.T) {
	response := layoutLiveCameraResponse{
		CameraID: "018f5d67-89ab-4def-8123-456789abcdef",
		Status:   "ok",
		HLSURL:   "/hls/018f5d67-89ab-4def-8123-456789abcdef/index.m3u8",
		LayoutItem: &layoutItemPositionResponse{
			ItemID:   "018f5d67-89ab-4def-8123-456789abcdea",
			LayoutID: "018f5d67-89ab-4def-8123-456789abcdeb",
			X:        0,
			Y:        0,
			Width:    2,
			Height:   2,
			TileType: "large",
		},
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	body := string(payload)
	if strings.Contains(body, "rtsp://") {
		t.Fatalf("live response leaked RTSP URL: %s", body)
	}
	for _, expected := range []string{`"hls_url"`, `"/hls/`, `"layout_item"`, `"tile_type":"large"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected %s in response body: %s", expected, body)
		}
	}
}
