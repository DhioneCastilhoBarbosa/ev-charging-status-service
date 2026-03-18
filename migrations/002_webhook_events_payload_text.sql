-- Preserva a ordem das chaves do JSON no webhook (JSONB reordena as chaves).
ALTER TABLE webhook_events
  ALTER COLUMN payload TYPE TEXT USING payload::text;
