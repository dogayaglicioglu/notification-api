-- Drop retry_count from outbox; retries are tracked by worker logic
ALTER TABLE outbox
DROP COLUMN IF EXISTS retry_count;

