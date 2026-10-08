package httpapi

import (
	"log/slog"
	"runtime/debug"
	"sync"

	"janus/pkg/plugin"
)

// SDKInjector 允许对基座原生生成的 sdk.js 进行包装与增强。
// 典型应用场景：包裹额外的客户端采集脚本或对基座 SDK 做定制。
type SDKInjector interface {
	Name() string
	WrapSDK(domain string, code string, rawSDK string) string
}

var (
	sdkInjectorLock   sync.RWMutex
	activeSDKInjector SDKInjector
)

// SetSDKInjector 设置活跃的 SDK 注入器。
func SetSDKInjector(injector SDKInjector) {
	sdkInjectorLock.Lock()
	defer sdkInjectorLock.Unlock()
	activeSDKInjector = injector
}

// ResetSDKInjector 重置 SDK 注入器（测试隔离用）。
func ResetSDKInjector() {
	sdkInjectorLock.Lock()
	defer sdkInjectorLock.Unlock()
	activeSDKInjector = nil
}

// WrapGeneratedSDK 执行包装逻辑，未配置注入器（或注入器 panic）时原样返回基座 SDK。
//
// 注入器是外部注入的代码：panic 一律回退到基座 SDK，因为基座的点击上报能力
// 不能因为注入器炸了就一起丢。读锁只用来取引用，调用插件时不持锁——
// 否则插件在自己的 WrapSDK 里注册/重置注入器会自锁。
func WrapGeneratedSDK(domain, code string, rawSDK string) (out string) {
	sdkInjectorLock.RLock()
	injector := activeSDKInjector
	sdkInjectorLock.RUnlock()
	if injector == nil {
		// 内部未设置时交给公开 API(pkg/plugin)的注入器;未设置时它原样返回。
		return plugin.WrapGeneratedSDK(domain, code, rawSDK)
	}
	defer func() {
		if rec := recover(); rec != nil {
			out = rawSDK
			slog.Error("SDK 注入器 panic,回退基座 SDK(fail-open)",
				"injector", injector.Name(), "panic", rec, "stack", string(debug.Stack()))
		}
	}()
	return injector.WrapSDK(domain, code, rawSDK)
}

// mutate4go-manifest-begin
// {"version":1,"tested_at":"2026-10-04T13:26:25+08:00","module_hash":"7454bc91e94ac38cecee755a896d735469168fb8938de4c067332ce4d5b98ce2","functions":[{"id":"func/SetSDKInjector","name":"SetSDKInjector","line":22,"end_line":26,"hash":"657711e9f614b3835ed5b6be94aebf9e419581a0c804e0e5cef36c67a3031537"},{"id":"func/ResetSDKInjector","name":"ResetSDKInjector","line":29,"end_line":33,"hash":"d7df9207b23f4f73dbd0f39b5b076e281664b1a29cb3b7d0c5675bf8dc03f63f"},{"id":"func/WrapGeneratedSDK","name":"WrapGeneratedSDK","line":40,"end_line":55,"hash":"148c1ae8e99c188f5c6058012672bc4b8a1af256f5d3a9ff3d3283c786fd837a"}]}
// mutate4go-manifest-end
