ALTER TABLE notification_rules
    ADD COLUMN IF NOT EXISTS message_template TEXT NOT NULL DEFAULT 'Motion detected on {{camera_name}}\nTime: {{event_time}}\nScore: {{score}}';
