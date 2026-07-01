package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeEventForInsertDefaultsMetadataAndSeverity(t *testing.T) {
	normalized, payload := normalizeEventForInsert(eventRecord{
		EventType:  "camera.create",
		EntityType: "camera",
		Message:    "camera created",
	})

	if normalized.Severity != "info" {
		t.Fatalf("severity = %q, want info", normalized.Severity)
	}
	if payload != "{}" {
		t.Fatalf("payload = %q, want {}", payload)
	}
	if strings.Contains(payload, "token") || strings.Contains(payload, "rtsp://") {
		t.Fatalf("event payload should not contain sensitive data: %s", payload)
	}
}

func TestParseEventFilter(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/events?event_type=camera.create&entity_type=camera&entity_id=018f5d67-89ab-4def-8123-456789abcdef&severity=info&start_time=2026-07-01T00:00:00Z&end_time=2026-07-01T01:00:00Z&limit=25", nil)

	filter, err := parseEventFilter(req)
	if err != nil {
		t.Fatalf("parseEventFilter() error = %v", err)
	}
	if filter.EventType != "camera.create" || filter.EntityType != "camera" || filter.EntityID == "" || filter.Severity != "info" {
		t.Fatalf("unexpected filter: %+v", filter)
	}
	if filter.StartTime == nil || filter.EndTime == nil {
		t.Fatalf("expected start and end times")
	}
	if filter.Limit != 25 {
		t.Fatalf("limit = %d, want 25", filter.Limit)
	}
}

func TestParseEventFilterRejectsInvalidEntityID(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/events?entity_id=not-a-uuid", nil)
	if _, err := parseEventFilter(req); err == nil {
		t.Fatalf("expected invalid entity_id to be rejected")
	}
}
