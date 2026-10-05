# 原语层采用 shadcn-vue CLI 原生族目录布局

原语层（`web/src/components/ui/`）从「扁平 kebab-case 文件」（`button.vue` / `card-header.vue` …）迁移为 shadcn-vue CLI 的原生落盘形态：`ui/<slug>/<PascalName>.vue` + 每族 `index.ts`（barrel 指回组件文件），引用统一走 `@/components/ui/<slug>`，并新增 `web/components.json` 声明别名与样式入口。ADR-0011 的双层与单向依赖决策不变，本 ADR 只修订落盘形态。

选择该形态而不是继续扁平文件：扁平件与 `shadcn-vue add button` 的产物（`ui/button/Button.vue` + `ui/button/index.ts`）同名不同形，CLI 追加组件时会生成平行结构，`@/components/ui/button` 同时命中文件与目录；ADR-0011 声称的「整目录覆盖升级」实际无法执行。迁移后 CLI 新增组件与人工目录天然一致，导入形态与 shadcn 文档一致。

代价与边界：既有原语外观已按本项目令牌定制（Element 语义色、控件高度等），CLI 只用于新增组件，不用于覆盖升级既有原语；`ui/` 内部保持同族相对导入，跨层仍由门禁第 13/14/15 项约束。布局与引用形态由门禁第 16–19 项固化：ui 根目录只允许 kebab-case 族目录、族目录 PascalCase 文件 + `index.ts` 全量 named export、`components.json` 必须存在、业务层与测试禁止深路径导入。
