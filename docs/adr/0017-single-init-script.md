# ADR-0017: 开发期 Schema 收敛为单一初始化脚本 (Single Init Script, No Incremental Migrations)

## 状态

已通过 (Accepted)

## 背景

ADR-0014 的双轨迁移流水线中,基座核心轨走 goose,`migrations/` 按 `NNNN_*.sql` 追加版本、版本记录落在 `goose_db_version`。但当前处于开发阶段,数据库可破坏性重建:增量迁移的收益(不丢数据地在线升级旧库)不存在——没有需要保住的开发库;代价却全在(每次改表都要新增迁移文件、维护 Down、遵守"已发布迁移不可修改"的纪律)。

## 决策

- `migrations/` 只保留唯一的 `init.sql`,它是核心 schema 的唯一事实来源:无版本表、无增量文件,也不再兼容历史 `schema_migrations`。
- 运行方式:`internal/db` 在既有 advisory lock 下读取 `init.sql` 并整段执行。无参数 Exec 走 Postgres 简单协议,整段落在同一个隐式事务里,任何一句失败都整体回滚,不会留下半截 schema。
- 脚本必须整段幂等:建表/索引用 `IF NOT EXISTS`,函数用 `OR REPLACE`,触发器 `DROP TRIGGER IF EXISTS` + `CREATE TRIGGER`,种子用 `ON CONFLICT DO NOTHING`。启动、重跑、并发多副本都安全,已有数据不被破坏。
- 既有表结构的演进不做"就地升级":改 `init.sql` 后 DROP 开发库重建。
- 外部工程专有表仍走 ADR-0014 的第二轨(goose + `goose_db_version_extra`),版本化机制不变;本 ADR 只修订该 ADR 编排图中"基座核心迁移使用 `goose_db_version`"的部分。

## 代价与边界

- 无法在线升级生产库:发版前的破坏性 schema 变更需要重建库或另行人工处理(生产部署尚未开始,接受)。
- 幂等语句不会修改既有表结构:`CREATE TABLE IF NOT EXISTS` 不会给旧表补列,这类改动必须重建开发库才能生效。
- 契约由 `internal/db/migrate_test.go` 固化:`migrations/` 下只有 `init.sql`、脚本不含 `+goose` 注解、全新空库能一次建出全量 schema、重复执行不破坏数据、核心轨不产生任何版本表。
