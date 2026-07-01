package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateLayoutItemsPreventsDuplicateCameras(t *testing.T) {
	cameraID := "018f5d67-89ab-4def-8123-456789abcdef"
	items := []layoutItemRequest{
		{CameraID: cameraID, X: 0, Y: 0, Width: 2, Height: 2},
		{CameraID: cameraID, X: 2, Y: 0, Width: 2, Height: 2},
	}

	if err := validateLayoutItems(items); err == nil {
		t.Fatalf("expected duplicate camera validation error")
	}
}

func TestValidateLayoutItemRequiresPositiveDimensions(t *testing.T) {
	item := layoutItemRequest{
		CameraID: "018f5d67-89ab-4def-8123-456789abcdef",
		X:        0,
		Y:        0,
		Width:    0,
		Height:   2,
	}

	if err := validateLayoutItem(item); err == nil {
		t.Fatalf("expected positive dimension validation error")
	}
}

func TestPlaybackPrepareResponseIncludesLayoutPosition(t *testing.T) {
	response := playbackPrepareCameraResponse{
		CameraID: "018f5d67-89ab-4def-8123-456789abcdef",
		Status:   "no_recording",
		LayoutItem: &layoutItemPositionResponse{
			ItemID:       "018f5d67-89ab-4def-8123-456789abcdea",
			LayoutID:     "018f5d67-89ab-4def-8123-456789abcdeb",
			X:            0,
			Y:            1,
			Width:        2,
			Height:       3,
			DisplayOrder: 4,
			TileType:     "portrait",
		},
	}

	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	body := string(payload)
	for _, expected := range []string{`"layout_item"`, `"x":0`, `"height":3`, `"tile_type":"portrait"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected %s in response body: %s", expected, body)
		}
	}
}
