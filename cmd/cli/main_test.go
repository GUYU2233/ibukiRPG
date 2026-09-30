package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestREPL(t *testing.T) {
	var out bytes.Buffer
	if err := repl(strings.NewReader("/help\n我踢翻桌子\n/quit\n"), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"/help", "[stub]", "再见"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q:\n%s", want, s)
		}
	}
}
