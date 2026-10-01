# Go 作为后端技术栈

我们选择 Go 作为 Janus 的后端语言。它是多租户自托管短链服务的核心——在 Docker Compose 里以单个二进制运行、位于 Caddy 之后,负责租户管理、域名激活校验、跳转路由与后台 API。

选择 Go 而非 Node/TypeScript 或 Python:单二进制部署与运维成本最低;Caddy on-demand TLS 的授权端点(ask endpoint)在 Go 里实现极轻;并发模型适合承担公开跳转流量;标准库对 HTTP/TLS 支持成熟。
