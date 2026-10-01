package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/buildinfo"
	"github.com/GUYU2233/ibukiRPG/internal/storage/eventstore"
	"github.com/GUYU2233/ibukiRPG/internal/storage/exchange"
)

// ExportSave 导出存档（.ibksave 字节与建议文件名）。branch 为空导出全部分支；debug 附带 AI 调用记账。
func (s *Session) ExportSave(ctx context.Context, slotID, branch string, debug bool) ([]byte, string, error) {
	if slotID == "" {
		slotID, _, _, _ = s.current()
	}
	if slotID == "" {
		return nil, "", ErrNoGame
	}
	b, m, err := exchange.Export(ctx, s.store, slotID, exchange.Options{Branch: branch, Debug: debug})
	if err != nil {
		return nil, "", err
	}
	name := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_").Replace(m.SaveName)
	if name == "" {
		name = "save"
	}
	return b, fmt.Sprintf("%s-t%d.ibksave", name, m.Summary.Turn), nil
}

func (s *Session) packLookup(id string) (string, bool) {
	p, err := s.reg.Load(id)
	if err != nil {
		return "", false
	}
	return p.Manifest.Version, true
}

// InspectSave 检查存档文件（导入前警告用）。
func (s *Session) InspectSave(b []byte) (exchange.Check, error) {
	return exchange.Inspect(b, buildinfo.Version, s.packLookup)
}

// ImportSave 导入存档为新存档，返回新存档 ID。
func (s *Session) ImportSave(ctx context.Context, b []byte) (string, exchange.Check, error) {
	return exchange.Import(ctx, s.store, b, buildinfo.Version, s.packLookup, eventstore.NewSlotID())
}
