package state

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/rules/rng"
)

// SchemaVersion 是 State JSON 结构版本，用于存档迁移。
const SchemaVersion = 1

// 关系数值范围。
const (
	RelMin = -100
	RelMax = 100
	// MaxMemories 是每个 NPC 保留的最近观察条数。
	MaxMemories = 12
	// MaxExchanges 是每个 NPC 保留的最近交谈条数（对话记忆）。
	MaxExchanges = 8
	// MaxEpisodes 是每个 NPC 保留的情节记忆条数（超出时先丢弃最不重要、最旧的）。
	MaxEpisodes = 16
)

// Player 是玩家角色的可变状态。
type Player struct {
	Name       string         `json:"name"`
	Location   string         `json:"location"`
	Gold       int            `json:"gold"`
	Inventory  map[string]int `json:"inventory"`
	Conditions []string       `json:"conditions"`
	Attributes map[string]int `json:"attributes"`
	Skills     map[string]int `json:"skills"`
}

// Belief 是 NPC 对某事的信念，可沿 Event → Observation → Belief 追溯（第 15、16 节）。
type Belief struct {
	Key         string `json:"key"`
	Text        string `json:"text"`
	Source      string `json:"source"`       // witness / dialogue / rumor ...
	SourceEvent string `json:"source_event"` // 触发观察的客观事件类型
	Turn        int    `json:"turn"`
	Minute      int64  `json:"minute"`
	Confidence  int    `json:"confidence"` // 千分比
}

// Memory 是 NPC 的一条观察记忆（短期）。
type Memory struct {
	Text   string `json:"text"`
	Turn   int    `json:"turn"`
	Minute int64  `json:"minute"`
	// Source 是被观察的客观事件类型；Action 是动作 ID（若有），供台词条件与“回忆”使用。
	Source  string `json:"source,omitempty"`
	Action  string `json:"action,omitempty"`
	Target  string `json:"target,omitempty"`
	Notable bool   `json:"notable,omitempty"`
}

// Exchange 是 NPC 与玩家的一次交谈记录（对话记忆）。
type Exchange struct {
	Line   string `json:"line"`
	Topic  string `json:"topic,omitempty"`
	Text   string `json:"text"` // 这次交谈的摘要（NPC 说了什么）
	Turn   int    `json:"turn"`
	Minute int64  `json:"minute"`
}

// Episode 是 NPC 的一条情节记忆：亲历或目击的重要事情（第 13、15 节）。
type Episode struct {
	Key        string `json:"key"`
	Text       string `json:"text"`
	Source     string `json:"source"`
	Importance int    `json:"importance"`
	Turn       int    `json:"turn"`
	Minute     int64  `json:"minute"`
}

// NPC 是 NPC 的可变状态。
type NPC struct {
	Location string   `json:"location"`
	Trust    int      `json:"trust"`
	Fear     int      `json:"fear"`
	Beliefs  []Belief `json:"beliefs"`
	Memories []Memory `json:"memories"`
	// 对话记忆：交谈次数、最后交谈时间、说过的台词（ID → 最后一次说的回合）、谈过的话题次数。
	Talks          int            `json:"talks,omitempty"`
	LastTalkTurn   int            `json:"last_talk_turn,omitempty"`
	LastTalkMinute int64          `json:"last_talk_minute,omitempty"`
	Said           map[string]int `json:"said,omitempty"`
	Topics         map[string]int `json:"topics,omitempty"`
	Exchanges      []Exchange     `json:"exchanges,omitempty"`
	Episodes       []Episode      `json:"episodes,omitempty"`
}

// LastExchange 返回最近一次交谈（没有则 nil）。
func (n *NPC) LastExchange() *Exchange {
	if len(n.Exchanges) == 0 {
		return nil
	}
	return &n.Exchanges[len(n.Exchanges)-1]
}

// Attitude 把关系数值翻译成文字（友好 / 亲近 / 中立 / 戒备 / 敌视 / 畏惧）。
func (n *NPC) Attitude() string {
	switch {
	case n.Fear >= 10 && n.Fear > n.Trust:
		return "畏惧"
	case n.Trust >= 15:
		return "友好"
	case n.Trust >= 5:
		return "亲近"
	case n.Trust <= -5:
		return "敌视"
	case n.Trust < 0:
		return "戒备"
	}
	return "中立"
}

// HasEpisode 报告 NPC 是否记得某件事。
func (n *NPC) HasEpisode(key string) bool {
	return slices.ContainsFunc(n.Episodes, func(e Episode) bool { return e.Key == key })
}

// Story 是 Story Node 的运行状态。
type Story struct {
	Status        string   `json:"status"` // inactive / active / resolved
	StartedTurn   int      `json:"started_turn"`
	StartedMinute int64    `json:"started_minute"`
	Steps         []string `json:"steps"`
	Outcome       string   `json:"outcome"`
}

// Story 状态值。
const (
	StoryInactive = "inactive"
	StoryActive   = "active"
	StoryResolved = "resolved"
)

// Pacing 是节奏状态（第 25 节）。Tension 为千分比。
type Pacing struct {
	Tension       int `json:"tension"`
	QuietTurns    int `json:"quiet_turns"`
	LastNudgeTurn int `json:"last_nudge_turn"`
}

// State 是一个存档的完整游戏状态。只能通过 Apply(event) 改变。
type State struct {
	Schema         int                 `json:"schema"`
	Seed           uint64              `json:"seed"`
	Package        string              `json:"package"`
	PackageVersion string              `json:"package_version"`
	Turn           int                 `json:"turn"`
	Minute         int64               `json:"minute"` // 自第 1 天 00:00 起的分钟数
	Player         Player              `json:"player"`
	NPCs           map[string]*NPC     `json:"npcs"`
	SceneFacts     map[string][]string `json:"scene_facts"`
	Canon          map[string][]string `json:"canon"` // Dynamic Canon：被提升的场景事实
	Flags          map[string]bool     `json:"flags"`
	Stories        map[string]*Story   `json:"stories"`
	Pacing         Pacing              `json:"pacing"`
	Noise          map[string]int      `json:"noise"`
	Vars           map[string]int      `json:"vars,omitempty"` // 故事变量（例如嫌疑值）
	RNG            rng.Counters        `json:"rng"`
	LastSeq        int64               `json:"last_seq"`
	// RPG 是战斗 / 成长 / 图鉴 / 关系网 / 角色卡 / 主线贴合度状态（v0.1.1-rc2，旧存档为空）。
	RPG *RPG `json:"rpg,omitempty"`
}

// New 根据内容包构造初始状态。
func New(p *loader.Package, seed uint64, playerName string) *State {
	if strings.TrimSpace(playerName) == "" {
		playerName = p.Player.Name()
	}
	s := &State{
		Schema:         SchemaVersion,
		Seed:           seed,
		Package:        p.Manifest.ID,
		PackageVersion: p.Manifest.Version,
		Minute:         startMinute(p),
		Player: Player{
			Name:       playerName,
			Location:   p.Manifest.Start.Location,
			Gold:       p.Player.Gold,
			Inventory:  map[string]int{},
			Conditions: []string{},
			Attributes: copyMap(p.Player.Attributes),
			Skills:     copyMap(p.Player.Skills),
		},
		NPCs:       map[string]*NPC{},
		SceneFacts: map[string][]string{},
		Canon:      map[string][]string{},
		Flags:      map[string]bool{},
		Stories:    map[string]*Story{},
		Noise:      map[string]int{},
		Vars:       map[string]int{},
		RNG:        rng.Counters{},
	}
	for k, v := range p.Variables {
		s.Vars[k] = v
	}
	for id, n := range p.Player.Inventory {
		s.Player.Inventory[id] = n
	}
	for _, id := range p.NPCIDs {
		c := p.Characters[id]
		s.NPCs[id] = &NPC{
			Location: c.Location,
			Trust:    c.RelationshipToPlayer["trust"],
			Fear:     c.RelationshipToPlayer["fear"],
			Beliefs:  []Belief{},
			Memories: []Memory{},
		}
	}
	for _, id := range p.StoryIDs {
		s.Stories[id] = &Story{Status: StoryInactive, Steps: []string{}}
	}
	start := p.Locations[p.Manifest.Start.Location]
	s.SceneFacts[start.ID] = append([]string{}, start.SceneFacts...)
	s.initRPG(p)
	return s
}

func startMinute(p *loader.Package) int64 {
	day := p.Manifest.Start.Day
	if day < 1 {
		day = 1
	}
	var h, m int
	if _, err := fmt.Sscanf(p.Manifest.Start.Time, "%d:%d", &h, &m); err != nil {
		h, m = 19, 0
	}
	return int64(day-1)*24*60 + int64(h*60+m)
}

func copyMap(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// Clone 深拷贝状态（JSON 往返，状态很小，简单可靠）。
func (s *State) Clone() *State {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	var c State
	if err := json.Unmarshal(b, &c); err != nil {
		panic(err)
	}
	return &c
}

// Marshal 返回确定性的 JSON 编码（encoding/json 对 map 按键排序）。
func (s *State) Marshal() ([]byte, error) { return json.Marshal(s) }

// Unmarshal 解码状态。
func Unmarshal(b []byte) (*State, error) {
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	if s.Schema != SchemaVersion {
		return nil, fmt.Errorf("unsupported state schema %d", s.Schema)
	}
	if s.RNG == nil {
		s.RNG = rng.Counters{}
	}
	if s.Vars == nil {
		s.Vars = map[string]int{}
	}
	return &s, nil
}

// NPCsAt 返回位于某地点的 NPC（按内容包声明顺序，确定性）。
func (s *State) NPCsAt(p *loader.Package, loc string) []string {
	var out []string
	for _, id := range p.NPCIDs {
		if n, ok := s.NPCs[id]; ok && n.Location == loc {
			out = append(out, id)
		}
	}
	return out
}

// HasCondition 报告玩家是否有某状态。
func (s *State) HasCondition(c string) bool { return slices.Contains(s.Player.Conditions, c) }

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Apply 把一条事件应用到状态。它是纯函数式的 reducer：
// 同样的初始状态 + 同样的事件序列 = 同样的最终状态（Deterministic Replay，第 30 节）。
func Apply(s *State, e event.Event) error {
	d := e.Data
	if e.Seq > s.LastSeq {
		s.LastSeq = e.Seq
	}
	if handled, err := applyRPG(s, e); handled {
		if d.Stream != "" && s.RNG[d.Stream] <= d.Counter {
			s.RNG[d.Stream] = d.Counter + 1
		}
		return err
	}
	// 任何消耗随机数的事件都携带 (stream, counter)，Replay 时据此恢复 RNG 计数器。
	if d.Stream != "" && s.RNG[d.Stream] <= d.Counter {
		s.RNG[d.Stream] = d.Counter + 1
	}
	switch e.Type {
	case event.GameStarted, event.ActionPerformed, event.FreeformPerformed,
		event.NoMechanicalEffect, event.AttentionChanged, event.AmbientEvent:
		// 纯记录事件：不改变结构化状态（其内容会被观察系统与日志消费）。
	case event.SkillCheckResolved:
		// RNG 计数器已在上方统一处理。
	case event.TimeAdvanced:
		if d.Minutes < 0 {
			return fmt.Errorf("negative time advance")
		}
		s.Minute += d.Minutes
	case event.LocationChanged:
		s.Player.Location = d.To
	case event.GoldChanged:
		s.Player.Gold += d.Delta
		if s.Player.Gold < 0 {
			return fmt.Errorf("gold would become negative")
		}
	case event.ItemAdded:
		s.Player.Inventory[d.Item] += d.Qty
	case event.ItemRemoved:
		n := s.Player.Inventory[d.Item] - d.Qty
		if n < 0 {
			return fmt.Errorf("remove %s: not enough", d.Item)
		}
		if n == 0 {
			delete(s.Player.Inventory, d.Item)
			if s.RPG != nil {
				for slot, it := range s.RPG.Equipment {
					if it == d.Item {
						delete(s.RPG.Equipment, slot)
					}
				}
			}
		} else {
			s.Player.Inventory[d.Item] = n
		}
	case event.RelationshipChanged, event.RelationshipNudge:
		n, ok := s.NPCs[d.Target]
		if !ok {
			return fmt.Errorf("unknown npc %q", d.Target)
		}
		n.Trust = clamp(n.Trust+d.Values["trust"], RelMin, RelMax)
		n.Fear = clamp(n.Fear+d.Values["fear"], 0, RelMax)
		if s.RPG != nil {
			s.logRel(e, d.Target, loader.PlayerID, d.Values, true)
		}
	case event.SceneFactChanged:
		facts := s.SceneFacts[d.Location]
		if d.Remove {
			facts = slices.DeleteFunc(facts, func(f string) bool { return f == d.Text })
		} else if !slices.Contains(facts, d.Text) {
			facts = append(facts, d.Text)
		}
		s.SceneFacts[d.Location] = facts
	case event.SceneFactsCleared:
		delete(s.SceneFacts, d.Location)
	case event.SceneFactPromoted:
		if !slices.Contains(s.Canon[d.Location], d.Text) {
			s.Canon[d.Location] = append(s.Canon[d.Location], d.Text)
		}
	case event.ConditionApplied, event.MinorConditionApplied:
		if !slices.Contains(s.Player.Conditions, d.Condition) {
			s.Player.Conditions = append(s.Player.Conditions, d.Condition)
			slices.Sort(s.Player.Conditions)
		}
	case event.ConditionRemoved:
		s.Player.Conditions = slices.DeleteFunc(s.Player.Conditions, func(c string) bool { return c == d.Condition })
	case event.NoiseGenerated:
		s.Noise[d.Location] = clamp(s.Noise[d.Location]+d.Delta, 0, 100)
	case event.FlagSet:
		if d.Remove {
			delete(s.Flags, d.Flag)
		} else {
			s.Flags[d.Flag] = true
		}
	case event.NPCMoved:
		n, ok := s.NPCs[d.Target]
		if !ok {
			return fmt.Errorf("unknown npc %q", d.Target)
		}
		n.Location = d.To
	case event.ObservedEventCreated:
		n, ok := s.NPCs[d.Witness]
		if !ok {
			return fmt.Errorf("unknown witness %q", d.Witness)
		}
		n.Memories = append(n.Memories, Memory{Text: d.Text, Turn: e.Turn, Minute: e.Minute, Source: d.Reason, Action: d.Action, Target: d.Target, Notable: d.Notable})
		if len(n.Memories) > MaxMemories {
			n.Memories = n.Memories[len(n.Memories)-MaxMemories:]
		}
	case event.BeliefUpdated:
		n, ok := s.NPCs[d.Witness]
		if !ok {
			return fmt.Errorf("unknown npc %q", d.Witness)
		}
		b := Belief{Key: d.Key, Text: d.Text, Source: d.Source, SourceEvent: d.Reason, Turn: e.Turn, Minute: e.Minute, Confidence: d.Confidence}
		idx := slices.IndexFunc(n.Beliefs, func(x Belief) bool { return x.Key == d.Key })
		if idx >= 0 {
			n.Beliefs[idx] = b
		} else {
			n.Beliefs = append(n.Beliefs, b)
		}
	case event.StoryStarted:
		st := s.story(d.Story)
		st.Status = StoryActive
		st.StartedTurn = e.Turn
		st.StartedMinute = e.Minute
	case event.StoryStepReached:
		st := s.story(d.Story)
		st.Steps = append(st.Steps, d.Step)
	case event.StoryResolved:
		st := s.story(d.Story)
		st.Status = StoryResolved
		st.Outcome = d.Outcome
	case event.PacingUpdated:
		s.Pacing.Tension = clamp(d.Tension, 0, 1000)
		s.Pacing.QuietTurns = d.QuietTurns
		if d.Nudge {
			s.Pacing.LastNudgeTurn = e.Turn
		}
	case event.DialogueOccurred:
		n, ok := s.NPCs[d.Target]
		if !ok {
			return fmt.Errorf("unknown npc %q", d.Target)
		}
		n.Talks++
		n.LastTalkTurn, n.LastTalkMinute = e.Turn, e.Minute
		if n.Said == nil {
			n.Said = map[string]int{}
		}
		if d.Step != "" {
			n.Said[d.Step] = e.Turn
		}
		if d.Key != "" {
			if n.Topics == nil {
				n.Topics = map[string]int{}
			}
			n.Topics[d.Key]++
		}
		n.Exchanges = append(n.Exchanges, Exchange{Line: d.Step, Topic: d.Key, Text: d.Text, Turn: e.Turn, Minute: e.Minute})
		if len(n.Exchanges) > MaxExchanges {
			n.Exchanges = n.Exchanges[len(n.Exchanges)-MaxExchanges:]
		}
	case event.MemoryRecorded:
		n, ok := s.NPCs[d.Witness]
		if !ok {
			return fmt.Errorf("unknown npc %q", d.Witness)
		}
		ep := Episode{Key: d.Key, Text: d.Text, Source: d.Source, Importance: d.Delta, Turn: e.Turn, Minute: e.Minute}
		if i := slices.IndexFunc(n.Episodes, func(x Episode) bool { return x.Key == d.Key }); i >= 0 {
			n.Episodes = slices.Delete(n.Episodes, i, i+1)
		}
		n.Episodes = append(n.Episodes, ep)
		for len(n.Episodes) > MaxEpisodes {
			// 丢弃最不重要的一条（同等重要时丢最旧的）
			drop := 0
			for i, x := range n.Episodes {
				if x.Importance < n.Episodes[drop].Importance {
					drop = i
				}
			}
			n.Episodes = slices.Delete(n.Episodes, drop, drop+1)
		}
	case event.VarChanged:
		if s.Vars == nil {
			s.Vars = map[string]int{}
		}
		s.Vars[d.Key] += d.Delta
	case event.TurnCompleted:
		s.Turn = e.Turn
	default:
		return fmt.Errorf("unknown event type %q", e.Type)
	}
	return nil
}

func (s *State) story(id string) *Story {
	st, ok := s.Stories[id]
	if !ok {
		st = &Story{Status: StoryInactive, Steps: []string{}}
		s.Stories[id] = st
	}
	return st
}
