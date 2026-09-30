package witness

import (
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

// Visibility 描述一个客观事件的可见范围（第 15 节）。Phase 0 只支持 local：同一地点可见。
type Visibility struct {
	Mode     string // local / hidden
	Location string
}

// Witnesses 返回能观察到该事件的 NPC（按内容包顺序，确定性）。
//
// Phase 0 的感知规则：同一地点的 NPC 都能看见；hidden 事件无人看见。
// 以后可以在此加入距离、视线、潜行、注意力等因素。
func Witnesses(s *state.State, p *loader.Package, v Visibility) []string {
	if v.Mode == "hidden" {
		return nil
	}
	return s.NPCsAt(p, v.Location)
}

// Confidence 返回目击者对所见之事的置信度（千分比）：当事人最确定，旁观者稍低。
func Confidence(witness, target string) int {
	if witness == target {
		return 1000
	}
	return 900
}
