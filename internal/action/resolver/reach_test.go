package resolver

import (
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

func TestReachableHitPrefersExits(t *testing.T) {
	p := &loader.Package{Locations: map[string]*loader.Location{
		"academy":   {Exits: []loader.Exit{{To: "undercity", Label: "搭列车回锡兰"}, {To: "hangar", Label: "学院机库"}}},
		"undercity": {},
		"arena":     {},
		"hangar":    {},
	}}
	text := "去锡兰"
	// “锡兰”同时命中锡兰斗兽场（arena），但 arena 不是出口；应走标签含“锡兰”的出口。
	hits := []aliasHit{{id: "arena", pos: 3, n: len("锡兰")}}
	if got := reachableHit(p, "academy", text, hits); got != "undercity" {
		t.Fatalf("got %q, want undercity", got)
	}
	// 直接可达的地点优先。
	hits = []aliasHit{{id: "arena", pos: 3, n: 6}, {id: "hangar", pos: 3, n: 6}}
	if got := reachableHit(p, "academy", "去锡兰机库", hits); got != "hangar" {
		t.Fatalf("got %q, want hangar", got)
	}
	// 都不可达时退回第一个（由引擎给出“去不了”的提示）。
	hits = []aliasHit{{id: "arena", pos: 3, n: len("斗兽场")}}
	if got := reachableHit(p, "academy", "去斗兽场", hits); got != "arena" {
		t.Fatalf("got %q, want arena", got)
	}
}
