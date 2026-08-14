-- 0004_visit_ip.sql — 访问记录增加访问者 IP 字段
-- 统计页展示访问来源 IP;由跳转处理器写入(X-Forwarded-For 优先,回退 RemoteAddr)。
ALTER TABLE visits ADD COLUMN ip TEXT NOT NULL DEFAULT '';
