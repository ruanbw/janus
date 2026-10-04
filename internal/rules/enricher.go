package rules

import (
	"context"
	"net/http"
	"sync"
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

// ApplyEnrichers 依次执行已注册的富化器。
func ApplyEnrichers(ctx context.Context, r *http.Request, fact *Fact) {
	enrichersLock.RLock()
	if len(enrichers) == 0 {
		enrichersLock.RUnlock()
		return
	}
	list := make([]FactEnricher, len(enrichers))
	copy(list, enrichers)
	enrichersLock.RUnlock()

	for _, e := range list {
		e.Enrich(ctx, r, fact)
	}
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
	return false
}
