package structured

import (
	"errors"
	"strings"
	"testing"
)

const good = `托克把传单抖开。
<<<WORLD>>>
{"v":1,"changes":[{"op":"patch","target":"brass:location/plaza","path":"fields.description","value":"x","reason":"r"}],
 "reveals":[{"entity":"brass:character/tock","fields":["faction"],"channel":"dialogue","source":"brass:character/tock"}],
 "impact":{"level":"minor"},"suggestions":["追问"],"minutes":10}
<<<END>>>`

func TestMergedOutput(t *testing.T) {
	cases := map[string]struct {
		in      string
		changes int
		fails   bool
	}{
		"normal":        {good, 1, false},
		"fenced":        {"正文\n<<<WORLD>>>\n```json\n{\"v\":1,\"changes\":[{\"op\":\"patch\",\"target\":\"a\",\"path\":\"p\",\"value\":1,\"reason\":\"r\"}]}\n```", 1, false},
		"truncated":     {"正文\n<<<WORLD>>>\n{\"v\":1,\"changes\":[{\"op\":\"patch\",\"target\":\"a\",\"path\":\"p\",\"value\":1,\"reason\":\"r\"},{\"op\":\"crea", 1, false},
		"trailingComma": {"正文\n<<<WORLD>>>\n{\"v\":1,\"changes\":[{\"op\":\"patch\",\"target\":\"a\",\"path\":\"p\",\"value\":1,\"reason\":\"r\"},],}", 1, false},
		"singleQuotes":  {"正文\n<<<WORLD>>>\n{'v':1,'changes':[{'op':'patch','target':'a','path':'p','value':'多\n行','reason':'r'}]}", 1, false},
		"extraText":     {"正文\n<<<WORLD>>>\n好的，以下是世界更新：{\"v\":1,\"changes\":[]} 希望有帮助", 0, false},
		"noSeparator":   {"正文正文。{\"v\":1,\"changes\":[{\"op\":\"patch\",\"target\":\"a\",\"path\":\"p\",\"value\":1,\"reason\":\"r\"}]}", 1, false},
		"garbage":       {"正文\n<<<WORLD>>>\n这不是 JSON", 0, true},
	}
	for name, c := range cases {
		narr, raw, _ := Split(c.in)
		if narr == "" || strings.Contains(narr, "{\"v\"") {
			t.Errorf("%s: narration %q", name, narr)
		}
		w, _, err := Parse(raw)
		if c.fails {
			if err == nil {
				t.Errorf("%s: expected failure", name)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v (raw %q → %q)", name, err, raw, Repair(raw))
			continue
		}
		if len(w.AllChanges()) != c.changes {
			t.Errorf("%s: changes %d", name, len(w.AllChanges()))
		}
	}
	if _, _, err := Parse(""); !errors.Is(err, ErrNoWorld) {
		t.Fatal("empty must be ErrNoWorld")
	}
}

func TestRefusal(t *testing.T) {
	if !IsRefusal("", errors.New(`http 400: {"code":"data_inspection_failed"}`)) {
		t.Fatal("provider refusal")
	}
	if !IsRefusal("抱歉，我无法继续这个故事。", nil) {
		t.Fatal("phrase refusal")
	}
	if IsRefusal(good, nil) || IsRefusal("", errors.New("timeout")) {
		t.Fatal("false positive")
	}
}

func TestFilterStopsAtSeparator(t *testing.T) {
	var got strings.Builder
	f := &Filter{Out: func(s string) { got.WriteString(s) }}
	for _, chunk := range []string{"你推开门。", "<<<WO", "RLD>>>{\"v\":1}"} {
		f.Write(chunk)
	}
	f.Flush()
	if got.String() != "你推开门。" {
		t.Fatalf("got %q", got.String())
	}
	got.Reset()
	f = &Filter{Out: func(s string) { got.WriteString(s) }}
	f.Write("a<<")
	f.Write("b")
	f.Flush()
	if got.String() != "a<<b" {
		t.Fatalf("got %q", got.String())
	}
}
