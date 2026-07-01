ALTER TABLE storage_locations
    ADD COLUMN IF NOT EXISTS exists BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS writable BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS total_bytes BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS free_bytes BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS used_bytes BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS used_percent NUMERIC(6, 2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS latest_validation_error TEXT;

ALTER TABLE cameras
    ADD COLUMN IF NOT EXISTS retention_days INTEGER NOT NULL DEFAULT 30,
    ADD COLUMN IF NOT EXISTS max_storage_bytes BIGINT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'cameras_retention_days_check'
    ) THEN
        ALTER TABLE cameras
            ADD CONSTRAINT cameras_retention_days_check CHECK (retention_days > 0);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'cameras_max_storage_bytes_check'
    ) THEN
        ALTER TABLE cameras
            ADD CONSTRAINT cameras_max_storage_bytes_check CHECK (max_storage_bytes IS NULL OR max_storage_bytes > 0);
    END IF;
END;
$$;

CREATE INDEX IF NOT EXISTS recording_segments_retention_idx
    ON recording_segments(camera_id, start_time)
    WHERE status = 'completed';
