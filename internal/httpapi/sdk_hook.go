package httpapi

import "sync"

// SDKInjector 允许对基座原生生成的 sdk.js 进行包装与增强。
// 典型应用场景：包裹反无头浏览器爬虫探针、时区核验、Canvas 指纹提取脚本。
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

// WrapGeneratedSDK 执行包装逻辑，未配置注入器时原样返回基座 SDK。
func WrapGeneratedSDK(domain, code string, rawSDK string) string {
	sdkInjectorLock.RLock()
	defer sdkInjectorLock.RUnlock()
	if activeSDKInjector != nil {
		return activeSDKInjector.WrapSDK(domain, code, rawSDK)
	}
	return rawSDK
}
