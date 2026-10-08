// Package rbac 实现基于 Casbin 的角色访问控制(RBAC):model 与策略均从内存文本
// 装载(不落盘、无外部依赖),按 (角色, HTTP 方法, 路径) 三元组判定租户(tenant)
// 与超管(superadmin)对 /api 路由的访问权限,供 gin-contrib/authz 等集成方使用。
package rbac

import (
	"fmt"
	"strings"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist/string-adapter"
)

const (
	// RoleTenant 普通租户:按显式路由矩阵放行,不能访问平台管理(/api/admin)路由。
	RoleTenant = "tenant"
	// RoleSuperadmin 平台超管:/api/* 全通(见 policyText 中一条通配策略)。
	RoleSuperadmin = "superadmin"
)

// modelText 是 Casbin 的 model 定义(与共享契约一致)。
// request_definition 三元组依次为:sub=主体(角色)、obj=客体(路径)、act=动作(HTTP 方法);
// matcher 用 keyMatch3 做 RESTful 路径匹配(* 贪婪匹配任意后缀),动作支持策略值 "*"(通配所有方法)。
const modelText = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == p.sub && keyMatch3(r.obj, p.obj) && (r.act == p.act || p.act == "*")
`

// policyText 是内存策略矩阵(与共享契约一致):
//   - tenant 按契约逐条放行(注意 keyMatch3 中 "/api/domains" 与 "/api/domains/*" 是两个条目,
//     裸路径不匹配带 "/*" 的条目,故需要成对列出);
//   - superadmin 一条 "p, superadmin, /api/*, *" 即覆盖 /api 下全部路径与全部方法。
const policyText = `
p, tenant, /api/auth/logout, POST
p, tenant, /api/auth/me, GET
p, tenant, /api/auth/change-password, POST
p, tenant, /api/me, GET
p, tenant, /api/me/error-pages, GET
p, tenant, /api/me/error-pages, PATCH
p, tenant, /api/config, GET
p, tenant, /api/domains, GET
p, tenant, /api/domains, POST
p, tenant, /api/domains/*, GET
p, tenant, /api/domains/*, PATCH
p, tenant, /api/domains/*, DELETE
p, tenant, /api/domains/*/recheck, POST
p, tenant, /api/links, GET
p, tenant, /api/links, POST
p, tenant, /api/links/*, GET
p, tenant, /api/links/*, PATCH
p, tenant, /api/links/*, DELETE
p, tenant, /api/links/*/purge, POST
p, tenant, /api/links/batch-delete, POST
p, tenant, /api/links/batch-purge, POST
p, tenant, /api/links/*/landing, POST
p, tenant, /api/links/*/visits, GET
p, tenant, /api/links/*/stats, GET
p, tenant, /api/visits/overview, GET
p, tenant, /api/links/*/rules, GET
p, tenant, /api/links/*/rules, PUT
p, tenant, /api/rules, GET
p, tenant, /api/rules, POST
p, tenant, /api/rules/options, GET
p, tenant, /api/rules/simulate, POST
p, tenant, /api/rules/validate-expr, POST
p, tenant, /api/rules/*, GET
p, tenant, /api/rules/*, PATCH
p, tenant, /api/rules/*, DELETE
p, superadmin, /api/*, *
`

// Enforcer 封装底层 Casbin enforcer,对外暴露按 (role, method, path) 判权的入口。
// 底层 enforcer 线程安全,可被多个请求并发调用。
type Enforcer struct {
	e *casbin.Enforcer
}

// New 构建 RBAC enforcer:model 与策略均从内存文本装载(不落盘)。
// 构建失败(如 model/策略文本非法)时返回错误,由调用方传导;不 panic。
func New() (*Enforcer, error) {
	m, err := model.NewModelFromString(modelText)
	if err != nil {
		return nil, fmt.Errorf("rbac: 解析 model 失败: %w", err)
	}
	a := stringadapter.NewAdapter(policyText)
	e, err := casbin.NewEnforcer(m, a)
	if err != nil {
		return nil, fmt.Errorf("rbac: 构建 enforcer 失败: %w", err)
	}
	return &Enforcer{e: e}, nil
}

// Enforcer 返回底层 *casbin.Enforcer,供 gin-contrib/authz 等集成方直接使用。
func (en *Enforcer) Enforcer() *casbin.Enforcer {
	return en.e
}

// Enforce 判定角色 role 是否被允许以 HTTP 方法 method 访问路径 path。
// 返回 false 即未授权(策略未放行);底层评估异常时按拒绝处理(鉴权默认安全)。
func (en *Enforcer) Enforce(role, method, path string) bool {
	ok, err := en.e.Enforce(role, path, method)
	if err != nil {
		return false
	}
	return ok
}

// AdminPathPrefix 是平台管理命名空间:基座的 /api/admin/* 与扩展挂在其下的路由都只对超管开放。
const AdminPathPrefix = "/api/admin"

// GrantTenantExtension 为外部注入的受保护扩展路由(janus.WithProtectedRoutes)登记租户策略。
//
// 默认策略(取最安全且仍可用的口径):
//   - superadmin:已由 "p, superadmin, /api/*, *" 覆盖,无需登记;
//   - tenant:**仅放行扩展实际注册的 (方法, 路径)**,逐条登记,而不是给一整个前缀开通配;
//   - 扩展路由落在 /api/admin 命名空间下,或首段就是参数/通配(如 /api/:x/...、/api/*any,
//     可以匹配到 /api/admin/...)→ 不登记租户策略,视为超管专属。扩展若要声明
//     「仅超管可用」,把路由挂在 /api/admin/ 下即可。
//
// ginPath 为 Gin 路由路径(含 :param / *param),内部转换为 keyMatch3 语法。
// 返回 true 表示已为 tenant 放行,false 表示按超管专属处理(未登记)。
// 须在开始处理请求前调用(启动期挂载路由时)。
func (en *Enforcer) GrantTenantExtension(method, ginPath string) (bool, error) {
	obj := GinPathToPolicyPath(ginPath)
	if !tenantGrantable(obj) {
		return false, nil
	}
	if _, err := en.e.AddPolicy(RoleTenant, obj, method); err != nil {
		return false, fmt.Errorf("rbac: 登记扩展路由策略失败(%s %s): %w", method, ginPath, err)
	}
	return true, nil
}

// tenantGrantable 判断某条策略路径能否安全地对 tenant 放行:
// 必须在 /api/ 下,且首段是字面量、不是 admin(参数/通配首段可能命中 /api/admin/...)。
func tenantGrantable(obj string) bool {
	rest, ok := strings.CutPrefix(obj, "/api/")
	if !ok {
		return false
	}
	first, _, _ := strings.Cut(rest, "/")
	if first == "" || first == "admin" || strings.ContainsAny(first, "{}*") {
		return false
	}
	return true
}

// GinPathToPolicyPath 将 Gin 路由路径中的动态参数转换为 Casbin keyMatch3 语法:
// ":param" → "{param}"(匹配单段),"*param" → "*"(匹配任意后缀)。
func GinPathToPolicyPath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "{" + strings.TrimPrefix(part, ":") + "}"
		} else if strings.HasPrefix(part, "*") {
			parts[i] = "*"
		}
	}
	return strings.Join(parts, "/")
}
