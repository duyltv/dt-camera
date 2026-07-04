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

func TestDefaultRecordingEnabled(t *testing.T) {
	storageID := "018f5d67-89ab-7def-8123-456789abcdef"
	emptyStorageID := " "
	explicitFalse := false
	explicitTrue := true

	cases := []struct {
		name      string
		value     *bool
		storageID *string
		want      bool
	}{
		{name: "defaults on with storage", storageID: &storageID, want: true},
		{name: "defaults off without storage", want: false},
		{name: "defaults off with empty storage", storageID: &emptyStorageID, want: false},
		{name: "explicit false wins", value: &explicitFalse, storageID: &storageID, want: false},
		{name: "explicit true wins", value: &explicitTrue, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := defaultRecordingEnabled(tc.value, tc.storageID); got != tc.want {
				t.Fatalf("defaultRecordingEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseNullablePositiveInt64SupportsClearingCameraLimit(t *testing.T) {
	current := int64(1024)

	kept, err := parseNullablePositiveInt64(&current, nil, "max_storage_bytes")
	if err != nil {
		t.Fatalf("omitted value returned error: %v", err)
	}
	if kept == nil || *kept != current {
		t.Fatalf("omitted value should keep current limit, got %v", kept)
	}

	cleared, err := parseNullablePositiveInt64(&current, json.RawMessage(`null`), "max_storage_bytes")
	if err != nil {
		t.Fatalf("null value returned error: %v", err)
	}
	if cleared != nil {
		t.Fatalf("null value should clear current limit, got %d", *cleared)
	}

	updated, err := parseNullablePositiveInt64(&current, json.RawMessage(`2048`), "max_storage_bytes")
	if err != nil {
		t.Fatalf("numeric value returned error: %v", err)
	}
	if updated == nil || *updated != 2048 {
		t.Fatalf("numeric value should update limit to 2048, got %v", updated)
	}
}

func TestParseNullablePositiveInt64RejectsInvalidCameraLimit(t *testing.T) {
	for _, raw := range []json.RawMessage{json.RawMessage(`0`), json.RawMessage(`-1`), json.RawMessage(`"bad"`)} {
		if _, err := parseNullablePositiveInt64(nil, raw, "max_storage_bytes"); err == nil {
			t.Fatalf("expected error for %s", raw)
		}
	}
}
