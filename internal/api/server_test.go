package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"corerp/internal/agents"
	"corerp/internal/auth"
	"corerp/internal/core"
	"corerp/internal/events"
	"corerp/internal/llm"
	"corerp/internal/narrative"
)

type mockResolver struct {
	defaultID string
	engines   map[string]RuntimeEngine
	stopped   map[string]bool
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
func (r *mockResolver) CreateInstance(sourceID, id, label, activeCharacter string) (core.RuntimeInstanceSummary, error) {
	if id == "" {
		return core.RuntimeInstanceSummary{}, fmt.Errorf("instance id required")
	}
	name := activeCharacter
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
	dialogue                []core.Message
	player                  core.PlayerRole
	world                   core.WorldConfig
	scenes                  core.SceneConfigList
	facts                   core.CanonFactsConfig
	director                core.DirectorConfig
	plan                    core.DirectorPlan
	trace                   core.TurnTrace
	traces                  []core.TurnTrace
	quarantine              []core.Event
	pending                 []core.PendingFact
	causalityChain          interface{}
	causalityNarrativeChain interface{}
	causalitySummary        string
	causalityNarrativeSum   string
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
	return core.RuntimeInstanceSummary{
		ID:               m.GetInstanceID(),
		Label:            "test",
		WorldName:        "test-world",
		ActiveCharacter:  m.name,
		LoadedCharacters: []string{m.name},
		Status:           "running",
	}
}
func (m *mockEngine) GetState() core.WorldState { return m.state }
func (m *mockEngine) GetCharacter() (core.Character, bool) {
	return core.Character{Identity: core.IdentityEnvelope{Name: m.name}}, true
}
func (m *mockEngine) GetPlayerRole() core.PlayerRole { return m.player }
func (m *mockEngine) UpdatePlayerRole(role core.PlayerRole) (core.PlayerRole, error) {
	m.player = role
	if m.player.Name == "" {
		m.player.Name = "玩家"
	}
	return m.player, nil
}
func (m *mockEngine) GetCharacterConfig(name string) (core.CharacterConfig, error) {
	return core.CharacterConfig{
		Character: m.name,
		Path:      "characters/test.yml",
		WorldPath: "worlds/test.yml",
		Card:      core.Character{Identity: core.IdentityEnvelope{Name: m.name}},
	}, nil
}
func (m *mockEngine) UpdateCharacterConfig(name string, card core.Character) (core.CharacterConfig, error) {
	m.name = card.Identity.Name
	return core.CharacterConfig{
		Character: m.name,
		Path:      "characters/test.yml",
		WorldPath: "worlds/test.yml",
		Card:      card,
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
		m.trace = core.TurnTrace{Turn: 1, Character: m.name, UserInput: "test", Narrative: "mock narrative"}
	}
	return m.trace, true
}
func (m *mockEngine) ListTurnTraces(limit int) []core.TurnTrace {
	if len(m.traces) == 0 {
		m.traces = []core.TurnTrace{
			{Turn: 2, Character: m.name, UserInput: "later"},
			{Turn: 1, Character: m.name, UserInput: "earlier"},
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
		m.pending = []core.PendingFact{{ID: "p1", Character: "Anya", Subject: "V", Predicate: "身份", Object: "佣兵", Source: "llm_extracted", Confidence: 0.4}}
	}
	return m.pending, map[string]interface{}{"pending_total": len(m.pending)}, nil
}
func (m *mockEngine) ConfirmPendingFact(eventID string) error { return nil }
func (m *mockEngine) DeletePendingFact(eventID string) error  { return nil }
func (m *mockEngine) PromotePendingFact(eventID string) error { return nil }
func (m *mockEngine) GetCharacterName() string                { return m.name }
func (m *mockEngine) GetLoadedCharacters() []string           { return []string{m.name} }
func (m *mockEngine) SwitchCharacter(name string) error       { m.name = name; return nil }
func (m *mockEngine) GetWorldName() string                    { return "test-world" }
func (m *mockEngine) GetWorldPaths() map[string]string {
	cfg, _ := m.GetWorldConfig()
	return map[string]string{m.name: cfg.Path}
}
func (m *mockEngine) GetMemorySnapshot(character string, factLimit, episodicLimit, dialogueLimit int) (core.MemorySnapshot, error) {
	return core.MemorySnapshot{
		Character:     m.name,
		WorkingMemory: "working",
		Dialogue:      m.dialogue,
	}, nil
}
func (m *mockEngine) ListSaveSlots() ([]core.SaveSlot, error) {
	return []core.SaveSlot{{Name: "slot-1", Character: m.name, Branch: "main"}}, nil
}
func (m *mockEngine) CreateSaveSlot(name, branch, note string) (core.SaveSlot, error) {
	return core.SaveSlot{Name: name, Character: m.name, Branch: branch, Note: note}, nil
}
func (m *mockEngine) LoadSaveSlot(name string) (core.SaveSlot, error) {
	return core.SaveSlot{Name: name, Character: m.name, Branch: "main"}, nil
}
func (m *mockEngine) CompareSaveSlots(saveA, saveB string) (core.WorldStateDiff, error) {
	return core.WorldStateDiff{SaveA: saveA, SaveB: saveB, Tension: &core.StateDiffEntry{A: 0.1, B: 0.2}}, nil
}
func (m *mockEngine) ListCheckpoints() ([]core.SaveSlot, error) {
	return []core.SaveSlot{{Name: "cp-1", Character: m.name, Branch: "main"}}, nil
}
func (m *mockEngine) CreateCheckpoint(name, branch, note string) (core.SaveSlot, error) {
	return core.SaveSlot{Name: name, Character: m.name, Branch: branch, Note: note}, nil
}
func (m *mockEngine) LoadCheckpoint(name string) (core.SaveSlot, error) {
	return core.SaveSlot{Name: name, Character: m.name, Branch: "main"}, nil
}
func (m *mockEngine) ListScenarioPresets() ([]core.ScenarioPreset, error) {
	return []core.ScenarioPreset{{Name: "preset-1", Character: m.name, Branch: "main"}}, nil
}
func (m *mockEngine) CreateScenarioPreset(name, branch, note string) (core.ScenarioPreset, error) {
	return core.ScenarioPreset{Name: name, Character: m.name, Branch: branch, Note: note}, nil
}
func (m *mockEngine) ApplyScenarioPreset(name string) (core.ScenarioPreset, error) {
	return core.ScenarioPreset{Name: name, Character: m.name, Branch: "main"}, nil
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

func newTestServer() *Server {
	return NewServer(&mockEngine{instanceID: "default", name: "Anya", state: core.WorldState{
		Scene:         core.SceneState{Location: "test", TimeOfDay: "day", Weather: "clear"},
		Relationships: make(map[string]core.Relationship),
		Variables:     make(map[string]interface{}),
		Flags:         make(map[string]bool),
	}, player: core.PlayerRole{Name: "玩家"}})
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
		{"/api/character", "POST", http.StatusMethodNotAllowed},
		{"/api/player-role", "DELETE", http.StatusMethodNotAllowed},
		{"/api/instances", "POST", http.StatusMethodNotAllowed},
		{"/api/instances/status", "POST", http.StatusMethodNotAllowed},
		{"/api/character-config", "DELETE", http.StatusMethodNotAllowed},
		{"/api/characters", "POST", http.StatusMethodNotAllowed},
		{"/api/world", "POST", http.StatusMethodNotAllowed},
		{"/api/worlds", "POST", http.StatusMethodNotAllowed},
		{"/api/world-config", "DELETE", http.StatusMethodNotAllowed},
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
		{"/api/memory", "POST", http.StatusMethodNotAllowed},
		{"/api/export", "POST", http.StatusMethodNotAllowed},
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
		{"/api/character", "GET"},
		{"/api/player-role", "GET"},
		{"/api/instances", "GET"},
		{"/api/instances/status", "GET"},
		{"/api/character-config", "GET"},
		{"/api/characters", "GET"},
		{"/api/world", "GET"},
		{"/api/worlds", "GET"},
		{"/api/world-config", "GET"},
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

	var payload struct {
		InstanceID string                      `json:"instance_id"`
		Instance   core.RuntimeInstanceSummary `json:"instance"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if payload.InstanceID != "default" {
		t.Fatalf("instance_id = %q, want default", payload.InstanceID)
	}
	if payload.Instance.ID != "default" {
		t.Fatalf("instance.id = %q, want default", payload.Instance.ID)
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
		Default   string                        `json:"default"`
		Instances []core.RuntimeInstanceSummary `json:"instances"`
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

	req := httptest.NewRequest("POST", "/api/instances/create", strings.NewReader(`{"id":"alt","label":"Alt","active_character":"V"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/instances/create → %d, want 200", rec.Code)
	}
	if _, ok := resolver.engines["alt"]; !ok {
		t.Fatal("resolver missing created instance")
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
	var payload core.RuntimeInstanceSummary
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if payload.ID != "alt" || payload.Status != "stopped" {
		t.Fatalf("payload = %#v, want alt/stopped", payload)
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

	body := `{"character":"V"}`
	req := httptest.NewRequest("POST", "/api/switch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST /api/switch → %d, want 200", rec.Code)
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

	body := `{"character":"Anya","card":{"identity":{"name":"Anya","immutable":["cold"],"adaptive":{"trust":3},"forbidden":["info_dump"],"voice":{"style":"brief","rhythm":"short"},"writing_guide":"stay sharp"},"goals":[{"id":"survive","priority":10,"type":"primary","condition":"always"}]}}`
	req := httptest.NewRequest("POST", "/api/character-config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("POST /api/character-config → %d, want 200", rec.Code)
	}
}

func TestCharacterConfigRouteRejectsInvalidGoalCondition(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/character-config", strings.NewReader(`{"character":"Anya","card":{"identity":{"name":"Anya"},"goals":[{"id":"secret","priority":8,"type":"hidden","condition":"trust >","reveal_condition":"scene == safehouse"}]}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/character-config invalid condition = %d, want %d", rec.Code, http.StatusBadRequest)
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

	postReq := httptest.NewRequest(http.MethodPost, "/api/director-config", strings.NewReader(`{"mode":"auto_single","max_speakers":1}`))
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
}

func TestCheckpointAndPresetRoutes(t *testing.T) {
	s := newTestServer()
	mux := http.NewServeMux()
	s.Register(mux)

	getCheckpoints := httptest.NewRequest(http.MethodGet, "/api/checkpoints", nil)
	getCheckpointsRec := httptest.NewRecorder()
	mux.ServeHTTP(getCheckpointsRec, getCheckpoints)
	if getCheckpointsRec.Code != http.StatusOK {
		t.Fatalf("GET /api/checkpoints = %d", getCheckpointsRec.Code)
	}

	postCheckpoint := httptest.NewRequest(http.MethodPost, "/api/checkpoints", strings.NewReader(`{"name":"cp-a","branch":"main","note":"before risk"}`))
	postCheckpoint.Header.Set("Content-Type", "application/json")
	postCheckpointRec := httptest.NewRecorder()
	mux.ServeHTTP(postCheckpointRec, postCheckpoint)
	if postCheckpointRec.Code != http.StatusOK {
		t.Fatalf("POST /api/checkpoints = %d", postCheckpointRec.Code)
	}

	loadCheckpoint := httptest.NewRequest(http.MethodPost, "/api/checkpoints/load", strings.NewReader(`{"name":"cp-a"}`))
	loadCheckpoint.Header.Set("Content-Type", "application/json")
	loadCheckpointRec := httptest.NewRecorder()
	mux.ServeHTTP(loadCheckpointRec, loadCheckpoint)
	if loadCheckpointRec.Code != http.StatusOK {
		t.Fatalf("POST /api/checkpoints/load = %d", loadCheckpointRec.Code)
	}

	getPresets := httptest.NewRequest(http.MethodGet, "/api/presets", nil)
	getPresetsRec := httptest.NewRecorder()
	mux.ServeHTTP(getPresetsRec, getPresets)
	if getPresetsRec.Code != http.StatusOK {
		t.Fatalf("GET /api/presets = %d", getPresetsRec.Code)
	}

	postPreset := httptest.NewRequest(http.MethodPost, "/api/presets", strings.NewReader(`{"name":"opening","branch":"main","note":"intro"}`))
	postPreset.Header.Set("Content-Type", "application/json")
	postPresetRec := httptest.NewRecorder()
	mux.ServeHTTP(postPresetRec, postPreset)
	if postPresetRec.Code != http.StatusOK {
		t.Fatalf("POST /api/presets = %d", postPresetRec.Code)
	}

	applyPreset := httptest.NewRequest(http.MethodPost, "/api/presets/apply", strings.NewReader(`{"name":"opening"}`))
	applyPreset.Header.Set("Content-Type", "application/json")
	applyPresetRec := httptest.NewRecorder()
	mux.ServeHTTP(applyPresetRec, applyPreset)
	if applyPresetRec.Code != http.StatusOK {
		t.Fatalf("POST /api/presets/apply = %d", applyPresetRec.Code)
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

	pConfirmReq := httptest.NewRequest(http.MethodPost, "/api/pending-facts/confirm", strings.NewReader(`{"id":"p1"}`))
	pConfirmReq.Header.Set("Content-Type", "application/json")
	pConfirmRec := httptest.NewRecorder()
	mux.ServeHTTP(pConfirmRec, pConfirmReq)
	if pConfirmRec.Code != http.StatusOK {
		t.Fatalf("POST /api/pending-facts/confirm = %d", pConfirmRec.Code)
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
