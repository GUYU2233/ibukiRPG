package worldtime

import "testing"

func TestFormat(t *testing.T) {
	if got := Format(19*60 + 5); got != "第1天 19:05 · 夜晚" {
		t.Fatal(got)
	}
	if got := Format(MinutesPerDay + 7*60); got != "第2天 07:00 · 清晨" {
		t.Fatal(got)
	}
	for in, want := range map[string]int64{"30s": 1, "5m": 5, "2h": 120, "15": 15, "": 0, "90s": 2} {
		got, err := ParseDuration(in)
		if err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %d, %v", in, got, err)
		}
	}
	if _, err := ParseDuration("abc"); err == nil {
		t.Fatal("want error")
	}
}
