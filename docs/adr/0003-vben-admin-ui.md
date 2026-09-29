# 后台 UI 采用 Vben Admin(Vue 3 SPA)

后台管理界面使用开源项目 Vben Admin(vue-vben-admin,Vue 3 + TypeScript)实现,作为独立前端,消费 Go 后端提供的 JSON API。

选择 Vue 3 SPA 而非 Go 服务端渲染(HTMX/templ):用户指定使用成熟的 Vue 后台管理框架,开箱即得表格、表单、权限等后台交互;前后端分离使 Go 后端成为 API-first 服务,同一套 API 也服务于将来租户的公开 REST API。

影响:前端作为独立应用构建与部署,不再内嵌进 Go 二进制(原 go:embed 方案已由 ADR-0006 取代);后台域名由 nginx 提供静态资源,`/api` 反代到 Go 后端;租户子域的短链跳转与落地页仍整站由 Go 后端承载。
