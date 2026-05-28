package runtime

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"corerp/internal/core"
	"corerp/internal/world"
)

type directorCandidateScore struct {
	name          string
	score         float64
	reason        string
	breakdown     map[string]float64
	kind          string
	source        string
	loaded        bool
	switchable    bool
	mentioned     bool
	mentionIndex  int
	present       bool
	silenceBoost  float64
	locationMatch bool
	factionMatch  bool
	pressureMatch bool
	hookMatch     bool
}

func (e *Engine) GetDirectorConfig() core.DirectorConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return normalizeDirectorConfig(e.directorCfg)
}

func (e *Engine) UpdateDirectorConfig(cfg core.DirectorConfig) core.DirectorConfig {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.directorCfg = normalizeDirectorConfig(cfg)
	if path := e.currentWorldPathLocked(); path != "" {
		if saved, err := world.SaveDirectorConfig(path, e.directorCfg); err == nil {
			e.directorCfg = normalizeDirectorConfig(saved)
		}
	}
	return e.directorCfg
}

func (e *Engine) GetDirectorPlan() core.DirectorPlan {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastPlan
}

func normalizeDirectorConfig(cfg core.DirectorConfig) core.DirectorConfig {
	switch strings.TrimSpace(cfg.Mode) {
	case "auto_single", "auto_chain":
	default:
		cfg.Mode = "manual"
	}
	if cfg.MaxSpeakers <= 0 {
		cfg.MaxSpeakers = 1
	}
	if cfg.Mode == "auto_single" {
		cfg.MaxSpeakers = 1
	}
	if cfg.Mode == "manual" {
		cfg.MaxSpeakers = 1
	}
	if cfg.MaxSpeakers > 3 {
		cfg.MaxSpeakers = 3
	}
	cfg.Weights = normalizeDirectorWeights(cfg.Weights)
	return cfg
}

func normalizeDirectorWeights(weights map[string]float64) map[string]float64 {
	out := map[string]float64{
		"mentioned":         100,
		"mention_order":     12,
		"continuity":        4,
		"present":           8,
		"location_match":    9,
		"faction_match":     6,
		"pressure_match":    7,
		"hook_match":        12,
		"silence_cap":       12,
		"silence_divisor":   5,
		"trust":             0.15,
		"intimacy":          0.1,
		"fear":              0.05,
		"opened_by_user":    6,
		"tension_switch":    3,
		"kind_persona":      3,
		"kind_npc":          1,
		"source_promoted":   4,
		"source_definition": 2,
		"source_background": 0,
		"loaded":            2,
		"faction_rivalry":   -2,
	}
	for key, value := range weights {
		if value == 0 {
			continue
		}
		out[key] = value
	}
	return out
}

func (e *Engine) directTurnLocked(userInput string, worldState core.WorldState) core.DirectorPlan {
	cfg := normalizeDirectorConfig(e.directorCfg)
	weights := cfg.Weights
	focusCharacter := e.GetFocusCharacter()
	plan := core.DirectorPlan{
		Mode:            cfg.Mode,
		Trigger:         "user_turn",
		PreviousSpeaker: focusCharacter,
		CreatedAt:       time.Now().UTC(),
	}
	populationCandidates := e.ensureScenePopulationCandidatesLocked(worldState, userInput)
	participantByName := make(map[string]core.ParticipantSummary)
	for _, participant := range e.sceneParticipantDetailsLocked() {
		participantByName[participant.Name] = participant
	}
	for _, name := range e.loadedCharacters {
		if _, ok := participantByName[name]; !ok {
			participantByName[name] = e.participantSummaryLocked(name, containsString(worldState.Scene.Characters, name))
		}
	}
	if cfg.Mode == "manual" {
		focusParticipant := participantByName[focusCharacter]
		plan.Selected = []string{focusCharacter}
		plan.Candidates = []string{focusCharacter}
		plan.CandidateDetails = []core.DirectorCandidate{{
			Name:       focusCharacter,
			Score:      1,
			Reason:     "director manual mode",
			Kind:       focusParticipant.Kind,
			Source:     focusParticipant.Source,
			Loaded:     focusParticipant.Loaded,
			Switchable: focusParticipant.Switchable,
			Present:    true,
			Selected:   true,
		}}
		plan.Reason = "director manual mode"
		plan.WorldSignals = buildWorldSignals(worldState, nil)
		plan.Steps = buildTurnSteps(plan.Selected, plan.PreviousSpeaker, false, worldState, userInput, nil)
		e.lastPlan = plan
		return plan
	}

	candidates := e.directorCandidatesLocked(worldState)
	plan.Candidates = append([]string(nil), candidates...)
	if len(candidates) == 0 {
		focusParticipant := participantByName[focusCharacter]
		plan.Selected = []string{focusCharacter}
		plan.CandidateDetails = []core.DirectorCandidate{{
			Name:       focusCharacter,
			Score:      1,
			Reason:     "no alternate candidates",
			Kind:       focusParticipant.Kind,
			Source:     focusParticipant.Source,
			Loaded:     focusParticipant.Loaded,
			Switchable: focusParticipant.Switchable,
			Present:    true,
			Selected:   true,
		}}
		plan.Reason = "no alternate candidates"
		plan.WorldSignals = buildWorldSignals(worldState, nil)
		plan.Steps = buildTurnSteps(plan.Selected, plan.PreviousSpeaker, false, worldState, userInput, nil)
		e.lastPlan = plan
		return plan
	}

	var scoredList []directorCandidateScore
	needle := strings.ToLower(strings.TrimSpace(userInput))
	for _, name := range candidates {
		participant := participantByName[name]
		score := 0.0
		var reasons []string
		breakdown := map[string]float64{}
		mentionIndex := strings.Index(needle, strings.ToLower(name))
		mentioned := mentionIndex >= 0
		present := containsString(worldState.Scene.Characters, name)
		silenceBoost := 0.0
		if mentioned {
			score += weights["mentioned"]
			breakdown["mentioned"] += weights["mentioned"]
			reasons = append(reasons, "mentioned by user")
			mentionOrderBoost := maxFloat(0, weights["mention_order"]-float64(mentionIndex))
			score += mentionOrderBoost
			breakdown["mention_order"] += mentionOrderBoost
			reasons = append(reasons, fmt.Sprintf("mention order %d", mentionIndex))
		}
		if name == focusCharacter && participant.Source != "scene_presence" && participant.Source != "scene_shell" && participant.Kind != "player" {
			score += weights["continuity"]
			breakdown["continuity"] += weights["continuity"]
			reasons = append(reasons, "current speaker continuity")
		}
		if present {
			score += weights["present"]
			breakdown["present"] += weights["present"]
			reasons = append(reasons, "present in scene")
		}
		if candidate, ok := populationCandidates[name]; ok {
			if candidate.LocationMatch {
				score += weights["location_match"]
				breakdown["location_match"] += weights["location_match"]
			}
			if candidate.FactionMatch {
				score += weights["faction_match"]
				breakdown["faction_match"] += weights["faction_match"]
			}
			if candidate.PressureMatch {
				score += weights["pressure_match"]
				breakdown["pressure_match"] += weights["pressure_match"]
			}
			if candidate.HookMatch {
				score += weights["hook_match"]
				breakdown["hook_match"] += weights["hook_match"]
			}
			if candidate.Reason != "" {
				reasons = append(reasons, candidate.Reason)
			}
		}
		if last, ok := e.memEngine.LastAssistantAt(name); ok {
			silence := time.Since(last).Minutes()
			if silence > 0 {
				silenceBoost = minFloat(silence/weights["silence_divisor"], weights["silence_cap"])
				score += silenceBoost
				breakdown["silence_boost"] += silenceBoost
				reasons = append(reasons, fmt.Sprintf("silent %.0fm", silence))
			}
		} else {
			silenceBoost = minFloat(weights["silence_cap"], 10)
			score += silenceBoost
			breakdown["silence_boost"] += silenceBoost
			reasons = append(reasons, "has not spoken recently")
		}
		if char, ok := e.agents.GetCharacter(name); ok {
			trustBoost := char.Identity.Adaptive["trust"] * weights["trust"]
			intimacyBoost := char.Identity.Adaptive["intimacy"] * weights["intimacy"]
			fearBoost := char.Identity.Adaptive["fear"] * weights["fear"]
			score += trustBoost
			score += intimacyBoost
			score += fearBoost
			breakdown["trust"] += trustBoost
			breakdown["intimacy"] += intimacyBoost
			breakdown["fear"] += fearBoost
		}
		if mentioned && mentionIndex == 0 {
			score += weights["opened_by_user"]
			breakdown["opened_by_user"] += weights["opened_by_user"]
			reasons = append(reasons, "opened by user cue")
		}
		if worldState.Tension >= 0.6 && name != focusCharacter {
			score += weights["tension_switch"]
			breakdown["tension_switch"] += weights["tension_switch"]
			reasons = append(reasons, "high tension favors switch")
		}
		if participant.Kind == "persona" {
			score += weights["kind_persona"]
			breakdown["kind_persona"] += weights["kind_persona"]
			reasons = append(reasons, "kind=persona")
		} else if participant.Kind == "npc" {
			score += weights["kind_npc"]
			breakdown["kind_npc"] += weights["kind_npc"]
			reasons = append(reasons, "kind=npc")
		}
		switch participant.Source {
		case "promoted_population":
			score += weights["source_promoted"]
			breakdown["source_promoted"] += weights["source_promoted"]
			reasons = append(reasons, "source=promoted")
		case "character_definition":
			score += weights["source_definition"]
			breakdown["source_definition"] += weights["source_definition"]
			reasons = append(reasons, "source=definition")
		case "background_population":
			score += weights["source_background"]
			breakdown["source_background"] += weights["source_background"]
			reasons = append(reasons, "source=background")
		}
		if participant.Loaded {
			score += weights["loaded"]
			breakdown["loaded"] += weights["loaded"]
			reasons = append(reasons, "loaded")
		}
		scoredList = append(scoredList, directorCandidateScore{
			name:          name,
			score:         score,
			reason:        strings.Join(reasons, ", "),
			breakdown:     breakdown,
			kind:          participant.Kind,
			source:        participant.Source,
			loaded:        participant.Loaded,
			switchable:    participant.Switchable,
			mentioned:     mentioned,
			mentionIndex:  mentionIndex,
			present:       present,
			silenceBoost:  silenceBoost,
			locationMatch: populationCandidates[name].LocationMatch,
			factionMatch:  populationCandidates[name].FactionMatch,
			pressureMatch: populationCandidates[name].PressureMatch,
			hookMatch:     populationCandidates[name].HookMatch,
		})
	}

	sort.SliceStable(scoredList, func(i, j int) bool {
		if scoredList[i].mentioned != scoredList[j].mentioned {
			return scoredList[i].mentioned
		}
		if scoredList[i].mentioned && scoredList[j].mentioned && scoredList[i].mentionIndex != scoredList[j].mentionIndex {
			return scoredList[i].mentionIndex < scoredList[j].mentionIndex
		}
		if scoredList[i].score == scoredList[j].score {
			return scoredList[i].name < scoredList[j].name
		}
		return scoredList[i].score > scoredList[j].score
	})

	maxSpeakers := cfg.MaxSpeakers
	if maxSpeakers > len(scoredList) {
		maxSpeakers = len(scoredList)
	}
	plan.Selected = buildSelectedSpeakers(scoredList, worldState, cfg.Mode == "auto_chain", maxSpeakers)
	if len(plan.Selected) == 0 {
		plan.Selected = []string{focusCharacter}
	}
	selectedSet := make(map[string]bool, len(plan.Selected))
	for _, name := range plan.Selected {
		selectedSet[name] = true
	}
	plan.CandidateDetails = buildDirectorCandidateDetails(scoredList, selectedSet)
	plan.WorldSignals = buildWorldSignals(worldState, scoredList)
	plan.Reason = buildPlanReason(scoredList, plan.Selected, cfg.Mode == "auto_chain")
	if lead := plan.Selected[0]; lead != "" && lead != focusCharacter {
		plan.Switched = true
	}
	plan.Steps = buildTurnSteps(plan.Selected, plan.PreviousSpeaker, cfg.Mode == "auto_chain", worldState, userInput, scoredList)
	e.lastPlan = plan
	return plan
}

func buildSelectedSpeakers(scoredList []directorCandidateScore, worldState core.WorldState, allowChain bool, maxSpeakers int) []string {
	if len(scoredList) == 0 || maxSpeakers <= 0 {
		return nil
	}

	selected := []string{scoredList[0].name}
	if !allowChain || maxSpeakers == 1 {
		return selected
	}

	lead := scoredList[0]
	if candidate := pickFollowup(scoredList, selected, func(s directorCandidateScore) bool {
		return s.mentioned && s.name != lead.name
	}); candidate != nil {
		selected = append(selected, candidate.name)
	}
	if len(selected) >= maxSpeakers {
		return selected
	}

	if candidate := pickFollowup(scoredList, selected, func(s directorCandidateScore) bool {
		if s.name == lead.name {
			return false
		}
		rel := relationshipWeight(worldState, lead.name, s.name)
		return rel >= 0.75 || (worldState.Tension >= 0.6 && rel > 0)
	}); candidate != nil {
		selected = append(selected, candidate.name)
	}
	if len(selected) >= maxSpeakers {
		return selected
	}

	if candidate := pickFollowup(scoredList, selected, func(s directorCandidateScore) bool {
		if s.name == lead.name {
			return false
		}
		return worldState.Tension >= 0.65 && (s.present || s.silenceBoost >= 8)
	}); candidate != nil {
		selected = append(selected, candidate.name)
	}
	if len(selected) >= maxSpeakers {
		return selected
	}

	for _, candidate := range scoredList {
		if len(selected) >= maxSpeakers {
			break
		}
		if containsString(selected, candidate.name) {
			continue
		}
		selected = append(selected, candidate.name)
	}
	return selected
}

func pickFollowup(scoredList []directorCandidateScore, selected []string, match func(directorCandidateScore) bool) *directorCandidateScore {
	for i := range scoredList {
		candidate := scoredList[i]
		if containsString(selected, candidate.name) || !match(candidate) {
			continue
		}
		return &candidate
	}
	return nil
}

func buildPlanReason(scoredList []directorCandidateScore, selected []string, allowChain bool) string {
	if len(scoredList) == 0 {
		return "no scored candidates"
	}
	if !allowChain || len(selected) <= 1 {
		return scoredList[0].reason
	}
	parts := []string{fmt.Sprintf("lead %s: %s", selected[0], scoredList[0].reason)}
	for _, name := range selected[1:] {
		for _, candidate := range scoredList {
			if candidate.name == name {
				parts = append(parts, fmt.Sprintf("followup %s: %s", name, candidate.reason))
				break
			}
		}
	}
	return strings.Join(parts, " | ")
}

func buildDirectorCandidateDetails(scoredList []directorCandidateScore, selectedSet map[string]bool) []core.DirectorCandidate {
	out := make([]core.DirectorCandidate, 0, len(scoredList))
	for _, candidate := range scoredList {
		out = append(out, core.DirectorCandidate{
			Name:           candidate.name,
			Score:          candidate.score,
			Reason:         candidate.reason,
			Kind:           candidate.kind,
			Source:         candidate.source,
			Loaded:         candidate.loaded,
			Switchable:     candidate.switchable,
			Mentioned:      candidate.mentioned,
			Present:        candidate.present,
			LocationMatch:  candidate.locationMatch,
			FactionMatch:   candidate.factionMatch,
			PressureMatch:  candidate.pressureMatch,
			HookMatch:      candidate.hookMatch,
			ScoreBreakdown: cloneScoreBreakdown(candidate.breakdown),
			DominantFactors: dominantDirectorFactors(candidate.breakdown),
			Selected:       selectedSet[candidate.name],
		})
	}
	return out
}

func dominantDirectorFactors(breakdown map[string]float64) []string {
	if len(breakdown) == 0 {
		return nil
	}
	labels := map[string]string{
		"mentioned":       "用户点名",
		"mention_order":   "点名顺位",
		"continuity":      "连续发言",
		"present":         "当前在场",
		"location_match":  "地点命中",
		"faction_match":   "势力命中",
		"pressure_match":  "压力命中",
		"hook_match":      "hook 命中",
		"silence_boost":   "静默补偿",
		"trust":           "信任倾向",
		"intimacy":        "亲密倾向",
		"fear":            "恐惧倾向",
		"opened_by_user":  "用户开场",
		"tension_switch":  "高张力切换",
		"kind_persona":    "persona 身份",
		"kind_npc":        "npc 身份",
		"source_promoted": "晋升人口来源",
		"source_definition":"人物定义来源",
		"source_background":"背景人口来源",
		"loaded":          "已加载",
	}
	type item struct {
		key   string
		value float64
	}
	items := make([]item, 0, len(breakdown))
	for key, value := range breakdown {
		if value <= 0.01 {
			continue
		}
		items = append(items, item{key: key, value: value})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].value == items[j].value {
			return items[i].key < items[j].key
		}
		return items[i].value > items[j].value
	})
	out := make([]string, 0, minInt(len(items), 3))
	for _, it := range items[:minInt(len(items), 3)] {
		label := labels[it.key]
		if label == "" {
			label = it.key
		}
		out = append(out, label)
	}
	return out
}

func buildWorldSignals(worldState core.WorldState, scoredList []directorCandidateScore) []string {
	signals := make([]string, 0, 4)
	if worldState.Tension >= 0.8 {
		signals = append(signals, "世界高张力")
	} else if worldState.Tension >= 0.6 {
		signals = append(signals, "世界张力上升")
	}
	pressureHits := 0
	factionHits := 0
	locationHits := 0
	for _, candidate := range scoredList {
		if candidate.pressureMatch {
			pressureHits++
		}
		if candidate.factionMatch {
			factionHits++
		}
		if candidate.locationMatch {
			locationHits++
		}
	}
	if pressureHits > 0 {
		signals = append(signals, fmt.Sprintf("%d 个候选命中当前 pressure", pressureHits))
	}
	if factionHits > 0 {
		signals = append(signals, fmt.Sprintf("%d 个候选命中当前 faction", factionHits))
	}
	if locationHits > 0 {
		signals = append(signals, fmt.Sprintf("%d 个候选在当前 scene 位置相关", locationHits))
	}
	if len(signals) == 0 {
		signals = append(signals, "当前主要由用户输入与在场状态驱动")
	}
	return signals
}

func cloneScoreBreakdown(src map[string]float64) map[string]float64 {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]float64, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func buildTurnSteps(selected []string, previousSpeaker string, allowChain bool, worldState core.WorldState, userInput string, scoredList []directorCandidateScore) []core.TurnStep {
	if len(selected) == 0 {
		return nil
	}
	if !allowChain && len(selected) > 1 {
		selected = selected[:1]
	}

	steps := make([]core.TurnStep, 0, len(selected))
	prev := previousSpeaker
	lead := selected[0]
	needle := strings.ToLower(strings.TrimSpace(userInput))
	for i, speaker := range selected {
		kind := "lead"
		reason := "lead speaker"
		if i > 0 {
			kind = "followup"
			reason = "director chained follow-up"
			if strings.Contains(needle, strings.ToLower(speaker)) {
				kind = "addressed_reply"
				reason = "user explicitly addressed this character"
			} else if relationshipWeight(worldState, lead, speaker) >= 0.75 {
				kind = "support_response"
				reason = fmt.Sprintf("strong relationship with lead %s", lead)
			} else if worldState.Tension >= 0.65 {
				kind = "tension_response"
				reason = "high tension reaction slot"
			}
			for _, candidate := range scoredList {
				if candidate.name == speaker && candidate.reason != "" {
					reason = fmt.Sprintf("%s; %s", reason, candidate.reason)
					break
				}
			}
		}
		budgetMode := "normal"
		if speaker != prev {
			budgetMode = "full_load"
		}
		steps = append(steps, core.TurnStep{
			ID:         fmt.Sprintf("turn_step_%d_%s", i, speaker),
			Index:      i,
			Speaker:    speaker,
			Kind:       kind,
			Reason:     reason,
			BudgetMode: budgetMode,
		})
		prev = speaker
	}
	return steps
}

func relationshipWeight(worldState core.WorldState, a, b string) float64 {
	if a == "" || b == "" || a == b {
		return 0
	}
	keys := []string{fmt.Sprintf("%s_%s", a, b), fmt.Sprintf("%s_%s", b, a)}
	for _, key := range keys {
		if rel, ok := worldState.Relationships[key]; ok {
			return (rel.Trust + rel.Intimacy + rel.Fear + rel.Respect + rel.Debt) / 50.0
		}
	}
	return 0
}

func (e *Engine) directorCandidatesLocked(worldState core.WorldState) []string {
	seen := map[string]bool{}
	var out []string
	activeWorldPath := e.currentWorldPathLocked()
	if strings.TrimSpace(activeWorldPath) != "" {
		if cfg, _, err := world.EnsureSeededPopulation(activeWorldPath); err == nil {
			structure, _ := world.LoadStructure(activeWorldPath)
			activeWorld := e.charWorlds[e.GetFocusCharacter()]
			for _, npc := range cfg.BackgroundNPCs {
				candidate := buildPopulationSceneCandidate(npc, worldState, structure, "")
				if !populationCandidateRelevant(candidate) {
					continue
				}
				e.ensureBackgroundNPCLoadedLocked(activeWorldPath, activeWorld, npc)
			}
		}
	}
	participantByName := make(map[string]core.ParticipantSummary)
	for _, participant := range e.sceneParticipantDetailsLocked() {
		participantByName[participant.Name] = participant
	}
	for _, name := range worldState.Scene.Characters {
		participant, ok := participantByName[name]
		if !ok {
			participant = e.participantSummaryLocked(name, true)
		}
		if participant.Kind == "player" || participant.Source == "player_role" || participant.Source == "scene_presence" || participant.Source == "scene_shell" || !participant.Switchable {
			continue
		}
		if _, ok := e.agents.GetCharacter(name); ok && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	for _, name := range e.loadedCharacters {
		if seen[name] {
			continue
		}
		participant, ok := participantByName[name]
		if !ok {
			participant = e.participantSummaryLocked(name, containsString(worldState.Scene.Characters, name))
		}
		if participant.Kind == "player" || participant.Source == "player_role" || participant.Source == "scene_presence" || participant.Source == "scene_shell" || !participant.Switchable {
			continue
		}
		if activeWorldPath != "" && e.currentWorldPathFor(name) != "" && e.currentWorldPathFor(name) != activeWorldPath {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
