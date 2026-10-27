CREATE TABLE IF NOT EXISTS `agent_run` (
  `id` BIGINT NOT NULL,
  `app_id` BIGINT NOT NULL,
  `user_id` BIGINT NOT NULL,
  `request_id` VARCHAR(128) NOT NULL,
  `kind` VARCHAR(32) NOT NULL,
  `status` VARCHAR(32) NOT NULL,
  `input_json` JSON NOT NULL,
  `active_key` VARCHAR(64) NULL,
  `source_version_id` BIGINT NULL,
  `last_sequence` BIGINT NOT NULL DEFAULT 0,
  `error_message` TEXT NULL,
  `started_at` DATETIME(3) NULL,
  `finished_at` DATETIME(3) NULL,
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_agent_run_request_id` (`request_id`),
  UNIQUE KEY `uk_agent_run_active_key` (`active_key`),
  KEY `idx_agent_run_app_id` (`app_id`),
  KEY `idx_agent_run_user_id` (`user_id`),
  KEY `idx_agent_run_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `run_event` (
  `id` BIGINT NOT NULL,
  `run_id` BIGINT NOT NULL,
  `sequence` BIGINT NOT NULL,
  `event_type` VARCHAR(64) NOT NULL,
  `payload` JSON NOT NULL,
  `created_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_run_event_sequence` (`run_id`, `sequence`),
  KEY `idx_run_event_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `source_version` (
  `id` BIGINT NOT NULL,
  `run_id` BIGINT NOT NULL,
  `app_id` BIGINT NOT NULL,
  `generation_type` VARCHAR(64) NOT NULL,
  `files_json` LONGTEXT NOT NULL,
  `created_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_source_version_run_id` (`run_id`),
  KEY `idx_source_version_app_id` (`app_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `task_outbox` (
  `id` BIGINT NOT NULL,
  `event_id` VARCHAR(128) NOT NULL,
  `aggregate_type` VARCHAR(64) NOT NULL,
  `aggregate_id` BIGINT NOT NULL,
  `event_type` VARCHAR(64) NOT NULL,
  `payload` JSON NOT NULL,
  `status` VARCHAR(32) NOT NULL,
  `attempts` INT NOT NULL DEFAULT 0,
  `next_attempt_at` DATETIME(3) NOT NULL,
  `claimed_at` DATETIME(3) NULL,
  `delivered_at` DATETIME(3) NULL,
  `last_error` TEXT NULL,
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_task_outbox_event_id` (`event_id`),
  KEY `idx_task_outbox_delivery` (`status`, `next_attempt_at`),
  KEY `idx_task_outbox_aggregate` (`aggregate_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
