-- +goose Up
-- 0003_domain_description.sql — 域名增加描述(备注)字段
-- 自有域名添加时可填写用途说明;平台默认域名保持空字符串。
ALTER TABLE domains ADD COLUMN description TEXT NOT NULL DEFAULT '';
