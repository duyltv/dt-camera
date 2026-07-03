ALTER TABLE cameras
    ADD COLUMN IF NOT EXISTS record_audio BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS stream_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS stream_audio BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS cameras_stream_enabled_idx
    ON cameras(is_enabled, stream_enabled)
    WHERE is_enabled = TRUE AND stream_enabled = TRUE;
