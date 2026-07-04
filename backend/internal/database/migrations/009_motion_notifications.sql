ALTER TABLE cameras
    ADD COLUMN IF NOT EXISTS motion_detection_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS motion_sensitivity NUMERIC(5, 3) NOT NULL DEFAULT 0.350,
    ADD COLUMN IF NOT EXISTS motion_min_duration_seconds INTEGER NOT NULL DEFAULT 1;

ALTER TABLE cameras
    DROP CONSTRAINT IF EXISTS cameras_motion_sensitivity_check;
ALTER TABLE cameras
    ADD CONSTRAINT cameras_motion_sensitivity_check
    CHECK (motion_sensitivity >= 0.010 AND motion_sensitivity <= 1.000);

ALTER TABLE cameras
    DROP CONSTRAINT IF EXISTS cameras_motion_min_duration_check;
ALTER TABLE cameras
    ADD CONSTRAINT cameras_motion_min_duration_check
    CHECK (motion_min_duration_seconds >= 0);

CREATE TABLE IF NOT EXISTS motion_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    camera_id UUID NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    recording_segment_id UUID REFERENCES recording_segments(id) ON DELETE SET NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    score NUMERIC(8, 5) NOT NULL DEFAULT 0,
    image_path TEXT,
    video_path TEXT,
    status TEXT NOT NULL DEFAULT 'detected',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT motion_events_status_check CHECK (status IN ('detected', 'notified', 'suppressed', 'failed')),
    CONSTRAINT motion_events_score_check CHECK (score >= 0)
);

CREATE INDEX IF NOT EXISTS motion_events_camera_occurred_idx
    ON motion_events(camera_id, occurred_at DESC);

DROP TRIGGER IF EXISTS motion_events_set_updated_at ON motion_events;
CREATE TRIGGER motion_events_set_updated_at
    BEFORE UPDATE ON motion_events
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS notification_channels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    method TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT notification_channels_method_check CHECK (method IN ('telegram'))
);

CREATE UNIQUE INDEX IF NOT EXISTS notification_channels_name_idx
    ON notification_channels(lower(name));

DROP TRIGGER IF EXISTS notification_channels_set_updated_at ON notification_channels;
CREATE TRIGGER notification_channels_set_updated_at
    BEFORE UPDATE ON notification_channels
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS notification_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    event_type TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    notification_channel_id UUID NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    camera_id UUID REFERENCES cameras(id) ON DELETE CASCADE,
    cooldown_seconds INTEGER NOT NULL DEFAULT 300,
    attach_image BOOLEAN NOT NULL DEFAULT TRUE,
    attach_video BOOLEAN NOT NULL DEFAULT TRUE,
    pre_event_seconds INTEGER NOT NULL DEFAULT 7,
    post_event_seconds INTEGER NOT NULL DEFAULT 3,
    video_fps INTEGER NOT NULL DEFAULT 4,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT notification_rules_event_type_check CHECK (event_type IN ('motion_detected', 'human_detected', 'classification')),
    CONSTRAINT notification_rules_cooldown_check CHECK (cooldown_seconds >= 0),
    CONSTRAINT notification_rules_pre_event_check CHECK (pre_event_seconds >= 0 AND pre_event_seconds <= 120),
    CONSTRAINT notification_rules_post_event_check CHECK (post_event_seconds >= 0 AND post_event_seconds <= 120),
    CONSTRAINT notification_rules_video_fps_check CHECK (video_fps >= 1 AND video_fps <= 15)
);

CREATE UNIQUE INDEX IF NOT EXISTS notification_rules_name_idx
    ON notification_rules(lower(name));

CREATE INDEX IF NOT EXISTS notification_rules_enabled_event_idx
    ON notification_rules(enabled, event_type)
    WHERE enabled = TRUE;

CREATE INDEX IF NOT EXISTS notification_rules_camera_idx
    ON notification_rules(camera_id);

DROP TRIGGER IF EXISTS notification_rules_set_updated_at ON notification_rules;
CREATE TRIGGER notification_rules_set_updated_at
    BEFORE UPDATE ON notification_rules
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS notification_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_rule_id UUID NOT NULL REFERENCES notification_rules(id) ON DELETE CASCADE,
    notification_channel_id UUID NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id UUID,
    camera_id UUID REFERENCES cameras(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    error TEXT,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT notification_deliveries_status_check CHECK (status IN ('pending', 'sent', 'suppressed', 'failed'))
);

CREATE INDEX IF NOT EXISTS notification_deliveries_rule_camera_created_idx
    ON notification_deliveries(notification_rule_id, camera_id, created_at DESC);

DROP TRIGGER IF EXISTS notification_deliveries_set_updated_at ON notification_deliveries;
CREATE TRIGGER notification_deliveries_set_updated_at
    BEFORE UPDATE ON notification_deliveries
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
