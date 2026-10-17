-- +goose Up
CREATE TABLE IF NOT EXISTS users (
    id             UUID PRIMARY KEY DEFAULT uuidv7(),
    email          TEXT NOT NULL UNIQUE,
    password_hash  TEXT NOT NULL,
    first_name     TEXT NOT NULL,
    last_name      TEXT NOT NULL,
    phone          TEXT DEFAULT '',
    role           TEXT NOT NULL DEFAULT 'user',
    active         BOOLEAN NOT NULL DEFAULT true,
    token_version  INT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at     TIMESTAMPTZ,

    CONSTRAINT users_role_check CHECK (role IN ('user', 'admin'))
);

CREATE INDEX idx_users_email ON users(email) WHERE deleted_at IS NULL;

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TABLE IF EXISTS users;
