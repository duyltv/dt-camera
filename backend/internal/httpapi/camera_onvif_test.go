package httpapi

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFirstProfileTokenReadsONVIFProfileAttribute(t *testing.T) {
	body := []byte(`<Envelope><Body><GetProfilesResponse><Profiles token="profile_1"></Profiles></GetProfilesResponse></Body></Envelope>`)
	if got := firstProfileToken(body); got != "profile_1" {
		t.Fatalf("expected profile_1, got %q", got)
	}
}

func TestFirstNestedElementTextFindsMediaXAddr(t *testing.T) {
	body := []byte(`<Capabilities><Device><XAddr>http://device</XAddr></Device><Media><XAddr>http://media</XAddr></Media></Capabilities>`)
	if got := firstNestedElementText(body, "Media", "XAddr"); got != "http://media" {
		t.Fatalf("expected media xaddr, got %q", got)
	}
}

func TestAddCredentialsToRTSPURLDoesNotExposeOutsideReturnedValue(t *testing.T) {
	got := addCredentialsToRTSPURL("rtsp://192.168.1.57/cam", "admin", "secret")
	if !strings.HasPrefix(got, "rtsp://admin:secret@192.168.1.57/") {
		t.Fatalf("credentials not embedded for server-side storage: %q", got)
	}
	if second := addCredentialsToRTSPURL(got, "other", "pass"); second != got {
		t.Fatalf("existing credentials should not be replaced")
	}
}

func TestCameraPreviewCachePathRejectsInvalidCameraID(t *testing.T) {
	server := &Server{cfg: Config{PreviewCacheRoot: t.TempDir()}}
	if _, err := server.cameraPreviewCachePath("../secret"); err == nil {
		t.Fatal("expected invalid camera id to be rejected")
	}
}

func TestCameraPreviewCacheRoundTrip(t *testing.T) {
	root := t.TempDir()
	server := &Server{cfg: Config{PreviewCacheRoot: root}}
	cameraID := "018f5d67-89ab-4def-8123-456789abcdef"
	image := []byte("jpeg")

	server.saveCameraPreviewCache(cameraID, image)
	got, ok := server.readCameraPreviewCache(cameraID)
	if !ok {
		t.Fatal("expected cached preview to be readable")
	}
	if string(got) != string(image) {
		t.Fatalf("cached preview = %q, want %q", string(got), string(image))
	}
	path, err := server.cameraPreviewCachePath(cameraID)
	if err != nil {
		t.Fatalf("cache path failed: %v", err)
	}
	if filepath.Dir(path) != root {
		t.Fatalf("cache path should stay in root, got %q", path)
	}
}
