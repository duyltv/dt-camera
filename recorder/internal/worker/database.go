package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

func fetchEnabledCameras(ctx context.Context, db *sql.DB) ([]Camera, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			c.id,
			c.name,
			c.rtsp_url,
			c.storage_location_id,
			s.container_path,
			c.retention_days,
			c.max_storage_bytes,
			c.updated_at
		FROM cameras c
		JOIN storage_locations s ON s.id = c.storage_location_id
		WHERE c.is_enabled = TRUE
			AND c.recording_enabled = TRUE
			AND c.storage_location_id IS NOT NULL
			AND s.is_enabled = TRUE
		ORDER BY c.name
	`)
	if err != nil {
		return nil, fmt.Errorf("query enabled cameras: %w", err)
	}
	defer rows.Close()

	var cameras []Camera
	for rows.Next() {
		var camera Camera
		if err := rows.Scan(
			&camera.ID,
			&camera.Name,
			&camera.RTSPURL,
			&camera.StorageLocationID,
			&camera.StoragePath,
			&camera.RetentionDays,
			&camera.MaxStorageBytes,
			&camera.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan camera: %w", err)
		}
		cameras = append(cameras, camera)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cameras: %w", err)
	}

	return cameras, nil
}

func upsertHeartbeat(ctx context.Context, db *sql.DB, workerID, version, status string, activeJobs int) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO recorder_heartbeats (worker_id, version, status, active_job_count, last_seen_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (worker_id)
		DO UPDATE SET
			version = EXCLUDED.version,
			status = EXCLUDED.status,
			active_job_count = EXCLUDED.active_job_count,
			last_seen_at = now()
	`, workerID, version, status, activeJobs)
	if err != nil {
		return fmt.Errorf("upsert heartbeat: %w", err)
	}
	return nil
}

func insertSegment(ctx context.Context, db *sql.DB, segment SegmentMetadata) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO recording_segments (
			camera_id,
			storage_location_id,
			file_path,
			start_time,
			end_time,
			duration_seconds,
			size_bytes,
			format,
			status
		)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9
		WHERE NOT EXISTS (
			SELECT 1 FROM recording_segments WHERE file_path = $3
		)
	`, segment.CameraID, segment.StorageLocationID, segment.FilePath, segment.StartTime, segment.EndTime, segment.DurationSeconds, segment.SizeBytes, segment.Format, segment.Status)
	if err != nil {
		return fmt.Errorf("insert segment metadata: %w", err)
	}
	return nil
}

func insertSystemEvent(ctx context.Context, db *sql.DB, eventType, entityType string, entityID *string, severity, message string, metadata map[string]any) error {
	if severity == "" {
		severity = "info"
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		payload = []byte(`{}`)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO system_events (event_type, entity_type, entity_id, severity, message, metadata)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb)
	`, eventType, entityType, nullableString(entityID), severity, message, string(payload))
	if err != nil {
		return fmt.Errorf("insert system event: %w", err)
	}
	return nil
}

func upsertRecorderJob(ctx context.Context, db *sql.DB, workerID string, camera Camera, status string, startedAt *time.Time, stoppedAt *time.Time, lastError string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO recorder_jobs (camera_id, worker_id, camera_name, status, started_at, stopped_at, last_error)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''))
		ON CONFLICT (camera_id)
		DO UPDATE SET
			worker_id = EXCLUDED.worker_id,
			camera_name = EXCLUDED.camera_name,
			status = EXCLUDED.status,
			started_at = COALESCE(EXCLUDED.started_at, recorder_jobs.started_at),
			stopped_at = EXCLUDED.stopped_at,
			last_error = EXCLUDED.last_error
	`, camera.ID, workerID, camera.Name, status, nullableTime(startedAt), nullableTime(stoppedAt), lastError)
	if err != nil {
		return fmt.Errorf("upsert recorder job: %w", err)
	}
	return nil
}

func nullableString(value *string) any {
	if value == nil || *value == "" {
		return nil
	}
	return *value
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}

func pingDatabase(ctx context.Context, db *sql.DB) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return nil
}
