-- +goose Up
-- 0012_custom_error_pages.sql — 自定义 404 与 429 错误页面
--
-- 1. 租户全局默认错误页面 (tenants)
-- 2. 规则专属错误页面与模式 (rules)

ALTER TABLE tenants
  ADD COLUMN custom_404_html TEXT NOT NULL DEFAULT '',
  ADD COLUMN custom_429_html TEXT NOT NULL DEFAULT '';

ALTER TABLE rules
  ADD COLUMN page_mode VARCHAR(16) NOT NULL DEFAULT 'default',
  ADD COLUMN custom_html TEXT NOT NULL DEFAULT '';

ALTER TABLE rules
  ADD CONSTRAINT rules_page_mode_check
  CHECK (page_mode IN ('default', 'custom'));
