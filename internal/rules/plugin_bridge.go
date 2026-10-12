package rules

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/netip"

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
// IP 是唯一需要特殊处理的字段:Fact.ClientIP() 优先返回随画像解析好的 netIP,
// 只写 dst.IP 会让「ip in_cidr」这类条件继续按**旧**地址判定 —— 插件改了 IP,
// 规则却看不见。所以 IP 变了就同步重解析 netIP;解析不出来(插件写了非法值)
// 则整条保留基线 IP:宁可忽略这次改写,也不能让画像里的 IP 变成一个无法求值的字符串。
//
// ipattr 是 IP 的派生字段,但富化器也可以显式写(ADR-0012 的机房/代理标记)。
// 判定口径:插件写的 ipattr 与基线不同 = 显式写入 → 尊重;
// 没写(值不变)而 IP 变了 → 按新 IP 重算,避免留下"公网 IP + private 属性"的自相矛盾画像。
func mergePluginFact(dst *Fact, p plugin.Fact) {
	baseIP, baseIPAttr := dst.IP, dst.IPAttr
	dst.IPAttr, dst.Country, dst.ASN, dst.Lang = p.IPAttr, p.Country, p.ASN, p.Lang
	dst.Ref, dst.UTM, dst.UA, dst.DevType, dst.OS = p.Ref, p.UTM, p.UA, p.DevType, p.OS
	dst.Browser, dst.Path, dst.Domain = p.Browser, p.Path, p.Domain

	if p.IP == baseIP || p.IP == "" {
		// 空串 = 插件没写 IP(清空不是改写):保留基线,也不打日志。
		return
	}
	addr, err := netip.ParseAddr(p.IP)
	if err != nil {
		dst.IP, dst.IPAttr = baseIP, baseIPAttr
		slog.Warn("插件富化器写入的 IP 无法解析,保留基线 IP", "base", baseIP, "plugin", p.IP)
		return
	}
	dst.IP = p.IP
	dst.netIP = net.IP(addr.AsSlice())
	if p.IPAttr == baseIPAttr {
		dst.IPAttr = ipAttr(p.IP)
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
