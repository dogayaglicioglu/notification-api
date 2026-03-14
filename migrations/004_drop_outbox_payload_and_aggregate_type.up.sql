-- Remove redundant outbox fields for ID-based event processing
ALTER TABLE outbox
DROP COLUMN IF EXISTS aggregate_type,
DROP COLUMN IF EXISTS payload;

