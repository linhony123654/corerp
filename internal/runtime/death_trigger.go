package runtime

import (
	"strings"
	"time"

	"corerp/internal/core"
	"corerp/internal/events"
)

func (e *Engine) handleCanonicalRuntimeEvent(evt core.Event, frame core.ActionFrame, narrative string) []dclRuleResult {
	results := e.applyDCLRulesForEvent(evt)
	if deathEvt, ok := e.deriveFocusDeathEvent(evt, frame, narrative); ok {
		if err := e.gatekeeper.Submit(deathEvt, events.SourceSystem()); err == nil {
			results = append(results, e.applyDCLRulesForEvent(deathEvt)...)
		}
	}
	return results
}

func (e *Engine) deriveFocusDeathEvent(evt core.Event, frame core.ActionFrame, narrative string) (core.Event, bool) {
	if evt.Type == "focus_death" {
		return core.Event{}, false
	}
	focus := strings.TrimSpace(e.GetFocusCharacter())
	if focus == "" {
		return core.Event{}, false
	}
	if evt.Target != focus && frame.Target != focus && evt.Actor != focus {
		return core.Event{}, false
	}
	if !isFatalFocusSignal(evt, frame, narrative) {
		return core.Event{}, false
	}
	death := events.BuildEvent("focus_death", focus, "", map[string]interface{}{
		"source_event": evt.ID,
		"source_type":  evt.Type,
		"actor":        evt.Actor,
		"target":       evt.Target,
		"reason":       "fatal_focus_signal",
	})
	death.SessionID = evt.SessionID
	death.SceneID = evt.SceneID
	death.Branch = evt.Branch
	death.Tag = core.TagSystem
	death.CreatedAt = time.Now().UTC()
	return death, true
}

func isFatalFocusSignal(evt core.Event, frame core.ActionFrame, narrative string) bool {
	switch evt.Type {
	case "death", "character_death", "fatal_wound":
		return true
	}
	for _, key := range []string{"fatal", "dead", "death", "killed"} {
		if boolPayload(evt.Payload, key) {
			return true
		}
	}
	if evt.Type == "attack" && numberValue(evt.Payload["intensity"]) >= 10 {
		return true
	}
	if frame.Action == "attack" && frame.Intensity >= 10 {
		return true
	}
	text := strings.ToLower(narrative)
	for _, phrase := range []string{"死亡", "死了", "身亡", "毙命", "断气", "dead", "dies", "died", "killed"} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func boolPayload(payload map[string]interface{}, key string) bool {
	value, ok := payload[key]
	if !ok {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		v = strings.ToLower(strings.TrimSpace(v))
		return v == "true" || v == "1" || v == "yes"
	default:
		return false
	}
}
