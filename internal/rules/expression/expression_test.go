package expression

import "testing"

func TestEvalBool(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatal(err)
	}
	vars := Vars{
		"actor":  map[string]any{"can_speak": true, "skills": map[string]any{"intimidation": 6}},
		"target": map[string]any{"type": "character", "resolve": 8},
	}
	cases := []struct {
		expr string
		want bool
	}{
		{`actor.can_speak == true`, true},
		{`target.type == "character"`, true},
		{`target.type == "item"`, false},
		{`actor.skills.intimidation + 3 > target.resolve`, true},
		{`actor.can_speak && target.resolve < 5`, false},
	}
	for _, c := range cases {
		got, err := e.EvalBool(c.expr, vars)
		if err != nil {
			t.Fatalf("%s: %v", c.expr, err)
		}
		if got != c.want {
			t.Errorf("%s: got %v want %v", c.expr, got, c.want)
		}
	}
}

func TestEvalNumeric(t *testing.T) {
	e, _ := New()
	v, err := e.Eval(`target.resolve * 2`, Vars{"target": map[string]any{"resolve": 8}})
	if err != nil {
		t.Fatal(err)
	}
	if v != int64(16) {
		t.Fatalf("got %v (%T)", v, v)
	}
}

func TestErrors(t *testing.T) {
	e, _ := New()
	if _, err := e.Compile(`actor.can_speak ==`); err == nil {
		t.Error("expected syntax error")
	}
	if _, err := e.Compile(`unknown_var == 1`); err == nil {
		t.Error("expected undeclared reference error")
	}
	if _, err := e.EvalBool(`1 + 1`, nil); err == nil {
		t.Error("expected non-bool error")
	}
	if _, err := e.EvalBool(`actor.missing == true`, nil); err == nil {
		t.Error("expected missing key error")
	}
}
