ALTER TABLE storage_locations
    ADD COLUMN IF NOT EXISTS max_storage_bytes BIGINT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'storage_locations_max_storage_bytes_check'
    ) THEN
        ALTER TABLE storage_locations
            ADD CONSTRAINT storage_locations_max_storage_bytes_check CHECK (max_storage_bytes IS NULL OR max_storage_bytes > 0);
    END IF;
END;
$$;
