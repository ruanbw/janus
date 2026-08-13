# Caddy 前置,以 on-demand TLS 为租户域名自动签发证书

TLS 终止与证书生命周期交给前置的 Caddy:CLOAK 采用 Caddy on-demand TLS + 授权端点(ask endpoint)。租户域名在数据库中被标记为"激活"后,Caddy 收到该域名的首个请求时向应用的授权端点询问该域名是否已激活,是则自动签发并自动续期 Let's Encrypt 证书;应用不直接操作 Caddy 配置。

选择 Caddy 而非应用内置 ACME(如 autocert/lego):用户希望把证书签发与续期交给成熟组件,避免自己维护 ACME 状态机与续签定时任务;on-demand 授权端点同时充当防滥用闸门——未被激活的域名(例如他人把任意域名解析到本服务器)不会触发签发。
