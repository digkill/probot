-- +goose Up
ALTER TABLE ai_agents DROP CONSTRAINT IF EXISTS ai_agents_role_check;
ALTER TABLE ai_agents ADD CONSTRAINT ai_agents_role_check
    CHECK (role IN ('content', 'platform_adapt', 'image', 'analytics_advisor', 'reputation'));

CREATE INDEX IF NOT EXISTS idx_mentions_inbox
    ON mentions (workspace_id, sentiment, status, found_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_mentions_inbox;
ALTER TABLE ai_agents DROP CONSTRAINT IF EXISTS ai_agents_role_check;
ALTER TABLE ai_agents ADD CONSTRAINT ai_agents_role_check
    CHECK (role IN ('content', 'platform_adapt', 'image', 'analytics_advisor'));
