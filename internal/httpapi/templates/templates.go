package templates

import (
	_ "embed"
)

//go:embed default_404.html
var default404HTML string

//go:embed default_429.html
var default429HTML string

// Default404HTML 返回系统内置的默认 404 HTML 页面内容。
func Default404HTML() string {
	return default404HTML
}

// Default429HTML 返回系统内置的默认 429 HTML 页面内容。
func Default429HTML() string {
	return default429HTML
}
