-- Central & Local Schema — SIMARC Offline-First
-- InnoDB, utf8mb4, UUID PK, DATETIME(3), DECIMAL(19,4)

CREATE TABLE IF NOT EXISTS schema_versions (
    version VARCHAR(32) PRIMARY KEY,
    applied_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    description VARCHAR(255)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS devices (
    id BINARY(16) PRIMARY KEY,
    device_name VARCHAR(255) NOT NULL,
    device_type ENUM('desktop','laptop','tablet') DEFAULT 'desktop',
    node_id VARCHAR(64) NOT NULL UNIQUE,
    registered_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    last_sync_at DATETIME(3) NULL,
    is_active TINYINT(1) DEFAULT 1,
    deleted_at DATETIME(3) NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS sync_queue (
    id BINARY(16) PRIMARY KEY DEFAULT (UUID_TO_BIN(UUID())) ,
    operation_id VARCHAR(64) NOT NULL UNIQUE,
    table_name VARCHAR(64) NOT NULL,
    record_id BINARY(16) NOT NULL,
    operation ENUM('create','update','delete') NOT NULL,
    payload JSON NOT NULL,
    created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
    retry_count INT DEFAULT 0,
    next_retry_at DATETIME(3) NULL,
    status ENUM('pending','sending','acknowledged','failed') DEFAULT 'pending',
    ack_received_at DATETIME(3) NULL,
    deleted_at DATETIME(3) NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE INDEX idx_sync_queue_status_retry ON sync_queue(status, next_retry_at);
CREATE INDEX idx_sync_queue_operation ON sync_queue(operation_id);

CREATE TABLE IF NOT EXISTS sync_batches (
    id BINARY(16) PRIMARY KEY DEFAULT (UUID_TO_BIN(UUID())) ,
    batch_id VARCHAR(64) NOT NULL UNIQUE,
    device_id BINARY(16) NOT NULL,
    items_count INT NOT NULL,
    status ENUM('pending','sent','acknowledged','failed') DEFAULT 'pending',
    sent_at DATETIME(3) NULL,
    ack_received_at DATETIME(3) NULL,
    created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS sync_inbox (
    id BINARY(16) PRIMARY KEY DEFAULT (UUID_TO_BIN(UUID())) ,
    source_node VARCHAR(64) NOT NULL,
    change_log_id BINARY(16) NOT NULL,
    table_name VARCHAR(64) NOT NULL,
    record_id BINARY(16) NOT NULL,
    payload JSON NOT NULL,
    applied_at DATETIME(3) NULL,
    created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
    deleted_at DATETIME(3) NULL,
    FOREIGN KEY (change_log_id) REFERENCES sync_cursors(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS sync_cursors (
    id BINARY(16) PRIMARY KEY DEFAULT (UUID_TO_BIN(UUID())) ,
    node_id VARCHAR(64) NOT NULL,
    table_name VARCHAR(64) NOT NULL,
    cursor_value VARCHAR(255) NOT NULL,
    updated_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY unique_cursor (node_id, table_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS sync_conflicts (
    id BINARY(16) PRIMARY KEY DEFAULT (UUID_TO_BIN(UUID())) ,
    operation_id VARCHAR(64) NOT NULL,
    table_name VARCHAR(64) NOT NULL,
    record_id BINARY(16) NOT NULL,
    local_payload JSON,
    remote_payload JSON,
    status ENUM('pending','resolved_local','resolved_server','resolved_merge') DEFAULT 'pending',
    resolved_by VARCHAR(64) NULL,
    resolved_at DATETIME(3) NULL,
    created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS change_log (
    id BINARY(16) PRIMARY KEY DEFAULT (UUID_TO_BIN(UUID())) ,
    node_id VARCHAR(64) NOT NULL,
    table_name VARCHAR(64) NOT NULL,
    record_id BINARY(16) NOT NULL,
    operation ENUM('create','update','delete') NOT NULL,
    payload JSON NOT NULL,
    server_sequence BIGINT UNSIGNED NOT NULL,
    committed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    deleted_at DATETIME(3) NULL,
    INDEX idx_change_log_seq (server_sequence),
    INDEX idx_change_log_node (node_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS sync_logs (
    id BINARY(16) PRIMARY KEY DEFAULT (UUID_TO_BIN(UUID())) ,
    event ENUM('push_start','push_ack','pull_start','pull_complete','conflict_detected','retry') NOT NULL,
    detail JSON,
    created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS audit_logs (
    id BINARY(16) PRIMARY KEY DEFAULT (UUID_TO_BIN(UUID())) ,
    user_id BINARY(16) NULL,
    action VARCHAR(255) NOT NULL,
    table_name VARCHAR(64) NULL,
    record_id BINARY(16) NULL,
    payload JSON,
    created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
