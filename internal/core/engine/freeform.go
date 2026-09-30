package engine

import (
	"fmt"

	"github.com/GUYU2233/ibukiRPG/internal/action/freeform"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

// execFreeform 执行 FreeformAction：只能产生白名单轻量事件（第 6.1 节）。
func (w *work) execFreeform() error {
	if w.cmd.Freeform == nil {
		return reject("你想做什么？")
	}
	ff := freeform.Sanitize(*w.cmd.Freeform, w.pkg(), w.s)
	start := len(w.events)
	if err := w.emit(event.TimeAdvanced, event.Data{Minutes: int64(ff.EstimatedMinutes), Reason: "freeform"}); err != nil {
		return err
	}
	target := ""
	if len(ff.Targets) > 0 {
		target = ff.Targets[0]
	}
	success := true
	hasCheck := ff.Check != nil
	if hasCheck {
		r, err := w.rollCheck(ff.Check.Skill, ff.Check.Difficulty, target)
		if err != nil {
			return err
		}
		success = r.Success
	}
	outcome := "success"
	if !success {
		outcome = "failure"
	}
	summary := fmt.Sprintf("%s%s", w.s.Player.Name, ff.Description)
	if err := w.emit(event.FreeformPerformed, event.Data{
		Actor: loader.PlayerID, Target: target, Text: ff.Description, Tags: ff.Tags,
		Outcome: outcome, Success: success, Location: w.s.Player.Location, Reason: ff.Reasonability,
	}); err != nil {
		return err
	}
	mechanical := false
	for _, e := range ff.Effects {
		if !freeform.Applies(e, success) {
			continue
		}
		var err error
		switch e.Type {
		case freeform.EffectNoise:
			err = w.emit(event.NoiseGenerated, event.Data{Location: w.s.Player.Location, Delta: e.Value})
			mechanical = true
		case freeform.EffectAttention:
			err = w.emit(event.AttentionChanged, event.Data{Target: e.Target, Text: e.Text, Location: w.s.Player.Location})
		case freeform.EffectSceneFact:
			if !containsStr(w.s.SceneFacts[w.s.Player.Location], e.Text) {
				err = w.emit(event.SceneFactChanged, event.Data{Location: w.s.Player.Location, Text: e.Text})
				mechanical = true
			}
		case freeform.EffectRelationship:
			err = w.emit(event.RelationshipNudge, event.Data{Target: e.Target, Values: map[string]int{"trust": e.Value}})
			mechanical = true
		case freeform.EffectCondition:
			if !w.s.HasCondition(e.Text) {
				err = w.emit(event.MinorConditionApplied, event.Data{Condition: e.Text})
				mechanical = true
			}
		}
		if err != nil {
			return err
		}
	}
	if !mechanical && !hasCheck {
		if err := w.emit(event.NoMechanicalEffect, event.Data{Text: ff.Description}); err != nil {
			return err
		}
	}
	notable := hasCheck || mechanical
	if err := w.observe(summary, notable, event.FreeformPerformed, "player:freeform:"+ff.Description, target); err != nil {
		return err
	}
	// 防御性断言：Freeform 只能产生白名单事件。
	for _, e := range w.events[start:] {
		if !event.FreeformWhitelist[e.Type] {
			return fmt.Errorf("freeform produced non-whitelisted event %s", e.Type)
		}
	}
	w.quiet = !mechanical && !hasCheck
	w.actx = map[string]any{"kind": "freeform", "id": "freeform", "target": target, "item": "", "success": success, "outcome": outcome}
	return nil
}
