// Package mobile 是供 gomobile bind 使用的 Android / iOS 桥接层。
//
// 只导出 gomobile 支持的类型（string），请求与响应均为版本化 JSON DTO。
// 构建：gomobile bind -target=android -androidapi 24 -javapkg com.guyu2233.ibukirpg -o build/android/ibukirpg.aar ./mobile
package mobile

import (
	"context"

	adapter "github.com/GUYU2233/ibukiRPG/internal/adapter/mobile"
	"github.com/GUYU2233/ibukiRPG/internal/buildinfo"
)

// Version 返回引擎版本。
func Version() string { return buildinfo.String() }

// Handle 处理 JSON 请求（RequestV1）并返回 JSON 响应（ResponseV1）。
func Handle(requestJSON string) string {
	return adapter.Handle(context.Background(), requestJSON)
}
