-- Local business tables for SIMARC clients (InnoDB, utf8mb4, UUID v7 PK)
CREATE TABLE IF NOT EXISTS arsip (
    id BINARY(16) NOT NULL PRIMARY KEY,
    nama VARCHAR(512) NOT NULL,
    kategori VARCHAR(64),
    status ENUM('draft','sent','archived','deleted') DEFAULT 'draft',
    file_path VARCHAR(1024),
    file_size BIGINT UNSIGNED,
    created_by BINARY(16),
    created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
    updated_by BINARY(16),
    updated_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    version INT NOT NULL DEFAULT 1,
    deleted_at DATETIME(3) NULL,
    INDEX idx_arsip_kategori (kategori), INDEX idx_arsip_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS pemberkasan (
    id BINARY(16) NOT NULL PRIMARY KEY,
    no_surat VARCHAR(64),
    tgl_terbit DATETIME(3),
    isinya TEXT,
    created_by BINARY(16),
    created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    version INT NOT NULL DEFAULT 1,
    deleted_at DATETIME(3) NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS users (
    id BINARY(16) NOT NULL PRIMARY KEY,
    username VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(64) DEFAULT 'Staff',
    active TINYINT(1) DEFAULT 1,
    created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
    version INT DEFAULT 1,
    deleted_at DATETIME(3) NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS roles (
    id BINARY(16) NOT NULL PRIMARY KEY,
    name VARCHAR(64) NOT NULL UNIQUE,
    description VARCHAR(255),
    created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
