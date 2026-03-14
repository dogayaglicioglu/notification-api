-- Alter batchId column from INTEGER to VARCHAR(36) for UUID support
ALTER TABLE notifications
ALTER COLUMN batchId TYPE VARCHAR(36);

