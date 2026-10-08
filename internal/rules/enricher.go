package rules

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"

	"janus/pkg/plugin"
)

// FactEnricher 用于在规则求值前，对基础访客画像进行扩展富化。
// 必须严格遵守：纯内存或本地离线库计算，耗时必须在微秒级，严禁引入数据库查询。
type FactEnricher interface {
	Name() string
	Enrich(ctx context.Context, r *http.Request, fact *Fact)
}

var (
	enrichersLock sync.RWMutex
	enrichers     []FactEnricher
)

// RegisterEnricher 注册外部画像富化器。
func RegisterEnricher(e FactEnricher) {
	if e == nil {
		return
	}
	enrichersLock.Lock()
	defer enrichersLock.Unlock()
	enrichers = append(enrichers, e)
}

// ResetEnrichers 清空已注册的富化器（测试隔离用）。
func ResetEnrichers() {
	enrichersLock.Lock()
	defer enrichersLock.Unlock()
	enrichers = nil
}

// enricherSnapshot 取注册表的快照副本。
// 读锁只用来拷贝,不跨插件调用:插件在 Enrich 里反过来注册/重置插件时不会自锁。
func enricherSnapshot() []FactEnricher {
	enrichersLock.RLock()
	defer enrichersLock.RUnlock()
	if len(enrichers) == 0 {
		return nil
	}
	list := make([]FactEnricher, len(enrichers))
	copy(list, enrichers)
	return list
}

// enrichOne 执行单个富化器,并把 panic 关在它自己身上。
// 关键不变式:富化器是外部注入的代码,panic 绝不允许冒到访客热路径 ——
// 冒出去只会被全局中间件兜成 500,而正确的退化是"这个维度当没有"。
// recover 之后继续跑后面的富化器:一个插件炸了不等于整次富化作废。
func enrichOne(e FactEnricher, ctx context.Context, r *http.Request, fact *Fact) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("画像富化器 panic,按未富化处理(fail-open)",
				"enricher", e.Name(), "panic", rec, "stack", string(debug.Stack()))
		}
	}()
	e.Enrich(ctx, r, fact)
}

// ApplyEnrichers 依次执行已注册的富化器。
func ApplyEnrichers(ctx context.Context, r *http.Request, fact *Fact) {
	for _, e := range enricherSnapshot() {
		enrichOne(e, ctx, r, fact)
	}
	applyPluginEnrichers(ctx, r, fact)
}

// HasASNProvider 判断是否已注册了能够提供 ASN 数据的画像富化器。
func HasASNProvider() bool {
	enrichersLock.RLock()
	defer enrichersLock.RUnlock()
	for _, e := range enrichers {
		if e.Name() == "asn" || e.Name() == "asn_enricher" {
			return true
		}
	}
	return plugin.HasASNProvider()
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-10-04T13:22:34+08:00","module_hash":"19540c4d1435046143bfe798887d7e6b04b5eeaa737e24155d30c883e100af7a","functions":[{"id":"func/RegisterEnricher","name":"RegisterEnricher","line":24,"end_line":31,"hash":"b268daf63ed16c0301f88813e1c9017718ceeff71ba9ee95b846163cb8fa823f"},{"id":"func/ResetEnrichers","name":"ResetEnrichers","line":34,"end_line":38,"hash":"05c618acc30c3743a35a34daf75b3ca706ffeba04630d0b31ed4d24fa7a84938"},{"id":"func/enricherSnapshot","name":"enricherSnapshot","line":42,"end_line":51,"hash":"6e96134b9896167c0c675eebf03b8f62485c8ed6019ca5f6c032d4c071a71797"},{"id":"func/enrichOne","name":"enrichOne","line":57,"end_line":65,"hash":"f5f8f2170f2cd16a453782f838abc86b875cd0bd4a8f556e312a37c9df801846"},{"id":"func/ApplyEnrichers","name":"ApplyEnrichers","line":68,"end_line":72,"hash":"33da766c39463d80d87c726c2e940a66e1b505aca525598f1f11e1a9199cf472"},{"id":"func/HasASNProvider","name":"HasASNProvider","line":75,"end_line":84,"hash":"b895f047e9d30ae679a093d8ba45ba79af54be929deb104457e22f2c14fe10dc"}]}
// mutate4go-manifest-end
