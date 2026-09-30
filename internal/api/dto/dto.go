package dto

import "encoding/json"

// V1 是当前 DTO 协议版本。
const V1 = "v1"

// RequestV1 是跨边界（移动端 / 调试端）请求的通用信封。
type RequestV1 struct {
	Version   string          `json:"version"`
	CommandID string          `json:"command_id,omitempty"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// ResponseV1 是通用响应信封。
type ResponseV1 struct {
	Version   string `json:"version"`
	CommandID string `json:"command_id,omitempty"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	Data      any    `json:"data,omitempty"`
}
