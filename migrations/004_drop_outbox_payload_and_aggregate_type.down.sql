-- Recreate dropped fields for rollback compatibility
ALTER TABLE outbox
ADD COLUMN IF NOT EXISTS aggregate_type VARCHAR(100) NOT NULL DEFAULT 'notification',
ADD COLUMN IF NOT EXISTS payload TEXT NOT NULL DEFAULT '{}';

