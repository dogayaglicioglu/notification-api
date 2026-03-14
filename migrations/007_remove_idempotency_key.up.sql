DROP INDEX IF EXISTS notifications_idempotency_key_idx;
ALTER TABLE notifications DROP COLUMN IF EXISTS idempotency_key;

