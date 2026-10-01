# Janus 后台管理界面(web/)

Janus 短链服务的后台管理界面:基于 Vue 3 + TypeScript + Vite + Tailwind CSS 4 + Reka UI(无头组件)的单页应用(SPA),采用 Vben Admin 的工程形态(目录分层、Pinia 状态、路由守卫、Axios 封装),消费 Go 后端 RESTful API。

## 技术栈

- Vue 3.5 + TypeScript + Vite 6
- Tailwind CSS 4(样式与设计令牌,支持深色模式)+ Reka UI(无头交互组件:Select/Dropdown/AlertDialog/Tooltip/Popover 等)
- @lucide/vue(图标)、Pinia(状态)、Vue Router 4(路由)、Axios(请求)
- d3-geo + topojson-client + world-atlas(总览页世界地图,懒加载,见下)
- 双层组件库:src/components/ui/ 是 shadcn 风格原语层,src/components/app/ 是项目组件层(AppButton/AppForm/AppTable,全局注册),见 UI_KIT.md

## 本地开发

```bash
pnpm install
pnpm dev        # http://localhost:5173,/api 代理到 http://localhost:8080
# 域名入口(SwitchHosts 配置后):http://app.janus.test:5173(vite.config.ts 已放行)
pnpm check:ui   # 自动化 UI 一致性与设计规范门禁检查（0 违规才通过）
pnpm type-check # Vue 3 + TypeScript 全量类型检查
```

环境要求:Postgres 与 Caddy 由 `docker compose up -d` 提供;Go 后端在终端启动并监听 `http://localhost:8080`(见仓库根 README §5.6:`JANUS_COOKIE_SECURE=false JANUS_ADDR=:8080 go run ./cmd/janus`)。

## 构建

```bash
pnpm build      # 产物输出到 dist/(不入库);生产镜像构建期自行执行 pnpm build
```

## 开发约定

- UI 组件库契约见 web/UI_KIT.md(组件 props、表单/表格/确认框/toast API、图标映射、设计令牌)。
- 深色模式:html.dark 由 src/stores/theme.ts 控制并持久化到 localStorage(janus-theme)。
- 响应式一律用 Tailwind 断点工具类,不在 JS 里判断视口宽度。

## 总览页世界地图

- 数据源:`world-atlas/countries-110m.json`(Natural Earth 110m 国界,ISC 许可),**随包发布、不联网拉取**——自托管部署常在内网/离线,运行时取 GeoJSON 会直接白屏。
- 渲染:`d3-geo` 的 `geoNaturalEarth1` 投影 + `topojson-client` 转 GeoJSON。投影与路径在 `src/views/overview/worldMap.ts` 里算一次并缓存,组件只拿路径字符串。
- 国界要素 id 是 ISO 3166-1 **numeric** 码,访问记录里存的是 **alpha-2**,靠 `i18n-iso-countries` 的 `getNumericCodes()` 换算(`constants/countries.ts` 的 `alpha2FromNumeric`)。
- 三样依赖都用动态 `import()`,只在总览页加载(~152KB / gzip ~55KB)。分包失败时降级成一段说明 + 重试按钮,国家排行榜照常可用。
- 填充色分 4 档(`< 5%` / `5–15%` / `15–35%` / `≥ 35%`),样式在 `main.css` 的 `.map-shape` / `.map-lv1..4`,深色模式走反向 ramp。
- 选型理由与被否决的备选见 `docs/adr/0010-overview-world-map.md`。

## 认证与请求约定

- 会话:登录后后端 `Set-Cookie`(janus_session,HTTP-only),请求自动携带(axios `withCredentials`)。
- CSRF:双提交 token——从 cookie `janus_csrf` 读取值,写方法请求头附加 `X-CSRF-Token`(见 `src/utils/request.ts`)。
- 统一错误:非 2xx 响应解析 `{ code, message, details? }`,抛出 `ApiError`(见 `src/types/api.ts`),页面用 message 友好展示;401 自动回登录页。

## 页面

| 路由 | 页面 | 说明 |
| --- | --- | --- |
| /login | 登录 | 邮箱+密码,可勾选记住我(30 天) |
| /register | 注册 | 邮箱+密码+前缀(slug),注册后引导邮箱验证 |
| /verify-email | 邮箱验证 | 从验证链接取 token 调用验证 |
| /forgot-password、/reset-password | 密码重置 | |
| /domains | 域名 | 平台默认域名/自有域名、状态与证书状态、添加/重检/停用恢复/删除 |
| /overview | 总览 | KPI 卡片 + 访问来源地世界地图(按访问占比给国家填色)+ 热门短链/流量结构/来源/设备/系统/浏览器分布(宽屏一行三列) |
| /links | 短链 | 列表(短码/目标/关联域名/状态/访问数)、创建(自定义或自动短码、多选域名、302/301)、编辑、启停、逻辑删除、彻底删除 |
| /stats | 统计 | 短链访问数与访问列表(时间、UA、来源) |
| /account | 账号设置 | 租户信息、配额用量、访客端错误页面、修改密码 |
| /admin/tenants | 平台管理(仅平台管理员) | 租户列表、封禁/解封、调整等级、移除域名 |

## 术语

遵循仓库 `CONTEXT.md` 词汇表:租户、平台管理员、等级、配额、短链、短码、目标 URL、重定向、域名、自有域名、平台默认域名、激活、访问、证书、后台。
