package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCameraResponseDoesNotExposeRTSPURL(t *testing.T) {
	storageLocationID := "018f5d67-89ab-7def-8123-456789abcdef"
	response := cameraResponse{
		ID:                "018f5d67-89ab-7def-8123-456789abcdea",
		StorageLocationID: &storageLocationID,
		Name:              "Front Gate",
		Enabled:           true,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}

	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal camera response: %v", err)
	}

	body := string(payload)
	if strings.Contains(body, "rtsp") {
		t.Fatalf("camera response leaked RTSP content: %s", body)
	}
}
