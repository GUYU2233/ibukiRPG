package sections_test

import (
	"strings"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/prompt/sections"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
	"github.com/GUYU2233/ibukiRPG/packages"
)

func TestRenderWithPackPatches(t *testing.T) {
	ev, _ := expression.New()
	var brass *loader.Package
	for _, f := range packages.Builtin() {
		p, err := loader.Load(f, ev)
		if err != nil {
			t.Fatal(err)
		}
		if p.Manifest.ID == "brass_trial" {
			brass = p
		}
	}
	if brass == nil || len(brass.Prompts) == 0 {
		t.Fatal("brass pack should ship prompt patches")
	}
	n := sections.Render(brass, sections.Narrator, "RETRIEVAL_POLICY")
	d := sections.Render(brass, sections.Director, "RETRIEVAL_POLICY")
	if !strings.HasPrefix(n, "[RETRIEVAL_POLICY]\n") || !strings.Contains(n, "先查，再写") || !strings.Contains(n, "mech_get_card") || strings.Contains(n, "pack_list(type=enemy)") {
		t.Fatalf("narrator policy:\n%s", n)
	}
	if !strings.Contains(d, "pack_list(type=enemy)") || strings.Contains(d, "mech_get_card / pack_search 核对") {
		t.Fatalf("director policy:\n%s", d)
	}
	if sections.Render(nil, sections.NPC, "NOPE") != "" {
		t.Fatal("unknown section should render empty")
	}
	brass.Prompts = append(brass.Prompts, loader.PromptPatch{Name: "TOOLS", Mode: "replace", Text: "只许用 pack_search。"})
	if got := sections.Render(brass, sections.NPC, "TOOLS"); got != "[TOOLS]\n只许用 pack_search。\n" {
		t.Fatalf("replace: %q", got)
	}
}
