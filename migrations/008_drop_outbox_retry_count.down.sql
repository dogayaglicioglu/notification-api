-- Re-add retry_count for rollback compatibility
ALTER TABLE outbox
ADD COLUMN IF NOT EXISTS retry_count INTEGER DEFAULT 0;

