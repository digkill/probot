-- +goose Up
CREATE TABLE telegram_accounts (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 name TEXT NOT NULL,
 enabled BOOLEAN NOT NULL DEFAULT false,
 telegram_user_id BIGINT NOT NULL DEFAULT 0,
 phone TEXT NOT NULL DEFAULT '',
 username TEXT NOT NULL DEFAULT '',
 first_name TEXT NOT NULL DEFAULT '',
 last_name TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL DEFAULT 'stopped' CHECK (status IN ('stopped','connecting','auth_required','connected','flood_wait','failed')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 last_connected_at TIMESTAMPTZ
);
CREATE INDEX telegram_accounts_workspace ON telegram_accounts(workspace_id, created_at);
-- Ciphertext includes version and nonce; account_id + key are authenticated as AAD.
CREATE TABLE telegram_state (
 account_id UUID NOT NULL REFERENCES telegram_accounts(id) ON DELETE CASCADE,
 key TEXT NOT NULL,
 value BYTEA NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY (account_id, key)
);
-- Durable event inbox: insert before acknowledging updates, deduplicate recovery.
CREATE TABLE telegram_events (
 id BIGSERIAL PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES telegram_accounts(id) ON DELETE CASCADE,
 type TEXT NOT NULL,
 chat_id BIGINT NOT NULL,
 message_id INTEGER NOT NULL,
 payload BYTEA NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE (account_id, type, chat_id, message_id)
);
CREATE INDEX telegram_events_account_cursor ON telegram_events(account_id, id);

-- +goose Down
DROP TABLE telegram_events;
DROP TABLE telegram_state;
DROP TABLE telegram_accounts;
