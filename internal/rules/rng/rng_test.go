package rng

import "testing"

func draw(s *Stream, n int) []uint64 {
	out := make([]uint64, n)
	for i := range out {
		out[i] = s.Uint64()
	}
	return out
}

func equal(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDeterministic(t *testing.T) {
	k := Key{"combat", "battle.001", "hit"}
	a := draw(New(42, k), 100)
	b := draw(New(42, k), 100)
	if !equal(a, b) {
		t.Fatal("same seed and key must produce identical sequences")
	}
}

func TestStreamsDiffer(t *testing.T) {
	base := draw(New(42, Key{"combat", "battle.001", "hit"}), 16)
	for _, k := range []Key{
		{"combat", "battle.001", "damage"},
		{"combat", "battle.002", "hit"},
		{"world", "battle.001", "hit"},
		{"combatb", "attle.001", "hit"}, // 长度前缀防碰撞
	} {
		if equal(base, draw(New(42, k), 16)) {
			t.Fatalf("stream %+v should differ from base", k)
		}
	}
	if equal(base, draw(New(43, Key{"combat", "battle.001", "hit"}), 16)) {
		t.Fatal("different world seed should differ")
	}
}

// 消耗一条流不影响另一条流：模拟“安装无关 Mod 不导致漂移”。
func TestIndependence(t *testing.T) {
	hitKey := Key{"combat", "battle.001", "hit"}
	want := draw(New(7, hitKey), 20)

	hit := New(7, hitKey)
	modStream := New(7, Key{"mod:magic", "battle.001", "sparkle"})
	got := make([]uint64, 0, 20)
	for i := 0; i < 20; i++ {
		_ = draw(modStream, i%3+1) // 交错消耗无关流
		got = append(got, hit.Uint64())
	}
	if !equal(want, got) {
		t.Fatal("consuming another stream must not affect this stream")
	}
}

func TestStateRoundTrip(t *testing.T) {
	s := New(1, Key{"loot", "chest.9", "drop"})
	_ = draw(s, 5)
	state, err := s.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	want := draw(s, 10)
	r := New(0, Key{"loot", "chest.9", "drop"})
	if err := r.UnmarshalBinary(state); err != nil {
		t.Fatal(err)
	}
	if !equal(want, draw(r, 10)) {
		t.Fatal("restored stream should continue identically")
	}
}

func TestRollRange(t *testing.T) {
	s := New(9, Key{"combat", "b", "d20"})
	for i := 0; i < 1000; i++ {
		if v := s.Roll(20); v < 1 || v > 20 {
			t.Fatalf("roll out of range: %d", v)
		}
	}
}

func TestCountersRepeatedDrawsDiffer(t *testing.T) {
	k := Key{"check", "player", "persuasion"}
	c := Counters{}
	seen := map[uint64]bool{}
	for i := 0; i < 20; i++ {
		s, n := c.Next(7, k)
		if n != uint64(i) {
			t.Fatalf("counter = %d, want %d", n, i)
		}
		seen[s.Uint64()] = true
	}
	if len(seen) != 20 {
		t.Fatalf("repeated draws should differ, got %d distinct", len(seen))
	}
	// 计数器可复现：同一 (seed, key, n) 总是相同结果。
	a, b := NewAt(7, k, 3), NewAt(7, k, 3)
	if a.Roll(20) != b.Roll(20) {
		t.Fatal("NewAt must be deterministic")
	}
	// 另一条流的计数器独立。
	if _, n := c.Next(7, Key{"check", "player", "intimidation"}); n != 0 {
		t.Fatalf("independent stream counter = %d", n)
	}
	// d20 分布大致覆盖 1..20。
	faces := map[int]bool{}
	for i := uint64(0); i < 400; i++ {
		faces[NewAt(1, k, i).Roll(20)] = true
	}
	if len(faces) != 20 {
		t.Fatalf("d20 faces covered = %d", len(faces))
	}
}
