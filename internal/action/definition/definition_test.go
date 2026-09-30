package definition

import (
	"path/filepath"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/package/manifest"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
)

// 加载 demo 包全部 Action，校验 ID 命名空间并用 CEL 编译 requirements 与难度表达式。
func TestDemoActionsCompile(t *testing.T) {
	dir := "../../../packages/demo"
	m, err := manifest.Load(filepath.Join(dir, manifest.FileName))
	if err != nil {
		t.Fatal(err)
	}
	ev, err := expression.New()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Content["actions"]) == 0 {
		t.Fatal("demo package has no actions")
	}
	for _, f := range m.Content["actions"] {
		d, err := Load(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		if !manifest.ValidID(d.ID) || manifest.NamespaceOf(d.ID) != m.Namespace {
			t.Errorf("%s: bad id %q", f, d.ID)
		}
		for _, r := range d.Requirements {
			if _, err := ev.Compile(r.Expr); err != nil {
				t.Errorf("%s: %v", d.ID, err)
			}
		}
		for _, c := range d.Checks {
			if _, err := ev.Compile(c.Difficulty); err != nil {
				t.Errorf("%s: %v", d.ID, err)
			}
		}
		if _, ok := d.Outcomes["success"]; !ok {
			t.Errorf("%s: missing success outcome", d.ID)
		}
	}
}

func TestIntimidateRequirements(t *testing.T) {
	d, err := Load("../../../packages/demo/actions/intimidate.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ev, _ := expression.New()
	vars := expression.Vars{
		"actor":  map[string]any{"can_speak": true},
		"target": map[string]any{"type": "character", "resolve": 11},
	}
	for _, r := range d.Requirements {
		ok, err := ev.EvalBool(r.Expr, vars)
		if err != nil || !ok {
			t.Fatalf("requirement %q: ok=%v err=%v", r, ok, err)
		}
	}
	diff, err := ev.Eval(d.Checks[0].Difficulty, vars)
	if err != nil || diff != int64(15) {
		t.Fatalf("difficulty = %v, %v", diff, err)
	}
}
