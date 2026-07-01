CREATE TABLE IF NOT EXISTS alert_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    severity TEXT NOT NULL DEFAULT 'warning',
    threshold JSONB NOT NULL DEFAULT '{}'::jsonb,
    cooldown_seconds INTEGER NOT NULL DEFAULT 300,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT alert_rules_type_check CHECK (type IN (
        'recorder_stale',
        'camera_recording_failed',
        'storage_low_disk',
        'live_stream_failed'
    )),
    CONSTRAINT alert_rules_severity_check CHECK (severity IN ('debug', 'info', 'warning', 'error')),
    CONSTRAINT alert_rules_cooldown_check CHECK (cooldown_seconds >= 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS alert_rules_name_idx ON alert_rules (lower(name));

CREATE INDEX IF NOT EXISTS alert_rules_enabled_type_idx
    ON alert_rules (enabled, type)
    WHERE enabled = TRUE;

DROP TRIGGER IF EXISTS alert_rules_set_updated_at ON alert_rules;
CREATE TRIGGER alert_rules_set_updated_at
    BEFORE UPDATE ON alert_rules
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_rule_id UUID NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id UUID,
    severity TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open',
    message TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    opened_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    acknowledged_at TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,
    CONSTRAINT alerts_severity_check CHECK (severity IN ('debug', 'info', 'warning', 'error')),
    CONSTRAINT alerts_status_check CHECK (status IN ('open', 'acknowledged', 'resolved')),
    CONSTRAINT alerts_acknowledged_at_check CHECK (acknowledged_at IS NULL OR acknowledged_at >= opened_at),
    CONSTRAINT alerts_resolved_at_check CHECK (resolved_at IS NULL OR resolved_at >= opened_at)
);

CREATE INDEX IF NOT EXISTS alerts_status_opened_at_idx
    ON alerts (status, opened_at DESC);

CREATE INDEX IF NOT EXISTS alerts_rule_entity_idx
    ON alerts (alert_rule_id, entity_type, entity_id, status);

-- A given rule+entity may have at most one OPEN alert at a time.
CREATE UNIQUE INDEX IF NOT EXISTS alerts_open_unique_idx
    ON alerts (alert_rule_id, entity_type, COALESCE(entity_id, '00000000-0000-0000-0000-000000000000'::uuid))
    WHERE status = 'open' AND entity_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS alerts_open_no_entity_unique_idx
    ON alerts (alert_rule_id, entity_type)
    WHERE status = 'open' AND entity_id IS NULL;

DROP TRIGGER IF EXISTS alerts_set_updated_at ON alerts;
CREATE TRIGGER alerts_set_updated_at
    BEFORE UPDATE ON alerts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();