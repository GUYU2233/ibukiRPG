// Package sim 是战斗模拟器：用一个确定性的“合理玩家”策略自动打完一场战斗，
// 供平衡性测试（胜率、剩余生命、回合数）与命令行演示使用。
package sim

import (
	"fmt"
	"slices"

	"github.com/GUYU2233/ibukiRPG/internal/combat"
	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

// Policy 返回玩家在当前战斗回合的指令（合理但不完美的策略）。
func Policy(eng *engine.Engine, s *state.State) command.Command {
	p := eng.Pkg
	c := s.R().Combat
	u := c.Unit(loader.PlayerID)
	cmd := command.Command{Kind: command.KindCombat, Source: "sim"}
	hp, mx := engine.UnitHP(u)
	// 1) 危险时用回复物品
	if u.Mech == nil && hp*100 < mx*35 {
		for _, id := range p.ItemIDs {
			it := p.Items[id]
			if it.Combat != nil && (it.Combat.Heal > 0 || it.Combat.HealPct > 0) && s.Player.Inventory[id] > 0 {
				cmd.Action, cmd.Item = "item", id
				return cmd
			}
		}
	}
	// 2) 能启动机甲就启动（敌人里有机甲或首领时）
	if u.Mech == nil && c.AllowMech {
		if m, ok := p.Combat.Mechs[p.PlayerCombat().Mech]; ok && s.R().Mercury >= max(m.EngageCost, 1)+m.PerTurn*2 {
			tough := false
			for _, e := range c.Alive(engine.SideEnemy) {
				d := p.Combat.Enemies[e.Ref]
				tough = tough || slices.Contains(e.Tags, "mech") || (d != nil && d.Tier == "boss")
			}
			if ok, _ := eng.Check(s, m.Requires); tough && ok {
				cmd.Action = "mech"
				return cmd
			}
		}
	}
	skills := engine.UnitSkills(p, u)
	usable := func(id string) *combat.SkillDef {
		sk := p.Combat.Skills[id]
		if sk == nil || !slices.Contains(skills, id) || engine.SkillBlocked(p, s, u, sk) != "" {
			return nil
		}
		return sk
	}
	// 3) 机甲形态：热量管理
	if u.Mech != nil {
		heatPct := combat.Pct(u.Mech.Heat, u.Mech.HeatMax)
		if heatPct >= 60 {
			for _, id := range skills {
				if sk := usable(id); sk != nil && sk.Restore.Heat > 0 {
					cmd.Action, cmd.Skill = "skill", id
					return cmd
				}
			}
		}
		best := ""
		for _, id := range skills {
			sk := usable(id)
			if sk == nil || sk.Power <= 0 || u.Mech.Heat+sk.Cost.Heat >= u.Mech.HeatMax {
				continue
			}
			if best == "" || sk.Power*targetsOf(c, sk) > p.Combat.Skills[best].Power*targetsOf(c, p.Combat.Skills[best]) {
				best = id
			}
		}
		if best != "" {
			cmd.Action, cmd.Skill = "skill", best
			return cmd
		}
		cmd.Action = "attack"
		return cmd
	}
	// 4) 人形态：同伴 / 自己重伤时治疗，否则用最强的攻击技能
	for _, id := range skills {
		sk := usable(id)
		if sk != nil && (sk.Heal > 0 || sk.HealPct > 0) && hp*100 < mx*50 {
			cmd.Action, cmd.Skill = "skill", id
			return cmd
		}
	}
	best := ""
	for _, id := range skills {
		sk := usable(id)
		if sk == nil || sk.Power <= 0 {
			continue
		}
		score := sk.Power * sk.Hits * targetsOf(c, sk)
		if best == "" || score > p.Combat.Skills[best].Power*p.Combat.Skills[best].Hits*targetsOf(c, p.Combat.Skills[best]) {
			best = id
		}
	}
	if best != "" {
		cmd.Action, cmd.Skill = "skill", best
		return cmd
	}
	cmd.Action = "attack"
	return cmd
}

func targetsOf(c *state.Combat, sk *combat.SkillDef) int {
	if sk.Target == combat.TargetAllEnemies {
		return max(len(c.Alive(engine.SideEnemy)), 1)
	}
	return 1
}

// Result 是一场模拟战斗的结果。
type Result struct {
	Outcome   string
	Rounds    int
	HPLeftPct int
	Actions   int
}

// Fight 在 s 上开始遭遇战并用 Policy 打完，返回结果与战后状态。
func Fight(eng *engine.Engine, s *state.State, encounter string, idPrefix string) (Result, *state.State, error) {
	n := 0
	exec := func(c command.Command) (*engine.Result, error) {
		n++
		c.ID = fmt.Sprintf("%s-%d", idPrefix, n)
		res, ns, err := eng.Execute(s, c)
		if err != nil {
			return nil, err
		}
		if !res.Accepted {
			return res, fmt.Errorf("rejected %s/%s: %s", c.Action, c.Skill, res.Reason)
		}
		s = ns
		return res, nil
	}
	if s.R().Combat == nil {
		if _, err := exec(command.Command{Kind: command.KindCombat, Action: "start", Target: encounter}); err != nil {
			return Result{}, s, err
		}
	}
	rounds, hpPct := 0, 0
	for s.RPG.Combat != nil {
		rounds = s.RPG.Combat.Round
		if n > 300 {
			return Result{}, s, fmt.Errorf("fight did not end")
		}
		before := s.RPG.Combat.Clone()
		r, err := exec(Policy(eng, s))
		if err != nil {
			return Result{}, s, err
		}
		// 战斗结束时（升级回血之前）的生命：在行动前的单位快照上重放本回合的伤害事件。
		hpPct = endHP(before, r)
		if s.RPG.Combat != nil {
			hp, mx := engine.UnitHP(s.RPG.Combat.Unit(loader.PlayerID))
			hpPct = combat.Pct(hp, mx)
		}
	}
	res := Result{Rounds: rounds, Actions: n, HPLeftPct: hpPct}
	rec := s.RPG.Encounters[encounter]
	if rec != nil {
		res.Outcome = rec.Result
	}
	return res, s, nil
}

func endHP(before *state.Combat, r *engine.Result) int {
	s := &state.State{RPG: &state.RPG{Combat: before}}
	for _, e := range r.Events {
		if e.Type == event.CombatEnded {
			break
		}
		switch e.Type {
		case event.UnitHPChanged, event.MechEngaged, event.MechDisengaged, event.UnitDefeated:
			_ = state.Apply(s, e)
		}
	}
	u := before.Unit(loader.PlayerID)
	hp, mx := engine.UnitHP(u)
	return combat.Pct(hp, mx)
}
