package orchestrator

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/ai/router"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/internal/world/overlay"
	"github.com/GUYU2233/ibukiRPG/packages"
)

func TestWorldSectionsHiddenLogScope(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, Options{DBPath: filepath.Join(t.TempDir(), "world-sections.db"), Package: packages.Demo()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	slot, err := s.NewGame(ctx, "", "测试", 7)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.loadGame(ctx, slot, false); err != nil {
		t.Fatal(err)
	}
	_, st, g, err := s.current()
	if err != nil {
		t.Fatal(err)
	}

	// Both entries target the current location, so relevance cannot hide a leak.
	hidden := overlay.LogEntry{Change: change.Change{
		ID: "scope-hidden-log", Target: st.Player.Location, Hidden: true,
		Summary: "SCOPE_HIDDEN_SUMMARY_MARKER", Reason: "SCOPE_HIDDEN_REASON_MARKER",
	}, Turn: st.Turn}
	public := overlay.LogEntry{Change: change.Change{
		ID: "scope-public-log", Target: st.Player.Location,
		Summary: "SCOPE_PUBLIC_SUMMARY_MARKER", Reason: "SCOPE_PUBLIC_REASON_MARKER",
	}, Turn: st.Turn}

	for _, tc := range []struct {
		name        string
		logs        []overlay.LogEntry
		wantChanges bool
	}{
		{name: "public_and_hidden", logs: []overlay.LogEntry{public, hidden}, wantChanges: true},
		{name: "hidden_only", logs: []overlay.LogEntry{hidden}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := st.Clone()
			st.World = overlay.New()
			st.World.Log = tc.logs
			got := s.worldSections(g, st, "", router.TierFull)
			for _, marker := range []string{hidden.Summary, hidden.Reason} {
				if strings.Contains(got, marker) {
					t.Errorf("hidden log marker %q leaked into worldSections:\n%s", marker, got)
				}
			}
			if strings.Contains(got, "[WORLD_CHANGES]") != tc.wantChanges {
				t.Errorf("WORLD_CHANGES presence: want %t, got:\n%s", tc.wantChanges, got)
			}
			if tc.wantChanges {
				for _, marker := range []string{public.Summary, public.Reason} {
					if !strings.Contains(got, marker) {
						t.Errorf("public log marker %q missing from worldSections:\n%s", marker, got)
					}
				}
			}
		})
	}
}
