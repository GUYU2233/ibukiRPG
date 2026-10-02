package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
)

// slotFlags 是存档的附加标记（saves.flags，JSON）。
type slotFlags struct {
	// ExtAck 是玩家确认过的世界变更日志长度：之后的外部工具变更需要提示。
	ExtAck int `json:"ext_ack"`
}

func parseFlags(raw string) slotFlags {
	var f slotFlags
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &f)
	}
	return f
}

// ExternalNotice 返回“外部工具修改了 N 项设定”的提示；没有未确认的外部修改时返回 nil。
func (s *Session) ExternalNotice(ctx context.Context) (*dto.ExternalNoticeV1, error) {
	slot, st, _, err := s.current()
	if err != nil {
		return nil, err
	}
	if st.World == nil {
		return nil, nil
	}
	sl, err := s.store.GetSlot(ctx, slot)
	if err != nil {
		return nil, err
	}
	ack := min(max(parseFlags(sl.Flags).ExtAck, 0), len(st.World.Log))
	n := &dto.ExternalNoticeV1{Source: change.SourceMCP}
	for _, e := range st.World.Log[ack:] {
		if strings.HasPrefix(e.Source, change.SourceMCP) && e.Reverts == "" && e.RevertedBy == "" {
			n.IDs = append(n.IDs, e.ID)
		}
	}
	if len(n.IDs) == 0 {
		return nil, nil
	}
	n.Count = len(n.IDs)
	n.Text = fmt.Sprintf("外部工具修改了 %d 项设定", n.Count)
	return n, nil
}

// AckExternalChanges 确认外部修改提示（之后只提示新的外部修改）。
func (s *Session) AckExternalChanges(ctx context.Context) error {
	slot, st, _, err := s.current()
	if err != nil {
		return err
	}
	sl, err := s.store.GetSlot(ctx, slot)
	if err != nil {
		return err
	}
	f := parseFlags(sl.Flags)
	if st.World != nil {
		f.ExtAck = len(st.World.Log)
	}
	b, _ := json.Marshal(f)
	return s.store.SetSlotFlags(ctx, slot, string(b))
}
