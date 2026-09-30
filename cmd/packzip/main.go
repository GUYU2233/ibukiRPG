// Command packzip 校验并打包故事包目录为可导入的 .zip（跨平台，不依赖系统 zip 命令）。
//
//	go run ./cmd/packzip -o build/packs/fog_lighthouse.zip packages/lighthouse
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/package/manifest"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
)

func main() {
	out := flag.String("o", "", "输出 .zip 路径（默认 build/packs/<id>-<version>.zip）")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法：packzip [-o out.zip] <故事包目录>")
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *out); err != nil {
		fmt.Fprintln(os.Stderr, "打包失败：", err)
		os.Exit(1)
	}
}

func run(dir, out string) error {
	src := os.DirFS(dir)
	b, err := fs.ReadFile(src, manifest.FileName)
	if err != nil {
		return fmt.Errorf("找不到 %s：%w", manifest.FileName, err)
	}
	m, err := manifest.Parse(b)
	if err != nil {
		return err
	}
	ev, err := expression.New()
	if err != nil {
		return err
	}
	if _, err := loader.Load(src, ev); err != nil {
		return fmt.Errorf("内容校验失败：%w", err)
	}
	if out == "" {
		out = filepath.Join("build", "packs", fmt.Sprintf("%s-%s.zip", m.ID, m.Version))
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		return err
	}
	f, err := os.Create(out) //nolint:gosec // 命令行参数指定的输出路径
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	n := 0
	err = fs.WalkDir(src, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(src, name)
		if err != nil {
			return err
		}
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		n++
		_, err = w.Write(data)
		return err
	})
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	fmt.Printf("已打包《%s》v%s（%d 个文件）→ %s\n", m.Name, m.Version, n, out)
	return nil
}
