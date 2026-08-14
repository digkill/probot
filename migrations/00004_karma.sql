-- +goose Up
CREATE TABLE link_partners (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    brand_id     UUID REFERENCES brands(id) ON DELETE SET NULL,
    name         TEXT NOT NULL,
    platform     TEXT NOT NULL DEFAULT '',
    their_url    TEXT NOT NULL DEFAULT '',
    our_url      TEXT NOT NULL DEFAULT '',
    contact      TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'outreach' CHECK (status IN (
        'outreach', 'waiting', 'we_linked', 'they_linked', 'complete', 'declined'
    )),
    notes        TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE engagement_tasks (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id        UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    brand_id            UUID REFERENCES brands(id) ON DELETE SET NULL,
    channel_id          UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    partner_id          UUID REFERENCES link_partners(id) ON DELETE SET NULL,
    mention_id          UUID REFERENCES mentions(id) ON DELETE SET NULL,
    kind                TEXT NOT NULL CHECK (kind IN ('like', 'comment', 'follow', 'share', 'link_exchange')),
    status              TEXT NOT NULL DEFAULT 'pending_approval' CHECK (status IN (
        'draft', 'pending_approval', 'approved', 'executing', 'done', 'failed', 'skipped', 'needs_manual'
    )),
    target_url          TEXT NOT NULL DEFAULT '',
    target_external_id  TEXT NOT NULL DEFAULT '',
    target_title        TEXT NOT NULL DEFAULT '',
    draft_text          TEXT NOT NULL DEFAULT '',
    result_url          TEXT NOT NULL DEFAULT '',
    points              INT NOT NULL DEFAULT 0,
    error_message       TEXT NOT NULL DEFAULT '',
    scheduled_at        TIMESTAMPTZ,
    executed_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_engagement_tasks_ws ON engagement_tasks (workspace_id, created_at DESC);
CREATE INDEX idx_engagement_tasks_status ON engagement_tasks (status, scheduled_at);

CREATE TABLE karma_events (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    channel_id   UUID REFERENCES channels(id) ON DELETE SET NULL,
    task_id      UUID REFERENCES engagement_tasks(id) ON DELETE SET NULL,
    kind         TEXT NOT NULL,
    points       INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_karma_events_ws ON karma_events (workspace_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS karma_events;
DROP TABLE IF EXISTS engagement_tasks;
DROP TABLE IF EXISTS link_partners;
