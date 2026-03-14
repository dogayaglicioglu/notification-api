ALTER TABLE notifications ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(255);
CREATE UNIQUE INDEX IF NOT EXISTS notifications_idempotency_key_idx
    ON notifications (idempotency_key)
    WHERE idempotency_key IS NOT NULL;

