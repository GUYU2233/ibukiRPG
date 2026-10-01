package engine

import (
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/combat"
	"github.com/GUYU2233/ibukiRPG/internal/combat/freeform"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/knowledge"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

// Profile 返回故事包的随机性档位（玩家不能覆盖，第 19 节决定 3）。
func Profile(p *loader.Package) freeform.Profile { return freeform.ProfileFor(p.Balance.Randomness) }

func clampMod(v int) int { return max(0, min(6, v)) }

// FreeContext 构造自由战斗裁定所需的局面信息（玩家视角：弱点只在知识层已知时标记 Known）。
func FreeContext(p *loader.Package, s *state.State, u *state.Unit) freeform.Context {
	c := s.RPG.Combat
	ctx := freeform.Context{SizeClasses: p.Balance.SizeClasses, ActorSize: "human", Maneuvers: p.Combat.Maneuvers, Tags: p.Combat.Tags,
		AbsurdWords: p.Balance.AbsurdWords, InMech: u.Mech != nil}
	if p.Player != nil && p.Player.Size != "" {
		ctx.ActorSize = p.Player.Size
	}
	as := EffStats(p, u)
	ctx.AttrMod = clampMod((as[combat.ATK] + as[combat.ACC]/10 - 10) / 3)
	if u.MaxSP > 0 && u.SP*4 < u.MaxSP && u.Mech == nil {
		ctx.SPLow = true
	}
	for _, t := range c.Alive(SideEnemy) {
		ts := EffStats(p, t)
		tg := freeform.Target{ID: t.ID, Name: t.Name, Defense: clampMod((ts[combat.DEF] + ts[combat.EVA]/4) / 3), Size: "human"}
		if e := p.Combat.Enemies[t.Ref]; e != nil {
			tg.Aliases = append(tg.Aliases, e.Aliases...)
			tg.Aliases = append(tg.Aliases, e.Name, t.Ref)
			if e.Size != "" {
				tg.Size = e.Size
			}
			tg.Mech = e.Mech
			switch e.Tier {
			case "elite":
				tg.Defense++
			case "boss":
				tg.Defense += 2
			}
			for _, pt := range e.Parts {
				mult := pt.Mult
				if mult == 0 {
					mult = 100
				}
				tg.Parts = append(tg.Parts, freeform.Part{ID: pt.ID, Name: pt.Name, Mult: mult, DC: pt.DC, Weak: pt.Weak,
					Known: s.Knowledge.Knows(t.Ref, "part:"+pt.ID)})
			}
		}
		if t.Mech != nil {
			tg.Mech = true
		}
		for _, st := range t.Statuses {
			if def := StatusDef(p, st.ID); def != nil {
				tg.Statuses = append(tg.Statuses, def.Tags...)
			}
			tg.Statuses = append(tg.Statuses, loader.Key(st.ID))
		}
		if t.Defending {
			tg.Statuses = append(tg.Statuses, "guarded")
		}
		if enc := p.Combat.Encounters[c.Encounter]; enc != nil && enc.Ambush && c.Round <= 1 {
			tg.Unaware = true
		}
		ctx.Targets = append(ctx.Targets, tg)
	}
	// 武器 / 工具：已装备 + 背包里的武器
	equipped := map[string]bool{}
	if s.RPG != nil {
		for _, id := range s.RPG.Equipment {
			equipped[id] = true
		}
	}
	for _, id := range p.ItemIDs {
		it := p.Items[id]
		if it.Kind != "weapon" && !it.Quick {
			continue
		}
		owned := s.Player.Inventory[id] > 0
		if !owned && !equipped[id] {
			continue
		}
		ctx.Means = append(ctx.Means, freeform.Means{ID: id, Name: it.Name, Aliases: it.Aliases, Equipped: equipped[id], Quick: it.Quick, Owned: owned, Pierce: it.Pierce})
	}
	for _, id := range UnitSkills(p, u) {
		sk := p.Combat.Skills[id]
		if sk == nil {
			continue
		}
		why := SkillBlocked(p, s, u, sk)
		ctx.Skills = append(ctx.Skills, freeform.SkillInfo{ID: id, Name: sk.Name, Usable: why == "", Why: why, Bonus: 1})
	}
	ctx.SceneFacts = s.SceneFacts[s.Player.Location]
	return ctx
}

// freeformAct 结算一次自由战斗行动（第 9 节）：合理性检查 → 修正 → 程度判定 → 伤害 / 状态 / 后果。
func (w *work) freeformAct(u *state.Unit) error {
	if w.cmd.Intent == nil {
		return reject("你想怎么做？")
	}
	p, s := w.pkg(), w.s
	in := *w.cmd.Intent
	ctx := FreeContext(p, s, u)
	ctx.Local = in.Source == "local"
	prof := Profile(p)
	res := freeform.Check(ctx, in, prof)
	if res.Plausibility == freeform.Impossible {
		reason := res.Reason
		if len(res.Options) > 0 {
			reason += "可以试试：" + strings.Join(res.Options, "、") + "。"
		}
		return reject("%s", reason)
	}
	if err := w.emit(event.CombatIntentParsed, event.Data{Intent: &in, Text: in.Raw, Source: in.Source}); err != nil {
		return err
	}
	final := res.Final
	switch final.Kind {
	case freeform.KindDefend, freeform.KindMove:
		if err := w.emit(event.ActionAdjudicated, event.Data{Actor: u.ID, Adjudication: &freeform.Adjudication{Check: res, Band: prof.Band}}); err != nil {
			return err
		}
		w.cmd.Action = "defend"
		return w.playerAct(u)
	case freeform.KindFlee:
		w.cmd.Action = "flee"
		return w.playerAct(u)
	case freeform.KindItem:
		w.cmd.Action, w.cmd.Item = "item", final.Item
		return w.playerAct(u)
	case freeform.KindMech:
		w.cmd.Action = "mech"
		if u.Mech != nil {
			w.cmd.Action = "eject"
		}
		return w.playerAct(u)
	}
	var t *state.Unit
	if res.Target != nil {
		t = w.combat().Unit(res.Target.ID)
	}
	if t == nil || t.Down {
		return reject("目标已经倒下了。")
	}
	g, key, counter := w.actStream(u.ID)
	out := freeform.Resolve(res, prof, func(sides int) int { return g.Roll(sides) })
	adj := &freeform.Adjudication{Check: res, Roll: &out, Band: prof.Band, Consequences: freeform.Consequences(prof, out.Degree)}
	pct, applyStatus, extra := freeform.Effect(out.Degree)
	var sk *combat.SkillDef
	power, pierce := 100, 0
	switch final.Kind {
	case freeform.KindSkill:
		sk = p.Combat.Skills[final.Skill]
		if sk != nil {
			power, pierce = sk.Power, sk.Pierce
			if sk.Cost.SP > 0 && u.Mech == nil {
				if err := w.emit(event.UnitResource, event.Data{Target: u.ID, Key: "sp", Delta: -min(sk.Cost.SP, u.SP)}); err != nil {
					return err
				}
			}
		}
	case freeform.KindManeuver:
		power = 0
		if res.Maneuver != nil {
			power = res.Maneuver.Power
		}
	case freeform.KindEnv:
		power = 80
	case freeform.KindTalk:
		power = 0
	}
	dmg := 0
	if pct > 0 && power > 0 {
		as, ts := EffStats(p, u), EffStats(p, t)
		v := prof.VariancePct
		variance := 100 - v + g.IntN(2*v+1)
		b := combat.Damage(as[combat.ATK], ts[combat.DEF], power, pierce, variance, out.Degree == freeform.Crit, t.Defending)
		dmg = b.Final * pct / 100
		if res.Part != nil && res.Part.Mult > 0 {
			dmg = dmg * res.Part.Mult / 100
		}
		for _, d := range res.DCParts {
			if d.Label == "以凡人之躯对抗机甲" {
				dmg = dmg * p.Balance.MortalVsMech / 100
			}
		}
		dmg = max(dmg, 1)
	}
	adj.Damage = dmg
	// 状态
	status, turns := "", 0
	if applyStatus {
		switch {
		case res.Maneuver != nil && res.Maneuver.Status != "":
			status, turns = res.Maneuver.Status, max(res.Maneuver.Turns, 1)
		case sk != nil && sk.Status != nil:
			status, turns = sk.Status.ID, max(sk.Status.Turns, 1)
		}
		if status != "" {
			turns += extra
			adj.Status = status
		}
	}
	// 部位
	var partBroken bool
	if res.Part != nil && dmg > 0 {
		if e := p.Combat.Enemies[t.Ref]; e != nil {
			for _, pt := range e.Parts {
				if pt.ID == res.Part.ID && pt.Break > 0 && t.PartDamage[pt.ID]+dmg >= pt.Break && !slices.Contains(t.Broken, pt.ID) {
					partBroken = true
				}
			}
		}
	}
	adj.PartBroken = partBroken
	adj.PartRevealed = res.Part != nil && !res.Part.Known && freeform.Succeeded(out.Degree)
	if err := w.emit(event.ActionAdjudicated, event.Data{Actor: u.ID, Target: t.ID, Adjudication: adj, Stream: key, Counter: counter}); err != nil {
		return err
	}
	d := event.Data{Actor: u.ID, Target: t.ID, Action: "freeform", Skill: final.Skill, Roll: out.Roll, DC: res.DC, Total: out.Total,
		Success: freeform.Succeeded(out.Degree), Critical: out.Degree == freeform.Crit, Fumble: out.Degree == freeform.CritFail, Delta: dmg,
		Key: out.Degree, Text: in.Raw}
	if err := w.emit(event.CombatActed, d); err != nil {
		return err
	}
	if dmg > 0 {
		if err := w.hurt(t, dmg, "attack"); err != nil {
			return err
		}
	}
	if res.Part != nil && dmg > 0 {
		if err := w.emit(event.PartHit, event.Data{Target: t.ID, Key: res.Part.ID, Delta: res.Part.Mult, Total: dmg, Remove: partBroken}); err != nil {
			return err
		}
		if partBroken {
			if e := p.Combat.Enemies[t.Ref]; e != nil {
				for _, pt := range e.Parts {
					if pt.ID == res.Part.ID && pt.Status != "" && !t.Down {
						if err := w.emit(event.UnitStatusApplied, event.Data{Target: t.ID, Condition: pt.Status, Delta: 2}); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	if adj.PartRevealed && p.Doc(t.Ref) != nil {
		if err := w.emit(event.KnowledgeRevealed, event.Data{Reveal: &knowledge.Reveal{Entity: t.Ref, Fields: []string{"part:" + res.Part.ID}, Channel: knowledge.ChannelWitness}}); err != nil {
			return err
		}
	}
	if status != "" && !t.Down && StatusDef(p, status) != nil {
		if err := w.emit(event.UnitStatusApplied, event.Data{Target: t.ID, Condition: status, Delta: turns}); err != nil {
			return err
		}
	}
	return w.consequences(u, t, adj.Consequences)
}

// consequences 应用失败后果（失衡 / 被反击 / 额外体力 / 武器卡住 / 受伤）。
func (w *work) consequences(u, t *state.Unit, cs []string) error {
	p := w.pkg()
	for _, c := range cs {
		switch c {
		case "stamina":
			if u.Mech == nil && u.SP > 0 {
				if err := w.emit(event.UnitResource, event.Data{Target: u.ID, Key: "sp", Delta: -min(5, u.SP)}); err != nil {
					return err
				}
			}
		case "counter":
			if t != nil && !t.Down && !u.Down {
				g, key, counter := w.actStream(t.ID)
				if err := w.perform(t, nil, []*state.Unit{u}, g, key, counter); err != nil {
					return err
				}
			}
		case "off_balance", "jam", "wound":
			id := firstStatus(p, c)
			if id != "" && !u.Down {
				if err := w.emit(event.UnitStatusApplied, event.Data{Target: u.ID, Condition: id, Delta: 1}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// firstStatus 找到带某个标签（或 ID 以其结尾）的状态定义。
func firstStatus(p *loader.Package, tag string) string {
	for _, id := range p.Combat.StatusIDs {
		st := p.Combat.Statuses[id]
		if loader.Key(id) == tag || slices.Contains(st.Tags, tag) {
			return id
		}
	}
	return ""
}
