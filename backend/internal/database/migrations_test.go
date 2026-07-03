package database

import (
	"strings"
	"testing"
)

func TestLoadMigrations(t *testing.T) {
	migrations, err := LoadMigrations()
	if err != nil {
		t.Fatalf("LoadMigrations() error = %v", err)
	}

	if len(migrations) < 6 {
		t.Fatalf("expected at least 6 migrations, got %d", len(migrations))
	}

	if migrations[0].Version != 1 {
		t.Fatalf("expected migration version 1, got %d", migrations[0].Version)
	}

	if migrations[0].Name != "001_initial_schema.sql" {
		t.Fatalf("unexpected migration name %q", migrations[0].Name)
	}
	if migrations[1].Version != 2 {
		t.Fatalf("expected migration version 2, got %d", migrations[1].Version)
	}
	if migrations[1].Name != "002_layout_item_fields.sql" {
		t.Fatalf("unexpected migration name %q", migrations[1].Name)
	}
	if migrations[2].Version != 3 {
		t.Fatalf("expected migration version 3, got %d", migrations[2].Version)
	}
	if migrations[2].Name != "003_auth_fields.sql" {
		t.Fatalf("unexpected migration name %q", migrations[2].Name)
	}
	if migrations[3].Version != 4 {
		t.Fatalf("expected migration version 4, got %d", migrations[3].Version)
	}
	if migrations[3].Name != "004_retention_storage_health.sql" {
		t.Fatalf("unexpected migration name %q", migrations[3].Name)
	}
	if migrations[4].Version != 5 {
		t.Fatalf("expected migration version 5, got %d", migrations[4].Version)
	}
	if migrations[4].Name != "005_system_events_recorder_jobs.sql" {
		t.Fatalf("unexpected migration name %q", migrations[4].Name)
	}
}

func TestSystemEventsMigrationAddsObservabilityTables(t *testing.T) {
	migrations, err := LoadMigrations()
	if err != nil {
		t.Fatalf("LoadMigrations() error = %v", err)
	}

	var eventMigration string
	for _, migration := range migrations {
		if migration.Name == "005_system_events_recorder_jobs.sql" {
			eventMigration = migration.SQL
			break
		}
	}

	for _, expected := range []string{"system_events", "recorder_jobs", "system_events_filter_idx", "recorder_jobs_status_idx"} {
		if !strings.Contains(eventMigration, expected) {
			t.Fatalf("event migration should include %s", expected)
		}
	}
}

func TestLayoutMigrationEnforcesGlobalDefaultUniqueness(t *testing.T) {
	migrations, err := LoadMigrations()
	if err != nil {
		t.Fatalf("LoadMigrations() error = %v", err)
	}

	var layoutMigration string
	for _, migration := range migrations {
		if migration.Name == "002_layout_item_fields.sql" {
			layoutMigration = migration.SQL
			break
		}
	}

	if !strings.Contains(layoutMigration, "layouts_global_default_unique_idx") {
		t.Fatalf("layout migration should include global default uniqueness index")
	}
	if !strings.Contains(layoutMigration, "WHERE is_default = TRUE AND user_id IS NULL") {
		t.Fatalf("layout migration should scope default uniqueness to global layouts")
	}
}

func TestRetentionStorageHealthMigrationAddsPhase11Columns(t *testing.T) {
	migrations, err := LoadMigrations()
	if err != nil {
		t.Fatalf("LoadMigrations() error = %v", err)
	}

	var retentionMigration string
	for _, migration := range migrations {
		if migration.Name == "004_retention_storage_health.sql" {
			retentionMigration = migration.SQL
			break
		}
	}

	for _, expected := range []string{"latest_validation_error", "retention_days", "max_storage_bytes", "recording_segments_retention_idx"} {
		if !strings.Contains(retentionMigration, expected) {
			t.Fatalf("retention migration should include %s", expected)
		}
	}
}

func TestCameraMediaFlagsMigrationAddsAudioAndStreamControls(t *testing.T) {
	migrations, err := LoadMigrations()
	if err != nil {
		t.Fatalf("LoadMigrations() error = %v", err)
	}

	var mediaMigration string
	for _, migration := range migrations {
		if migration.Name == "007_camera_media_flags.sql" {
			mediaMigration = migration.SQL
			break
		}
	}

	for _, expected := range []string{"record_audio", "stream_enabled", "stream_audio", "cameras_stream_enabled_idx"} {
		if !strings.Contains(mediaMigration, expected) {
			t.Fatalf("media migration should include %s", expected)
		}
	}
}
