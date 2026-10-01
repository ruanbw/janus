-- +goose Up
-- 0007_landing_pages.sql — 落地页型短链:短链类型、落地页来源、落地页地址与点击计数
-- 短链类型:redirect(访问即跳转目标)/ landing(访问先到落地页,按钮点击后到目标)
ALTER TABLE links ADD COLUMN link_type TEXT NOT NULL DEFAULT 'redirect'
    CHECK (link_type IN ('redirect','landing'));
-- 落地页来源:url(填写落地页地址)/ upload(上传 zip,平台托管在"短码/"路径下)
ALTER TABLE links ADD COLUMN landing_source TEXT NOT NULL DEFAULT 'url'
    CHECK (landing_source IN ('url','upload'));
-- 落地页地址(仅 landing+url 来源使用,空串默认)
ALTER TABLE links ADD COLUMN landing_url TEXT NOT NULL DEFAULT '';
-- 点击计数(裸计数器:落地页按钮经 SDK 回传 +1,不记明细)
ALTER TABLE links ADD COLUMN clicks BIGINT NOT NULL DEFAULT 0;
