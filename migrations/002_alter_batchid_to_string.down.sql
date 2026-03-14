-- Rollback: Alter batchId column back to INTEGER
ALTER TABLE notifications
ALTER COLUMN batchId TYPE INTEGER;

