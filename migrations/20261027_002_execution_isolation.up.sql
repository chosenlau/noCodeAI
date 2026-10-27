CREATE TABLE IF NOT EXISTS `app_execution` (
 `app_id` BIGINT NOT NULL,
 `epoch` BIGINT NOT NULL DEFAULT 0,
 `owner` VARCHAR(128) NOT NULL DEFAULT '',
 `lease_until` DATETIME(3) NOT NULL,
 `current_version_id` BIGINT NOT NULL DEFAULT 0,
 `current_path` TEXT NOT NULL,
 PRIMARY KEY (`app_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `tool_execution` (
 `id` BIGINT NOT NULL,
 `execution_id` VARCHAR(128) NOT NULL,
 `call_id` VARCHAR(128) NOT NULL,
 `tool_name` VARCHAR(128) NOT NULL,
 `arguments_hash` VARCHAR(128) NOT NULL,
 `status` VARCHAR(32) NOT NULL,
 `result` LONGTEXT NOT NULL,
 `error` TEXT NOT NULL,
 `created_at` DATETIME(3) NOT NULL,
 `updated_at` DATETIME(3) NOT NULL,
 PRIMARY KEY (`id`),
 UNIQUE KEY `uk_tool_execution_call` (`execution_id`, `call_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
