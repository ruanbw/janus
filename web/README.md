# CLOAK 后台管理界面(web/)

CLOAK 短链服务的后台管理界面:基于 Vue 3 + TypeScript + Vite + Ant Design Vue 的单页应用(SPA),采用 Vben Admin 的工程形态(目录分层、Pinia 状态、路由守卫、Axios 封装),消费 Go 后端 RESTful API。

## 技术栈

- Vue 3.5 + TypeScript + Vite 6
- Ant Design Vue 4(组件库)
- Pinia(状态)、Vue Router 4(路由)、Axios(请求)

## 本地开发

```bash
pnpm install
pnpm dev        # http://localhost:5173,/api 代理到 http://localhost:8081
# 域名入口(SwitchHosts 配置后):http://app.cloak.test:5173(vite.config.ts 已放行)
```

环境要求:Postgres 与 Caddy 由 `docker compose up -d` 提供;Go 后端在终端启动并监听 `http://localhost:8081`(见仓库根 README §5.6:`CLOAK_COOKIE_SECURE=false CLOAK_ADDR=:8081 go run ./cmd/cloak`;本机 8080 被 nginx 占用)。

## 构建

```bash
pnpm build      # 产物输出到 dist/,生产环境经 go:embed 内嵌进 Go 二进制
```

## 认证与请求约定

- 会话:登录后后端 `Set-Cookie`(cloak_session,HTTP-only),请求自动携带(axios `withCredentials`)。
- CSRF:双提交 token——从 cookie `cloak_csrf` 读取值,写方法请求头附加 `X-CSRF-Token`(见 `src/utils/request.ts`)。
- 统一错误:非 2xx 响应解析 `{ code, message, details? }`,抛出 `ApiError`(见 `src/types/api.ts`),页面用 message 友好展示;401 自动回登录页。

## 页面

| 路由 | 页面 | 说明 |
| --- | --- | --- |
| /login | 登录 | 邮箱+密码,可勾选记住我(30 天) |
| /register | 注册 | 邮箱+密码+前缀(slug),注册后引导邮箱验证 |
| /verify-email | 邮箱验证 | 从验证链接取 token 调用验证 |
| /forgot-password、/reset-password | 密码重置 | |
| /domains | 域名 | 平台默认域名/自有域名、状态与证书状态、添加/重检/停用恢复/删除 |
| /links | 短链 | 列表(短码/目标/关联域名/状态/访问数)、创建(自定义或自动短码、多选域名、302/301)、编辑、启停、逻辑删除、彻底删除 |
| /stats | 统计 | 短链访问数与访问列表(时间、UA、来源) |
| /api-keys | API Key | 生成(明文仅展示一次)、吊销 |
| /account | 账号设置 | 租户信息、配额用量、自动生成短码长度、修改密码 |
| /admin/tenants | 平台管理(仅平台管理员) | 租户列表、封禁/解封、调整等级、移除域名 |

## 术语

遵循仓库 `CONTEXT.md` 词汇表:租户、平台管理员、等级、配额、短链、短码、目标 URL、重定向、域名、自有域名、平台默认域名、激活、访问、证书、后台。
