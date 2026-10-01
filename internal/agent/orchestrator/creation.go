package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/router"
	"github.com/GUYU2233/ibukiRPG/internal/ai/structured"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

// CreationOptions 返回故事包的角色创建选项（预设主角 / 出身 / 属性点）。
func (s *Session) CreationOptions(packID string) (dto.CreationOptionsV1, error) {
	p, err := s.reg.Load(packID)
	if err != nil {
		return dto.CreationOptionsV1{}, err
	}
	out := dto.CreationOptionsV1{PackID: p.Manifest.ID, AttributeNames: p.Rules.Attributes, BaseAttributes: p.Player.Attributes,
		AttributePoints: p.Creation.AttributePoints, AttrMax: p.Creation.AttrMax, Rules: p.Creation.Rules, MaxPower: maxPower(p)}
	out.Presets = append(out.Presets, dto.CreationPresetV1{ID: loader.PlayerID, Name: p.Player.Name(), Description: p.Player.Description, Attributes: p.Player.Attributes})
	for _, id := range p.NPCIDs {
		if ch := p.Characters[id]; ch != nil && ch.Playable {
			out.Presets = append(out.Presets, dto.CreationPresetV1{ID: id, Name: ch.Name(), Description: ch.Description, Attributes: ch.Attributes})
		}
	}
	for _, b := range p.Creation.Backgrounds {
		out.Backgrounds = append(out.Backgrounds, dto.CreationBackgroundV1{ID: b.ID, Name: b.Name, Description: b.Description, Attributes: b.Attributes, Gold: b.Gold})
	}
	return out, nil
}

func maxPower(p *loader.Package) int {
	if p.Creation.MaxPower > 0 {
		return p.Creation.MaxPower
	}
	if t := p.Balance.Tier(p.Balance.PlayerStartTier); t != nil {
		return t.Power[1]
	}
	return 0
}

// creationPower 计算自建 / 预设角色的强度分。
func creationPower(p *loader.Package, c engine.Creation) int {
	attrs := map[string]int{}
	skills := map[string]int{}
	src := p.Player
	if c.Preset != "" && c.Preset != loader.PlayerID && p.Characters[c.Preset] != nil {
		src = p.Characters[c.Preset]
	}
	for k, v := range src.Attributes {
		attrs[k] = v
	}
	for k, v := range src.Skills {
		skills[k] = v
	}
	if c.Custom {
		for k, v := range c.Attributes {
			attrs[k] += v
		}
		if b := p.Creation.Background(c.Background); b != nil {
			for k, v := range b.Attributes {
				attrs[k] += v
			}
		}
	}
	return engine.PowerScore(p, attrs, skills, nil)
}

const charReviewSystem = `你是 TRPG 角色审查员。根据故事包的世界规则检查玩家自建的角色：是否符合世界观（lore_fit: ok|minor|bad）、强度是否超标、是否与已有设定冲突。
只输出一个 JSON：{"lore_fit":"ok|minor|bad","conflicts":["..."],"suggestions":["..."],"recommended":{"name":"","background":"","appearance":"","personality":"","story":""}}
recommended 是你建议的修改后版本（保留玩家意图，只改有问题的部分）。不要编造故事包里没有的势力或地点。`

// ReviewCreation 审查自建角色：先规则层（属性点 / 禁用词 / 强度），再审查 Agent（世界观 / 冲突），给出建议与推荐角色卡。
// 没有 AI 时只返回规则层结果与一张按规则修正过的推荐卡。
func (s *Session) ReviewCreation(ctx context.Context, packID string, c engine.Creation) (dto.CreationReviewV1, error) {
	p, err := s.reg.Load(packID)
	if err != nil {
		return dto.CreationReviewV1{}, err
	}
	rv := dto.CreationReviewV1{Source: "rules", Problems: engine.CheckCreation(p, c), Power: creationPower(p, c), MaxPower: maxPower(p), LoreFit: "ok"}
	if rv.MaxPower > 0 && rv.Power > rv.MaxPower {
		rv.Problems = append(rv.Problems, fmt.Sprintf("强度 %d 超过开局上限 %d", rv.Power, rv.MaxPower))
	}
	rec := fixCreation(p, c)
	if c.Custom {
		if pr, _, ok := s.providerFor(router.TaskCharReview); ok {
			s.aiReview(ctx, pr, p, c, &rv, &rec)
		}
	}
	rv.OK = len(rv.Problems) == 0 && rv.LoreFit != "bad"
	rv.Recommended, _ = json.Marshal(rec)
	return rv, nil
}

func (s *Session) aiReview(ctx context.Context, pr provider.Provider, p *loader.Package, c engine.Creation, rv *dto.CreationReviewV1, rec *engine.Creation) {
	var bgs []string
	for _, b := range p.Creation.Backgrounds {
		bgs = append(bgs, b.ID+"="+b.Name+"："+b.Description)
	}
	worldJSON, _ := json.Marshal(map[string]any{"story": p.Manifest.Name, "summary": p.Manifest.Description, "rules": p.Creation.Rules,
		"backgrounds": bgs, "limits": p.Balance.AbilityLimits, "forbidden": p.Creation.Forbidden})
	charJSON, _ := json.Marshal(c)
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	resp, err := pr.Generate(cctx, provider.Request{Messages: []provider.Message{{Role: "system", Content: charReviewSystem},
		{Role: "user", Content: fmt.Sprintf("[WORLD]\n%s\n[CHARACTER]\n%s", worldJSON, charJSON)}}, Temperature: 0.3, MaxTokens: 600, JSON: true})
	if err != nil || structured.IsRefusal(resp.Text, nil) {
		return
	}
	var out struct {
		LoreFit     string          `json:"lore_fit"`
		Conflicts   []string        `json:"conflicts"`
		Suggestions []string        `json:"suggestions"`
		Recommended engine.Creation `json:"recommended"`
	}
	raw := strings.TrimSpace(resp.Text)
	if i, j := strings.Index(raw, "{"), strings.LastIndex(raw, "}"); i >= 0 && j > i {
		raw = raw[i : j+1]
	}
	if json.Unmarshal([]byte(raw), &out) != nil {
		return
	}
	rv.Source = "ai"
	switch out.LoreFit {
	case "ok", "minor", "bad":
		rv.LoreFit = out.LoreFit
	}
	rv.Conflicts, rv.Suggestions = out.Conflicts, out.Suggestions
	// 推荐卡只采纳文字字段；属性与出身仍由规则层决定（AI 不能抬高强度）
	r := out.Recommended
	if r.Name != "" {
		rec.Name = r.Name
	}
	if r.Background != "" && p.Creation.Background(r.Background) != nil {
		rec.Background = r.Background
	}
	for _, f := range []struct {
		dst *string
		v   string
		max int
	}{{&rec.Appearance, r.Appearance, 200}, {&rec.Personality, r.Personality, 200}, {&rec.Story, r.Story, 400}} {
		if f.v != "" && len([]rune(f.v)) <= f.max {
			*f.dst = f.v
		}
	}
	if len(engine.CheckCreation(p, *rec)) > 0 {
		*rec = fixCreation(p, c)
	}
}

// fixCreation 按规则修正角色（裁剪属性点、补出身、截断文字、去掉禁用词），作为推荐卡的底稿。
func fixCreation(p *loader.Package, c engine.Creation) engine.Creation {
	r := c
	if !c.Custom {
		return r
	}
	cr := &p.Creation
	if len(cr.Backgrounds) > 0 && cr.Background(r.Background) == nil {
		r.Background = cr.Backgrounds[0].ID
	}
	attrs := map[string]int{}
	left := cr.AttributePoints
	for _, k := range slices.Sorted(maps.Keys(c.Attributes)) {
		if _, ok := p.Rules.Attributes[k]; !ok {
			continue
		}
		v := max(c.Attributes[k], 0)
		if cr.AttrMax > 0 {
			v = min(v, cr.AttrMax-p.Player.Attributes[k])
		}
		v = max(min(v, left), 0)
		if v > 0 {
			attrs[k] = v
			left -= v
		}
	}
	r.Attributes = attrs
	cut := func(s string, n int) string {
		for _, f := range cr.Forbidden {
			if f != "" {
				s = strings.ReplaceAll(s, f, "")
			}
		}
		if rs := []rune(s); len(rs) > n {
			return string(rs[:n])
		}
		return s
	}
	r.Name, r.Appearance, r.Personality, r.Story = cut(r.Name, 20), cut(r.Appearance, 200), cut(r.Personality, 200), cut(r.Story, 400)
	return r
}
