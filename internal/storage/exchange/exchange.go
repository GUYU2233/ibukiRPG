// Package exchange 是存档导出 / 导入（架构 V0.3 第 10.6 节）：`.ibksave` 是一个 zip，
// 包含 manifest.json（格式、引擎版本、故事包版本、分支）与 save.json（事件、快照、对话记录、记忆、检查点）。
// 导出永不包含 API Key 或服务商配置；导入前可以先 Inspect 检查版本与故事包是否匹配。
package exchange

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/GUYU2233/ibukiRPG/internal/storage/eventstore"
)

// Format 是存档文件格式标识；Version 是格式版本。
const (
	Format  = "ibksave"
	Version = 2
)

// Manifest 是 .ibksave 的清单。
type Manifest struct {
	Format        string                  `json:"format"`
	Version       int                     `json:"version"`
	EngineVersion string                  `json:"engine_version"`
	Packages      []eventstore.PackageRef `json:"packages"`
	SaveName      string                  `json:"save_name"`
	PlayerName    string                  `json:"player_name"`
	Branches      []eventstore.Branch     `json:"branches"`
	Checkpoints   int                     `json:"checkpoints"`
	Events        int                     `json:"events"`
	ExportedAt    int64                   `json:"exported_at"`
	Summary       eventstore.Summary      `json:"summary"`
	Debug         bool                    `json:"debug,omitempty"`
}

// payload 是 save.json（Dump 的完整序列化；Dump 自身的 JSON 标签隐藏了大字段）。
type payload struct {
	Slot        eventstore.Slot             `json:"slot"`
	Branches    []eventstore.Branch         `json:"branches"`
	Events      []eventstore.BranchEvent    `json:"events"`
	Snapshots   []eventstore.BranchSnapshot `json:"snapshots"`
	Transcript  []eventstore.BranchEntry    `json:"transcript"`
	Memory      []eventstore.BranchMemory   `json:"memory"`
	Commands    []eventstore.BranchCommand  `json:"commands"`
	Checkpoints []eventstore.Checkpoint     `json:"checkpoints"`
	AICalls     []eventstore.AICall         `json:"ai_calls,omitempty"`
}

// Options 是导出选项。
type Options struct {
	Branch string // 只导出该分支（空 = 全部）
	Debug  bool   // 附带 AI 调用记账（调试包；仍不含正文与密钥）
}

// 错误。
var (
	ErrNotSave    = errors.New("这不是 ibukiRPG 存档文件（.ibksave）")
	ErrLegacy     = errors.New("这个存档来自 v0.1.x。0.2.0 改成了开放世界，存档结构完全不同，无法导入。如需继续旧存档，请安装 v0.1.3-rc1")
	ErrTooNew     = errors.New("这个存档来自更新版本的 ibukiRPG，请先升级 App")
	ErrSecret     = errors.New("存档文件里疑似包含 API Key，已拒绝导入（导出功能从不写入密钥，这个文件可能被改动过）")
	secretRe      = regexp.MustCompile(`(?i)("api_?key"\s*:\s*"[^"]+")|(\bsk-[A-Za-z0-9]{16,})|(Bearer\s+[A-Za-z0-9._-]{16,})`)
	errNoManifest = errors.New("存档缺少 manifest.json")
)

// Export 导出存档为 .ibksave 字节。
func Export(ctx context.Context, st *eventstore.Store, slotID string, opt Options) ([]byte, Manifest, error) {
	d, err := st.DumpSlot(ctx, slotID, opt.Branch, opt.Debug)
	if err != nil {
		return nil, Manifest{}, err
	}
	p := payload{Slot: d.Slot, Branches: d.Branches, Events: d.Events, Snapshots: d.Snapshots, Transcript: d.Transcript, Memory: d.Memory,
		Commands: d.Commands, Checkpoints: d.Checkpoints, AICalls: d.AICalls}
	if opt.Branch != "" {
		p.Slot.Branch = opt.Branch
		p.Slot.PendingFork = eventstore.ForkRef{}
	}
	m := Manifest{Format: Format, Version: Version, EngineVersion: d.Slot.EngineVersion, Packages: d.Slot.Packages, SaveName: d.Slot.Name,
		PlayerName: d.Slot.PlayerName, Branches: d.Branches, Checkpoints: len(d.Checkpoints), Events: len(d.Events), ExportedAt: time.Now().UnixMilli(),
		Summary: d.Slot.Summary, Debug: opt.Debug}
	data, err := json.Marshal(p)
	if err != nil {
		return nil, m, err
	}
	// 防御：导出内容绝不能出现密钥模式（例如玩家把 Key 打进了输入框）——打码而不是失败
	data = secretRe.ReplaceAll(data, []byte(`"[redacted]"`))
	mb, _ := json.MarshalIndent(m, "", "  ")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range []struct {
		name string
		b    []byte
	}{{"manifest.json", mb}, {"save.json", data}} {
		w, err := zw.Create(f.name)
		if err != nil {
			return nil, m, err
		}
		if _, err := w.Write(f.b); err != nil {
			return nil, m, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, m, err
	}
	return buf.Bytes(), m, nil
}

func readZip(b []byte) (Manifest, []byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return Manifest{}, nil, ErrNotSave
	}
	var m Manifest
	var data []byte
	found := false
	for _, f := range zr.File {
		if f.UncompressedSize64 > 512<<20 {
			return m, nil, errors.New("存档文件过大")
		}
		rc, err := f.Open()
		if err != nil {
			return m, nil, err
		}
		raw, err := io.ReadAll(io.LimitReader(rc, 512<<20))
		_ = rc.Close()
		if err != nil {
			return m, nil, err
		}
		switch f.Name {
		case "manifest.json":
			if err := json.Unmarshal(raw, &m); err != nil {
				return m, nil, ErrNotSave
			}
			found = true
		case "save.json":
			data = raw
		}
	}
	if !found {
		return m, nil, errNoManifest
	}
	return m, data, nil
}

// Check 是导入前检查的结果。
type Check struct {
	Manifest Manifest `json:"manifest"`
	OK       bool     `json:"ok"`
	Problems []string `json:"problems,omitempty"` // 阻止导入
	Warnings []string `json:"warnings,omitempty"` // 可以导入，但需要提醒
}

// PackLookup 返回已安装故事包的版本（ok=false 表示没有安装）。
type PackLookup func(id string) (version string, ok bool)

// Inspect 检查存档文件（版本、故事包匹配、密钥扫描），不写入数据库。
func Inspect(b []byte, engineVersion string, packs PackLookup) (Check, error) {
	m, data, err := readZip(b)
	if err != nil {
		return Check{}, err
	}
	c := Check{Manifest: m}
	if m.Format != Format {
		return c, ErrNotSave
	}
	if m.Version < Version || strings.HasPrefix(m.EngineVersion, "0.1.") {
		c.Problems = append(c.Problems, ErrLegacy.Error())
	}
	if m.Version > Version {
		c.Problems = append(c.Problems, ErrTooNew.Error())
	}
	if secretRe.Match(data) {
		c.Problems = append(c.Problems, ErrSecret.Error())
	}
	if newer(m.EngineVersion, engineVersion) {
		c.Warnings = append(c.Warnings, fmt.Sprintf("存档由更新的引擎 %s 导出（当前 %s），可能有不兼容的内容。", m.EngineVersion, engineVersion))
	}
	for _, p := range m.Packages {
		v, ok := packs(p.ID)
		switch {
		case !ok:
			c.Problems = append(c.Problems, fmt.Sprintf("缺少故事包 %s（%s）。请先导入该故事包。", p.ID, p.Version))
		case v != p.Version:
			c.Warnings = append(c.Warnings, fmt.Sprintf("故事包 %s 的版本不同（存档 %s，已安装 %s）。", p.ID, p.Version, v))
		}
	}
	c.OK = len(c.Problems) == 0
	return c, nil
}

// Import 导入存档为新存档（新的存档 ID；名字重复时加“（导入）”）。返回新存档 ID。
func Import(ctx context.Context, st *eventstore.Store, b []byte, engineVersion string, packs PackLookup, newID string) (string, Check, error) {
	c, err := Inspect(b, engineVersion, packs)
	if err != nil {
		return "", c, err
	}
	if !c.OK {
		return "", c, errors.New(strings.Join(c.Problems, "\n"))
	}
	_, data, _ := readZip(b)
	var p payload
	if err := json.Unmarshal(data, &p); err != nil {
		return "", c, fmt.Errorf("存档内容损坏：%w", err)
	}
	d := &eventstore.Dump{Slot: p.Slot, Branches: p.Branches, Events: p.Events, Snapshots: p.Snapshots, Transcript: p.Transcript, Memory: p.Memory,
		Commands: p.Commands, Checkpoints: p.Checkpoints}
	name := strings.TrimSpace(p.Slot.Name)
	if name == "" {
		name = "导入的存档"
	}
	name += "（导入）"
	if err := st.RestoreSlot(ctx, d, newID, name); err != nil {
		return "", c, err
	}
	return newID, c, nil
}

// newer 报告版本 a 是否比 b 新（只比较数字段；预发布后缀视为相同）。
func newer(a, b string) bool {
	pa, pb := nums(a), nums(b)
	for i := range 3 {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func nums(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	for i, p := range strings.SplitN(v, ".", 3) {
		_, _ = fmt.Sscanf(p, "%d", &out[i])
	}
	return out
}
