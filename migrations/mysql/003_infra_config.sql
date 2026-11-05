-- Infra parameters are global. Nullable active keys let users reuse deleted keys.
CREATE TABLE IF NOT EXISTS infra_config (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  category VARCHAR(50) NOT NULL,
  name VARCHAR(100) NOT NULL,
  config_key VARCHAR(100) NOT NULL,
  active_key VARCHAR(100) GENERATED ALWAYS AS (CASE WHEN deleted = 0 THEN config_key ELSE NULL END) VIRTUAL,
  value VARCHAR(500) NOT NULL,
  type INT NOT NULL,
  visible BOOLEAN NOT NULL DEFAULT FALSE,
  remark VARCHAR(500) NOT NULL DEFAULT '',
  creator VARCHAR(64) NOT NULL DEFAULT '',
  create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updater VARCHAR(64) NOT NULL DEFAULT '',
  update_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  deleted TINYINT UNSIGNED NOT NULL DEFAULT 0,
  UNIQUE KEY uk_infra_config_active_key (active_key),
  KEY idx_infra_config_create_time (create_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
