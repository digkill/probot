-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE contacts (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    email        TEXT NOT NULL,
    phone        TEXT NOT NULL DEFAULT '',
    name         TEXT NOT NULL DEFAULT '',
    tags         TEXT[] NOT NULL DEFAULT '{}',
    status       TEXT NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active', 'unsubscribed', 'bounced', 'complained')),
    -- Every other imported column, keyed by its original header.
    attributes   JSONB NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, email)
);
CREATE INDEX contacts_ws_created ON contacts (workspace_id, created_at DESC, id);
CREATE INDEX contacts_ws_status ON contacts (workspace_id, status);
CREATE INDEX contacts_tags ON contacts USING GIN (tags);
CREATE INDEX contacts_search ON contacts USING GIN ((email || ' ' || name || ' ' || phone) gin_trgm_ops);

-- Imported column order per workspace, so exports reproduce the source layout.
CREATE TABLE contact_fields (
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    key          TEXT NOT NULL,
    position     INT NOT NULL,
    PRIMARY KEY (workspace_id, key)
);

-- +goose Down
DROP TABLE contact_fields;
DROP TABLE contacts;
