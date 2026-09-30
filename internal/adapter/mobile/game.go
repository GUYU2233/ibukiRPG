package mobile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/GUYU2233/ibukiRPG/internal/agent/orchestrator"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/packages"
)

// DBFileName 是数据目录下的存档数据库文件名。
const DBFileName = "ibukirpg.db"

// PackDirName 是数据目录下存放导入故事包的子目录。
const PackDirName = "packs"

var (
	sessMu   sync.Mutex
	sess     *orchestrator.Session
	sessPath string
	sinkMu   sync.RWMutex
	sink     func(string)
)

// SetSink 设置流式事件回调（JSON 字符串）。移动端通过 gomobile 接口注册。
func SetSink(f func(string)) {
	sinkMu.Lock()
	sink = f
	sinkMu.Unlock()
}

func emit(e dto.StreamEventV1) {
	sinkMu.RLock()
	f := sink
	sinkMu.RUnlock()
	if f == nil {
		return
	}
	b, err := json.Marshal(e)
	if err == nil {
		f(string(b))
	}
}

// Session 返回当前 Session（CLI 等 Go 端调用者复用）。
func Session() *orchestrator.Session {
	sessMu.Lock()
	defer sessMu.Unlock()
	return sess
}

// Close 关闭当前 Session（释放数据库文件）。之后需要重新 init。
func Close() error {
	sessMu.Lock()
	defer sessMu.Unlock()
	if sess == nil {
		return nil
	}
	err := sess.Close()
	sess, sessPath = nil, ""
	return err
}

func current() (*orchestrator.Session, error) {
	sessMu.Lock()
	defer sessMu.Unlock()
	if sess == nil {
		return nil, errors.New("引擎尚未初始化：请先调用 init")
	}
	return sess, nil
}

func initSession(ctx context.Context, dataDir string) (any, error) {
	if dataDir == "" {
		return nil, errors.New("init: data_dir is required")
	}
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, DBFileName)
	sessMu.Lock()
	defer sessMu.Unlock()
	if sess != nil && sessPath == path {
		return map[string]any{"db": path, "reused": true}, nil
	}
	if sess != nil {
		_ = sess.Close()
		sess = nil
	}
	s, err := orchestrator.Open(ctx, orchestrator.Options{DBPath: path, Packs: packages.Builtin(), PackDir: filepath.Join(dataDir, PackDirName), Sink: emit})
	if err != nil {
		return nil, err
	}
	sess, sessPath = s, path
	return map[string]any{"db": path, "reused": false, "package": s.Package().Manifest.Name, "package_version": s.Package().Manifest.Version}, nil
}

// gameBundle 是进入游戏界面所需的全部数据（新建 / 读档 / 恢复后调用）。
type gameBundle struct {
	Scene       dto.SceneV1        `json:"scene"`
	Transcript  []dto.EntryV1      `json:"transcript"`
	Suggestions []dto.SuggestionV1 `json:"suggestions"`
}

func bundle(ctx context.Context, s *orchestrator.Session) (any, error) {
	scene, err := s.Scene(ctx)
	if err != nil {
		return nil, err
	}
	tr, err := s.Transcript(ctx, 300, 0)
	if err != nil {
		return nil, err
	}
	sug, err := s.Suggestions()
	if err != nil {
		return nil, err
	}
	return gameBundle{Scene: scene, Transcript: tr, Suggestions: sug}, nil
}

func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	if len(raw) == 0 {
		return v, nil
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, fmt.Errorf("invalid payload: %w", err)
	}
	return v, nil
}

// gameDispatch 处理游戏相关请求；返回 handled=false 表示不是游戏请求。
func gameDispatch(ctx context.Context, req dto.RequestV1) (any, bool, error) {
	if req.Type == "init" {
		p, err := decode[struct {
			DataDir string `json:"data_dir"`
		}](req.Payload)
		if err != nil {
			return nil, true, err
		}
		v, err := initSession(ctx, p.DataDir)
		return v, true, err
	}
	if req.Type == "presets" {
		return provider.Presets(), true, nil
	}
	handled := map[string]bool{
		"configure_ai": true, "ai_status": true, "test_ai": true, "list_saves": true, "new_game": true, "load_game": true,
		"delete_save": true, "copy_save": true, "rename_save": true, "submit_text": true, "quick_action": true,
		"get_scene": true, "get_suggestions": true, "get_character": true, "get_inventory": true, "get_npcs": true,
		"get_journal": true, "get_transcript": true, "get_bundle": true, "get_hud": true,
		"list_packs": true, "import_pack": true, "delete_pack": true,
		"get_codex": true, "get_card": true, "get_relations": true, "get_cards": true, "get_growth": true, "get_combat": true, "get_portrait": true, "get_mechs": true, "get_mech": true,
	}
	if !handled[req.Type] {
		return nil, false, nil
	}
	s, err := current()
	if err != nil {
		return nil, true, err
	}
	v, err := gameRequest(ctx, s, req)
	return v, true, err
}

type slotPayload struct {
	SlotID string `json:"slot_id"`
	Name   string `json:"name"`
}

func gameRequest(ctx context.Context, s *orchestrator.Session, req dto.RequestV1) (any, error) {
	switch req.Type {
	case "configure_ai":
		cfg, err := decode[provider.Config](req.Payload)
		if err != nil {
			return nil, err
		}
		return s.ConfigureAI(cfg), nil
	case "ai_status":
		return s.AIStatus(), nil
	case "test_ai":
		cfg, err := decode[provider.Config](req.Payload)
		if err != nil {
			return nil, err
		}
		reply, lat, err := s.TestAI(ctx, cfg)
		out := map[string]any{"ok": err == nil, "reply": reply, "latency_ms": lat.Milliseconds()}
		if err != nil {
			out["error"] = err.Error()
		}
		return out, nil
	case "list_saves":
		return s.ListSaves(ctx)
	case "new_game":
		p, err := decode[struct {
			PackID     string `json:"pack_id"`
			SaveName   string `json:"save_name"`
			PlayerName string `json:"player_name"`
			Seed       uint64 `json:"seed"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		if _, err := s.NewGameIn(ctx, p.PackID, p.SaveName, p.PlayerName, p.Seed); err != nil {
			return nil, err
		}
		return bundle(ctx, s)
	case "load_game":
		p, err := decode[slotPayload](req.Payload)
		if err != nil {
			return nil, err
		}
		if err := s.LoadGame(ctx, p.SlotID); err != nil {
			return nil, err
		}
		return bundle(ctx, s)
	case "get_bundle":
		return bundle(ctx, s)
	case "delete_save":
		p, err := decode[slotPayload](req.Payload)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"deleted": true}, s.DeleteSave(ctx, p.SlotID)
	case "copy_save":
		p, err := decode[slotPayload](req.Payload)
		if err != nil {
			return nil, err
		}
		id, err := s.CopySave(ctx, p.SlotID, p.Name)
		return map[string]string{"slot_id": id}, err
	case "rename_save":
		p, err := decode[slotPayload](req.Payload)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"renamed": true}, s.RenameSave(ctx, p.SlotID, p.Name)
	case "submit_text":
		p, err := decode[struct {
			Text string `json:"text"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		return s.Submit(ctx, req.CommandID, p.Text)
	case "quick_action":
		qa, err := decode[dto.QuickActionV1](req.Payload)
		if err != nil {
			return nil, err
		}
		return s.Quick(ctx, req.CommandID, qa)
	case "get_scene":
		return s.Scene(ctx)
	case "get_hud":
		sc, err := s.Scene(ctx)
		return sc.Hud, err
	case "list_packs":
		return s.Packs(ctx), nil
	case "import_pack":
		p, err := decode[struct {
			Path string `json:"path"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		if p.Path == "" {
			return nil, errors.New("请选择要导入的 .zip 文件")
		}
		return s.ImportPack(ctx, p.Path)
	case "delete_pack":
		p, err := decode[struct {
			ID string `json:"id"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"deleted": true}, s.DeletePack(ctx, p.ID)
	case "get_suggestions":
		return s.Suggestions()
	case "get_character":
		return s.Character()
	case "get_inventory":
		return s.Inventory()
	case "get_npcs":
		return s.NPCs()
	case "get_journal":
		return s.Journal(ctx)
	case "get_codex":
		return s.Codex()
	case "get_card":
		p, err := decode[struct {
			ID string `json:"id"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		return s.Card(p.ID)
	case "get_relations":
		return s.Relations()
	case "get_cards":
		return s.Cards()
	case "get_growth":
		return s.Growth()
	case "get_combat":
		return s.Combat()
	case "get_mechs":
		return s.Mechs()
	case "get_mech":
		p, err := decode[struct {
			ID string `json:"id"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		return s.Mech(p.ID)
	case "get_portrait":
		p, err := decode[struct {
			ID string `json:"id"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		v, _, err := s.Portrait(ctx, p.ID)
		return v, err
	case "get_transcript":
		p, err := decode[struct {
			Limit    int   `json:"limit"`
			BeforeID int64 `json:"before_id"`
		}](req.Payload)
		if err != nil {
			return nil, err
		}
		return s.Transcript(ctx, p.Limit, p.BeforeID)
	}
	return nil, fmt.Errorf("unknown request type %q", req.Type)
}
