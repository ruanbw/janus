# 组件库分两层：shadcn 原语层 + 项目组件层

后台的组件库拆成两层。`web/src/components/ui/` 只放 shadcn 风格的原语（`button.vue` / `input.vue` / `switch.vue` 等 kebab-case 文件），是可以整目录覆盖升级的「下载件」；`web/src/components/app/` 放本项目的 `App*` 组件（`AppButton` / `AppInput` / `AppTable` …），全部构建在 `ui/` 之上。两层之间的方向是单向的：`ui/` 不得引用项目层令牌与 `app/`，`app/` 不得绕过 `ui/` 直接拼装 reka-ui。

选择这样分而不是继续单层：单层时"下载的 shadcn 组件"和"自己写的业务组件"住在同一目录，`shadcn add button` 装进来的文件和 `AppButton.vue` 平级，谁覆盖谁说不清；组件外观只能在 `App*` 里手搓，`AppButton` / `AppSwitch` / `AppTag` 各自维护一份变体表。分层之后，原语管外观与状态，项目组件管契约（antd 兼容的 props、插槽、表单联动），替换原语不影响 16 个 view。

视觉方向定为 nova：中性 zinc 灰阶、近黑主色、紧凑字号（11/12/13/14/16/20/24/32）、小圆角（控件 6px / 卡片 8px / 弹窗 10px）、极浅阴影，靛蓝只保留给焦点环、链接与品牌标签。全部令牌集中在 `web/src/styles/theme.css` 一个文件，字体、字号尺度、圆角、阴影、明暗两套色板都在那里，`main.css` 只剩基础层与动效。令牌值只用 hex / rgb 写，因为 UI 门禁脚本要静态解析颜色做对比度校验。

影响：多一层目录和一道跨层纪律，需要机器守卫——`web/scripts/check-ui-consistency.mjs` 增加三条规则拦截 `ui/` 反向依赖 `app/`、`app/` 直连 reka-ui、`ui/` 使用项目层令牌。改主题只改 `theme.css` 一个文件；换组件库只动 `ui/` 目录。
