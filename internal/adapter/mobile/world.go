package mobile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/GUYU2233/ibukiRPG/internal/agent/orchestrator"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/router"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/storage/eventstore"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
)

// LegacyDBFileName 是 0.1.x 的存档数据库（0.2.0 不再读取，第 10.7 节）。
const LegacyDBFileName = "ibukirpg.db"

// worldTypes 是 0.2.0 新增的请求（开放世界 / 回溯 / 设置 / 存档交换）。
var worldTypes = map[string]bool{
	"get_tasks": true, "set_prompt_settings": true, "get_prompt_settings": true,
	"get_creation": true, "review_creation": true,
	"get_pending_decision": true, "resolve_decision": true,
	"search_world_changes": true, "revert_change": true,
	"get_timeline": true, "rollback_to": true, "cancel_rollback": true, "switch_branch": true, "rename_branch": true, "delete_branch": true,
	"create_checkpoint": true, "restore_checkpoint": true, "rename_checkpoint": true, "delete_checkpoint": true,
	"export_save": true, "inspect_save": true, "import_save": true,
	"get_usage": true, "wait": true, "get_world_panel": true, "get_entity": true,
	"preview_change": true, "apply_preview": true, "run_audit": true, "resolve_audit_suggestion": true,
}

// legacyInfo 报告数据目录里是否还有 0.1.x 的旧存档。
func legacyInfo(dataDir string) map[string]any {
	p := filepath.Join(dataDir, LegacyDBFileName)
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		return map[string]any{"legacy_notice": eventstore.LegacyMessage, "legacy_bytes": fi.Size()}
	}
	return nil
}

// deleteLegacy 删除 0.1.x 旧存档（玩家在提示里确认后调用）。
func deleteLegacy(dataDir string) error {
	for _, suf := range []string{"", "-wal", "-shm", "-journal"} {
		if err := os.Remove(filepath.Join(dataDir, LegacyDBFileName+suf)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// aiPayload 是 configure_ai 的新格式（多服务商 + 生成设置）；没有 providers 字段时按旧格式 provider.Config 处理。
type aiPayload struct {
	Providers []router.Provider `json:"providers"`
	Settings  router.Settings   `json:"settings"`
	Prompts   *change.Settings  `json:"prompts,omitempty"`
}

func configureAI(s *orchestrator.Session, raw json.RawMessage) (any, error) {
	var probe map[string]json.RawMessage
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &probe); err != nil {
			return nil, fmt.Errorf("invalid payload: %w", err)
		}
	}
	if _, ok := probe["providers"]; !ok {
		cfg, err := decode[provider.Config](raw)
		if err != nil {
			return nil, err
		}
		return s.ConfigureAI(cfg), nil
	}
	p, err := decode[aiPayload](raw)
	if err != nil {
		return nil, err
	}
	if p.Prompts != nil {
		s.SetPromptSettings(*p.Prompts)
	}
	return s.ConfigureProviders(p.Providers, p.Settings), nil
}

type idPayload struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Turn   int    `json:"turn"`
	Branch string `json:"branch"`
}

// worldRequest 处理 0.2.0 新增请求。
func worldRequest(ctx context.Context, s *orchestrator.Session, req dto.RequestV1) (any, error) {
	ok := map[string]bool{"ok": true}
	switch req.Type {
	case "get_tasks":
		return map[string]any{"tasks": router.Tasks(), "status": s.AIStatus()}, nil
	case "get_prompt_settings":
		return s.PromptSettings(), nil
	case "set_prompt_settings":
		p, err := decode[change.Settings](req.Payload)
		if err != nil {
			return nil, err
		}
		s.SetPromptSettings(p)
		return s.PromptSettings(), nil
	case "get_creation":
		p, err := decode[struct {
			PackID string `json:"pack_id"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		return s.CreationOptions(p.PackID)
	case "review_creation":
		p, err := decode[struct {
			PackID   string          `json:"pack_id"`
			Creation engine.Creation `json:"creation"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		return s.ReviewCreation(ctx, p.PackID, p.Creation)
	case "get_pending_decision":
		d, err := s.PendingDecision()
		return map[string]any{"decision": d}, err
	case "resolve_decision":
		p, err := decode[struct {
			ID         string `json:"id"`
			Action     string `json:"action"` // accept | rollback | dismiss
			NotifyOnly bool   `json:"notify_only"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		if _, err := s.ResolveDecision(ctx, p.ID, p.Action, p.NotifyOnly); err != nil {
			return nil, err
		}
		return bundle(ctx, s)
	case "search_world_changes":
		p, err := decode[struct {
			Query  string `json:"query"`
			Source string `json:"source"`
			Limit  int    `json:"limit"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		return s.WorldChanges(p.Query, p.Source, p.Limit)
	case "revert_change":
		p, err := decode[idPayload](req.Payload)
		if err != nil {
			return nil, err
		}
		return s.RevertChange(ctx, p.ID)
	case "preview_change":
		// 玩家明确要求的修改（卡片“让 AI 修改”、新建 NPC 等）：先预览，确认后 apply_preview
		p, err := decode[struct {
			Changes []change.Change `json:"changes"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		return s.PreviewChanges(ctx, change.SourceUserRequest, p.Changes)
	case "apply_preview":
		p, err := decode[struct {
			Token string `json:"preview_token"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		return s.ApplyPreview(ctx, p.Token, 0)
	case "run_audit":
		return s.RunAudit(ctx)
	case "resolve_audit_suggestion":
		p, err := decode[struct {
			ID     string `json:"id"`
			Action string `json:"action"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		return s.ResolveAuditSuggestion(ctx, p.ID, p.Action)
	case "get_timeline":
		return s.Timeline(ctx)
	case "rollback_to":
		p, err := decode[idPayload](req.Payload)
		if err != nil {
			return nil, err
		}
		if err := s.RollbackTo(ctx, p.Turn); err != nil {
			return nil, err
		}
		return bundle(ctx, s)
	case "cancel_rollback":
		if err := s.CancelRollback(ctx); err != nil {
			return nil, err
		}
		return bundle(ctx, s)
	case "switch_branch":
		p, err := decode[idPayload](req.Payload)
		if err != nil {
			return nil, err
		}
		if err := s.SwitchBranch(ctx, p.Branch); err != nil {
			return nil, err
		}
		return bundle(ctx, s)
	case "rename_branch":
		p, err := decode[idPayload](req.Payload)
		if err != nil {
			return nil, err
		}
		return ok, s.RenameBranch(ctx, p.Branch, p.Name)
	case "delete_branch":
		p, err := decode[idPayload](req.Payload)
		if err != nil {
			return nil, err
		}
		return ok, s.DeleteBranch(ctx, p.Branch)
	case "create_checkpoint":
		p, err := decode[idPayload](req.Payload)
		if err != nil {
			return nil, err
		}
		return s.CreateCheckpoint(ctx, p.Name, p.Turn)
	case "restore_checkpoint":
		p, err := decode[idPayload](req.Payload)
		if err != nil {
			return nil, err
		}
		if err := s.RestoreCheckpoint(ctx, p.ID); err != nil {
			return nil, err
		}
		return bundle(ctx, s)
	case "rename_checkpoint":
		p, err := decode[idPayload](req.Payload)
		if err != nil {
			return nil, err
		}
		return ok, s.RenameCheckpoint(ctx, p.ID, p.Name)
	case "delete_checkpoint":
		p, err := decode[idPayload](req.Payload)
		if err != nil {
			return nil, err
		}
		return ok, s.DeleteCheckpoint(ctx, p.ID)
	case "export_save":
		// Android 先导出到缓存目录，再通过 SAF 写到玩家选择的位置。
		p, err := decode[struct {
			SlotID string `json:"slot_id"`
			Dir    string `json:"dir"`
			Branch string `json:"branch"`
			Debug  bool   `json:"debug"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		if p.Dir == "" {
			return nil, errors.New("export_save: dir is required")
		}
		b, name, err := s.ExportSave(ctx, p.SlotID, p.Branch, p.Debug)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(p.Dir, 0o750); err != nil {
			return nil, err
		}
		path := filepath.Join(p.Dir, name)
		if err := os.WriteFile(path, b, 0o600); err != nil {
			return nil, err
		}
		return map[string]any{"path": path, "name": name, "bytes": len(b)}, nil
	case "inspect_save", "import_save":
		p, err := decode[struct {
			Path string `json:"path"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		if p.Path == "" {
			return nil, errors.New("请选择要导入的 .ibksave 文件")
		}
		b, err := os.ReadFile(p.Path)
		if err != nil {
			return nil, err
		}
		if req.Type == "inspect_save" {
			return s.InspectSave(b)
		}
		id, chk, err := s.ImportSave(ctx, b)
		if err != nil {
			return nil, err
		}
		return map[string]any{"slot_id": id, "check": chk}, nil
	case "get_usage":
		p, err := decode[idPayload](req.Payload)
		if err != nil {
			return nil, err
		}
		return s.Usage(ctx, p.Turn)
	case "wait":
		p, err := decode[struct {
			Target string `json:"target"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		return s.Quick(ctx, req.CommandID, dto.QuickActionV1{Kind: "wait", Target: p.Target})
	case "get_world_panel":
		p, err := decode[struct {
			Tab string `json:"tab"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		return s.WorldPanel(p.Tab)
	case "get_entity":
		p, err := decode[idPayload](req.Payload)
		if err != nil {
			return nil, err
		}
		v, found, err := s.Entity(p.ID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, errors.New("你还不知道这个条目")
		}
		return v, nil
	}
	return nil, fmt.Errorf("unknown request type %q", req.Type)
}
