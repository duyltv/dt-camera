package httpapi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateStoragePath(t *testing.T) {
	tempDir := t.TempDir()
	if result := validateStoragePath(tempDir); !result.Valid {
		t.Fatalf("expected temp dir to be valid, got %q", result.Message)
	}

	missingPath := filepath.Join(tempDir, "missing")
	if result := validateStoragePath(missingPath); result.Valid {
		t.Fatalf("expected missing path to be invalid")
	}

	filePath := filepath.Join(tempDir, "file")
	if err := os.WriteFile(filePath, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	if result := validateStoragePath(filePath); result.Valid {
		t.Fatalf("expected file path to be invalid")
	}

	if result := validateStoragePath(""); result.Valid {
		t.Fatalf("expected empty path to be invalid")
	}
}
