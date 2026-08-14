-- +goose Up
ALTER TABLE mentions
    ADD COLUMN IF NOT EXISTS sentiment TEXT NOT NULL DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS severity TEXT NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS watch_query TEXT NOT NULL DEFAULT '';

ALTER TABLE mentions DROP CONSTRAINT IF EXISTS mentions_status_check;
ALTER TABLE mentions ADD CONSTRAINT mentions_status_check
    CHECK (status IN ('new', 'reviewed', 'replied', 'ignored', 'escalated'));

ALTER TABLE crawl_sources
    ADD COLUMN IF NOT EXISTS brand_id UUID REFERENCES brands(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS purpose TEXT NOT NULL DEFAULT 'mentions';

ALTER TABLE crawl_sources DROP CONSTRAINT IF EXISTS crawl_sources_kind_check;
ALTER TABLE crawl_sources ADD CONSTRAINT crawl_sources_kind_check
    CHECK (kind IN ('rss', 'sitemap', 'html', 'api', 'discovery', 'news', 'hn', 'reddit_search', 'reviews'));

CREATE INDEX IF NOT EXISTS idx_mentions_sentiment ON mentions (workspace_id, sentiment, found_at DESC);
CREATE INDEX IF NOT EXISTS idx_crawl_sources_due ON crawl_sources (enabled, last_run_at);

-- +goose Down
DROP INDEX IF EXISTS idx_crawl_sources_due;
DROP INDEX IF EXISTS idx_mentions_sentiment;
ALTER TABLE crawl_sources DROP CONSTRAINT IF EXISTS crawl_sources_kind_check;
ALTER TABLE crawl_sources ADD CONSTRAINT crawl_sources_kind_check
    CHECK (kind IN ('rss', 'sitemap', 'html', 'api', 'discovery'));
ALTER TABLE crawl_sources DROP COLUMN IF EXISTS purpose;
ALTER TABLE crawl_sources DROP COLUMN IF EXISTS brand_id;
ALTER TABLE mentions DROP CONSTRAINT IF EXISTS mentions_status_check;
ALTER TABLE mentions ADD CONSTRAINT mentions_status_check
    CHECK (status IN ('new', 'reviewed', 'replied', 'ignored'));
ALTER TABLE mentions DROP COLUMN IF EXISTS watch_query;
ALTER TABLE mentions DROP COLUMN IF EXISTS severity;
ALTER TABLE mentions DROP COLUMN IF EXISTS sentiment;
