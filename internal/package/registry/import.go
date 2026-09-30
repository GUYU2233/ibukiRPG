package registry

import (
	"archive/zip"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/package/manifest"
)

// 导入限制（防止压缩炸弹与异常文件）。
const (
	MaxZipFiles   = 2000
	MaxTotalBytes = 64 << 20
	MaxFileBytes  = 16 << 20
)

// ImportResult 是导入结果。
type ImportResult struct {
	Entry    Entry
	Replaced bool   // 覆盖了同 id 的旧版本
	Previous string // 旧版本号
}

// Import 从 .zip 导入故事包：解压到临时目录 → 校验 manifest、命名空间、依赖、引擎版本 →
// 完整加载校验内容（引用完整性、CEL 表达式）→ 原子替换到用户目录。任何一步失败都不会留下残留。
func (r *Registry) Import(zipPath string) (ImportResult, error) {
	if r.userDir == "" {
		return ImportResult{}, errors.New("当前环境不支持导入故事包")
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return ImportResult{}, errors.New("无法打开文件：请选择 .zip 格式的故事包")
	}
	defer func() { _ = zr.Close() }()
	tmp := filepath.Join(r.userDir, ".import-"+randHex())
	if err := os.MkdirAll(tmp, 0o750); err != nil {
		return ImportResult{}, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := extract(&zr.Reader, tmp); err != nil {
		return ImportResult{}, err
	}
	b, err := os.ReadFile(filepath.Join(tmp, manifest.FileName)) //nolint:gosec // 刚解压的私有目录
	if err != nil {
		return ImportResult{}, errors.New("读取 manifest.yaml 失败")
	}
	m, err := manifest.Parse(b)
	if err != nil {
		return ImportResult{}, fmt.Errorf("manifest.yaml 校验失败：%w", err)
	}
	if strings.TrimSpace(m.Name) == "" {
		return ImportResult{}, errors.New("manifest.yaml 缺少 name（故事包名称）")
	}
	if msg := compatError(m); msg != "" {
		return ImportResult{}, errors.New(msg)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	users := r.users()
	all := append(slices.Clone(r.builtin), users...)
	var res ImportResult
	for _, e := range all {
		switch {
		case e.Builtin && e.Manifest.ID == m.ID:
			return ImportResult{}, fmt.Errorf("故事包 id %q 与内置故事包「%s」相同，请修改 manifest.yaml 中的 id", m.ID, e.Manifest.Name)
		case e.Manifest.ID == m.ID:
			res.Replaced, res.Previous = true, e.Manifest.Version
		case e.Err == "" && e.Manifest.Namespace == m.Namespace:
			return ImportResult{}, fmt.Errorf("命名空间 %q 已被故事包「%s」使用，请换一个命名空间", m.Namespace, e.Manifest.Name)
		}
	}
	for _, d := range m.Dependencies {
		if d.Optional {
			continue
		}
		idx := slices.IndexFunc(all, func(e Entry) bool { return e.Manifest.ID == d.ID && e.Err == "" })
		if idx < 0 {
			return ImportResult{}, fmt.Errorf("缺少依赖的故事包 %q，请先导入它", d.ID)
		}
		if ok, _ := manifest.Satisfies(all[idx].Manifest.Version, d.Version); !ok {
			return ImportResult{}, fmt.Errorf("依赖 %q 需要版本 %s，已安装的是 %s", d.ID, d.Version, all[idx].Manifest.Version)
		}
	}
	if _, err := loader.Load(os.DirFS(tmp), r.ev); err != nil {
		return ImportResult{}, fmt.Errorf("故事包内容校验失败：%w", err)
	}
	dst := filepath.Join(r.userDir, m.ID)
	if res.Replaced {
		for _, e := range users {
			if e.Manifest.ID == m.ID {
				dst = e.Dir
			}
		}
		old := dst + ".old-" + randHex()
		if err := os.Rename(dst, old); err != nil { //nolint:gosec // m.ID 已按 id 格式校验（不含路径分隔符），dst 位于用户目录内
			return ImportResult{}, fmt.Errorf("替换旧版本失败：%w", err)
		}
		defer func() { _ = os.RemoveAll(old) }() //nolint:gosec // 同上
	}
	if err := os.Rename(tmp, dst); err != nil { //nolint:gosec // 同上
		return ImportResult{}, fmt.Errorf("安装失败：%w", err)
	}
	r.invalidate(m.ID)
	res.Entry = Entry{Manifest: m, Dir: dst, fsys: os.DirFS(dst)}
	return res, nil
}

func randHex() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// extract 安全解压：拒绝绝对路径、..、符号链接与超限文件；manifest.yaml 可以在根目录或唯一的顶层文件夹里。
func extract(zr *zip.Reader, dst string) error {
	if len(zr.File) > MaxZipFiles {
		return fmt.Errorf("压缩包里的文件太多（超过 %d 个）", MaxZipFiles)
	}
	prefix, err := rootPrefix(zr)
	if err != nil {
		return err
	}
	var total int64
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if f.FileInfo().IsDir() || strings.HasPrefix(path.Base(name), ".") || strings.HasPrefix(name, "__MACOSX/") {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("压缩包里不能包含符号链接：%s", f.Name)
		}
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		rel := strings.TrimPrefix(name, prefix)
		clean := path.Clean(rel)
		if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." || path.IsAbs(clean) || strings.Contains(clean, ":") {
			return fmt.Errorf("压缩包里有不安全的路径：%s", f.Name)
		}
		if f.UncompressedSize64 > MaxFileBytes {
			return fmt.Errorf("文件 %s 太大（单个文件不能超过 %d MB）", rel, MaxFileBytes>>20)
		}
		total += int64(f.UncompressedSize64)
		if total > MaxTotalBytes {
			return fmt.Errorf("故事包解压后太大（不能超过 %d MB）", MaxTotalBytes>>20)
		}
		if err := writeFile(f, filepath.Join(dst, filepath.FromSlash(clean))); err != nil {
			return err
		}
	}
	return nil
}

func rootPrefix(zr *zip.Reader) (string, error) {
	tops := map[string]bool{}
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if strings.HasPrefix(name, "__MACOSX/") || strings.HasPrefix(path.Base(name), ".") {
			continue
		}
		if name == manifest.FileName {
			return "", nil
		}
		top, _, _ := strings.Cut(name, "/")
		tops[top] = true
	}
	if len(tops) == 1 {
		for t := range tops {
			for _, f := range zr.File {
				if strings.ReplaceAll(f.Name, "\\", "/") == t+"/"+manifest.FileName {
					return t + "/", nil
				}
			}
		}
	}
	return "", errors.New("压缩包里找不到 manifest.yaml：它应该在压缩包根目录，或者唯一的顶层文件夹里")
}

func writeFile(f *zip.File, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("读取 %s 失败：%w", f.Name, err)
	}
	defer func() { _ = rc.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // 路径已清理
	if err != nil {
		return err
	}
	// 以声明大小 +1 为上限读取，防止大小字段作假。
	n, err := io.Copy(out, io.LimitReader(rc, int64(f.UncompressedSize64)+1)) //nolint:gosec // 已限制
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("解压 %s 失败：%w", f.Name, err)
	}
	if n > int64(f.UncompressedSize64) { //nolint:gosec // 已限制
		return fmt.Errorf("文件 %s 的大小与记录不符", f.Name)
	}
	return nil
}
