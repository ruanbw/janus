-- 0013_drop_code_length.sql — 移除「自动生成短码长度偏好」
--
-- 短码长度不再由租户配置,系统固定按 domain.AutoCodeLength(= 6)生成,
-- 该列已无任何读写方,直接删除。
ALTER TABLE tenants DROP COLUMN IF EXISTS code_length;
