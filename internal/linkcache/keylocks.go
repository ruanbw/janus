package linkcache

import "sync"

// keyLocks 按 (domainID, code) 分片的加载锁:每个 key 一把,互不阻塞,且会在没人用时回收。
//
// 为什么不用一把全局锁:回源是一次数据库往返(JOIN + link_targets,毫秒级到超时级)。
// 一把全局锁意味着**一条冷门短链的慢查询会阻塞全平台所有冷启动的跳转**,
// 而它们在语义上毫无关联。
//
// 为什么不用 golang.org/x/sync/singleflight:它在 go.mod 里只是 indirect,提为直接依赖
// 要动 go.mod/go.sum(本波明确不新增依赖);而且它按 key 合并的是"结果",本包需要的是
// "同一 key 串行 + 拿到锁后复查缓存"——复查这一步同时承担了失效语义,自己写更直白。
//
// 回收:refs 归零(既没有持有者也没有等待者)时把这条从表里摘掉,
// 免得长期运行按"历史出现过的短码数"无界增长。
type keyLocks struct {
	mu    sync.Mutex
	locks map[key]*keyLock
}

type keyLock struct {
	mu   sync.Mutex
	refs int
}

// lock 锁住某个 key,返回解锁函数(必须调用)。
func (l *keyLocks) lock(k key) func() {
	l.mu.Lock()
	if l.locks == nil {
		l.locks = make(map[key]*keyLock)
	}
	kl := l.locks[k]
	if kl == nil {
		kl = &keyLock{}
		l.locks[k] = kl
	}
	// refs 在表锁内自增:等锁的 goroutine 也持有一份引用,
	// 于是持有者解锁时表里这条一定还在,不会出现"有人等着一把已被摘掉的锁"。
	kl.refs++
	l.mu.Unlock()

	kl.mu.Lock()
	return func() {
		kl.mu.Unlock()
		l.mu.Lock()
		kl.refs--
		if kl.refs == 0 {
			delete(l.locks, k)
		}
		l.mu.Unlock()
	}
}
