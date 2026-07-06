CREATE TABLE IF NOT EXISTS identities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'person',
    known BOOLEAN NOT NULL DEFAULT TRUE,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS identity_reference_images (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    identity_id UUID NOT NULL REFERENCES identities(id) ON DELETE CASCADE,
    image_path TEXT NOT NULL,
    face_crop_path TEXT,
    embedding_json JSONB,
    quality_score DOUBLE PRECISION,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS observations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    camera_id UUID NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    event_id UUID,
    observed_at TIMESTAMPTZ NOT NULL,
    observation_type TEXT NOT NULL,
    bbox_json JSONB,
    confidence DOUBLE PRECISION,
    frame_path TEXT,
    crop_path TEXT,
    attributes_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    embedding_json JSONB,
    track_id UUID,
    identity_id UUID REFERENCES identities(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE observations
    ADD COLUMN IF NOT EXISTS camera_id UUID,
    ADD COLUMN IF NOT EXISTS event_id UUID,
    ADD COLUMN IF NOT EXISTS observed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS observation_type TEXT,
    ADD COLUMN IF NOT EXISTS bbox_json JSONB,
    ADD COLUMN IF NOT EXISTS confidence DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS frame_path TEXT,
    ADD COLUMN IF NOT EXISTS crop_path TEXT,
    ADD COLUMN IF NOT EXISTS attributes_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS embedding_json JSONB,
    ADD COLUMN IF NOT EXISTS track_id UUID,
    ADD COLUMN IF NOT EXISTS identity_id UUID,
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE observations
    ALTER COLUMN attributes_json SET DEFAULT '{}'::jsonb,
    ALTER COLUMN created_at SET DEFAULT now();

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'observations_camera_id_fkey'
    ) THEN
        ALTER TABLE observations
            ADD CONSTRAINT observations_camera_id_fkey
            FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'observations_identity_id_fkey'
    ) THEN
        ALTER TABLE observations
            ADD CONSTRAINT observations_identity_id_fkey
            FOREIGN KEY (identity_id) REFERENCES identities(id) ON DELETE SET NULL;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS identity_match_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    observation_id UUID NOT NULL REFERENCES observations(id) ON DELETE CASCADE,
    identity_id UUID REFERENCES identities(id) ON DELETE SET NULL,
    method TEXT NOT NULL,
    similarity_score DOUBLE PRECISION,
    threshold DOUBLE PRECISION,
    decision TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ai_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    camera_id UUID NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    source_event_id UUID,
    job_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    priority INT NOT NULL DEFAULT 100,
    frame_path TEXT,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    attempts INT NOT NULL DEFAULT 0,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS observations_camera_observed_idx
    ON observations(camera_id, observed_at DESC);

CREATE INDEX IF NOT EXISTS observations_type_observed_idx
    ON observations(observation_type, observed_at DESC);

CREATE INDEX IF NOT EXISTS observations_identity_observed_idx
    ON observations(identity_id, observed_at DESC);

CREATE INDEX IF NOT EXISTS identity_reference_images_identity_idx
    ON identity_reference_images(identity_id);

CREATE INDEX IF NOT EXISTS identity_match_attempts_observation_idx
    ON identity_match_attempts(observation_id);

CREATE INDEX IF NOT EXISTS ai_jobs_status_priority_created_idx
    ON ai_jobs(status, priority, created_at);

ALTER TABLE notification_rules
    DROP CONSTRAINT IF EXISTS notification_rules_event_type_check;

ALTER TABLE notification_rules
    ADD CONSTRAINT notification_rules_event_type_check
    CHECK (event_type IN (
        'motion_detected',
        'human_detected',
        'person_detected',
        'person_recognized',
        'unknown_person_detected',
        'classification',
        'observation_created'
    ));

DROP TRIGGER IF EXISTS identities_set_updated_at ON identities;
CREATE TRIGGER identities_set_updated_at
    BEFORE UPDATE ON identities
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
