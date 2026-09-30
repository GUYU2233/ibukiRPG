// Package registry 是故事包注册表：发现内置故事包（随引擎内嵌）与玩家导入的故事包（.zip），
// 负责导入校验、删除与按存档加载（第 31-33、44-45 节）。
package registry

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/GUYU2233/ibukiRPG/internal/buildinfo"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/package/manifest"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
)

// Entry 是注册表中的一个故事包。
type Entry struct {
	Manifest *manifest.Manifest
	Builtin  bool
	// Dir 是导入包的解压目录（内置包为空）。
	Dir string
	// Err 非空表示这个导入包已损坏或与当前引擎不兼容（仍会列出，便于玩家删除）。
	Err  string
	fsys fs.FS
}

// ID 返回故事包 id（损坏的包返回目录名）。
func (e Entry) ID() string { return e.Manifest.ID }

// Playable 报告能否用于新游戏 / 读档。
func (e Entry) Playable() bool { return e.Err == "" }

// FS 返回故事包文件系统。
func (e Entry) FS() fs.FS { return e.fsys }

// Registry 管理可用的故事包。并发安全。
type Registry struct {
	mu      sync.Mutex
	builtin []Entry
	userDir string
	ev      *expression.Evaluator
	cache   map[string]*loader.Package
}

// New 创建注册表。builtins 是各内置故事包的根目录（manifest.yaml 所在目录）；
// userDir 是导入包的存放目录（为空表示不支持导入）。
func New(builtins []fs.FS, userDir string, ev *expression.Evaluator) (*Registry, error) {
	r := &Registry{userDir: userDir, ev: ev, cache: map[string]*loader.Package{}}
	for _, f := range builtins {
		b, err := fs.ReadFile(f, manifest.FileName)
		if err != nil {
			return nil, fmt.Errorf("builtin pack: %w", err)
		}
		m, err := manifest.Parse(b)
		if err != nil {
			return nil, fmt.Errorf("builtin pack: %w", err)
		}
		if slices.ContainsFunc(r.builtin, func(e Entry) bool { return e.Manifest.ID == m.ID }) {
			return nil, fmt.Errorf("builtin pack %q declared twice", m.ID)
		}
		r.builtin = append(r.builtin, Entry{Manifest: m, Builtin: true, fsys: f})
	}
	if len(r.builtin) == 0 {
		return nil, errors.New("no builtin story pack")
	}
	if userDir != "" {
		if err := os.MkdirAll(userDir, 0o750); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// DefaultID 返回默认故事包（第一个内置包）。
func (r *Registry) DefaultID() string { return r.builtin[0].Manifest.ID }

// List 返回全部故事包：内置在前（声明顺序），导入的按名称排序。损坏的导入包也会列出（Err 非空）。
func (r *Registry) List() []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append(slices.Clone(r.builtin), r.users()...)
}

func (r *Registry) users() []Entry {
	if r.userDir == "" {
		return nil
	}
	dirs, err := os.ReadDir(r.userDir)
	if err != nil {
		return nil
	}
	var out []Entry
	for _, d := range dirs {
		if !d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			continue
		}
		dir := filepath.Join(r.userDir, d.Name())
		e := Entry{Dir: dir, fsys: os.DirFS(dir)}
		b, err := os.ReadFile(filepath.Join(dir, manifest.FileName)) //nolint:gosec // 应用私有目录
		if err == nil {
			e.Manifest, err = manifest.Parse(b)
		}
		if err != nil {
			e.Manifest = &manifest.Manifest{ID: d.Name(), Name: d.Name()}
			e.Err = "故事包已损坏：" + err.Error()
		} else if msg := compatError(e.Manifest); msg != "" {
			e.Err = msg
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Manifest.Name < out[j].Manifest.Name })
	return out
}

// compatError 检查引擎版本要求与包类型。
func compatError(m *manifest.Manifest) string {
	if ok, err := manifest.Satisfies(buildinfo.Version, m.Engine); err != nil || !ok {
		return fmt.Sprintf("需要引擎版本 %s，当前是 %s。请先更新 App。", m.Engine, buildinfo.Version)
	}
	if m.Type != manifest.TypeStory && m.Type != manifest.TypeWorld {
		return fmt.Sprintf("这是 %s 类型的包，目前只能游玩故事包（type: story 或 world）。", m.Type)
	}
	return ""
}

// Get 查找故事包（包括损坏的导入包）。
func (r *Registry) Get(id string) (Entry, bool) {
	for _, e := range r.List() {
		if e.Manifest.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// Load 加载并校验故事包内容（结果按 id@version 缓存）。
func (r *Registry) Load(id string) (*loader.Package, error) {
	e, ok := r.Get(id)
	if !ok {
		return nil, fmt.Errorf("没有安装故事包 %q", id)
	}
	if e.Err != "" {
		return nil, errors.New(e.Err)
	}
	key := e.Manifest.ID + "@" + e.Manifest.Version + "@" + e.Dir
	r.mu.Lock()
	p, ok := r.cache[key]
	r.mu.Unlock()
	if ok {
		return p, nil
	}
	p, err := loader.Load(e.fsys, r.ev)
	if err != nil {
		return nil, fmt.Errorf("故事包「%s」内容有误：%w", e.Manifest.Name, err)
	}
	r.mu.Lock()
	r.cache[key] = p
	r.mu.Unlock()
	return p, nil
}

// Delete 删除一个导入的故事包。内置包不能删除。使用它的存档会保留，重新导入后即可继续。
func (r *Registry) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if slices.ContainsFunc(r.builtin, func(e Entry) bool { return e.Manifest.ID == id }) {
		return errors.New("内置故事包不能删除")
	}
	for _, e := range r.users() {
		if e.Manifest.ID == id {
			if err := os.RemoveAll(e.Dir); err != nil {
				return fmt.Errorf("删除失败：%w", err)
			}
			r.invalidate(id)
			return nil
		}
	}
	return errors.New("没有找到这个故事包")
}

func (r *Registry) invalidate(id string) {
	for k := range r.cache {
		if strings.HasPrefix(k, id+"@") {
			delete(r.cache, k)
		}
	}
}

// MaxCoverBytes 限制封面图片大小。
const MaxCoverBytes = 1 << 20

// Cover 读取封面图片（没有封面返回 nil）。
func (e Entry) Cover() []byte {
	if e.Manifest.Cover == "" || e.fsys == nil {
		return nil
	}
	b, err := fs.ReadFile(e.fsys, e.Manifest.Cover)
	if err != nil || len(b) > MaxCoverBytes {
		return nil
	}
	return b
}
