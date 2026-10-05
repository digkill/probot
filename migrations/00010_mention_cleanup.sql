-- +goose Up
-- Deleted mentions stay as tombstones so crawlers do not re-add the same link.
ALTER TABLE mentions ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

ALTER TABLE mentions DROP CONSTRAINT IF EXISTS mentions_status_check;
ALTER TABLE mentions ADD CONSTRAINT mentions_status_check
    CHECK (status IN ('new', 'reviewed', 'replied', 'ignored', 'escalated', 'false_positive'));

CREATE INDEX IF NOT EXISTS idx_mentions_suppressed
    ON mentions (workspace_id, url)
    WHERE deleted_at IS NOT NULL OR status = 'false_positive';

-- +goose Down
DROP INDEX IF EXISTS idx_mentions_suppressed;
DELETE FROM mentions WHERE deleted_at IS NOT NULL;
UPDATE mentions SET status = 'ignored' WHERE status = 'false_positive';
ALTER TABLE mentions DROP CONSTRAINT IF EXISTS mentions_status_check;
ALTER TABLE mentions ADD CONSTRAINT mentions_status_check
    CHECK (status IN ('new', 'reviewed', 'replied', 'ignored', 'escalated'));
ALTER TABLE mentions DROP COLUMN IF EXISTS deleted_at;
