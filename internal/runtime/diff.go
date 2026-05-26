package runtime

import (
	"fmt"
	"sort"
	"time"

	"corerp/internal/core"
	"corerp/internal/events"
)

func (e *Engine) CompareBranchesDetailed(branchA, branchB string, index int) (core.WorldStateDiff, error) {
	stateA, err := e.branchState(branchA, index)
	if err != nil {
		return core.WorldStateDiff{}, err
	}
	stateB, err := e.branchState(branchB, index)
	if err != nil {
		return core.WorldStateDiff{}, err
	}
	diff := diffStates(stateA, stateB)
	diff.BranchA = branchA
	diff.BranchB = branchB
	return diff, nil
}

func (e *Engine) CompareSaveSlots(saveA, saveB string) (core.WorldStateDiff, error) {
	e.mu.RLock()
	slots, err := e.readSaveSlots()
	e.mu.RUnlock()
	if err != nil {
		return core.WorldStateDiff{}, err
	}
	var a, b *core.SaveSlot
	for i := range slots {
		if slots[i].Name == saveA {
			a = &slots[i]
		}
		if slots[i].Name == saveB {
			b = &slots[i]
		}
	}
	if a == nil || b == nil {
		return core.WorldStateDiff{}, fmt.Errorf("save diff requires two valid save names")
	}
	diff := diffStates(a.WorldState, b.WorldState)
	diff.SaveA = saveA
	diff.SaveB = saveB
	return diff, nil
}

func (e *Engine) MergeBranchState(sourceBranch, targetBranch string, mergeFlags, mergeVariables bool) (core.BranchMergeResult, error) {
	sourceState, err := e.branchState(sourceBranch, -1)
	if err != nil {
		return core.BranchMergeResult{}, err
	}
	targetState, err := e.branchState(targetBranch, -1)
	if err != nil {
		return core.BranchMergeResult{}, err
	}

	result := core.BranchMergeResult{
		SourceBranch: sourceBranch,
		TargetBranch: targetBranch,
	}

	now := time.Now()
	if mergeFlags {
		keys := sortedBoolKeys(sourceState.Flags, targetState.Flags)
		for _, key := range keys {
			if sourceState.Flags[key] == targetState.Flags[key] {
				continue
			}
			typ := "flag_unset"
			if sourceState.Flags[key] {
				typ = "flag_set"
			}
			evt := events.BuildEvent(typ, "system", "", map[string]interface{}{"key": key})
			evt.Branch = targetBranch
			evt.Canonical = true
			evt.CreatedAt = now
			if err := e.eventStore.Append(evt); err != nil {
				return result, err
			}
			result.FlagsMerged++
			result.EventsAppended++
			now = now.Add(time.Millisecond)
		}
	}
	if mergeVariables {
		keys := sortedAnyKeys(sourceState.Variables, targetState.Variables)
		for _, key := range keys {
			sourceVal, sourceOK := sourceState.Variables[key]
			targetVal, targetOK := targetState.Variables[key]
			if sourceOK == targetOK && fmt.Sprintf("%v", sourceVal) == fmt.Sprintf("%v", targetVal) {
				continue
			}
			if !sourceOK {
				continue
			}
			evt := events.BuildEvent("variable_set", "system", "", map[string]interface{}{
				"key":   key,
				"value": sourceVal,
			})
			evt.Branch = targetBranch
			evt.Canonical = true
			evt.CreatedAt = now
			if err := e.eventStore.Append(evt); err != nil {
				return result, err
			}
			result.VariablesMerged++
			result.EventsAppended++
			now = now.Add(time.Millisecond)
		}
	}
	return result, nil
}

func (e *Engine) branchState(branch string, index int) (core.WorldState, error) {
	if branch == "" {
		branch = "main"
	}
	timeline, err := e.gatekeeper.Replay().GetTimeline(branch, 1000000)
	if err != nil {
		return core.WorldState{}, err
	}
	if len(timeline) == 0 {
		return core.WorldState{
			Relationships: map[string]core.Relationship{},
			Variables:     map[string]interface{}{},
			Flags:         map[string]bool{},
		}, nil
	}
	if index < 0 || index >= len(timeline) {
		index = len(timeline) - 1
	}
	eventID := timeline[index].Event.ID
	return e.gatekeeper.Replay().ReplayTo(eventID, branch)
}

func diffStates(a, b core.WorldState) core.WorldStateDiff {
	diff := core.WorldStateDiff{}
	if a.Scene.Location != b.Scene.Location || a.Scene.TimeOfDay != b.Scene.TimeOfDay || a.Scene.Weather != b.Scene.Weather || a.Scene.Description != b.Scene.Description || fmt.Sprintf("%v", a.Scene.Characters) != fmt.Sprintf("%v", b.Scene.Characters) {
		diff.Scene = map[string]core.StateDiffEntry{}
		if a.Scene.Location != b.Scene.Location {
			diff.Scene["location"] = core.StateDiffEntry{A: a.Scene.Location, B: b.Scene.Location}
		}
		if a.Scene.TimeOfDay != b.Scene.TimeOfDay {
			diff.Scene["time_of_day"] = core.StateDiffEntry{A: a.Scene.TimeOfDay, B: b.Scene.TimeOfDay}
		}
		if a.Scene.Weather != b.Scene.Weather {
			diff.Scene["weather"] = core.StateDiffEntry{A: a.Scene.Weather, B: b.Scene.Weather}
		}
		if a.Scene.Description != b.Scene.Description {
			diff.Scene["description"] = core.StateDiffEntry{A: a.Scene.Description, B: b.Scene.Description}
		}
		if fmt.Sprintf("%v", a.Scene.Characters) != fmt.Sprintf("%v", b.Scene.Characters) {
			diff.Scene["characters"] = core.StateDiffEntry{A: a.Scene.Characters, B: b.Scene.Characters}
		}
	}
	if a.Clock != b.Clock {
		diff.Clock = &core.StateDiffEntry{A: a.Clock, B: b.Clock}
	}
	if a.Tension != b.Tension {
		diff.Tension = &core.StateDiffEntry{A: a.Tension, B: b.Tension}
	}
	diff.Flags = compareBoolMaps(a.Flags, b.Flags)
	diff.Variables = compareAnyMaps(a.Variables, b.Variables)
	diff.Relationships = compareRelationshipMaps(a.Relationships, b.Relationships)
	return diff
}

func compareBoolMaps(a, b map[string]bool) map[string]core.StateDiffEntry {
	keys := sortedBoolKeys(a, b)
	out := map[string]core.StateDiffEntry{}
	for _, key := range keys {
		if a[key] != b[key] {
			out[key] = core.StateDiffEntry{A: a[key], B: b[key]}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func compareAnyMaps(a, b map[string]interface{}) map[string]core.StateDiffEntry {
	keys := sortedAnyKeys(a, b)
	out := map[string]core.StateDiffEntry{}
	for _, key := range keys {
		av, aok := a[key]
		bv, bok := b[key]
		if aok == bok && fmt.Sprintf("%v", av) == fmt.Sprintf("%v", bv) {
			continue
		}
		out[key] = core.StateDiffEntry{A: av, B: bv}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func compareRelationshipMaps(a, b map[string]core.Relationship) map[string]core.StateDiffEntry {
	keys := map[string]bool{}
	for key := range a {
		keys[key] = true
	}
	for key := range b {
		keys[key] = true
	}
	var ordered []string
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	out := map[string]core.StateDiffEntry{}
	for _, key := range ordered {
		if a[key] != b[key] {
			out[key] = core.StateDiffEntry{A: a[key], B: b[key]}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedBoolKeys(a, b map[string]bool) []string {
	keys := map[string]bool{}
	for key := range a {
		keys[key] = true
	}
	for key := range b {
		keys[key] = true
	}
	var ordered []string
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	return ordered
}

func sortedAnyKeys(a, b map[string]interface{}) []string {
	keys := map[string]bool{}
	for key := range a {
		keys[key] = true
	}
	for key := range b {
		keys[key] = true
	}
	var ordered []string
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	return ordered
}
