// Package buildinfo 提供构建版本信息（可通过 -ldflags 注入）。
package buildinfo

// Name 项目名称。
const Name = "ibukiRPG"

// 以下变量可在构建时通过 -ldflags "-X" 覆盖。
var (
	// Version 引擎版本号。
	Version = "0.2.0-rc1"
	// Commit 构建所用 git 提交。
	Commit = "unknown"
)

// String 返回可读的版本字符串。
func String() string {
	return Name + " " + Version + " (" + Commit + ")"
}
