package engine

import (
	"fmt"
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/combat"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
	"github.com/GUYU2233/ibukiRPG/internal/rules/rng"
)

// 战斗双方。
const (
	SideParty = "party"
	SideEnemy = "enemy"
)

// StatusOverheated 是内置的过热状态（跳过下一次行动）。
const StatusOverheated = "overheated"

var builtinStatuses = map[string]*combat.StatusDef{
	StatusOverheated: {ID: StatusOverheated, Name: "过热", Icon: "local_fire_department", Turns: 1, Stun: true, Debuff: true, Description: "机体温度越过红线，驾驶系统强制冷却，跳过下一次行动。"},
}

// StatusDef 返回状态定义（含内置状态）。
func StatusDef(p *loader.Package, id string) *combat.StatusDef {
	if s, ok := p.Combat.Statuses[id]; ok {
		return s
	}
	if s, ok := builtinStatuses[id]; ok {
		return s
	}
	return &combat.StatusDef{ID: id, Name: id}
}

// PlayerStats 返回玩家当前的战斗数值（等级、属性、装备）。
func PlayerStats(p *loader.Package, s *state.State) combat.Stats {
	var eq map[string]string
	if s.RPG != nil {
		eq = s.RPG.Equipment
	}
	return p.StatsFor(p.PlayerCombat(), s.PlayerLevel(p), s.Player.Attributes, eq)
}

// PlayerHP 返回玩家当前生命与最大生命。
func PlayerHP(p *loader.Package, s *state.State) (int, int) {
	mx := PlayerStats(p, s)[combat.HP]
	w := 0
	if s.RPG != nil {
		w = s.RPG.Wounds
	}
	return max(mx-w, 0), mx
}

// EffStats 返回单位的有效数值（机甲形态 + 状态修正）。
func EffStats(p *loader.Package, u *state.Unit) combat.Stats {
	base := combat.Stats(u.Stats)
	if u.Mech != nil {
		base = combat.Stats(u.Mech.Stats)
	}
	out := base.Add(nil)
	for _, st := range u.Statuses {
		out = out.Add(StatusDef(p, st.ID).Mods)
	}
	for _, k := range combat.StatKeys {
		out[k] = max(out[k], 0)
	}
	return out
}

// UnitHP 返回单位当前承受伤害的生命池（机甲形态时为机体耐久）。
func UnitHP(u *state.Unit) (int, int) {
	if u.Mech != nil {
		return u.Mech.HP, u.Mech.MaxHP
	}
	return u.HP, u.MaxHP
}

// UnitSkills 返回单位当前可用的技能（机甲形态使用机甲技能）。
func UnitSkills(p *loader.Package, u *state.Unit) []string {
	var out []string
	if u.Mech != nil {
		out = append(out, u.Mech.Skills...)
	}
	for _, id := range u.Skills {
		sk, ok := p.Combat.Skills[id]
		if !ok {
			continue
		}
		if (u.Mech != nil && sk.HumanOnly) || (u.Mech == nil && sk.MechOnly) || slices.Contains(out, id) {
			continue
		}
		out = append(out, id)
	}
	return out
}

// SkillBlocked 返回技能当前不能使用的原因（空串表示可以使用）。
func SkillBlocked(p *loader.Package, s *state.State, u *state.Unit, sk *combat.SkillDef) string {
	switch {
	case u.Mech == nil && sk.MechOnly:
		return "需要机甲形态"
	case u.Mech != nil && sk.HumanOnly:
		return "机甲形态下无法使用"
	case u.SP < sk.Cost.SP && u.Mech == nil:
		return "体力不足"
	case sk.Cost.Mercury > 0 && (u.Ref != loader.PlayerID || s.R().Mercury < sk.Cost.Mercury):
		return p.Combat.Config.ResourceName + "不足"
	}
	return ""
}

// ---------- 开始 ----------

func (w *work) startCombat(ref string) error {
	p := w.pkg()
	r := w.s.R()
	if r.Combat != nil {
		return reject("战斗已经开始了。")
	}
	var enc combat.Encounter
	nodeID := ""
	{
		e, ok := p.Combat.Encounters[ref]
		if !ok {
			return reject("没有这场战斗。")
		}
		enc = *e
	}
	pc := p.PlayerCombat()
	st := PlayerStats(p, w.s)
	hp, mx := PlayerHP(p, w.s)
	if hp <= 0 {
		hp = 1
	}
	units := []event.Unit{{
		ID: loader.PlayerID, Ref: loader.PlayerID, Side: SideParty, Name: w.s.Player.Name, Level: w.s.PlayerLevel(p),
		HP: hp, MaxHP: mx, SP: st[combat.SP], MaxSP: st[combat.SP], Stats: st, Skills: slices.Clone(r.Skills),
	}}
	if pc.Mech != "" {
		units[0].Tags = []string{"pilot"}
	}
	for _, a := range enc.Allies {
		n := w.s.NPCs[a]
		if n == nil || n.Location == "" {
			continue // 已阵亡或不存在的同伴不会出现
		}
		ast, lvl := p.NPCStats(a)
		c := p.Characters[a]
		units = append(units, event.Unit{ID: a, Ref: a, Side: SideParty, Name: c.Name(), Level: lvl, HP: ast[combat.HP], MaxHP: ast[combat.HP], SP: ast[combat.SP], MaxSP: ast[combat.SP], Stats: ast, Skills: slices.Clone(c.Combat.Skills)})
	}
	count := map[string]int{}
	for _, id := range enc.Enemies {
		count[id]++
	}
	seen := map[string]int{}
	suffix := []string{"甲", "乙", "丙", "丁", "戊", "己", "庚", "辛"}
	for i, id := range enc.Enemies {
		d, ok := p.Combat.Enemies[id]
		if !ok {
			return fmt.Errorf("encounter %s: unknown enemy %s", enc.ID, id)
		}
		name := d.Name
		if count[id] > 1 {
			name += suffix[seen[id]%len(suffix)]
			seen[id]++
		}
		est := p.EnemyStats(d)
		tags := slices.Clone(d.Tags)
		if d.Mech {
			tags = append(tags, "mech")
		}
		units = append(units, event.Unit{ID: fmt.Sprintf("e%d", i+1), Ref: id, Side: SideEnemy, Name: name, Level: d.Level, HP: est[combat.HP], MaxHP: est[combat.HP], SP: est[combat.SP], MaxSP: est[combat.SP], Stats: est, Skills: slices.Clone(d.Skills), Tags: tags})
	}
	vals := map[string]int{}
	if enc.AllowMech {
		vals["mech"] = 1
	}
	if enc.NoFlee {
		vals["no_flee"] = 1
	}
	if err := w.emit(event.CombatStarted, event.Data{Story: enc.ID, Title: enc.Title, Key: nodeID, Units: units, Values: vals, Location: w.s.Player.Location, Text: enc.Intro}); err != nil {
		return err
	}
	for _, id := range enc.Enemies {
		if mc := p.Combat.Enemies[id].MechCard; mc != "" {
			if _, ok := r.Codex[mc]; !ok {
				if err := w.emit(event.CodexUnlocked, event.Data{Key: mc, Reason: "mech"}); err != nil {
					return err
				}
			}
		}
		if _, ok := r.Codex[id]; !ok {
			if err := w.emit(event.CodexUnlocked, event.Data{Key: id, Reason: "encounter"}); err != nil {
				return err
			}
		}
	}
	if err := w.newRound(); err != nil {
		return err
	}
	w.combatTouched = true
	return w.advance()
}

func (w *work) combat() *state.Combat { return w.s.R().Combat }

func (w *work) newRound() error {
	c := w.combat()
	key := rng.Key{Namespace: "combat", Entity: "round", Purpose: "init"}
	counter := w.s.RNG[key.String()]
	g := rng.NewAt(w.s.Seed, key, counter)
	type ini struct {
		id  string
		val int
		idx int
	}
	var list []ini
	vals := map[string]int{}
	for i, u := range c.Units {
		if u.Down {
			continue
		}
		v := EffStats(w.pkg(), u)[combat.SPD] + g.Roll(10)
		list = append(list, ini{u.ID, v, i})
		vals[u.ID] = v
	}
	slices.SortStableFunc(list, func(a, b ini) int {
		if a.val != b.val {
			return b.val - a.val
		}
		return a.idx - b.idx
	})
	order := make([]string, 0, len(list))
	for _, x := range list {
		order = append(order, x.id)
	}
	return w.emit(event.CombatRoundStarted, event.Data{Delta: c.Round + 1, Tags: order, Values: vals, Stream: key.String(), Counter: counter})
}

// advance 推进行动顺序：自动执行敌人与同伴的回合，直到轮到玩家或战斗结束。
func (w *work) advance() error {
	p := w.pkg()
	for guard := 0; guard < 400; guard++ {
		c := w.combat()
		if c == nil {
			return nil
		}
		next := c.Cursor + 1
		if next >= len(c.Order) {
			if c.Round >= p.Combat.Config.MaxRounds {
				return w.endCombat("fled", "stalemate")
			}
			if err := w.newRound(); err != nil {
				return err
			}
			continue
		}
		u := c.Unit(c.Order[next])
		if err := w.emit(event.CombatTurnStarted, event.Data{Actor: u.ID, Delta: next}); err != nil {
			return err
		}
		if u.Down {
			continue
		}
		stunned := slices.ContainsFunc(u.Statuses, func(x state.UnitStatus) bool { return StatusDef(p, x.ID).Stun })
		if err := w.upkeep(u); err != nil {
			return err
		}
		if ended, err := w.checkEnd(); ended || err != nil {
			return err
		}
		if u.Down {
			continue
		}
		if stunned {
			if err := w.emit(event.CombatActed, event.Data{Actor: u.ID, Action: "stunned"}); err != nil {
				return err
			}
			continue
		}
		if u.ID == loader.PlayerID {
			return nil
		}
		if err := w.aiAct(u); err != nil {
			return err
		}
		if ended, err := w.checkEnd(); ended || err != nil {
			return err
		}
	}
	return fmt.Errorf("combat did not converge")
}

// upkeep 是单位回合开始的结算：持续伤害、状态倒计时、体力恢复、机甲能源消耗与冷却。
func (w *work) upkeep(u *state.Unit) error {
	p := w.pkg()
	cfg := &p.Combat.Config
	for _, st := range slices.Clone(u.Statuses) {
		def := StatusDef(p, st.ID)
		dmg := def.DOT
		if def.DotPct != 0 {
			_, mx := UnitHP(u)
			dmg += mx * def.DotPct / 100
		}
		if dmg != 0 {
			if err := w.hurt(u, dmg, "status:"+st.ID); err != nil {
				return err
			}
		}
		if err := w.emit(event.UnitStatusTicked, event.Data{Target: u.ID, Condition: st.ID}); err != nil {
			return err
		}
		if u.Down {
			return nil
		}
	}
	if u.Mech == nil && u.SP < u.MaxSP {
		regen := max(u.MaxSP*cfg.SPRegenPct/100, 1)
		if err := w.emit(event.UnitResource, event.Data{Target: u.ID, Key: "sp", Delta: min(regen, u.MaxSP-u.SP)}); err != nil {
			return err
		}
	}
	if u.Mech != nil {
		m := p.Combat.Mechs[u.Mech.Ref]
		if m != nil && u.Mech.Heat > 0 && m.CoolPerTurn > 0 {
			if err := w.emit(event.UnitResource, event.Data{Target: u.ID, Key: "heat", Delta: -min(m.CoolPerTurn, u.Mech.Heat)}); err != nil {
				return err
			}
		}
		perTurn := 0
		if f, ok := MechForm(p, w.s, u.Mech.Ref); ok {
			perTurn = f.PerTurn
		}
		if m != nil && perTurn > 0 && u.ID == loader.PlayerID {
			have := w.s.R().Mercury
			use := min(perTurn, have)
			if use > 0 {
				if err := w.emit(event.UnitResource, event.Data{Target: u.ID, Key: "mercury", Delta: -use}); err != nil {
					return err
				}
			}
			if have-use <= 0 {
				return w.emit(event.MechDisengaged, event.Data{Target: u.ID, Reason: "mercury"})
			}
		}
	}
	return nil
}

// hurt 对单位造成伤害（负数为治疗），处理倒下与机甲损毁。
func (w *work) hurt(u *state.Unit, dmg int, reason string) error {
	hp, mx := UnitHP(u)
	delta := -dmg
	if delta < -hp {
		delta = -hp
	}
	if delta > mx-hp {
		delta = mx - hp
	}
	if delta == 0 {
		return nil
	}
	if err := w.emit(event.UnitHPChanged, event.Data{Target: u.ID, Delta: delta, Reason: reason}); err != nil {
		return err
	}
	if hp+delta > 0 {
		return nil
	}
	if u.Mech != nil {
		p := w.pkg()
		ref := u.Mech.Ref
		if err := w.emit(event.MechDisengaged, event.Data{Target: u.ID, Reason: "destroyed"}); err != nil {
			return err
		}
		if u.ID == loader.PlayerID {
			if st := p.Combat.Config.MechDestroyedStatus; w.s.MechOf(p, ref).Status != st {
				return w.emit(event.MechStatusChanged, event.Data{Target: ref, Key: st, Reason: "战斗中被击毁"})
			}
		}
		return nil
	}
	return w.emit(event.UnitDefeated, event.Data{Target: u.ID, Reason: reason})
}

// checkEnd 判定胜负：玩家倒下为战败（不是游戏结束），敌人全灭为胜利。
func (w *work) checkEnd() (bool, error) {
	c := w.combat()
	if c == nil {
		return true, nil
	}
	if pu := c.Unit(loader.PlayerID); pu == nil || pu.Down {
		return true, w.endCombat("defeat", "")
	}
	if len(c.Alive(SideEnemy)) == 0 {
		return true, w.endCombat("victory", "")
	}
	return false, nil
}

// ---------- 玩家行动 ----------

func (w *work) execCombat() error {
	if w.cmd.Action == "start" {
		return w.startCombat(w.cmd.Target)
	}
	c := w.combat()
	if c == nil {
		return reject("现在没有在战斗。")
	}
	cur := c.Current()
	if cur == nil || cur.ID != loader.PlayerID {
		return reject("还没轮到你行动。")
	}
	w.combatTouched = true
	if err := w.playerAct(cur); err != nil {
		return err
	}
	if ended, err := w.checkEnd(); ended || err != nil {
		return err
	}
	return w.advance()
}

func (w *work) playerAct(u *state.Unit) error {
	p := w.pkg()
	c := w.combat()
	g, key, counter := w.actStream(u.ID)
	switch w.cmd.Action {
	case "freeform":
		return w.freeformAct(u)
	case "attack":
		t, err := w.pickTarget(u, combat.TargetEnemy, w.cmd.Target)
		if err != nil {
			return err
		}
		return w.perform(u, nil, t, g, key, counter)
	case "skill":
		sk, ok := p.Combat.Skills[w.cmd.Skill]
		if !ok || !slices.Contains(UnitSkills(p, u), sk.ID) {
			return reject("你还不会这个技能。")
		}
		if why := SkillBlocked(p, w.s, u, sk); why != "" {
			return reject("%s：%s。", sk.Name, why)
		}
		t, err := w.pickTarget(u, sk.Target, w.cmd.Target)
		if err != nil {
			return err
		}
		return w.perform(u, sk, t, g, key, counter)
	case "item":
		return w.useCombatItem(u, g, key, counter)
	case "defend":
		if err := w.emit(event.CombatActed, event.Data{Actor: u.ID, Action: "defend", Success: true, Stream: key, Counter: counter}); err != nil {
			return err
		}
		if err := w.emit(event.UnitDefending, event.Data{Target: u.ID}); err != nil {
			return err
		}
		if u.Mech != nil {
			if u.Mech.Heat > 0 {
				return w.emit(event.UnitResource, event.Data{Target: u.ID, Key: "heat", Delta: -min(20, u.Mech.Heat)})
			}
			return nil
		}
		if bonus := min(max(u.MaxSP*15/100, 1), u.MaxSP-u.SP); bonus > 0 {
			return w.emit(event.UnitResource, event.Data{Target: u.ID, Key: "sp", Delta: bonus})
		}
		return nil
	case "flee":
		if c.NoFlee {
			return reject("这场战斗无路可退。")
		}
		ps, es := 0, 0
		for _, x := range c.Alive(SideParty) {
			ps = max(ps, EffStats(p, x)[combat.SPD])
		}
		for _, x := range c.Alive(SideEnemy) {
			es = max(es, EffStats(p, x)[combat.SPD])
		}
		chance := combat.FleeChance(ps, es)
		roll := g.Roll(100)
		ok := roll <= chance
		if err := w.emit(event.CombatActed, event.Data{Actor: u.ID, Action: "flee", Roll: roll, DC: chance, Success: ok, Stream: key, Counter: counter}); err != nil {
			return err
		}
		if ok {
			return w.endCombat("fled", "")
		}
		return nil
	case "mech":
		if u.Mech != nil {
			return reject("你已经在机甲里了。")
		}
		mid := p.PlayerCombat().Mech
		m, ok := p.Combat.Mechs[mid]
		if !ok {
			return reject("你没有可以驾驶的机甲。")
		}
		if !c.AllowMech {
			return reject("这里无法启动%s。", m.Name)
		}
		if m.Requires != "" {
			ok, err := w.eng.Eval.EvalBool(m.Requires, w.vars("", ""))
			if err != nil {
				return err
			}
			if !ok {
				return reject("%s还没有回应你。", m.Name)
			}
		}
		if w.s.R().Mercury < max(m.EngageCost, 1) {
			return reject("%s不足，%s无法启动。", p.Combat.Config.ResourceName, m.Name)
		}
		if why := mechUseBlocked(p, w.s, mid); why != "" {
			return reject("%s。", why)
		}
		form, _ := MechForm(p, w.s, mid)
		if err := w.emit(event.CombatActed, event.Data{Actor: u.ID, Action: "mech", Skill: m.ID, Success: true, Stream: key, Counter: counter}); err != nil {
			return err
		}
		if m.EngageCost > 0 {
			if err := w.emit(event.UnitResource, event.Data{Target: u.ID, Key: "mercury", Delta: -m.EngageCost}); err != nil {
				return err
			}
		}
		ms := form.Stats
		if err := w.revealMech(m.ID, pilotReveal(m), "pilot"); err != nil {
			return err
		}
		return w.emit(event.MechEngaged, event.Data{Target: u.ID, Units: []event.Unit{{ID: u.ID, Ref: m.ID, Name: m.Name, HP: ms[combat.HP], MaxHP: ms[combat.HP], Stats: ms, Skills: form.Skills, HeatMax: form.HeatMax}}})
	case "eject":
		if u.Mech == nil {
			return reject("你并不在机甲里。")
		}
		if err := w.emit(event.CombatActed, event.Data{Actor: u.ID, Action: "eject", Success: true, Stream: key, Counter: counter}); err != nil {
			return err
		}
		return w.emit(event.MechDisengaged, event.Data{Target: u.ID, Reason: "eject"})
	}
	return reject("战斗中可以：攻击、技能、物品、防御、逃跑。")
}

func (w *work) actStream(id string) (*rng.Stream, string, uint64) {
	key := rng.Key{Namespace: "combat", Entity: id, Purpose: "act"}
	counter := w.s.RNG[key.String()]
	return rng.NewAt(w.s.Seed, key, counter), key.String(), counter
}

// pickTarget 解析玩家指定的目标（为空时选第一个合法目标）。
func (w *work) pickTarget(u *state.Unit, kind, want string) ([]*state.Unit, error) {
	c := w.combat()
	foe, ally := SideEnemy, SideParty
	if u.Side == SideEnemy {
		foe, ally = SideParty, SideEnemy
	}
	switch kind {
	case combat.TargetSelf:
		return []*state.Unit{u}, nil
	case combat.TargetAllEnemies:
		return c.Alive(foe), nil
	case combat.TargetAllAllies:
		return c.Alive(ally), nil
	}
	side := foe
	if kind == combat.TargetAlly {
		side = ally
	}
	alive := c.Alive(side)
	if len(alive) == 0 {
		return nil, reject("没有可以选择的目标。")
	}
	if want == "" {
		if kind == combat.TargetAlly {
			return []*state.Unit{u}, nil
		}
		return alive[:1], nil
	}
	for _, x := range alive {
		if x.ID == want || x.Ref == want {
			return []*state.Unit{x}, nil
		}
	}
	return nil, reject("目标不在战场上，或已经倒下。")
}

// perform 结算一次攻击或技能（sk 为 nil 表示普通攻击）。
func (w *work) perform(u *state.Unit, sk *combat.SkillDef, targets []*state.Unit, g *rng.Stream, key string, counter uint64) error {
	p := w.pkg()
	skillID, action := "", "attack"
	power, hits, accB, critB, pierce := 100, 1, 0, 0, 0
	if sk != nil {
		skillID, action = sk.ID, "skill"
		power, hits, accB, critB, pierce = sk.Power, sk.Hits, sk.AccBonus, sk.CritBonus, sk.Pierce
		if sk.Cost.SP > 0 && u.Mech == nil {
			if err := w.emit(event.UnitResource, event.Data{Target: u.ID, Key: "sp", Delta: -min(sk.Cost.SP, u.SP)}); err != nil {
				return err
			}
		}
		if sk.Cost.Mercury > 0 && u.Ref == loader.PlayerID {
			if err := w.emit(event.UnitResource, event.Data{Target: u.ID, Key: "mercury", Delta: -sk.Cost.Mercury}); err != nil {
				return err
			}
		}
	}
	heat := 0
	if u.Mech != nil {
		if m := p.Combat.Mechs[u.Mech.Ref]; m != nil && sk == nil {
			heat = m.AttackHeat
		}
		if sk != nil {
			heat = sk.Cost.Heat
		}
	}
	as := EffStats(p, u)
	first := true
	stamp := func(d event.Data) event.Data {
		if first {
			d.Stream, d.Counter = key, counter
			first = false
		}
		return d
	}
	if sk == nil || sk.Power > 0 {
		for _, t := range targets {
			for h := 0; h < max(hits, 1); h++ {
				if t.Down {
					break
				}
				ts := EffStats(p, t)
				chance := combat.HitChance(as[combat.ACC], ts[combat.EVA], accB)
				roll := g.Roll(100)
				hit := roll <= chance
				cchance := combat.CritChance(as[combat.CRIT], critB)
				croll := g.Roll(100)
				variance := 90 + g.IntN(21)
				d := event.Data{Actor: u.ID, Target: t.ID, Action: action, Skill: skillID, Roll: roll, DC: chance, Success: hit}
				if !hit {
					if err := w.emit(event.CombatActed, stamp(d)); err != nil {
						return err
					}
					continue
				}
				crit := croll <= cchance
				b := combat.Damage(as[combat.ATK], ts[combat.DEF], power, pierce, variance, crit, t.Defending)
				d.Critical, d.Delta = crit, b.Final
				d.Values = map[string]int{"atk": as[combat.ATK], "def": ts[combat.DEF], "power": power, "raw": b.Raw, "mitigated": b.Mitigated, "variance": variance, "crit_chance": cchance, "crit_roll": croll}
				if b.Guarded {
					d.Values["guarded"] = 1
				}
				if err := w.emit(event.CombatActed, stamp(d)); err != nil {
					return err
				}
				if err := w.hurt(t, b.Final, "attack"); err != nil {
					return err
				}
				if sk != nil && sk.Status != nil && !t.Down {
					if err := w.tryStatus(t, sk.Status, g); err != nil {
						return err
					}
				}
			}
		}
	} else {
		ids := []string{}
		for _, t := range targets {
			ids = append(ids, t.ID)
		}
		if err := w.emit(event.CombatActed, stamp(event.Data{Actor: u.ID, Target: strings.Join(ids, ","), Action: action, Skill: skillID, Success: true})); err != nil {
			return err
		}
	}
	if sk != nil {
		for _, t := range targets {
			if t.Down {
				continue
			}
			if err := w.support(t, sk.Heal, sk.HealPct, sk.Restore, sk.Cleanse); err != nil {
				return err
			}
			if sk.Status != nil && sk.Power <= 0 {
				if err := w.tryStatus(t, sk.Status, g); err != nil {
					return err
				}
			}
		}
	}
	if heat > 0 && u.Mech != nil {
		return w.addHeat(u, heat)
	}
	return nil
}

func (w *work) addHeat(u *state.Unit, heat int) error {
	p := w.pkg()
	add := min(heat, u.Mech.HeatMax-u.Mech.Heat)
	if add > 0 {
		if err := w.emit(event.UnitResource, event.Data{Target: u.ID, Key: "heat", Delta: add}); err != nil {
			return err
		}
	}
	if u.Mech.Heat < u.Mech.HeatMax {
		return nil
	}
	if err := w.emit(event.UnitOverheated, event.Data{Target: u.ID}); err != nil {
		return err
	}
	if err := w.emit(event.UnitStatusApplied, event.Data{Target: u.ID, Condition: StatusOverheated, Delta: 1}); err != nil {
		return err
	}
	if err := w.emit(event.UnitResource, event.Data{Target: u.ID, Key: "heat", Delta: -u.Mech.HeatMax / 2}); err != nil {
		return err
	}
	if m := p.Combat.Mechs[u.Mech.Ref]; m != nil && m.OverheatDamage > 0 {
		dmg := max(u.MaxHP*m.OverheatDamage/100, 1)
		dmg = min(dmg, u.HP-1)
		if dmg > 0 {
			return w.emit(event.UnitHPChanged, event.Data{Target: u.ID, Delta: -dmg, Key: "pilot", Reason: "overheat"})
		}
	}
	return nil
}

func (w *work) tryStatus(t *state.Unit, sa *combat.StatusApply, g *rng.Stream) error {
	chance := sa.Chance
	if chance == 0 {
		chance = 100
	}
	if g.Roll(100) > chance {
		return nil
	}
	turns := sa.Turns
	if turns == 0 {
		turns = max(StatusDef(w.pkg(), sa.ID).Turns, 1)
	}
	return w.emit(event.UnitStatusApplied, event.Data{Target: t.ID, Condition: sa.ID, Delta: turns})
}

// support 结算治疗 / 恢复 / 净化。
func (w *work) support(t *state.Unit, heal, healPct int, restore combat.Cost, cleanse bool) error {
	p := w.pkg()
	_, mx := UnitHP(t)
	if amt := heal + mx*healPct/100; amt > 0 {
		if err := w.hurt(t, -amt, "heal"); err != nil {
			return err
		}
	}
	if restore.SP > 0 && t.Mech == nil && t.SP < t.MaxSP {
		if err := w.emit(event.UnitResource, event.Data{Target: t.ID, Key: "sp", Delta: min(restore.SP, t.MaxSP-t.SP)}); err != nil {
			return err
		}
	}
	if restore.Mercury > 0 && t.Ref == loader.PlayerID {
		room := p.Combat.Config.MercuryMax - w.s.R().Mercury
		if add := min(restore.Mercury, room); add > 0 {
			if err := w.emit(event.UnitResource, event.Data{Target: t.ID, Key: "mercury", Delta: add}); err != nil {
				return err
			}
		}
	}
	if restore.Heat > 0 && t.Mech != nil && t.Mech.Heat > 0 {
		if err := w.emit(event.UnitResource, event.Data{Target: t.ID, Key: "heat", Delta: -min(restore.Heat, t.Mech.Heat)}); err != nil {
			return err
		}
	}
	if cleanse {
		for _, st := range slices.Clone(t.Statuses) {
			if StatusDef(p, st.ID).Debuff {
				if err := w.emit(event.UnitStatusRemoved, event.Data{Target: t.ID, Condition: st.ID}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (w *work) useCombatItem(u *state.Unit, g *rng.Stream, key string, counter uint64) error {
	p := w.pkg()
	it, ok := p.Items[w.cmd.Item]
	if !ok || it.Combat == nil {
		return reject("这件东西在战斗中派不上用场。")
	}
	if w.s.Player.Inventory[it.ID] <= 0 {
		return reject("你身上没有%s。", it.Name)
	}
	kind := it.Combat.Target
	if kind == "" {
		kind = combat.TargetSelf
		if it.Combat.Damage > 0 {
			kind = combat.TargetEnemy
		}
	}
	targets, err := w.pickTarget(u, kind, w.cmd.Target)
	if err != nil {
		return err
	}
	if err := w.emit(event.CombatActed, event.Data{Actor: u.ID, Action: "item", Item: it.ID, Success: true, Stream: key, Counter: counter}); err != nil {
		return err
	}
	if err := w.emit(event.ItemRemoved, event.Data{Item: it.ID, Qty: 1}); err != nil {
		return err
	}
	for _, t := range targets {
		if it.Combat.Damage > 0 {
			if err := w.hurt(t, it.Combat.Damage, "item"); err != nil {
				return err
			}
		}
		if t.Down {
			continue
		}
		cure := it.Combat.Cure
		if err := w.support(t, it.Combat.Heal, it.Combat.HealPct, combat.Cost{SP: it.Combat.SP, Mercury: it.Combat.Mercury, Heat: it.Combat.Cool}, false); err != nil {
			return err
		}
		for _, c := range cure {
			if t.HasStatus(c) {
				if err := w.emit(event.UnitStatusRemoved, event.Data{Target: t.ID, Condition: c}); err != nil {
					return err
				}
			}
		}
		if it.Combat.Status != nil {
			if err := w.tryStatus(t, it.Combat.Status, g); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---------- 确定性 AI ----------

func (w *work) aiRules(u *state.Unit) []combat.AIRule {
	p := w.pkg()
	if u.Side == SideEnemy {
		if d := p.Combat.Enemies[u.Ref]; d != nil {
			return d.AI
		}
		return nil
	}
	if c := p.Characters[u.Ref]; c != nil && c.Combat != nil {
		return c.Combat.AI
	}
	return nil
}

func (w *work) aiVars(u *state.Unit) expression.Vars {
	p := w.pkg()
	c := w.combat()
	foe, ally := SideEnemy, SideParty
	if u.Side == SideEnemy {
		foe, ally = SideParty, SideEnemy
	}
	summary := func(side string) map[string]any {
		alive := c.Alive(side)
		minPct, mech := int64(100), false
		for _, x := range alive {
			hp, mx := UnitHP(x)
			minPct = min(minPct, int64(combat.Pct(hp, mx)))
			mech = mech || x.Mech != nil || slices.Contains(x.Tags, "mech")
		}
		return map[string]any{"alive": int64(len(alive)), "min_hp_pct": minPct, "mech": mech}
	}
	hp, mx := UnitHP(u)
	heat := int64(0)
	if u.Mech != nil {
		heat = int64(combat.Pct(u.Mech.Heat, u.Mech.HeatMax))
	}
	sts := []any{}
	for _, s := range u.Statuses {
		sts = append(sts, s.ID)
	}
	self := map[string]any{"id": u.ID, "ref": u.Ref, "hp": int64(hp), "hp_pct": int64(combat.Pct(hp, mx)), "sp": int64(u.SP), "heat_pct": heat, "mech": u.Mech != nil, "statuses": sts}
	_ = p
	return expression.Vars{"self": self, "foes": summary(foe), "allies": summary(ally), "battle": map[string]any{"round": int64(c.Round), "turn": int64(c.Round)}}
}

// aiAct 按确定性规则选择行动：第一个满足条件（且消耗足够、概率命中）的规则生效；
// 否则对生命比例最低的敌人普通攻击。随机性只来自该单位的命名 RNG 流。
func (w *work) aiAct(u *state.Unit) error {
	p := w.pkg()
	g, key, counter := w.actStream(u.ID)
	vars := w.aiVars(u)
	skills := UnitSkills(p, u)
	for _, r := range w.aiRules(u) {
		if r.When != "" {
			ok, err := w.eng.Eval.EvalBool(r.When, vars)
			if err != nil {
				return fmt.Errorf("%s ai: %w", u.Ref, err)
			}
			if !ok {
				continue
			}
		}
		if r.Chance > 0 && g.Roll(100) > r.Chance {
			continue
		}
		if r.Skill == "defend" {
			if err := w.emit(event.CombatActed, event.Data{Actor: u.ID, Action: "defend", Success: true, Stream: key, Counter: counter}); err != nil {
				return err
			}
			return w.emit(event.UnitDefending, event.Data{Target: u.ID})
		}
		var sk *combat.SkillDef
		kind := combat.TargetEnemy
		if r.Skill != "" {
			s, ok := p.Combat.Skills[r.Skill]
			if !ok || !slices.Contains(skills, s.ID) || SkillBlocked(p, w.s, u, s) != "" {
				continue
			}
			sk, kind = s, s.Target
		}
		targets := w.aiTargets(u, kind, r.Target, g)
		if len(targets) == 0 {
			continue
		}
		return w.perform(u, sk, targets, g, key, counter)
	}
	targets := w.aiTargets(u, combat.TargetEnemy, "lowest_hp", g)
	if len(targets) == 0 {
		return w.emit(event.CombatActed, event.Data{Actor: u.ID, Action: "wait", Stream: key, Counter: counter})
	}
	return w.perform(u, nil, targets, g, key, counter)
}

func (w *work) aiTargets(u *state.Unit, kind, how string, g *rng.Stream) []*state.Unit {
	c := w.combat()
	foe, ally := SideEnemy, SideParty
	if u.Side == SideEnemy {
		foe, ally = SideParty, SideEnemy
	}
	switch kind {
	case combat.TargetSelf:
		return []*state.Unit{u}
	case combat.TargetAllEnemies:
		return c.Alive(foe)
	case combat.TargetAllAllies:
		return c.Alive(ally)
	}
	side := foe
	if kind == combat.TargetAlly || how == "weakest_ally" {
		side = ally
		if how == "" {
			how = "weakest_ally"
		}
	}
	alive := c.Alive(side)
	if len(alive) == 0 {
		return nil
	}
	pct := func(x *state.Unit) int { hp, mx := UnitHP(x); return combat.Pct(hp, mx) }
	pick := alive[0]
	switch how {
	case "self":
		return []*state.Unit{u}
	case "random":
		pick = alive[g.IntN(len(alive))]
	case "highest_hp":
		for _, x := range alive {
			if pct(x) > pct(pick) {
				pick = x
			}
		}
	case "player":
		for _, x := range alive {
			if x.ID == loader.PlayerID {
				pick = x
			}
		}
	case "first":
	default: // lowest_hp / weakest_ally
		for _, x := range alive {
			if pct(x) < pct(pick) {
				pick = x
			}
		}
	}
	return []*state.Unit{pick}
}

// ---------- 结束 ----------

var defaultDefeatHP = map[string]int{"captured": 50, "injured": 25, "rescued": 40}

func (w *work) endCombat(outcome, branch string) error {
	p := w.pkg()
	c := w.combat()
	encID, nodeID := c.Encounter, c.Node
	var enc *combat.Encounter
	if e, ok := p.Combat.Encounters[encID]; ok {
		enc = e
	} else {
		enc = &combat.Encounter{ID: encID, Title: c.Title, Defeat: combat.Defeat{Branch: "injured"}}
	}
	pu := c.Unit(loader.PlayerID)
	_, mx := PlayerHP(p, w.s)
	cur, _ := PlayerHP(p, w.s)
	final := cur
	if pu != nil {
		final = pu.HP
	}
	if outcome == "defeat" {
		branch = enc.Defeat.Branch
		if branch == "" {
			branch = "injured"
		}
		pct := enc.Defeat.HPPct
		if pct == 0 {
			pct = defaultDefeatHP[branch]
		}
		final = max(mx*pct/100, 1)
	}
	final = max(final, 1)
	var enemies []*state.Unit
	var allies []string
	for _, u := range c.Units {
		if u.Side == SideEnemy {
			enemies = append(enemies, u)
		} else if u.ID != loader.PlayerID {
			allies = append(allies, u.ID)
		}
	}
	key := rng.Key{Namespace: "combat", Entity: "loot", Purpose: encID}
	counter := w.s.RNG[key.String()]
	d := event.Data{Story: encID, Title: enc.Title, Outcome: outcome, Reason: branch, Key: nodeID}
	if outcome == "victory" {
		d.Stream, d.Counter = key.String(), counter
	}
	if final != cur {
		if err := w.emit(event.PlayerVitals, event.Data{Values: map[string]int{"hp": final - cur}}); err != nil {
			return err
		}
	}
	if err := w.emit(event.CombatEnded, d); err != nil {
		return err
	}
	for _, a := range allies {
		if err := w.emit(event.RelationEdgeChanged, event.Data{Actor: a, Target: loader.PlayerID, Values: map[string]int{"trust": 2}, Reason: "并肩作战：" + enc.Title, Notable: true}); err != nil {
			return err
		}
	}
	switch outcome {
	case "victory":
		xp, gold := enc.XPBonus, enc.GoldBonus
		g := rng.NewAt(w.s.Seed, key, counter)
		for _, u := range enemies {
			def := p.Combat.Enemies[u.Ref]
			xp += def.XP
			gold += def.Gold
			for _, dr := range def.Drops {
				ch := dr.Chance
				if ch == 0 {
					ch = 100
				}
				if g.Roll(100) <= ch {
					if err := w.emit(event.ItemAdded, event.Data{Item: dr.Item, Qty: max(dr.Qty, 1), Reason: "loot"}); err != nil {
						return err
					}
				}
			}
		}
		if gold > 0 {
			if err := w.emit(event.GoldChanged, event.Data{Delta: gold, Reason: "loot"}); err != nil {
				return err
			}
		}
		if err := w.grantXP(xp, "战斗胜利"); err != nil {
			return err
		}
		for _, o := range enc.Victory {
			if err := w.applyOutcome(o, "", ""); err != nil {
				return err
			}
		}
	case "defeat":
		if cond := p.Combat.Config.DefeatConditions[branch]; cond != "" && !w.s.HasCondition(cond) {
			if err := w.emit(event.ConditionApplied, event.Data{Condition: cond}); err != nil {
				return err
			}
		}
		for _, o := range enc.Defeat.Effects {
			if err := w.applyOutcome(o, "", ""); err != nil {
				return err
			}
		}
		if enc.Defeat.MoveTo != "" {
			if err := w.movePlayer(enc.Defeat.MoveTo); err != nil {
				return err
			}
		}
	default:
		for _, o := range enc.Fled {
			if err := w.applyOutcome(o, "", ""); err != nil {
				return err
			}
		}
	}
	w.combatOutcome = outcome
	return nil
}

// grantXP 发放经验并处理升级（升级时回满生命，获得属性点与技能点）。
func (w *work) grantXP(xp int, reason string) error {
	if xp <= 0 {
		return nil
	}
	p := w.pkg()
	cfg := &p.Combat.Config
	if err := w.emit(event.XPGained, event.Data{Delta: xp, Reason: reason}); err != nil {
		return err
	}
	for {
		r := w.s.R()
		lvl := w.s.PlayerLevel(p)
		need := cfg.XPToNext(lvl)
		if lvl >= cfg.MaxLevel || r.XP < need {
			return nil
		}
		if err := w.emit(event.LevelUp, event.Data{Delta: lvl + 1, Values: map[string]int{"cost": need, "attr_points": cfg.AttrPointsPerLevel, "skill_points": cfg.SkillPointsPerLevel}}); err != nil {
			return err
		}
		if r.Wounds > 0 {
			if err := w.emit(event.PlayerVitals, event.Data{Values: map[string]int{"hp": r.Wounds}, Reason: "level_up"}); err != nil {
				return err
			}
		}
	}
}

// Check 在给定状态下求值一个布尔 CEL 表达式（供模拟器 / 查询层判断机甲启动条件）。
func (e *Engine) Check(s *state.State, expr string) (bool, error) {
	if expr == "" {
		return true, nil
	}
	return e.Eval.EvalBool(expr, Vars(e.Pkg, s, "", ""))
}
