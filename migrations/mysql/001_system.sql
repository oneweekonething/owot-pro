-- System module; all DATETIME values are stored in UTC.
-- Nullable active keys permit repeated soft-delete/recreate cycles.
CREATE TABLE IF NOT EXISTS system_dict_type (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  type VARCHAR(100) NOT NULL,
  active_type VARCHAR(100) NULL,
  status INT NOT NULL DEFAULT 0,
  remark VARCHAR(500) NOT NULL DEFAULT '',
  creator VARCHAR(64) NOT NULL DEFAULT '',
  create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updater VARCHAR(64) NOT NULL DEFAULT '',
  update_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  deleted TINYINT UNSIGNED NOT NULL DEFAULT 0,
  UNIQUE KEY uk_dict_type_active (active_type),
  KEY idx_system_dict_type_type (type),
  KEY idx_system_dict_type_create_time (create_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS system_dict_data (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  sort INT NOT NULL DEFAULT 0,
  label VARCHAR(100) NOT NULL,
  value VARCHAR(100) NOT NULL,
  dict_type VARCHAR(100) NOT NULL,
  active_value VARCHAR(100) NULL,
  status INT NOT NULL DEFAULT 0,
  color_type VARCHAR(100) NOT NULL DEFAULT '',
  css_class VARCHAR(100) NOT NULL DEFAULT '',
  remark VARCHAR(500) NOT NULL DEFAULT '',
  creator VARCHAR(64) NOT NULL DEFAULT '',
  create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updater VARCHAR(64) NOT NULL DEFAULT '',
  update_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  deleted TINYINT UNSIGNED NOT NULL DEFAULT 0,
  UNIQUE KEY uk_dict_data_active (dict_type, active_value),
  KEY idx_system_dict_data_dict_type (dict_type),
  KEY idx_system_dict_data_create_time (create_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS system_notice (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  tenant_id BIGINT NOT NULL,
  title VARCHAR(50) NOT NULL,
  type INT NOT NULL,
  content LONGTEXT NOT NULL,
  status INT NOT NULL DEFAULT 0,
  creator VARCHAR(64) NOT NULL DEFAULT '',
  create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updater VARCHAR(64) NOT NULL DEFAULT '',
  update_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  deleted TINYINT UNSIGNED NOT NULL DEFAULT 0,
  KEY idx_system_notice_tenant_id (tenant_id),
  KEY idx_system_notice_create_time (create_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS system_operate_log (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  tenant_id BIGINT NOT NULL,
  trace_id VARCHAR(64) NOT NULL DEFAULT '',
  user_id BIGINT NOT NULL,
  user_type INT NOT NULL,
  type VARCHAR(50) NOT NULL,
  sub_type VARCHAR(50) NOT NULL,
  biz_id BIGINT NOT NULL DEFAULT 0,
  action VARCHAR(2000) NOT NULL DEFAULT '',
  extra LONGTEXT,
  request_method VARCHAR(16) NOT NULL DEFAULT '',
  request_url VARCHAR(2048) NOT NULL DEFAULT '',
  user_ip VARCHAR(64) NOT NULL DEFAULT '',
  user_agent VARCHAR(1024) NOT NULL DEFAULT '',
  creator VARCHAR(64) NOT NULL DEFAULT '',
  create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updater VARCHAR(64) NOT NULL DEFAULT '',
  update_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  deleted TINYINT UNSIGNED NOT NULL DEFAULT 0,
  KEY idx_system_operate_log_tenant_id (tenant_id),
  KEY idx_system_operate_log_user_id (user_id),
  KEY idx_system_operate_log_create_time (create_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS system_login_log (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  tenant_id BIGINT NOT NULL,
  log_type INT NOT NULL,
  trace_id VARCHAR(64) NOT NULL DEFAULT '',
  user_id BIGINT NOT NULL DEFAULT 0,
  user_type INT NOT NULL,
  username VARCHAR(100) NOT NULL,
  result INT NOT NULL,
  user_ip VARCHAR(64) NOT NULL DEFAULT '',
  user_agent VARCHAR(1024) NOT NULL DEFAULT '',
  creator VARCHAR(64) NOT NULL DEFAULT '',
  create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updater VARCHAR(64) NOT NULL DEFAULT '',
  update_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  deleted TINYINT UNSIGNED NOT NULL DEFAULT 0,
  KEY idx_system_login_log_tenant_id (tenant_id),
  KEY idx_system_login_log_user_id (user_id),
  KEY idx_system_login_log_create_time (create_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS sys_outbox_event (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  topic VARCHAR(128) NOT NULL,
  tenant_id BIGINT NOT NULL DEFAULT 0,
  payload LONGTEXT NOT NULL,
  status INT NOT NULL DEFAULT 0,
  retry_count INT NOT NULL DEFAULT 0,
  next_retry_time DATETIME(3) NOT NULL,
  create_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  update_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  KEY idx_outbox_status_retry (status, next_retry_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
