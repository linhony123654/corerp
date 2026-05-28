package runtime

import (
	"strings"

	"corerp/internal/core"
)

func normalizeMemorySnapshotCompatibility(snapshot core.MemorySnapshot) core.MemorySnapshot {
	snapshot.FocusCharacter = strings.TrimSpace(snapshot.FocusCharacter)
	snapshot.Character = strings.TrimSpace(snapshot.Character)
	if snapshot.FocusCharacter == "" {
		snapshot.FocusCharacter = snapshot.Character
	}
	snapshot.Character = ""
	return snapshot
}

func normalizeSaveSlotCompatibility(slot core.SaveSlot) core.SaveSlot {
	slot.FocusCharacter = strings.TrimSpace(slot.FocusCharacter)
	slot.Character = strings.TrimSpace(slot.Character)
	if slot.FocusCharacter == "" {
		slot.FocusCharacter = slot.Character
	}
	slot.Character = ""
	return slot
}

func normalizeScenarioPresetCompatibility(preset core.ScenarioPreset) core.ScenarioPreset {
	preset.FocusCharacter = strings.TrimSpace(preset.FocusCharacter)
	preset.Character = strings.TrimSpace(preset.Character)
	if preset.FocusCharacter == "" {
		preset.FocusCharacter = preset.Character
	}
	preset.Character = ""
	return preset
}

func normalizeTurnTraceCompatibility(trace core.TurnTrace) core.TurnTrace {
	trace.FocusCharacter = strings.TrimSpace(trace.FocusCharacter)
	trace.Character = strings.TrimSpace(trace.Character)
	if trace.FocusCharacter == "" {
		trace.FocusCharacter = trace.Character
	}
	trace.Character = ""
	for i := range trace.StepTraces {
		trace.StepTraces[i] = normalizeTurnStepTraceCompatibility(trace.StepTraces[i])
	}
	return trace
}

func normalizeTurnStepTraceCompatibility(stepTrace core.TurnStepTrace) core.TurnStepTrace {
	stepTrace.Character = strings.TrimSpace(stepTrace.Character)
	if stepTrace.Step.Speaker == "" {
		stepTrace.Step.Speaker = stepTrace.Character
	}
	stepTrace.Character = ""
	return stepTrace
}

func normalizePendingFactCompatibility(fact core.PendingFact) core.PendingFact {
	fact.FocusCharacter = strings.TrimSpace(fact.FocusCharacter)
	fact.Character = strings.TrimSpace(fact.Character)
	if fact.FocusCharacter == "" {
		fact.FocusCharacter = fact.Character
	}
	fact.Character = ""
	return fact
}

func normalizeExperimentSnapshotCompatibility(snapshot core.ExperimentSnapshot) core.ExperimentSnapshot {
	snapshot.FocusCharacter = strings.TrimSpace(snapshot.FocusCharacter)
	snapshot.Participants = normalizeStringList(snapshot.Participants)
	if len(snapshot.ParticipantDetails) > 0 && len(snapshot.Participants) == 0 {
		snapshot.Participants = participantSummaryNames(snapshot.ParticipantDetails)
	}
	if snapshot.LatestTrace != nil {
		trace := normalizeTurnTraceCompatibility(*snapshot.LatestTrace)
		snapshot.LatestTrace = &trace
		if snapshot.FocusCharacter == "" {
			snapshot.FocusCharacter = trace.FocusCharacter
		}
	}
	return snapshot
}

func normalizeExperimentReportCompatibility(report core.ExperimentReport) core.ExperimentReport {
	report.CurrentCheckpoint = strings.TrimSpace(report.CurrentCheckpoint)
	report.CompareCheckpoint = strings.TrimSpace(report.CompareCheckpoint)
	report.Current = normalizeExperimentSnapshotCompatibility(report.Current)
	if report.Compare != nil {
		compare := normalizeExperimentSnapshotCompatibility(*report.Compare)
		report.Compare = &compare
	}
	return report
}

func normalizeRuntimeInstanceSummaryCompatibility(summary core.RuntimeInstanceSummary) core.RuntimeInstanceSummary {
	summary.FocusCharacter = strings.TrimSpace(summary.FocusCharacter)
	if len(summary.Participants) > 0 {
		summary.Participants = normalizeStringList(summary.Participants)
	}
	if len(summary.ParticipantDetails) > 0 && len(summary.Participants) == 0 {
		summary.Participants = participantSummaryNames(summary.ParticipantDetails)
	}
	return summary
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func participantSummaryNames(items []core.ParticipantSummary) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
