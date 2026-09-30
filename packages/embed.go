// Package packages 内嵌随引擎分发的内置故事包（Android 上它们随引擎库一起打进 APK）。
package packages

import (
	"embed"
	"io/fs"
)

//go:embed all:demo all:lighthouse
var embedded embed.FS

// BuiltinDirs 是内置故事包目录，顺序即故事包列表中的顺序（第一个是默认故事包）。
var BuiltinDirs = []string{"demo", "lighthouse"}

func sub(dir string) fs.FS {
	s, err := fs.Sub(embedded, dir)
	if err != nil {
		panic(err) // 编译期内嵌，不可能失败
	}
	return s
}

// Demo 返回默认故事包「边境酒馆」的只读文件系统（根目录即 manifest.yaml 所在目录）。
func Demo() fs.FS { return sub("demo") }

// Builtin 返回全部内置故事包。
func Builtin() []fs.FS {
	out := make([]fs.FS, 0, len(BuiltinDirs))
	for _, d := range BuiltinDirs {
		out = append(out, sub(d))
	}
	return out
}
