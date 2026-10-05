-- +goose Up
-- Only a SHA-256 of the emailed token is stored, so a DB leak does not grant resets.
CREATE TABLE password_resets (
    token_hash BYTEA PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX password_resets_user ON password_resets(user_id, created_at);

-- +goose Down
DROP TABLE password_resets;
