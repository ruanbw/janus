package domain

// 后台任务队列的停机语义测试。
//
// 队列挂在 DNSChecker 上(httpapi 一个 API 实例 = 一个校验器 = 一份队列),而
// 没有 Close 就意味着每个实例留下 defaultTaskWorkers 个永不退出的 goroutine;
// 任务拿到的又是 context.Background(),Close 也没法让在跑的任务早点收手。

import (
	"context"
	"sync"
	"testing"
	"time"
)

// 任务必须拿到队列的上下文(而不是 context.Background())。
func TestTaskQueuePassesQueueContext(t *testing.T) {
	q := NewTaskQueue(1, 1)
	defer q.Close()
	got := make(chan context.Context, 1)
	if !q.Submit("k", func(ctx context.Context) { got <- ctx }) {
		t.Fatal("Submit = false, want true")
	}
	select {
	case ctx := <-got:
		if ctx == nil {
			t.Fatal("任务收到的 ctx 为 nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("任务未执行")
	}
}

// Close 必须取消传给任务的 ctx:在跑的任务靠它感知停机。
func TestTaskQueueCloseCancelsRunningTask(t *testing.T) {
	q := NewTaskQueue(1, 1)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	q.Submit("k", func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(cancelled)
	})
	<-started

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		q.Close()
	}()

	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("Close 之后任务上下文未被取消:任务拿不到停机信号,只能等它自己跑完")
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close 未在任务结束后返回")
	}
}

// Close 要等正在执行的任务真正结束(不能提前返回把调用方放走)。
func TestTaskQueueCloseWaitsForRunningTask(t *testing.T) {
	q := NewTaskQueue(1, 1)
	started := make(chan struct{})
	finished := make(chan struct{})
	q.Submit("k", func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		time.Sleep(20 * time.Millisecond)
		close(finished)
	})
	<-started
	q.Close()
	select {
	case <-finished:
	default:
		t.Fatal("Close 在任务结束前就返回了")
	}
}

// Close 之后 Submit 必须安静地拒绝(而不是往已关闭的 channel 发送 panic)。
func TestTaskQueueSubmitAfterClose(t *testing.T) {
	q := NewTaskQueue(1, 1)
	q.Close()
	if q.Submit("k", func(context.Context) {}) {
		t.Fatal("Close 之后 Submit = true, want false")
	}
	// 幂等:重复 Close 不能 panic
	q.Close()
}

// 单例语义不能被停机改动带走:同一个 key 同时只跑一个任务。
func TestTaskQueueDeduplicatesByKey(t *testing.T) {
	q := NewTaskQueue(1, 1)
	defer q.Close()
	release := make(chan struct{})
	var mu sync.Mutex
	runs := 0
	if !q.Submit("dup", func(context.Context) {
		mu.Lock()
		runs++
		mu.Unlock()
		<-release
	}) {
		t.Fatal("首次 Submit = false, want true")
	}
	// 等它真的开始跑,再提交同 key
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := runs
		mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("任务未开始执行")
		}
		time.Sleep(time.Millisecond)
	}
	if q.Submit("dup", func(context.Context) { t.Error("重复 key 的任务不应被执行") }) {
		t.Fatal("同 key 的重复 Submit = true, want false(single-flight)")
	}
	close(release)
}
