package orchestrator

import (
	"fmt"
	"slices"

	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
)

// WorldPanel 返回世界面板某一页（8 个页签见 dto.WorldTabs）。所有内容都经过知识层过滤。
func (s *Session) WorldPanel(tab string) (dto.WorldPanelV1, error) {
	_, st, g, err := s.current()
	if err != nil {
		return dto.WorldPanelV1{}, err
	}
	if tab == "" {
		tab = dto.WorldTabs[0]
	}
	if !slices.Contains(dto.WorldTabs, tab) {
		return dto.WorldPanelV1{}, fmt.Errorf("unknown tab %q", tab)
	}
	out := dto.WorldPanelV1{Tab: tab}
	switch tab {
	case "characters":
		out.Entities = g.q.Entities(st, "character")
	case "relations":
		r := g.q.Relations(st)
		out.Relations = &r
	case "codex":
		c := g.q.Codex(st)
		out.Codex = &c
	case "equipment":
		inv := g.q.Inventory(st)
		out.Inventory = &inv
	case "map":
		out.Entities = g.q.Entities(st, "location")
	case "factions":
		out.Entities = g.q.Entities(st, "faction")
	case "timeline":
		out.Events = g.q.TimelineEvents(st)
	case "log":
		out.Changes = g.q.WorldChanges(st, "", "", 100)
	}
	return out, nil
}

// Entity 返回单个实体的玩家视图（未知实体返回 false）。
func (s *Session) Entity(id string) (dto.EntityViewV1, bool, error) {
	_, st, g, err := s.current()
	if err != nil {
		return dto.EntityViewV1{}, false, err
	}
	v, ok := g.q.EntityView(st, id)
	return v, ok, nil
}
