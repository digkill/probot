-- +goose Up
ALTER TABLE ai_agents DROP CONSTRAINT IF EXISTS ai_agents_role_check;
ALTER TABLE ai_agents ADD CONSTRAINT ai_agents_role_check
    CHECK (role IN ('content', 'platform_adapt', 'image', 'analytics_advisor', 'reputation', 'research'));

ALTER TABLE crawl_sources DROP CONSTRAINT IF EXISTS crawl_sources_kind_check;
ALTER TABLE crawl_sources ADD CONSTRAINT crawl_sources_kind_check
    CHECK (kind IN ('rss', 'sitemap', 'html', 'api', 'discovery', 'news', 'hn', 'reddit_search', 'reviews', 'ai_search'));

-- +goose Down
ALTER TABLE crawl_sources DROP CONSTRAINT IF EXISTS crawl_sources_kind_check;
ALTER TABLE crawl_sources ADD CONSTRAINT crawl_sources_kind_check
    CHECK (kind IN ('rss', 'sitemap', 'html', 'api', 'discovery', 'news', 'hn', 'reddit_search', 'reviews'));
ALTER TABLE ai_agents DROP CONSTRAINT IF EXISTS ai_agents_role_check;
ALTER TABLE ai_agents ADD CONSTRAINT ai_agents_role_check
    CHECK (role IN ('content', 'platform_adapt', 'image', 'analytics_advisor', 'reputation'));
