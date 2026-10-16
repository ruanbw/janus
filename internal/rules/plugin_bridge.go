package rules

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"time"

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

// mergePluginFact 把插件富化后的公开字段写回内部画像(Seen/hdr/rawQuery 等内部字段保持不变)。
//
// IP 与派生字段 ipattr 必须一起处理,口径与 LazyVisitorContext.backfillEnriched 完全一致 ——
// 两条富化路径必须给出同一个画像,否则同一份插件配置经不同入口得到的裁决不同。
//
//   - IP:插件写了且与基线不同才算改写。解析成功先 Unmap 再 String() 归一后回填
//     (v4-mapped 的 ::ffff:1.2.3.4 必须变成 1.2.3.4),否则 Field("ip") 返回原串而
//     ClientIP() 归一,`ip eq` 与 `ip in_cidr` 会对同一个访客给出相反结论;
//     解析不出来(插件写坏了)则**只回退 IP**:宁可忽略这次改写,也不能让画像里的 IP
//     变成一个无法求值的字符串。
//   - ipattr:插件显式写入(与基线不同)优先,与 IP 能否解析无关 —— 显式写的是 ipattr,
//     不该被一次坏 IP 连带丢弃。只改了 IP 则按新 IP 重算,避免留下
//     "公网 IP + private 属性"的自相矛盾画像;其余情况保留基线。
//   - 空串 = 插件没写(清空不是改写):保留基线,也不打日志。
func mergePluginFact(dst *Fact, p plugin.Fact) {
	baseIP, baseIPAttr := dst.IP, dst.IPAttr
	dst.IPAttr, dst.Country, dst.ASN, dst.Lang = p.IPAttr, p.Country, p.ASN, p.Lang
	dst.Ref, dst.UTM, dst.UA, dst.DevType, dst.OS = p.Ref, p.UTM, p.UA, p.DevType, p.OS
	dst.Browser, dst.Path, dst.Domain = p.Browser, p.Path, p.Domain

	ipChanged := false
	if p.IP != "" && p.IP != baseIP {
		if addr, err := netip.ParseAddr(p.IP); err == nil {
			addr = addr.Unmap()
			dst.IP = addr.String()
			dst.netIP = net.IP(addr.AsSlice())
			ipChanged = true
		} else {
			dst.IP = baseIP
			slog.Warn("插件富化器写入的 IP 无法解析,保留基线 IP", "base", baseIP, "plugin", p.IP)
		}
	}
	switch {
	case p.IPAttr != "" && p.IPAttr != baseIPAttr:
		// 插件显式写入(ADR-0012 的机房/代理标记):以它为准。
		dst.IPAttr = p.IPAttr
	case ipChanged:
		// 插件只改了 IP:ipattr 必须按新 IP 重算,哪怕重算结果为空。
		dst.IPAttr = ipAttrFromAddr(dst.ClientIP())
	}
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
	start := time.Now()
	plugin.ApplyEnrichers(ctx, r, &pf)
	// pkg/plugin 内部逐个调用富化器,基座拿不到单个插件名,只能测聚合耗时;
	// 超阈值时用固定标识定位到"是 pkg/plugin 这一侧慢"(见 watchPluginCall)。
	watchPluginCall("enricher", "pkg/plugin", start)
	mergePluginFact(fact, pf)
}

// checkPluginInterceptors 执行 pkg/plugin 注册的拦截器;未注册时直接放行。
func checkPluginInterceptors(ctx context.Context, r *http.Request, linkID int64, fact *Fact) (*Decision, bool) {
	if !plugin.HasInterceptors() {
		return nil, false
	}
	pf := toPluginFact(*fact)
	start := time.Now()
	pd, ok := plugin.CheckInterceptors(ctx, r, linkID, &pf)
	watchPluginCall("interceptor", "pkg/plugin", start)
	if !ok || pd == nil {
		return nil, false
	}
	d := fromPluginDecision(*pd)
	return &d, true
}
