-- 0005_drop_api_keys.sql — 移除开发者 API Key 功能
-- 已部署环境如存在 api_keys 表则一并删除;新装环境 0001 已不再创建该表。
DROP TABLE IF EXISTS api_keys;
