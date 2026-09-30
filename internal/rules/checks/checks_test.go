package checks

import "testing"

func TestResolve(t *testing.T) {
	cases := []struct {
		roll, mod, dc int
		success       bool
	}{
		{10, 4, 14, true},
		{10, 3, 14, false},
		{20, -5, 25, true}, // 天然 20
		{1, 30, 5, false},  // 天然 1
	}
	for _, c := range cases {
		if got := Resolve("persuasion", "说服", c.roll, c.mod, c.dc); got.Success != c.success {
			t.Errorf("Resolve(%d,%d,%d) = %v", c.roll, c.mod, c.dc, got.Success)
		}
	}
	if Modifier(3, 12, 0) != 4 || Modifier(2, 9, 0) != 1 || Modifier(0, 7, -1) != -3 {
		t.Fatal("modifier")
	}
	if ClampDC(99) != MaxDC || ClampDC(-3) != MinDC {
		t.Fatal("clamp")
	}
	if Chance(0, 11) != 50 || Chance(100, 5) != 95 {
		t.Fatalf("chance: %d %d", Chance(0, 11), Chance(100, 5))
	}
	if Explain(Resolve("persuasion", "说服", 14, 4, 15)) != "掷出 14，说服修正 +4，合计 18 ≥ 难度 15 → 成功" {
		t.Fatal(Explain(Resolve("persuasion", "说服", 14, 4, 15)))
	}
}
