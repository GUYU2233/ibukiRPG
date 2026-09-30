package mobile

import (
	"strings"
	"testing"
)

func TestBridge(t *testing.T) {
	if Version() == "" {
		t.Fatal("empty version")
	}
	if out := Handle(`{"version":"v1","type":"sqlite_smoke"}`); !strings.Contains(out, `"ok":true`) {
		t.Fatalf("unexpected: %s", out)
	}
}
