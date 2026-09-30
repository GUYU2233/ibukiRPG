package loader_test

import (
	"os"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
	"github.com/GUYU2233/ibukiRPG/packages"
)

func TestLoadDemo(t *testing.T) {
	ev, err := expression.New()
	if err != nil {
		t.Fatal(err)
	}
	p, err := loader.Load(packages.Demo(), ev)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.NPCIDs) < 3 || len(p.Actions) < 5 || len(p.Stories) != 1 || len(p.LocationIDs) != 3 {
		t.Fatalf("unexpected content counts: npcs=%d actions=%d stories=%d locations=%d",
			len(p.NPCIDs), len(p.Actions), len(p.Stories), len(p.LocationIDs))
	}
	if p.Player.Gold != 15 || p.Player.Skills["persuasion"] != 3 {
		t.Fatalf("player template: %+v", p.Player)
	}
	if p.EntityName("demo:character/borin") != "伯林" {
		t.Fatal("entity name")
	}
	if _, err := loader.Load(os.DirFS("../../../packages/demo"), ev); err != nil {
		t.Fatalf("dir fs: %v", err)
	}
}
