package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"corerp/internal/agents"
	"corerp/internal/auth"
	"corerp/internal/core"
	"corerp/internal/events"
	"corerp/internal/llm"
	"corerp/internal/memory"
	"corerp/internal/narrative"
	"corerp/internal/runtime"
)

var _ RuntimeEngine = (*runtime.Engine)(nil)

func TestAPIContractCanonicalSchemasExcludeLegacyFocusMirrors(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "api-contract.yaml"))
	if err != nil {
		t.Fatalf("read api contract: %v", err)
	}
	contract := string(data)
	schemas := []string{
		"MemorySnapshot",
		"SaveSlot",
		"ScenarioPreset",
		"CharacterConfig",
		"RuntimeInstancePayload",
		"ExperimentSnapshot",
		"ExperimentReplayPayload",
		"ExperimentReplayEvidencePayload",
		"TurnTrace",
	}
	for _, schema := range schemas {
		block := apiContractSchemaBlock(t, contract, schema)
		for _, legacy := range []string{"character:", "active_character:", "loaded_characters:"} {
			if strings.Contains(block, legacy) {
				t.Fatalf("schema %s unexpectedly exposes legacy focus mirror %q:\n%s", schema, legacy, block)
			}
		}
	}
}

func apiContractSchemaBlock(t *testing.T, contract, schema string) string {
	t.Helper()
	marker := "\n    " + schema + ":\n"
	start := strings.Index(contract, marker)
	if start < 0 {
		t.Fatalf("schema %s not found", schema)
	}
	start += 1
	rest := contract[start+len("    "+schema+":\n"):]
	next := strings.Index(rest, "\n    ")
	if next < 0 {
		return contract[start:]
	}
	return contract[start : start+len("    "+schema+":\n")+next]
}

type mockResolver struct {
	defaultID  string
	engines    map[string]RuntimeEngine
	stopped    map[string]bool
	lastCreate struct {
		sourceID       string
		id             string
		label          string
		focusCharacter string
	}
}

type realRuntimeResolver struct {
	manager *runtime.Manager
}

func (r realRuntimeResolver) DefaultInstanceID() string {
	return r.manager.DefaultID()
}

func (r realRuntimeResolver) ResolveInstance(id string) (RuntimeEngine, error) {
	return r.manager.Resolve(id)
}

func (r realRuntimeResolver) ListInstances() []core.RuntimeInstanceSummary {
	return r.manager.List()
}

func (r realRuntimeResolver) InstanceStatus(id string) (core.RuntimeInstanceSummary, error) {
	return r.manager.Status(id)
}

func (r realRuntimeResolver) SetDefaultInstance(id string) error {
	return r.manager.SetDefault(id)
}

func (r realRuntimeResolver) StopInstance(id string) (core.RuntimeInstanceSummary, error) {
	return r.manager.Stop(id)
}

func (r realRuntimeResolver) DeleteInstance(id string) error {
	return r.manager.Delete(id)
}

func (r realRuntimeResolver) CreateInstance(sourceID, id, label, focusCharacter string) (core.RuntimeInstanceSummary, error) {
	return r.manager.CreateFrom(sourceID, id, label, focusCharacter)
}

func (r *mockResolver) DefaultInstanceID() string { return r.defaultID }
func (r *mockResolver) ResolveInstance(id string) (RuntimeEngine, error) {
	if id == "" {
		id = r.defaultID
	}
	engine, ok := r.engines[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errInstanceNotFound, id)
	}
	return engine, nil
}
func (r *mockResolver) ListInstances() []core.RuntimeInstanceSummary {
	var out []core.RuntimeInstanceSummary
	for id, engine := range r.engines {
		summary := engine.InstanceSummary()
		summary.ID = id
		summary.IsDefault = id == r.defaultID
		if r.stopped[id] {
			summary.Status = "stopped"
		}
		out = append(out, summary)
	}
	return out
}
func (r *mockResolver) InstanceStatus(id string) (core.RuntimeInstanceSummary, error) {
	if id == "" {
		id = r.defaultID
	}
	engine, ok := r.engines[id]
	if !ok {
		return core.RuntimeInstanceSummary{}, fmt.Errorf("%w: %s", errInstanceNotFound, id)
	}
	summary := engine.InstanceSummary()
	summary.ID = id
	summary.IsDefault = id == r.defaultID
	if r.stopped[id] {
		summary.Status = "stopped"
	}
	return summary, nil
}
func (r *mockResolver) SetDefaultInstance(id string) error {
	if _, ok := r.engines[id]; !ok {
		return fmt.Errorf("%w: %s", errInstanceNotFound, id)
	}
	r.defaultID = id
	return nil
}
func (r *mockResolver) StopInstance(id string) (core.RuntimeInstanceSummary, error) {
	if id == "" {
		id = r.defaultID
	}
	if _, ok := r.engines[id]; !ok {
		return core.RuntimeInstanceSummary{}, fmt.Errorf("%w: %s", errInstanceNotFound, id)
	}
	if r.stopped == nil {
		r.stopped = map[string]bool{}
	}
	r.stopped[id] = true
	return r.InstanceStatus(id)
}
func (r *mockResolver) DeleteInstance(id string) error {
	if _, ok := r.engines[id]; !ok {
		return fmt.Errorf("%w: %s", errInstanceNotFound, id)
	}
	if len(r.engines) == 1 {
		return fmt.Errorf("%w: cannot delete the only instance", errInstanceConflict)
	}
	if r.defaultID == id {
		return fmt.Errorf("%w: cannot delete default instance: set another default first", errInstanceConflict)
	}
	delete(r.engines, id)
	delete(r.stopped, id)
	if r.defaultID == id {
		r.defaultID = ""
	}
	return nil
}
func (r *mockResolver) CreateInstance(sourceID, id, label, focusCharacter string) (core.RuntimeInstanceSummary, error) {
	if id == "" {
		return core.RuntimeInstanceSummary{}, fmt.Errorf("instance id required")
	}
	r.lastCreate.sourceID = sourceID
	r.lastCreate.id = id
	r.lastCreate.label = label
	r.lastCreate.focusCharacter = focusCharacter
	name := focusCharacter
	if name == "" {
		name = "Anya"
	}
	engine := &mockEngine{instanceID: id, name: name, state: core.WorldState{}}
	r.engines[id] = engine
	return engine.InstanceSummary(), nil
}

// mockEngine satisfies RuntimeEngine for smoke testing.
type mockEngine struct {
	name                    string
	instanceID              string
	state                   core.WorldState
	tickCount               int
	dialogue                []core.Message
	player                  core.PlayerRole
	world                   core.WorldConfig
	worldStructure          core.WorldStructureConfig
	scenes                  core.SceneConfigList
	facts                   core.CanonFactsConfig
	population              core.PopulationConfig
	populationInsights      core.PopulationInsights
	director                core.DirectorConfig
	plan                    core.DirectorPlan
	trace                   core.TurnTrace
	traces                  []core.TurnTrace
	memorySnapshot          *core.MemorySnapshot
	experimentReports       []core.ExperimentReport
	quarantine              []core.Event
	pending                 []core.PendingFact
	causalityChain          interface{}
	causalityNarrativeChain interface{}
	causalitySummary        string
	causalityNarrativeSum   string
	participantDetails      []core.ParticipantSummary
	sceneParticipants       []string
	loadedCharacters        []string
	loadedCheckpoints       []string
}

func (m *mockEngine) ProcessTurn(input string) (<-chan string, error) {
	ch := make(chan string, 1)
	ch <- "mock response"
	close(ch)
	return ch, nil
}
func (m *mockEngine) GetInstanceID() string {
	if m.instanceID == "" {
		m.instanceID = "default"
	}
	return m.instanceID
}
func (m *mockEngine) InstanceSummary() core.RuntimeInstanceSummary {
	details := m.GetSceneParticipantDetails()
	participants := m.GetSceneParticipants()
	return core.RuntimeInstanceSummary{
		ID:                 m.GetInstanceID(),
		Label:              "test",
		WorldName:          "test-world",
		FocusCharacter:     m.name,
		Participants:       participants,
		ParticipantDetails: details,
		Status:             "running",
	}
}
func (m *mockEngine) GetState() core.WorldState { return m.state }
func (m *mockEngine) GetCharacter() (core.Character, bool) {
	return core.Character{Identity: core.IdentityEnvelope{Name: m.name}}, true
}
func (m *mockEngine) GetFocusDefinition() (core.Character, bool) {
	return m.GetCharacter()
}
func (m *mockEngine) GetPlayerRole() core.PlayerRole { return m.player }
func (m *mockEngine) UpdatePlayerRole(role core.PlayerRole) (core.PlayerRole, error) {
	m.player = role
	if m.player.Name == "" {
		m.player.Name = "玩家"
	}
	return m.player, nil
}
func (m *mockEngine) GetFocusDefinitionConfig(name string) (core.CharacterConfig, error) {
	return core.CharacterConfig{
		FocusCharacter: m.name,
		Path:           "characters/test.yml",
		WorldPath:      "worlds/test.yml",
		Card:           core.Character{Identity: core.IdentityEnvelope{Name: m.name}},
	}, nil
}
func (m *mockEngine) UpdateFocusDefinitionConfig(name string, card core.Character) (core.CharacterConfig, error) {
	m.name = card.Identity.Name
	return core.CharacterConfig{
		FocusCharacter: m.name,
		Path:           "characters/test.yml",
		WorldPath:      "worlds/test.yml",
		Card:           card,
	}, nil
}
func (m *mockEngine) GetWorldConfig() (core.WorldConfig, error) {
	if m.world.Name == "" {
		m.world = core.WorldConfig{Name: "test-world", CoreRules: "rules", Path: "worlds/test", Format: "world_dir"}
	}
	return m.world, nil
}
func (m *mockEngine) UpdateWorldConfig(cfg core.WorldConfig) (core.WorldConfig, error) {
	m.world = cfg
	if m.world.Path == "" {
		m.world.Path = "worlds/test"
	}
	return m.world, nil
}
func (m *mockEngine) GetWorldStructureConfig() (core.WorldStructureConfig, error) {
	if m.worldStructure.Path == "" {
		m.worldStructure = core.WorldStructureConfig{
			Path: "worlds/test",
			Ruleset: core.WorldRulesetConfig{
				Path: "worlds/test/world/ruleset.yml",
				Rules: []core.WorldRule{{
					ID:      "test_rule",
					Title:   "测试规则",
					Summary: "默认规则",
				}},
			},
			Seed: core.WorldSeedConfig{
				Path:             "worlds/test/world/seed.yml",
				Premise:          "默认设定",
				CurrentSituation: "默认局势",
				Variables:        map[string]interface{}{},
			},
			Factions:  []core.WorldFactionConfig{},
			Locations: []core.WorldLocationConfig{},
			Pressures: []core.WorldPressureConfig{},
		}
	}
	return m.worldStructure, nil
}
func (m *mockEngine) UpdateWorldStructureConfig(cfg core.WorldStructureConfig) (core.WorldStructureConfig, error) {
	m.worldStructure = cfg
	if m.worldStructure.Path == "" {
		m.worldStructure.Path = "worlds/test"
	}
	if m.worldStructure.Ruleset.Path == "" {
		m.worldStructure.Ruleset.Path = "worlds/test/world/ruleset.yml"
	}
	if m.worldStructure.Seed.Path == "" {
		m.worldStructure.Seed.Path = "worlds/test/world/seed.yml"
	}
	if m.worldStructure.Seed.Variables == nil {
		m.worldStructure.Seed.Variables = map[string]interface{}{}
	}
	return m.worldStructure, nil
}
func (m *mockEngine) ListSceneConfigs() (core.SceneConfigList, error) {
	if len(m.scenes.Scenes) == 0 {
		m.scenes = core.SceneConfigList{
			Selected: "default",
			Scenes: []core.SceneConfig{{
				Name:  "default",
				Path:  "worlds/test/scenes/default.yml",
				Scene: core.SceneState{Location: "test", TimeOfDay: "day"},
			}},
		}
	}
	return m.scenes, nil
}
func (m *mockEngine) UpdateSceneConfig(scene core.SceneConfig) (core.SceneConfig, error) {
	m.scenes.Selected = scene.Name
	m.scenes.Scenes = []core.SceneConfig{scene}
	return scene, nil
}
func (m *mockEngine) GetCanonFactsConfig() (core.CanonFactsConfig, error) {
	if m.facts.Path == "" {
		m.facts = core.CanonFactsConfig{
			Path:  "worlds/test/canon/facts.yml",
			Facts: []core.FactFrame{{Subject: "A", Predicate: "是", Object: "B", Confidence: 1}},
		}
	}
	return m.facts, nil
}
func (m *mockEngine) UpdateCanonFactsConfig(cfg core.CanonFactsConfig) (core.CanonFactsConfig, error) {
	m.facts = cfg
	if m.facts.Path == "" {
		m.facts.Path = "worlds/test/canon/facts.yml"
	}
	return m.facts, nil
}
func (m *mockEngine) GetPopulationConfig() (core.PopulationConfig, error) {
	if m.population.Path == "" {
		m.population = core.PopulationConfig{
			Path: "worlds/test/population",
			BackgroundNPCs: []core.BackgroundNPC{{
				ID:       "tea_vendor",
				Name:     "茶摊老板",
				Role:     "商贩",
				Location: "镇口",
			}},
			Policy: core.PromotionPolicy{PromoteThreshold: 10, MajorThreshold: 25},
		}
	}
	return m.population, nil
}
func (m *mockEngine) GetPopulationInsights() (core.PopulationInsights, error) {
	if m.populationInsights.Path == "" {
		m.populationInsights = core.PopulationInsights{
			Path:      "worlds/test/population",
			WorldPath: "worlds/test",
			Promoted: []core.PopulationCharacterInsight{{
				ID:            "tea_vendor",
				Name:          "茶摊老板",
				Status:        "promoted",
				IdentityCore:  "tea_vendor_core",
				Attention:     core.PopulationAttention{Score: 14, DirectInteractions: 2, Mentions: 1},
				Adaptive:      map[string]float64{"trust": 4.2, "fear": 2.1},
				GrowthSummary: "互动2 · 提及1",
			}},
			Background: []core.PopulationCharacterInsight{{
				ID:            "gate_guard",
				Name:          "守门人",
				Attention:     core.PopulationAttention{Score: 3, Mentions: 1},
				GrowthSummary: "提及1",
			}},
		}
	}
	return m.populationInsights, nil
}
func (m *mockEngine) UpdatePopulationConfig(cfg core.PopulationConfig) (core.PopulationConfig, error) {
	m.population = cfg
	if m.population.Path == "" {
		m.population.Path = "worlds/test/population"
	}
	return m.population, nil
}
func (m *mockEngine) GetDirectorConfig() core.DirectorConfig {
	if m.director.Mode == "" {
		m.director = core.DirectorConfig{Mode: "manual", MaxSpeakers: 1}
	}
	return m.director
}
func (m *mockEngine) UpdateDirectorConfig(cfg core.DirectorConfig) core.DirectorConfig {
	m.director = cfg
	return m.director
}
func (m *mockEngine) GetDirectorPlan() core.DirectorPlan {
	if len(m.plan.Selected) == 0 {
		m.plan = core.DirectorPlan{Mode: m.GetDirectorConfig().Mode, Selected: []string{m.name}}
	}
	return m.plan
}
func (m *mockEngine) GetLatestTrace() (core.TurnTrace, bool) {
	if m.trace.Turn == 0 {
		m.trace = core.TurnTrace{Turn: 1, FocusCharacter: m.name, UserInput: "test", Narrative: "mock narrative"}
	}
	return m.trace, true
}
func (m *mockEngine) ListTurnTraces(limit int) []core.TurnTrace {
	if len(m.traces) == 0 {
		m.traces = []core.TurnTrace{
			{Turn: 2, FocusCharacter: m.name, UserInput: "later"},
			{Turn: 1, FocusCharacter: m.name, UserInput: "earlier"},
		}
	}
	if limit <= 0 || limit >= len(m.traces) {
		return m.traces
	}
	return m.traces[:limit]
}
func (m *mockEngine) GetTraceByTurn(turn int) (core.TurnTrace, bool) {
	trace, _ := m.GetLatestTrace()
	trace.Turn = turn
	return trace, true
}
func (m *mockEngine) ListQuarantineEvents(character string, limit int) ([]core.Event, error) {
	if len(m.quarantine) == 0 {
		m.quarantine = []core.Event{{ID: "q1", Type: "fact_extracted", Actor: "Anya", Canonical: false}}
	}
	return m.quarantine, nil
}
func (m *mockEngine) PromoteQuarantineEvent(eventID string) error { return nil }
func (m *mockEngine) RejectQuarantineEvent(eventID string) error  { return nil }
func (m *mockEngine) ListPendingFacts(character string, limit int) ([]core.PendingFact, map[string]interface{}, error) {
	if len(m.pending) == 0 {
		m.pending = []core.PendingFact{{ID: "p1", FocusCharacter: "Anya", Subject: "V", Predicate: "身份", Object: "佣兵", Source: "llm_extracted", Confidence: 0.4}}
	}
	return m.pending, map[string]interface{}{"pending_total": len(m.pending)}, nil
}
func (m *mockEngine) ConfirmPendingFact(eventID string) error { return nil }
func (m *mockEngine) DeletePendingFact(eventID string) error  { return nil }
func (m *mockEngine) PromotePendingFact(eventID string) error { return nil }
func (m *mockEngine) GetFocusCharacter() string               { return m.name }
func (m *mockEngine) GetLoadedCharacters() []string {
	if len(m.loadedCharacters) > 0 {
		return append([]string(nil), m.loadedCharacters...)
	}
	return []string{m.name}
}
func (m *mockEngine) GetSceneParticipants() []string {
	if m.sceneParticipants != nil {
		return append([]string(nil), m.sceneParticipants...)
	}
	return []string{m.name}
}
func (m *mockEngine) GetSceneParticipantDetails() []core.ParticipantSummary {
	if len(m.participantDetails) > 0 {
		return m.participantDetails
	}
	return []core.ParticipantSummary{{Name: m.name, Kind: "persona", Source: "character_definition", Loaded: true, Switchable: true, Present: true, Focus: true}}
}
func (m *mockEngine) SwitchFocusCharacter(name string) error { m.name = name; return nil }
func (m *mockEngine) EnterWorld(path string) (core.ScenarioPreset, error) {
	m.name = "entered-character"
	m.world.Path = path
	m.world.Name = "entered-world"
	return core.ScenarioPreset{Name: "opening", FocusCharacter: m.name, Branch: "main"}, nil
}
func (m *mockEngine) GetWorldName() string { return "test-world" }
func (m *mockEngine) GetWorldPaths() map[string]string {
	cfg, _ := m.GetWorldConfig()
	return map[string]string{m.name: cfg.Path}
}
func (m *mockEngine) GetFocusMemorySnapshot(character string, factLimit, episodicLimit, dialogueLimit int) (core.MemorySnapshot, error) {
	if m.memorySnapshot != nil {
		return *m.memorySnapshot, nil
	}
	return core.MemorySnapshot{
		FocusCharacter: m.name,
		WorkingMemory:  "working",
		Dialogue:       m.dialogue,
	}, nil
}
func (m *mockEngine) ListSaveSlots() ([]core.SaveSlot, error) {
	return []core.SaveSlot{{Name: "slot-1", FocusCharacter: m.name, Branch: "main"}}, nil
}
func (m *mockEngine) CreateSaveSlot(name, branch, note string) (core.SaveSlot, error) {
	return core.SaveSlot{Name: name, FocusCharacter: m.name, Branch: branch, Note: note}, nil
}
func (m *mockEngine) LoadSaveSlot(name string) (core.SaveSlot, error) {
	return core.SaveSlot{Name: name, FocusCharacter: m.name, Branch: "main"}, nil
}
func (m *mockEngine) CompareSaveSlots(saveA, saveB string) (core.WorldStateDiff, error) {
	return core.WorldStateDiff{SaveA: saveA, SaveB: saveB, Tension: &core.StateDiffEntry{A: 0.1, B: 0.2}}, nil
}
func (m *mockEngine) ListCheckpoints() ([]core.SaveSlot, error) {
	return []core.SaveSlot{{Name: "cp-1", FocusCharacter: m.name, Branch: "main"}}, nil
}
func (m *mockEngine) CreateCheckpoint(name, branch, note string) (core.SaveSlot, error) {
	return core.SaveSlot{Name: name, FocusCharacter: m.name, Branch: branch, Note: note}, nil
}
func (m *mockEngine) LoadCheckpoint(name string) (core.SaveSlot, error) {
	m.loadedCheckpoints = append(m.loadedCheckpoints, name)
	return core.SaveSlot{Name: name, FocusCharacter: m.name, Branch: "main"}, nil
}
func (m *mockEngine) ListScenarioPresets() ([]core.ScenarioPreset, error) {
	return []core.ScenarioPreset{{Name: "preset-1", FocusCharacter: m.name, Branch: "main"}}, nil
}
func (m *mockEngine) CreateScenarioPreset(name, branch, note string) (core.ScenarioPreset, error) {
	return core.ScenarioPreset{Name: name, FocusCharacter: m.name, Branch: branch, Note: note}, nil
}
func (m *mockEngine) ApplyScenarioPreset(name string) (core.ScenarioPreset, error) {
	return core.ScenarioPreset{Name: name, FocusCharacter: m.name, Branch: "main"}, nil
}
func (m *mockEngine) ListExperimentReports() ([]core.ExperimentReport, error) {
	out := append([]core.ExperimentReport(nil), m.experimentReports...)
	for i := range out {
		out[i] = normalizeExperimentReportCompatibility(out[i])
	}
	return out, nil
}
func (m *mockEngine) CreateExperimentReport(report core.ExperimentReport) (core.ExperimentReport, error) {
	report = normalizeExperimentReportCompatibility(report)
	m.experimentReports = append(m.experimentReports, report)
	return report, nil
}
func (m *mockEngine) GetNPCActions(name string, since int) []agents.NPCActionLog { return nil }
func (m *mockEngine) GetCausalityChain(id string, d int) (interface{}, error) {
	if m.causalityChain != nil {
		return m.causalityChain, nil
	}
	return []string{}, nil
}
func (m *mockEngine) GetCausalityChainNarrative(id string, d int) (interface{}, error) {
	if m.causalityNarrativeChain != nil {
		return m.causalityNarrativeChain, nil
	}
	return []string{}, nil
}
func (m *mockEngine) GetCausalitySummary(id string, d int) (string, error) {
	if m.causalitySummary != "" {
		return m.causalitySummary, nil
	}
	return "summary", nil
}
func (m *mockEngine) GetCausalitySummaryNarrative(id string, d int) (string, error) {
	if m.causalityNarrativeSum != "" {
		return m.causalityNarrativeSum, nil
	}
	return "summary", nil
}
func (m *mockEngine) ReplayTo(id string) (core.WorldState, error)         { return m.state, nil }
func (m *mockEngine) ReplayAtTime(h, min, d int) (core.WorldState, error) { return m.state, nil }
func (m *mockEngine) ForkTimeline(id, branch string) error                { return nil }
func (m *mockEngine) GetTimeline(branch string, limit int) ([]events.EventTimeline, error) {
	return nil, nil
}
func (m *mockEngine) ListBranches() ([]string, error) { return []string{"main"}, nil }
func (m *mockEngine) CompareBranchesDetailed(branchA, branchB string, index int) (core.WorldStateDiff, error) {
	return core.WorldStateDiff{BranchA: branchA, BranchB: branchB, Flags: map[string]core.StateDiffEntry{"x": {A: false, B: true}}}, nil
}
func (m *mockEngine) MergeBranchState(sourceBranch, targetBranch string, mergeFlags, mergeVariables bool) (core.BranchMergeResult, error) {
	return core.BranchMergeResult{SourceBranch: sourceBranch, TargetBranch: targetBranch, FlagsMerged: 1, EventsAppended: 1}, nil
}
func (m *mockEngine) CompressEvents(from, to int) (*narrative.CompressionResult, error) {
	return &narrative.CompressionResult{}, nil
}
func (m *mockEngine) CompressionStats() map[string]interface{}       { return map[string]interface{}{} }
func (m *mockEngine) SwitchLLM(name, endpoint, apiKey, model string) {}
func (m *mockEngine) LLMRoutes() map[string]interface{}              { return map[string]interface{}{} }
func (m *mockEngine) GetDialogueLimit(limit int) []core.Message      { return m.dialogue }
func (m *mockEngine) ResetDialogue()                                 { m.dialogue = nil }
func (m *mockEngine) DebugInfo() map[string]interface{}              { return map[string]interface{}{} }
func (m *mockEngine) SetTension(v float64)                           { m.state.Tension = v }
func (m *mockEngine) QueryActionLog(character string, fired, blocked bool, limit int) []interface{} {
	return nil
}
func (m *mockEngine) ActionLogStats() map[string]interface{} {
	return map[string]interface{}{"total_entries": 0}
}
func (m *mockEngine) TickStatus() map[string]interface{} {
	return map[string]interface{}{"running": true, "tick_count": m.tickCount}
}
func (m *mockEngine) ManualTick() { m.tickCount++ }
func (m *mockEngine) PauseTick()  {}
func (m *mockEngine) ResumeTick() {}

func newTestServer() *Server {
	return NewServer(&mockEngine{instanceID: "default", name: "Anya", state: core.WorldState{
		Scene:         core.SceneState{Location: "test", TimeOfDay: "day", Weather: "clear"},
		Relationships: make(map[string]core.Relationship),
		Variables:     make(map[string]interface{}),
		Flags:         make(map[string]bool),
	}, player: core.PlayerRole{Name: "玩家"}})
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func containsPromotedNPC(items []core.PopulationCharacterInsight, target string) bool {
	for _, item := range items {
		if item.Name == target {
			return true
		}
	}
	return false
}

func findPromotedNPC(items []core.PopulationCharacterInsight, target string) (core.PopulationCharacterInsight, bool) {
	for _, item := range items {
		if item.Name == target {
			return item, true
		}
	}
	return core.PopulationCharacterInsight{}, false
}

func stringifyInterfaceSlice(value interface{}) []string {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, fmt.Sprint(item))
	}
	return out
}

func newRealRuntimeEngineForAPITest(t *testing.T, dbPath, dataDir, worldDir, instanceID, worldName, rules string) *runtime.Engine {
	t.Helper()

	store, err := events.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	memEngine, err := memory.New(dbPath)
	if err != nil {
		t.Fatalf("new memory engine: %v", err)
	}
	t.Cleanup(func() { memEngine.Close() })

	gatekeeper := events.NewGatekeeper(store)
	agentsMgr := agents.NewEnvelopeManager()
	agentsMgr.LoadCharacter("111", core.Character{
		Identity: core.IdentityEnvelope{
			Name:      "111",
			Adaptive:  map[string]float64{"trust": 6},
			Immutable: []string{"cold"},
		},
	})

	charWorlds := map[string]runtime.CharWorld{
		"111": {
			WorldName: worldName,
			CoreRules: rules,
			Scene: core.SceneState{
				Location:    "外城",
				TimeOfDay:   "深夜",
				Weather:     "阴",
				Characters:  []string{"111", "玩家"},
				Description: "API 长窗口验证场景",
			},
		},
	}

	engine, err := runtime.New(
		store,
		gatekeeper,
		memEngine,
		memory.NewDecayEngine(memEngine.DB()),
		agentsMgr,
		llm.NewRouter(llm.NewAdapter("http://127.0.0.1:1/v1", "", "test-model")),
		"111",
		[]string{"111"},
		charWorlds,
	)
	if err != nil {
		t.Fatalf("new runtime engine: %v", err)
	}
	engine.ConfigurePersistence(dataDir, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.SyncActiveWorldContext()
	return engine
}

func newRealWorldRuntimeEngineForAPITest(t *testing.T, dbPath, dataDir, worldDir, instanceID, worldName, rules string, scene core.SceneState) *runtime.Engine {
	t.Helper()

	store, err := events.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	memEngine, err := memory.New(dbPath)
	if err != nil {
		t.Fatalf("new memory engine: %v", err)
	}
	t.Cleanup(func() { memEngine.Close() })

	gatekeeper := events.NewGatekeeper(store)
	agentsMgr := agents.NewEnvelopeManager()
	agentsMgr.LoadCharacter("111", core.Character{
		Identity: core.IdentityEnvelope{
			Name:      "111",
			Adaptive:  map[string]float64{"trust": 6},
			Immutable: []string{"cold"},
		},
	})

	charWorlds := map[string]runtime.CharWorld{
		"111": {
			WorldName: worldName,
			CoreRules: rules,
			Scene:     scene,
		},
	}

	engine, err := runtime.New(
		store,
		gatekeeper,
		memEngine,
		memory.NewDecayEngine(memEngine.DB()),
		agentsMgr,
		llm.NewRouter(llm.NewAdapter("http://127.0.0.1:1/v1", "", "test-model")),
		"111",
		[]string{"111"},
		charWorlds,
	)
	if err != nil {
		t.Fatalf("new runtime engine: %v", err)
	}
	engine.ConfigurePersistence(dataDir, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.SyncActiveWorldContext()
	return engine
}

func writeAPITestWorldBundle(t *testing.T, worldDir, name, rules string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(worldDir, "canon"), 0755); err != nil {
		t.Fatalf("mkdir canon: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(worldDir, "scenes"), 0755); err != nil {
		t.Fatalf("mkdir scenes: %v", err)
	}
	worldYAML := "meta:\n  name: " + name + "\ncore_rules: |\n  " + rules + "\n"
	sceneYAML := "scene:\n" +
		"  location: 外城\n" +
		"  time_of_day: 深夜\n" +
		"  weather: 阴\n" +
		"  present_chars:\n" +
		"    - 111\n" +
		"    - 玩家\n" +
		"  atmosphere: API 长窗口验证场景\n"
	if err := os.WriteFile(filepath.Join(worldDir, "world.yml"), []byte(worldYAML), 0644); err != nil {
		t.Fatalf("write world.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "scenes", "default.yml"), []byte(sceneYAML), 0644); err != nil {
		t.Fatalf("write default scene: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "canon", "facts.yml"), []byte("facts: []\n"), 0644); err != nil {
		t.Fatalf("write facts.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "canon", "ontology.yml"), []byte("ontology:\n  characters: []\n  locations: []\n  factions: []\n  items: []\n  lore: []\n  events: []\n  timelines: []\n  settings: []\n"), 0644); err != nil {
		t.Fatalf("write ontology.yml: %v", err)
	}
}

func copyTestDir(t *testing.T, src, dst string) {
	t.Helper()
	if err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		return out.Chmod(info.Mode())
	}); err != nil {
		t.Fatalf("copy dir %s -> %s: %v", src, dst, err)
	}
}

func initTestLLMStore(t *testing.T) {
	t.Helper()
	if err := llm.InitConfigStore(filepath.Join(t.TempDir(), "llm_configs.json")); err != nil {
		t.Fatalf("init llm store: %v", err)
	}
}

func withRepoRoot(t *testing.T) {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}

func TestStaticIndexSmoke(t *testing.T) {
	withRepoRoot(t)
	auth.Init("")

	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / → %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "CoreRP") {
		t.Fatalf("GET / body missing app shell marker")
	}
}

func TestLoginSmokeAndAuthRedirect(t *testing.T) {
	auth.Init("test-password")
	t.Cleanup(func() { auth.Init("") })

	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	loginReq := httptest.NewRequest("GET", "/login", nil)
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("GET /login → %d, want 200", loginRec.Code)
	}

	req := httptest.NewRequest("GET", "/api/state", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/state without auth → %d, want 401", rec.Code)
	}

	rootReq := httptest.NewRequest("GET", "/", nil)
	rootRec := httptest.NewRecorder()
	mux.ServeHTTP(rootRec, rootReq)

	if rootRec.Code != http.StatusSeeOther {
		t.Fatalf("GET / with auth enabled → %d, want 303", rootRec.Code)
	}
	if got := rootRec.Header().Get("Location"); got != "/login" {
		t.Fatalf("GET / redirect location = %q, want /login", got)
	}
}

func TestRouteWrongMethod(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	tests := []struct {
		path   string
		method string
		want   int
	}{
		{"/api/state", "POST", http.StatusMethodNotAllowed},
		{"/api/health", "POST", http.StatusMethodNotAllowed},
		{"/api/ready", "POST", http.StatusMethodNotAllowed},
		{"/api/version", "POST", http.StatusMethodNotAllowed},
		{"/api/state", "DELETE", http.StatusMethodNotAllowed},
		{"/api/focus-definition", "POST", http.StatusMethodNotAllowed},
		{"/api/character", "POST", http.StatusMethodNotAllowed},
		{"/api/player-role", "DELETE", http.StatusMethodNotAllowed},
		{"/api/instances", "POST", http.StatusMethodNotAllowed},
		{"/api/instances/status", "POST", http.StatusMethodNotAllowed},
		{"/api/focus-definition-config", "DELETE", http.StatusMethodNotAllowed},
		{"/api/character-config", "DELETE", http.StatusMethodNotAllowed},
		{"/api/characters", "POST", http.StatusMethodNotAllowed},
		{"/api/world", "POST", http.StatusMethodNotAllowed},
		{"/api/worlds", "DELETE", http.StatusMethodNotAllowed},
		{"/api/world-config", "DELETE", http.StatusMethodNotAllowed},
		{"/api/world-structure", "DELETE", http.StatusMethodNotAllowed},
		{"/api/population", "DELETE", http.StatusMethodNotAllowed},
		{"/api/population-insights", "POST", http.StatusMethodNotAllowed},
		{"/api/director-config", "DELETE", http.StatusMethodNotAllowed},
		{"/api/trace", "POST", http.StatusMethodNotAllowed},
		{"/api/traces", "POST", http.StatusMethodNotAllowed},
		{"/api/trace/latest", "POST", http.StatusMethodNotAllowed},
		{"/api/scenes", "DELETE", http.StatusMethodNotAllowed},
		{"/api/canon-facts", "DELETE", http.StatusMethodNotAllowed},
		{"/api/quarantine", "POST", http.StatusMethodNotAllowed},
		{"/api/quarantine/promote", "GET", http.StatusMethodNotAllowed},
		{"/api/quarantine/reject", "GET", http.StatusMethodNotAllowed},
		{"/api/pending-facts", "POST", http.StatusMethodNotAllowed},
		{"/api/pending-facts/confirm", "GET", http.StatusMethodNotAllowed},
		{"/api/pending-facts/delete", "GET", http.StatusMethodNotAllowed},
		{"/api/pending-facts/promote", "GET", http.StatusMethodNotAllowed},
		{"/api/instances/stop", "GET", http.StatusMethodNotAllowed},
		{"/api/instances/delete", "GET", http.StatusMethodNotAllowed},
		{"/api/saves/diff", "POST", http.StatusMethodNotAllowed},
		{"/api/checkpoints/load", "GET", http.StatusMethodNotAllowed},
		{"/api/presets/apply", "GET", http.StatusMethodNotAllowed},
		{"/api/experiment-reports/replay", "GET", http.StatusMethodNotAllowed},
		{"/api/experiment-reports/replay-batch", "GET", http.StatusMethodNotAllowed},
		{"/api/experiment-reports/replay-advance", "GET", http.StatusMethodNotAllowed},
		{"/api/proof-audits", "POST", http.StatusMethodNotAllowed},
		{"/api/memory", "POST", http.StatusMethodNotAllowed},
		{"/api/export", "POST", http.StatusMethodNotAllowed},
		{"/api/runtime-audit", "POST", http.StatusMethodNotAllowed},
		{"/api/saves/load", "GET", http.StatusMethodNotAllowed},
		{"/api/npc-actions", "POST", http.StatusMethodNotAllowed},
		{"/api/npc-action-log", "POST", http.StatusMethodNotAllowed},
		{"/api/dialogue", "POST", http.StatusMethodNotAllowed},
		{"/api/branches", "POST", http.StatusMethodNotAllowed},
		{"/api/branches/diff", "POST", http.StatusMethodNotAllowed},
		{"/api/branches/merge", "GET", http.StatusMethodNotAllowed},
		{"/api/timeline", "POST", http.StatusMethodNotAllowed},
		{"/api/compression-stats", "POST", http.StatusMethodNotAllowed},
		{"/api/llm-routes", "POST", http.StatusMethodNotAllowed},
		{"/api/debug/memory", "POST", http.StatusMethodNotAllowed},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != tc.want {
			t.Errorf("%s %s → %d, want %d", tc.method, tc.path, rec.Code, tc.want)
		}
	}
}

func TestRouteValidMethod2xx(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	tests := []struct {
		path   string
		method string
	}{
		{"/api/state", "GET"},
		{"/api/health", "GET"},
		{"/api/ready", "GET"},
		{"/api/version", "GET"},
		{"/api/focus-definition", "GET"},
		{"/api/character", "GET"},
		{"/api/player-role", "GET"},
		{"/api/instances", "GET"},
		{"/api/instances/status", "GET"},
		{"/api/focus-definition-config", "GET"},
		{"/api/character-config", "GET"},
		{"/api/characters", "GET"},
		{"/api/world", "GET"},
		{"/api/worlds", "GET"},
		{"/api/world-config", "GET"},
		{"/api/world-structure", "GET"},
		{"/api/population", "GET"},
		{"/api/population-insights", "GET"},
		{"/api/director-config", "GET"},
		{"/api/trace", "GET"},
		{"/api/traces", "GET"},
		{"/api/trace/latest", "GET"},
		{"/api/scenes", "GET"},
		{"/api/canon-facts", "GET"},
		{"/api/quarantine", "GET"},
		{"/api/pending-facts", "GET"},
		{"/api/memory", "GET"},
		{"/api/export", "GET"},
		{"/api/saves", "GET"},
		{"/api/checkpoints", "GET"},
		{"/api/presets", "GET"},
		{"/api/runtime-audit", "GET"},
		{"/api/experiment-reports", "GET"},
		{"/api/saves/diff?a=slot-1&b=slot-2", "GET"},
		{"/api/npc-actions", "GET"},
		{"/api/npc-action-log", "GET"},
		{"/api/npc-action-log?stats=1", "GET"},
		{"/api/dialogue", "GET"},
		{"/api/branches", "GET"},
		{"/api/branches/diff?a=main&b=alt", "GET"},
		{"/api/timeline", "GET"},
		{"/api/compression-stats", "GET"},
		{"/api/llm-routes", "GET"},
		{"/api/debug/memory", "GET"},
		{"/api/usage", "GET"},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code >= 500 {
			t.Errorf("%s %s → %d (server error)", tc.method, tc.path, rec.Code)
		}
	}
}

func TestStateIncludesInstanceMetadata(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("GET", "/api/state", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/state → %d, want 200", rec.Code)
	}
	body := append([]byte(nil), rec.Body.Bytes()...)

	var payload struct {
		InstanceID string `json:"instance_id"`
		Instance   struct {
			ID                 string                    `json:"id"`
			FocusCharacter     string                    `json:"focus_character"`
			Participants       []string                  `json:"participants"`
			ParticipantDetails []core.ParticipantSummary `json:"participant_details"`
		} `json:"instance"`
		FocusCharacter     string                    `json:"focus_character"`
		Participants       []string                  `json:"participants"`
		ParticipantDetails []core.ParticipantSummary `json:"participant_details"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if payload.InstanceID != "default" {
		t.Fatalf("instance_id = %q, want default", payload.InstanceID)
	}
	if payload.Instance.ID != "default" {
		t.Fatalf("instance.id = %q, want default", payload.Instance.ID)
	}
	if payload.Instance.FocusCharacter != "Anya" {
		t.Fatalf("instance.focus_character = %q, want Anya", payload.Instance.FocusCharacter)
	}
	if payload.FocusCharacter != "Anya" {
		t.Fatalf("focus_character = %q, want Anya", payload.FocusCharacter)
	}
	if len(payload.Participants) != 1 || payload.Participants[0] != "Anya" {
		t.Fatalf("participants = %#v, want [Anya]", payload.Participants)
	}
	if len(payload.Instance.Participants) != 1 || payload.Instance.Participants[0] != "Anya" {
		t.Fatalf("instance.participants = %#v, want [Anya]", payload.Instance.Participants)
	}
	if len(payload.ParticipantDetails) != 1 || payload.ParticipantDetails[0].Name != "Anya" {
		t.Fatalf("participant_details = %#v, want Anya detail", payload.ParticipantDetails)
	}
	if len(payload.Instance.ParticipantDetails) != 1 || payload.Instance.ParticipantDetails[0].Name != "Anya" {
		t.Fatalf("instance.participant_details = %#v, want Anya detail", payload.Instance.ParticipantDetails)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode raw state: %v", err)
	}
	instanceRaw, _ := raw["instance"].(map[string]interface{})
	if _, ok := instanceRaw["active_character"]; ok {
		t.Fatalf("state.instance unexpectedly exposes active_character: %#v", instanceRaw)
	}
	if _, ok := instanceRaw["loaded_characters"]; ok {
		t.Fatalf("state.instance unexpectedly exposes loaded_characters: %#v", instanceRaw)
	}
}

func TestRuntimeAuditAggregatesAuthoringEvidence(t *testing.T) {
	engine := &mockEngine{
		instanceID: "audit-instance",
		name:       "Anya",
		state: core.WorldState{
			Scene:   core.SceneState{Location: "旧街夜市", Description: "审计场景"},
			Tension: 0.73,
		},
		plan: core.DirectorPlan{
			Mode:         "auto_chain",
			Selected:     []string{"蓝姐", "谭叔"},
			WorldSignals: []string{"pressure: missing_rider", "faction: property_union"},
		},
		trace: core.TurnTrace{
			Turn:           7,
			FocusCharacter: "Anya",
			UserInput:      "先说现在为什么乱了",
		},
		traces: []core.TurnTrace{
			{Turn: 7, FocusCharacter: "Anya", UserInput: "先说现在为什么乱了"},
			{Turn: 6, FocusCharacter: "Anya", UserInput: "上一轮"},
		},
		experimentReports: []core.ExperimentReport{{
			Name:              "neon-guard-120t",
			SourceInstanceID:  "audit-instance",
			CompareInstanceID: "audit-compare",
			CurrentCheckpoint: "neon-guard-120t-current",
			CompareCheckpoint: "neon-guard-120t-compare",
			CreatedAt:         time.Date(2026, 5, 27, 18, 0, 0, 0, time.UTC),
		}},
	}
	s := NewServer(engine)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/runtime-audit?trace_limit=1&checkpoint_limit=1&preset_limit=1&report_limit=1&population_limit=1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/runtime-audit = %d body=%s", rec.Code, rec.Body.String())
	}
	body := append([]byte(nil), rec.Body.Bytes()...)

	var payload struct {
		InstanceID string `json:"instance_id"`
		Instance   struct {
			ID                 string                    `json:"id"`
			FocusCharacter     string                    `json:"focus_character"`
			Participants       []string                  `json:"participants"`
			ParticipantDetails []core.ParticipantSummary `json:"participant_details"`
		} `json:"instance"`
		State             core.WorldState         `json:"state"`
		FocusCharacter    string                  `json:"focus_character"`
		RecentTraces      []core.TurnTrace        `json:"recent_traces"`
		LatestTrace       *core.TurnTrace         `json:"latest_trace"`
		Checkpoints       []core.SaveSlot         `json:"checkpoints"`
		Presets           []core.ScenarioPreset   `json:"presets"`
		ExperimentReports []core.ExperimentReport `json:"experiment_reports"`
		Population        core.PopulationInsights `json:"population"`
		AuditSummary      []string                `json:"audit_summary"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode runtime audit: %v", err)
	}
	if payload.InstanceID != "audit-instance" {
		t.Fatalf("instance_id = %q, want audit-instance", payload.InstanceID)
	}
	if payload.Instance.ID != "audit-instance" {
		t.Fatalf("instance.id = %q, want audit-instance", payload.Instance.ID)
	}
	if payload.Instance.FocusCharacter != "Anya" {
		t.Fatalf("instance.focus_character = %q, want Anya", payload.Instance.FocusCharacter)
	}
	if payload.FocusCharacter != "Anya" {
		t.Fatalf("focus_character = %q, want Anya", payload.FocusCharacter)
	}
	if payload.State.Scene.Location != "旧街夜市" {
		t.Fatalf("state.scene.location = %q, want 旧街夜市", payload.State.Scene.Location)
	}
	if len(payload.RecentTraces) != 1 || payload.RecentTraces[0].Turn != 7 {
		t.Fatalf("recent_traces = %#v, want top trace only", payload.RecentTraces)
	}
	if payload.LatestTrace == nil || payload.LatestTrace.Turn != 7 {
		t.Fatalf("latest_trace = %#v, want turn 7", payload.LatestTrace)
	}
	if len(payload.Checkpoints) != 1 || payload.Checkpoints[0].Name != "cp-1" {
		t.Fatalf("checkpoints = %#v, want capped checkpoint list", payload.Checkpoints)
	}
	if len(payload.Presets) != 1 || payload.Presets[0].Name != "preset-1" {
		t.Fatalf("presets = %#v, want capped preset list", payload.Presets)
	}
	if len(payload.ExperimentReports) != 1 || payload.ExperimentReports[0].Name != "neon-guard-120t" {
		t.Fatalf("experiment_reports = %#v, want capped report list", payload.ExperimentReports)
	}
	if payload.ExperimentReports[0].CurrentCheckpoint != "neon-guard-120t-current" || payload.ExperimentReports[0].CompareCheckpoint != "neon-guard-120t-compare" {
		t.Fatalf("experiment report checkpoints = %q / %q, want runtime audit to expose archived replay anchors", payload.ExperimentReports[0].CurrentCheckpoint, payload.ExperimentReports[0].CompareCheckpoint)
	}
	if len(payload.Population.Promoted) != 1 || payload.Population.Promoted[0].Name != "茶摊老板" {
		t.Fatalf("population.promoted = %#v, want capped promoted insights", payload.Population.Promoted)
	}
	if len(payload.AuditSummary) == 0 {
		t.Fatalf("audit_summary = %#v, want consolidated summary lines", payload.AuditSummary)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode raw runtime audit: %v", err)
	}
	instanceRaw, _ := raw["instance"].(map[string]interface{})
	if _, ok := instanceRaw["active_character"]; ok {
		t.Fatalf("runtime audit instance unexpectedly exposes active_character: %#v", instanceRaw)
	}
	if _, ok := instanceRaw["loaded_characters"]; ok {
		t.Fatalf("runtime audit instance unexpectedly exposes loaded_characters: %#v", instanceRaw)
	}
	if etag := rec.Header().Get("ETag"); etag == "" {
		t.Fatalf("ETag header empty, want cacheable audit response")
	}
}

func TestHealthEndpoint(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("GET", "/api/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/health → %d, want 200", rec.Code)
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("status = %#v, want ok", payload["status"])
	}
}

func TestReadyEndpoint(t *testing.T) {
	oldVersion, oldCommit, oldTime := BuildVersion, BuildCommit, BuildTime
	BuildVersion, BuildCommit, BuildTime = "test-version", "test-commit", "2026-05-26T09:40:00Z"
	t.Cleanup(func() {
		BuildVersion, BuildCommit, BuildTime = oldVersion, oldCommit, oldTime
	})

	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("GET", "/api/ready", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/ready → %d, want 200", rec.Code)
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode ready: %v", err)
	}
	if payload["status"] != "ready" {
		t.Fatalf("status = %#v, want ready", payload["status"])
	}
	if payload["build"] != "test-version" {
		t.Fatalf("build = %#v, want test-version", payload["build"])
	}
	if payload["build_commit"] != "test-commit" {
		t.Fatalf("build_commit = %#v, want test-commit", payload["build_commit"])
	}
	if payload["build_time"] != "2026-05-26T09:40:00Z" {
		t.Fatalf("build_time = %#v, want test time", payload["build_time"])
	}
}

func TestReadyEndpointUnavailableWithoutEngine(t *testing.T) {
	s := &Server{}
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("GET", "/api/ready", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /api/ready without engine → %d, want 503", rec.Code)
	}
}

func TestVersionEndpoint(t *testing.T) {
	oldVersion, oldCommit, oldTime := BuildVersion, BuildCommit, BuildTime
	BuildVersion, BuildCommit, BuildTime = "test-version", "test-commit", "2026-05-26T09:40:00Z"
	t.Cleanup(func() {
		BuildVersion, BuildCommit, BuildTime = oldVersion, oldCommit, oldTime
	})

	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("GET", "/api/version", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/version → %d, want 200", rec.Code)
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode version: %v", err)
	}
	if payload["service"] != "corerp" {
		t.Fatalf("service = %#v, want corerp", payload["service"])
	}
	if payload["version"] != "test-version" {
		t.Fatalf("version = %#v, want test-version", payload["version"])
	}
	if payload["commit"] != "test-commit" {
		t.Fatalf("commit = %#v, want test-commit", payload["commit"])
	}
	if payload["time"] != "2026-05-26T09:40:00Z" {
		t.Fatalf("time = %#v, want test time", payload["time"])
	}
}

func TestInstancesEndpoint(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("GET", "/api/instances", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/instances → %d, want 200", rec.Code)
	}

	var payload struct {
		Default   string `json:"default"`
		Instances []struct {
			ID string `json:"id"`
		} `json:"instances"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode instances: %v", err)
	}
	if payload.Default != "default" {
		t.Fatalf("default = %q, want default", payload.Default)
	}
	if len(payload.Instances) != 1 || payload.Instances[0].ID != "default" {
		t.Fatalf("instances = %#v, want single default instance", payload.Instances)
	}
}

func TestInstancesEndpointUsesParticipantsAsSceneTruth(t *testing.T) {
	engine := &mockEngine{
		instanceID:        "default",
		name:              "Anya",
		state:             core.WorldState{},
		sceneParticipants: []string{"蓝姐", "谭叔"},
		participantDetails: []core.ParticipantSummary{
			{Name: "蓝姐", Present: true, Loaded: false, Switchable: true},
			{Name: "谭叔", Present: true, Loaded: true, Switchable: true},
		},
		loadedCharacters: []string{"Anya", "远处旁观者"},
	}
	s := NewServer(engine)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("GET", "/api/instances", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/instances = %d", rec.Code)
	}
	body := append([]byte(nil), rec.Body.Bytes()...)

	var payload struct {
		Instances []struct {
			ID                 string                    `json:"id"`
			FocusCharacter     string                    `json:"focus_character"`
			Participants       []string                  `json:"participants"`
			ParticipantDetails []core.ParticipantSummary `json:"participant_details"`
		} `json:"instances"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode instances: %v", err)
	}
	if len(payload.Instances) != 1 {
		t.Fatalf("instances = %#v, want 1 item", payload.Instances)
	}
	got := payload.Instances[0]
	if len(got.Participants) != 2 || got.Participants[0] != "蓝姐" || got.Participants[1] != "谭叔" {
		t.Fatalf("participants = %#v, want [蓝姐 谭叔]", got.Participants)
	}
	if got.FocusCharacter != "Anya" {
		t.Fatalf("focus_character = %#v, want Anya", got.FocusCharacter)
	}
	if len(got.ParticipantDetails) != 2 || got.ParticipantDetails[0].Name != "蓝姐" || got.ParticipantDetails[1].Name != "谭叔" {
		t.Fatalf("participant_details = %#v, want scene details", got.ParticipantDetails)
	}
	var raw struct {
		Instances []map[string]interface{} `json:"instances"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode raw instances: %v", err)
	}
	if _, ok := raw.Instances[0]["active_character"]; ok {
		t.Fatalf("/api/instances unexpectedly exposes active_character: %#v", raw.Instances[0])
	}
	if _, ok := raw.Instances[0]["loaded_characters"]; ok {
		t.Fatalf("/api/instances unexpectedly exposes loaded_characters: %#v", raw.Instances[0])
	}
}

func TestInvalidInstanceIDReturns404(t *testing.T) {
	defaultEngine := &mockEngine{instanceID: "default", name: "Anya", state: core.WorldState{}}
	otherEngine := &mockEngine{instanceID: "alt", name: "V", state: core.WorldState{}}
	s := NewServer(defaultEngine, &mockResolver{
		defaultID: "default",
		engines: map[string]RuntimeEngine{
			"default": defaultEngine,
			"alt":     otherEngine,
		},
	})
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("GET", "/api/state?instance_id=missing", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /api/state?instance_id=missing → %d, want 404", rec.Code)
	}
}

func TestInstanceCreateEndpoint(t *testing.T) {
	defaultEngine := &mockEngine{instanceID: "default", name: "Anya", state: core.WorldState{}}
	resolver := &mockResolver{
		defaultID: "default",
		engines:   map[string]RuntimeEngine{"default": defaultEngine},
	}
	s := NewServer(defaultEngine, resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("POST", "/api/instances/create", strings.NewReader(`{"id":"alt","label":"Alt","focus_character":"V"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/instances/create → %d, want 200", rec.Code)
	}
	if _, ok := resolver.engines["alt"]; !ok {
		t.Fatal("resolver missing created instance")
	}
	body := append([]byte(nil), rec.Body.Bytes()...)
	var payload struct {
		ID             string `json:"id"`
		FocusCharacter string `json:"focus_character"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode create payload: %v", err)
	}
	if payload.FocusCharacter != "V" {
		t.Fatalf("payload focus_character = %#v, want V", payload.FocusCharacter)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode raw create payload: %v", err)
	}
	if _, ok := raw["active_character"]; ok {
		t.Fatalf("create payload unexpectedly exposes active_character: %#v", raw)
	}
	if _, ok := raw["loaded_characters"]; ok {
		t.Fatalf("create payload unexpectedly exposes loaded_characters: %#v", raw)
	}
}

func TestInstanceCreateEndpointAcceptsSourceID(t *testing.T) {
	defaultEngine := &mockEngine{instanceID: "default", name: "Anya", state: core.WorldState{}}
	resolver := &mockResolver{
		defaultID: "default",
		engines:   map[string]RuntimeEngine{"default": defaultEngine},
	}
	s := NewServer(defaultEngine, resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("POST", "/api/instances/create", strings.NewReader(`{"source_id":"default","id":"exp-a","label":"Experiment A"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/instances/create source_id → %d, want 200", rec.Code)
	}
	if resolver.lastCreate.sourceID != "default" {
		t.Fatalf("resolver.lastCreate = %#v, want sourceID default", resolver.lastCreate)
	}
	if resolver.lastCreate.id != "exp-a" {
		t.Fatalf("resolver.lastCreate = %#v, want id exp-a", resolver.lastCreate)
	}
}

func TestInstanceCreateEndpointIgnoresActiveCharacterFallback(t *testing.T) {
	defaultEngine := &mockEngine{instanceID: "default", name: "Anya", state: core.WorldState{}}
	resolver := &mockResolver{
		defaultID: "default",
		engines:   map[string]RuntimeEngine{"default": defaultEngine},
	}
	s := NewServer(defaultEngine, resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("POST", "/api/instances/create", strings.NewReader(`{"id":"alt-compat","label":"Compat","active_character":"V"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/instances/create active_character-only → %d, want 200", rec.Code)
	}
	if resolver.lastCreate.focusCharacter != "" {
		t.Fatalf("resolver.lastCreate = %#v, want empty focusCharacter when focus_character omitted", resolver.lastCreate)
	}
}

func TestInstanceDefaultEndpoint(t *testing.T) {
	defaultEngine := &mockEngine{instanceID: "default", name: "Anya", state: core.WorldState{}}
	altEngine := &mockEngine{instanceID: "alt", name: "V", state: core.WorldState{}}
	resolver := &mockResolver{
		defaultID: "default",
		engines: map[string]RuntimeEngine{
			"default": defaultEngine,
			"alt":     altEngine,
		},
	}
	s := NewServer(defaultEngine, resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("POST", "/api/instances/default", strings.NewReader(`{"id":"alt"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/instances/default → %d, want 200", rec.Code)
	}
	if resolver.defaultID != "alt" {
		t.Fatalf("defaultID = %q, want alt", resolver.defaultID)
	}
}

func TestInstanceStatusEndpoint(t *testing.T) {
	defaultEngine := &mockEngine{instanceID: "default", name: "Anya", state: core.WorldState{}}
	altEngine := &mockEngine{instanceID: "alt", name: "V", state: core.WorldState{}}
	resolver := &mockResolver{
		defaultID: "default",
		engines: map[string]RuntimeEngine{
			"default": defaultEngine,
			"alt":     altEngine,
		},
		stopped: map[string]bool{"alt": true},
	}
	s := NewServer(defaultEngine, resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("GET", "/api/instances/status?id=alt", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/instances/status?id=alt → %d, want 200", rec.Code)
	}
	body := append([]byte(nil), rec.Body.Bytes()...)
	var payload struct {
		ID             string `json:"id"`
		Status         string `json:"status"`
		FocusCharacter string `json:"focus_character"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if payload.ID != "alt" || payload.Status != "stopped" {
		t.Fatalf("payload = %#v, want alt/stopped", payload)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode raw status: %v", err)
	}
	if _, ok := raw["active_character"]; ok {
		t.Fatalf("status payload unexpectedly exposes active_character: %#v", raw)
	}
	if _, ok := raw["loaded_characters"]; ok {
		t.Fatalf("status payload unexpectedly exposes loaded_characters: %#v", raw)
	}
}

func TestInstanceStopEndpoint(t *testing.T) {
	defaultEngine := &mockEngine{instanceID: "default", name: "Anya", state: core.WorldState{}}
	altEngine := &mockEngine{instanceID: "alt", name: "V", state: core.WorldState{}}
	resolver := &mockResolver{
		defaultID: "default",
		engines: map[string]RuntimeEngine{
			"default": defaultEngine,
			"alt":     altEngine,
		},
	}
	s := NewServer(defaultEngine, resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("POST", "/api/instances/stop", strings.NewReader(`{"id":"alt"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/instances/stop → %d, want 200", rec.Code)
	}
	if !resolver.stopped["alt"] {
		t.Fatal("expected alt instance to be marked stopped")
	}
}

func TestInstanceDeleteEndpoint(t *testing.T) {
	defaultEngine := &mockEngine{instanceID: "default", name: "Anya", state: core.WorldState{}}
	altEngine := &mockEngine{instanceID: "alt", name: "V", state: core.WorldState{}}
	resolver := &mockResolver{
		defaultID: "default",
		engines: map[string]RuntimeEngine{
			"default": defaultEngine,
			"alt":     altEngine,
		},
	}
	s := NewServer(defaultEngine, resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("POST", "/api/instances/delete", strings.NewReader(`{"id":"alt"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/instances/delete → %d, want 200", rec.Code)
	}
	if _, ok := resolver.engines["alt"]; ok {
		t.Fatal("expected alt instance to be deleted")
	}
}

func TestInstanceDeleteEndpointConflict(t *testing.T) {
	defaultEngine := &mockEngine{instanceID: "default", name: "Anya", state: core.WorldState{}}
	resolver := &mockResolver{
		defaultID: "default",
		engines: map[string]RuntimeEngine{
			"default": defaultEngine,
		},
	}
	s := NewServer(defaultEngine, resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("POST", "/api/instances/delete", strings.NewReader(`{"id":"default"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("POST /api/instances/delete only/default → %d, want 409", rec.Code)
	}
}

func TestInstanceDeleteEndpointNotFound(t *testing.T) {
	defaultEngine := &mockEngine{instanceID: "default", name: "Anya", state: core.WorldState{}}
	resolver := &mockResolver{
		defaultID: "default",
		engines: map[string]RuntimeEngine{
			"default": defaultEngine,
		},
	}
	s := NewServer(defaultEngine, resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("POST", "/api/instances/delete", strings.NewReader(`{"id":"missing"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /api/instances/delete missing → %d, want 404", rec.Code)
	}
}

func TestInstanceDefaultEndpointNotFound(t *testing.T) {
	defaultEngine := &mockEngine{instanceID: "default", name: "Anya", state: core.WorldState{}}
	resolver := &mockResolver{
		defaultID: "default",
		engines: map[string]RuntimeEngine{
			"default": defaultEngine,
		},
	}
	s := NewServer(defaultEngine, resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("POST", "/api/instances/default", strings.NewReader(`{"id":"missing"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /api/instances/default missing → %d, want 404", rec.Code)
	}
}

func TestChatRoutePost(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	body := `{"message":"hello"}`
	req := httptest.NewRequest("POST", "/api/chat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST /api/chat → %d, want 200", rec.Code)
	}
}

func TestSwitchRoutePost(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	body := `{"focus_character":"V"}`
	req := httptest.NewRequest("POST", "/api/switch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST /api/switch → %d, want 200", rec.Code)
	}
}

func TestSwitchRouteRejectsNonSwitchableParticipant(t *testing.T) {
	s := NewServer(&mockEngine{
		name: "Anya",
		participantDetails: []core.ParticipantSummary{
			{Name: "Anya", Kind: "persona", Source: "character_definition", Loaded: true, Switchable: true, Present: true, Focus: true},
			{Name: "玩家", Kind: "player", Source: "player_role", Loaded: false, Switchable: false, Present: true},
		},
	})
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("POST", "/api/switch", strings.NewReader(`{"focus_character":"玩家"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/switch non-switchable → %d, want 400", rec.Code)
	}
}

func TestWorldsRoutePostEntersWorld(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/worlds", strings.NewReader(`{"path":"worlds/neon_block"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/worlds = %d", rec.Code)
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["focus_character"] != "entered-character" {
		t.Fatalf("payload = %#v, want entered character", payload)
	}
	if _, ok := payload["character"]; ok {
		t.Fatalf("character compatibility mirror should be absent on /api/worlds POST payload = %#v", payload)
	}
	if _, ok := payload["participants"].([]interface{}); !ok {
		t.Fatalf("participants = %#v, want array", payload["participants"])
	}
	if _, ok := payload["participant_details"].([]interface{}); !ok {
		t.Fatalf("participant_details = %#v, want array", payload["participant_details"])
	}
}

func TestDCLRoutesListInstallAndRemoveDeclarativeMod(t *testing.T) {
	oldDCLRoot := DCLRoot
	oldWorldRoot := WorldCatalogRoot
	DCLRoot = t.TempDir()
	WorldCatalogRoot = filepath.Join(t.TempDir(), "worlds")
	t.Cleanup(func() {
		DCLRoot = oldDCLRoot
		WorldCatalogRoot = oldWorldRoot
	})
	writeAPIDCLFile(t, filepath.Join(DCLRoot, "sample_loop.dcl", "manifest.yml"), `
id: sample_loop
name: Sample Loop
version: 0.1.0
entry_world: sample_loop_world
`)
	writeAPIDCLFile(t, filepath.Join(DCLRoot, "sample_loop.dcl", "patches", "world.yml"), `
core_rules: "sample loop rules"
pressures:
  - id: loop_pressure
    name: Loop Pressure
    kind: suspicion
    intensity: 0.2
    target: mansion
`)

	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/api/dcl", nil))
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/dcl = %d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "sample_loop") {
		t.Fatalf("GET /api/dcl body = %s, want sample_loop", getRec.Body.String())
	}

	installRec := httptest.NewRecorder()
	installReq := httptest.NewRequest(http.MethodPost, "/api/dcl/install", strings.NewReader(`{"id":"sample_loop"}`))
	installReq.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(installRec, installReq)
	if installRec.Code != http.StatusOK {
		t.Fatalf("POST /api/dcl/install = %d body=%s", installRec.Code, installRec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(WorldCatalogRoot, "sample_loop_world", "world", "pressures.yml")); err != nil {
		t.Fatalf("installed world pressures missing: %v", err)
	}

	removeRec := httptest.NewRecorder()
	removeReq := httptest.NewRequest(http.MethodPost, "/api/dcl/remove", strings.NewReader(`{"id":"sample_loop","delete_world":true,"delete_package":true}`))
	removeReq.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(removeRec, removeReq)
	if removeRec.Code != http.StatusOK {
		t.Fatalf("POST /api/dcl/remove = %d body=%s", removeRec.Code, removeRec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(WorldCatalogRoot, "sample_loop_world")); !os.IsNotExist(err) {
		t.Fatalf("installed world still exists or stat failed unexpectedly: %v", err)
	}
	if _, err := os.Stat(filepath.Join(DCLRoot, "sample_loop.dcl")); !os.IsNotExist(err) {
		t.Fatalf("dcl package still exists or stat failed unexpectedly: %v", err)
	}
}

func writeAPIDCLFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestExportRouteReturnsFocusDefinitionAsPrimary(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/export?format=json&limit=5", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/export = %d", rec.Code)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode /api/export: %v", err)
	}
	if payload["focus_character"] != "Anya" {
		t.Fatalf("focus_character = %#v, want Anya", payload["focus_character"])
	}
	if _, ok := payload["focus_definition"].(map[string]interface{}); !ok {
		t.Fatalf("focus_definition = %#v, want object", payload["focus_definition"])
	}
	if _, ok := payload["character"]; ok {
		t.Fatalf("top-level character compatibility mirror should be absent on /api/export payload = %#v", payload)
	}
	if _, ok := payload["participants"].([]interface{}); !ok {
		t.Fatalf("participants = %#v, want array", payload["participants"])
	}
	if _, ok := payload["participant_details"].([]interface{}); !ok {
		t.Fatalf("participant_details = %#v, want array", payload["participant_details"])
	}
}

func TestLegacyFocusDefinitionRoutesAdvertiseSuccessor(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	tests := []struct {
		path        string
		wantLinkRef string
	}{
		{path: "/api/character", wantLinkRef: "/api/focus-definition"},
		{path: "/api/character-config", wantLinkRef: "/api/focus-definition-config"},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", tc.path, rec.Code)
		}
		if got := rec.Header().Get("Deprecation"); got != "true" {
			t.Fatalf("GET %s Deprecation = %q, want true", tc.path, got)
		}
		if got := rec.Header().Get("Link"); !strings.Contains(got, tc.wantLinkRef) {
			t.Fatalf("GET %s Link = %q, want successor %q", tc.path, got, tc.wantLinkRef)
		}
	}
}

func TestWorldsRouteGetIncludesActivePath(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/worlds", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/worlds = %d", rec.Code)
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["active_path"] == "" {
		t.Fatalf("payload = %#v, want active_path", payload)
	}
}

func TestForkRoutePost(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	body := `{"event_id":"evt_1","branch":"alt"}`
	req := httptest.NewRequest("POST", "/api/fork", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST /api/fork → %d, want 200", rec.Code)
	}
}

func TestCompressRoutePost(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	body := `{"from":1,"to":5}`
	req := httptest.NewRequest("POST", "/api/compress", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST /api/compress → %d, want 200", rec.Code)
	}
}

func TestSaveRoutePost(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("POST", "/api/saves", strings.NewReader(`{"name":"slot-a","branch":"main","note":"checkpoint"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST /api/saves → %d, want 200", rec.Code)
	}
}

func TestSaveLoadRoutePost(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("POST", "/api/saves/load", strings.NewReader(`{"name":"slot-a"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST /api/saves/load → %d, want 200", rec.Code)
	}
}

func TestCharacterConfigRoutePost(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	body := `{"focus_character":"Anya","card":{"identity":{"name":"Anya","immutable":["cold"],"adaptive":{"trust":3},"forbidden":["info_dump"],"voice":{"style":"brief","rhythm":"short"},"writing_guide":"stay sharp"},"goals":[{"id":"survive","priority":10,"type":"primary","condition":"always"}]}}`
	req := httptest.NewRequest("POST", "/api/focus-definition-config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST /api/focus-definition-config → %d, want 200", rec.Code)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode /api/focus-definition-config POST: %v", err)
	}
	if payload["focus_character"] != "Anya" {
		t.Fatalf("focus_character = %#v, want Anya", payload["focus_character"])
	}
	if _, ok := payload["character"]; ok {
		t.Fatalf("canonical /api/focus-definition-config payload should not expose character mirror: %#v", payload)
	}
}

func TestCharacterConfigRouteGetUsesCanonicalFocusCharacter(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/focus-definition-config?focus_character=Anya", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/focus-definition-config → %d, want 200", rec.Code)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode /api/focus-definition-config GET: %v", err)
	}
	if payload["focus_character"] != "Anya" {
		t.Fatalf("focus_character = %#v, want Anya", payload["focus_character"])
	}
	if _, ok := payload["character"]; ok {
		t.Fatalf("canonical /api/focus-definition-config GET should not expose character mirror: %#v", payload)
	}
}

func TestCharacterConfigRouteRejectsInvalidGoalCondition(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/focus-definition-config", strings.NewReader(`{"focus_character":"Anya","card":{"identity":{"name":"Anya"},"goals":[{"id":"secret","priority":8,"type":"hidden","condition":"trust >","reveal_condition":"scene == safehouse"}]}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/focus-definition-config invalid condition = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "condition invalid") {
		t.Fatalf("body = %q, want condition validation error", rec.Body.String())
	}
}

func TestPlayerRoleRoutePost(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	body := `{"name":"贾宝玉","description":"荣国府公子","bound_character":"贾宝玉"}`
	req := httptest.NewRequest("POST", "/api/player-role", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST /api/player-role → %d, want 200", rec.Code)
	}
}

func TestWorldConfigRoutes(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	getReq := httptest.NewRequest(http.MethodGet, "/api/world-config", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/world-config = %d", getRec.Code)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/api/world-config", strings.NewReader(`{"name":"大观园","core_rules":"园中规矩"}`))
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	mux.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST /api/world-config = %d", postRec.Code)
	}
}

func TestPopulationRoutes(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	getReq := httptest.NewRequest(http.MethodGet, "/api/population", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/population = %d", getRec.Code)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/api/population", strings.NewReader(`{
		"background_npcs":[{"id":"tea_vendor","name":"茶摊老板","role":"商贩"}],
		"policy":{"promote_threshold":14,"major_threshold":30}
	}`))
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	mux.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST /api/population = %d", postRec.Code)
	}
}

func TestWorldStructureRoutes(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	getReq := httptest.NewRequest(http.MethodGet, "/api/world-structure", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/world-structure = %d", getRec.Code)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/api/world-structure", strings.NewReader(`{
		"ruleset":{"rules":[{"id":"curfew","title":"宵禁","summary":"夜间不得随意出行"}]},
		"seed":{"premise":"城中戒严","current_situation":"街头搜查","variables":{"alert":"high"}},
		"factions":[{"id":"guard","name":"巡城司","role":"law"}],
		"locations":[{"id":"east_gate","name":"东门","kind":"gate"}],
		"pressures":[{"id":"panic","name":"恐慌","kind":"social","intensity":0.6}]
	}`))
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	mux.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST /api/world-structure = %d", postRec.Code)
	}
}

func TestPopulationInsightsRoute(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	getReq := httptest.NewRequest(http.MethodGet, "/api/population-insights", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/population-insights = %d", getRec.Code)
	}
	var payload core.PopulationInsights
	if err := json.NewDecoder(getRec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode population insights: %v", err)
	}
	if len(payload.Promoted) != 1 || payload.Promoted[0].Name != "茶摊老板" {
		t.Fatalf("population insights = %#v", payload)
	}
}

func TestSimTickRouteSupportsBatchCount(t *testing.T) {
	engine := &mockEngine{instanceID: "default", name: "Anya", state: core.WorldState{}}
	s := NewServer(engine)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/sim/tick", strings.NewReader(`{"count":4}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/sim/tick batch = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode sim tick payload: %v", err)
	}
	if payload["count"].(float64) != 4 {
		t.Fatalf("count = %#v, want 4", payload["count"])
	}
	status, _ := payload["tick_status"].(map[string]interface{})
	if status["tick_count"].(float64) != 4 {
		t.Fatalf("tick_status = %#v, want tick_count 4", status)
	}
}

func TestAPIStructureInterventionDivergesLongWindowOutcomeAcrossInstances(t *testing.T) {
	baseDir := t.TempDir()
	baselineWorldDir := filepath.Join(baseDir, "baseline-world")
	intervenedWorldDir := filepath.Join(baseDir, "intervened-world")
	writeAPITestWorldBundle(t, baselineWorldDir, "低压世界", "没有控制区和压力时，世界不应自然长出主要角色")
	writeAPITestWorldBundle(t, intervenedWorldDir, "高压世界", "控制区和压力会改变长期世界结果")

	baselineEngine := newRealRuntimeEngineForAPITest(
		t,
		filepath.Join(t.TempDir(), "baseline.db"),
		filepath.Join(baseDir, "baseline-data"),
		baselineWorldDir,
		"baseline",
		"低压世界",
		"没有控制区和压力时，世界不应自然长出主要角色",
	)
	intervenedEngine := newRealRuntimeEngineForAPITest(
		t,
		filepath.Join(t.TempDir(), "intervened.db"),
		filepath.Join(baseDir, "intervened-data"),
		intervenedWorldDir,
		"intervened",
		"高压世界",
		"控制区和压力会改变长期世界结果",
	)

	resolver := &mockResolver{
		defaultID: "baseline",
		engines: map[string]RuntimeEngine{
			"baseline":   baselineEngine,
			"intervened": intervenedEngine,
		},
	}
	s := NewServer(baselineEngine, resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	postJSON := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	getJSON := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	populationBody := `{
		"background_npcs":[
			{"id":"watcher","name":"巡夜人","role":"guard","location":"外城","faction":"guard","traits":["警觉","克制"],"hooks":["宵禁","盘查"]},
			{"id":"runner","name":"线人","role":"informant","location":"外城","faction":"smugglers","traits":["灵活","谨慎"],"hooks":["走私","风声"]}
		],
		"policy":{"promote_threshold":4.2,"major_threshold":8,"interaction_weight":3,"mention_weight":1,"event_weight":2,"relationship_weight":3,"scene_weight":2}
	}`
	for _, instanceID := range []string{"baseline", "intervened"} {
		rec := postJSON("/api/population?instance_id="+instanceID, populationBody)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /api/population?instance_id=%s = %d body=%s", instanceID, rec.Code, rec.Body.String())
		}
	}

	baselineStructure := `{
		"locations":[{"id":"outer_city","name":"外城","kind":"district","description":"无人控制的普通街区","controller":""}]
	}`
	rec := postJSON("/api/world-structure?instance_id=baseline", baselineStructure)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/world-structure baseline = %d body=%s", rec.Code, rec.Body.String())
	}
	intervenedStructure := `{
		"factions":[
			{"id":"guard","name":"巡城司","role":"law","description":"负责宵禁和盘查","relationships":["敌对 smugglers"]},
			{"id":"smugglers","name":"走私帮","role":"criminal","description":"夜里持续活动","relationships":["敌对 guard"]}
		],
		"locations":[{"id":"outer_city","name":"外城","kind":"district","description":"巡城司控制区","controller":"guard"}],
		"pressures":[{"id":"curfew","name":"宵禁升级","kind":"conflict","description":"外城盘查与走私冲突持续加剧","intensity":0.9,"target":"guard"}]
	}`
	rec = postJSON("/api/world-structure?instance_id=intervened", intervenedStructure)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/world-structure intervened = %d body=%s", rec.Code, rec.Body.String())
	}

	for _, instanceID := range []string{"baseline", "intervened"} {
		rec := postJSON("/api/sim/tick?instance_id="+instanceID, `{"count":36}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /api/sim/tick?instance_id=%s batch = %d body=%s", instanceID, rec.Code, rec.Body.String())
		}
	}

	var baselineStatus map[string]interface{}
	rec = getJSON("/api/sim/status?instance_id=baseline")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/sim/status baseline = %d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&baselineStatus); err != nil {
		t.Fatalf("decode baseline sim/status: %v", err)
	}
	var intervenedStatus map[string]interface{}
	rec = getJSON("/api/sim/status?instance_id=intervened")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/sim/status intervened = %d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&intervenedStatus); err != nil {
		t.Fatalf("decode intervened sim/status: %v", err)
	}

	if baselineStatus["tension"].(float64) != 0 {
		t.Fatalf("baseline tension = %#v, want calm long-window baseline", baselineStatus["tension"])
	}
	if intervenedStatus["tension"].(float64) <= baselineStatus["tension"].(float64) {
		t.Fatalf("baseline tension=%v intervened tension=%v, want higher intervened tension", baselineStatus["tension"], intervenedStatus["tension"])
	}
	baselineTickHistory, ok := baselineStatus["tick_history"].([]interface{})
	if !ok || len(baselineTickHistory) != 12 {
		t.Fatalf("baseline tick_history = %#v, want 12 recent snapshots", baselineStatus["tick_history"])
	}
	intervenedTickHistory, ok := intervenedStatus["tick_history"].([]interface{})
	if !ok || len(intervenedTickHistory) != 12 {
		t.Fatalf("intervened tick_history = %#v, want 12 recent snapshots", intervenedStatus["tick_history"])
	}
	baselineTrajectory, ok := baselineStatus["trajectory_summary"].([]interface{})
	if !ok || len(baselineTrajectory) == 0 {
		t.Fatalf("baseline trajectory_summary = %#v, want API long-window summary", baselineStatus["trajectory_summary"])
	}
	intervenedTrajectory, ok := intervenedStatus["trajectory_summary"].([]interface{})
	if !ok || len(intervenedTrajectory) == 0 {
		t.Fatalf("intervened trajectory_summary = %#v, want API long-window summary", intervenedStatus["trajectory_summary"])
	}
	if fmt.Sprintf("%v", baselineTrajectory) == fmt.Sprintf("%v", intervenedTrajectory) {
		t.Fatalf("baseline trajectory=%#v intervened trajectory=%#v, want API summaries to diverge", baselineTrajectory, intervenedTrajectory)
	}

	intervenedPressureDiagHits := 0
	baselinePressureDiagHits := 0
	for _, raw := range baselineTickHistory {
		snapshot, _ := raw.(map[string]interface{})
		diagnostics, _ := snapshot["diagnostics"].([]interface{})
		for _, diagRaw := range diagnostics {
			diag, _ := diagRaw.(map[string]interface{})
			if metric, _ := diag["metric"].(string); metric == "active_pressure" || metric == "scene_control" {
				baselinePressureDiagHits++
			}
		}
	}
	for _, raw := range intervenedTickHistory {
		snapshot, _ := raw.(map[string]interface{})
		diagnostics, _ := snapshot["diagnostics"].([]interface{})
		for _, diagRaw := range diagnostics {
			diag, _ := diagRaw.(map[string]interface{})
			if metric, _ := diag["metric"].(string); metric == "active_pressure" || metric == "scene_control" {
				intervenedPressureDiagHits++
			}
		}
	}
	if baselinePressureDiagHits != 0 {
		t.Fatalf("baseline diagnostics = %#v, want no pressure/control diagnostics in calm world", baselineStatus["tick_history"])
	}
	if intervenedPressureDiagHits == 0 {
		t.Fatalf("intervened diagnostics = %#v, want pressure/control diagnostics across recent history", intervenedStatus["tick_history"])
	}

	var baselineInsights core.PopulationInsights
	rec = getJSON("/api/population-insights?instance_id=baseline")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/population-insights baseline = %d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&baselineInsights); err != nil {
		t.Fatalf("decode baseline population-insights: %v", err)
	}
	var intervenedInsights core.PopulationInsights
	rec = getJSON("/api/population-insights?instance_id=intervened")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/population-insights intervened = %d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&intervenedInsights); err != nil {
		t.Fatalf("decode intervened population-insights: %v", err)
	}

	foundPromoted := false
	for _, npc := range intervenedInsights.Promoted {
		if npc.Name == "巡夜人" {
			foundPromoted = true
			if npc.IdentityCore == "" || npc.Attention.Score < 4.2 {
				t.Fatalf("intervened promoted npc = %#v, want persisted identity core and score through API path", npc)
			}
		}
	}
	if !foundPromoted {
		t.Fatalf("intervened promoted = %#v, want 巡夜人 promoted through API path", intervenedInsights.Promoted)
	}
	if len(intervenedInsights.Promoted) == 0 || intervenedInsights.Promoted[0].Name != "巡夜人" {
		t.Fatalf("intervened promoted = %#v, want 巡夜人 to dominate pressure-driven API path", intervenedInsights.Promoted)
	}
}

func TestAPIWorldOutcomeSampleMatrixAcrossHundredTicks(t *testing.T) {
	type sample struct {
		id                 string
		worldName          string
		structureBody      string
		expectedLeader     string
		expectedPressureID string
		expectCalmTension  bool
	}

	baseDir := t.TempDir()
	samples := []sample{
		{
			id:        "calm",
			worldName: "低压世界",
			structureBody: `{
				"locations":[{"id":"outer_city","name":"外城","kind":"district","description":"无人控制的普通街区","controller":""}]
			}`,
			expectCalmTension: true,
		},
		{
			id:                 "guard",
			worldName:          "巡城司样本",
			expectedLeader:     "巡夜人",
			expectedPressureID: "curfew",
			structureBody: `{
				"factions":[
					{"id":"guard","name":"巡城司","role":"law","description":"负责宵禁和盘查","relationships":["敌对 smugglers"]},
					{"id":"smugglers","name":"走私帮","role":"criminal","description":"夜里持续活动","relationships":["敌对 guard"]}
				],
				"locations":[{"id":"outer_city","name":"外城","kind":"district","description":"巡城司控制区","controller":"guard"}],
				"pressures":[{"id":"curfew","name":"宵禁升级","kind":"conflict","description":"巡城司扩大盘查","intensity":0.9,"target":"guard"}]
			}`,
		},
		{
			id:                 "smuggler",
			worldName:          "走私样本",
			expectedLeader:     "线人",
			expectedPressureID: "smuggling",
			structureBody: `{
				"factions":[
					{"id":"guard","name":"巡城司","role":"law","description":"负责宵禁和盘查","relationships":["敌对 smugglers"]},
					{"id":"smugglers","name":"走私帮","role":"criminal","description":"夜里持续活动","relationships":["敌对 guard"]}
				],
				"locations":[{"id":"outer_city","name":"外城","kind":"district","description":"走私帮暗巷控制区","controller":"smugglers"}],
				"pressures":[{"id":"smuggling","name":"走私潮上涨","kind":"criminal","description":"走私帮快速扩张","intensity":0.88,"target":"smugglers"}]
			}`,
		},
	}

	resolver := &mockResolver{defaultID: "calm", engines: map[string]RuntimeEngine{}}
	for _, sample := range samples {
		worldDir := filepath.Join(baseDir, sample.id+"-world")
		writeAPITestWorldBundle(t, worldDir, sample.worldName, "API 多样本 120 tick 验证")
		engine := newRealRuntimeEngineForAPITest(
			t,
			filepath.Join(t.TempDir(), sample.id+".db"),
			filepath.Join(baseDir, sample.id+"-data"),
			worldDir,
			sample.id,
			sample.worldName,
			"API 多样本 120 tick 验证",
		)
		resolver.engines[sample.id] = engine
	}

	s := NewServer(resolver.engines["calm"], resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	postJSON := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	getJSON := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	populationBody := `{
		"background_npcs":[
			{"id":"watcher","name":"巡夜人","role":"guard","location":"外城","faction":"guard","traits":["警觉","克制"],"hooks":["宵禁","盘查"]},
			{"id":"runner","name":"线人","role":"informant","location":"外城","faction":"smugglers","traits":["灵活","谨慎"],"hooks":["走私","风声"]}
		],
		"policy":{"promote_threshold":7.3,"major_threshold":10,"interaction_weight":3,"mention_weight":1,"event_weight":2,"relationship_weight":3,"scene_weight":2}
	}`

	for _, sample := range samples {
		rec := postJSON("/api/population?instance_id="+sample.id, populationBody)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /api/population?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
		}
		rec = postJSON("/api/world-structure?instance_id="+sample.id, sample.structureBody)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /api/world-structure?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
		}
		rec = postJSON("/api/sim/tick?instance_id="+sample.id, `{"count":120}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /api/sim/tick?instance_id=%s count=120 = %d body=%s", sample.id, rec.Code, rec.Body.String())
		}
	}

	results := map[string]map[string]string{}
	for _, sample := range samples {
		rec := getJSON("/api/sim/status?instance_id=" + sample.id)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/sim/status?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
		}
		var status map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
			t.Fatalf("decode sim/status %s: %v", sample.id, err)
		}
		rec = getJSON("/api/population-insights?instance_id=" + sample.id)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/population-insights?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
		}
		var insights core.PopulationInsights
		if err := json.NewDecoder(rec.Body).Decode(&insights); err != nil {
			t.Fatalf("decode population-insights %s: %v", sample.id, err)
		}
		promoted := make([]string, 0, len(insights.Promoted))
		for _, npc := range insights.Promoted {
			promoted = append(promoted, npc.Name)
		}
		topPromoted := ""
		if len(insights.Promoted) > 0 {
			topPromoted = insights.Promoted[0].Name
		}
		sort.Strings(promoted)
		trajectory, _ := status["trajectory_summary"].([]interface{})
		trajectoryText := fmt.Sprintf("%v", trajectory)

		if sample.expectCalmTension {
			if status["tension"].(float64) != 0 {
				t.Fatalf("%s tension = %#v, want calm API sample to remain stable", sample.id, status["tension"])
			}
		} else {
			if !containsString(promoted, sample.expectedLeader) {
				t.Fatalf("%s promoted = %#v, want %s promoted through API sample", sample.id, promoted, sample.expectedLeader)
			}
			if topPromoted != sample.expectedLeader {
				t.Fatalf("%s top promoted = %q, want %q to dominate through API sample", sample.id, topPromoted, sample.expectedLeader)
			}
			if !strings.Contains(trajectoryText, sample.expectedPressureID) {
				t.Fatalf("%s trajectory = %s, want pressure %s in API summary", sample.id, trajectoryText, sample.expectedPressureID)
			}
		}

		results[sample.id] = map[string]string{
			"trajectory": trajectoryText,
			"promoted":   strings.Join(promoted, ","),
			"top":        topPromoted,
		}
	}

	if results["guard"]["trajectory"] == results["smuggler"]["trajectory"] {
		t.Fatalf("guard vs smuggler trajectory = %#v vs %#v, want API sample matrix divergence", results["guard"], results["smuggler"])
	}
	if results["guard"]["top"] == results["smuggler"]["top"] {
		t.Fatalf("guard vs smuggler top promoted = %#v vs %#v, want different API promoted leaders", results["guard"], results["smuggler"])
	}
}

func TestAPIWorldOutcomeSampleMatrixAcrossTwoHundredTicks(t *testing.T) {
	type sample struct {
		id                 string
		worldName          string
		structureBody      string
		expectedLeader     string
		expectedPressureID string
		expectCalmTension  bool
		expectTensionFloor float64
	}

	baseDir := t.TempDir()
	samples := []sample{
		{
			id:        "calm200",
			worldName: "低压世界 200",
			structureBody: `{
				"locations":[{"id":"outer_city","name":"外城","kind":"district","description":"无人控制的普通街区","controller":""}]
			}`,
			expectCalmTension: true,
		},
		{
			id:                 "guard200",
			worldName:          "巡城司样本 200",
			expectedLeader:     "巡夜人",
			expectedPressureID: "curfew",
			expectTensionFloor: 0.7,
			structureBody: `{
				"factions":[
					{"id":"guard","name":"巡城司","role":"law","description":"负责宵禁和盘查","relationships":["敌对 smugglers"]},
					{"id":"smugglers","name":"走私帮","role":"criminal","description":"夜里持续活动","relationships":["敌对 guard"]}
				],
				"locations":[{"id":"outer_city","name":"外城","kind":"district","description":"巡城司控制区","controller":"guard"}],
				"pressures":[{"id":"curfew","name":"宵禁升级","kind":"conflict","description":"巡城司扩大盘查","intensity":0.92,"target":"guard"}]
			}`,
		},
		{
			id:                 "smuggler200",
			worldName:          "走私样本 200",
			expectedLeader:     "线人",
			expectedPressureID: "smuggling",
			expectTensionFloor: 0.65,
			structureBody: `{
				"factions":[
					{"id":"guard","name":"巡城司","role":"law","description":"负责宵禁和盘查","relationships":["敌对 smugglers"]},
					{"id":"smugglers","name":"走私帮","role":"criminal","description":"夜里持续活动","relationships":["敌对 guard"]}
				],
				"locations":[{"id":"outer_city","name":"外城","kind":"district","description":"走私帮暗巷控制区","controller":"smugglers"}],
				"pressures":[{"id":"smuggling","name":"走私潮上涨","kind":"criminal","description":"走私帮快速扩张","intensity":0.9,"target":"smugglers"}]
			}`,
		},
		{
			id:                 "infra200",
			worldName:          "电网样本 200",
			expectedLeader:     "修灯人",
			expectedPressureID: "blackout",
			expectTensionFloor: 0.55,
			structureBody: `{
				"factions":[
					{"id":"operators","name":"电网维护队","role":"utility","description":"负责夜间抢修","relationships":["紧张 guard"]},
					{"id":"guard","name":"巡城司","role":"law","description":"需要电网维持秩序","relationships":["依赖 operators"]}
				],
				"locations":[{"id":"outer_city","name":"外城","kind":"district","description":"断电频发的维护区","controller":"operators"}],
				"pressures":[{"id":"blackout","name":"电网失稳","kind":"infrastructure","description":"外城频繁断电，维护队持续抢修","intensity":0.87,"target":"operators"}]
			}`,
		},
	}

	resolver := &mockResolver{defaultID: "calm200", engines: map[string]RuntimeEngine{}}
	for _, sample := range samples {
		worldDir := filepath.Join(baseDir, sample.id+"-world")
		writeAPITestWorldBundle(t, worldDir, sample.worldName, "API 多样本 200 tick 验证")
		engine := newRealRuntimeEngineForAPITest(
			t,
			filepath.Join(t.TempDir(), sample.id+".db"),
			filepath.Join(baseDir, sample.id+"-data"),
			worldDir,
			sample.id,
			sample.worldName,
			"API 多样本 200 tick 验证",
		)
		resolver.engines[sample.id] = engine
	}

	s := NewServer(resolver.engines["calm200"], resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	postJSON := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	getJSON := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	populationBody := `{
		"background_npcs":[
			{"id":"watcher","name":"巡夜人","role":"guard","location":"外城","faction":"guard","traits":["警觉","克制"],"hooks":["宵禁","盘查"]},
			{"id":"runner","name":"线人","role":"informant","location":"外城","faction":"smugglers","traits":["灵活","谨慎"],"hooks":["走私","风声"]},
			{"id":"fixer","name":"修灯人","role":"utility","location":"外城","faction":"operators","traits":["耐心","稳重"],"hooks":["停电","供电","断路"]}
		],
		"policy":{"promote_threshold":7.3,"major_threshold":12,"interaction_weight":3,"mention_weight":1,"event_weight":2,"relationship_weight":3,"scene_weight":2}
	}`

	for _, sample := range samples {
		rec := postJSON("/api/population?instance_id="+sample.id, populationBody)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /api/population?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
		}
		rec = postJSON("/api/world-structure?instance_id="+sample.id, sample.structureBody)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /api/world-structure?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
		}
		rec = postJSON("/api/sim/tick?instance_id="+sample.id, `{"count":200}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /api/sim/tick?instance_id=%s count=200 = %d body=%s", sample.id, rec.Code, rec.Body.String())
		}
	}

	results := map[string]map[string]string{}
	for _, sample := range samples {
		rec := getJSON("/api/sim/status?instance_id=" + sample.id)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/sim/status?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
		}
		var status map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
			t.Fatalf("decode sim/status %s: %v", sample.id, err)
		}
		rec = getJSON("/api/population-insights?instance_id=" + sample.id)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/population-insights?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
		}
		var insights core.PopulationInsights
		if err := json.NewDecoder(rec.Body).Decode(&insights); err != nil {
			t.Fatalf("decode population-insights %s: %v", sample.id, err)
		}
		promoted := make([]string, 0, len(insights.Promoted))
		for _, npc := range insights.Promoted {
			promoted = append(promoted, npc.Name)
		}
		topPromoted := ""
		if len(insights.Promoted) > 0 {
			topPromoted = insights.Promoted[0].Name
		}
		sort.Strings(promoted)
		trajectory, _ := status["trajectory_summary"].([]interface{})
		trajectoryText := fmt.Sprintf("%v", trajectory)

		if sample.expectCalmTension {
			if status["tension"].(float64) != 0 {
				t.Fatalf("%s tension = %#v, want calm 200-tick API sample to remain stable", sample.id, status["tension"])
			}
		} else {
			if status["tension"].(float64) < sample.expectTensionFloor {
				t.Fatalf("%s tension = %#v, want >= %.2f in 200-tick API sample", sample.id, status["tension"], sample.expectTensionFloor)
			}
			if !containsString(promoted, sample.expectedLeader) {
				t.Fatalf("%s promoted = %#v, want %s promoted through 200-tick API sample", sample.id, promoted, sample.expectedLeader)
			}
			if topPromoted != sample.expectedLeader {
				t.Fatalf("%s top promoted = %q, want %q to dominate through 200-tick API sample", sample.id, topPromoted, sample.expectedLeader)
			}
			if !strings.Contains(trajectoryText, sample.expectedPressureID) {
				t.Fatalf("%s trajectory = %s, want pressure %s in 200-tick API summary", sample.id, trajectoryText, sample.expectedPressureID)
			}
		}

		results[sample.id] = map[string]string{
			"trajectory": trajectoryText,
			"promoted":   strings.Join(promoted, ","),
			"top":        topPromoted,
		}
	}

	if results["guard200"]["trajectory"] == results["smuggler200"]["trajectory"] {
		t.Fatalf("guard vs smuggler 200-tick trajectory = %#v vs %#v, want API sample matrix divergence", results["guard200"], results["smuggler200"])
	}
	if results["guard200"]["top"] == results["smuggler200"]["top"] {
		t.Fatalf("guard vs smuggler 200-tick top promoted = %#v vs %#v, want different API promoted leaders", results["guard200"], results["smuggler200"])
	}
	if results["infra200"]["trajectory"] == results["guard200"]["trajectory"] {
		t.Fatalf("infra vs guard 200-tick trajectory = %#v vs %#v, want broader API world outcome divergence", results["infra200"], results["guard200"])
	}
}

func TestAPIRealWorldDirectorySampleMatrixAcrossHundredTwentyTicks(t *testing.T) {
	type sample struct {
		id                 string
		worldName          string
		sourceDir          string
		scene              core.SceneState
		populationBody     string
		structureBody      string
		expectedLeader     string
		expectedPressureID string
		expectTensionFloor float64
	}

	baseDir := t.TempDir()
	samples := []sample{
		{
			id:        "neon-real",
			worldName: "霓虹里街区",
			sourceDir: "neon_block",
			scene: core.SceneState{
				Location:    "旧街夜市",
				TimeOfDay:   "深夜",
				Weather:     "闷热有雨",
				Characters:  []string{"蓝姐", "谭叔", "玩家"},
				Description: "真实世界目录中的默认夜市场景",
			},
			expectedLeader:     "蓝姐",
			expectedPressureID: "missing_rider",
			expectTensionFloor: 0.45,
		},
		{
			id:        "wedding-real",
			worldName: "新婚",
			sourceDir: "1_7",
			scene: core.SceneState{
				Location:    "未知地点",
				TimeOfDay:   "白天",
				Weather:     "阴雨",
				Characters:  []string{"许灵_单阶段人设", "玩家"},
				Description: "真实导入世界的默认接站场景",
			},
			populationBody: `{
				"background_npcs":[
					{"id":"steward","name":"婚礼管家","role":"steward","location":"未知地点","faction":"wedding_hosts","traits":["周到","急切"],"hooks":["要把迟到接站压下去","不想婚礼前出乱子"]},
					{"id":"driver","name":"代驾老周","role":"driver","location":"车站外","faction":"station_runners","traits":["疲惫","圆滑"],"hooks":["谁临时改了接站安排","想把责任甩出去"]},
					{"id":"guard","name":"站台保安","role":"guard","location":"候车厅","faction":"station_runners","traits":["谨慎","怕麻烦"],"hooks":["担心现场起争执","不想事情闹大"]}
				],
				"policy":{"promote_threshold":6.8,"major_threshold":11,"interaction_weight":3,"mention_weight":1,"event_weight":2,"relationship_weight":3,"scene_weight":2}
			}`,
			structureBody: `{
				"locations":[
					{"id":"arrival_point","name":"未知地点","kind":"arrival","description":"婚礼接站与临时协调点","controller":"wedding_hosts"},
					{"id":"station_gate","name":"车站外","kind":"transit","description":"接站车与代驾聚集的混乱出口","controller":"station_runners"},
					{"id":"platform_hall","name":"候车厅","kind":"waiting","description":"旅客和保安都不想久留的大厅","controller":"station_runners"}
				],
				"factions":[
					{"id":"wedding_hosts","name":"婚礼主家","role":"family","relationships":["压制 station_runners"]},
					{"id":"station_runners","name":"接站跑腿圈","role":"logistics","relationships":["不信任 wedding_hosts"]}
				],
				"pressures":[
					{"id":"pickup_delay","name":"接站迟到","kind":"coordination","description":"婚礼前的接站安排持续失序","intensity":0.84,"target":"wedding_hosts"},
					{"id":"arrival_gossip","name":"站台风声","kind":"rumor","description":"谁被怠慢、谁在甩锅开始扩散","intensity":0.62,"target":"未知地点"}
				]
			}`,
			expectedLeader:     "婚礼管家",
			expectedPressureID: "arrival_gossip",
			expectTensionFloor: 0.50,
		},
		{
			id:        "dream-real",
			worldName: "《红楼梦》完整版、",
			sourceDir: "《红楼梦》完整版、-角色卡-202604190812",
			scene: core.SceneState{
				Location:    "未知地点",
				TimeOfDay:   "未知时间",
				Weather:     "未知天气",
				Characters:  []string{"薛宝钗", "玩家"},
				Description: "真实导入世界的默认闺阁场景",
			},
			populationBody: `{
				"background_npcs":[
					{"id":"yinger","name":"莺儿","role":"侍女","location":"未知地点","faction":"xue_house","traits":["机灵","知分寸"],"hooks":["替宝姑娘探听风声","不想让诗社话头失控"]},
					{"id":"housemaid","name":"婆子","role":"杂役","location":"回廊","faction":"rong_house","traits":["谨慎","嘴碎"],"hooks":["最怕传错话","担心被责罚"]},
					{"id":"page","name":"小厮","role":"跑腿","location":"书房外","faction":"poetry_circle","traits":["轻快","爱看热闹"],"hooks":["去回话","把诗社消息带错边"]}
				],
				"policy":{"promote_threshold":6.8,"major_threshold":11,"interaction_weight":3,"mention_weight":1,"event_weight":2,"relationship_weight":3,"scene_weight":2}
			}`,
			structureBody: `{
				"locations":[
					{"id":"boudoir","name":"未知地点","kind":"residence","description":"闺阁内室，消息传得不快却更要紧","controller":"xue_house"},
					{"id":"corridor","name":"回廊","kind":"transit","description":"丫鬟婆子擦身而过、最容易串话","controller":"rong_house"},
					{"id":"study_gate","name":"书房外","kind":"service","description":"回话与递帖都得经过的地方","controller":"poetry_circle"}
				],
				"factions":[
					{"id":"xue_house","name":"薛家房内","role":"household","relationships":["顾忌 rong_house"]},
					{"id":"rong_house","name":"荣府杂役","role":"household","relationships":["议论 xue_house"]},
					{"id":"poetry_circle","name":"诗社往来圈","role":"social","relationships":["牵动 xue_house"]}
				],
				"pressures":[
					{"id":"poetry_society","name":"诗社风声","kind":"social","description":"诗社流言让宝钗身边的人先紧张起来","intensity":0.81,"target":"xue_house"},
					{"id":"maids_whisper","name":"回廊私语","kind":"rumor","description":"回廊里关于谁该出面的话越传越偏","intensity":0.58,"target":"未知地点"}
				]
			}`,
			expectedLeader:     "莺儿",
			expectedPressureID: "maids_whisper",
			expectTensionFloor: 0.48,
		},
	}

	resolver := &mockResolver{defaultID: "neon-real", engines: map[string]RuntimeEngine{}}
	for _, sample := range samples {
		worldDir := filepath.Join(baseDir, sample.id+"-world")
		copyTestDir(t, filepath.Join("..", "..", "worlds", sample.sourceDir), worldDir)
		engine := newRealWorldRuntimeEngineForAPITest(
			t,
			filepath.Join(t.TempDir(), sample.id+".db"),
			filepath.Join(baseDir, sample.id+"-data"),
			worldDir,
			sample.id,
			sample.worldName,
			"API 真实世界目录长窗口验证",
			sample.scene,
		)
		resolver.engines[sample.id] = engine
	}

	s := NewServer(resolver.engines["neon-real"], resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	postJSON := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	getJSON := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	for _, sample := range samples {
		if sample.populationBody != "" {
			rec := postJSON("/api/population?instance_id="+sample.id, sample.populationBody)
			if rec.Code != http.StatusOK {
				t.Fatalf("POST /api/population?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
			}
		}
		if sample.structureBody != "" {
			rec := postJSON("/api/world-structure?instance_id="+sample.id, sample.structureBody)
			if rec.Code != http.StatusOK {
				t.Fatalf("POST /api/world-structure?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
			}
		}
		rec := postJSON("/api/sim/tick?instance_id="+sample.id, `{"count":120}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /api/sim/tick?instance_id=%s count=120 = %d body=%s", sample.id, rec.Code, rec.Body.String())
		}
	}

	results := map[string]map[string]string{}
	for _, sample := range samples {
		rec := getJSON("/api/sim/status?instance_id=" + sample.id)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/sim/status?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
		}
		var status map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
			t.Fatalf("decode sim/status %s: %v", sample.id, err)
		}
		rec = getJSON("/api/population-insights?instance_id=" + sample.id)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/population-insights?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
		}
		var insights core.PopulationInsights
		if err := json.NewDecoder(rec.Body).Decode(&insights); err != nil {
			t.Fatalf("decode population-insights %s: %v", sample.id, err)
		}
		promoted := make([]string, 0, len(insights.Promoted))
		for _, npc := range insights.Promoted {
			promoted = append(promoted, npc.Name)
		}
		sort.Strings(promoted)
		trajectory, _ := status["trajectory_summary"].([]interface{})
		trajectoryText := fmt.Sprintf("%v", trajectory)

		if status["tension"].(float64) < sample.expectTensionFloor {
			t.Fatalf("%s tension = %#v, want >= %.2f in real-world API sample", sample.id, status["tension"], sample.expectTensionFloor)
		}
		if !containsString(promoted, sample.expectedLeader) {
			t.Fatalf("%s promoted = %#v, want %s promoted through real-world API sample", sample.id, promoted, sample.expectedLeader)
		}
		if !strings.Contains(trajectoryText, sample.expectedPressureID) {
			t.Fatalf("%s trajectory = %s, want pressure %s in real-world API summary", sample.id, trajectoryText, sample.expectedPressureID)
		}

		results[sample.id] = map[string]string{
			"trajectory": trajectoryText,
			"promoted":   strings.Join(promoted, ","),
		}
	}

	if results["neon-real"]["trajectory"] == results["wedding-real"]["trajectory"] {
		t.Fatalf("neon vs wedding real-world API trajectory = %#v vs %#v, want divergent summaries across world families", results["neon-real"], results["wedding-real"])
	}
	if results["wedding-real"]["promoted"] == results["dream-real"]["promoted"] {
		t.Fatalf("wedding vs dream real-world API promoted = %#v vs %#v, want different promoted leaders across imported world families", results["wedding-real"], results["dream-real"])
	}
}

func TestAPIRealWorldDirectorySampleMatrixAcrossTwoHundredTicks(t *testing.T) {
	type sample struct {
		id                 string
		worldName          string
		sourceDir          string
		scene              core.SceneState
		populationBody     string
		structureBody      string
		expectedLeader     string
		expectedPressureID string
		expectTensionFloor float64
	}

	baseDir := t.TempDir()
	samples := []sample{
		{
			id:        "neon-real-200",
			worldName: "霓虹里街区",
			sourceDir: "neon_block",
			scene: core.SceneState{
				Location:    "旧街夜市",
				TimeOfDay:   "深夜",
				Weather:     "闷热有雨",
				Characters:  []string{"蓝姐", "谭叔", "玩家"},
				Description: "真实世界目录中的默认夜市场景",
			},
			expectedLeader:     "蓝姐",
			expectedPressureID: "missing_rider",
			expectTensionFloor: 0.45,
		},
		{
			id:        "wedding-real-200",
			worldName: "新婚",
			sourceDir: "1_7",
			scene: core.SceneState{
				Location:    "未知地点",
				TimeOfDay:   "白天",
				Weather:     "阴雨",
				Characters:  []string{"许灵_单阶段人设", "玩家"},
				Description: "真实导入世界的默认接站场景",
			},
			populationBody: `{
				"background_npcs":[
					{"id":"steward","name":"婚礼管家","role":"steward","location":"未知地点","faction":"wedding_hosts","traits":["周到","急切"],"hooks":["要把迟到接站压下去","不想婚礼前出乱子"]},
					{"id":"driver","name":"代驾老周","role":"driver","location":"车站外","faction":"station_runners","traits":["疲惫","圆滑"],"hooks":["谁临时改了接站安排","想把责任甩出去"]},
					{"id":"guard","name":"站台保安","role":"guard","location":"候车厅","faction":"station_runners","traits":["谨慎","怕麻烦"],"hooks":["担心现场起争执","不想事情闹大"]}
				],
				"policy":{"promote_threshold":6.8,"major_threshold":11,"interaction_weight":3,"mention_weight":1,"event_weight":2,"relationship_weight":3,"scene_weight":2}
			}`,
			structureBody: `{
				"locations":[
					{"id":"arrival_point","name":"未知地点","kind":"arrival","description":"婚礼接站与临时协调点","controller":"wedding_hosts"},
					{"id":"station_gate","name":"车站外","kind":"transit","description":"接站车与代驾聚集的混乱出口","controller":"station_runners"},
					{"id":"platform_hall","name":"候车厅","kind":"waiting","description":"旅客和保安都不想久留的大厅","controller":"station_runners"}
				],
				"factions":[
					{"id":"wedding_hosts","name":"婚礼主家","role":"family","relationships":["压制 station_runners"]},
					{"id":"station_runners","name":"接站跑腿圈","role":"logistics","relationships":["不信任 wedding_hosts"]}
				],
				"pressures":[
					{"id":"pickup_delay","name":"接站迟到","kind":"coordination","description":"婚礼前的接站安排持续失序","intensity":0.84,"target":"wedding_hosts"},
					{"id":"arrival_gossip","name":"站台风声","kind":"rumor","description":"谁被怠慢、谁在甩锅开始扩散","intensity":0.62,"target":"未知地点"}
				]
			}`,
			expectedLeader:     "婚礼管家",
			expectedPressureID: "arrival_gossip",
			expectTensionFloor: 0.50,
		},
		{
			id:        "dream-real-200",
			worldName: "《红楼梦》完整版、",
			sourceDir: "《红楼梦》完整版、-角色卡-202604190812",
			scene: core.SceneState{
				Location:    "未知地点",
				TimeOfDay:   "未知时间",
				Weather:     "未知天气",
				Characters:  []string{"薛宝钗", "玩家"},
				Description: "真实导入世界的默认闺阁场景",
			},
			populationBody: `{
				"background_npcs":[
					{"id":"yinger","name":"莺儿","role":"侍女","location":"未知地点","faction":"xue_house","traits":["机灵","知分寸"],"hooks":["替宝姑娘探听风声","不想让诗社话头失控"]},
					{"id":"housemaid","name":"婆子","role":"杂役","location":"回廊","faction":"rong_house","traits":["谨慎","嘴碎"],"hooks":["最怕传错话","担心被责罚"]},
					{"id":"page","name":"小厮","role":"跑腿","location":"书房外","faction":"poetry_circle","traits":["轻快","爱看热闹"],"hooks":["去回话","把诗社消息带错边"]}
				],
				"policy":{"promote_threshold":6.8,"major_threshold":11,"interaction_weight":3,"mention_weight":1,"event_weight":2,"relationship_weight":3,"scene_weight":2}
			}`,
			structureBody: `{
				"locations":[
					{"id":"boudoir","name":"未知地点","kind":"residence","description":"闺阁内室，消息传得不快却更要紧","controller":"xue_house"},
					{"id":"corridor","name":"回廊","kind":"transit","description":"丫鬟婆子擦身而过、最容易串话","controller":"rong_house"},
					{"id":"study_gate","name":"书房外","kind":"service","description":"回话与递帖都得经过的地方","controller":"poetry_circle"}
				],
				"factions":[
					{"id":"xue_house","name":"薛家房内","role":"household","relationships":["顾忌 rong_house"]},
					{"id":"rong_house","name":"荣府杂役","role":"household","relationships":["议论 xue_house"]},
					{"id":"poetry_circle","name":"诗社往来圈","role":"social","relationships":["牵动 xue_house"]}
				],
				"pressures":[
					{"id":"poetry_society","name":"诗社风声","kind":"social","description":"诗社流言让宝钗身边的人先紧张起来","intensity":0.81,"target":"xue_house"},
					{"id":"maids_whisper","name":"回廊私语","kind":"rumor","description":"回廊里关于谁该出面的话越传越偏","intensity":0.58,"target":"未知地点"}
				]
			}`,
			expectedLeader:     "莺儿",
			expectedPressureID: "maids_whisper",
			expectTensionFloor: 0.48,
		},
		{
			id:        "campus-real-200",
			worldName: "校园别墅",
			sourceDir: "48111430a81be7d4",
			scene: core.SceneState{
				Location:    "别墅",
				TimeOfDay:   "白天",
				Weather:     "晴朗炎热",
				Characters:  []string{"赵小亮", "玩家", "沈佳"},
				Description: "真实世界目录中的校园别墅场景",
			},
			structureBody: `{
				"locations":[
					{"id":"villa","name":"别墅","kind":"residence","description":"三层建筑住宅，玩家与赵小亮的活动中心","controller":"villa_family"},
					{"id":"school","name":"明南高中","kind":"school","description":"半封闭式管理学校，设有高二4班","controller":"school_faculty"},
					{"id":"village","name":"上溪村","kind":"rural","description":"偏远乡下，玩家奶奶居住地","controller":"village_locals"}
				],
				"factions":[
					{"id":"villa_family","name":"别墅家庭","role":"household","relationships":["顾忌 school_faculty"]},
					{"id":"school_faculty","name":"明南高中教职工","role":"authority","relationships":["监督 villa_family"]},
					{"id":"village_locals","name":"上溪村村民","role":"community","relationships":["远离 villa_family"]}
				],
				"pressures":[
					{"id":"family_secret","name":"家庭秘密","kind":"domestic","description":"别墅内的异常关系引发暗流","intensity":0.75,"target":"villa_family"},
					{"id":"school_rumors","name":"校园传闻","kind":"rumor","description":"明南高中关于玩家的传闻开始扩散","intensity":0.60,"target":"别墅"}
				]
			}`,
			expectedLeader:     "别墅巡守",
			expectedPressureID: "family_secret",
			expectTensionFloor: 0.42,
		},
		{
			id:        "stream-real-200",
			worldName: "直播顶层",
			sourceDir: "a0c85d27e38863a4",
			scene: core.SceneState{
				Location:    "客厅",
				TimeOfDay:   "夜晚",
				Weather:     "阴雨",
				Characters:  []string{"ANJONI小玖", "玩家"},
				Description: "真实世界目录中的直播顶层场景",
			},
			structureBody: `{
				"locations":[
					{"id":"penthouse","name":"客厅","kind":"residence","description":"汤臣一品顶层大平层，直播与社交中心","controller":"streamer_team"},
					{"id":"stream_room","name":"直播间","kind":"workspace","description":"专业直播设备与拍摄区域","controller":"streamer_team"},
					{"id":"backstage","name":"后台","kind":"logistics","description":"运营团队与来访者等候区","controller":"rival_streamers"}
				],
				"factions":[
					{"id":"streamer_team","name":"主播团队","role":"content","relationships":["防范 rival_streamers"]},
					{"id":"rival_streamers","name":"竞品主播","role":"competition","relationships":["觊觎 streamer_team 流量"]}
				],
				"pressures":[
					{"id":"platform_riot","name":"平台风波","kind":"crisis","description":"斗鱼平台政策变动引发主播圈动荡","intensity":0.78,"target":"streamer_team"},
					{"id":"stream_rivalry","name":"直播竞争","kind":"competition","description":"竞品主播暗中挖角与流量争夺","intensity":0.65,"target":"客厅"}
				]
			}`,
			expectedLeader:     "客厅巡守",
			expectedPressureID: "platform_riot",
			expectTensionFloor: 0.42,
		},
	}

	resolver := &mockResolver{defaultID: "neon-real-200", engines: map[string]RuntimeEngine{}}
	for _, sample := range samples {
		worldDir := filepath.Join(baseDir, sample.id+"-world")
		copyTestDir(t, filepath.Join("..", "..", "worlds", sample.sourceDir), worldDir)
		engine := newRealWorldRuntimeEngineForAPITest(
			t,
			filepath.Join(t.TempDir(), sample.id+".db"),
			filepath.Join(baseDir, sample.id+"-data"),
			worldDir,
			sample.id,
			sample.worldName,
			"API 真实世界目录 200 tick 长窗口验证",
			sample.scene,
		)
		resolver.engines[sample.id] = engine
	}

	s := NewServer(resolver.engines["neon-real-200"], resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	postJSON := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	getJSON := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	for _, sample := range samples {
		if sample.populationBody != "" {
			rec := postJSON("/api/population?instance_id="+sample.id, sample.populationBody)
			if rec.Code != http.StatusOK {
				t.Fatalf("POST /api/population?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
			}
		}
		if sample.structureBody != "" {
			rec := postJSON("/api/world-structure?instance_id="+sample.id, sample.structureBody)
			if rec.Code != http.StatusOK {
				t.Fatalf("POST /api/world-structure?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
			}
		}
		rec := postJSON("/api/sim/tick?instance_id="+sample.id, `{"count":200}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /api/sim/tick?instance_id=%s count=200 = %d body=%s", sample.id, rec.Code, rec.Body.String())
		}
	}

	results := map[string]map[string]string{}
	for _, sample := range samples {
		rec := getJSON("/api/sim/status?instance_id=" + sample.id)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/sim/status?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
		}
		var status map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
			t.Fatalf("decode sim/status %s: %v", sample.id, err)
		}
		trajectory, ok := status["trajectory_summary"].([]interface{})
		if !ok || len(trajectory) == 0 {
			t.Fatalf("%s trajectory_summary = %#v, want API long-window summary after 200 ticks", sample.id, status["trajectory_summary"])
		}
		tickHistory, ok := status["tick_history"].([]interface{})
		if !ok || len(tickHistory) != 12 {
			t.Fatalf("%s tick_history = %#v, want capped API recent snapshots after 200 ticks", sample.id, status["tick_history"])
		}

		rec = getJSON("/api/population-insights?instance_id=" + sample.id)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/population-insights?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
		}
		var insights core.PopulationInsights
		if err := json.NewDecoder(rec.Body).Decode(&insights); err != nil {
			t.Fatalf("decode population-insights %s: %v", sample.id, err)
		}
		promoted := make([]string, 0, len(insights.Promoted))
		for _, npc := range insights.Promoted {
			promoted = append(promoted, npc.Name)
		}
		sort.Strings(promoted)
		trajectoryText := fmt.Sprintf("%v", trajectory)

		if status["tension"].(float64) < sample.expectTensionFloor {
			t.Fatalf("%s tension = %#v, want >= %.2f in real-world 200-tick API sample", sample.id, status["tension"], sample.expectTensionFloor)
		}
		if !containsString(promoted, sample.expectedLeader) {
			t.Fatalf("%s promoted = %#v, want %s promoted through real-world 200-tick API sample", sample.id, promoted, sample.expectedLeader)
		}
		if !strings.Contains(trajectoryText, sample.expectedPressureID) {
			t.Fatalf("%s trajectory = %s, want pressure %s in real-world 200-tick API summary", sample.id, trajectoryText, sample.expectedPressureID)
		}

		results[sample.id] = map[string]string{
			"trajectory": trajectoryText,
			"promoted":   strings.Join(promoted, ","),
		}
	}

	if results["neon-real-200"]["trajectory"] == results["wedding-real-200"]["trajectory"] {
		t.Fatalf("neon vs wedding real-world 200-tick API trajectory = %#v vs %#v, want divergent summaries across world families", results["neon-real-200"], results["wedding-real-200"])
	}
	if results["wedding-real-200"]["promoted"] == results["dream-real-200"]["promoted"] {
		t.Fatalf("wedding vs dream real-world 200-tick API promoted = %#v vs %#v, want different promoted leaders across imported world families", results["wedding-real-200"], results["dream-real-200"])
	}
}

func TestAPIRealWorldDirectoryStabilityAcrossFiveHundredTicks(t *testing.T) {
	if os.Getenv("CORERP_RUN_SLOW_PROOF_TESTS") != "1" {
		t.Skip("set CORERP_RUN_SLOW_PROOF_TESTS=1 to run 500 tick proof audit stability test")
	}

	type sample struct {
		id                 string
		worldName          string
		sourceDir          string
		scene              core.SceneState
		populationBody     string
		structureBody      string
		expectedLeader     string
		expectedPressureID string
		expectTensionFloor float64
	}

	baseDir := t.TempDir()
	samples := []sample{
		{
			id:        "neon-stability-500",
			worldName: "霓虹里街区",
			sourceDir: "neon_block",
			scene: core.SceneState{
				Location:    "旧街夜市",
				TimeOfDay:   "深夜",
				Weather:     "闷热有雨",
				Characters:  []string{"蓝姐", "谭叔", "玩家"},
				Description: "真实世界目录中的默认夜市场景",
			},
			expectedLeader:     "蓝姐",
			expectedPressureID: "missing_rider",
			expectTensionFloor: 0.45,
		},
		{
			id:        "wedding-stability-500",
			worldName: "新婚",
			sourceDir: "1_7",
			scene: core.SceneState{
				Location:    "未知地点",
				TimeOfDay:   "白天",
				Weather:     "阴雨",
				Characters:  []string{"许灵_单阶段人设", "玩家"},
				Description: "真实导入世界的默认接站场景",
			},
			populationBody: `{
				"background_npcs":[
					{"id":"steward","name":"婚礼管家","role":"steward","location":"未知地点","faction":"wedding_hosts","traits":["周到","急切"],"hooks":["要把迟到接站压下去","不想婚礼前出乱子"]},
					{"id":"driver","name":"代驾老周","role":"driver","location":"车站外","faction":"station_runners","traits":["疲惫","圆滑"],"hooks":["谁临时改了接站安排","想把责任甩出去"]},
					{"id":"guard","name":"站台保安","role":"guard","location":"候车厅","faction":"station_runners","traits":["谨慎","怕麻烦"],"hooks":["担心现场起争执","不想事情闹大"]}
				],
				"policy":{"promote_threshold":6.8,"major_threshold":11,"interaction_weight":3,"mention_weight":1,"event_weight":2,"relationship_weight":3,"scene_weight":2}
			}`,
			structureBody: `{
				"locations":[
					{"id":"arrival_point","name":"未知地点","kind":"arrival","description":"婚礼接站与临时协调点","controller":"wedding_hosts"},
					{"id":"station_gate","name":"车站外","kind":"transit","description":"接站车与代驾聚集的混乱出口","controller":"station_runners"},
					{"id":"platform_hall","name":"候车厅","kind":"waiting","description":"旅客和保安都不想久留的大厅","controller":"station_runners"}
				],
				"factions":[
					{"id":"wedding_hosts","name":"婚礼主家","role":"family","relationships":["压制 station_runners"]},
					{"id":"station_runners","name":"接站跑腿圈","role":"logistics","relationships":["不信任 wedding_hosts"]}
				],
				"pressures":[
					{"id":"pickup_delay","name":"接站迟到","kind":"coordination","description":"婚礼前的接站安排持续失序","intensity":0.84,"target":"wedding_hosts"},
					{"id":"arrival_gossip","name":"站台风声","kind":"rumor","description":"谁被怠慢、谁在甩锅开始扩散","intensity":0.62,"target":"未知地点"}
				]
			}`,
			expectedLeader:     "婚礼管家",
			expectedPressureID: "arrival_gossip",
			expectTensionFloor: 0.50,
		},
		{
			id:        "dream-stability-500",
			worldName: "《红楼梦》完整版、",
			sourceDir: "《红楼梦》完整版、-角色卡-202604190812",
			scene: core.SceneState{
				Location:    "未知地点",
				TimeOfDay:   "未知时间",
				Weather:     "未知天气",
				Characters:  []string{"薛宝钗", "玩家"},
				Description: "真实导入世界的默认闺阁场景",
			},
			populationBody: `{
				"background_npcs":[
					{"id":"yinger","name":"莺儿","role":"侍女","location":"未知地点","faction":"xue_house","traits":["机灵","知分寸"],"hooks":["替宝姑娘探听风声","不想让诗社话头失控"]},
					{"id":"housemaid","name":"婆子","role":"杂役","location":"回廊","faction":"rong_house","traits":["谨慎","嘴碎"],"hooks":["最怕传错话","担心被责罚"]},
					{"id":"page","name":"小厮","role":"跑腿","location":"书房外","faction":"poetry_circle","traits":["轻快","爱看热闹"],"hooks":["去回话","把诗社消息带错边"]}
				],
				"policy":{"promote_threshold":6.8,"major_threshold":11,"interaction_weight":3,"mention_weight":1,"event_weight":2,"relationship_weight":3,"scene_weight":2}
			}`,
			structureBody: `{
				"locations":[
					{"id":"boudoir","name":"未知地点","kind":"residence","description":"闺阁内室，消息传得不快却更要紧","controller":"xue_house"},
					{"id":"corridor","name":"回廊","kind":"transit","description":"丫鬟婆子擦身而过、最容易串话","controller":"rong_house"},
					{"id":"study_gate","name":"书房外","kind":"service","description":"回话与递帖都得经过的地方","controller":"poetry_circle"}
				],
				"factions":[
					{"id":"xue_house","name":"薛家房内","role":"household","relationships":["顾忌 rong_house"]},
					{"id":"rong_house","name":"荣府杂役","role":"household","relationships":["议论 xue_house"]},
					{"id":"poetry_circle","name":"诗社往来圈","role":"social","relationships":["牵动 xue_house"]}
				],
				"pressures":[
					{"id":"poetry_society","name":"诗社风声","kind":"social","description":"诗社流言让宝钗身边的人先紧张起来","intensity":0.81,"target":"xue_house"},
					{"id":"maids_whisper","name":"回廊私语","kind":"rumor","description":"回廊里关于谁该出面的话越传越偏","intensity":0.58,"target":"未知地点"}
				]
			}`,
			expectedLeader:     "莺儿",
			expectedPressureID: "maids_whisper",
			expectTensionFloor: 0.48,
		},
		{
			id:        "campus-stability-500",
			worldName: "校园别墅",
			sourceDir: "48111430a81be7d4",
			scene: core.SceneState{
				Location:    "别墅",
				TimeOfDay:   "白天",
				Weather:     "晴朗炎热",
				Characters:  []string{"赵小亮", "玩家", "沈佳"},
				Description: "真实世界目录中的校园别墅场景",
			},
			structureBody: `{
				"locations":[
					{"id":"villa","name":"别墅","kind":"residence","description":"三层建筑住宅，玩家与赵小亮的活动中心","controller":"villa_family"},
					{"id":"school","name":"明南高中","kind":"school","description":"半封闭式管理学校，设有高二4班","controller":"school_faculty"},
					{"id":"village","name":"上溪村","kind":"rural","description":"偏远乡下，玩家奶奶居住地","controller":"village_locals"}
				],
				"factions":[
					{"id":"villa_family","name":"别墅家庭","role":"household","relationships":["顾忌 school_faculty"]},
					{"id":"school_faculty","name":"明南高中教职工","role":"authority","relationships":["监督 villa_family"]},
					{"id":"village_locals","name":"上溪村村民","role":"community","relationships":["远离 villa_family"]}
				],
				"pressures":[
					{"id":"family_secret","name":"家庭秘密","kind":"domestic","description":"别墅内的异常关系引发暗流","intensity":0.75,"target":"villa_family"},
					{"id":"school_rumors","name":"校园传闻","kind":"rumor","description":"明南高中关于玩家的传闻开始扩散","intensity":0.60,"target":"别墅"}
				]
			}`,
			expectedLeader:     "别墅巡守",
			expectedPressureID: "family_secret",
			expectTensionFloor: 0.42,
		},
		{
			id:        "stream-stability-500",
			worldName: "直播顶层",
			sourceDir: "a0c85d27e38863a4",
			scene: core.SceneState{
				Location:    "客厅",
				TimeOfDay:   "夜晚",
				Weather:     "阴雨",
				Characters:  []string{"ANJONI小玖", "玩家"},
				Description: "真实世界目录中的直播顶层场景",
			},
			structureBody: `{
				"locations":[
					{"id":"penthouse","name":"客厅","kind":"residence","description":"汤臣一品顶层大平层，直播与社交中心","controller":"streamer_team"},
					{"id":"stream_room","name":"直播间","kind":"workspace","description":"专业直播设备与拍摄区域","controller":"streamer_team"},
					{"id":"backstage","name":"后台","kind":"logistics","description":"运营团队与来访者等候区","controller":"rival_streamers"}
				],
				"factions":[
					{"id":"streamer_team","name":"主播团队","role":"content","relationships":["防范 rival_streamers"]},
					{"id":"rival_streamers","name":"竞品主播","role":"competition","relationships":["觊觎 streamer_team 流量"]}
				],
				"pressures":[
					{"id":"platform_riot","name":"平台风波","kind":"crisis","description":"斗鱼平台政策变动引发主播圈动荡","intensity":0.78,"target":"streamer_team"},
					{"id":"stream_rivalry","name":"直播竞争","kind":"competition","description":"竞品主播暗中挖角与流量争夺","intensity":0.65,"target":"客厅"}
				]
			}`,
			expectedLeader:     "客厅巡守",
			expectedPressureID: "platform_riot",
			expectTensionFloor: 0.42,
		},
	}

	resolver := &mockResolver{defaultID: "neon-stability-500", engines: map[string]RuntimeEngine{}}
	for _, sample := range samples {
		worldDir := filepath.Join(baseDir, sample.id+"-world")
		copyTestDir(t, filepath.Join("..", "..", "worlds", sample.sourceDir), worldDir)
		engine := newRealWorldRuntimeEngineForAPITest(
			t,
			filepath.Join(t.TempDir(), sample.id+".db"),
			filepath.Join(baseDir, sample.id+"-data"),
			worldDir,
			sample.id,
			sample.worldName,
			"API 真实世界目录 500 tick 长窗口稳定性验证",
			sample.scene,
		)
		resolver.engines[sample.id] = engine
	}

	s := NewServer(resolver.engines["neon-stability-500"], resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	postJSON := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	getJSON := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	for _, sample := range samples {
		if sample.populationBody != "" {
			rec := postJSON("/api/population?instance_id="+sample.id, sample.populationBody)
			if rec.Code != http.StatusOK {
				t.Fatalf("POST /api/population?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
			}
		}
		if sample.structureBody != "" {
			rec := postJSON("/api/world-structure?instance_id="+sample.id, sample.structureBody)
			if rec.Code != http.StatusOK {
				t.Fatalf("POST /api/world-structure?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
			}
		}
		for _, count := range []int{200, 200, 100} {
			rec := postJSON(fmt.Sprintf("/api/sim/tick?instance_id=%s", sample.id), fmt.Sprintf(`{"count":%d}`, count))
			if rec.Code != http.StatusOK {
				t.Fatalf("POST /api/sim/tick?instance_id=%s count=%d = %d body=%s", sample.id, count, rec.Code, rec.Body.String())
			}
		}
	}

	results := map[string]map[string]string{}
	for _, sample := range samples {
		rec := getJSON("/api/sim/status?instance_id=" + sample.id)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/sim/status?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
		}
		var status map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
			t.Fatalf("decode sim/status %s: %v", sample.id, err)
		}
		trajectory, ok := status["trajectory_summary"].([]interface{})
		if !ok || len(trajectory) == 0 {
			t.Fatalf("%s trajectory_summary = %#v, want API long-window summary after 500 ticks", sample.id, status["trajectory_summary"])
		}
		tickHistory, ok := status["tick_history"].([]interface{})
		if !ok || len(tickHistory) != 12 {
			t.Fatalf("%s tick_history = %#v, want capped API recent snapshots after 500 ticks", sample.id, status["tick_history"])
		}

		rec = getJSON("/api/population-insights?instance_id=" + sample.id)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/population-insights?instance_id=%s = %d body=%s", sample.id, rec.Code, rec.Body.String())
		}
		var insights core.PopulationInsights
		if err := json.NewDecoder(rec.Body).Decode(&insights); err != nil {
			t.Fatalf("decode population-insights %s: %v", sample.id, err)
		}
		promoted := make([]string, 0, len(insights.Promoted))
		for _, npc := range insights.Promoted {
			promoted = append(promoted, npc.Name)
		}
		sort.Strings(promoted)
		trajectoryText := fmt.Sprintf("%v", trajectory)

		if status["tension"].(float64) < sample.expectTensionFloor {
			t.Fatalf("%s tension = %#v, want >= %.2f in real-world 500-tick API stability sample", sample.id, status["tension"], sample.expectTensionFloor)
		}
		if !containsString(promoted, sample.expectedLeader) {
			t.Fatalf("%s promoted = %#v, want %s promoted through real-world 500-tick API stability sample", sample.id, promoted, sample.expectedLeader)
		}
		if !strings.Contains(trajectoryText, sample.expectedPressureID) {
			t.Fatalf("%s trajectory = %s, want pressure %s in real-world 500-tick API stability summary", sample.id, trajectoryText, sample.expectedPressureID)
		}

		results[sample.id] = map[string]string{
			"trajectory": trajectoryText,
			"promoted":   strings.Join(promoted, ","),
		}
	}

	if results["neon-stability-500"]["trajectory"] == results["wedding-stability-500"]["trajectory"] {
		t.Fatalf("neon vs wedding 500-tick API stability trajectory = %#v vs %#v, want divergent summaries", results["neon-stability-500"], results["wedding-stability-500"])
	}
	if results["wedding-stability-500"]["promoted"] == results["dream-stability-500"]["promoted"] {
		t.Fatalf("wedding vs dream 500-tick API stability promoted = %#v vs %#v, want different leaders", results["wedding-stability-500"], results["dream-stability-500"])
	}
}

func TestCharactersRouteUsesParticipantsView(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/characters", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/characters = %d", rec.Code)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode /api/characters: %v", err)
	}
	if payload["focus_character"] != "Anya" {
		t.Fatalf("focus_character = %#v, want Anya", payload["focus_character"])
	}
	if _, ok := payload["active"]; ok {
		t.Fatalf("active compatibility mirror should be absent on /api/characters payload = %#v", payload)
	}
	participants, ok := payload["participants"].([]interface{})
	if !ok || len(participants) != 1 || participants[0] != "Anya" {
		t.Fatalf("participants = %#v, want [Anya]", payload["participants"])
	}
	details, ok := payload["participant_details"].([]interface{})
	if !ok || len(details) != 1 {
		t.Fatalf("participant_details = %#v, want 1 item", payload["participant_details"])
	}
	detail, ok := details[0].(map[string]interface{})
	if !ok || detail["name"] != "Anya" {
		t.Fatalf("participant_details[0] = %#v, want name=Anya", details[0])
	}
	if _, ok := payload["characters"]; ok {
		t.Fatalf("characters compatibility mirror should be absent on /api/characters payload = %#v", payload)
	}
}

func TestMemoryRoutePrefersFocusCharacterOverLegacyCharacter(t *testing.T) {
	s := newTestServer()
	engine := s.engine.(*mockEngine)
	engine.memorySnapshot = &core.MemorySnapshot{
		Character:      "LegacyName",
		FocusCharacter: "FocusName",
		WorkingMemory:  "working",
	}
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/memory?focus_character=FocusName", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/memory = %d", rec.Code)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode /api/memory: %v", err)
	}
	if payload["focus_character"] != "FocusName" {
		t.Fatalf("focus_character = %#v, want FocusName", payload["focus_character"])
	}
	if _, ok := payload["character"]; ok {
		t.Fatalf("top-level character compatibility mirror should be absent on /api/memory payload = %#v", payload)
	}
}

func TestPendingFactsRoutePrefersFocusCharacterOverLegacyCharacter(t *testing.T) {
	s := newTestServer()
	engine := s.engine.(*mockEngine)
	engine.pending = []core.PendingFact{{
		ID:             "p1",
		Character:      "LegacyName",
		FocusCharacter: "FocusName",
		Subject:        "V",
		Predicate:      "身份",
		Object:         "佣兵",
		Source:         "llm_extracted",
		Confidence:     0.4,
	}}
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/pending-facts?focus_character=FocusName", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/pending-facts = %d", rec.Code)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode /api/pending-facts: %v", err)
	}
	if payload["focus_character"] != "FocusName" {
		t.Fatalf("focus_character = %#v, want FocusName", payload["focus_character"])
	}
	if _, ok := payload["character"]; ok {
		t.Fatalf("top-level character compatibility mirror should be absent on /api/pending-facts payload = %#v", payload)
	}
	facts, ok := payload["facts"].([]interface{})
	if !ok || len(facts) != 1 {
		t.Fatalf("facts = %#v, want 1 item", payload["facts"])
	}
	fact, ok := facts[0].(map[string]interface{})
	if !ok {
		t.Fatalf("facts[0] = %#v, want object", facts[0])
	}
	if fact["focus_character"] != "FocusName" {
		t.Fatalf("facts[0].focus_character = %#v, want FocusName", fact["focus_character"])
	}
	if _, ok := fact["character"]; ok {
		t.Fatalf("facts[0].character mirror should be absent on /api/pending-facts payload = %#v", fact)
	}
}

func TestCharactersRouteDoesNotFallbackToLoadedCharacters(t *testing.T) {
	engine := &mockEngine{
		instanceID:         "default",
		name:               "Anya",
		state:              core.WorldState{},
		participantDetails: nil,
		sceneParticipants:  []string{},
		loadedCharacters:   []string{"Anya"},
	}
	s := NewServer(engine)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/characters", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/characters = %d", rec.Code)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode /api/characters: %v", err)
	}
	if participants, ok := payload["participants"].([]interface{}); ok && len(participants) != 0 {
		t.Fatalf("participants = %#v, want empty without scene participants", payload["participants"])
	}
	if _, ok := payload["characters"]; ok {
		t.Fatalf("characters compatibility mirror should be absent on /api/characters payload = %#v", payload)
	}
}

func TestDirectorConfigRoute(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	getReq := httptest.NewRequest(http.MethodGet, "/api/director-config", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/director-config = %d", getRec.Code)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/api/director-config", strings.NewReader(`{"mode":"auto_single","max_speakers":1,"weights":{"mentioned":55,"pressure_match":9}}`))
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	mux.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST /api/director-config = %d", postRec.Code)
	}
}

func TestTraceRoute(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/trace/latest", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/trace/latest = %d", rec.Code)
	}
	var latest struct {
		FocusCharacter string `json:"focus_character"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &latest); err != nil {
		t.Fatalf("decode /api/trace/latest: %v", err)
	}
	if latest.FocusCharacter != "Anya" {
		t.Fatalf("latest focus_character = %#v, want Anya", latest.FocusCharacter)
	}
	var latestRaw map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &latestRaw); err != nil {
		t.Fatalf("decode raw /api/trace/latest: %v", err)
	}
	if _, ok := latestRaw["character"]; ok {
		t.Fatalf("/api/trace/latest should not expose empty legacy character mirror: %#v", latestRaw)
	}

	reqByTurn := httptest.NewRequest(http.MethodGet, "/api/trace?turn=3", nil)
	recByTurn := httptest.NewRecorder()
	mux.ServeHTTP(recByTurn, reqByTurn)
	if recByTurn.Code != http.StatusOK {
		t.Fatalf("GET /api/trace?turn=3 = %d", recByTurn.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/traces?limit=2", nil)
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GET /api/traces?limit=2 = %d", listRec.Code)
	}
	var listed struct {
		Traces []struct {
			FocusCharacter string `json:"focus_character"`
		} `json:"traces"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode /api/traces: %v", err)
	}
	if len(listed.Traces) == 0 {
		t.Fatalf("traces = %#v, want items", listed.Traces)
	}
	if listed.Traces[0].FocusCharacter != "Anya" {
		t.Fatalf("trace focus_character = %#v, want Anya", listed.Traces[0].FocusCharacter)
	}
	var listedRaw struct {
		Traces []map[string]interface{} `json:"traces"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listedRaw); err != nil {
		t.Fatalf("decode raw /api/traces: %v", err)
	}
	if _, ok := listedRaw.Traces[0]["character"]; ok {
		t.Fatalf("/api/traces should not expose empty legacy character mirror: %#v", listedRaw.Traces[0])
	}
}

func TestTraceRoutesNormalizeLegacyCharacterFields(t *testing.T) {
	s := newTestServer()
	engine := s.engine.(*mockEngine)
	engine.trace = core.TurnTrace{
		Turn:      4,
		Character: "LegacyFocus",
		UserInput: "legacy",
		StepTraces: []core.TurnStepTrace{{
			Character: "LegacySpeaker",
		}},
	}
	engine.traces = []core.TurnTrace{engine.trace}
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/trace/latest", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/trace/latest legacy = %d", rec.Code)
	}
	var latestRaw map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &latestRaw); err != nil {
		t.Fatalf("decode legacy trace: %v", err)
	}
	if latestRaw["focus_character"] != "LegacyFocus" {
		t.Fatalf("legacy trace focus_character = %#v, want LegacyFocus", latestRaw["focus_character"])
	}
	if _, ok := latestRaw["character"]; ok {
		t.Fatalf("legacy trace should not expose character mirror: %#v", latestRaw)
	}
	steps, ok := latestRaw["step_traces"].([]interface{})
	if !ok || len(steps) != 1 {
		t.Fatalf("legacy trace step_traces = %#v, want 1", latestRaw["step_traces"])
	}
	step, ok := steps[0].(map[string]interface{})
	if !ok {
		t.Fatalf("legacy trace step = %#v, want object", steps[0])
	}
	if _, ok := step["character"]; ok {
		t.Fatalf("legacy step trace should not expose character mirror: %#v", step)
	}
	stepMeta, ok := step["step"].(map[string]interface{})
	if !ok || stepMeta["speaker"] != "LegacySpeaker" {
		t.Fatalf("legacy step speaker = %#v, want LegacySpeaker", step["step"])
	}
}

func TestCheckpointAndPresetRoutes(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)
	assertNoLegacyCharacter := func(label string, body []byte) {
		t.Helper()
		var raw map[string]interface{}
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatalf("decode raw %s: %v", label, err)
		}
		if _, ok := raw["character"]; ok {
			t.Fatalf("%s unexpectedly exposes legacy character mirror: %#v", label, raw)
		}
	}

	getCheckpoints := httptest.NewRequest(http.MethodGet, "/api/checkpoints", nil)
	getCheckpointsRec := httptest.NewRecorder()
	mux.ServeHTTP(getCheckpointsRec, getCheckpoints)
	if getCheckpointsRec.Code != http.StatusOK {
		t.Fatalf("GET /api/checkpoints = %d", getCheckpointsRec.Code)
	}
	var checkpointsPayload struct {
		Checkpoints []struct {
			FocusCharacter string `json:"focus_character"`
		} `json:"checkpoints"`
	}
	if err := json.Unmarshal(getCheckpointsRec.Body.Bytes(), &checkpointsPayload); err != nil {
		t.Fatalf("decode /api/checkpoints: %v", err)
	}
	if len(checkpointsPayload.Checkpoints) != 1 {
		t.Fatalf("checkpoints = %#v, want 1 item", checkpointsPayload.Checkpoints)
	}
	if checkpointsPayload.Checkpoints[0].FocusCharacter != "Anya" {
		t.Fatalf("checkpoint focus_character = %#v, want Anya", checkpointsPayload.Checkpoints[0].FocusCharacter)
	}
	var checkpointsRaw struct {
		Checkpoints []map[string]interface{} `json:"checkpoints"`
	}
	if err := json.Unmarshal(getCheckpointsRec.Body.Bytes(), &checkpointsRaw); err != nil {
		t.Fatalf("decode raw /api/checkpoints: %v", err)
	}
	if _, ok := checkpointsRaw.Checkpoints[0]["character"]; ok {
		t.Fatalf("/api/checkpoints unexpectedly exposes legacy character mirror: %#v", checkpointsRaw.Checkpoints[0])
	}

	postCheckpoint := httptest.NewRequest(http.MethodPost, "/api/checkpoints", strings.NewReader(`{"name":"cp-a","branch":"main","note":"before risk"}`))
	postCheckpoint.Header.Set("Content-Type", "application/json")
	postCheckpointRec := httptest.NewRecorder()
	mux.ServeHTTP(postCheckpointRec, postCheckpoint)
	if postCheckpointRec.Code != http.StatusOK {
		t.Fatalf("POST /api/checkpoints = %d", postCheckpointRec.Code)
	}
	var checkpointCreated struct {
		FocusCharacter string `json:"focus_character"`
	}
	if err := json.Unmarshal(postCheckpointRec.Body.Bytes(), &checkpointCreated); err != nil {
		t.Fatalf("decode POST /api/checkpoints: %v", err)
	}
	if checkpointCreated.FocusCharacter != "Anya" {
		t.Fatalf("checkpoint focus_character = %#v, want Anya", checkpointCreated.FocusCharacter)
	}
	assertNoLegacyCharacter("POST /api/checkpoints", postCheckpointRec.Body.Bytes())

	loadCheckpoint := httptest.NewRequest(http.MethodPost, "/api/checkpoints/load", strings.NewReader(`{"name":"cp-a"}`))
	loadCheckpoint.Header.Set("Content-Type", "application/json")
	loadCheckpointRec := httptest.NewRecorder()
	mux.ServeHTTP(loadCheckpointRec, loadCheckpoint)
	if loadCheckpointRec.Code != http.StatusOK {
		t.Fatalf("POST /api/checkpoints/load = %d", loadCheckpointRec.Code)
	}
	var checkpointLoaded struct {
		FocusCharacter string `json:"focus_character"`
	}
	if err := json.Unmarshal(loadCheckpointRec.Body.Bytes(), &checkpointLoaded); err != nil {
		t.Fatalf("decode POST /api/checkpoints/load: %v", err)
	}
	if checkpointLoaded.FocusCharacter != "Anya" {
		t.Fatalf("checkpoint load focus_character = %#v, want Anya", checkpointLoaded.FocusCharacter)
	}
	assertNoLegacyCharacter("POST /api/checkpoints/load", loadCheckpointRec.Body.Bytes())

	getPresets := httptest.NewRequest(http.MethodGet, "/api/presets", nil)
	getPresetsRec := httptest.NewRecorder()
	mux.ServeHTTP(getPresetsRec, getPresets)
	if getPresetsRec.Code != http.StatusOK {
		t.Fatalf("GET /api/presets = %d", getPresetsRec.Code)
	}
	var presetsPayload struct {
		Presets []struct {
			FocusCharacter string `json:"focus_character"`
		} `json:"presets"`
	}
	if err := json.Unmarshal(getPresetsRec.Body.Bytes(), &presetsPayload); err != nil {
		t.Fatalf("decode /api/presets: %v", err)
	}
	if len(presetsPayload.Presets) != 1 {
		t.Fatalf("presets = %#v, want 1 item", presetsPayload.Presets)
	}
	if presetsPayload.Presets[0].FocusCharacter != "Anya" {
		t.Fatalf("preset focus_character = %#v, want Anya", presetsPayload.Presets[0].FocusCharacter)
	}
	var presetsRaw struct {
		Presets []map[string]interface{} `json:"presets"`
	}
	if err := json.Unmarshal(getPresetsRec.Body.Bytes(), &presetsRaw); err != nil {
		t.Fatalf("decode raw /api/presets: %v", err)
	}
	if _, ok := presetsRaw.Presets[0]["character"]; ok {
		t.Fatalf("/api/presets unexpectedly exposes legacy character mirror: %#v", presetsRaw.Presets[0])
	}

	postPreset := httptest.NewRequest(http.MethodPost, "/api/presets", strings.NewReader(`{"name":"opening","branch":"main","note":"intro"}`))
	postPreset.Header.Set("Content-Type", "application/json")
	postPresetRec := httptest.NewRecorder()
	mux.ServeHTTP(postPresetRec, postPreset)
	if postPresetRec.Code != http.StatusOK {
		t.Fatalf("POST /api/presets = %d", postPresetRec.Code)
	}
	var presetCreated struct {
		FocusCharacter string `json:"focus_character"`
	}
	if err := json.Unmarshal(postPresetRec.Body.Bytes(), &presetCreated); err != nil {
		t.Fatalf("decode POST /api/presets: %v", err)
	}
	if presetCreated.FocusCharacter != "Anya" {
		t.Fatalf("preset focus_character = %#v, want Anya", presetCreated.FocusCharacter)
	}
	assertNoLegacyCharacter("POST /api/presets", postPresetRec.Body.Bytes())

	applyPreset := httptest.NewRequest(http.MethodPost, "/api/presets/apply", strings.NewReader(`{"name":"opening"}`))
	applyPreset.Header.Set("Content-Type", "application/json")
	applyPresetRec := httptest.NewRecorder()
	mux.ServeHTTP(applyPresetRec, applyPreset)
	if applyPresetRec.Code != http.StatusOK {
		t.Fatalf("POST /api/presets/apply = %d", applyPresetRec.Code)
	}
	var presetApplied struct {
		FocusCharacter string `json:"focus_character"`
	}
	if err := json.Unmarshal(applyPresetRec.Body.Bytes(), &presetApplied); err != nil {
		t.Fatalf("decode POST /api/presets/apply: %v", err)
	}
	if presetApplied.FocusCharacter != "Anya" {
		t.Fatalf("preset apply focus_character = %#v, want Anya", presetApplied.FocusCharacter)
	}
	assertNoLegacyCharacter("POST /api/presets/apply", applyPresetRec.Body.Bytes())
}

func TestExperimentReportRoutes(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	getReq := httptest.NewRequest(http.MethodGet, "/api/experiment-reports", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/experiment-reports = %d", getRec.Code)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/api/experiment-reports", strings.NewReader(`{
	  "name":"lab-001",
	  "note":"pressure divergence",
	  "batch_count":36,
	  "source_instance_id":"default",
	  "compare_instance_id":"alt-a",
	  "current_checkpoint":"lab-001-current",
	  "compare_checkpoint":"lab-001-compare",
	  "outcome_summary":["default vs alt-a","tension gap: 0.80 vs 0.10"],
	  "conclusion":["长期张力主导：default（gap 0.70）"],
	  "current":{
	    "instance_id":"default",
	    "focus_character":"Anya",
	    "participants":["Anya","玩家"],
	    "participant_details":[{"name":"Anya","kind":"persona","source":"character_definition","loaded":true,"switchable":true,"present":true,"focus":true}],
	    "scene_location":"外城",
	    "scene_description":"长窗口实验",
	    "tick_count":36,
	    "tension":0.8,
	    "trajectory_summary":["trend a"],
	    "director_plan":{"mode":"auto_chain","selected":["Anya"],"world_signals":["pressure:curfew"]},
	    "latest_trace":{"turn":12,"focus_character":"Anya","user_input":"继续观察","director_plan":{"mode":"auto_chain"}}
	  },
	  "compare":{"instance_id":"alt-a","tick_count":36,"tension":0.1,"trajectory_summary":["trend b"]}
	}`))
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	mux.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST /api/experiment-reports = %d", postRec.Code)
	}

	var saved core.ExperimentReport
	if err := json.Unmarshal(postRec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if saved.Name != "lab-001" {
		t.Fatalf("saved.Name = %q, want lab-001", saved.Name)
	}
	if saved.Current.InstanceID != "default" || saved.Compare == nil || saved.Compare.InstanceID != "alt-a" {
		t.Fatalf("saved report snapshots = %#v", saved)
	}
	if saved.CurrentCheckpoint != "lab-001-current" || saved.CompareCheckpoint != "lab-001-compare" {
		t.Fatalf("saved checkpoints = %q / %q, want API to preserve checkpoint anchors", saved.CurrentCheckpoint, saved.CompareCheckpoint)
	}
	if saved.Current.DirectorPlan == nil || saved.Current.LatestTrace == nil {
		t.Fatalf("saved report authoring evidence = %#v, want director plan and latest trace", saved.Current)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/experiment-reports", nil)
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GET /api/experiment-reports after save = %d", listRec.Code)
	}

	var listed struct {
		Reports []core.ExperimentReport `json:"reports"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode report list: %v", err)
	}
	if len(listed.Reports) != 1 {
		t.Fatalf("listed reports = %d, want 1", len(listed.Reports))
	}
	if listed.Reports[0].CurrentCheckpoint != "lab-001-current" || listed.Reports[0].CompareCheckpoint != "lab-001-compare" {
		t.Fatalf("listed checkpoints = %q / %q, want GET to preserve checkpoint anchors", listed.Reports[0].CurrentCheckpoint, listed.Reports[0].CompareCheckpoint)
	}
}

func TestExperimentReportRoutesNormalizeLegacyTraceFocus(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	postReq := httptest.NewRequest(http.MethodPost, "/api/experiment-reports", strings.NewReader(`{
	  "name":"compat-report",
	  "source_instance_id":"default",
	  "current":{
	    "instance_id":"default",
	    "latest_trace":{"turn":12,"character":"LegacyFocus","user_input":"继续观察"}
	  },
	  "compare":{
	    "instance_id":"alt-a",
	    "latest_trace":{"turn":8,"focus_character":"CompareFocus","character":"LegacyCompare"}
	  }
	}`))
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	mux.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST /api/experiment-reports compat = %d", postRec.Code)
	}

	var saved core.ExperimentReport
	if err := json.Unmarshal(postRec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode compat report: %v", err)
	}
	if saved.Current.FocusCharacter != "LegacyFocus" {
		t.Fatalf("saved.Current.FocusCharacter = %q, want LegacyFocus", saved.Current.FocusCharacter)
	}
	if saved.Current.LatestTrace == nil || saved.Current.LatestTrace.FocusCharacter != "LegacyFocus" {
		t.Fatalf("saved.Current.LatestTrace = %#v, want normalized focus", saved.Current.LatestTrace)
	}
	if saved.Compare == nil || saved.Compare.LatestTrace == nil || saved.Compare.LatestTrace.FocusCharacter != "CompareFocus" {
		t.Fatalf("saved.Compare.LatestTrace = %#v, want CompareFocus", saved.Compare)
	}
}

func TestExperimentReportReplayCreatesReplayBranches(t *testing.T) {
	resolver := &mockResolver{
		defaultID: "default",
		engines: map[string]RuntimeEngine{
			"default": &mockEngine{
				instanceID: "default",
				name:       "Anya",
				experimentReports: []core.ExperimentReport{{
					Name:              "lab-001",
					SourceInstanceID:  "default",
					CompareInstanceID: "alt-a",
					CurrentCheckpoint: "lab-001-current",
					CompareCheckpoint: "lab-001-compare",
					Current: core.ExperimentSnapshot{
						InstanceID:     "default",
						FocusCharacter: "Anya",
					},
					Compare: &core.ExperimentSnapshot{
						InstanceID:     "alt-a",
						FocusCharacter: "V",
					},
				}},
			},
			"alt-a": &mockEngine{instanceID: "alt-a", name: "V"},
		},
	}
	s := NewServer(resolver.engines["default"], resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/experiment-reports/replay", strings.NewReader(`{"name":"lab-001"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/experiment-reports/replay = %d body=%s", rec.Code, rec.Body.String())
	}
	body := append([]byte(nil), rec.Body.Bytes()...)

	var payload struct {
		ReportName      string `json:"report_name"`
		CurrentInstance *struct {
			ID             string `json:"id"`
			FocusCharacter string `json:"focus_character"`
		} `json:"current_instance"`
		CompareInstance *struct {
			ID             string `json:"id"`
			FocusCharacter string `json:"focus_character"`
		} `json:"compare_instance"`
		CurrentEvidence *struct {
			SimStatus    map[string]interface{} `json:"sim_status"`
			AuditSummary []string               `json:"audit_summary"`
		} `json:"current_evidence"`
		CompareEvidence *struct {
			SimStatus    map[string]interface{} `json:"sim_status"`
			AuditSummary []string               `json:"audit_summary"`
		} `json:"compare_evidence"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode replay result: %v", err)
	}
	if payload.ReportName != "lab-001" {
		t.Fatalf("payload.ReportName = %q, want lab-001", payload.ReportName)
	}
	if payload.CurrentInstance == nil || payload.CompareInstance == nil {
		t.Fatalf("payload instances = %#v, want current+compare replay branches", payload)
	}
	if !strings.Contains(payload.CurrentInstance.ID, "lab-001-current-") {
		t.Fatalf("current replay id = %q, want lab-001-current-*", payload.CurrentInstance.ID)
	}
	if !strings.Contains(payload.CompareInstance.ID, "lab-001-compare-") {
		t.Fatalf("compare replay id = %q, want lab-001-compare-*", payload.CompareInstance.ID)
	}
	if payload.CurrentInstance.FocusCharacter != "Anya" || payload.CompareInstance.FocusCharacter != "V" {
		t.Fatalf("replay focus characters = %#v / %#v, want Anya / V", payload.CurrentInstance, payload.CompareInstance)
	}
	if payload.CurrentEvidence == nil || len(payload.CurrentEvidence.AuditSummary) == 0 {
		t.Fatalf("current replay evidence = %#v, want embedded audit summary", payload.CurrentEvidence)
	}
	if payload.CompareEvidence == nil || len(payload.CompareEvidence.AuditSummary) == 0 {
		t.Fatalf("compare replay evidence = %#v, want embedded audit summary", payload.CompareEvidence)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode raw replay result: %v", err)
	}
	currentRaw, _ := raw["current_instance"].(map[string]interface{})
	compareRaw, _ := raw["compare_instance"].(map[string]interface{})
	if _, ok := currentRaw["active_character"]; ok {
		t.Fatalf("replay current_instance unexpectedly exposes active_character: %#v", currentRaw)
	}
	if _, ok := currentRaw["loaded_characters"]; ok {
		t.Fatalf("replay current_instance unexpectedly exposes loaded_characters: %#v", currentRaw)
	}
	if _, ok := compareRaw["active_character"]; ok {
		t.Fatalf("replay compare_instance unexpectedly exposes active_character: %#v", compareRaw)
	}
	if _, ok := compareRaw["loaded_characters"]; ok {
		t.Fatalf("replay compare_instance unexpectedly exposes loaded_characters: %#v", compareRaw)
	}

	currentEngine, ok := resolver.engines[payload.CurrentInstance.ID].(*mockEngine)
	if !ok {
		t.Fatalf("current replay engine missing for %q", payload.CurrentInstance.ID)
	}
	compareEngine, ok := resolver.engines[payload.CompareInstance.ID].(*mockEngine)
	if !ok {
		t.Fatalf("compare replay engine missing for %q", payload.CompareInstance.ID)
	}
	if len(currentEngine.loadedCheckpoints) != 1 || currentEngine.loadedCheckpoints[0] != "lab-001-current" {
		t.Fatalf("current loaded checkpoints = %#v, want [lab-001-current]", currentEngine.loadedCheckpoints)
	}
	if len(compareEngine.loadedCheckpoints) != 1 || compareEngine.loadedCheckpoints[0] != "lab-001-compare" {
		t.Fatalf("compare loaded checkpoints = %#v, want [lab-001-compare]", compareEngine.loadedCheckpoints)
	}
}

func TestExperimentReportReplayBatchFiltersByWorld(t *testing.T) {
	resolver := &mockResolver{
		defaultID: "default",
		engines: map[string]RuntimeEngine{
			"default": &mockEngine{
				instanceID: "default",
				name:       "Anya",
				experimentReports: []core.ExperimentReport{
					{
						Name:              "neon-a",
						SourceInstanceID:  "default",
						CompareInstanceID: "alt-a",
						CurrentCheckpoint: "neon-a-current",
						CompareCheckpoint: "neon-a-compare",
						Current: core.ExperimentSnapshot{
							InstanceID:     "default",
							WorldName:      "neon_block",
							FocusCharacter: "Anya",
						},
						Compare: &core.ExperimentSnapshot{
							InstanceID:     "alt-a",
							WorldName:      "neon_block",
							FocusCharacter: "V",
						},
					},
					{
						Name:              "garden-a",
						SourceInstanceID:  "default",
						CompareInstanceID: "alt-b",
						CurrentCheckpoint: "garden-a-current",
						CompareCheckpoint: "garden-a-compare",
						Current: core.ExperimentSnapshot{
							InstanceID:     "default",
							WorldName:      "garden",
							FocusCharacter: "Anya",
						},
						Compare: &core.ExperimentSnapshot{
							InstanceID:     "alt-b",
							WorldName:      "garden",
							FocusCharacter: "Mina",
						},
					},
					{
						Name:              "neon-no-checkpoint",
						SourceInstanceID:  "default",
						CompareInstanceID: "alt-a",
						Current: core.ExperimentSnapshot{
							InstanceID:     "default",
							WorldName:      "neon_block",
							FocusCharacter: "Anya",
						},
					},
				},
			},
			"alt-a": &mockEngine{instanceID: "alt-a", name: "V"},
			"alt-b": &mockEngine{instanceID: "alt-b", name: "Mina"},
		},
	}
	s := NewServer(resolver.engines["default"], resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/experiment-reports/replay-batch", strings.NewReader(`{"world_name":"neon_block"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/experiment-reports/replay-batch = %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Mode      string   `json:"mode"`
		WorldName string   `json:"world_name"`
		Total     int      `json:"total"`
		Successes []string `json:"successes"`
		Results   []struct {
			ReportName string `json:"report_name"`
			WorldName  string `json:"world_name"`
			Replay     *struct {
				ReportName string `json:"report_name"`
				WorldName  string `json:"world_name"`
			} `json:"replay"`
			Error string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode replay batch result: %v", err)
	}
	if payload.Mode != "replay" {
		t.Fatalf("payload.Mode = %q, want replay", payload.Mode)
	}
	if payload.WorldName != "neon_block" {
		t.Fatalf("payload.WorldName = %q, want neon_block", payload.WorldName)
	}
	if payload.Total != 1 {
		t.Fatalf("payload.Total = %d, want 1 eligible neon_block report", payload.Total)
	}
	if len(payload.Successes) != 1 || payload.Successes[0] != "neon-a" {
		t.Fatalf("payload.Successes = %#v, want [neon-a]", payload.Successes)
	}
	if len(payload.Results) != 1 {
		t.Fatalf("payload.Results = %#v, want 1 result", payload.Results)
	}
	if payload.Results[0].ReportName != "neon-a" || payload.Results[0].WorldName != "neon_block" {
		t.Fatalf("payload.Results[0] = %#v, want neon-a/neon_block", payload.Results[0])
	}
	if payload.Results[0].Replay == nil || payload.Results[0].Replay.ReportName != "neon-a" || payload.Results[0].Replay.WorldName != "neon_block" {
		t.Fatalf("payload.Results[0].Replay = %#v, want replay metadata", payload.Results[0].Replay)
	}

	matched := 0
	for id, engine := range resolver.engines {
		if !strings.Contains(id, "neon-a-") {
			continue
		}
		if replayEngine, ok := engine.(*mockEngine); ok && len(replayEngine.loadedCheckpoints) == 1 {
			matched++
		}
	}
	if matched != 2 {
		t.Fatalf("matched replay engines = %d, want 2 current/compare branches for neon-a", matched)
	}
}

func TestExperimentReportReplayBatchRealRuntimeRoundTrip(t *testing.T) {
	baseDir := t.TempDir()
	worldDir := filepath.Join(baseDir, "world")
	writeAPITestWorldBundle(t, worldDir, "Replay Real Runtime", "真实 runtime replay round-trip")
	dataDir := filepath.Join(baseDir, "data")

	current := newRealRuntimeEngineForAPITest(t, filepath.Join(baseDir, "runtime.db"), dataDir, worldDir, "current", "Replay Real Runtime", "真实 runtime replay round-trip")
	current.SetInstanceMetadata("current", time.Now().UTC())
	current.SeedScene(core.SceneState{
		Location:    "外城",
		TimeOfDay:   "深夜",
		Weather:     "阴",
		Characters:  []string{"111", "玩家"},
		Description: "replay current baseline",
	})
	compare := newRealRuntimeEngineForAPITest(t, filepath.Join(baseDir, "runtime.db"), dataDir, worldDir, "compare", "Replay Real Runtime", "真实 runtime replay round-trip")
	compare.SetInstanceMetadata("compare", time.Now().UTC())
	compare.SeedScene(core.SceneState{
		Location:    "外城",
		TimeOfDay:   "深夜",
		Weather:     "雨",
		Characters:  []string{"111", "玩家"},
		Description: "replay compare baseline",
	})

	currentCheckpoint, err := current.CreateCheckpoint("real-replay-current", "main", "current baseline")
	if err != nil {
		t.Fatalf("CreateCheckpoint current: %v", err)
	}
	compareCheckpoint, err := compare.CreateCheckpoint("real-replay-compare", "main", "compare baseline")
	if err != nil {
		t.Fatalf("CreateCheckpoint compare: %v", err)
	}
	_, err = current.CreateExperimentReport(core.ExperimentReport{
		Name:              "real-replay",
		SourceInstanceID:  "current",
		CompareInstanceID: "compare",
		CurrentCheckpoint: currentCheckpoint.Name,
		CompareCheckpoint: compareCheckpoint.Name,
		Current: core.ExperimentSnapshot{
			InstanceID:     "current",
			WorldName:      "Replay Real Runtime",
			FocusCharacter: "111",
		},
		Compare: &core.ExperimentSnapshot{
			InstanceID:     "compare",
			WorldName:      "Replay Real Runtime",
			FocusCharacter: "111",
		},
	})
	if err != nil {
		t.Fatalf("CreateExperimentReport: %v", err)
	}

	manager := runtime.NewManager()
	if err := manager.Register("current", "Current", current, true); err != nil {
		t.Fatalf("register current: %v", err)
	}
	if err := manager.Register("compare", "Compare", compare, false); err != nil {
		t.Fatalf("register compare: %v", err)
	}
	t.Cleanup(func() {
		for _, summary := range manager.List() {
			if engine, err := manager.Resolve(summary.ID); err == nil {
				engine.Stop()
			}
		}
	})

	s := NewServer(current, realRuntimeResolver{manager: manager})
	mux := http.NewServeMux()
	s.Register(mux)

	replayReq := httptest.NewRequest(http.MethodPost, "/api/experiment-reports/replay-batch", strings.NewReader(`{"world_name":"Replay Real Runtime"}`))
	replayReq.Header.Set("Content-Type", "application/json")
	replayRec := httptest.NewRecorder()
	mux.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("POST /api/experiment-reports/replay-batch = %d body=%s", replayRec.Code, replayRec.Body.String())
	}

	var replayPayload experimentReplayBatchPayload
	if err := json.Unmarshal(replayRec.Body.Bytes(), &replayPayload); err != nil {
		t.Fatalf("decode replay batch: %v", err)
	}
	if replayPayload.Total != 1 || len(replayPayload.Successes) != 1 || len(replayPayload.Results) != 1 {
		t.Fatalf("replay batch payload = %#v, want one successful real runtime replay", replayPayload)
	}
	replay := replayPayload.Results[0].Replay
	if replay == nil || replay.CurrentInstance == nil || replay.CompareInstance == nil {
		t.Fatalf("replay result = %#v, want current and compare replay branches", replayPayload.Results[0])
	}
	if replay.CurrentEvidence == nil || len(replay.CurrentEvidence.AuditSummary) == 0 {
		t.Fatalf("current replay evidence = %#v, want audit summary from real runtime", replay.CurrentEvidence)
	}
	if replay.CompareEvidence == nil || len(replay.CompareEvidence.AuditSummary) == 0 {
		t.Fatalf("compare replay evidence = %#v, want audit summary from real runtime", replay.CompareEvidence)
	}

	advanceBody := fmt.Sprintf(`{
		"world_name":"Replay Real Runtime",
		"count":3,
		"replays":[{
			"report_name":"real-replay",
			"world_name":"Replay Real Runtime",
			"current_instance_id":%q,
			"compare_instance_id":%q
		}]
	}`, replay.CurrentInstance.ID, replay.CompareInstance.ID)
	advanceReq := httptest.NewRequest(http.MethodPost, "/api/experiment-reports/replay-advance", strings.NewReader(advanceBody))
	advanceReq.Header.Set("Content-Type", "application/json")
	advanceRec := httptest.NewRecorder()
	mux.ServeHTTP(advanceRec, advanceReq)
	if advanceRec.Code != http.StatusOK {
		t.Fatalf("POST /api/experiment-reports/replay-advance = %d body=%s", advanceRec.Code, advanceRec.Body.String())
	}

	var advancePayload experimentReplayBatchPayload
	if err := json.Unmarshal(advanceRec.Body.Bytes(), &advancePayload); err != nil {
		t.Fatalf("decode replay advance: %v", err)
	}
	if advancePayload.Mode != "tick" || advancePayload.Count != 3 || len(advancePayload.Successes) != 1 {
		t.Fatalf("advance payload = %#v, want one successful 3 tick replay advance", advancePayload)
	}
	advanced := advancePayload.Results[0].Replay
	if advanced == nil || advanced.CurrentEvidence == nil || advanced.CompareEvidence == nil {
		t.Fatalf("advanced replay = %#v, want refreshed evidence for both branches", advancePayload.Results[0])
	}
	currentTickCount, _ := advanced.CurrentEvidence.SimStatus["tick_count"].(float64)
	compareTickCount, _ := advanced.CompareEvidence.SimStatus["tick_count"].(float64)
	if currentTickCount != 0 || compareTickCount != 0 {
		t.Fatalf("manual replay tick loop counts = %.0f / %.0f, want loop counters unchanged when using manual ticks", currentTickCount, compareTickCount)
	}
	currentSummary := fmt.Sprint(advanced.CurrentEvidence.SimStatus["last_tick_summary"])
	compareSummary := fmt.Sprint(advanced.CompareEvidence.SimStatus["last_tick_summary"])
	if !strings.Contains(currentSummary, "world clock") || !strings.Contains(compareSummary, "world clock") {
		t.Fatalf("replay summaries = %q / %q, want real ManualTick evidence after advance", currentSummary, compareSummary)
	}
}

func TestAuthorWorldLevelInterventionReplayControlsRuntimeWithoutCharacterConfig(t *testing.T) {
	baseDir := t.TempDir()
	currentWorldDir := filepath.Join(baseDir, "current-world")
	compareWorldDir := filepath.Join(baseDir, "compare-world")
	writeAPITestWorldBundle(t, currentWorldDir, "Author World Ops", "作者通过世界结构与人口运营 runtime")
	writeAPITestWorldBundle(t, compareWorldDir, "Author World Ops", "作者通过世界结构与人口运营 runtime")

	current := newRealRuntimeEngineForAPITest(t, filepath.Join(baseDir, "current.db"), filepath.Join(baseDir, "current-data"), currentWorldDir, "author-current", "Author World Ops", "作者通过世界结构与人口运营 runtime")
	current.SetInstanceMetadata("author-current", time.Now().UTC())
	compare := newRealRuntimeEngineForAPITest(t, filepath.Join(baseDir, "compare.db"), filepath.Join(baseDir, "compare-data"), compareWorldDir, "author-compare", "Author World Ops", "作者通过世界结构与人口运营 runtime")
	compare.SetInstanceMetadata("author-compare", time.Now().UTC())

	beforeCurrentCfg, err := current.GetFocusDefinitionConfig("111")
	if err != nil {
		t.Fatalf("GetFocusDefinitionConfig current before: %v", err)
	}
	beforeCompareCfg, err := compare.GetFocusDefinitionConfig("111")
	if err != nil {
		t.Fatalf("GetFocusDefinitionConfig compare before: %v", err)
	}

	manager := runtime.NewManager()
	if err := manager.Register("author-current", "Author Current", current, true); err != nil {
		t.Fatalf("register current: %v", err)
	}
	if err := manager.Register("author-compare", "Author Compare", compare, false); err != nil {
		t.Fatalf("register compare: %v", err)
	}
	t.Cleanup(func() {
		for _, summary := range manager.List() {
			if engine, err := manager.Resolve(summary.ID); err == nil {
				engine.Stop()
			}
		}
	})

	s := NewServer(current, realRuntimeResolver{manager: manager})
	mux := http.NewServeMux()
	s.Register(mux)

	postJSON := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	getJSON := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	populationBody := `{
		"background_npcs":[
			{"id":"watcher","name":"巡夜人","role":"guard","location":"外城","faction":"guard","traits":["警觉","克制"],"hooks":["宵禁","盘查"]},
			{"id":"runner","name":"线人","role":"informant","location":"外城","faction":"smugglers","traits":["灵活","谨慎"],"hooks":["走私","风声"]}
		],
		"policy":{"promote_threshold":4.2,"major_threshold":8,"interaction_weight":3,"mention_weight":1,"event_weight":2,"relationship_weight":3,"scene_weight":2}
	}`
	for _, instanceID := range []string{"author-current", "author-compare"} {
		rec := postJSON("/api/population?instance_id="+instanceID, populationBody)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /api/population?instance_id=%s = %d body=%s", instanceID, rec.Code, rec.Body.String())
		}
	}

	baselineStructure := `{
		"locations":[{"id":"outer_city","name":"外城","kind":"district","description":"无人控制的普通街区","controller":""}]
	}`
	rec := postJSON("/api/world-structure?instance_id=author-current", baselineStructure)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/world-structure current = %d body=%s", rec.Code, rec.Body.String())
	}
	intervenedStructure := `{
		"factions":[
			{"id":"guard","name":"巡城司","role":"law","description":"负责宵禁和盘查","relationships":["敌对 smugglers"]},
			{"id":"smugglers","name":"走私帮","role":"criminal","description":"夜里持续活动","relationships":["敌对 guard"]}
		],
		"locations":[{"id":"outer_city","name":"外城","kind":"district","description":"巡城司控制区","controller":"guard"}],
		"pressures":[{"id":"curfew","name":"宵禁升级","kind":"conflict","description":"外城盘查与走私冲突持续加剧","intensity":0.9,"target":"guard"}]
	}`
	rec = postJSON("/api/world-structure?instance_id=author-compare", intervenedStructure)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/world-structure compare = %d body=%s", rec.Code, rec.Body.String())
	}

	for _, instanceID := range []string{"author-current", "author-compare"} {
		rec := postJSON("/api/sim/tick?instance_id="+instanceID, `{"count":36}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /api/sim/tick?instance_id=%s = %d body=%s", instanceID, rec.Code, rec.Body.String())
		}
	}

	var currentStatus map[string]interface{}
	rec = getJSON("/api/sim/status?instance_id=author-current")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/sim/status current = %d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&currentStatus); err != nil {
		t.Fatalf("decode current status: %v", err)
	}
	var compareStatus map[string]interface{}
	rec = getJSON("/api/sim/status?instance_id=author-compare")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/sim/status compare = %d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&compareStatus); err != nil {
		t.Fatalf("decode compare status: %v", err)
	}
	if compareStatus["tension"].(float64) <= currentStatus["tension"].(float64) {
		t.Fatalf("current tension=%v compare tension=%v, want world-level intervention to raise runtime pressure", currentStatus["tension"], compareStatus["tension"])
	}
	if fmt.Sprint(currentStatus["trajectory_summary"]) == fmt.Sprint(compareStatus["trajectory_summary"]) {
		t.Fatalf("trajectory summaries did not diverge after world-level authoring: current=%#v compare=%#v", currentStatus["trajectory_summary"], compareStatus["trajectory_summary"])
	}

	var currentInsights core.PopulationInsights
	rec = getJSON("/api/population-insights?instance_id=author-current")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/population-insights current = %d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&currentInsights); err != nil {
		t.Fatalf("decode current population insights: %v", err)
	}
	var compareInsights core.PopulationInsights
	rec = getJSON("/api/population-insights?instance_id=author-compare")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/population-insights compare = %d body=%s", rec.Code, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&compareInsights); err != nil {
		t.Fatalf("decode compare population insights: %v", err)
	}
	if _, ok := findPromotedNPC(compareInsights.Promoted, "巡夜人"); !ok {
		t.Fatalf("compare promoted = %#v, want world-level intervention to promote 巡夜人", compareInsights.Promoted)
	}

	currentCheckpoint, err := current.CreateCheckpoint("author-world-current", "baseline", "world-level baseline after ticks")
	if err != nil {
		t.Fatalf("CreateCheckpoint current: %v", err)
	}
	compareCheckpoint, err := compare.CreateCheckpoint("author-world-compare", "intervention", "world-level intervention after ticks")
	if err != nil {
		t.Fatalf("CreateCheckpoint compare: %v", err)
	}
	_, err = current.CreateExperimentReport(core.ExperimentReport{
		Name:              "author-world-level-control",
		SourceInstanceID:  "author-current",
		CompareInstanceID: "author-compare",
		CurrentCheckpoint: currentCheckpoint.Name,
		CompareCheckpoint: compareCheckpoint.Name,
		Current: core.ExperimentSnapshot{
			InstanceID:        "author-current",
			WorldName:         "Author World Ops",
			FocusCharacter:    "111",
			Tension:           currentStatus["tension"].(float64),
			TrajectorySummary: stringifyInterfaceSlice(currentStatus["trajectory_summary"]),
		},
		Compare: &core.ExperimentSnapshot{
			InstanceID:        "author-compare",
			WorldName:         "Author World Ops",
			FocusCharacter:    "111",
			Tension:           compareStatus["tension"].(float64),
			TrajectorySummary: stringifyInterfaceSlice(compareStatus["trajectory_summary"]),
		},
	})
	if err != nil {
		t.Fatalf("CreateExperimentReport: %v", err)
	}

	replayRec := postJSON("/api/experiment-reports/replay-batch", `{"world_name":"Author World Ops"}`)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("POST /api/experiment-reports/replay-batch = %d body=%s", replayRec.Code, replayRec.Body.String())
	}
	var replayPayload experimentReplayBatchPayload
	if err := json.Unmarshal(replayRec.Body.Bytes(), &replayPayload); err != nil {
		t.Fatalf("decode replay batch: %v", err)
	}
	if replayPayload.Total != 1 || len(replayPayload.Successes) != 1 || len(replayPayload.Results) != 1 {
		t.Fatalf("replay payload = %#v, want one replayed authoring report", replayPayload)
	}
	replay := replayPayload.Results[0].Replay
	if replay == nil || replay.CurrentInstance == nil || replay.CompareInstance == nil {
		t.Fatalf("replay = %#v, want current/compare replay branches", replayPayload.Results[0])
	}

	advanceBody := fmt.Sprintf(`{
		"world_name":"Author World Ops",
		"count":6,
		"replays":[{
			"report_name":"author-world-level-control",
			"world_name":"Author World Ops",
			"current_instance_id":%q,
			"compare_instance_id":%q
		}]
	}`, replay.CurrentInstance.ID, replay.CompareInstance.ID)
	advanceRec := postJSON("/api/experiment-reports/replay-advance", advanceBody)
	if advanceRec.Code != http.StatusOK {
		t.Fatalf("POST /api/experiment-reports/replay-advance = %d body=%s", advanceRec.Code, advanceRec.Body.String())
	}
	var advancePayload experimentReplayBatchPayload
	if err := json.Unmarshal(advanceRec.Body.Bytes(), &advancePayload); err != nil {
		t.Fatalf("decode replay advance: %v", err)
	}
	if len(advancePayload.Results) != 1 || advancePayload.Results[0].Replay == nil {
		t.Fatalf("advance payload = %#v, want replay evidence", advancePayload)
	}
	advanced := advancePayload.Results[0].Replay
	if advanced.CurrentEvidence == nil || advanced.CompareEvidence == nil {
		t.Fatalf("advanced evidence = %#v, want both branches", advanced)
	}
	if advanced.CompareEvidence.Population == nil || !containsPromotedNPC(advanced.CompareEvidence.Population.Promoted, "巡夜人") {
		t.Fatalf("compare replay population = %#v, want replay evidence to preserve world-level promotion", advanced.CompareEvidence.Population)
	}
	if len(advanced.CurrentEvidence.AuditSummary) == 0 || len(advanced.CompareEvidence.AuditSummary) == 0 {
		t.Fatalf("advanced audit summaries = %#v / %#v, want author-facing replay diagnosis", advanced.CurrentEvidence.AuditSummary, advanced.CompareEvidence.AuditSummary)
	}

	afterCurrentCfg, err := current.GetFocusDefinitionConfig("111")
	if err != nil {
		t.Fatalf("GetFocusDefinitionConfig current after: %v", err)
	}
	afterCompareCfg, err := compare.GetFocusDefinitionConfig("111")
	if err != nil {
		t.Fatalf("GetFocusDefinitionConfig compare after: %v", err)
	}
	if beforeCurrentCfg.Card.Identity.Name != afterCurrentCfg.Card.Identity.Name || beforeCompareCfg.Card.Identity.Name != afterCompareCfg.Card.Identity.Name {
		t.Fatalf("focus definition changed current %q->%q compare %q->%q, want no role-card rescue in authoring workflow", beforeCurrentCfg.Card.Identity.Name, afterCurrentCfg.Card.Identity.Name, beforeCompareCfg.Card.Identity.Name, afterCompareCfg.Card.Identity.Name)
	}
}

func TestAuthorWorldLevelInterventionReplayMatrixAcrossWorldFamilies(t *testing.T) {
	type sample struct {
		worldName           string
		rules               string
		location            string
		sceneDescription    string
		promotedNPC         string
		populationBody      string
		baselineStructure   string
		intervenedStructure string
	}

	samples := []sample{
		{
			worldName:        "Author Matrix Outer City",
			rules:            "作者通过外城治安结构运营世界",
			location:         "外城",
			sceneDescription: "外城宵禁样本",
			promotedNPC:      "巡夜人",
			populationBody: `{
				"background_npcs":[
					{"id":"watcher","name":"巡夜人","role":"guard","location":"外城","faction":"guard","traits":["警觉","克制"],"hooks":["宵禁","盘查"]},
					{"id":"runner","name":"线人","role":"informant","location":"外城","faction":"smugglers","traits":["灵活","谨慎"],"hooks":["走私","风声"]}
				],
				"policy":{"promote_threshold":4.2,"major_threshold":8,"interaction_weight":3,"mention_weight":1,"event_weight":2,"relationship_weight":3,"scene_weight":2}
			}`,
			baselineStructure: `{
				"locations":[{"id":"outer_city","name":"外城","kind":"district","description":"无人控制的普通街区","controller":""}]
			}`,
			intervenedStructure: `{
				"factions":[
					{"id":"guard","name":"巡城司","role":"law","description":"负责宵禁和盘查","relationships":["敌对 smugglers"]},
					{"id":"smugglers","name":"走私帮","role":"criminal","description":"夜里持续活动","relationships":["敌对 guard"]}
				],
				"locations":[{"id":"outer_city","name":"外城","kind":"district","description":"巡城司控制区","controller":"guard"}],
				"pressures":[{"id":"curfew","name":"宵禁升级","kind":"conflict","description":"外城盘查与走私冲突持续加剧","intensity":0.9,"target":"guard"}]
			}`,
		},
		{
			worldName:        "Author Matrix Harbor",
			rules:            "作者通过港口物流结构运营世界",
			location:         "码头",
			sceneDescription: "码头风暴样本",
			promotedNPC:      "码头调度",
			populationBody: `{
				"background_npcs":[
					{"id":"dispatcher","name":"码头调度","role":"dispatcher","location":"码头","faction":"harbor_union","traits":["急切","务实"],"hooks":["货船误点","工人罢工"]},
					{"id":"broker","name":"货运掮客","role":"broker","location":"货仓","faction":"cargo_brokers","traits":["圆滑","贪利"],"hooks":["压低运价","转移责任"]}
				],
				"policy":{"promote_threshold":4.2,"major_threshold":8,"interaction_weight":3,"mention_weight":1,"event_weight":2,"relationship_weight":3,"scene_weight":2}
			}`,
			baselineStructure: `{
				"locations":[{"id":"dock","name":"码头","kind":"logistics","description":"普通卸货区","controller":""}]
			}`,
			intervenedStructure: `{
				"factions":[
					{"id":"harbor_union","name":"港口工会","role":"labor","description":"控制装卸节奏","relationships":["抗衡 cargo_brokers"]},
					{"id":"cargo_brokers","name":"货运掮客","role":"market","description":"争夺货运报价","relationships":["压迫 harbor_union"]}
				],
				"locations":[{"id":"dock","name":"码头","kind":"logistics","description":"港口工会控制的卸货区","controller":"harbor_union"}],
				"pressures":[{"id":"dock_strike","name":"码头罢工","kind":"labor","description":"货船误点和罢工让码头调度被推到前台","intensity":0.9,"target":"harbor_union"}]
			}`,
		},
	}

	baseDir := t.TempDir()
	manager := runtime.NewManager()
	t.Cleanup(func() {
		for _, summary := range manager.List() {
			if engine, err := manager.Resolve(summary.ID); err == nil {
				engine.Stop()
			}
		}
	})

	var server *Server
	var mux *http.ServeMux
	postJSON := func(path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	getJSON := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	type sampleRuntime struct {
		sample  sample
		current *runtime.Engine
		compare *runtime.Engine
	}
	runtimes := make([]sampleRuntime, 0, len(samples))
	for i, sample := range samples {
		prefix := fmt.Sprintf("matrix-%d", i)
		currentID := prefix + "-current"
		compareID := prefix + "-compare"
		currentWorldDir := filepath.Join(baseDir, prefix+"-current-world")
		compareWorldDir := filepath.Join(baseDir, prefix+"-compare-world")
		writeAPITestWorldBundle(t, currentWorldDir, sample.worldName, sample.rules)
		writeAPITestWorldBundle(t, compareWorldDir, sample.worldName, sample.rules)

		scene := core.SceneState{
			Location:    sample.location,
			TimeOfDay:   "深夜",
			Weather:     "阴",
			Characters:  []string{"111", "玩家"},
			Description: sample.sceneDescription,
		}
		current := newRealWorldRuntimeEngineForAPITest(t, filepath.Join(baseDir, prefix+"-current.db"), filepath.Join(baseDir, prefix+"-current-data"), currentWorldDir, currentID, sample.worldName, sample.rules, scene)
		current.SetInstanceMetadata(currentID, time.Now().UTC())
		compare := newRealWorldRuntimeEngineForAPITest(t, filepath.Join(baseDir, prefix+"-compare.db"), filepath.Join(baseDir, prefix+"-compare-data"), compareWorldDir, compareID, sample.worldName, sample.rules, scene)
		compare.SetInstanceMetadata(compareID, time.Now().UTC())
		if err := manager.Register(currentID, sample.worldName+" Current", current, i == 0); err != nil {
			t.Fatalf("register %s: %v", currentID, err)
		}
		if err := manager.Register(compareID, sample.worldName+" Compare", compare, false); err != nil {
			t.Fatalf("register %s: %v", compareID, err)
		}
		runtimes = append(runtimes, sampleRuntime{sample: sample, current: current, compare: compare})
		if i == 0 {
			server = NewServer(current, realRuntimeResolver{manager: manager})
			mux = http.NewServeMux()
			server.Register(mux)
		}
	}
	if server == nil || mux == nil {
		t.Fatal("server was not initialized")
	}

	replayEntries := make([]experimentReplayAdvanceEntry, 0, len(runtimes))
	for i, item := range runtimes {
		prefix := fmt.Sprintf("matrix-%d", i)
		currentID := prefix + "-current"
		compareID := prefix + "-compare"
		for _, instanceID := range []string{currentID, compareID} {
			rec := postJSON("/api/population?instance_id="+instanceID, item.sample.populationBody)
			if rec.Code != http.StatusOK {
				t.Fatalf("POST /api/population?instance_id=%s = %d body=%s", instanceID, rec.Code, rec.Body.String())
			}
		}
		rec := postJSON("/api/world-structure?instance_id="+currentID, item.sample.baselineStructure)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /api/world-structure baseline %s = %d body=%s", item.sample.worldName, rec.Code, rec.Body.String())
		}
		rec = postJSON("/api/world-structure?instance_id="+compareID, item.sample.intervenedStructure)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST /api/world-structure intervention %s = %d body=%s", item.sample.worldName, rec.Code, rec.Body.String())
		}
		for _, instanceID := range []string{currentID, compareID} {
			rec := postJSON("/api/sim/tick?instance_id="+instanceID, `{"count":30}`)
			if rec.Code != http.StatusOK {
				t.Fatalf("POST /api/sim/tick?instance_id=%s = %d body=%s", instanceID, rec.Code, rec.Body.String())
			}
		}

		var currentStatus map[string]interface{}
		rec = getJSON("/api/sim/status?instance_id=" + currentID)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/sim/status current %s = %d body=%s", item.sample.worldName, rec.Code, rec.Body.String())
		}
		if err := json.NewDecoder(rec.Body).Decode(&currentStatus); err != nil {
			t.Fatalf("decode current status %s: %v", item.sample.worldName, err)
		}
		var compareStatus map[string]interface{}
		rec = getJSON("/api/sim/status?instance_id=" + compareID)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/sim/status compare %s = %d body=%s", item.sample.worldName, rec.Code, rec.Body.String())
		}
		if err := json.NewDecoder(rec.Body).Decode(&compareStatus); err != nil {
			t.Fatalf("decode compare status %s: %v", item.sample.worldName, err)
		}
		if fmt.Sprint(currentStatus["trajectory_summary"]) == fmt.Sprint(compareStatus["trajectory_summary"]) {
			t.Fatalf("%s trajectories did not diverge: current=%#v compare=%#v", item.sample.worldName, currentStatus["trajectory_summary"], compareStatus["trajectory_summary"])
		}

		var currentInsights core.PopulationInsights
		rec = getJSON("/api/population-insights?instance_id=" + currentID)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/population-insights current %s = %d body=%s", item.sample.worldName, rec.Code, rec.Body.String())
		}
		if err := json.NewDecoder(rec.Body).Decode(&currentInsights); err != nil {
			t.Fatalf("decode current population %s: %v", item.sample.worldName, err)
		}
		var compareInsights core.PopulationInsights
		rec = getJSON("/api/population-insights?instance_id=" + compareID)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/population-insights compare %s = %d body=%s", item.sample.worldName, rec.Code, rec.Body.String())
		}
		if err := json.NewDecoder(rec.Body).Decode(&compareInsights); err != nil {
			t.Fatalf("decode compare population %s: %v", item.sample.worldName, err)
		}
		if !containsPromotedNPC(compareInsights.Promoted, item.sample.promotedNPC) {
			t.Fatalf("%s compare promoted = %#v, want %s promoted", item.sample.worldName, compareInsights.Promoted, item.sample.promotedNPC)
		}

		currentCheckpoint, err := item.current.CreateCheckpoint(prefix+"-current", "baseline", "matrix baseline")
		if err != nil {
			t.Fatalf("CreateCheckpoint current %s: %v", item.sample.worldName, err)
		}
		compareCheckpoint, err := item.compare.CreateCheckpoint(prefix+"-compare", "intervention", "matrix intervention")
		if err != nil {
			t.Fatalf("CreateCheckpoint compare %s: %v", item.sample.worldName, err)
		}
		reportName := prefix + "-author-world-level"
		_, err = item.current.CreateExperimentReport(core.ExperimentReport{
			Name:              reportName,
			SourceInstanceID:  currentID,
			CompareInstanceID: compareID,
			CurrentCheckpoint: currentCheckpoint.Name,
			CompareCheckpoint: compareCheckpoint.Name,
			Current: core.ExperimentSnapshot{
				InstanceID:        currentID,
				WorldName:         item.sample.worldName,
				FocusCharacter:    "111",
				Tension:           currentStatus["tension"].(float64),
				TrajectorySummary: stringifyInterfaceSlice(currentStatus["trajectory_summary"]),
			},
			Compare: &core.ExperimentSnapshot{
				InstanceID:        compareID,
				WorldName:         item.sample.worldName,
				FocusCharacter:    "111",
				Tension:           compareStatus["tension"].(float64),
				TrajectorySummary: stringifyInterfaceSlice(compareStatus["trajectory_summary"]),
			},
		})
		if err != nil {
			t.Fatalf("CreateExperimentReport %s: %v", item.sample.worldName, err)
		}

		replayRec := postJSON("/api/experiment-reports/replay-batch?instance_id="+currentID, fmt.Sprintf(`{"world_name":%q}`, item.sample.worldName))
		if replayRec.Code != http.StatusOK {
			t.Fatalf("POST /api/experiment-reports/replay-batch %s = %d body=%s", item.sample.worldName, replayRec.Code, replayRec.Body.String())
		}
		var replayPayload experimentReplayBatchPayload
		if err := json.Unmarshal(replayRec.Body.Bytes(), &replayPayload); err != nil {
			t.Fatalf("decode replay batch %s: %v", item.sample.worldName, err)
		}
		if replayPayload.Total != 1 || len(replayPayload.Successes) != 1 || len(replayPayload.Results) != 1 {
			t.Fatalf("%s replay payload = %#v, want one replayed report", item.sample.worldName, replayPayload)
		}
		replay := replayPayload.Results[0].Replay
		if replay == nil || replay.CurrentInstance == nil || replay.CompareInstance == nil {
			t.Fatalf("%s replay = %#v, want current/compare replay branches", item.sample.worldName, replayPayload.Results[0])
		}
		replayEntries = append(replayEntries, experimentReplayAdvanceEntry{
			ReportName:        reportName,
			WorldName:         item.sample.worldName,
			CurrentInstanceID: replay.CurrentInstance.ID,
			CompareInstanceID: replay.CompareInstance.ID,
		})
	}

	advanceBody, err := json.Marshal(map[string]interface{}{
		"count":   4,
		"replays": replayEntries,
	})
	if err != nil {
		t.Fatalf("marshal replay advance body: %v", err)
	}
	advanceRec := postJSON("/api/experiment-reports/replay-advance", string(advanceBody))
	if advanceRec.Code != http.StatusOK {
		t.Fatalf("POST /api/experiment-reports/replay-advance matrix = %d body=%s", advanceRec.Code, advanceRec.Body.String())
	}
	var advancePayload experimentReplayBatchPayload
	if err := json.Unmarshal(advanceRec.Body.Bytes(), &advancePayload); err != nil {
		t.Fatalf("decode replay advance matrix: %v", err)
	}
	if advancePayload.Total != len(samples) || len(advancePayload.Successes) != len(samples) || len(advancePayload.Results) != len(samples) {
		t.Fatalf("advance payload = %#v, want all matrix replays advanced", advancePayload)
	}
	for _, result := range advancePayload.Results {
		if result.Replay == nil || result.Replay.CurrentEvidence == nil || result.Replay.CompareEvidence == nil {
			t.Fatalf("advance result = %#v, want evidence for both branches", result)
		}
		if len(result.Replay.CurrentEvidence.AuditSummary) == 0 || len(result.Replay.CompareEvidence.AuditSummary) == 0 {
			t.Fatalf("advance evidence summaries = %#v, want audit summaries", result.Replay)
		}
	}
}

func TestExperimentReportReplayAdvanceTicksReplayBranches(t *testing.T) {
	resolver := &mockResolver{
		defaultID: "default",
		engines: map[string]RuntimeEngine{
			"default":                 &mockEngine{instanceID: "default", name: "Anya"},
			"neon-a-current-replay":   &mockEngine{instanceID: "neon-a-current-replay", name: "Anya"},
			"neon-a-compare-replay":   &mockEngine{instanceID: "neon-a-compare-replay", name: "V"},
			"garden-a-current-replay": &mockEngine{instanceID: "garden-a-current-replay", name: "Mina"},
		},
	}
	s := NewServer(resolver.engines["default"], resolver)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/experiment-reports/replay-advance", strings.NewReader(`{
		"world_name":"neon_block",
		"count":4,
		"replays":[
			{"report_name":"neon-a","world_name":"neon_block","current_instance_id":"neon-a-current-replay","compare_instance_id":"neon-a-compare-replay"},
			{"report_name":"garden-a","world_name":"garden","current_instance_id":"garden-a-current-replay"}
		]
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/experiment-reports/replay-advance = %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Mode      string   `json:"mode"`
		WorldName string   `json:"world_name"`
		Count     int      `json:"count"`
		Total     int      `json:"total"`
		Successes []string `json:"successes"`
		Results   []struct {
			ReportName string `json:"report_name"`
			WorldName  string `json:"world_name"`
			Replay     *struct {
				ReportName      string `json:"report_name"`
				WorldName       string `json:"world_name"`
				CurrentInstance *struct {
					ID string `json:"id"`
				} `json:"current_instance"`
				CompareInstance *struct {
					ID string `json:"id"`
				} `json:"compare_instance"`
				CurrentEvidence *struct {
					SimStatus    map[string]interface{} `json:"sim_status"`
					LatestTrace  *core.TurnTrace        `json:"latest_trace"`
					AuditSummary []string               `json:"audit_summary"`
				} `json:"current_evidence"`
				CompareEvidence *struct {
					SimStatus    map[string]interface{} `json:"sim_status"`
					LatestTrace  *core.TurnTrace        `json:"latest_trace"`
					AuditSummary []string               `json:"audit_summary"`
				} `json:"compare_evidence"`
			} `json:"replay"`
			Error string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode replay advance result: %v", err)
	}
	if payload.Mode != "tick" {
		t.Fatalf("payload.Mode = %q, want tick", payload.Mode)
	}
	if payload.WorldName != "neon_block" {
		t.Fatalf("payload.WorldName = %q, want neon_block", payload.WorldName)
	}
	if payload.Count != 4 {
		t.Fatalf("payload.Count = %d, want 4", payload.Count)
	}
	if payload.Total != 1 {
		t.Fatalf("payload.Total = %d, want 1 filtered replay entry", payload.Total)
	}
	if len(payload.Successes) != 1 || payload.Successes[0] != "neon-a" {
		t.Fatalf("payload.Successes = %#v, want [neon-a]", payload.Successes)
	}
	if len(payload.Results) != 1 || payload.Results[0].Replay == nil {
		t.Fatalf("payload.Results = %#v, want 1 replay result", payload.Results)
	}
	if payload.Results[0].Replay.CurrentInstance == nil || payload.Results[0].Replay.CurrentInstance.ID != "neon-a-current-replay" {
		t.Fatalf("current replay payload = %#v, want neon-a-current-replay", payload.Results[0].Replay)
	}
	if payload.Results[0].Replay.CompareInstance == nil || payload.Results[0].Replay.CompareInstance.ID != "neon-a-compare-replay" {
		t.Fatalf("compare replay payload = %#v, want neon-a-compare-replay", payload.Results[0].Replay)
	}
	if payload.Results[0].Replay.CurrentEvidence == nil || payload.Results[0].Replay.CurrentEvidence.SimStatus["tick_count"] == nil {
		t.Fatalf("current replay evidence = %#v, want sim status", payload.Results[0].Replay.CurrentEvidence)
	}
	if got := int(payload.Results[0].Replay.CurrentEvidence.SimStatus["tick_count"].(float64)); got != 4 {
		t.Fatalf("current evidence tick_count = %d, want 4", got)
	}
	if payload.Results[0].Replay.CompareEvidence == nil || payload.Results[0].Replay.CompareEvidence.SimStatus["tick_count"] == nil {
		t.Fatalf("compare replay evidence = %#v, want sim status", payload.Results[0].Replay.CompareEvidence)
	}
	if got := int(payload.Results[0].Replay.CompareEvidence.SimStatus["tick_count"].(float64)); got != 4 {
		t.Fatalf("compare evidence tick_count = %d, want 4", got)
	}
	if payload.Results[0].Replay.CurrentEvidence.LatestTrace == nil || payload.Results[0].Replay.CompareEvidence.LatestTrace == nil {
		t.Fatalf("replay evidence traces = %#v / %#v, want latest trace for both branches", payload.Results[0].Replay.CurrentEvidence, payload.Results[0].Replay.CompareEvidence)
	}
	if len(payload.Results[0].Replay.CurrentEvidence.AuditSummary) == 0 || len(payload.Results[0].Replay.CompareEvidence.AuditSummary) == 0 {
		t.Fatalf("replay evidence summaries = %#v / %#v, want audit summary for both branches", payload.Results[0].Replay.CurrentEvidence, payload.Results[0].Replay.CompareEvidence)
	}

	currentEngine := resolver.engines["neon-a-current-replay"].(*mockEngine)
	compareEngine := resolver.engines["neon-a-compare-replay"].(*mockEngine)
	gardenEngine := resolver.engines["garden-a-current-replay"].(*mockEngine)
	if currentEngine.tickCount != 4 {
		t.Fatalf("currentEngine.tickCount = %d, want 4", currentEngine.tickCount)
	}
	if compareEngine.tickCount != 4 {
		t.Fatalf("compareEngine.tickCount = %d, want 4", compareEngine.tickCount)
	}
	if gardenEngine.tickCount != 0 {
		t.Fatalf("gardenEngine.tickCount = %d, want 0 after world filter", gardenEngine.tickCount)
	}
}

func TestProofAuditsRouteListsLatestAuditArtifacts(t *testing.T) {
	root := t.TempDir()
	auditRoot := filepath.Join(root, "proof-audits")
	if err := os.MkdirAll(filepath.Join(auditRoot, "20260527T223320Z"), 0o755); err != nil {
		t.Fatalf("mkdir proof audit root: %v", err)
	}
	summary := `# World Proof Audit

- Created At (UTC): 20260527T223320Z

## Final

- Overall: PASS
- Output Directory: data/proof-audits/20260527T223320Z
`
	if err := os.WriteFile(filepath.Join(auditRoot, "20260527T223320Z", "SUMMARY.md"), []byte(summary), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(auditRoot, "20260527T223320Z", "runtime.log"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write runtime log: %v", err)
	}

	s := newTestServer()
	s.SetProofAuditRoot(auditRoot)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/proof-audits?limit=3", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/proof-audits = %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Root        string `json:"root"`
		Count       int    `json:"count"`
		ProofAudits []struct {
			Name           string `json:"name"`
			Overall        string `json:"overall"`
			SummaryPath    string `json:"summary_path"`
			SummaryPreview string `json:"summary_preview"`
			Files          []struct {
				Name string `json:"name"`
				Size int64  `json:"size"`
			} `json:"files"`
		} `json:"proof_audits"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode proof audits payload: %v", err)
	}
	if payload.Root != filepath.ToSlash(auditRoot) {
		t.Fatalf("payload.Root = %q, want %q", payload.Root, filepath.ToSlash(auditRoot))
	}
	if payload.Count != 1 || len(payload.ProofAudits) != 1 {
		t.Fatalf("payload.Count/proof_audits = %d/%d, want 1/1", payload.Count, len(payload.ProofAudits))
	}
	if payload.ProofAudits[0].Name != "20260527T223320Z" {
		t.Fatalf("payload.ProofAudits[0].Name = %q, want 20260527T223320Z", payload.ProofAudits[0].Name)
	}
	if payload.ProofAudits[0].Overall != "PASS" {
		t.Fatalf("payload.ProofAudits[0].Overall = %q, want PASS", payload.ProofAudits[0].Overall)
	}
	if !strings.Contains(payload.ProofAudits[0].SummaryPreview, "World Proof Audit") {
		t.Fatalf("payload.ProofAudits[0].SummaryPreview = %q, want heading preview", payload.ProofAudits[0].SummaryPreview)
	}
	if len(payload.ProofAudits[0].Files) != 2 {
		t.Fatalf("payload.ProofAudits[0].Files = %#v, want summary + runtime log", payload.ProofAudits[0].Files)
	}
}

func TestSceneAndFactsRoutes(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	sceneReq := httptest.NewRequest(http.MethodPost, "/api/scenes", strings.NewReader(`{"name":"default","scene":{"location":"潇湘馆","time_of_day":"夜","weather":"雨","characters":["黛玉"],"description":"竹影"}}`))
	sceneReq.Header.Set("Content-Type", "application/json")
	sceneRec := httptest.NewRecorder()
	mux.ServeHTTP(sceneRec, sceneReq)
	if sceneRec.Code != http.StatusOK {
		t.Fatalf("POST /api/scenes = %d", sceneRec.Code)
	}

	factsReq := httptest.NewRequest(http.MethodPost, "/api/canon-facts", strings.NewReader(`{"facts":[{"subject":"黛玉","predicate":"住处","object":"潇湘馆","confidence":1}]}`))
	factsReq.Header.Set("Content-Type", "application/json")
	factsRec := httptest.NewRecorder()
	mux.ServeHTTP(factsRec, factsReq)
	if factsRec.Code != http.StatusOK {
		t.Fatalf("POST /api/canon-facts = %d", factsRec.Code)
	}
}

func TestQuarantineAndPendingFactRoutes(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	qReq := httptest.NewRequest(http.MethodGet, "/api/quarantine", nil)
	qRec := httptest.NewRecorder()
	mux.ServeHTTP(qRec, qReq)
	if qRec.Code != http.StatusOK {
		t.Fatalf("GET /api/quarantine = %d", qRec.Code)
	}
	var quarantinePayload map[string]interface{}
	if err := json.NewDecoder(qRec.Body).Decode(&quarantinePayload); err != nil {
		t.Fatalf("decode /api/quarantine: %v", err)
	}
	if quarantinePayload["focus_character"] != "Anya" {
		t.Fatalf("quarantine focus_character = %#v, want Anya", quarantinePayload["focus_character"])
	}
	if _, ok := quarantinePayload["character"]; ok {
		t.Fatalf("quarantine top-level character compatibility mirror should be absent: %#v", quarantinePayload)
	}

	qPromoteReq := httptest.NewRequest(http.MethodPost, "/api/quarantine/promote", strings.NewReader(`{"id":"q1"}`))
	qPromoteReq.Header.Set("Content-Type", "application/json")
	qPromoteRec := httptest.NewRecorder()
	mux.ServeHTTP(qPromoteRec, qPromoteReq)
	if qPromoteRec.Code != http.StatusOK {
		t.Fatalf("POST /api/quarantine/promote = %d", qPromoteRec.Code)
	}

	pReq := httptest.NewRequest(http.MethodGet, "/api/pending-facts", nil)
	pRec := httptest.NewRecorder()
	mux.ServeHTTP(pRec, pReq)
	if pRec.Code != http.StatusOK {
		t.Fatalf("GET /api/pending-facts = %d", pRec.Code)
	}
	var pendingPayload map[string]interface{}
	if err := json.NewDecoder(pRec.Body).Decode(&pendingPayload); err != nil {
		t.Fatalf("decode /api/pending-facts: %v", err)
	}
	if pendingPayload["focus_character"] != "Anya" {
		t.Fatalf("pending focus_character = %#v, want Anya", pendingPayload["focus_character"])
	}
	if _, ok := pendingPayload["character"]; ok {
		t.Fatalf("pending top-level character compatibility mirror should be absent: %#v", pendingPayload)
	}
	facts, ok := pendingPayload["facts"].([]interface{})
	if !ok || len(facts) != 1 {
		t.Fatalf("pending facts = %#v, want 1 item", pendingPayload["facts"])
	}
	fact, ok := facts[0].(map[string]interface{})
	if !ok {
		t.Fatalf("pending fact[0] = %#v, want object", facts[0])
	}
	if fact["focus_character"] != "Anya" {
		t.Fatalf("pending fact focus_character = %#v, want Anya", fact["focus_character"])
	}
	if _, ok := fact["character"]; ok {
		t.Fatalf("pending fact character mirror should be absent on canonical path: %#v", fact)
	}

	pConfirmReq := httptest.NewRequest(http.MethodPost, "/api/pending-facts/confirm", strings.NewReader(`{"id":"p1"}`))
	pConfirmReq.Header.Set("Content-Type", "application/json")
	pConfirmRec := httptest.NewRecorder()
	mux.ServeHTTP(pConfirmRec, pConfirmReq)
	if pConfirmRec.Code != http.StatusOK {
		t.Fatalf("POST /api/pending-facts/confirm = %d", pConfirmRec.Code)
	}
}

func TestNPCActionsRouteUsesFocusCharacterWithoutTopLevelCharacterMirror(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/npc-actions", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/npc-actions = %d", rec.Code)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode /api/npc-actions: %v", err)
	}
	if payload["focus_character"] != "Anya" {
		t.Fatalf("npc-actions focus_character = %#v, want Anya", payload["focus_character"])
	}
	if _, ok := payload["character"]; ok {
		t.Fatalf("npc-actions top-level character compatibility mirror should be absent: %#v", payload)
	}
}

func TestBranchAndSaveDiffRoutes(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	saveReq := httptest.NewRequest(http.MethodGet, "/api/saves/diff?a=slot-a&b=slot-b", nil)
	saveRec := httptest.NewRecorder()
	mux.ServeHTTP(saveRec, saveReq)
	if saveRec.Code != http.StatusOK {
		t.Fatalf("GET /api/saves/diff = %d", saveRec.Code)
	}

	branchReq := httptest.NewRequest(http.MethodGet, "/api/branches/diff?a=main&b=alt", nil)
	branchRec := httptest.NewRecorder()
	mux.ServeHTTP(branchRec, branchReq)
	if branchRec.Code != http.StatusOK {
		t.Fatalf("GET /api/branches/diff = %d", branchRec.Code)
	}

	mergeReq := httptest.NewRequest(http.MethodPost, "/api/branches/merge", strings.NewReader(`{"source":"alt","target":"main","merge_flags":true}`))
	mergeReq.Header.Set("Content-Type", "application/json")
	mergeRec := httptest.NewRecorder()
	mux.ServeHTTP(mergeRec, mergeReq)
	if mergeRec.Code != http.StatusOK {
		t.Fatalf("POST /api/branches/merge = %d", mergeRec.Code)
	}
}

func TestDialogueResetRoute(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("POST", "/api/dialogue/reset", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST /api/dialogue/reset → %d, want 200", rec.Code)
	}
}

func TestCausalityMissingID(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("GET", "/api/causality", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("GET /api/causality (no id) → %d, want 400", rec.Code)
	}
}

func TestCausalityNarrativeModeResponse(t *testing.T) {
	engine := &mockEngine{
		instanceID: "default",
		name:       "Anya",
		state:      core.WorldState{},
		causalityNarrativeChain: map[string]interface{}{
			"event": map[string]interface{}{
				"id":   "d1",
				"type": "dialogue",
			},
			"causes": []interface{}{
				map[string]interface{}{
					"event": map[string]interface{}{
						"id":   "u1",
						"type": "user_message",
					},
				},
			},
			"effects": []interface{}{},
		},
		causalityNarrativeSum: "[dialogue] 111 \u2192 用户 | “收到，我来回应。”\n  ↑ 因为:\n  [user_message] user \u2192 111 | “111”\n",
		causalityChain:        []string{"non-narrative"},
		causalitySummary:      "plain summary",
	}
	s := NewServer(engine)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("GET", "/api/causality?id=d1&mode=narrative&depth=5", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/causality narrative → %d, want 200", rec.Code)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode causality payload: %v", err)
	}
	if payload["event_id"] != "d1" {
		t.Fatalf("event_id = %#v, want d1", payload["event_id"])
	}
	if payload["depth"].(float64) != 5 {
		t.Fatalf("depth = %#v, want 5", payload["depth"])
	}
	if payload["summary"] != engine.causalityNarrativeSum {
		t.Fatalf("summary = %#v, want narrative summary", payload["summary"])
	}
	chain, ok := payload["chain"].(map[string]interface{})
	if !ok {
		t.Fatalf("chain type = %T, want object", payload["chain"])
	}
	event, ok := chain["event"].(map[string]interface{})
	if !ok || event["id"] != "d1" {
		t.Fatalf("chain.event = %#v, want d1 root", chain["event"])
	}
	causes, ok := chain["causes"].([]interface{})
	if !ok || len(causes) != 1 {
		t.Fatalf("chain.causes = %#v, want single cause", chain["causes"])
	}
}

func TestCausalityDefaultModeUsesPlainSummary(t *testing.T) {
	engine := &mockEngine{
		instanceID: "default",
		name:       "Anya",
		state:      core.WorldState{},
		causalityChain: map[string]interface{}{
			"event": map[string]interface{}{"id": "evt-plain", "type": "dialogue"},
		},
		causalitySummary:      "plain summary",
		causalityNarrativeSum: "narrative summary",
	}
	s := NewServer(engine)
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("GET", "/api/causality?id=evt-plain", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/causality plain → %d, want 200", rec.Code)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode causality payload: %v", err)
	}
	if payload["summary"] != "plain summary" {
		t.Fatalf("summary = %#v, want plain summary", payload["summary"])
	}
	chain, ok := payload["chain"].(map[string]interface{})
	if !ok {
		t.Fatalf("chain type = %T, want object", payload["chain"])
	}
	event, ok := chain["event"].(map[string]interface{})
	if !ok || event["id"] != "evt-plain" {
		t.Fatalf("chain.event = %#v, want evt-plain", chain["event"])
	}
}

func TestReplayMissingParams(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("GET", "/api/replay", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("GET /api/replay (no id/time) → %d, want 400", rec.Code)
	}
}

func TestLLMConfigsMethodDispatch(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	// GET
	req := httptest.NewRequest("GET", "/api/llm-configs", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/llm-configs → %d, want 200", rec.Code)
	}

	// DELETE should fail — uses /api/llm-configs/ path prefix
	req2 := httptest.NewRequest("DELETE", "/api/llm-configs/test-config", nil)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code < 200 || rec2.Code >= 500 {
		t.Errorf("DELETE /api/llm-configs/test-config → %d", rec2.Code)
	}
}

func TestLLMActiveRejectsEmptyConfig(t *testing.T) {
	initTestLLMStore(t)
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest("POST", "/api/llm-active", strings.NewReader(`{"prompt_price":1.0,"completion_price":4.0}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/llm-active empty config → %d, want 400", rec.Code)
	}
}

func TestLLMActiveSwitchByName(t *testing.T) {
	initTestLLMStore(t)
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	createReq := httptest.NewRequest("POST", "/api/llm-configs", strings.NewReader(`{"name":"cfg-a","endpoint":"http://example.test/v1","api_key":"secret","model":"demo"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("POST /api/llm-configs → %d", createRec.Code)
	}

	switchReq := httptest.NewRequest("POST", "/api/llm-active", strings.NewReader(`{"name":"cfg-a"}`))
	switchReq.Header.Set("Content-Type", "application/json")
	switchRec := httptest.NewRecorder()
	mux.ServeHTTP(switchRec, switchReq)
	if switchRec.Code != http.StatusOK {
		t.Fatalf("POST /api/llm-active by name → %d", switchRec.Code)
	}
}

func TestLLMConfigSingleItemPostUpdatesPricingOnly(t *testing.T) {
	initTestLLMStore(t)
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	createReq := httptest.NewRequest("POST", "/api/llm-configs", strings.NewReader(`{"name":"cfg-p","endpoint":"http://example.test/v1","api_key":"secret-key","model":"demo","prompt_price":1.0,"completion_price":4.0}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("POST /api/llm-configs create → %d", createRec.Code)
	}

	switchReq := httptest.NewRequest("POST", "/api/llm-active", strings.NewReader(`{"name":"cfg-p"}`))
	switchReq.Header.Set("Content-Type", "application/json")
	switchRec := httptest.NewRecorder()
	mux.ServeHTTP(switchRec, switchReq)
	if switchRec.Code != http.StatusOK {
		t.Fatalf("POST /api/llm-active by name → %d", switchRec.Code)
	}

	updateReq := httptest.NewRequest("POST", "/api/llm-configs/cfg-p", strings.NewReader(`{"prompt_price":2.5,"completion_price":9.5}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	mux.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("POST /api/llm-configs/cfg-p update → %d", updateRec.Code)
	}

	getReq := httptest.NewRequest("GET", "/api/llm-configs/cfg-p", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/llm-configs/cfg-p → %d", getRec.Code)
	}
	var cfg struct {
		Endpoint        string  `json:"endpoint"`
		Model           string  `json:"model"`
		PromptPrice     float64 `json:"prompt_price"`
		CompletionPrice float64 `json:"completion_price"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.Endpoint != "http://example.test/v1" || cfg.Model != "demo" {
		t.Fatalf("config mutated unexpectedly: %+v", cfg)
	}
	if cfg.PromptPrice != 2.5 || cfg.CompletionPrice != 9.5 {
		t.Fatalf("pricing = (%v,%v), want (2.5,9.5)", cfg.PromptPrice, cfg.CompletionPrice)
	}

	activeReq := httptest.NewRequest("GET", "/api/llm-active", nil)
	activeRec := httptest.NewRecorder()
	mux.ServeHTTP(activeRec, activeReq)
	if activeRec.Code != http.StatusOK {
		t.Fatalf("GET /api/llm-active → %d", activeRec.Code)
	}
	var active struct {
		PromptPrice     float64 `json:"prompt_price"`
		CompletionPrice float64 `json:"completion_price"`
	}
	if err := json.NewDecoder(activeRec.Body).Decode(&active); err != nil {
		t.Fatalf("decode active: %v", err)
	}
	if active.PromptPrice != 2.5 || active.CompletionPrice != 9.5 {
		t.Fatalf("active pricing = (%v,%v), want (2.5,9.5)", active.PromptPrice, active.CompletionPrice)
	}
}

func TestDirectorRoute(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	body := `{"action":"set_tension","value":0.8}`
	req := httptest.NewRequest("POST", "/api/director", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST /api/director → %d, want 200", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if v, ok := resp["action"].(string); !ok || v != "set_tension" {
		t.Errorf("action = %v, want set_tension", resp["action"])
	}
}
