ALTER TABLE layouts
    ADD COLUMN IF NOT EXISTS settings JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE layout_items
    ADD COLUMN IF NOT EXISTS display_order INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS tile_type TEXT NOT NULL DEFAULT 'custom';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'layout_items_tile_type_check'
    ) THEN
        ALTER TABLE layout_items
            ADD CONSTRAINT layout_items_tile_type_check
            CHECK (tile_type IN ('small', 'large', 'portrait', 'landscape', 'custom'));
    END IF;
END;
$$;

CREATE UNIQUE INDEX IF NOT EXISTS layouts_global_default_unique_idx
    ON layouts (is_default)
    WHERE is_default = TRUE AND user_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS layout_items_unique_camera_idx
    ON layout_items (layout_id, camera_id)
    WHERE camera_id IS NOT NULL;
