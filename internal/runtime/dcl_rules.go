package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"corerp/internal/core"
	"corerp/internal/events"

	"gopkg.in/yaml.v3"
)

type dclRuleFile struct {
	Rules []dclRule `yaml:"rules"`
	Hooks []dclHook `yaml:"hooks"`
}

type dclHook struct {
	ID          string                 `yaml:"id"`
	Trigger     string                 `yaml:"trigger"`
	Description string                 `yaml:"description"`
	Effect      map[string]interface{} `yaml:"effect"`
}

type dclRule struct {
	ID          string                   `yaml:"id"`
	Description string                   `yaml:"description"`
	When        dclRuleCondition         `yaml:"when"`
	Do          []map[string]interface{} `yaml:"do"`
}

type dclRuleCondition struct {
	EventType string `yaml:"event_type"`
	Trigger   string `yaml:"trigger"`
}

type dclRuleResult struct {
	RuleID  string   `json:"rule_id"`
	ModID   string   `json:"mod_id"`
	Actions []string `json:"actions"`
}

func (e *Engine) applyDCLRulesForEvent(trigger core.Event) []dclRuleResult {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.applyDCLRulesForEventLocked(trigger)
}

func (e *Engine) applyDCLRulesForEventLocked(trigger core.Event) []dclRuleResult {
	if strings.TrimSpace(trigger.Type) == "" {
		return nil
	}
	rules := e.loadDCLRulesLocked()
	var results []dclRuleResult
	for _, rule := range rules {
		if !rule.matches(trigger) {
			continue
		}
		result := dclRuleResult{RuleID: rule.Rule.ID, ModID: rule.ModID}
		for _, action := range rule.Rule.Do {
			name, payload := singleAction(action)
			if name == "" {
				continue
			}
			if err := e.applyDCLActionLocked(rule, name, payload, trigger); err != nil {
				result.Actions = append(result.Actions, fmt.Sprintf("%s:error:%v", name, err))
				continue
			}
			result.Actions = append(result.Actions, name)
		}
		if len(result.Actions) > 0 {
			e.emitDCLRuleAppliedLocked(rule, trigger, result.Actions)
			results = append(results, result)
		}
	}
	return results
}

type installedDCLRule struct {
	ModID string
	Rule  dclRule
}

func (r installedDCLRule) matches(evt core.Event) bool {
	typ := strings.TrimSpace(r.Rule.When.EventType)
	if typ == "" {
		typ = strings.TrimSpace(r.Rule.When.Trigger)
	}
	return typ != "" && typ == evt.Type
}

func (e *Engine) loadDCLRulesLocked() []installedDCLRule {
	worldPath := strings.TrimSpace(e.currentWorldPathLocked())
	if worldPath == "" {
		return nil
	}
	matches, err := filepath.Glob(filepath.Join(worldPath, "mods", "*", "hooks.yml"))
	if err != nil || len(matches) == 0 {
		return nil
	}
	sort.Strings(matches)
	var out []installedDCLRule
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var doc dclRuleFile
		if err := yaml.Unmarshal(data, &doc); err != nil {
			continue
		}
		modID := filepath.Base(filepath.Dir(path))
		for _, rule := range doc.Rules {
			rule.ID = strings.TrimSpace(rule.ID)
			if rule.ID == "" {
				continue
			}
			out = append(out, installedDCLRule{ModID: modID, Rule: rule})
		}
		for _, hook := range doc.Hooks {
			rule := legacyHookToRule(hook)
			if rule.ID == "" {
				continue
			}
			out = append(out, installedDCLRule{ModID: modID, Rule: rule})
		}
	}
	return out
}

func legacyHookToRule(hook dclHook) dclRule {
	hook.ID = strings.TrimSpace(hook.ID)
	if hook.ID == "" || strings.TrimSpace(hook.Trigger) == "" {
		return dclRule{}
	}
	var actions []map[string]interface{}
	keys := make([]string, 0, len(hook.Effect))
	for key := range hook.Effect {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		actions = append(actions, map[string]interface{}{key: hook.Effect[key]})
	}
	return dclRule{
		ID:          hook.ID,
		Description: hook.Description,
		When:        dclRuleCondition{EventType: strings.TrimSpace(hook.Trigger)},
		Do:          actions,
	}
}

func singleAction(raw map[string]interface{}) (string, interface{}) {
	if len(raw) != 1 {
		return "", nil
	}
	for key, value := range raw {
		return strings.TrimSpace(key), value
	}
	return "", nil
}

func (e *Engine) applyDCLActionLocked(rule installedDCLRule, name string, payload interface{}, trigger core.Event) error {
	switch name {
	case "restore_checkpoint":
		return e.restoreDCLCheckpointLocked(payload, rule, trigger)
	case "increment_variable":
		key, by, err := parseVariableIncrement(payload)
		if err != nil {
			return err
		}
		state := e.stateMgr.Get()
		if state.Variables == nil {
			state.Variables = map[string]interface{}{}
		}
		state.Variables[key] = numberValue(state.Variables[key]) + by
		e.stateMgr.Set(state)
		return e.submitDCLEventLocked("variable_set", rule, trigger, map[string]interface{}{"key": key, "value": state.Variables[key]})
	case "set_variable":
		key, value, err := parseVariableSet(payload)
		if err != nil {
			return err
		}
		state := e.stateMgr.Get()
		if state.Variables == nil {
			state.Variables = map[string]interface{}{}
		}
		state.Variables[key] = value
		e.stateMgr.Set(state)
		return e.submitDCLEventLocked("variable_set", rule, trigger, map[string]interface{}{"key": key, "value": value})
	case "add_pressure":
		id, by, err := parsePressureDelta(payload)
		if err != nil {
			return err
		}
		state := e.stateMgr.Get()
		if state.Variables == nil {
			state.Variables = map[string]interface{}{}
		}
		state.Variables["world.pressure."+id+".dcl_delta"] = numberValue(state.Variables["world.pressure."+id+".dcl_delta"]) + by
		state.Tension += by
		e.stateMgr.Set(state)
		if err := e.submitDCLEventLocked("world_pressure", rule, trigger, map[string]interface{}{"pressure_id": id, "delta": by, "reason": "dcl_rule:" + rule.Rule.ID}); err != nil {
			return err
		}
		return e.submitDCLEventLocked("tension_change", rule, trigger, map[string]interface{}{"delta": by, "reason": "dcl_rule:" + rule.Rule.ID})
	case "emit_event":
		typ, payload, err := parseEmitEvent(payload)
		if err != nil {
			return err
		}
		return e.submitDCLEventLocked(typ, rule, trigger, payload)
	case "add_memory_flag":
		target, key, err := parseMemoryFlag(payload)
		if err != nil {
			return err
		}
		if target == "focus" {
			target = e.GetFocusCharacter()
		}
		flagKey := "memory_flag." + target + "." + key
		state := e.stateMgr.Get()
		if state.Flags == nil {
			state.Flags = map[string]bool{}
		}
		state.Flags[flagKey] = true
		e.stateMgr.Set(state)
		return e.submitDCLEventLocked("flag_set", rule, trigger, map[string]interface{}{"key": flagKey})
	default:
		return fmt.Errorf("unsupported dcl action %q", name)
	}
}

func (e *Engine) restoreDCLCheckpointLocked(payload interface{}, rule installedDCLRule, trigger core.Event) error {
	name := strings.TrimSpace(fmt.Sprintf("%v", payload))
	if m, ok := payload.(map[string]interface{}); ok {
		name = strings.TrimSpace(fmt.Sprintf("%v", m["name"]))
	}
	slot, err := e.resolveDCLCheckpointLocked(name)
	if err != nil {
		return err
	}
	state := slot.WorldState
	if strings.TrimSpace(slot.EventID) != "" {
		replayed, err := e.gatekeeper.Replay().ReplayTo(slot.EventID, slot.Branch)
		if err != nil {
			return err
		}
		state = replayed
	}
	focusCharacter := strings.TrimSpace(slot.FocusCharacter)
	if err := e.switchCharacterLocked(focusCharacter, false); err != nil {
		return err
	}
	e.playerRole = normalizePlayerRole(slot.PlayerRole)
	state.Scene = normalizeSceneForCharacter(state.Scene, e.GetFocusCharacter(), e.playerRoleNameLocked())
	e.stateMgr.Set(state)
	return e.submitDCLEventLocked("checkpoint_restore", rule, trigger, map[string]interface{}{
		"checkpoint":  slot.Name,
		"branch":      slot.Branch,
		"world_state": state,
	})
}

func (e *Engine) resolveDCLCheckpointLocked(name string) (core.SaveSlot, error) {
	slots, err := e.readSaveSlots()
	if err != nil {
		return core.SaveSlot{}, err
	}
	if len(slots) == 0 {
		return core.SaveSlot{}, fmt.Errorf("no checkpoints available")
	}
	if name != "" && name != "last_safe_checkpoint" {
		for _, slot := range slots {
			if slot.Name == name {
				return normalizeSaveSlotCompatibility(slot), nil
			}
		}
		return core.SaveSlot{}, fmt.Errorf("checkpoint %q not found", name)
	}
	sort.Slice(slots, func(i, j int) bool {
		return slots[i].CreatedAt.After(slots[j].CreatedAt)
	})
	return normalizeSaveSlotCompatibility(slots[0]), nil
}

func (e *Engine) submitDCLEventLocked(typ string, rule installedDCLRule, trigger core.Event, payload map[string]interface{}) error {
	if payload == nil {
		payload = map[string]interface{}{}
	}
	payload["dcl_mod"] = rule.ModID
	payload["dcl_rule"] = rule.Rule.ID
	payload["trigger_event"] = trigger.ID
	evt := events.BuildEvent(typ, "dcl:"+rule.ModID, trigger.Actor, payload)
	evt.SessionID = e.sessionID
	evt.SceneID = e.stateMgr.Get().Scene.Location
	evt.Tag = core.TagSystem
	return e.gatekeeper.Submit(evt, events.SourceSystem())
}

func (e *Engine) emitDCLRuleAppliedLocked(rule installedDCLRule, trigger core.Event, actions []string) {
	_ = e.submitDCLEventLocked("dcl_rule_applied", rule, trigger, map[string]interface{}{
		"actions": append([]string(nil), actions...),
	})
}

func parseVariableIncrement(payload interface{}) (string, float64, error) {
	m, ok := payload.(map[string]interface{})
	if !ok {
		return "", 0, fmt.Errorf("increment_variable expects object")
	}
	if len(m) == 1 {
		for key, value := range m {
			return strings.TrimSpace(key), numberValue(value), nil
		}
	}
	key := strings.TrimSpace(fmt.Sprintf("%v", m["key"]))
	if key == "" {
		return "", 0, fmt.Errorf("increment_variable key is required")
	}
	by := numberValue(m["by"])
	if by == 0 {
		by = 1
	}
	return key, by, nil
}

func parseVariableSet(payload interface{}) (string, interface{}, error) {
	m, ok := payload.(map[string]interface{})
	if !ok {
		return "", nil, fmt.Errorf("set_variable expects object")
	}
	key := strings.TrimSpace(fmt.Sprintf("%v", m["key"]))
	if key == "" {
		return "", nil, fmt.Errorf("set_variable key is required")
	}
	return key, m["value"], nil
}

func parsePressureDelta(payload interface{}) (string, float64, error) {
	m, ok := payload.(map[string]interface{})
	if !ok {
		return "", 0, fmt.Errorf("add_pressure expects object")
	}
	id := strings.TrimSpace(fmt.Sprintf("%v", m["id"]))
	if id == "" {
		return "", 0, fmt.Errorf("add_pressure id is required")
	}
	by := numberValue(m["by"])
	if by == 0 {
		by = numberValue(m["intensity"])
	}
	return id, by, nil
}

func parseEmitEvent(payload interface{}) (string, map[string]interface{}, error) {
	m, ok := payload.(map[string]interface{})
	if !ok {
		return "", nil, fmt.Errorf("emit_event expects object")
	}
	typ := strings.TrimSpace(fmt.Sprintf("%v", m["type"]))
	if typ == "" {
		return "", nil, fmt.Errorf("emit_event type is required")
	}
	out := map[string]interface{}{}
	if raw, ok := m["payload"].(map[string]interface{}); ok {
		for key, value := range raw {
			out[key] = value
		}
	}
	return typ, out, nil
}

func parseMemoryFlag(payload interface{}) (string, string, error) {
	m, ok := payload.(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("add_memory_flag expects object")
	}
	target := strings.TrimSpace(fmt.Sprintf("%v", m["target"]))
	key := strings.TrimSpace(fmt.Sprintf("%v", m["key"]))
	if target == "" {
		target = "focus"
	}
	if key == "" {
		return "", "", fmt.Errorf("add_memory_flag key is required")
	}
	return target, key, nil
}

func numberValue(value interface{}) float64 {
	switch v := value.(type) {
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case float64:
		return v
	case float32:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	case string:
		var f float64
		fmt.Sscanf(v, "%f", &f)
		return f
	default:
		return 0
	}
}
