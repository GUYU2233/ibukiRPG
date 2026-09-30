// Package mobile 是供 gomobile bind 使用的 Android / iOS 桥接层。
//
// 只导出 gomobile 支持的类型（string 与接口），请求与响应均为版本化 JSON DTO（第 49 节）。
// 构建：gomobile bind -target=android -androidapi 24 -javapkg com.guyu2233.ibukirpg -o build/android/ibukirpg.aar ./mobile
//
// 调用约定：
//   - Handle 是同步阻塞调用，Android 端请在后台线程（Dispatchers.IO）调用；
//   - submit_text / quick_action 执行期间，叙事增量通过 EventSink.OnEvent 推送（StreamEventV1 JSON）；
//   - 首次调用前先发送 {"version":"v1","type":"init","payload":{"data_dir":"..."}}。
package mobile

import (
	"context"

	adapter "github.com/GUYU2233/ibukiRPG/internal/adapter/mobile"
	"github.com/GUYU2233/ibukiRPG/internal/buildinfo"
)

// EventSink 接收流式事件（JSON）。在 Kotlin 中实现该接口并通过 SetEventSink 注册。
type EventSink interface {
	OnEvent(eventJSON string)
}

// Version 返回引擎版本。
func Version() string { return buildinfo.String() }

// Handle 处理 JSON 请求（RequestV1）并返回 JSON 响应（ResponseV1）。
func Handle(requestJSON string) string {
	return adapter.Handle(context.Background(), requestJSON)
}

// SetEventSink 注册流式事件接收者；传 nil 取消注册。
func SetEventSink(s EventSink) {
	if s == nil {
		adapter.SetSink(nil)
		return
	}
	adapter.SetSink(s.OnEvent)
}
