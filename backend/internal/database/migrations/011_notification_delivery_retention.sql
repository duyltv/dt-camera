CREATE INDEX IF NOT EXISTS notification_deliveries_created_idx
    ON notification_deliveries(created_at DESC, id DESC);

DELETE FROM notification_deliveries
WHERE id NOT IN (
    SELECT id
    FROM notification_deliveries
    ORDER BY created_at DESC, id DESC
    LIMIT 1000
);
