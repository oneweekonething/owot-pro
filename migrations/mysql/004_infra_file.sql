-- Storage configurations and file content are global. File metadata also
-- records the owner for tenant-scoped management and upload completion.
CREATE TABLE IF NOT EXISTS infra_file_config (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  storage INT NOT NULL,
  remark VARCHAR(500) NOT NULL DEFAULT '',
  master BOOLEAN NOT NULL DEFAULT FALSE,
  master_slot INT GENERATED ALWAYS AS (CASE WHEN deleted = 0 AND master = 1 THEN 1 ELSE NULL END) VIRTUAL,
  config JSON NOT NULL,
  creator VARCHAR(64) NOT NULL DEFAULT '',
  create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updater VARCHAR(64) NOT NULL DEFAULT '',
  update_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  deleted TINYINT UNSIGNED NOT NULL DEFAULT 0,
  UNIQUE KEY uk_infra_file_master (master_slot),
  KEY idx_infra_file_config_create_time (create_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS infra_file (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  config_id BIGINT NOT NULL,
  owner_tenant_id BIGINT NOT NULL,
  owner_user_id BIGINT NOT NULL,
  name VARCHAR(180) NOT NULL,
  path VARCHAR(320) NOT NULL,
  url VARCHAR(1500) NOT NULL,
  type VARCHAR(128) NOT NULL DEFAULT '',
  size BIGINT NOT NULL DEFAULT 0,
  upload_expires_at DATETIME(3) DEFAULT NULL,
  avatar BOOLEAN NOT NULL DEFAULT FALSE,
  cleanup_pending BOOLEAN NOT NULL DEFAULT FALSE,
  creator VARCHAR(64) NOT NULL DEFAULT '',
  create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updater VARCHAR(64) NOT NULL DEFAULT '',
  update_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  deleted TINYINT UNSIGNED NOT NULL DEFAULT 0,
  UNIQUE KEY uk_infra_file_path (config_id, path),
  KEY idx_infra_file_owner (owner_tenant_id),
  KEY idx_infra_file_pending (upload_expires_at),
  KEY idx_infra_file_create_time (create_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS infra_file_content (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  config_id BIGINT NOT NULL,
  path VARCHAR(320) NOT NULL,
  content LONGBLOB NOT NULL,
  creator VARCHAR(64) NOT NULL DEFAULT '',
  create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updater VARCHAR(64) NOT NULL DEFAULT '',
  update_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  deleted TINYINT UNSIGNED NOT NULL DEFAULT 0,
  UNIQUE KEY uk_infra_file_content_path (config_id, path)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

INSERT INTO infra_file_config (id, name, storage, master, config)
SELECT 1, '数据库存储', 1, 1, '{}'
WHERE NOT EXISTS (SELECT 1 FROM infra_file_config);
