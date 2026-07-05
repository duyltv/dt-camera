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
			c.record_audio,
			c.motion_detection_enabled,
			c.motion_sensitivity,
			c.motion_min_duration_seconds,
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
			&camera.RecordAudio,
			&camera.MotionDetectionEnabled,
			&camera.MotionSensitivity,
			&camera.MotionMinDurationSeconds,
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
	_, err := insertSegmentWithID(ctx, db, segment)
	return err
}

func insertSegmentWithID(ctx context.Context, db *sql.DB, segment SegmentMetadata) (string, error) {
	var id string
	err := db.QueryRowContext(ctx, `
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
		RETURNING id
	`, segment.CameraID, segment.StorageLocationID, segment.FilePath, segment.StartTime, segment.EndTime, segment.DurationSeconds, segment.SizeBytes, segment.Format, segment.Status).Scan(&id)
	if err == sql.ErrNoRows {
		err = db.QueryRowContext(ctx, `SELECT id FROM recording_segments WHERE file_path = $1`, segment.FilePath).Scan(&id)
	}
	if err != nil {
		return "", fmt.Errorf("insert segment metadata: %w", err)
	}
	return id, nil
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

func insertMotionEvent(ctx context.Context, db *sql.DB, event MotionEvent) (string, error) {
	return insertMotionEventWithMetadata(ctx, db, event, nil)
}

func insertMotionEventWithMetadata(ctx context.Context, db *sql.DB, event MotionEvent, metadata map[string]any) (string, error) {
	var id string
	if metadata == nil {
		metadata = map[string]any{}
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		payload = []byte(`{}`)
	}
	_, err = db.ExecContext(ctx, `
		UPDATE motion_events
		SET status = 'suppressed'
		WHERE camera_id = $1
			AND ($2::uuid IS NULL OR recording_segment_id = $2::uuid)
			AND abs(extract(epoch from (occurred_at - $3::timestamptz))) < 1
	`, event.CameraID, nullableString(&event.RecordingSegmentID), event.OccurredAt)
	if err != nil {
		return "", fmt.Errorf("dedupe motion event: %w", err)
	}
	err = db.QueryRowContext(ctx, `
		INSERT INTO motion_events (
			camera_id, recording_segment_id, occurred_at, score, image_path, video_path, status, metadata
		)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), 'detected', $7::jsonb)
		RETURNING id
	`, event.CameraID, nullableString(&event.RecordingSegmentID), event.OccurredAt, event.Score, event.ImagePath, event.VideoPath, string(payload)).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert motion event: %w", err)
	}
	return id, nil
}

func updateMotionEventEvidence(ctx context.Context, db *sql.DB, id, imagePath, videoPath string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE motion_events
		SET image_path = NULLIF($2, ''), video_path = NULLIF($3, '')
		WHERE id = $1
	`, id, imagePath, videoPath)
	if err != nil {
		return fmt.Errorf("update motion event evidence: %w", err)
	}
	return nil
}

func fetchNotificationRules(ctx context.Context, db *sql.DB, eventType, cameraID string) ([]NotificationRule, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			r.id,
			r.name,
			r.event_type,
			r.notification_channel_id,
			r.cooldown_seconds,
			r.message_template,
			r.attach_image,
			r.attach_video,
			r.pre_event_seconds,
			r.post_event_seconds,
			r.video_fps,
			ch.id,
			ch.name,
			ch.method,
			ch.enabled,
			ch.config
		FROM notification_rules r
		JOIN notification_channels ch ON ch.id = r.notification_channel_id
		WHERE r.enabled = TRUE
			AND ch.enabled = TRUE
			AND r.event_type = $1
			AND (r.camera_id IS NULL OR r.camera_id = $2)
		ORDER BY r.name
	`, eventType, cameraID)
	if err != nil {
		return nil, fmt.Errorf("query notification rules: %w", err)
	}
	defer rows.Close()

	var rules []NotificationRule
	for rows.Next() {
		var rule NotificationRule
		var rawConfig []byte
		if err := rows.Scan(
			&rule.ID,
			&rule.Name,
			&rule.EventType,
			&rule.NotificationChannelID,
			&rule.CooldownSeconds,
			&rule.MessageTemplate,
			&rule.AttachImage,
			&rule.AttachVideo,
			&rule.PreEventSeconds,
			&rule.PostEventSeconds,
			&rule.VideoFPS,
			&rule.Channel.ID,
			&rule.Channel.Name,
			&rule.Channel.Method,
			&rule.Channel.Enabled,
			&rawConfig,
		); err != nil {
			return nil, fmt.Errorf("scan notification rule: %w", err)
		}
		rule.Channel.Config = parseStringConfig(rawConfig)
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notification rules: %w", err)
	}
	return rules, nil
}

func shouldSendNotification(ctx context.Context, db *sql.DB, rule NotificationRule, cameraID string, now time.Time) (bool, error) {
	if rule.CooldownSeconds <= 0 {
		return true, nil
	}
	var last sql.NullTime
	if err := db.QueryRowContext(ctx, `
		SELECT max(created_at)
		FROM notification_deliveries
		WHERE notification_rule_id = $1
			AND camera_id = $2
			AND status IN ('sent', 'pending')
	`, rule.ID, cameraID).Scan(&last); err != nil {
		return false, fmt.Errorf("query notification cooldown: %w", err)
	}
	if !last.Valid {
		return true, nil
	}
	return now.Sub(last.Time) >= time.Duration(rule.CooldownSeconds)*time.Second, nil
}

func insertNotificationDelivery(ctx context.Context, db *sql.DB, rule NotificationRule, eventType, entityType, entityID, cameraID, status, errText string, sentAt *time.Time) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO notification_deliveries (
			notification_rule_id, notification_channel_id, event_type, entity_type,
			entity_id, camera_id, status, error, sent_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9)
	`, rule.ID, rule.NotificationChannelID, eventType, entityType, nullableString(&entityID), nullableString(&cameraID), status, errText, nullableTime(sentAt))
	if err != nil {
		return fmt.Errorf("insert notification delivery: %w", err)
	}
	return nil
}

func updateMotionEventStatus(ctx context.Context, db *sql.DB, id, status string) error {
	_, err := db.ExecContext(ctx, `UPDATE motion_events SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("update motion event status: %w", err)
	}
	return nil
}

func parseStringConfig(raw []byte) map[string]string {
	values := map[string]any{}
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}
	if err := json.Unmarshal(raw, &values); err != nil {
		return out
	}
	for key, value := range values {
		if s, ok := value.(string); ok {
			out[key] = s
		}
	}
	return out
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
