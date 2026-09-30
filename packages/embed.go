// Package packages 内嵌随引擎分发的内容包（目前只有 demo 世界包）。
package packages

import (
	"embed"
	"io/fs"
)

//go:embed all:demo
var embedded embed.FS

// Demo 返回 demo 世界包的只读文件系统（根目录即 manifest.yaml 所在目录）。
func Demo() fs.FS {
	sub, err := fs.Sub(embedded, "demo")
	if err != nil {
		panic(err) // 编译期内嵌，不可能失败
	}
	return sub
}
