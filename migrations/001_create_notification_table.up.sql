-- Create notification table
CREATE TABLE IF NOT EXISTS notifications (
    id SERIAL PRIMARY KEY,
    recipient VARCHAR(255) NOT NULL,
    channel VARCHAR(50) NOT NULL,
    content TEXT NOT NULL,
    priority INTEGER NOT NULL,
    status INTEGER NOT NULL,
    date TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    batchId VARCHAR(36)
);

-- Create index on recipient for faster queries
CREATE INDEX IF NOT EXISTS idx_notifications_recipient ON notifications(recipient);

-- Create index on date for sorting
CREATE INDEX IF NOT EXISTS idx_notifications_date ON notifications(date DESC);

-- Create index on status for filtering
CREATE INDEX IF NOT EXISTS idx_notifications_status ON notifications(status);

-- Create index on batchId for grouping
CREATE INDEX IF NOT EXISTS idx_notifications_batchId ON notifications(batchId);

