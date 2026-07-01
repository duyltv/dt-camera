package worker

import (
	"strings"
	"testing"
)

func TestSanitizeLogRedactsRTSPURLs(t *testing.T) {
	input := "tcp://192.168.1.57:554 failed; error opening rtsp://user:secret@192.168.1.57:554/cam/realmonitor?channel=1"
	output := sanitizeLog(input)

	if strings.Contains(output, "secret") || strings.Contains(output, "192.168.1.57") {
		t.Fatalf("sanitizeLog leaked sensitive RTSP content: %s", output)
	}
	if !strings.Contains(output, "rtsp://<redacted>") {
		t.Fatalf("sanitizeLog did not include redaction marker: %s", output)
	}
	if !strings.Contains(output, "tcp://<redacted>") {
		t.Fatalf("sanitizeLog did not redact derived tcp URL: %s", output)
	}
}
