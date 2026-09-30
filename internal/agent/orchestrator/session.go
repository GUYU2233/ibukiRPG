package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/GUYU2233/ibukiRPG/internal/action/resolver"
	"github.com/GUYU2233/ibukiRPG/internal/agent/narrator"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/api/query"
	"github.com/GUYU2233/ibukiRPG/internal/buildinfo"
	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/package/manifest"
	"github.com/GUYU2233/ibukiRPG/internal/package/registry"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
	"github.com/GUYU2233/ibukiRPG/internal/storage/eventstore"
	"github.com/GUYU2233/ibukiRPG/internal/world/worldtime"
)

// ErrNoGame 表示还没有载入存档。
var ErrNoGame = errors.New("还没有载入游戏：请先新建或读取存档")

// ErrBusy 表示上一回合尚未结束。
var ErrBusy = errors.New("上一个行动还在处理中，请稍候")

// Options 是 Session 配置。
type Options struct {
	DBPath string
	// Package 是单一内置故事包（测试与旧调用方式）；Packs 非空时忽略。
	Package fs.FS
	// Packs 是内置故事包列表（第一个为默认）；PackDir 是导入故事包的存放目录（为空表示不支持导入）。
	Packs     []fs.FS
	PackDir   string
	Transport http.RoundTripper // 测试注入（录制传输 / 故障模拟）
	Sink      func(dto.StreamEventV1)
	// 超时（0 表示默认值）。
	ResolverTimeout time.Duration
	NarratorTimeout time.Duration
}

// Session 是一局游戏的编排器（Critical Path，第 38 节）：
// 输入 → Resolver → Validator/Core → Event → Narrator → Guard → 输出。
type Session struct {
	opts  Options
	store *eventstore.Store
	reg   *registry.Registry
	ev    *expression.Evaluator

	turnMu sync.Mutex
	mu     sync.RWMutex
	slot   string
	st     *state.State
	g      *game // 当前存档绑定的故事包运行时
	ai     provider.Config
	aiErr  string
	sink   func(dto.StreamEventV1)
}

// Open 打开存档数据库并加载内容包。
func Open(ctx context.Context, opts Options) (*Session, error) {
	if opts.ResolverTimeout == 0 {
		opts.ResolverTimeout = 15 * time.Second
	}
	if opts.NarratorTimeout == 0 {
		opts.NarratorTimeout = 45 * time.Second
	}
	ev, err := expression.New()
	if err != nil {
		return nil, err
	}
	builtins := opts.Packs
	if len(builtins) == 0 && opts.Package != nil {
		builtins = []fs.FS{opts.Package}
	}
	reg, err := registry.New(builtins, opts.PackDir, ev)
	if err != nil {
		return nil, err
	}
	// 内置默认故事包必须能加载（尽早暴露内容错误）。
	if _, err := reg.Load(reg.DefaultID()); err != nil {
		return nil, fmt.Errorf("load package: %w", err)
	}
	st, err := eventstore.Open(ctx, opts.DBPath)
	if err != nil {
		return nil, err
	}
	return &Session{opts: opts, store: st, reg: reg, ev: ev, ai: provider.Config{Kind: provider.KindOffline}, sink: opts.Sink}, nil
}

// game 是一个故事包的运行时：内容、引擎与查询层。
type game struct {
	pkg *loader.Package
	eng *engine.Engine
	q   *query.Q
}

func (s *Session) gameFor(id string) (*game, error) {
	p, err := s.reg.Load(id)
	if err != nil {
		return nil, err
	}
	return &game{pkg: p, eng: engine.New(p, s.ev), q: &query.Q{Pkg: p, Eval: s.ev}}, nil
}

// Registry 返回故事包注册表。
func (s *Session) Registry() *registry.Registry { return s.reg }

// Close 关闭数据库。
func (s *Session) Close() error { return s.store.Close() }

// SetSink 设置流式事件接收者。
func (s *Session) SetSink(f func(dto.StreamEventV1)) {
	s.mu.Lock()
	s.sink = f
	s.mu.Unlock()
}

func (s *Session) emit(e dto.StreamEventV1) {
	s.mu.RLock()
	f := s.sink
	s.mu.RUnlock()
	if f != nil {
		e.Version = dto.V1
		f(e)
	}
}

// Package 返回当前存档的故事包；没有载入存档时返回默认故事包。
func (s *Session) Package() *loader.Package {
	s.mu.RLock()
	g := s.g
	s.mu.RUnlock()
	if g != nil {
		return g.pkg
	}
	p, _ := s.reg.Load(s.reg.DefaultID())
	return p
}

// ---------- AI 配置 ----------

// ConfigureAI 设置运行时 AI Provider（密钥只保存在内存）。
func (s *Session) ConfigureAI(cfg provider.Config) dto.AIStatusV1 {
	s.mu.Lock()
	s.ai = cfg.Normalize()
	s.aiErr = ""
	s.mu.Unlock()
	return s.AIStatus()
}

// AIStatus 返回当前 AI 状态（不含密钥）。
func (s *Session) AIStatus() dto.AIStatusV1 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return dto.AIStatusV1{Kind: s.ai.Kind, BaseURL: s.ai.BaseURL, Model: s.ai.Model, HasKey: s.ai.APIKey != "", Online: s.ai.Online(), LastError: s.aiErr}
}

// TestAI 用给定配置发一个极小请求。
func (s *Session) TestAI(ctx context.Context, cfg provider.Config) (string, time.Duration, error) {
	cfg = cfg.Normalize()
	if cfg.Kind == provider.KindOffline {
		return "离线模式无需联网", 0, nil
	}
	if !cfg.Online() {
		return "", 0, errors.New("请填写 Base URL、模型名和 API Key")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	resp, err := provider.Ping(ctx, provider.NewOpenAICompatible(cfg, s.opts.Transport))
	if err != nil {
		return "", 0, err
	}
	return strings.TrimSpace(resp.Text), resp.Latency, nil
}

func (s *Session) components() (resolver.Resolver, narrator.Narrator, bool) {
	s.mu.RLock()
	cfg := s.ai
	s.mu.RUnlock()
	if !cfg.Online() {
		return resolver.Offline{}, narrator.Template{}, false
	}
	p := provider.NewOpenAICompatible(cfg, s.opts.Transport)
	r := &resolver.WithFallback{Primary: &resolver.LLM{Provider: p}, Fallback: resolver.Offline{}, OnFallback: func(err error) {
		s.mu.Lock()
		s.aiErr = "解析降级为离线规则：" + err.Error()
		s.mu.Unlock()
	}}
	return r, &narrator.LLM{Provider: p}, true
}

// ---------- 存档 ----------

func randomSeed() uint64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return binary.LittleEndian.Uint64(b[:])
}

// NewCommandID 生成 command_id（UI 通常自己生成并在重试时复用）。
func NewCommandID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "cmd-" + hex.EncodeToString(b[:])
}

func (g *game) summary(st *state.State) eventstore.Summary {
	sum := eventstore.Summary{Location: g.pkg.EntityName(st.Player.Location), Time: worldtime.Format(st.Minute), Turn: st.Turn, Gold: st.Player.Gold}
	if story := g.q.ActiveStory(st); story != nil {
		sum.Story = story.Title
	}
	return sum
}

// NewGame 用默认故事包新建存档。
func (s *Session) NewGame(ctx context.Context, saveName, playerName string, seed uint64) (string, error) {
	return s.NewGameIn(ctx, "", saveName, playerName, seed)
}

// NewGameIn 用指定故事包新建存档并设为当前游戏（存档绑定故事包 id + 版本，第 44 节）。
// packID 为空时使用默认故事包；seed 为 0 时随机。
func (s *Session) NewGameIn(ctx context.Context, packID, saveName, playerName string, seed uint64) (string, error) {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	if packID == "" {
		packID = s.reg.DefaultID()
	}
	g, err := s.gameFor(packID)
	if err != nil {
		return "", err
	}
	if seed == 0 {
		seed = randomSeed()
	}
	playerName = strings.TrimSpace(playerName)
	if r := []rune(playerName); len(r) > 12 {
		playerName = string(r[:12])
	}
	st := state.New(g.pkg, seed, playerName)
	saveName = strings.TrimSpace(saveName)
	if saveName == "" {
		saveName = st.Player.Name + "的旅程"
	}
	slot := eventstore.Slot{
		ID: eventstore.NewSlotID(), Name: saveName, PlayerName: st.Player.Name, EngineVersion: buildinfo.Version,
		Packages: []eventstore.PackageRef{{ID: g.pkg.Manifest.ID, Version: g.pkg.Manifest.Version}}, Seed: seed, Summary: g.summary(st),
	}
	if err := s.store.CreateSlot(ctx, slot, st); err != nil {
		return "", err
	}
	intro := strings.TrimSpace(g.pkg.Manifest.Start.Intro)
	if err := s.store.AppendEntries(ctx, slot.ID, []eventstore.Entry{{Kind: "intro", Text: intro}}); err != nil {
		return "", err
	}
	s.mu.Lock()
	s.slot, s.st, s.g = slot.ID, st, g
	s.mu.Unlock()
	return slot.ID, nil
}

// LoadGame 读取存档（快照 + 事件），并为缺失叙事的回合补上模板叙事。
func (s *Session) LoadGame(ctx context.Context, slotID string) error {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	sl, err := s.store.GetSlot(ctx, slotID)
	if err != nil {
		return err
	}
	g, err := s.gameForSlot(sl)
	if err != nil {
		return err
	}
	st, err := s.store.Load(ctx, slotID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.slot, s.st, s.g = slotID, st, g
	s.mu.Unlock()
	return s.repairNarrations(ctx, slotID, g)
}

// slotPack 返回存档绑定的故事包（旧存档没有记录时视为默认故事包）。
func (s *Session) slotPack(sl eventstore.Slot) eventstore.PackageRef {
	if len(sl.Packages) > 0 {
		return sl.Packages[0]
	}
	return eventstore.PackageRef{ID: s.reg.DefaultID()}
}

// packStatus 检查存档绑定的故事包是否可用；返回给玩家看的原因（可用时为空）。
func (s *Session) packStatus(ref eventstore.PackageRef) (registry.Entry, string) {
	e, ok := s.reg.Get(ref.ID)
	if !ok {
		return e, fmt.Sprintf("这个存档使用的故事包 %q（%s）没有安装或已被删除。重新导入该故事包后即可继续。", ref.ID, ref.Version)
	}
	if e.Err != "" {
		return e, e.Err
	}
	if ref.Version != "" && ref.Version != e.Manifest.Version {
		ok, err := manifest.Satisfies(ref.Version, e.Manifest.SaveCompat)
		if err != nil || !ok || e.Manifest.SaveCompat == "" {
			return e, fmt.Sprintf("存档使用的是「%s」%s 版，已安装的是 %s 版，两者不兼容。", e.Manifest.Name, ref.Version, e.Manifest.Version)
		}
	}
	return e, ""
}

func (s *Session) gameForSlot(sl eventstore.Slot) (*game, error) {
	ref := s.slotPack(sl)
	if _, msg := s.packStatus(ref); msg != "" {
		return nil, errors.New(msg)
	}
	return s.gameFor(ref.ID)
}

// repairNarrations 处理“事件已提交但叙事未写入”（例如叙事中途退出）：用模板补写，绝不重新执行命令。
func (s *Session) repairNarrations(ctx context.Context, slotID string, g *game) error {
	pend, err := s.store.PendingNarrations(ctx, slotID)
	if err != nil {
		return err
	}
	for _, rec := range pend {
		var res engine.Result
		if err := json.Unmarshal(rec.Result, &res); err != nil || !res.Accepted || len(res.Events) == 0 {
			_ = s.store.SetNarration(ctx, slotID, rec.CommandID, "（这一回合没有留下叙事记录。）", eventstore.Entry{Kind: "narration", Turn: res.Turn})
			continue
		}
		first, last := res.Events[0].Seq, res.Events[len(res.Events)-1].Seq
		before, err := s.store.StateAt(ctx, slotID, first-1)
		if err != nil {
			return err
		}
		after, err := s.store.StateAt(ctx, slotID, last)
		if err != nil {
			return err
		}
		var cmd command.Command
		cmd.ID = rec.CommandID
		for _, e := range res.Events {
			if e.Type == "LocationChanged" {
				cmd.Kind = command.KindMove
			}
		}
		b := narrator.Build(g.pkg, before, after, cmd, &res)
		meta, _ := json.Marshal(map[string]any{"source": "template(recovered)"})
		if err := s.store.SetNarration(ctx, slotID, rec.CommandID, b.Base, eventstore.Entry{Kind: "narration", Turn: res.Turn, Meta: meta}); err != nil {
			return err
		}
	}
	return nil
}

// ListSaves 列出存档。
func (s *Session) ListSaves(ctx context.Context) ([]dto.SlotV1, error) {
	slots, err := s.store.ListSlots(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	cur := s.slot
	s.mu.RUnlock()
	out := make([]dto.SlotV1, 0, len(slots))
	for _, sl := range slots {
		ref := s.slotPack(sl)
		e, msg := s.packStatus(ref)
		name := ref.ID
		if e.Manifest != nil && e.Manifest.Name != "" {
			name = e.Manifest.Name
		}
		out = append(out, dto.SlotV1{ID: sl.ID, Name: sl.Name, PlayerName: sl.PlayerName, Location: sl.Summary.Location, Time: sl.Summary.Time,
			Turn: sl.Summary.Turn, Gold: sl.Summary.Gold, Story: sl.Summary.Story, UpdatedAt: sl.UpdatedAt, CreatedAt: sl.CreatedAt, Current: sl.ID == cur,
			PackID: ref.ID, PackName: name, PackVersion: ref.Version, PackProblem: msg})
	}
	return out, nil
}

// DeleteSave 删除存档；删除当前存档会回到“未载入”状态。
func (s *Session) DeleteSave(ctx context.Context, slotID string) error {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	if err := s.store.DeleteSlot(ctx, slotID); err != nil {
		return err
	}
	s.mu.Lock()
	if s.slot == slotID {
		s.slot, s.st, s.g = "", nil, nil
	}
	s.mu.Unlock()
	return nil
}

// CopySave 复制存档。
func (s *Session) CopySave(ctx context.Context, slotID, name string) (string, error) {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	if strings.TrimSpace(name) == "" {
		sl, err := s.store.GetSlot(ctx, slotID)
		if err != nil {
			return "", err
		}
		name = sl.Name + "（副本）"
	}
	return s.store.CopySlot(ctx, slotID, name)
}

// RenameSave 重命名存档。
func (s *Session) RenameSave(ctx context.Context, slotID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("存档名不能为空")
	}
	return s.store.RenameSlot(ctx, slotID, name)
}

func (s *Session) current() (string, *state.State, *game, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.st == nil {
		return "", nil, nil, ErrNoGame
	}
	return s.slot, s.st, s.g, nil
}

// ---------- 查询 ----------

// Scene 返回场景视图。
func (s *Session) Scene(ctx context.Context) (dto.SceneV1, error) {
	slot, st, g, err := s.current()
	if err != nil {
		return dto.SceneV1{}, err
	}
	v := g.q.Scene(st)
	v.SlotID = slot
	if sl, err := s.store.GetSlot(ctx, slot); err == nil {
		v.SaveName = sl.Name
	}
	return v, nil
}

// Suggestions 返回快捷建议。
func (s *Session) Suggestions() ([]dto.SuggestionV1, error) {
	_, st, g, err := s.current()
	if err != nil {
		return nil, err
	}
	return g.q.Suggestions(st), nil
}

// Character 返回角色面板。
func (s *Session) Character() (dto.CharacterV1, error) {
	_, st, g, err := s.current()
	if err != nil {
		return dto.CharacterV1{}, err
	}
	return g.q.Character(st), nil
}

// Inventory 返回背包面板。
func (s *Session) Inventory() (dto.InventoryV1, error) {
	_, st, g, err := s.current()
	if err != nil {
		return dto.InventoryV1{}, err
	}
	return g.q.Inventory(st), nil
}

// NPCs 返回人物关系面板。
func (s *Session) NPCs() ([]dto.NPCV1, error) {
	_, st, g, err := s.current()
	if err != nil {
		return nil, err
	}
	return g.q.NPCs(st), nil
}

// Journal 返回日志（最新在后）。
func (s *Session) Journal(ctx context.Context) ([]dto.JournalEntryV1, error) {
	slot, st, g, err := s.current()
	if err != nil {
		return nil, err
	}
	evs, err := s.store.Events(ctx, slot, 0, 0)
	if err != nil {
		return nil, err
	}
	return g.q.Journal(evs, st.Player.Name), nil
}

// Transcript 返回对话记录。
func (s *Session) Transcript(ctx context.Context, limit int, beforeID int64) ([]dto.EntryV1, error) {
	slot, _, _, err := s.current()
	if err != nil {
		return nil, err
	}
	es, err := s.store.Transcript(ctx, slot, limit, beforeID)
	if err != nil {
		return nil, err
	}
	return toEntries(es), nil
}

// State 返回当前状态副本（调试 / 测试）。
func (s *Session) State() (*state.State, error) {
	_, st, _, err := s.current()
	if err != nil {
		return nil, err
	}
	return st.Clone(), nil
}

// SlotID 返回当前存档 ID。
func (s *Session) SlotID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.slot
}

type entryMeta struct {
	Check     *dto.CheckV1   `json:"check,omitempty"`
	Chips     []string       `json:"chips,omitempty"`
	Options   []dto.OptionV1 `json:"options,omitempty"`
	Corrected bool           `json:"corrected,omitempty"`
	Source    string         `json:"source,omitempty"`
}

func toEntry(e eventstore.Entry) dto.EntryV1 {
	v := dto.EntryV1{ID: e.ID, CommandID: e.CommandID, Turn: e.Turn, Kind: e.Kind, Text: e.Text}
	if len(e.Meta) > 0 {
		var m entryMeta
		if json.Unmarshal(e.Meta, &m) == nil {
			v.Check, v.Chips, v.Options, v.Corrected, v.Source = m.Check, m.Chips, m.Options, m.Corrected, m.Source
		}
	}
	return v
}

// toEntries 转换并在同一命令内按展示顺序排序（玩家 → 检定 → 叙事 → 系统 → 事件）。
func toEntries(es []eventstore.Entry) []dto.EntryV1 {
	out := make([]dto.EntryV1, 0, len(es))
	for _, e := range es {
		out = append(out, toEntry(e))
	}
	return groupByCommand(out)
}

func groupByCommand(in []dto.EntryV1) []dto.EntryV1 {
	var out []dto.EntryV1
	i := 0
	for i < len(in) {
		j := i + 1
		for j < len(in) && in[j].CommandID == in[i].CommandID && in[i].CommandID != "" {
			j++
		}
		group := append([]dto.EntryV1{}, in[i:j]...)
		slices.SortStableFunc(group, func(a, b dto.EntryV1) int { return query.KindRank(a.Kind) - query.KindRank(b.Kind) })
		out = append(out, group...)
		i = j
	}
	return out
}

func fromEntry(v dto.EntryV1) eventstore.Entry {
	e := eventstore.Entry{CommandID: v.CommandID, Turn: v.Turn, Kind: v.Kind, Text: v.Text}
	m := entryMeta{Check: v.Check, Chips: v.Chips, Options: v.Options, Corrected: v.Corrected, Source: v.Source}
	if m.Check != nil || len(m.Chips) > 0 || len(m.Options) > 0 || m.Corrected || m.Source != "" {
		e.Meta, _ = json.Marshal(m)
	}
	return e
}
