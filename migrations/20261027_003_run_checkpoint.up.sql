CREATE TABLE IF NOT EXISTS `run_checkpoint` (
  `run_id` BIGINT NOT NULL,
  `document` JSON NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`run_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
