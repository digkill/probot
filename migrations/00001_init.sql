-- +goose Up
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE workspaces (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE workspace_members (
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role         TEXT NOT NULL CHECK (role IN ('owner', 'editor', 'viewer')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id)
);

CREATE TABLE brands (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    slug         TEXT NOT NULL,
    canonical_url TEXT NOT NULL DEFAULT '',
    tone_of_voice TEXT NOT NULL DEFAULT '',
    forbidden_words TEXT[] NOT NULL DEFAULT '{}',
    cta_default  TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, slug)
);

CREATE TABLE platform_definitions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('builtin', 'atproto', 'mastodon_like', 'webhook', 'manual', 'rss_only')),
    base_url    TEXT NOT NULL DEFAULT '',
    publish_mode TEXT NOT NULL DEFAULT 'api' CHECK (publish_mode IN ('api', 'webhook', 'manual', 'crawl_only')),
    char_limit  INT,
    tags        TEXT[] NOT NULL DEFAULT '{}',
    audience_fit REAL NOT NULL DEFAULT 0.5,
    effort      TEXT NOT NULL DEFAULT 'medium',
    risk        TEXT NOT NULL DEFAULT 'medium',
    lang        TEXT NOT NULL DEFAULT 'en',
    region      TEXT NOT NULL DEFAULT 'global',
    status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'pending_review', 'disabled')),
    meta        JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE custom_platforms (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    slug         TEXT NOT NULL,
    publish_mode TEXT NOT NULL CHECK (publish_mode IN ('webhook', 'manual', 'http_form')),
    webhook_url  TEXT NOT NULL DEFAULT '',
    http_template JSONB NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, slug)
);

CREATE TABLE channels (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    brand_id      UUID NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    platform_def_id UUID REFERENCES platform_definitions(id) ON DELETE SET NULL,
    custom_platform_id UUID REFERENCES custom_platforms(id) ON DELETE SET NULL,
    name          TEXT NOT NULL,
    external_ref  TEXT NOT NULL DEFAULT '',
    credentials   BYTEA,
    health        TEXT NOT NULL DEFAULT 'ok' CHECK (health IN ('ok', 'warn', 'paused')),
    meta          JSONB NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE campaigns (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    brand_id     UUID NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    playbook     TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'done', 'archived')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE content_pieces (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    brand_id     UUID NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    campaign_id  UUID REFERENCES campaigns(id) ON DELETE SET NULL,
    title        TEXT NOT NULL DEFAULT '',
    body         TEXT NOT NULL DEFAULT '',
    cta_url      TEXT NOT NULL DEFAULT '',
    media_urls   TEXT[] NOT NULL DEFAULT '{}',
    utm_source   TEXT NOT NULL DEFAULT '',
    utm_medium   TEXT NOT NULL DEFAULT '',
    utm_campaign TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'ready', 'archived')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE publications (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    content_id      UUID NOT NULL REFERENCES content_pieces(id) ON DELETE CASCADE,
    channel_id      UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    campaign_id     UUID REFERENCES campaigns(id) ON DELETE SET NULL,
    status          TEXT NOT NULL DEFAULT 'draft' CHECK (status IN (
        'draft', 'scheduled', 'publishing', 'live', 'failed', 'needs_manual_confirm'
    )),
    body_override   TEXT NOT NULL DEFAULT '',
    scheduled_at    TIMESTAMPTZ,
    published_at    TIMESTAMPTZ,
    external_id     TEXT NOT NULL DEFAULT '',
    external_url    TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL,
    error_message   TEXT NOT NULL DEFAULT '',
    sort_order      INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (idempotency_key)
);

CREATE TABLE cross_links (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    campaign_id     UUID REFERENCES campaigns(id) ON DELETE CASCADE,
    from_publication_id UUID REFERENCES publications(id) ON DELETE CASCADE,
    to_publication_id   UUID REFERENCES publications(id) ON DELETE SET NULL,
    to_url          TEXT NOT NULL DEFAULT '',
    kind            TEXT NOT NULL DEFAULT 'cross_post' CHECK (kind IN ('cross_post', 'canonical', 'mention')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE metric_snapshots (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    publication_id UUID NOT NULL REFERENCES publications(id) ON DELETE CASCADE,
    reach          BIGINT NOT NULL DEFAULT 0,
    likes          BIGINT NOT NULL DEFAULT 0,
    comments       BIGINT NOT NULL DEFAULT 0,
    shares         BIGINT NOT NULL DEFAULT 0,
    clicks         BIGINT NOT NULL DEFAULT 0,
    raw            JSONB NOT NULL DEFAULT '{}',
    captured_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_metric_snapshots_pub_time ON metric_snapshots (publication_id, captured_at DESC);

CREATE TABLE mentions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    brand_id     UUID REFERENCES brands(id) ON DELETE SET NULL,
    campaign_id  UUID REFERENCES campaigns(id) ON DELETE SET NULL,
    source       TEXT NOT NULL DEFAULT '',
    url          TEXT NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    snippet      TEXT NOT NULL DEFAULT '',
    author       TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'reviewed', 'replied', 'ignored')),
    hash         TEXT NOT NULL,
    found_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, hash)
);

CREATE TABLE crawl_sources (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL CHECK (kind IN ('rss', 'sitemap', 'html', 'api', 'discovery')),
    url          TEXT NOT NULL,
    query        TEXT NOT NULL DEFAULT '',
    interval_sec INT NOT NULL DEFAULT 3600,
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    last_run_at  TIMESTAMPTZ,
    meta         JSONB NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE crawl_jobs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id    UUID REFERENCES crawl_sources(id) ON DELETE SET NULL,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'done', 'failed')),
    error_message TEXT NOT NULL DEFAULT '',
    started_at   TIMESTAMPTZ,
    finished_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE ai_agents (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    role         TEXT NOT NULL CHECK (role IN ('content', 'platform_adapt', 'image', 'analytics_advisor')),
    provider     TEXT NOT NULL DEFAULT 'openai',
    model        TEXT NOT NULL DEFAULT '',
    base_url     TEXT NOT NULL DEFAULT '',
    api_key_env  TEXT NOT NULL DEFAULT 'OPENAI_API_KEY',
    system_prompt TEXT NOT NULL DEFAULT '',
    params       JSONB NOT NULL DEFAULT '{}',
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE ai_runs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    agent_id     UUID NOT NULL REFERENCES ai_agents(id) ON DELETE CASCADE,
    input        JSONB NOT NULL DEFAULT '{}',
    output       JSONB NOT NULL DEFAULT '{}',
    status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'done', 'failed')),
    tokens_in    INT NOT NULL DEFAULT 0,
    tokens_out   INT NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at  TIMESTAMPTZ
);

CREATE TABLE audit_logs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID REFERENCES workspaces(id) ON DELETE SET NULL,
    user_id      UUID REFERENCES users(id) ON DELETE SET NULL,
    action       TEXT NOT NULL,
    entity_type  TEXT NOT NULL DEFAULT '',
    entity_id    UUID,
    meta         JSONB NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_publications_status ON publications (status, scheduled_at);
CREATE INDEX idx_channels_brand ON channels (brand_id);
CREATE INDEX idx_mentions_brand ON mentions (brand_id, found_at DESC);

-- +goose Down
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS ai_runs;
DROP TABLE IF EXISTS ai_agents;
DROP TABLE IF EXISTS crawl_jobs;
DROP TABLE IF EXISTS crawl_sources;
DROP TABLE IF EXISTS mentions;
DROP TABLE IF EXISTS metric_snapshots;
DROP TABLE IF EXISTS cross_links;
DROP TABLE IF EXISTS publications;
DROP TABLE IF EXISTS content_pieces;
DROP TABLE IF EXISTS campaigns;
DROP TABLE IF EXISTS channels;
DROP TABLE IF EXISTS custom_platforms;
DROP TABLE IF EXISTS platform_definitions;
DROP TABLE IF EXISTS brands;
DROP TABLE IF EXISTS workspace_members;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS workspaces;
