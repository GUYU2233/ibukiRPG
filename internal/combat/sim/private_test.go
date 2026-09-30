package sim_test

import (
	"archive/zip"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/combat/sim"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
)

// TestBalancePrivate 模拟私有故事包（不随仓库分发）的主线战斗链。只在设置了环境变量时运行：
//
//	IBUKI_PRIVATE_PACK=/path/to/pack-dir-or.zip go test ./internal/combat/sim -run Private -v
//
// 故事包需要遵循“章节式战斗链”约定：IBUKI_PRIVATE_CHAIN 用逗号列出遭遇战 ID（按顺序），
// 每项可写成 “遭遇ID@地点ID” 以便在开战前移动玩家；以 ! 开头的是可选强敌（只报告胜率，不做断言）。
// 未设置 IBUKI_PRIVATE_CHAIN 时使用天之炽包的默认链。
func TestBalancePrivate(t *testing.T) {
	path := os.Getenv("IBUKI_PRIVATE_PACK")
	if path == "" {
		t.Skip("IBUKI_PRIVATE_PACK not set")
	}
	var fsys fs.FS
	if strings.HasSuffix(path, ".zip") {
		z, err := zip.OpenReader(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = z.Close() }()
		fsys = z
	} else {
		fsys = os.DirFS(path)
	}
	ev, _ := expression.New()
	p, err := loader.Load(fsys, ev)
	if err != nil {
		t.Fatal(err)
	}
	eng := engine.New(p, ev)
	chain := os.Getenv("IBUKI_PRIVATE_CHAIN")
	if chain == "" {
		chain = "tzc:encounter/arena_bout1@tzc:location/ceylon_arena,tzc:encounter/arena_bout2@tzc:location/ceylon_arena," +
			"+dragon_awake,tzc:encounter/hangar_raid@tzc:location/marston_hangar,tzc:encounter/cult_ambush@tzc:location/florence_cathedral," +
			"tzc:encounter/eight_legged@tzc:location/florence_cathedral,!tzc:encounter/king_of_light@tzc:location/florence_square," +
			"tzc:encounter/minerva_breach@tzc:location/minerva,tzc:encounter/omega@tzc:location/minerva"
	}
	type step struct {
		enc, loc string
		optional bool
		flag     string
	}
	var steps []step
	for _, x := range strings.Split(chain, ",") {
		x = strings.TrimSpace(x)
		if strings.HasPrefix(x, "+") {
			steps = append(steps, step{flag: x[1:]})
			continue
		}
		st := step{}
		if strings.HasPrefix(x, "!") {
			st.optional, x = true, x[1:]
		}
		st.enc, st.loc, _ = strings.Cut(x, "@")
		steps = append(steps, st)
	}
	res := map[string]*stats{}
	levels := map[string]int{}
	const seeds = 200
	for seed := uint64(1); seed <= seeds; seed++ {
		s := state.New(p, seed, "")
		for i, st := range steps {
			if st.flag != "" {
				s = s.Clone()
				s.Flags[st.flag] = true
				if m := p.PlayerCombat().Mech; m != "" {
					s.R().Mech(m).Status = "ready"
				}
				continue
			}
			s = s.Clone()
			s.R().Wounds = 0
			s.R().Mercury = p.Combat.Config.MercuryMax
			for _, id := range p.ItemIDs {
				if it := p.Items[id]; it.Combat != nil && it.Combat.Heal > 0 && s.Player.Inventory[id] < 2 {
					s.Player.Inventory[id] = 2
				}
			}
			if st.loc != "" {
				s.Player.Location = st.loc
			}
			if enc := p.Combat.Encounters[st.enc]; enc != nil {
				for _, a := range enc.Allies {
					if n := s.NPCs[a]; n != nil {
						n.Location = s.Player.Location
					}
				}
			}
			r, ns, err := sim.Fight(eng, s, st.enc, "p"+string(rune('a'+i)))
			if err != nil {
				t.Fatalf("seed %d %s: %v", seed, st.enc, err)
			}
			if res[st.enc] == nil {
				res[st.enc] = &stats{}
			}
			res[st.enc].add(r)
			levels[st.enc] += s.PlayerLevel(p)
			if st.optional {
				continue // 可选强敌：不论胜负都不影响后续（用战前状态继续）
			}
			s = ns
		}
	}
	for _, st := range steps {
		if st.flag != "" {
			continue
		}
		r := res[st.enc]
		tag := ""
		if st.optional {
			tag = "（可选强敌）"
		}
		t.Logf("%-36s Lv%-2d win %3d%%  avg hp left %3d%%  avg rounds %2d %s", st.enc, levels[st.enc]/seeds, r.rate(), r.hpSum/max(r.total, 1), r.rounds/max(r.total, 1), tag)
		if st.optional {
			continue
		}
		if r.rate() < 55 {
			t.Errorf("%s: win rate %d%% too low", st.enc, r.rate())
		}
		if avg := r.hpSum / max(r.total, 1); r.rate() == 100 && avg > 92 {
			t.Errorf("%s: too easy (always wins with %d%% hp left)", st.enc, avg)
		}
	}
}
