-- +goose Up
CREATE TABLE short_links (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    code            TEXT NOT NULL UNIQUE,
    target_url      TEXT NOT NULL,
    publication_id  UUID REFERENCES publications(id) ON DELETE SET NULL,
    campaign_id     UUID REFERENCES campaigns(id) ON DELETE SET NULL,
    brand_id        UUID REFERENCES brands(id) ON DELETE SET NULL,
    label           TEXT NOT NULL DEFAULT '',
    clicks          BIGINT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_short_links_workspace ON short_links (workspace_id, created_at DESC);

CREATE TABLE short_link_clicks (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    short_link_id UUID NOT NULL REFERENCES short_links(id) ON DELETE CASCADE,
    referer       TEXT NOT NULL DEFAULT '',
    user_agent    TEXT NOT NULL DEFAULT '',
    ip_hash       TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_short_link_clicks_link ON short_link_clicks (short_link_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS short_link_clicks;
DROP TABLE IF EXISTS short_links;
