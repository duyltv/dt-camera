CREATE TABLE IF NOT EXISTS system_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id UUID,
    severity TEXT NOT NULL DEFAULT 'info',
    message TEXT NOT NULL,
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT system_events_severity_check CHECK (severity IN ('debug', 'info', 'warning', 'error'))
);

CREATE INDEX IF NOT EXISTS system_events_created_at_idx
    ON system_events(created_at DESC);

CREATE INDEX IF NOT EXISTS system_events_filter_idx
    ON system_events(event_type, entity_type, entity_id, severity, created_at DESC);

CREATE TABLE IF NOT EXISTS recorder_jobs (
    camera_id UUID PRIMARY KEY REFERENCES cameras(id) ON DELETE CASCADE,
    worker_id TEXT NOT NULL,
    camera_name TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TIMESTAMPTZ,
    stopped_at TIMESTAMPTZ,
    last_error TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DROP TRIGGER IF EXISTS recorder_jobs_set_updated_at ON recorder_jobs;
CREATE TRIGGER recorder_jobs_set_updated_at
    BEFORE UPDATE ON recorder_jobs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS recorder_jobs_status_idx
    ON recorder_jobs(status, updated_at DESC);
