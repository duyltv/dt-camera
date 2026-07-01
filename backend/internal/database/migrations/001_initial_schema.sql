CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'user',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT users_role_check CHECK (role IN ('admin', 'user'))
);

CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS storage_locations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    container_path TEXT NOT NULL,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    health_status TEXT NOT NULL DEFAULT 'unknown',
    last_checked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS cameras (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    storage_location_id UUID REFERENCES storage_locations(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    rtsp_url TEXT NOT NULL,
    location TEXT,
    camera_group TEXT,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    recording_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    recording_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS recording_segments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    camera_id UUID NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    storage_location_id UUID REFERENCES storage_locations(id) ON DELETE SET NULL,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ,
    duration_seconds NUMERIC(12, 3),
    file_path TEXT NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    format TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'recording',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT recording_segments_status_check CHECK (status IN ('recording', 'completed', 'failed', 'deleted')),
    CONSTRAINT recording_segments_time_check CHECK (end_time IS NULL OR end_time >= start_time),
    CONSTRAINT recording_segments_duration_check CHECK (duration_seconds IS NULL OR duration_seconds >= 0),
    CONSTRAINT recording_segments_size_check CHECK (size_bytes >= 0)
);

CREATE TABLE IF NOT EXISTS recorder_heartbeats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    worker_id TEXT NOT NULL UNIQUE,
    version TEXT,
    status TEXT NOT NULL DEFAULT 'unknown',
    active_job_count INTEGER NOT NULL DEFAULT 0,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT recorder_heartbeats_active_job_count_check CHECK (active_job_count >= 0)
);

CREATE TABLE IF NOT EXISTS layouts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS layout_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    layout_id UUID NOT NULL REFERENCES layouts(id) ON DELETE CASCADE,
    camera_id UUID REFERENCES cameras(id) ON DELETE SET NULL,
    position_x INTEGER NOT NULL,
    position_y INTEGER NOT NULL,
    width INTEGER NOT NULL,
    height INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT layout_items_position_check CHECK (position_x >= 0 AND position_y >= 0),
    CONSTRAINT layout_items_size_check CHECK (width > 0 AND height > 0)
);

CREATE TABLE IF NOT EXISTS user_camera_permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    camera_id UUID NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    can_view_live BOOLEAN NOT NULL DEFAULT FALSE,
    can_view_playback BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, camera_id)
);

CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions(user_id);
CREATE INDEX IF NOT EXISTS sessions_expires_at_idx ON sessions(expires_at);

CREATE INDEX IF NOT EXISTS cameras_storage_location_id_idx ON cameras(storage_location_id);
CREATE INDEX IF NOT EXISTS cameras_enabled_idx ON cameras(is_enabled);

CREATE INDEX IF NOT EXISTS recording_segments_camera_start_time_idx
    ON recording_segments(camera_id, start_time);

CREATE INDEX IF NOT EXISTS recording_segments_camera_end_time_idx
    ON recording_segments(camera_id, end_time);

CREATE INDEX IF NOT EXISTS recording_segments_camera_time_range_gist_idx
    ON recording_segments
    USING GIST (camera_id, tstzrange(start_time, COALESCE(end_time, start_time), '[]'));

CREATE INDEX IF NOT EXISTS recording_segments_storage_location_id_idx
    ON recording_segments(storage_location_id);

CREATE INDEX IF NOT EXISTS recorder_heartbeats_last_seen_at_idx
    ON recorder_heartbeats(last_seen_at);

CREATE INDEX IF NOT EXISTS layouts_user_id_idx ON layouts(user_id);
CREATE INDEX IF NOT EXISTS layout_items_layout_id_idx ON layout_items(layout_id);
CREATE INDEX IF NOT EXISTS user_camera_permissions_user_id_idx ON user_camera_permissions(user_id);
CREATE INDEX IF NOT EXISTS user_camera_permissions_camera_id_idx ON user_camera_permissions(camera_id);

DROP TRIGGER IF EXISTS users_set_updated_at ON users;
CREATE TRIGGER users_set_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS sessions_set_updated_at ON sessions;
CREATE TRIGGER sessions_set_updated_at
    BEFORE UPDATE ON sessions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS storage_locations_set_updated_at ON storage_locations;
CREATE TRIGGER storage_locations_set_updated_at
    BEFORE UPDATE ON storage_locations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS cameras_set_updated_at ON cameras;
CREATE TRIGGER cameras_set_updated_at
    BEFORE UPDATE ON cameras
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS recording_segments_set_updated_at ON recording_segments;
CREATE TRIGGER recording_segments_set_updated_at
    BEFORE UPDATE ON recording_segments
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS recorder_heartbeats_set_updated_at ON recorder_heartbeats;
CREATE TRIGGER recorder_heartbeats_set_updated_at
    BEFORE UPDATE ON recorder_heartbeats
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS layouts_set_updated_at ON layouts;
CREATE TRIGGER layouts_set_updated_at
    BEFORE UPDATE ON layouts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS layout_items_set_updated_at ON layout_items;
CREATE TRIGGER layout_items_set_updated_at
    BEFORE UPDATE ON layout_items
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS user_camera_permissions_set_updated_at ON user_camera_permissions;
CREATE TRIGGER user_camera_permissions_set_updated_at
    BEFORE UPDATE ON user_camera_permissions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
