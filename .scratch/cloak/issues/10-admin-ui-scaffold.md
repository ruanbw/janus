# 10 — admin-ui-scaffold(后台界面:Vben Admin 脚手架 + 登录注册)

**What to build:** Vben Admin 项目初始化,接入 Go API(会话鉴权、请求封装、统一错误),登录/注册/登出页面可用。

**Blocked by:** 02

**Status:** resolved

- [ ] Vben Admin 项目初始化,可本地开发并构建
- [ ] 请求封装接入 Go API(会话鉴权、统一错误展示)
- [ ] 登录/注册/登出页面可完成完整流程(注册含 slug 与邮箱验证引导)

## Comments

- 已实现:web/ 前端项目(Vue 3 + TypeScript + Vite + Ant Design Vue 4,采用 Vben Admin 工程形态:Pinia 状态、路由守卫、Axios 封装、分层 views/api/utils/constants)。因执行环境无 shell(子代理 bash 被移除),无法运行 `pnpm create vben`,已手写等价骨架(package.json/vite.config.ts/tsconfig/index.html 等)。
- 请求封装(`web/src/utils/request.ts`):会话 cookie 自动携带(withCredentials)、CSRF 双提交 token(从 cookie `cloak_csrf` 读取,写方法附加 `X-CSRF-Token` 头)、统一错误 `{code, message, details?}` 解析为 `ApiError` 并用 message 友好展示、401 自动回登录页。
- 页面:登录(含"记住我"30 天)、注册(email+password+slug,slug 前端规则校验:小写字母/数字开头结尾可含连字符,409/400 错误按后端 message 展示)、邮箱验证引导(说明验证链接在 `docker logs cloak-backend-1`)、忘记/重置密码、登出(顶栏)、修改密码(含超管首次登录无 oldPassword)。
- 用到的契约端点:POST /api/auth/register、POST /api/auth/verify-email、POST /api/auth/login、POST /api/auth/logout、GET /api/auth/me、POST /api/auth/change-password、POST /api/auth/forgot-password、POST /api/auth/reset-password。
- 待联调:本环境无 shell 无法 `pnpm install`/`pnpm dev`/真实后端联调,需主会话在 web/ 下执行 `pnpm install && pnpm dev`(代理 http://localhost:8081)与 `pnpm build` 验证;注册→提取验证 token→验证→登录流程需主会话配合 docker logs 联调。
