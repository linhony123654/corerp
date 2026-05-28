package runtime

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"corerp/internal/core"
	"corerp/internal/emotion"
	"corerp/internal/events"
	"corerp/internal/world"
)

const populationAttentionEventWindow = 72 * time.Hour

type populationSceneCandidate struct {
	Name          string
	LocationMatch bool
	FactionMatch  bool
	PressureMatch bool
	HookMatch     bool
	Reason        string
}

func (e *Engine) reconcilePopulationLocked() {
	path := strings.TrimSpace(e.currentWorldPathLocked())
	if path == "" {
		return
	}

	cfg, _, err := world.EnsureSeededPopulation(path)
	if err != nil || len(cfg.BackgroundNPCs) == 0 {
		return
	}
	eventList, err := e.eventStore.GetCanonicalEvents()
	if err != nil {
		return
	}

	updated, changed, promoted, demoted, identityShifts := reconcilePopulationConfig(cfg, e.stateMgr.Get(), eventList, e.npcTickExposure, e.factionEng)
	if !changed {
		return
	}
	if _, err := world.SavePopulation(path, updated); err != nil {
		return
	}
	e.ensurePromotedCharactersLoadedLocked(path, updated)
	e.refreshPopulationDesiresLocked(updated, promoted, identityShifts)
	for _, npc := range promoted {
		evt := events.BuildEvent("population_promoted", "system", npc.Name, map[string]interface{}{
			"npc_id":        npc.ID,
			"npc_name":      npc.Name,
			"status":        npc.Status,
			"identity_core": npc.IdentityCore,
			"score":         npc.Attention.Score,
		})
		evt.Tag = core.TagSystem
		_ = e.gatekeeper.Submit(evt, events.SourceSystem())
	}
	for _, npc := range demoted {
		evt := events.BuildEvent("population_demoted", "system", npc.Name, map[string]interface{}{
			"npc_id":        npc.ID,
			"npc_name":      npc.Name,
			"identity_core": npc.IdentityCore,
			"score":         npc.Attention.Score,
			"reason":        "attention fell below demotion threshold",
		})
		evt.Tag = core.TagSystem
		_ = e.gatekeeper.Submit(evt, events.SourceSystem())
	}
	for _, shift := range identityShifts {
		evt := events.BuildEvent("population_identity_shift", "system", shift.Name, map[string]interface{}{
			"npc_id":        shift.ID,
			"npc_name":      shift.Name,
			"identity_core": shift.IdentityCore,
			"adaptive":      shift.Adaptive,
			"summary":       shift.GrowthSummary,
		})
		evt.Tag = core.TagSystem
		_ = e.gatekeeper.Submit(evt, events.SourceSystem())
	}
}

func (e *Engine) refreshPopulationDesiresLocked(cfg core.PopulationConfig, promoted []core.PromotedNPC, identityShifts []core.PopulationCharacterInsight) {
	if e.desireStore == nil {
		return
	}
	seen := make(map[string]bool)
	for _, npc := range promoted {
		seen[npc.Name] = true
	}
	for _, shift := range identityShifts {
		seen[shift.Name] = true
	}
	for name := range seen {
		char, ok := e.agents.GetCharacter(name)
		if !ok {
			continue
		}
		var goals []emotion.GoalSeed
		var hidden []emotion.HiddenGoalSeed
		for _, g := range char.Goals {
			if g.Type == "hidden" {
				hidden = append(hidden, emotion.HiddenGoalSeed{ID: g.ID, Priority: g.Priority})
			} else {
				goals = append(goals, emotion.GoalSeed{ID: g.ID, Priority: g.Priority, Target: g.Target})
			}
		}
		emotion.ReplaceDesires(e.desireStore, name, char.Identity.Immutable, char.Identity.Adaptive, goals, hidden)
	}
}

func (e *Engine) syncAutonomousScenePopulationLocked(state *core.WorldState) []string {
	if state == nil {
		return nil
	}
	candidates := e.ensureScenePopulationCandidatesLocked(*state, "")
	if len(candidates) == 0 {
		return nil
	}
	names := make([]string, 0, len(candidates))
	for name, candidate := range candidates {
		if !candidate.LocationMatch && !candidate.FactionMatch && !candidate.PressureMatch {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	added := make([]string, 0, len(names))
	for _, name := range names {
		if containsString(state.Scene.Characters, name) {
			continue
		}
		state.Scene.Characters = append(state.Scene.Characters, name)
		added = append(added, name)
	}
	return added
}

func (e *Engine) ensureScenePopulationCandidatesLocked(state core.WorldState, userInput string) map[string]populationSceneCandidate {
	path := strings.TrimSpace(e.currentWorldPathLocked())
	if path == "" {
		return nil
	}
	cfg, _, err := world.EnsureSeededPopulation(path)
	if err != nil {
		return nil
	}
	e.ensurePromotedCharactersLoadedLocked(path, cfg)
	structure, _ := world.LoadStructure(path)

	activeWorld := e.charWorlds[e.GetFocusCharacter()]
	sceneCandidates := make(map[string]populationSceneCandidate)
	for _, npc := range cfg.BackgroundNPCs {
		candidate := buildPopulationSceneCandidate(npc, state, structure, userInput)
		if !populationCandidateRelevant(candidate) {
			continue
		}
		e.ensureBackgroundNPCLoadedLocked(path, activeWorld, npc)
		sceneCandidates[npc.Name] = candidate
	}
	return sceneCandidates
}

func (e *Engine) ensureBackgroundNPCLoadedLocked(path string, activeWorld CharWorld, npc core.BackgroundNPC) {
	if _, ok := e.agents.GetCharacter(npc.Name); !ok {
		e.agents.LoadCharacter(npc.Name, core.Character{
			WorldPath: path,
			Identity: core.IdentityEnvelope{
				Name:         npc.Name,
				Immutable:    append([]string(nil), npc.Traits...),
				Adaptive:     map[string]float64{"trust": 3, "fear": 2},
				Forbidden:    nil,
				Voice:        core.VoiceConfig{},
				WritingGuide: strings.Join(npc.Hooks, " / "),
			},
		})
	}
	if !containsString(e.loadedCharacters, npc.Name) {
		e.loadedCharacters = append(e.loadedCharacters, npc.Name)
	}
	if e.worldPaths == nil {
		e.worldPaths = map[string]string{}
	}
	e.worldPaths[npc.Name] = path
	if e.charWorlds == nil {
		e.charWorlds = map[string]CharWorld{}
	}
	if _, ok := e.charWorlds[npc.Name]; !ok {
		e.charWorlds[npc.Name] = CharWorld{
			WorldName: activeWorld.WorldName,
			CoreRules: activeWorld.CoreRules,
			Scene:     activeWorld.Scene,
		}
	}
}

func populationCandidateRelevant(candidate populationSceneCandidate) bool {
	return candidate.Name != "" && (candidate.LocationMatch || candidate.FactionMatch || candidate.PressureMatch || candidate.HookMatch)
}

func buildPopulationSceneCandidate(npc core.BackgroundNPC, state core.WorldState, structure core.WorldStructureConfig, userInput string) populationSceneCandidate {
	candidate := populationSceneCandidate{
		Name:          npc.Name,
		LocationMatch: strings.TrimSpace(npc.Location) != "" && strings.TrimSpace(npc.Location) == strings.TrimSpace(state.Scene.Location),
	}
	candidate.FactionMatch = backgroundNPCFactionMatch(npc, state, structure)
	candidate.PressureMatch = backgroundNPCPressureMatch(npc, state, structure)
	candidate.HookMatch = backgroundNPCHookMatch(npc, userInput)
	candidate.Reason = backgroundNPCSceneReason(npc, candidate)
	return candidate
}

func backgroundNPCFactionMatch(npc core.BackgroundNPC, state core.WorldState, structure core.WorldStructureConfig) bool {
	faction := strings.TrimSpace(npc.Faction)
	if faction == "" {
		return false
	}
	sceneLocation := strings.TrimSpace(state.Scene.Location)
	for _, location := range structure.Locations {
		if strings.TrimSpace(location.Name) != sceneLocation {
			continue
		}
		if strings.TrimSpace(location.Controller) == faction {
			return true
		}
	}
	return false
}

func backgroundNPCPressureMatch(npc core.BackgroundNPC, state core.WorldState, structure core.WorldStructureConfig) bool {
	sceneLocation := strings.TrimSpace(state.Scene.Location)
	for _, pressure := range structure.Pressures {
		if pressure.Intensity <= 0 {
			continue
		}
		target := strings.TrimSpace(pressure.Target)
		if target == "" {
			continue
		}
		if target == sceneLocation || (strings.TrimSpace(npc.Faction) != "" && target == strings.TrimSpace(npc.Faction)) || (strings.TrimSpace(npc.Location) != "" && target == strings.TrimSpace(npc.Location)) {
			return true
		}
	}
	return false
}

func backgroundNPCHookMatch(npc core.BackgroundNPC, userInput string) bool {
	needle := strings.ToLower(strings.TrimSpace(userInput))
	if needle == "" {
		return false
	}
	for _, part := range append(append([]string{npc.Role, npc.Faction, npc.Name}, npc.Traits...), npc.Hooks...) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(needle, strings.ToLower(part)) || strings.Contains(strings.ToLower(part), needle) {
			return true
		}
	}
	return false
}

func backgroundNPCSceneReason(npc core.BackgroundNPC, candidate populationSceneCandidate) string {
	reasons := make([]string, 0, 4)
	if candidate.LocationMatch {
		reasons = append(reasons, "background npc shares current scene location")
	}
	if candidate.FactionMatch {
		reasons = append(reasons, "npc faction controls current scene")
	}
	if candidate.PressureMatch {
		reasons = append(reasons, "npc is tied to current world pressure")
	}
	if candidate.HookMatch {
		reasons = append(reasons, "user input matches npc hook")
	}
	if len(reasons) == 0 {
		return "background npc available in current world"
	}
	return strings.Join(reasons, ", ")
}

func (e *Engine) GetPopulationInsights() (core.PopulationInsights, error) {
	e.mu.RLock()
	path := strings.TrimSpace(e.currentWorldPathLocked())
	focusCharacter := e.GetFocusCharacter()
	e.mu.RUnlock()
	if path == "" {
		return core.PopulationInsights{}, fmt.Errorf("world path for focus character '%s' is not configured", focusCharacter)
	}
	cfg, _, err := world.EnsureSeededPopulation(path)
	if err != nil {
		return core.PopulationInsights{}, err
	}
	eventList, err := e.eventStore.GetCanonicalEvents()
	if err != nil {
		return core.PopulationInsights{}, err
	}
	identityByID := make(map[string]core.IdentityCoreConfig, len(cfg.IdentityCores))
	for _, coreCfg := range cfg.IdentityCores {
		identityByID[coreCfg.ID] = coreCfg
	}
	insights := core.PopulationInsights{
		Path:       cfg.Path,
		WorldPath:  path,
		Promoted:   make([]core.PopulationCharacterInsight, 0, len(cfg.PromotedNPCs)),
		Background: make([]core.PopulationCharacterInsight, 0, len(cfg.BackgroundNPCs)),
	}
	for _, npc := range cfg.PromotedNPCs {
		coreCfg := identityByID[npc.IdentityCore]
		insights.Promoted = append(insights.Promoted, core.PopulationCharacterInsight{
			ID:            npc.ID,
			Name:          npc.Name,
			Status:        npc.Status,
			IdentityCore:  npc.IdentityCore,
			Attention:     npc.Attention,
			LastEventID:   npc.LastEventID,
			Adaptive:      cloneAdaptive(coreCfg.Adaptive),
			Immutable:     append([]string(nil), coreCfg.Immutable...),
			SpeechHints:   append([]string(nil), coreCfg.SpeechHints...),
			Drives:        append([]string(nil), coreCfg.Drives...),
			WorldPath:     path,
			GrowthSummary: growthSummary(npc.Attention),
			History:       buildPopulationHistory(eventList, npc.Name),
		})
	}
	for _, npc := range cfg.BackgroundNPCs {
		insights.Background = append(insights.Background, core.PopulationCharacterInsight{
			ID:            npc.ID,
			Name:          npc.Name,
			Attention:     npc.Attention,
			WorldPath:     path,
			GrowthSummary: growthSummary(npc.Attention),
			History:       buildPopulationHistory(eventList, npc.Name),
		})
	}
	sort.Slice(insights.Promoted, func(i, j int) bool {
		if insights.Promoted[i].Attention.Score == insights.Promoted[j].Attention.Score {
			return insights.Promoted[i].Name < insights.Promoted[j].Name
		}
		return insights.Promoted[i].Attention.Score > insights.Promoted[j].Attention.Score
	})
	sort.Slice(insights.Background, func(i, j int) bool {
		if insights.Background[i].Attention.Score == insights.Background[j].Attention.Score {
			return insights.Background[i].Name < insights.Background[j].Name
		}
		return insights.Background[i].Attention.Score > insights.Background[j].Attention.Score
	})
	return insights, nil
}

func (e *Engine) populationHighlightsLocked() []string {
	path := strings.TrimSpace(e.currentWorldPathLocked())
	if path == "" {
		return nil
	}
	cfg, _, err := world.EnsureSeededPopulation(path)
	if err != nil {
		return nil
	}
	highlights := make([]string, 0, 2)
	if len(cfg.PromotedNPCs) > 0 {
		promoted := append([]core.PromotedNPC(nil), cfg.PromotedNPCs...)
		sort.Slice(promoted, func(i, j int) bool {
			if promoted[i].Attention.Score == promoted[j].Attention.Score {
				return promoted[i].Name < promoted[j].Name
			}
			return promoted[i].Attention.Score > promoted[j].Attention.Score
		})
		top := promoted[:minInt(len(promoted), 2)]
		parts := make([]string, 0, len(top))
		for _, npc := range top {
			parts = append(parts, fmt.Sprintf("%s(%.1f)", npc.Name, npc.Attention.Score))
		}
		highlights = append(highlights, "promoted: "+strings.Join(parts, ", "))
	}
	if len(cfg.BackgroundNPCs) > 0 {
		background := append([]core.BackgroundNPC(nil), cfg.BackgroundNPCs...)
		sort.Slice(background, func(i, j int) bool {
			if background[i].Attention.Score == background[j].Attention.Score {
				return background[i].Name < background[j].Name
			}
			return background[i].Attention.Score > background[j].Attention.Score
		})
		top := background[:minInt(len(background), 2)]
		parts := make([]string, 0, len(top))
		for _, npc := range top {
			parts = append(parts, fmt.Sprintf("%s(%.1f)", npc.Name, npc.Attention.Score))
		}
		highlights = append(highlights, "rising: "+strings.Join(parts, ", "))
	}
	return highlights
}

func (e *Engine) ensurePromotedCharactersLoadedLocked(path string, cfg core.PopulationConfig) {
	activeWorld := e.charWorlds[e.GetFocusCharacter()]
	identityByID := make(map[string]core.IdentityCoreConfig, len(cfg.IdentityCores))
	for _, coreCfg := range cfg.IdentityCores {
		identityByID[coreCfg.ID] = coreCfg
	}
	for _, promoted := range cfg.PromotedNPCs {
		coreCfg, ok := identityByID[promoted.IdentityCore]
		if !ok {
			continue
		}
		e.agents.LoadCharacter(promoted.Name, core.Character{
			WorldPath: path,
			Identity: core.IdentityEnvelope{
				Name:         promoted.Name,
				Immutable:    append([]string(nil), coreCfg.Immutable...),
				Adaptive:     cloneAdaptive(coreCfg.Adaptive),
				Forbidden:    nil,
				Voice:        core.VoiceConfig{},
				WritingGuide: strings.Join(coreCfg.SpeechHints, " / "),
			},
			Goals: nil,
		})
		if !containsString(e.loadedCharacters, promoted.Name) {
			e.loadedCharacters = append(e.loadedCharacters, promoted.Name)
		}
		if e.worldPaths == nil {
			e.worldPaths = map[string]string{}
		}
		e.worldPaths[promoted.Name] = path
		if path == e.currentWorldPathFor(e.GetFocusCharacter()) {
			e.activeWorldPath = path
		}
		if e.charWorlds == nil {
			e.charWorlds = map[string]CharWorld{}
		}
		if _, ok := e.charWorlds[promoted.Name]; !ok {
			e.charWorlds[promoted.Name] = CharWorld{
				WorldName: activeWorld.WorldName,
				CoreRules: activeWorld.CoreRules,
				Scene:     activeWorld.Scene,
			}
		}
	}
}

func reconcilePopulationConfig(cfg core.PopulationConfig, state core.WorldState, eventList []core.Event, tickExposure map[string]int, factionEng interface{ Tensions() map[string]float64 }) (core.PopulationConfig, bool, []core.PromotedNPC, []core.PromotedNPC, []core.PopulationCharacterInsight) {
	updated := cfg
	changed := false
	var promotedNow []core.PromotedNPC
	var demotedNow []core.PromotedNPC
	var identityShifts []core.PopulationCharacterInsight

	promotedIndex := make(map[string]int, len(updated.PromotedNPCs))
	for i, npc := range updated.PromotedNPCs {
		promotedIndex[populationKey(npc.ID, npc.Name)] = i
	}
	identityIndex := make(map[string]int, len(updated.IdentityCores))
	for i, coreCfg := range updated.IdentityCores {
		identityIndex[coreCfg.ID] = i
	}

	for i := range updated.BackgroundNPCs {
		npc := &updated.BackgroundNPCs[i]
		exposure := 0
		if tickExposure != nil {
			exposure = tickExposure[npc.Name]
		}
		attention, lastEventID := calculatePopulationAttention(*npc, state, eventList, updated.Policy, exposure, factionEng)
		if !populationAttentionEqual(npc.Attention, attention) {
			npc.Attention = attention
			changed = true
		}

		key := populationKey(npc.ID, npc.Name)
		if idx, ok := promotedIndex[key]; ok {
			p := &updated.PromotedNPCs[idx]
			if coreIdx, ok := identityIndex[p.IdentityCore]; ok {
				before := updated.IdentityCores[coreIdx]
				evolved := evolveIdentityCore(before, *npc, eventList)
				if !identityCoreEqual(before, evolved) {
					updated.IdentityCores[coreIdx] = evolved
					identityShifts = append(identityShifts, core.PopulationCharacterInsight{
						ID:            p.ID,
						Name:          p.Name,
						Status:        p.Status,
						IdentityCore:  p.IdentityCore,
						Adaptive:      cloneAdaptive(evolved.Adaptive),
						GrowthSummary: summarizeAdaptiveShift(before.Adaptive, evolved.Adaptive),
					})
					changed = true
				}
			}
			if !populationAttentionEqual(p.Attention, attention) || p.LastEventID != lastEventID || p.Status != promotionStatus(attention.Score, updated.Policy) {
				p.Attention = attention
				p.LastEventID = lastEventID
				p.Status = promotionStatus(attention.Score, updated.Policy)
				changed = true
			}
			continue
		}

		if attention.Score < updated.Policy.PromoteThreshold {
			continue
		}

		identityCoreID, identityCore := ensureIdentityCore(*npc)
		identityCore = evolveIdentityCore(identityCore, *npc, eventList)
		if _, ok := identityIndex[identityCoreID]; !ok {
			updated.IdentityCores = append(updated.IdentityCores, identityCore)
			identityIndex[identityCoreID] = len(updated.IdentityCores) - 1
			changed = true
		}

		promotedNPC := core.PromotedNPC{
			ID:           defaultPopulationID(npc.ID, npc.Name),
			Name:         npc.Name,
			From:         "background",
			Status:       promotionStatus(attention.Score, updated.Policy),
			IdentityCore: identityCoreID,
			Attention:    attention,
			LastEventID:  lastEventID,
		}
		updated.PromotedNPCs = append(updated.PromotedNPCs, promotedNPC)
		promotedIndex[key] = len(updated.PromotedNPCs) - 1
		promotedNow = append(promotedNow, promotedNPC)
		changed = true
	}

	// Demote promoted NPCs whose attention has fallen below threshold
	demoteThreshold := updated.Policy.PromoteThreshold / 2
	if demoteThreshold <= 0 {
		demoteThreshold = 5
	}
	for i := len(updated.PromotedNPCs) - 1; i >= 0; i-- {
		p := updated.PromotedNPCs[i]
		bgKey := populationKey(p.ID, p.Name)
		bgIdx := -1
		for j := range updated.BackgroundNPCs {
			if populationKey(updated.BackgroundNPCs[j].ID, updated.BackgroundNPCs[j].Name) == bgKey {
				bgIdx = j
				break
			}
		}
		if bgIdx < 0 {
			continue
		}
		if updated.BackgroundNPCs[bgIdx].Attention.Score < demoteThreshold {
			updated.PromotedNPCs = append(updated.PromotedNPCs[:i], updated.PromotedNPCs[i+1:]...)
			p.Attention = updated.BackgroundNPCs[bgIdx].Attention
			demotedNow = append(demotedNow, p)
			changed = true
		}
	}

	return updated, changed, promotedNow, demotedNow, identityShifts
}

func calculatePopulationAttention(npc core.BackgroundNPC, state core.WorldState, eventList []core.Event, policy core.PromotionPolicy, tickExposure int, factionEng interface{ Tensions() map[string]float64 }) (core.PopulationAttention, string) {
	var att core.PopulationAttention
	lastEventID := ""
	name := strings.TrimSpace(npc.Name)
	id := strings.TrimSpace(npc.ID)

	for _, evt := range eventList {
		if populationEventStale(evt) {
			continue
		}
		matched := false
		if matchesPopulationRef(evt.Actor, name, id) || matchesPopulationRef(evt.Target, name, id) {
			att.DirectInteractions++
			att.SharedEvents++
			matched = true
		}
		if evt.Type == "world_pressure" && worldPressureTouchesNPC(evt, npc) {
			att.SharedEvents++
			matched = true
		}
		if evt.SceneID != "" && npc.Location != "" && evt.SceneID == npc.Location {
			att.SharedEvents++
			matched = true
		}
		if mentionsPopulation(evt, name) {
			att.Mentions++
			matched = true
		}
		if matched {
			lastEventID = evt.ID
		}
		switch evt.Type {
		case "trust_change", "fear_change", "intimacy_change":
			if matchesPopulationRef(evt.Actor, name, id) || matchesPopulationRef(evt.Target, name, id) {
				if delta, ok := evt.Payload["delta"].(float64); ok {
					att.RelationshipDelta += math.Abs(delta)
				}
			}
		}
	}

	if npc.Location != "" && state.Scene.Location == npc.Location {
		att.SceneCarryover = 1
	}

	// Tick exposure bonus: drives population growth without user input
	att.Score = float64(att.DirectInteractions)*policy.InteractionWeight +
		float64(att.Mentions)*policy.MentionWeight +
		float64(att.SharedEvents)*policy.EventWeight +
		att.RelationshipDelta*policy.RelationshipWeight +
		float64(att.SceneCarryover)*policy.SceneWeight

	// Add tick exposure bonus (capped at 5.0)
	exposureBonus := float64(tickExposure) * 0.05
	if exposureBonus > 5.0 {
		exposureBonus = 5.0
	}
	att.Score += exposureBonus

	// Faction tension bonus: if NPC's faction has high tension, they get extra attention
	if factionEng != nil && npc.Faction != "" {
		for facID, tension := range factionEng.Tensions() {
			if strings.TrimSpace(facID) == strings.TrimSpace(npc.Faction) && tension > 0.5 {
				att.Score += 0.5
				break
			}
		}
	}

	// Pressure match bonus: if NPC's location has active pressure events
	for _, evt := range eventList {
		if populationEventStale(evt) {
			continue
		}
		if evt.Type == "pressure" || evt.Type == "faction_pressure" || evt.Type == "world_pressure" {
			if loc, _ := evt.Payload["location"].(string); loc != "" && loc == npc.Location {
				att.Score += 0.3
				break
			}
			if target := strings.TrimSpace(evt.Target); target != "" && target == strings.TrimSpace(npc.Faction) {
				att.Score += 0.3
				break
			}
		}
	}

	// Event relevance bonus: event types matching NPC's role or hooks
	for _, evt := range eventList {
		if populationEventStale(evt) {
			continue
		}
		if matchesPopulationRef(evt.Actor, name, id) || matchesPopulationRef(evt.Target, name, id) {
			if npc.Role != "" && (strings.Contains(evt.Type, npc.Role) || roleRelevantEventType(evt.Type, npc.Role)) {
				att.Score += 0.2
			}
			for _, hook := range npc.Hooks {
				if hook != "" {
					payloadStr := fmt.Sprintf("%v", evt.Payload)
					if strings.Contains(payloadStr, hook) {
						att.Score += 0.2
						break
					}
				}
			}
		}
	}

	// Relationship network bonus: count relationships involving this NPC
	for key := range state.Relationships {
		parts := strings.Split(key, "-")
		if len(parts) == 2 && (strings.TrimSpace(parts[0]) == name || strings.TrimSpace(parts[1]) == name) {
			att.Score += 0.1
		}
	}

	return att, lastEventID
}

func populationEventStale(evt core.Event) bool {
	if evt.CreatedAt.IsZero() {
		return false
	}
	return time.Since(evt.CreatedAt) > populationAttentionEventWindow
}

// roleRelevantEventType checks if an event type is relevant to a given role.
func roleRelevantEventType(eventType, role string) bool {
	role = strings.ToLower(role)
	switch eventType {
	case "pressure", "faction_pressure", "faction_conflict":
		return role == "leader" || role == "guard" || role == "spy"
	case "trade", "market_shift":
		return role == "merchant" || role == "trader"
	case "conflict", "threaten", "attack":
		return role == "guard" || role == "soldier" || role == "fighter"
	case "heal", "aid":
		return role == "healer" || role == "doctor"
	case "speak", "talk", "rumor":
		return role == "informant" || role == "bard"
	}
	return false
}

func mentionsPopulation(evt core.Event, name string) bool {
	if name == "" {
		return false
	}
	content, _ := evt.Payload["content"].(string)
	if content == "" {
		content, _ = evt.Payload["description"].(string)
	}
	return content != "" && strings.Contains(content, name)
}

func matchesPopulationRef(value, name, id string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	return value == name || (id != "" && value == id)
}

func worldPressureTouchesNPC(evt core.Event, npc core.BackgroundNPC) bool {
	if evt.Type != "world_pressure" {
		return false
	}
	target := strings.TrimSpace(evt.Target)
	if target != "" {
		if target == strings.TrimSpace(npc.Faction) || target == strings.TrimSpace(npc.Location) || matchesPopulationRef(target, npc.Name, npc.ID) {
			return true
		}
	}
	if payloadTarget, _ := evt.Payload["target"].(string); payloadTarget != "" {
		payloadTarget = strings.TrimSpace(payloadTarget)
		if payloadTarget == strings.TrimSpace(npc.Faction) || payloadTarget == strings.TrimSpace(npc.Location) || matchesPopulationRef(payloadTarget, npc.Name, npc.ID) {
			return true
		}
	}
	return false
}

func ensureIdentityCore(npc core.BackgroundNPC) (string, core.IdentityCoreConfig) {
	id := defaultPopulationID(npc.ID, npc.Name) + "_core"
	immutable := append([]string(nil), npc.Traits...)
	if len(immutable) == 0 && npc.Role != "" {
		immutable = append(immutable, npc.Role)
	}
	drives := append([]string(nil), npc.Hooks...)
	if len(drives) == 0 && npc.Faction != "" {
		drives = append(drives, "维持在"+npc.Faction+"中的位置")
	}
	return id, core.IdentityCoreConfig{
		ID:          id,
		Name:        npc.Name,
		Immutable:   immutable,
		Adaptive:    map[string]float64{"trust": 3, "fear": 2},
		SpeechHints: append([]string(nil), npc.Traits...),
		Drives:      drives,
	}
}

func promotionStatus(score float64, policy core.PromotionPolicy) string {
	if score >= policy.MajorThreshold {
		return "major"
	}
	return "promoted"
}

func populationKey(id, name string) string {
	return defaultPopulationID(id, name)
}

func defaultPopulationID(id, name string) string {
	id = strings.TrimSpace(id)
	if id != "" {
		return id
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "npc"
	}
	name = strings.ToLower(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('_')
		default:
			b.WriteString(fmt.Sprintf("%x", r))
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "npc"
	}
	return out
}

func populationAttentionEqual(a, b core.PopulationAttention) bool {
	return a.DirectInteractions == b.DirectInteractions &&
		a.Mentions == b.Mentions &&
		a.SharedEvents == b.SharedEvents &&
		a.SceneCarryover == b.SceneCarryover &&
		a.RelationshipDelta == b.RelationshipDelta &&
		a.Score == b.Score
}

func cloneAdaptive(src map[string]float64) map[string]float64 {
	if len(src) == 0 {
		return map[string]float64{"trust": 3, "fear": 2}
	}
	out := make(map[string]float64, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func growthSummary(att core.PopulationAttention) string {
	parts := make([]string, 0, 5)
	if att.DirectInteractions > 0 {
		parts = append(parts, fmt.Sprintf("互动%d", att.DirectInteractions))
	}
	if att.Mentions > 0 {
		parts = append(parts, fmt.Sprintf("提及%d", att.Mentions))
	}
	if att.SharedEvents > 0 {
		parts = append(parts, fmt.Sprintf("卷入事件%d", att.SharedEvents))
	}
	if att.RelationshipDelta > 0 {
		parts = append(parts, fmt.Sprintf("关系漂移%.2f", att.RelationshipDelta))
	}
	if att.SceneCarryover > 0 {
		parts = append(parts, "当前场景在场")
	}
	if len(parts) == 0 {
		return "尚未被世界卷入"
	}
	return strings.Join(parts, " · ")
}

func summarizeAdaptiveShift(before, after map[string]float64) string {
	keys := []string{"trust", "fear", "intimacy"}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		bv := before[key]
		av := after[key]
		if bv == av {
			continue
		}
		delta := av - bv
		parts = append(parts, fmt.Sprintf("%s%+.2f", key, delta))
	}
	if len(parts) == 0 {
		return "adaptive 无明显变化"
	}
	return strings.Join(parts, " · ")
}

func buildPopulationHistory(eventList []core.Event, name string) []core.PopulationGrowthEvent {
	history := make([]core.PopulationGrowthEvent, 0, 4)
	for i := len(eventList) - 1; i >= 0 && len(history) < 4; i-- {
		evt := eventList[i]
		if strings.TrimSpace(evt.Target) != name {
			continue
		}
		switch evt.Type {
		case "population_promoted":
			history = append(history, core.PopulationGrowthEvent{
				EventID:   evt.ID,
				Type:      evt.Type,
				Summary:   fmt.Sprintf("晋升为%s，score %.2f", safePayloadString(evt.Payload["status"], "promoted"), numberPayload(evt.Payload["score"])),
				CreatedAt: evt.CreatedAt,
			})
		case "population_identity_shift":
			history = append(history, core.PopulationGrowthEvent{
				EventID:   evt.ID,
				Type:      evt.Type,
				Summary:   safePayloadString(evt.Payload["summary"], "adaptive 漂移"),
				Adaptive:  payloadAdaptive(evt.Payload["adaptive"]),
				CreatedAt: evt.CreatedAt,
			})
		case "population_demoted":
			history = append(history, core.PopulationGrowthEvent{
				EventID:   evt.ID,
				Type:      evt.Type,
				Summary:   fmt.Sprintf("降级回背景人口，score %.2f", numberPayload(evt.Payload["score"])),
				CreatedAt: evt.CreatedAt,
			})
		}
	}
	return history
}

func payloadAdaptive(value interface{}) map[string]float64 {
	raw, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]float64, len(raw))
	for k, v := range raw {
		out[k] = numberPayload(v)
	}
	return out
}

func numberPayload(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return 0
	}
}

func safePayloadString(v interface{}, fallback string) string {
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func evolveIdentityCore(coreCfg core.IdentityCoreConfig, npc core.BackgroundNPC, eventList []core.Event) core.IdentityCoreConfig {
	evolved := coreCfg
	evolved.Adaptive = cloneAdaptive(evolved.Adaptive)
	name := strings.TrimSpace(npc.Name)
	id := strings.TrimSpace(npc.ID)

	// Ensure all base traits exist
	ensureTrait(evolved.Adaptive, "trust", 3.0)
	ensureTrait(evolved.Adaptive, "fear", 2.0)
	ensureTrait(evolved.Adaptive, "intimacy", 2.0)
	ensureTrait(evolved.Adaptive, "respect", 3.0)
	ensureTrait(evolved.Adaptive, "loyalty", 2.5)
	ensureTrait(evolved.Adaptive, "curiosity", 3.0)
	ensureTrait(evolved.Adaptive, "aggression", 2.0)

	trustDelta := 0.0
	fearDelta := 0.0
	intimacyDelta := 0.0
	respectDelta := 0.0
	loyaltyDelta := 0.0
	curiosityDelta := 0.0
	aggressionDelta := 0.0

	for _, evt := range eventList {
		if !matchesPopulationRef(evt.Actor, name, id) && !matchesPopulationRef(evt.Target, name, id) {
			continue
		}
		switch evt.Type {
		case "trust_change":
			if delta, ok := evt.Payload["delta"].(float64); ok {
				trustDelta += delta * 0.6
				loyaltyDelta += delta * 0.3
			}
		case "fear_change":
			if delta, ok := evt.Payload["delta"].(float64); ok {
				fearDelta += delta * 0.8
				trustDelta -= math.Abs(delta) * 0.2
				aggressionDelta += math.Abs(delta) * 0.15
			}
		case "intimacy_change":
			if delta, ok := evt.Payload["delta"].(float64); ok {
				intimacyDelta += delta * 0.7
				loyaltyDelta += delta * 0.2
			}
		case "attack", "threat", "threaten":
			fearDelta += 0.4
			aggressionDelta += 0.3
			trustDelta -= 0.15
		case "dialogue", "negotiation", "speak", "talk":
			trustDelta += 0.05
			curiosityDelta += 0.03
		case "observe", "watch":
			curiosityDelta += 0.1
		case "hide", "flee":
			fearDelta += 0.2
			aggressionDelta -= 0.1
		case "faction_conflict", "faction_pressure":
			loyaltyDelta += 0.15
			fearDelta += 0.1
		case "pressure":
			fearDelta += 0.1
			curiosityDelta += 0.05
		}

		// Role-based trait amplification
		role := strings.ToLower(npc.Role)
		switch role {
		case "leader", "captain":
			if evt.Type == "faction_conflict" || evt.Type == "faction_pressure" {
				loyaltyDelta += 0.1
				respectDelta += 0.05
			}
		case "guard", "soldier":
			if evt.Type == "attack" || evt.Type == "threaten" {
				aggressionDelta += 0.1
				respectDelta += 0.05
			}
		case "spy", "informant":
			if evt.Type == "observe" || evt.Type == "dialogue" {
				curiosityDelta += 0.1
			}
		case "healer", "doctor":
			if evt.Type == "aid" || evt.Type == "heal" {
				intimacyDelta += 0.1
				trustDelta += 0.1
			}
		}
	}

	// Slow variable: traits drift gradually rather than jumping
	// Apply only a fraction of the delta per tick to simulate gradual personality change
	slowFactor := 0.5
	evolved.Adaptive["trust"] = clampAdaptive(evolved.Adaptive["trust"] + trustDelta*slowFactor)
	evolved.Adaptive["fear"] = clampAdaptive(evolved.Adaptive["fear"] + fearDelta*slowFactor)
	evolved.Adaptive["intimacy"] = clampAdaptive(evolved.Adaptive["intimacy"] + intimacyDelta*slowFactor)
	evolved.Adaptive["respect"] = clampAdaptive(evolved.Adaptive["respect"] + respectDelta*slowFactor)
	evolved.Adaptive["loyalty"] = clampAdaptive(evolved.Adaptive["loyalty"] + loyaltyDelta*slowFactor)
	evolved.Adaptive["curiosity"] = clampAdaptive(evolved.Adaptive["curiosity"] + curiosityDelta*slowFactor)
	evolved.Adaptive["aggression"] = clampAdaptive(evolved.Adaptive["aggression"] + aggressionDelta*slowFactor)

	if len(evolved.Drives) == 0 && len(npc.Hooks) > 0 {
		evolved.Drives = append([]string(nil), npc.Hooks...)
	}
	if len(evolved.SpeechHints) == 0 && len(npc.Traits) > 0 {
		evolved.SpeechHints = append([]string(nil), npc.Traits...)
	}
	return evolved
}

func identityCoreEqual(a, b core.IdentityCoreConfig) bool {
	if a.ID != b.ID || a.Name != b.Name || len(a.Immutable) != len(b.Immutable) || len(a.SpeechHints) != len(b.SpeechHints) || len(a.Drives) != len(b.Drives) || len(a.Adaptive) != len(b.Adaptive) {
		return false
	}
	for i := range a.Immutable {
		if a.Immutable[i] != b.Immutable[i] {
			return false
		}
	}
	for i := range a.SpeechHints {
		if a.SpeechHints[i] != b.SpeechHints[i] {
			return false
		}
	}
	for i := range a.Drives {
		if a.Drives[i] != b.Drives[i] {
			return false
		}
	}
	for k, v := range a.Adaptive {
		if b.Adaptive[k] != v {
			return false
		}
	}
	return true
}

func clampAdaptive(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 10 {
		return 10
	}
	return math.Round(v*100) / 100
}

func ensureTrait(adaptive map[string]float64, trait string, defaultVal float64) {
	if _, ok := adaptive[trait]; !ok {
		adaptive[trait] = defaultVal
	}
}
