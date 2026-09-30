package engine

import (
	"fmt"

	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/perception/witness"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
	"github.com/GUYU2233/ibukiRPG/internal/rules/rng"
)

// storyVars 在通用变量之上加入 action 与 story。
func (w *work) storyVars(id string) expression.Vars {
	v := w.vars(fmt.Sprint(w.actx["target"]), fmt.Sprint(w.actx["item"]))
	act := map[string]any{"kind": "", "id": "", "target": "", "item": "", "success": false, "outcome": ""}
	for k, x := range w.actx {
		act[k] = x
	}
	v["action"] = act
	st := w.s.Stories[id]
	steps := make([]any, 0, len(st.Steps))
	for _, s := range st.Steps {
		steps = append(steps, s)
	}
	elapsed := int64(0)
	if st.Status == state.StoryActive {
		elapsed = w.s.Minute - st.StartedMinute
	}
	v["story"] = map[string]any{"status": st.Status, "elapsed_minutes": elapsed, "steps": steps}
	return v
}

// runStories 是 Phase 0 的 Story Director：推进已激活故事的步骤 / 结局，并检查新故事的触发条件。
// 故事效果同样走内容包声明的 Effect → Event，Director 不直接写状态。
func (w *work) runStories(preActive map[string]bool) error {
	p := w.pkg()
	for _, id := range p.StoryIDs {
		story := p.Stories[id]
		st := w.s.Stories[id]
		switch {
		case st.Status == state.StoryActive && preActive[id]:
			if err := w.advanceStory(story); err != nil {
				return err
			}
		case st.Status == state.StoryInactive && story.Trigger != "":
			ok, err := w.eng.Eval.EvalBool(story.Trigger, w.storyVars(id))
			if err != nil {
				return fmt.Errorf("%s trigger: %w", id, err)
			}
			if ok {
				if err := w.startStory(story, "trigger"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (w *work) advanceStory(story *loader.Story) error {
	for _, step := range story.Steps {
		if containsStr(w.s.Stories[story.ID].Steps, step.ID) {
			continue // Phase 0：步骤只触发一次
		}
		ok, err := w.eng.Eval.EvalBool(step.When, w.storyVars(story.ID))
		if err != nil {
			return fmt.Errorf("%s/%s: %w", story.ID, step.ID, err)
		}
		if !ok {
			continue
		}
		if err := w.emit(event.StoryStepReached, event.Data{Story: story.ID, Step: step.ID}); err != nil {
			return err
		}
		for _, o := range step.Effects {
			if err := w.applyOutcome(o, "", ""); err != nil {
				return err
			}
		}
		w.storyTouched = true
	}
	for _, out := range story.Outcomes {
		ok, err := w.eng.Eval.EvalBool(out.When, w.storyVars(story.ID))
		if err != nil {
			return fmt.Errorf("%s/%s: %w", story.ID, out.ID, err)
		}
		if !ok {
			continue
		}
		if err := w.emit(event.StoryResolved, event.Data{Story: story.ID, Outcome: out.ID, Title: out.Title}); err != nil {
			return err
		}
		// 结局后撤下开场时写入的短期场景事实（例如“奥托正在大声指责米拉”）。
		for _, o := range story.IntroEffects {
			fact := fmt.Sprint(o.Effect.Values["fact"])
			if o.Effect.Type == "scene_fact" && containsStr(w.s.SceneFacts[w.s.Player.Location], fact) {
				if err := w.emit(event.SceneFactChanged, event.Data{Location: w.s.Player.Location, Text: fact, Remove: true}); err != nil {
					return err
				}
			}
		}
		for _, o := range out.Effects {
			if err := w.applyOutcome(o, "", ""); err != nil {
				return err
			}
		}
		text := fmt.Sprintf("《%s》的结局：%s", story.Title, out.Title)
		if err := w.observe(text, true, event.StoryResolved, "story:"+story.ID, "", ""); err != nil {
			return err
		}
		// 在场者把结局记成情节记忆（亲历者之后的台词可以据此改变）。
		for _, npc := range witness.Witnesses(w.s, w.pkg(), witness.Visibility{Mode: "local", Location: w.s.Player.Location}) {
			if err := w.remember(npc, "story:"+loader.Key(story.ID)+":"+out.ID, text, "witness", 3); err != nil {
				return err
			}
		}
		w.storyTouched = true
		w.tension -= 300
		return nil
	}
	return nil
}

func (w *work) startStory(story *loader.Story, reason string) error {
	if err := w.emit(event.StoryStarted, event.Data{Story: story.ID, Title: story.Title, Reason: reason}); err != nil {
		return err
	}
	for _, o := range story.IntroEffects {
		if err := w.applyOutcome(o, "", ""); err != nil {
			return err
		}
	}
	w.storyTouched = true
	w.tension += 400
	return nil
}

// runPacing 是 Pacing System（第 25 节）：统计平静回合，必要时推一把——
// 优先提前触发允许 pacing 触发的故事，否则挑一条环境事件。只影响事件选择。
func (w *work) runPacing() error {
	p := w.pkg()
	pc := w.s.Pacing
	quiet := pc.QuietTurns
	if w.quiet && !w.storyTouched {
		quiet++
	} else {
		quiet = 0
	}
	tension := pc.Tension - 30 + w.tension
	nudge := false
	need := p.Pacing.QuietTurnsForNudge
	if need > 0 && quiet >= need && (pc.LastNudgeTurn == 0 || w.turn-pc.LastNudgeTurn >= need) {
		for _, id := range p.StoryIDs {
			story := p.Stories[id]
			if w.s.Stories[id].Status != state.StoryInactive || story.PacingTrigger == "" {
				continue
			}
			ok, err := w.eng.Eval.EvalBool(story.PacingTrigger, w.storyVars(id))
			if err != nil {
				return fmt.Errorf("%s pacing trigger: %w", id, err)
			}
			if ok {
				if err := w.startStory(story, "pacing"); err != nil {
					return err
				}
				tension += 400
				nudge = true
				break
			}
		}
		if !nudge {
			var cands []loader.Ambient
			for _, a := range p.Pacing.Ambient {
				if a.Location == w.s.Player.Location && !containsStr(w.s.SceneFacts[a.Location], a.Fact) {
					cands = append(cands, a)
				}
			}
			if len(cands) > 0 {
				key := rng.Key{Namespace: "story", Entity: "pacing", Purpose: "ambient"}
				counter := w.s.RNG[key.String()]
				a := cands[rng.NewAt(w.s.Seed, key, counter).IntN(len(cands))]
				if err := w.emit(event.AmbientEvent, event.Data{Location: a.Location, Text: a.Text, Stream: key.String(), Counter: counter}); err != nil {
					return err
				}
				if a.Fact != "" {
					if err := w.emit(event.SceneFactChanged, event.Data{Location: a.Location, Text: a.Fact}); err != nil {
						return err
					}
				}
				nudge = true
			}
		}
		if nudge {
			quiet = 0
		}
	}
	if tension < 0 {
		tension = 0
	}
	if tension > 1000 {
		tension = 1000
	}
	return w.emit(event.PacingUpdated, event.Data{Tension: tension, QuietTurns: quiet, Nudge: nudge})
}
