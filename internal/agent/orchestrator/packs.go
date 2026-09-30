package orchestrator

import (
	"context"
	"encoding/base64"

	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/package/registry"
)

func (s *Session) packView(e registry.Entry, saves map[string]int) dto.PackV1 {
	m := e.Manifest
	v := dto.PackV1{
		ID: m.ID, Name: m.Name, Version: m.Version, Type: string(m.Type), Author: m.AuthorText(), Tagline: m.Tagline,
		Description: m.Description, Tags: append([]string{}, m.Tags...), Icon: m.Icon, Accent: m.Accent, Engine: m.Engine,
		Builtin: e.Builtin, Playable: e.Playable(), Error: e.Err, SaveCount: saves[m.ID], IsDefault: m.ID == s.reg.DefaultID(),
	}
	if b := e.Cover(); len(b) > 0 {
		v.Cover = base64.StdEncoding.EncodeToString(b)
	}
	return v
}

func (s *Session) saveCounts(ctx context.Context) map[string]int {
	out := map[string]int{}
	slots, err := s.store.ListSlots(ctx)
	if err != nil {
		return out
	}
	for _, sl := range slots {
		out[s.slotPack(sl).ID]++
	}
	return out
}

// Packs 列出全部故事包（故事包选择界面）。
func (s *Session) Packs(ctx context.Context) []dto.PackV1 {
	counts := s.saveCounts(ctx)
	out := []dto.PackV1{}
	for _, e := range s.reg.List() {
		out = append(out, s.packView(e, counts))
	}
	return out
}

// ImportResultV1 是导入故事包的结果。
type ImportResultV1 struct {
	Pack     dto.PackV1 `json:"pack"`
	Replaced bool       `json:"replaced"`
	Previous string     `json:"previous_version,omitempty"`
}

// ImportPack 从 .zip 导入故事包（校验失败返回中文原因）。
func (s *Session) ImportPack(ctx context.Context, zipPath string) (ImportResultV1, error) {
	r, err := s.reg.Import(zipPath)
	if err != nil {
		return ImportResultV1{}, err
	}
	return ImportResultV1{Pack: s.packView(r.Entry, s.saveCounts(ctx)), Replaced: r.Replaced, Previous: r.Previous}, nil
}

// DeletePack 删除导入的故事包。使用它的存档保留（显示“缺少故事包”），重新导入后可继续。
// 如果当前正在玩这个故事包，会先退出当前游戏。
func (s *Session) DeletePack(_ context.Context, id string) error {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	if err := s.reg.Delete(id); err != nil {
		return err
	}
	s.mu.Lock()
	if s.g != nil && s.g.pkg.Manifest.ID == id {
		s.slot, s.st, s.g = "", nil, nil
	}
	s.mu.Unlock()
	return nil
}
