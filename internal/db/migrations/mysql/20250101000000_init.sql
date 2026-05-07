-- +goose Up
CREATE TABLE IF NOT EXISTS sia_accounts (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    connect_key VARCHAR(255) DEFAULT '',
    quota_key VARCHAR(255) DEFAULT '',
    fund_target_bytes BIGINT UNSIGNED NOT NULL DEFAULT 0,
    last_funding_event_id BIGINT DEFAULT 0,
    last_funding_event_at TIMESTAMP NULL DEFAULT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL DEFAULT NULL,
    UNIQUE KEY unique_user_id (user_id)
);

CREATE TABLE IF NOT EXISTS sia_app_accounts (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    sia_account_id BIGINT UNSIGNED NOT NULL,
    account_key VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL DEFAULT NULL,
    UNIQUE KEY unique_account_key (account_key),
    INDEX idx_sia_app_accounts_sia_account_id (sia_account_id)
);

CREATE TABLE IF NOT EXISTS sia_funding_cursors (
    id BIGINT PRIMARY KEY,
    last_funding_event_id BIGINT NOT NULL DEFAULT 0,
    last_funding_event_at DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00'
);

INSERT IGNORE INTO sia_funding_cursors (id, last_funding_event_id, last_funding_event_at) VALUES (1, 0, '1970-01-01 00:00:00');

CREATE TABLE IF NOT EXISTS sia_slabs (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    sia_app_account_id BIGINT UNSIGNED NOT NULL,
    slab_id VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL DEFAULT NULL,
    UNIQUE KEY unique_app_account_slab (sia_app_account_id, slab_id),
    INDEX idx_sia_slabs_sia_app_account_id (sia_app_account_id)
);

CREATE TABLE IF NOT EXISTS sia_objects (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    created_at DATETIME(3) NULL,
    updated_at DATETIME(3) NULL,
    deleted_at DATETIME(3) NULL,
    sia_app_account_id BIGINT UNSIGNED NOT NULL,
    object_id VARCHAR(255) NOT NULL,
    INDEX idx_sia_objects_deleted_at (deleted_at),
    UNIQUE INDEX idx_app_account_object (sia_app_account_id, object_id)
);

CREATE TABLE IF NOT EXISTS sia_object_slabs (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    created_at DATETIME(3) NULL,
    updated_at DATETIME(3) NULL,
    deleted_at DATETIME(3) NULL,
    sia_object_id BIGINT UNSIGNED NOT NULL,
    sia_slab_id BIGINT UNSIGNED NOT NULL,
    INDEX idx_sia_object_slabs_deleted_at (deleted_at),
    UNIQUE INDEX idx_object_slab (sia_object_id, sia_slab_id)
);

ALTER TABLE sia_object_slabs
    ADD CONSTRAINT fk_sia_object_slabs_object
    FOREIGN KEY (sia_object_id) REFERENCES sia_objects(id) ON DELETE CASCADE;

ALTER TABLE sia_object_slabs
    ADD CONSTRAINT fk_sia_object_slabs_slab
    FOREIGN KEY (sia_slab_id) REFERENCES sia_slabs(id) ON DELETE CASCADE;

CREATE TABLE IF NOT EXISTS sia_auth_requests (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    created_at DATETIME(3) NULL,
    updated_at DATETIME(3) NULL,
    deleted_at DATETIME(3) NULL,
    request_id VARCHAR(64) NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    UNIQUE INDEX idx_sia_auth_requests_request_id (request_id),
    INDEX idx_sia_auth_requests_user_id (user_id),
    INDEX idx_sia_auth_requests_deleted_at (deleted_at)
);

-- +goose Down
DROP TABLE IF EXISTS sia_auth_requests;
DROP TABLE IF EXISTS sia_object_slabs;
DROP TABLE IF EXISTS sia_objects;
DROP TABLE IF EXISTS sia_slabs;
DROP TABLE IF EXISTS sia_funding_cursors;
DROP TABLE IF EXISTS sia_app_accounts;
DROP TABLE IF EXISTS sia_accounts;
