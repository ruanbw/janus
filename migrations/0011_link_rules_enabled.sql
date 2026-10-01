-- +goose Up
-- 0011_link_rules_enabled.sql — 短链维度的规则总开关
--
-- 允许租户在短链级别控制是否启用规则裁决。
-- 当 rules_enabled = false 时，当前短链完全跳过规则求值，直接执行原跳转/落地页分发流程。
-- 默认 TRUE，保证既有短链行为保持不变。

ALTER TABLE links
  ADD COLUMN rules_enabled BOOLEAN NOT NULL DEFAULT TRUE;
