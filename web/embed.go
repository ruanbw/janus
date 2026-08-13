// Package webui 将前端构建产物(web/dist)内嵌进 Go 二进制(spec 决策 #1、ADR-0003):
// 单二进制部署,后台域名下除 /api 与 /internal 外的请求由 SPA 处理(含 history 路由回退)。
// 注意:embed 模式不能引用包目录之外的文件,因此本包放在 web/ 下(与 dist 同目录)。
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler 返回内嵌前端静态文件处理器;未知路径回退到 index.html(SPA 路由)。
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" {
			if _, err := fs.Stat(sub, p); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// SPA fallback:未命中的路径一律返回 index.html
		http.ServeFileFS(w, r, sub, "index.html")
	})
}
