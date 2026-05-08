-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS sia_accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL UNIQUE,
    connect_key TEXT DEFAULT '',
    quota_key TEXT DEFAULT '',
    fund_target_bytes INTEGER NOT NULL DEFAULT 0,
    last_funding_event_id INTEGER DEFAULT 0,
    last_funding_event_at TIMESTAMP DEFAULT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP DEFAULT NULL
);

CREATE TABLE IF NOT EXISTS sia_app_accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    sia_account_id INTEGER NOT NULL,
    account_key BLOB NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP DEFAULT NULL
);

CREATE INDEX IF NOT EXISTS idx_sia_app_accounts_sia_account_id ON sia_app_accounts(sia_account_id);

CREATE TABLE IF NOT EXISTS sia_funding_cursors (
    id INTEGER PRIMARY KEY,
    last_funding_event_id INTEGER NOT NULL DEFAULT 0,
    last_funding_event_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00'
);

INSERT OR IGNORE INTO sia_funding_cursors (id, last_funding_event_id, last_funding_event_at) VALUES (1, 0, '1970-01-01 00:00:00');

CREATE TABLE IF NOT EXISTS sia_slabs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    sia_app_account_id INTEGER NOT NULL,
    slab_id TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP DEFAULT NULL,
    UNIQUE (sia_app_account_id, slab_id)
);

CREATE INDEX IF NOT EXISTS idx_sia_slabs_sia_app_account_id ON sia_slabs(sia_app_account_id);

CREATE TABLE IF NOT EXISTS sia_objects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    sia_app_account_id INTEGER NOT NULL,
    object_id TEXT NOT NULL,
    UNIQUE (sia_app_account_id, object_id)
);

CREATE INDEX IF NOT EXISTS idx_sia_objects_deleted_at ON sia_objects(deleted_at);

CREATE TABLE IF NOT EXISTS sia_object_slabs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    sia_object_id INTEGER NOT NULL,
    sia_slab_id INTEGER NOT NULL,
    UNIQUE (sia_object_id, sia_slab_id)
);

CREATE INDEX IF NOT EXISTS idx_sia_object_slabs_deleted_at ON sia_object_slabs(deleted_at);

CREATE TABLE IF NOT EXISTS sia_auth_requests (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    request_id TEXT NOT NULL,
    user_id INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_sia_auth_requests_request_id ON sia_auth_requests(request_id);
CREATE INDEX IF NOT EXISTS idx_sia_auth_requests_user_id ON sia_auth_requests(user_id);
CREATE INDEX IF NOT EXISTS idx_sia_auth_requests_deleted_at ON sia_auth_requests(deleted_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS sia_auth_requests;
DROP TABLE IF EXISTS sia_object_slabs;
DROP TABLE IF EXISTS sia_objects;
DROP TABLE IF EXISTS sia_slabs;
DROP TABLE IF EXISTS sia_funding_cursors;
DROP TABLE IF EXISTS sia_app_accounts;
DROP TABLE IF EXISTS sia_accounts;
-- +goose StatementEnd
