package rules

import (
	"context"
	"net/http"

	"janus/pkg/plugin"
)

// 公开扩展 API(pkg/plugin)与基座内部注册表的接线。
//
// 外部模块受 Go internal 规则限制,只能通过 pkg/plugin 注册扩展;基座热路径读的是
// 本包自己的注册表。两边不接线,公开 API 注册的插件就永远不会被调用。
// 顺序约定:先跑内部注册表,再跑 pkg/plugin 注册表;panic 隔离两边各自负责。

func toPluginFact(f Fact) plugin.Fact {
	return plugin.Fact{
		IP: f.IP, IPAttr: f.IPAttr, Country: f.Country, ASN: f.ASN, Lang: f.Lang,
		Ref: f.Ref, UTM: f.UTM, UA: f.UA, DevType: f.DevType, OS: f.OS,
		Browser: f.Browser, Path: f.Path, Domain: f.Domain,
	}
}

// mergePluginFact 把插件富化后的公开字段写回内部画像(netIP/Seen 等内部字段保持不变)。
func mergePluginFact(dst *Fact, p plugin.Fact) {
	dst.IP, dst.IPAttr, dst.Country, dst.ASN, dst.Lang = p.IP, p.IPAttr, p.Country, p.ASN, p.Lang
	dst.Ref, dst.UTM, dst.UA, dst.DevType, dst.OS = p.Ref, p.UTM, p.UA, p.DevType, p.OS
	dst.Browser, dst.Path, dst.Domain = p.Browser, p.Path, p.Domain
}

func fromPluginDecision(d plugin.Decision) Decision {
	return Decision{
		RuleID: d.RuleID, Name: d.Name, Action: d.Action, Destination: d.Destination,
		Priority: d.Priority, PageMode: d.PageMode, CustomHTML: d.CustomHTML,
	}
}

// ToPluginDecision 内部裁决 → 公开 DTO(httpapi 交付扩展点用)。
func ToPluginDecision(d Decision) plugin.Decision {
	return plugin.Decision{
		RuleID: d.RuleID, Name: d.Name, Action: d.Action, Destination: d.Destination,
		Priority: d.Priority, PageMode: d.PageMode, CustomHTML: d.CustomHTML,
	}
}

// applyPluginEnrichers 执行 pkg/plugin 注册的富化器;未注册时不做任何转换。
func applyPluginEnrichers(ctx context.Context, r *http.Request, fact *Fact) {
	if !plugin.HasEnrichers() {
		return
	}
	pf := toPluginFact(*fact)
	plugin.ApplyEnrichers(ctx, r, &pf)
	mergePluginFact(fact, pf)
}

// checkPluginInterceptors 执行 pkg/plugin 注册的拦截器;未注册时直接放行。
func checkPluginInterceptors(ctx context.Context, r *http.Request, linkID int64, fact *Fact) (*Decision, bool) {
	if !plugin.HasInterceptors() {
		return nil, false
	}
	pf := toPluginFact(*fact)
	pd, ok := plugin.CheckInterceptors(ctx, r, linkID, &pf)
	if !ok || pd == nil {
		return nil, false
	}
	d := fromPluginDecision(*pd)
	return &d, true
}
