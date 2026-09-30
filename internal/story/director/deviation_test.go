package director

import "testing"

func TestScore(t *testing.T) {
	locs := []string{"a", "b"}
	cases := []struct {
		name string
		in   Input
		want int
	}{
		{"on track", Input{AnchorLocations: locs, PlayerLocation: "a", Budget: 10}, OnTrackDecay},
		{"off track", Input{AnchorLocations: locs, PlayerLocation: "z", Budget: 10}, OffTrackPerTurn},
		{"progress", Input{AnchorLocations: locs, PlayerLocation: "z", Progress: true, TurnsSinceProgress: 40, Budget: 10}, ProgressBonus},
		{"passive", Input{AnchorLocations: locs, PlayerLocation: "z", Passive: true, TurnsSinceProgress: 40, Budget: 10}, 0},
		{"stall", Input{AnchorLocations: locs, PlayerLocation: "a", TurnsSinceProgress: 18, Budget: 10}, OnTrackDecay + StallBase + 2},
		{"stall cap", Input{AnchorLocations: locs, PlayerLocation: "a", TurnsSinceProgress: 200, Budget: 10}, OnTrackDecay + StallMax},
		{"deviant", Input{AnchorLocations: locs, PlayerLocation: "z", Deviant: []Hit{{15, "动粗"}, {6, "逃跑"}}}, OffTrackPerTurn + 21},
		{"passive still counts deviant", Input{Passive: true, Deviant: []Hit{{8, "x"}}}, 8},
	}
	for _, c := range cases {
		if got := Score(c.in); got.Delta != c.want {
			t.Errorf("%s: delta %d want %d (%v)", c.name, got.Delta, c.want, got.Reasons)
		}
	}
	a := Score(cases[6].in)
	b := Score(cases[6].in)
	if a.Delta != b.Delta || len(a.Reasons) != 3 {
		t.Fatalf("score must be deterministic with reasons: %+v", a)
	}
}

func TestLevelAdherence(t *testing.T) {
	if Level(10, 35, 70) != 0 || Level(35, 35, 70) != 1 || Level(90, 35, 70) != 2 {
		t.Fatal("level thresholds")
	}
	if Adherence(-5) != 100 || Adherence(30) != 70 || Adherence(130) != 0 {
		t.Fatal("adherence clamp")
	}
}
