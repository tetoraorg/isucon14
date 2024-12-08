ALTER TABLE chairs
  ADD COLUMN total_distance INTEGER NOT NULL DEFAULT 0 COMMENT '総移動距離',
  ADD COLUMN total_distance_updated_at DATETIME(6) DEFAULT NULL COMMENT '総移動距離更新日時';

UPDATE chairs
LEFT JOIN (
  SELECT 
    `chair_id`,
    SUM(IFNULL(`distance`, 0)) AS `total_distance`,
    MAX(`created_at`) AS `total_distance_updated_at`
  FROM (
    SELECT 
      `chair_id`,
      `created_at`,
      ABS(`latitude` - LAG(`latitude`) OVER (PARTITION BY `chair_id` ORDER BY `created_at`)) +
      ABS(`longitude` - LAG(`longitude`) OVER (PARTITION BY `chair_id` ORDER BY `created_at`)) AS `distance`
    FROM `chair_locations`
  ) AS `tmp`
  GROUP BY `chair_id`
) AS `distance_table` 
ON `distance_table`.`chair_id` = `chairs`.`id`
SET 
  `chairs`.`total_distance` = IFNULL(`distance_table`.`total_distance`, 0),
  `chairs`.`total_distance_updated_at` = `distance_table`.`total_distance_updated_at`;

-- 最新のchair_locationsのみ残す
DELETE FROM chair_locations
WHERE id NOT IN (
  SELECT id
  FROM (
    SELECT id
    FROM chair_locations
    WHERE (chair_id, created_at) IN (
      SELECT chair_id, MAX(created_at)
      FROM chair_locations
      GROUP BY chair_id
    )
  ) AS latest_records
);
