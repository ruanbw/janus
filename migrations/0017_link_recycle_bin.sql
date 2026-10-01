-- +goose Up
-- 0017_link_recycle_bin.sql — 短链回收站:逻辑删除后释放「同一域名下短码」占用
--
-- 问题:link_domains 上的 UNIQUE (domain_id, code) 对**已逻辑删除**的短链仍然生效。
-- 于是 SoftDeleteLink 之后,同一域名下再也建不出同短码的短链(创建返回 409),
-- 而旧记录在列表里被 `deleted_at IS NULL` 过滤掉 —— 短码被永久锁死。
--
-- 方案:在 link_domains 上 denormalize 一个布尔 `link_deleted`(该短链是否已逻辑删除),
-- 用触发器跟随 links.deleted_at 保持一致,再把全表 UNIQUE 约束换成
-- **部分唯一索引** `UNIQUE (domain_id, code) WHERE NOT link_deleted`。
--
-- 为什么必须是「列 + 触发器」而不是「触发器里查一次」:
--   Postgres 的部分唯一索引谓词(索引的 WHERE 子句)只允许引用被索引表自身的列,
--   不允许子查询 / JOIN。想让唯一性「只对未删除短链生效」,就必须把这个状态
--   物理地放到 link_domains 上。
--   另一条路是去掉唯一约束,在 link_domains 的 BEFORE INSERT 触发器里
--   `SELECT ... FROM links JOIN link_domains ...` 判冲突并抛 23505 —— 那样
--   唯一性从索引引擎退回到应用逻辑,每次插入多一次查询,还要自己处理并发,
--   正确性明显更脆弱。带列的部分唯一索引把保证留在索引里,代价只是一列 + 一个触发器。
--
-- 触发器放在 links 上(AFTER UPDATE OF deleted_at)而不是放在 link_domains 的写路径上:
--   deleted_at 的唯一事实来源就是 links,谁改它就同步谁,不会出现「有人绕过入口」的漏同步。
--   另外补一个 link_domains 的 BEFORE INSERT 触发器,让「直接插入关联行」
--   (含未来的脚本 / 数据修复)也自动带上正确的 link_deleted,列值永远与父行不脱节。
--
-- 「还原」语义:restore 把 deleted_at 置 NULL,触发器把 link_domains.link_deleted
-- 翻回 false,此时若该 (domain_id, code) 已被另一条存活短链占用,部分唯一索引
-- 直接拒绝 → 对外 409。也就是「还原后短码又变回占用」由数据库保证,应用层无需自查。

ALTER TABLE link_domains
    ADD COLUMN IF NOT EXISTS link_deleted BOOLEAN NOT NULL DEFAULT FALSE;

-- 回填:让列值与既有 links.deleted_at 一致(全新库无逻辑删除行,全为 FALSE)。
UPDATE link_domains ld
   SET link_deleted = (l.deleted_at IS NOT NULL)
  FROM links l
 WHERE l.id = ld.link_id;

-- 全表唯一约束换成「只对未删除短链生效」的部分唯一索引
ALTER TABLE link_domains DROP CONSTRAINT IF EXISTS link_domains_domain_id_code_key;
DROP INDEX IF EXISTS link_domains_domain_id_code_key;
CREATE UNIQUE INDEX IF NOT EXISTS link_domains_live_code_uniq
    ON link_domains (domain_id, code)
    WHERE NOT link_deleted;

-- links.deleted_at 变化 → 同步本短链所有关联行的 link_deleted
-- 注意:函数体里的分号必须用 StatementBegin/StatementEnd 包住 —— goose 默认按分号
-- 切分语句,不加这对注解会把 $$ ... $$ 从中间劈开,报 unterminated dollar-quoted string。
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION link_domains_sync_link_deleted() RETURNS trigger AS $$
BEGIN
    UPDATE link_domains
       SET link_deleted = (NEW.deleted_at IS NOT NULL)
     WHERE link_id = NEW.id;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_links_sync_link_deleted ON links;
CREATE TRIGGER trg_links_sync_link_deleted
    AFTER UPDATE OF deleted_at ON links
    FOR EACH ROW
    WHEN (OLD.deleted_at IS DISTINCT FROM NEW.deleted_at)
    EXECUTE FUNCTION link_domains_sync_link_deleted();

-- 新插入的关联行一律从父短链派生 link_deleted(不信任调用方传入的值)
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION link_domains_derive_link_deleted() RETURNS trigger AS $$
BEGIN
    SELECT (l.deleted_at IS NOT NULL) INTO NEW.link_deleted
      FROM links l WHERE l.id = NEW.link_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_link_domains_derive_link_deleted ON link_domains;
CREATE TRIGGER trg_link_domains_derive_link_deleted
    BEFORE INSERT ON link_domains
    FOR EACH ROW
    EXECUTE FUNCTION link_domains_derive_link_deleted();
