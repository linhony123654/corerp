package runtime

import (
	"bytes"
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
	"corerp/internal/api"
	"corerp/internal/auth"
	"corerp/internal/core"
	"corerp/internal/emotion"
	"corerp/internal/events"
	"corerp/internal/llm"
	"corerp/internal/memory"
	"corerp/internal/simulation"
	"corerp/internal/state"
	"corerp/internal/world"
)

func TestSyncActiveWorldContextUpdatesSceneAndMetadata(t *testing.T) {
	engine := &Engine{
		stateMgr:       state.New(),
		focusCharacter: "V",
		charWorlds: map[string]CharWorld{
			"V": {
				WorldName: "Night City",
				CoreRules: "chrome first",
				Scene: core.SceneState{
					Location:    "Afterlife",
					TimeOfDay:   "night",
					Weather:     "acid rain",
					Characters:  []string{"V", "Johnny"},
					Description: "Back room booth",
				},
			},
		},
	}

	engine.SyncActiveWorldContext()

	if engine.worldName != "Night City" {
		t.Fatalf("worldName = %q, want %q", engine.worldName, "Night City")
	}
	if engine.coreRules != "chrome first" {
		t.Fatalf("coreRules = %q, want %q", engine.coreRules, "chrome first")
	}

	scene := engine.GetState().Scene
	if scene.Location != "Afterlife" {
		t.Fatalf("scene.Location = %q, want %q", scene.Location, "Afterlife")
	}
	if !containsString(scene.Characters, "V") || !containsString(scene.Characters, "Johnny") || !containsString(scene.Characters, "玩家") {
		t.Fatalf("scene.Characters = %#v, want scene participants plus player", scene.Characters)
	}
}

func TestNormalizeSceneForCharacterPreservesExistingParticipants(t *testing.T) {
	scene := normalizeSceneForCharacter(core.SceneState{
		Location:    "废弃地铁站",
		Characters:  []string{"安雅", "用户"},
		Description: "test",
	}, "111", "贾宝玉")

	if len(scene.Characters) != 3 {
		t.Fatalf("scene.Characters len = %d, want 3", len(scene.Characters))
	}
	if scene.Characters[0] != "安雅" || scene.Characters[1] != "贾宝玉" || scene.Characters[2] != "111" {
		t.Fatalf("scene.Characters = %#v, want [安雅 贾宝玉 111]", scene.Characters)
	}
}

func TestSyncActiveWorldContextIgnoresUnknownCharacter(t *testing.T) {
	engine := &Engine{
		stateMgr:       state.New(),
		focusCharacter: "Unknown",
		charWorlds: map[string]CharWorld{
			"V": {
				WorldName: "Night City",
				CoreRules: "chrome first",
				Scene:     core.SceneState{Location: "Afterlife"},
			},
		},
	}

	engine.stateMgr.Set(core.WorldState{
		Scene:         core.SceneState{Location: "Existing"},
		Relationships: map[string]core.Relationship{},
		Variables:     map[string]interface{}{},
		Flags:         map[string]bool{},
	})

	engine.SyncActiveWorldContext()

	if engine.worldName != "" {
		t.Fatalf("worldName = %q, want empty for unknown character", engine.worldName)
	}
	if got := engine.GetState().Scene.Location; got != "Existing" {
		t.Fatalf("scene.Location = %q, want existing scene preserved", got)
	}
}

func TestWorldConfigUsesExplicitActiveWorldPath(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "explicit-world")
	writeTestWorldBundle(t, worldDir, "显式世界", "world first", core.SceneState{
		Location:    "测试街",
		TimeOfDay:   "黄昏",
		Weather:     "阴",
		Characters:  []string{"玩家"},
		Description: "显式 world context",
	})

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.focusCharacter = "临时视角"
	engine.activeWorldPath = worldDir

	cfg, err := engine.GetWorldConfig()
	if err != nil {
		t.Fatalf("GetWorldConfig: %v", err)
	}
	if cfg.Name != "显式世界" {
		t.Fatalf("cfg.Name = %q, want 显式世界", cfg.Name)
	}
}

func TestDirectorLoadsSceneBackgroundNPCsFromPopulation(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "neon-block")
	copyDir(t, filepath.Join("..", "..", "worlds", "neon_block"), worldDir)

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.worldPaths["111"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "霓虹里街区",
		CoreRules: "world first",
		Scene: core.SceneState{
			Location:    "旧街夜市",
			TimeOfDay:   "深夜",
			Weather:     "闷热有雨",
			Characters:  []string{"玩家"},
			Description: "night market",
		},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()

	engine.mu.Lock()
	state := engine.stateMgr.Get()
	candidates := engine.directorCandidatesLocked(state)
	if !containsString(candidates, "蓝姐") {
		engine.mu.Unlock()
		t.Fatalf("directorCandidatesLocked = %#v, want 蓝姐 from scene/location population before direct turn scoring", candidates)
	}
	plan := engine.directTurnLocked("蓝姐怎么看这事？", state)
	engine.mu.Unlock()

	if !containsString(plan.Candidates, "蓝姐") {
		t.Fatalf("director candidates = %#v, want 蓝姐 from scene population", plan.Candidates)
	}
	if got := plan.Selected[0]; got != "蓝姐" {
		t.Fatalf("lead speaker = %q, want 蓝姐", got)
	}
	foundDetail := false
	for _, candidate := range plan.CandidateDetails {
		if candidate.Name == "蓝姐" {
			foundDetail = true
			if !candidate.Selected || !candidate.LocationMatch {
				t.Fatalf("candidate detail = %#v, want selected scene candidate", candidate)
			}
			if !candidate.FactionMatch || !candidate.PressureMatch {
				t.Fatalf("candidate detail = %#v, want faction and pressure match", candidate)
			}
			if candidate.Score <= 0 {
				t.Fatalf("candidate detail score = %v, want > 0", candidate.Score)
			}
			if len(candidate.ScoreBreakdown) == 0 {
				t.Fatalf("candidate detail breakdown = %#v, want score breakdown", candidate.ScoreBreakdown)
			}
			break
		}
	}
	if !foundDetail {
		t.Fatalf("candidate details = %#v, want 蓝姐 detail", plan.CandidateDetails)
	}
	if _, ok := engine.agents.GetCharacter("蓝姐"); !ok {
		t.Fatalf("background npc 蓝姐 should be loaded into agents")
	}
}

func TestUpdatePlayerRoleRewritesScenePresence(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	engine.SyncActiveWorldContext()

	role, err := engine.UpdatePlayerRole(core.PlayerRole{
		Name:           "贾宝玉",
		Description:    "荣国府公子",
		BoundCharacter: "贾宝玉",
	})
	if err != nil {
		t.Fatalf("UpdatePlayerRole: %v", err)
	}
	if role.Name != "贾宝玉" {
		t.Fatalf("role.Name = %q, want 贾宝玉", role.Name)
	}
	scene := engine.GetState().Scene
	if len(scene.Characters) < 2 || scene.Characters[1] != "贾宝玉" {
		t.Fatalf("scene.Characters = %#v, want player role present", scene.Characters)
	}
}

func TestSwitchCharacterLoadsSceneParticipantShellWhenMissing(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	engine.SyncActiveWorldContext()

	engine.mu.Lock()
	state := engine.stateMgr.Get()
	state.Scene.Characters = []string{"111", "街区观察者"}
	engine.stateMgr.Set(state)
	engine.mu.Unlock()

	if _, ok := engine.agents.GetCharacter("街区观察者"); ok {
		t.Fatalf("街区观察者 should not be preloaded")
	}
	if err := engine.SwitchFocusCharacter("街区观察者"); err != nil {
		t.Fatalf("SwitchCharacter scene participant: %v", err)
	}
	if got := engine.GetFocusCharacter(); got != "街区观察者" {
		t.Fatalf("focus character = %q, want 街区观察者", got)
	}
	if _, ok := engine.agents.GetCharacter("街区观察者"); !ok {
		t.Fatalf("scene participant shell was not loaded")
	}
	if !containsString(engine.GetLoadedCharacters(), "街区观察者") {
		t.Fatalf("loaded characters = %#v, want 街区观察者 present", engine.GetLoadedCharacters())
	}
}

func TestSceneParticipantsDoNotFallbackToLoadedCharacters(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)

	engine.mu.Lock()
	state := engine.stateMgr.Get()
	state.Scene.Characters = nil
	engine.stateMgr.Set(state)
	engine.mu.Unlock()

	if participants := engine.GetSceneParticipants(); len(participants) != 0 {
		t.Fatalf("scene participants = %#v, want empty without scene truth", participants)
	}
	if details := engine.GetSceneParticipantDetails(); len(details) != 0 {
		t.Fatalf("participant details = %#v, want empty without scene truth", details)
	}
	if loaded := engine.GetLoadedCharacters(); len(loaded) == 0 {
		t.Fatalf("loaded characters = %#v, want compatibility-loaded roster to remain intact", loaded)
	}
}

func TestDirectorExcludesSceneParticipantShellFromCandidates(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	engine.SyncActiveWorldContext()

	engine.mu.Lock()
	state := engine.stateMgr.Get()
	state.Scene.Characters = []string{"蓝姐", "谭叔", "街区观察者", "玩家"}
	state.Scene.Location = "旧街夜市"
	engine.stateMgr.Set(state)
	engine.ensureSceneParticipantLoadedLocked("街区观察者", state.Scene)
	engine.focusCharacter = "街区观察者"
	plan := engine.directTurnLocked("你们先说现在怎么回事。", engine.stateMgr.Get())
	engine.mu.Unlock()

	if containsString(plan.Candidates, "街区观察者") {
		t.Fatalf("director candidates = %#v, want scene shell excluded", plan.Candidates)
	}
	for _, candidate := range plan.CandidateDetails {
		if candidate.Name == "街区观察者" {
			t.Fatalf("candidate details = %#v, want no scene shell detail", plan.CandidateDetails)
		}
	}
}

func newMultiCharacterTestEngine(t *testing.T) *Engine {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "runtime.db")
	return newRuntimeEngineOnDB(t, dbPath)
}

func newRuntimeEngineOnDB(t *testing.T, dbPath string) *Engine {
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
	agentsMgr.LoadCharacter("安雅", core.Character{
		Identity: core.IdentityEnvelope{
			Name:      "安雅",
			Adaptive:  map[string]float64{"trust": 3},
			Immutable: []string{"alert"},
		},
	})

	charWorlds := map[string]CharWorld{
		"111": {
			WorldName: "夜之城 2077",
			CoreRules: "chrome first",
			Scene: core.SceneState{
				Location:    "废弃地铁站",
				TimeOfDay:   "凌晨 3 点",
				Weather:     "酸雨",
				Characters:  []string{"111", "用户"},
				Description: "111 scene",
			},
		},
		"安雅": {
			WorldName: "安全屋",
			CoreRules: "stay hidden",
			Scene: core.SceneState{
				Location:    "地下安全屋",
				TimeOfDay:   "深夜",
				Weather:     "无风",
				Characters:  []string{"安雅", "用户"},
				Description: "Anya scene",
			},
		},
	}

	engine, err := New(
		store,
		gatekeeper,
		memEngine,
		memory.NewDecayEngine(memEngine.DB()),
		agentsMgr,
		llm.NewRouter(llm.NewAdapter("http://127.0.0.1:1/v1", "", "test-model")),
		"111",
		[]string{"111", "安雅"},
		charWorlds,
	)
	if err != nil {
		t.Fatalf("new runtime engine: %v", err)
	}

	memEngine.PushDialogue(core.Message{Role: "user", Content: "111 hello"}, "111")
	memEngine.PushDialogue(core.Message{Role: "assistant", Content: "111 reply"}, "111")
	memEngine.PushDialogue(core.Message{Role: "user", Content: "anya hello"}, "安雅")
	memEngine.PushDialogue(core.Message{Role: "assistant", Content: "anya reply"}, "安雅")

	return engine
}

func TestLoadStateThenSyncActiveWorldContextKeepsActiveCharacterScene(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)

	staleEvent := core.Event{
		ID:        "scene_old",
		Type:      "scene_init",
		Canonical: true,
		Payload: map[string]interface{}{
			"location":    "旧场景",
			"time_of_day": "傍晚",
			"weather":     "雾",
			"characters":  []interface{}{"安雅", "用户"},
			"description": "stale scene",
		},
	}
	if err := engine.eventStore.Append(staleEvent); err != nil {
		t.Fatalf("append stale event: %v", err)
	}

	if err := engine.LoadState(); err != nil {
		t.Fatalf("load state: %v", err)
	}
	if got := engine.GetState().Scene.Location; got != "旧场景" {
		t.Fatalf("pre-sync location = %q, want stale projected scene", got)
	}

	engine.SyncActiveWorldContext()

	state := engine.GetState()
	if got := state.Scene.Location; got != "废弃地铁站" {
		t.Fatalf("post-sync location = %q, want active character scene", got)
	}
	if got := engine.GetWorldName(); got != "夜之城 2077" {
		t.Fatalf("world = %q, want 夜之城 2077", got)
	}
	if msgs := engine.GetDialogueLimit(10); len(msgs) != 2 || msgs[0].Content != "111 hello" {
		t.Fatalf("dialogue for 111 = %#v, want 111 dialogue restored", msgs)
	}
}

func TestNewEngineDefaultsToAutoChainDirector(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	cfg := engine.GetDirectorConfig()
	if cfg.Mode != "auto_chain" || cfg.MaxSpeakers != 2 {
		t.Fatalf("director config = %#v, want auto_chain/2", cfg)
	}
	if len(cfg.Weights) == 0 {
		t.Fatalf("director weights = %#v, want default weights", cfg.Weights)
	}
}

func TestDirectorConfigCanOverrideWeights(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	cfg := engine.UpdateDirectorConfig(core.DirectorConfig{
		Mode:        "auto_single",
		MaxSpeakers: 1,
		Weights: map[string]float64{
			"mentioned":      50,
			"pressure_match": 11,
		},
	})
	if cfg.Weights["mentioned"] != 50 {
		t.Fatalf("mentioned weight = %v, want 50", cfg.Weights["mentioned"])
	}
	if cfg.Weights["pressure_match"] != 11 {
		t.Fatalf("pressure_match weight = %v, want 11", cfg.Weights["pressure_match"])
	}
	if cfg.Weights["present"] == 0 {
		t.Fatalf("present weight should keep default, got %#v", cfg.Weights)
	}
}

func TestWorldSpecificDirectorConfigLoadsOnWorldSwitch(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldA := filepath.Join(root, "world-a")
	worldB := filepath.Join(root, "world-b")
	writeTestWorldBundle(t, worldA, "世界A", "rules a", core.SceneState{
		Location:    "A街",
		TimeOfDay:   "夜",
		Weather:     "晴",
		Characters:  []string{"111", "玩家"},
		Description: "scene a",
	})
	writeTestWorldBundle(t, worldB, "世界B", "rules b", core.SceneState{
		Location:    "B街",
		TimeOfDay:   "夜",
		Weather:     "雨",
		Characters:  []string{"安雅", "玩家"},
		Description: "scene b",
	})
	if _, err := world.SaveDirectorConfig(worldA, core.DirectorConfig{
		Mode:        "auto_single",
		MaxSpeakers: 1,
		Weights:     map[string]float64{"mentioned": 51},
	}); err != nil {
		t.Fatalf("save director worldA: %v", err)
	}
	if _, err := world.SaveDirectorConfig(worldB, core.DirectorConfig{
		Mode:        "auto_chain",
		MaxSpeakers: 2,
		Weights:     map[string]float64{"mentioned": 93},
	}); err != nil {
		t.Fatalf("save director worldB: %v", err)
	}

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldA,
		"安雅":  worldB,
	})
	engine.charWorlds["111"] = CharWorld{
		WorldName: "世界A",
		CoreRules: "rules a",
		Scene:     core.SceneState{Location: "A街", Characters: []string{"111", "玩家"}},
	}
	engine.charWorlds["安雅"] = CharWorld{
		WorldName: "世界B",
		CoreRules: "rules b",
		Scene:     core.SceneState{Location: "B街", Characters: []string{"安雅", "玩家"}},
	}

	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()
	if got := engine.GetDirectorConfig().Weights["mentioned"]; got != 51 {
		t.Fatalf("world A mentioned weight = %v, want 51", got)
	}

	if err := engine.SwitchFocusCharacter("安雅"); err != nil {
		t.Fatalf("SwitchCharacter: %v", err)
	}
	if got := engine.GetDirectorConfig().Weights["mentioned"]; got != 93 {
		t.Fatalf("world B mentioned weight = %v, want 93", got)
	}
}

func TestNeonBlockWorldCarriesDirectorDefaults(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "neon-block")
	copyDir(t, filepath.Join("..", "..", "worlds", "neon_block"), worldDir)

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.charWorlds["111"] = CharWorld{
		WorldName: "霓虹里街区",
		CoreRules: "world first",
		Scene:     core.SceneState{Location: "旧街夜市", Characters: []string{"111", "玩家"}},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()

	cfg := engine.GetDirectorConfig()
	if cfg.Mode != "auto_chain" || cfg.MaxSpeakers != 2 {
		t.Fatalf("director config = %#v, want neon block defaults", cfg)
	}
	if cfg.Weights["pressure_match"] != 11 || cfg.Weights["hook_match"] != 14 {
		t.Fatalf("director weights = %#v, want neon block world weights", cfg.Weights)
	}
}

func TestNeonBlockWorldPresetCanBeListedAndApplied(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "neon-block")
	copyDir(t, filepath.Join("..", "..", "worlds", "neon_block"), worldDir)

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"蓝姐":  worldDir,
		"111": worldDir,
	})
	engine.charWorlds["蓝姐"] = CharWorld{
		WorldName: "霓虹里街区",
		CoreRules: "world first",
		Scene:     core.SceneState{Location: "旧街夜市", Characters: []string{"蓝姐", "谭叔", "玩家"}},
	}
	engine.focusCharacter = "蓝姐"
	engine.SyncActiveWorldContext()

	presets, err := engine.ListScenarioPresets()
	if err != nil {
		t.Fatalf("ListScenarioPresets: %v", err)
	}
	found := false
	for _, preset := range presets {
		if preset.Name == "opening_witness_conflict" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("presets = %#v, want opening_witness_conflict", presets)
	}

	applied, err := engine.ApplyScenarioPreset("opening_witness_conflict")
	if err != nil {
		t.Fatalf("ApplyScenarioPreset: %v", err)
	}
	if applied.FocusCharacter != "蓝姐" || applied.Scene.Location != "旧街夜市" {
		t.Fatalf("applied preset = %#v", applied)
	}
	if got := engine.GetState().Scene.Description; !strings.Contains(got, "监控黑屏") {
		t.Fatalf("scene description = %q, want opening preset scene", got)
	}
}

func TestEnterWorldAppliesNeonBlockOpeningPreset(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "neon-block")
	copyDir(t, filepath.Join("..", "..", "worlds", "neon_block"), worldDir)
	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{})

	preset, err := engine.EnterWorld(worldDir)
	if err != nil {
		t.Fatalf("EnterWorld: %v", err)
	}
	if preset.Name != "opening_witness_conflict" {
		t.Fatalf("preset.Name = %q, want opening_witness_conflict", preset.Name)
	}
	if got := engine.GetFocusCharacter(); got != "蓝姐" {
		t.Fatalf("focus character = %q, want 蓝姐", got)
	}
	if got := engine.GetWorldName(); got != "霓虹里街区" {
		t.Fatalf("world name = %q, want 霓虹里街区", got)
	}
	if engine.GetWorldPaths()["蓝姐"] != worldDir {
		t.Fatalf("world paths = %#v, want 蓝姐 bound to entered world", engine.GetWorldPaths())
	}
	if got := engine.GetState().Scene.Location; got != "旧街夜市" {
		t.Fatalf("scene location = %q, want 旧街夜市", got)
	}
	insights, err := engine.GetPopulationInsights()
	if err != nil {
		t.Fatalf("GetPopulationInsights after EnterWorld: %v", err)
	}
	if insights.WorldPath != worldDir {
		t.Fatalf("population insights world path = %q, want %q", insights.WorldPath, worldDir)
	}
	if len(insights.Background) == 0 {
		t.Fatalf("population insights background = %#v, want neon_block background NPCs", insights.Background)
	}
}

func TestComposeTurnPromptIncludesRoleDirective(t *testing.T) {
	base := "=== 指令 ===\nbase prompt"
	step := core.TurnStep{
		Index:      0,
		Speaker:    "安雅",
		Kind:       "addressed_reply",
		BudgetMode: "full_load",
	}

	prompt := composeTurnPrompt(base, step, "玩家", nil, nil)
	if !strings.Contains(prompt, "当前 step: #1 | speaker=安雅 | kind=addressed_reply | budget=full_load") {
		t.Fatalf("prompt missing step header: %s", prompt)
	}
	if !strings.Contains(prompt, "被 玩家 明确点名") {
		t.Fatalf("prompt missing addressed reply directive: %s", prompt)
	}
}

func TestStepPromptDirectivesDifferByKind(t *testing.T) {
	lead := strings.Join(stepPromptDirectives(core.TurnStep{Kind: "lead"}, "玩家", nil), "\n")
	support := strings.Join(stepPromptDirectives(core.TurnStep{Kind: "support_response"}, "玩家", nil), "\n")
	tension := strings.Join(stepPromptDirectives(core.TurnStep{Kind: "tension_response"}, "玩家", nil), "\n")

	if !strings.Contains(lead, "正面回应") {
		t.Fatalf("lead directives = %s, want direct response rule", lead)
	}
	if !strings.Contains(support, "关系、态度或站位") {
		t.Fatalf("support directives = %s, want support rule", support)
	}
	if !strings.Contains(tension, "高张力反应位") {
		t.Fatalf("tension directives = %s, want tension rule", tension)
	}
}

func TestFilterAllowedActionsForStep(t *testing.T) {
	base := []string{"speak", "trust", "negotiate", "hide", "move", "threaten", "attack"}

	addressed := filterAllowedActionsForStep(core.TurnStep{Kind: "addressed_reply"}, base)
	if strings.Join(addressed, ",") != "speak,trust,negotiate" {
		t.Fatalf("addressed actions = %v, want speak/trust/negotiate", addressed)
	}

	support := filterAllowedActionsForStep(core.TurnStep{Kind: "support_response"}, base)
	if strings.Join(support, ",") != "trust,speak,negotiate" {
		t.Fatalf("support actions = %v, want trust/speak/negotiate", support)
	}

	tension := filterAllowedActionsForStep(core.TurnStep{Kind: "tension_response"}, base)
	if !containsString(tension, "threaten") || !containsString(tension, "attack") {
		t.Fatalf("tension actions = %v, want threaten/attack present", tension)
	}
}

func TestFilterAllowedActionsForAdaptive(t *testing.T) {
	base := []string{"threaten", "hide", "attack", "speak", "negotiate"}

	highTrust := filterAllowedActionsForAdaptive(base, map[string]float64{"trust": 8, "intimacy": 7})
	if containsString(highTrust, "attack") || containsString(highTrust, "threaten") {
		t.Fatalf("high trust actions = %v, want aggressive actions removed", highTrust)
	}
	if len(highTrust) == 0 || highTrust[0] != "speak" {
		t.Fatalf("high trust actions = %v, want speak-first after filtering", highTrust)
	}

	highFear := filterAllowedActionsForAdaptive(base, map[string]float64{"fear": 8})
	if containsString(highFear, "attack") {
		t.Fatalf("high fear actions = %v, want attack removed", highFear)
	}
	if len(highFear) == 0 || highFear[0] != "hide" {
		t.Fatalf("high fear actions = %v, want hide-first", highFear)
	}
}

func TestNormalizeActionForStepDowngradesOutOfRoleAction(t *testing.T) {
	frame := core.ActionFrame{
		Actor:     "安雅",
		Action:    "attack",
		Target:    "111",
		Intensity: 8,
	}
	normalized := normalizeActionForStep(core.TurnStep{Kind: "addressed_reply"}, frame, []string{"speak", "trust", "negotiate"})
	if normalized.Action != "speak" {
		t.Fatalf("normalized action = %q, want speak", normalized.Action)
	}
	if normalized.Intensity != 1 {
		t.Fatalf("normalized intensity = %d, want 1", normalized.Intensity)
	}
}

func TestComposeTurnPromptIncludesHandoff(t *testing.T) {
	base := "=== 指令 ===\nbase prompt"
	step := core.TurnStep{Index: 1, Speaker: "111", Kind: "support_response", BudgetMode: "normal"}
	handoff := &core.StepHandoff{
		FromSpeaker:    "安雅",
		StepIndex:      0,
		Kind:           "lead",
		Action:         "speak",
		Target:         "玩家",
		OutcomeSummary: "speak->玩家 | events:dialogue",
		Narrative:      "安雅简短地回应了玩家。",
		Events: []core.TraceEvent{{
			ID: "evt_1", Type: "dialogue", Actor: "安雅", Target: "玩家",
		}},
	}

	prompt := composeTurnPrompt(base, step, "玩家", nil, handoff)
	if !strings.Contains(prompt, "上一步交接") {
		t.Fatalf("prompt missing handoff section: %s", prompt)
	}
	if !strings.Contains(prompt, "from: 安雅 | step=1 | kind=lead") {
		t.Fatalf("prompt missing handoff origin: %s", prompt)
	}
	if !strings.Contains(prompt, "summary: speak->玩家 | events:dialogue") {
		t.Fatalf("prompt missing handoff summary: %s", prompt)
	}
}

func TestMultiCharacterHTTPFlowKeepsStateWorldAndDialogueAligned(t *testing.T) {
	auth.Init("")
	auth.SetSecureCookie(false)

	engine := newMultiCharacterTestEngine(t)
	engine.SyncActiveWorldContext()

	server := api.NewServer(engine)
	mux := http.NewServeMux()
	server.Register(mux)

	assertState := func(wantLocation, wantWorld string, wantDialogueFirst string) {
		t.Helper()

		stateReq := httptest.NewRequest(http.MethodGet, "/api/state", nil)
		stateRec := httptest.NewRecorder()
		mux.ServeHTTP(stateRec, stateReq)
		if stateRec.Code != http.StatusOK {
			t.Fatalf("GET /api/state = %d", stateRec.Code)
		}
		var stateResp core.WorldState
		if err := json.NewDecoder(stateRec.Body).Decode(&stateResp); err != nil {
			t.Fatalf("decode state: %v", err)
		}
		if stateResp.Scene.Location != wantLocation {
			t.Fatalf("state scene = %q, want %q", stateResp.Scene.Location, wantLocation)
		}

		worldReq := httptest.NewRequest(http.MethodGet, "/api/world", nil)
		worldRec := httptest.NewRecorder()
		mux.ServeHTTP(worldRec, worldReq)
		if worldRec.Code != http.StatusOK {
			t.Fatalf("GET /api/world = %d", worldRec.Code)
		}
		var worldResp map[string]string
		if err := json.NewDecoder(worldRec.Body).Decode(&worldResp); err != nil {
			t.Fatalf("decode world: %v", err)
		}
		if worldResp["name"] != wantWorld {
			t.Fatalf("world name = %q, want %q", worldResp["name"], wantWorld)
		}

		dialogueReq := httptest.NewRequest(http.MethodGet, "/api/dialogue?limit=10", nil)
		dialogueRec := httptest.NewRecorder()
		mux.ServeHTTP(dialogueRec, dialogueReq)
		if dialogueRec.Code != http.StatusOK {
			t.Fatalf("GET /api/dialogue = %d", dialogueRec.Code)
		}
		var dialogueResp struct {
			Messages []core.Message `json:"messages"`
		}
		if err := json.NewDecoder(dialogueRec.Body).Decode(&dialogueResp); err != nil {
			t.Fatalf("decode dialogue: %v", err)
		}
		if len(dialogueResp.Messages) == 0 || dialogueResp.Messages[0].Content != wantDialogueFirst {
			t.Fatalf("dialogue first = %#v, want first content %q", dialogueResp.Messages, wantDialogueFirst)
		}
	}

	assertState("废弃地铁站", "夜之城 2077", "111 hello")

	body := bytes.NewBufferString(`{"focus_character":"安雅"}`)
	switchReq := httptest.NewRequest(http.MethodPost, "/api/switch", body)
	switchReq.Header.Set("Content-Type", "application/json")
	switchRec := httptest.NewRecorder()
	mux.ServeHTTP(switchRec, switchReq)
	if switchRec.Code != http.StatusOK {
		t.Fatalf("POST /api/switch = %d", switchRec.Code)
	}

	assertState("地下安全屋", "安全屋", "anya hello")
}

func TestConfigurePersistenceAndCharacterConfig(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	charPath := filepath.Join(t.TempDir(), "anya.yml")
	if err := os.WriteFile(charPath, []byte(`identity:
  name: "安雅"
  immutable: ["alert"]
  adaptive: {trust: 3}
  forbidden: ["info_dump"]
  voice: {style: "brief", rhythm: "short"}
  writing_guide: "old"
goals:
  primary:
    - id: survive
      priority: 10
      condition: "always"
`), 0644); err != nil {
		t.Fatalf("write char file: %v", err)
	}

	engine.ConfigurePersistence(t.TempDir(), map[string]string{"安雅": charPath}, map[string]string{"安雅": "worlds/anya_world.yml"})
	cfg, err := engine.GetFocusDefinitionConfig("安雅")
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if cfg.Path != charPath {
		t.Fatalf("config path = %q, want %q", cfg.Path, charPath)
	}

	updated, err := engine.UpdateFocusDefinitionConfig("安雅", core.Character{
		Identity: core.IdentityEnvelope{
			Immutable:    []string{"cool-headed", "guarded"},
			Adaptive:     map[string]float64{"trust": 4},
			Forbidden:    []string{"info_dump", "fourth_wall_break"},
			Voice:        core.VoiceConfig{Style: "spare", Rhythm: "short"},
			WritingGuide: "new guide",
		},
		Goals: []core.Goal{{ID: "survive", Priority: 10, Type: "primary", Condition: "always"}},
	})
	if err != nil {
		t.Fatalf("update config: %v", err)
	}
	if updated.Card.Identity.Name != "安雅" {
		t.Fatalf("updated name = %q, want 安雅", updated.Card.Identity.Name)
	}
	data, err := os.ReadFile(charPath)
	if err != nil {
		t.Fatalf("read updated char file: %v", err)
	}
	if !bytes.Contains(data, []byte("new guide")) {
		t.Fatalf("updated file missing writing guide: %s", string(data))
	}

	char, ok := engine.GetCharacter()
	if !ok {
		t.Fatalf("active character should still exist after config update")
	}
	if char.Identity.Name != "111" {
		t.Fatalf("active character = %q, want unchanged active character 111", char.Identity.Name)
	}

	if err := engine.SwitchFocusCharacter("安雅"); err != nil {
		t.Fatalf("switch to updated character: %v", err)
	}
	updatedChar, ok := engine.GetCharacter()
	if !ok {
		t.Fatalf("updated character should exist")
	}
	if updatedChar.Identity.WritingGuide != "new guide" {
		t.Fatalf("updated writing guide = %q, want new guide", updatedChar.Identity.WritingGuide)
	}
	if got := engine.GetState().Scene.Characters; !containsString(got, "安雅") {
		t.Fatalf("scene characters after switch = %#v, want 安雅 present", got)
	}
}

func TestWorldConfigSceneAndFactsEditing(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "red-mansion")
	if err := os.MkdirAll(filepath.Join(worldDir, "canon"), 0755); err != nil {
		t.Fatalf("mkdir canon: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(worldDir, "scenes"), 0755); err != nil {
		t.Fatalf("mkdir scenes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "world.yml"), []byte("meta:\n  name: 大观园\ncore_rules: |\n  初始规则\n"), 0644); err != nil {
		t.Fatalf("write world: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "scenes", "default.yml"), []byte("scene:\n  location: 怡红院\n  time_of_day: 清晨\n  weather: 晴\n  present_chars:\n    - 111\n    - 玩家\n  atmosphere: 初始场景\n"), 0644); err != nil {
		t.Fatalf("write scene: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "canon", "facts.yml"), []byte("facts:\n  - subject: 宝玉\n    predicate: 身份\n    object: 荣国府公子\n    confidence: 1.0\n"), 0644); err != nil {
		t.Fatalf("write facts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "canon", "ontology.yml"), []byte("ontology:\n  characters: []\n  locations: []\n  factions: []\n  items: []\n  lore: []\n  events: []\n  timelines: []\n  settings: []\n"), 0644); err != nil {
		t.Fatalf("write ontology: %v", err)
	}

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
		"安雅":  "worlds/anya_world.yml",
	})
	engine.worldPaths["111"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "大观园",
		CoreRules: "初始规则",
		Scene: core.SceneState{
			Location:    "怡红院",
			TimeOfDay:   "清晨",
			Weather:     "晴",
			Characters:  []string{"111", "玩家"},
			Description: "初始场景",
		},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()

	cfg, err := engine.GetWorldConfig()
	if err != nil {
		t.Fatalf("GetWorldConfig: %v", err)
	}
	if cfg.Name != "大观园" {
		t.Fatalf("world name = %q, want 大观园", cfg.Name)
	}

	if _, err := engine.UpdateWorldConfig(core.WorldConfig{Name: "荣国府", CoreRules: "新规则"}); err != nil {
		t.Fatalf("UpdateWorldConfig: %v", err)
	}
	if got := engine.GetWorldName(); got != "荣国府" {
		t.Fatalf("active world = %q, want 荣国府", got)
	}

	scenes, err := engine.ListSceneConfigs()
	if err != nil {
		t.Fatalf("ListSceneConfigs: %v", err)
	}
	if scenes.Selected != "default" || len(scenes.Scenes) == 0 {
		t.Fatalf("scenes = %#v, want default scene", scenes)
	}

	if _, err := engine.UpdateSceneConfig(core.SceneConfig{
		Name: "default",
		Scene: core.SceneState{
			Location:    "潇湘馆",
			TimeOfDay:   "夜",
			Weather:     "雨",
			Characters:  []string{"111", "黛玉"},
			Description: "更新场景",
		},
	}); err != nil {
		t.Fatalf("UpdateSceneConfig: %v", err)
	}
	if got := engine.GetState().Scene.Location; got != "潇湘馆" {
		t.Fatalf("state scene = %q, want 潇湘馆", got)
	}

	facts, err := engine.GetCanonFactsConfig()
	if err != nil {
		t.Fatalf("GetCanonFactsConfig: %v", err)
	}
	if len(facts.Facts) != 1 || facts.Facts[0].Subject != "宝玉" {
		t.Fatalf("facts = %#v, want seeded fact", facts.Facts)
	}

	updatedFacts, err := engine.UpdateCanonFactsConfig(core.CanonFactsConfig{
		Facts: []core.FactFrame{
			{Subject: "黛玉", Predicate: "住处", Object: "潇湘馆", Confidence: 1},
			{Subject: "宝玉", Predicate: "住处", Object: "怡红院", Confidence: 1},
		},
	})
	if err != nil {
		t.Fatalf("UpdateCanonFactsConfig: %v", err)
	}
	if len(updatedFacts.Facts) != 2 {
		t.Fatalf("updated facts len = %d, want 2", len(updatedFacts.Facts))
	}

	memFacts, err := engine.memEngine.GetAllFacts("111")
	if err != nil {
		t.Fatalf("GetAllFacts: %v", err)
	}
	if len(memFacts) != 2 || memFacts[0].Subject != "黛玉" {
		t.Fatalf("memory facts = %#v, want replaced canon facts", memFacts)
	}

	population, err := engine.GetPopulationConfig()
	if err != nil {
		t.Fatalf("GetPopulationConfig: %v", err)
	}
	if population.Policy.PromoteThreshold != 10 || population.Path != filepath.ToSlash(filepath.Clean(worldDir)) {
		t.Fatalf("population defaults = %#v", population)
	}
	if len(population.BackgroundNPCs) == 0 {
		t.Fatalf("population should be auto-seeded, got %#v", population)
	}

	updatedPopulation, err := engine.UpdatePopulationConfig(core.PopulationConfig{
		BackgroundNPCs: []core.BackgroundNPC{{
			ID:       "tea_vendor",
			Name:     "茶摊老板",
			Role:     "商贩",
			Location: "荣国府外街",
		}},
		Policy: core.PromotionPolicy{
			PromoteThreshold:  14,
			MajorThreshold:    28,
			InteractionWeight: 4,
		},
	})
	if err != nil {
		t.Fatalf("UpdatePopulationConfig: %v", err)
	}
	if len(updatedPopulation.BackgroundNPCs) != 1 || updatedPopulation.Policy.PromoteThreshold != 14 {
		t.Fatalf("updated population = %#v", updatedPopulation)
	}

	structure, err := engine.GetWorldStructureConfig()
	if err != nil {
		t.Fatalf("GetWorldStructureConfig: %v", err)
	}
	if structure.Path != filepath.ToSlash(filepath.Clean(worldDir)) || structure.Ruleset.Path == "" {
		t.Fatalf("structure defaults = %#v", structure)
	}

	updatedStructure, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
		Ruleset: core.WorldRulesetConfig{
			Rules: []core.WorldRule{{
				ID:      "night_watch",
				Title:   "宵禁",
				Summary: "入夜后外城盘查加严",
			}},
		},
		Seed: core.WorldSeedConfig{
			Premise:          "旧都已乱",
			CurrentSituation: "街头戒严",
			StartingScene:    "宁荣街",
			TimeAnchor:       "深秋",
			Stability:        "fragile",
			Variables: map[string]interface{}{
				"district_alert": "high",
			},
		},
		Factions: []core.WorldFactionConfig{{
			ID:          "city_guard",
			Name:        "巡城司",
			Role:        "law",
			Description: "负责夜间盘查",
		}},
		Locations: []core.WorldLocationConfig{{
			ID:         "ningrong_street",
			Name:       "宁荣街",
			Kind:       "street",
			Controller: "巡城司",
		}},
		Pressures: []core.WorldPressureConfig{{
			ID:        "curfew",
			Name:      "宵禁升级",
			Kind:      "security",
			Intensity: 0.7,
			Target:    "宁荣街",
		}},
	})
	if err != nil {
		t.Fatalf("UpdateWorldStructureConfig: %v", err)
	}
	if len(updatedStructure.Ruleset.Rules) != 1 || updatedStructure.Seed.StartingScene != "宁荣街" {
		t.Fatalf("updated structure = %#v", updatedStructure)
	}
}

func TestReconcilePopulationPromotesBackgroundNPC(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "population-world")
	writeTestWorldBundle(t, worldDir, "人口世界", "人物会被世界抬升", core.SceneState{
		Location:    "镇口",
		TimeOfDay:   "午后",
		Weather:     "晴",
		Characters:  []string{"111", "玩家"},
		Description: "镇口茶摊生意正忙",
	})

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.worldPaths["111"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "人口世界",
		CoreRules: "人物会被世界抬升",
		Scene: core.SceneState{
			Location:    "镇口",
			TimeOfDay:   "午后",
			Weather:     "晴",
			Characters:  []string{"111", "玩家"},
			Description: "镇口茶摊生意正忙",
		},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()

	_, err := engine.UpdatePopulationConfig(core.PopulationConfig{
		BackgroundNPCs: []core.BackgroundNPC{{
			ID:       "tea_vendor",
			Name:     "茶摊老板",
			Role:     "商贩",
			Location: "镇口",
			Traits:   []string{"健谈", "精明"},
			Hooks:    []string{"想知道最近城里的消息"},
		}},
		Policy: core.PromotionPolicy{
			PromoteThreshold:   8,
			MajorThreshold:     20,
			InteractionWeight:  3,
			MentionWeight:      1,
			EventWeight:        2,
			RelationshipWeight: 4,
			SceneWeight:        2,
		},
	})
	if err != nil {
		t.Fatalf("UpdatePopulationConfig: %v", err)
	}

	for _, evt := range []core.Event{
		{
			ID:        "pop_evt_1",
			Type:      "dialogue",
			Actor:     "茶摊老板",
			Target:    "111",
			Payload:   map[string]interface{}{"content": "茶摊老板压低声音，说昨夜巡城司抓了人。"},
			SceneID:   "镇口",
			Canonical: true,
			CreatedAt: time.Now().UTC(),
		},
		{
			ID:        "pop_evt_2",
			Type:      "trust_change",
			Actor:     "111",
			Target:    "茶摊老板",
			Payload:   map[string]interface{}{"delta": 1.5},
			SceneID:   "镇口",
			Canonical: true,
			CreatedAt: time.Now().UTC().Add(time.Second),
		},
		{
			ID:        "pop_evt_3",
			Type:      "user_message",
			Actor:     "user",
			Target:    "111",
			Payload:   map[string]interface{}{"content": "我再问问茶摊老板，昨夜到底发生了什么。"},
			SceneID:   "镇口",
			Canonical: true,
			CreatedAt: time.Now().UTC().Add(2 * time.Second),
		},
	} {
		if err := engine.eventStore.Append(evt); err != nil {
			t.Fatalf("append event %s: %v", evt.ID, err)
		}
	}

	engine.reconcilePopulationLocked()

	population, err := engine.GetPopulationConfig()
	if err != nil {
		t.Fatalf("GetPopulationConfig: %v", err)
	}
	if len(population.PromotedNPCs) != 1 {
		t.Fatalf("promoted npcs = %#v, want 1", population.PromotedNPCs)
	}
	promoted := population.PromotedNPCs[0]
	if promoted.Name != "茶摊老板" || promoted.Status != "major" {
		t.Fatalf("promoted npc = %#v", promoted)
	}
	if promoted.Attention.Score < population.Policy.PromoteThreshold {
		t.Fatalf("promotion score = %.2f, want >= %.2f", promoted.Attention.Score, population.Policy.PromoteThreshold)
	}
	if len(population.IdentityCores) != 1 || population.IdentityCores[0].Name != "茶摊老板" {
		t.Fatalf("identity cores = %#v", population.IdentityCores)
	}
	if got := population.IdentityCores[0].Adaptive["trust"]; got <= 3 {
		t.Fatalf("identity core trust = %.2f, want > 3 after lived events", got)
	}
	if got := population.IdentityCores[0].Adaptive["intimacy"]; got < 0 {
		t.Fatalf("identity core intimacy = %.2f, want non-negative", got)
	}
	char, ok := engine.agents.GetCharacter("茶摊老板")
	if !ok {
		t.Fatalf("promoted runtime character not loaded")
	}
	if got := char.Identity.Adaptive["trust"]; got <= 3 {
		t.Fatalf("runtime character trust = %.2f, want > 3 after sync", got)
	}

	canonical, err := engine.eventStore.GetCanonicalEvents()
	if err != nil {
		t.Fatalf("GetCanonicalEvents: %v", err)
	}
	foundPromotionEvent := false
	for _, evt := range canonical {
		if evt.Type == "population_promoted" && evt.Target == "茶摊老板" {
			foundPromotionEvent = true
			break
		}
	}
	if !foundPromotionEvent {
		t.Fatalf("canonical events missing population_promoted: %#v", canonical)
	}
}

func TestReconcilePopulationDemotesStalePromotedNPC(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "population-demotion-world")
	writeTestWorldBundle(t, worldDir, "人口降级世界", "长期不再被世界卷入的人物应退回背景人口", core.SceneState{
		Location:    "镇口",
		TimeOfDay:   "午后",
		Weather:     "晴",
		Characters:  []string{"111", "玩家"},
		Description: "镇口茶摊曾经很热闹",
	})

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.worldPaths["111"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "人口降级世界",
		CoreRules: "长期不再被世界卷入的人物应退回背景人口",
		Scene: core.SceneState{
			Location:    "镇口",
			TimeOfDay:   "午后",
			Weather:     "晴",
			Characters:  []string{"111", "玩家"},
			Description: "镇口茶摊曾经很热闹",
		},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()
	engine.stateMgr.Set(core.WorldState{
		Scene: core.SceneState{
			Location:    "远郊",
			TimeOfDay:   "深夜",
			Weather:     "雨",
			Characters:  []string{"111", "玩家"},
			Description: "当前场景已经离开镇口很久",
		},
		Relationships: map[string]core.Relationship{},
		Variables:     map[string]interface{}{},
		Flags:         map[string]bool{},
	})

	if _, err := engine.UpdatePopulationConfig(core.PopulationConfig{
		BackgroundNPCs: []core.BackgroundNPC{{
			ID:       "tea_vendor",
			Name:     "茶摊老板",
			Role:     "商贩",
			Location: "镇口",
			Traits:   []string{"健谈", "精明"},
			Hooks:    []string{"想知道最近城里的消息"},
			Attention: core.PopulationAttention{
				Score: 12,
			},
		}},
		PromotedNPCs: []core.PromotedNPC{{
			ID:           "tea_vendor",
			Name:         "茶摊老板",
			From:         "background",
			Status:       "promoted",
			IdentityCore: "tea_vendor_core",
			Attention: core.PopulationAttention{
				Score: 12,
			},
			LastEventID: "old_pop_evt",
		}},
		IdentityCores: []core.IdentityCoreConfig{{
			ID:          "tea_vendor_core",
			Name:        "茶摊老板",
			Immutable:   []string{"健谈", "精明"},
			Adaptive:    map[string]float64{"trust": 3, "fear": 2},
			SpeechHints: []string{"健谈"},
			Drives:      []string{"维持茶摊消息网"},
		}},
		Policy: core.PromotionPolicy{
			PromoteThreshold:   8,
			MajorThreshold:     20,
			InteractionWeight:  3,
			MentionWeight:      1,
			EventWeight:        2,
			RelationshipWeight: 4,
			SceneWeight:        2,
		},
	}); err != nil {
		t.Fatalf("UpdatePopulationConfig: %v", err)
	}

	oldEvent := core.Event{
		ID:        "old_pop_evt",
		Type:      "dialogue",
		Actor:     "茶摊老板",
		Target:    "111",
		Payload:   map[string]interface{}{"content": "茶摊老板曾经提供过关键消息。"},
		SceneID:   "镇口",
		Canonical: true,
		CreatedAt: time.Now().UTC().Add(-populationAttentionEventWindow - time.Hour),
	}
	if err := engine.eventStore.Append(oldEvent); err != nil {
		t.Fatalf("append stale population event: %v", err)
	}

	engine.reconcilePopulationLocked()

	population, err := engine.GetPopulationConfig()
	if err != nil {
		t.Fatalf("GetPopulationConfig: %v", err)
	}
	if len(population.PromotedNPCs) != 0 {
		t.Fatalf("promoted npcs = %#v, want stale promoted NPC demoted", population.PromotedNPCs)
	}
	if len(population.BackgroundNPCs) != 1 || population.BackgroundNPCs[0].Attention.Score >= population.Policy.PromoteThreshold/2 {
		t.Fatalf("background attention after demotion = %#v, want below demotion threshold", population.BackgroundNPCs)
	}

	canonical, err := engine.eventStore.GetCanonicalEvents()
	if err != nil {
		t.Fatalf("GetCanonicalEvents: %v", err)
	}
	foundDemotionEvent := false
	for _, evt := range canonical {
		if evt.Type == "population_demoted" && evt.Target == "茶摊老板" {
			foundDemotionEvent = true
			break
		}
	}
	if !foundDemotionEvent {
		t.Fatalf("canonical events missing population_demoted: %#v", canonical)
	}

	insights, err := engine.GetPopulationInsights()
	if err != nil {
		t.Fatalf("GetPopulationInsights: %v", err)
	}
	if len(insights.Promoted) != 0 {
		t.Fatalf("promoted insights = %#v, want empty after demotion", insights.Promoted)
	}
	var demotionInHistory bool
	for _, npc := range insights.Background {
		if npc.Name != "茶摊老板" {
			continue
		}
		for _, item := range npc.History {
			if item.Type == "population_demoted" {
				demotionInHistory = true
				break
			}
		}
	}
	if !demotionInHistory {
		t.Fatalf("background insights = %#v, want population_demoted history after demotion", insights.Background)
	}
}

func TestGetPopulationInsightsSeedsBackgroundPopulation(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "seeded-population-world")
	writeTestWorldBundle(t, worldDir, "种群世界", "世界里应该先有人", core.SceneState{
		Location:    "镇口",
		TimeOfDay:   "午后",
		Weather:     "晴",
		Characters:  []string{"111", "玩家"},
		Description: "街面上有人来人往",
	})

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.worldPaths["111"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "种群世界",
		CoreRules: "世界里应该先有人",
		Scene: core.SceneState{
			Location:    "镇口",
			TimeOfDay:   "午后",
			Weather:     "晴",
			Characters:  []string{"111", "玩家"},
			Description: "街面上有人来人往",
		},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()

	insights, err := engine.GetPopulationInsights()
	if err != nil {
		t.Fatalf("GetPopulationInsights: %v", err)
	}
	if len(insights.Background) == 0 {
		t.Fatalf("background insights = %#v, want auto-seeded background population", insights)
	}
	if insights.Background[0].Name == "" || insights.Background[0].WorldPath == "" {
		t.Fatalf("background insight missing fields: %#v", insights.Background[0])
	}
}

func TestQuarantineAndPendingFactsReview(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)

	qevt := core.Event{
		ID:            "q_evt_1",
		Type:          "fact_extracted",
		Actor:         "111",
		Payload:       map[string]interface{}{"narrative": "test"},
		Canonical:     false,
		Confidence:    0.4,
		Confirmations: 0,
		CreatedAt:     time.Now(),
	}
	if err := engine.eventStore.Append(qevt); err != nil {
		t.Fatalf("append quarantined event: %v", err)
	}

	pipeline := memory.NewConfidencePipeline(engine.memEngine.DB())
	if err := pipeline.Migrate(); err != nil {
		t.Fatalf("migrate pending facts: %v", err)
	}
	if err := pipeline.SubmitFact(core.FactFrame{
		Subject: "V", Predicate: "身份", Object: "雇佣兵", Confidence: 0.4,
	}, "111", "llm_extracted"); err != nil {
		t.Fatalf("submit pending fact: %v", err)
	}

	events, err := engine.ListQuarantineEvents("111", 10)
	if err != nil {
		t.Fatalf("ListQuarantineEvents: %v", err)
	}
	if len(events) != 1 || events[0].ID != "q_evt_1" {
		t.Fatalf("quarantine events = %#v, want q_evt_1", events)
	}
	if err := engine.PromoteQuarantineEvent("q_evt_1"); err != nil {
		t.Fatalf("PromoteQuarantineEvent: %v", err)
	}
	promoted, err := engine.eventStore.GetByID("q_evt_1")
	if err != nil || !promoted.Canonical {
		t.Fatalf("promoted event = %#v, err=%v", promoted, err)
	}

	items, stats, err := engine.ListPendingFacts("111", 10)
	if err != nil {
		t.Fatalf("ListPendingFacts: %v", err)
	}
	if len(items) != 1 || stats["pending_total"].(int) != 1 {
		t.Fatalf("pending facts = %#v, stats=%#v", items, stats)
	}
	if err := engine.ConfirmPendingFact(items[0].ID); err != nil {
		t.Fatalf("ConfirmPendingFact: %v", err)
	}
	items, _, err = engine.ListPendingFacts("111", 10)
	if err != nil {
		t.Fatalf("ListPendingFacts after confirm: %v", err)
	}
	if items[0].Confirmations != 1 {
		t.Fatalf("confirmations = %d, want 1", items[0].Confirmations)
	}
	if err := engine.PromotePendingFact(items[0].ID); err != nil {
		t.Fatalf("PromotePendingFact: %v", err)
	}
	facts, err := engine.memEngine.GetAllFacts("111")
	if err != nil {
		t.Fatalf("GetAllFacts: %v", err)
	}
	found := false
	for _, fact := range facts {
		if fact.Subject == "V" && fact.Predicate == "身份" && fact.Object == "雇佣兵" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("promoted fact not found in semantic memory: %#v", facts)
	}
	items, _, err = engine.ListPendingFacts("111", 10)
	if err != nil {
		t.Fatalf("ListPendingFacts after promote: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("pending facts still present = %#v", items)
	}
}

func TestDirectorAutoSingleSwitchesSpeaker(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	engine.SyncActiveWorldContext()
	engine.UpdateDirectorConfig(core.DirectorConfig{Mode: "auto_single", MaxSpeakers: 1})

	engine.stateMgr.Set(core.WorldState{
		Scene: core.SceneState{
			Location:   "大观园",
			Characters: []string{"111", "安雅"},
		},
		Relationships: map[string]core.Relationship{},
		Variables:     map[string]interface{}{},
		Flags:         map[string]bool{},
		Tension:       0.7,
	})

	engine.memEngine.PushDialogue(core.Message{Role: "assistant", Content: "111 recent reply"}, "111")

	ch, err := engine.ProcessTurn("安雅，你怎么看？")
	if err != nil {
		t.Fatalf("ProcessTurn: %v", err)
	}
	for range ch {
	}

	if got := engine.GetFocusCharacter(); got != "安雅" {
		t.Fatalf("focus character = %q, want 安雅", got)
	}
	plan := engine.GetDirectorPlan()
	if plan.Mode != "auto_single" || len(plan.Selected) == 0 || plan.Selected[0] != "安雅" {
		t.Fatalf("director plan = %#v, want 安雅 selected", plan)
	}
	if len(plan.WorldSignals) == 0 {
		t.Fatalf("director world signals missing: %#v", plan)
	}
	if len(plan.CandidateDetails) == 0 || len(plan.CandidateDetails[0].DominantFactors) == 0 {
		t.Fatalf("director dominant factors missing: %#v", plan.CandidateDetails)
	}
}

func TestDirectorAutoChainBuildsRoleBasedSteps(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	engine.SyncActiveWorldContext()
	engine.UpdateDirectorConfig(core.DirectorConfig{Mode: "auto_chain", MaxSpeakers: 3})

	engine.stateMgr.Set(core.WorldState{
		Scene: core.SceneState{
			Location:   "大观园",
			Characters: []string{"111", "安雅"},
		},
		Relationships: map[string]core.Relationship{
			"安雅_111": {Trust: 0.6, Intimacy: 0.4},
		},
		Variables: map[string]interface{}{},
		Flags:     map[string]bool{},
		Tension:   0.8,
	})

	ch, err := engine.ProcessTurn("安雅先说，111补一句。")
	if err != nil {
		t.Fatalf("ProcessTurn: %v", err)
	}
	for range ch {
	}

	plan := engine.GetDirectorPlan()
	if len(plan.Steps) < 2 {
		t.Fatalf("director steps = %#v, want at least 2 steps", plan.Steps)
	}
	if plan.Steps[0].Speaker != "安雅" || plan.Steps[0].Kind != "lead" {
		t.Fatalf("lead step = %#v, want 安雅 lead", plan.Steps[0])
	}
	if plan.Steps[1].Speaker != "111" {
		t.Fatalf("followup step = %#v, want 111 second", plan.Steps[1])
	}
	if plan.Steps[1].Kind != "addressed_reply" && plan.Steps[1].Kind != "support_response" && plan.Steps[1].Kind != "tension_response" {
		t.Fatalf("followup kind = %q, want role-based kind", plan.Steps[1].Kind)
	}
}

func TestDirectorCanSelectPromotedPopulationNPC(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "promoted-director-world")
	writeTestWorldBundle(t, worldDir, "晋升世界", "被关注的人会进入叙事前台", core.SceneState{
		Location:    "镇口",
		TimeOfDay:   "傍晚",
		Weather:     "阴",
		Characters:  []string{"111", "玩家"},
		Description: "镇口有个熟悉的茶摊",
	})

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
		"安雅":  worldDir,
	})
	engine.worldPaths["111"] = worldDir
	engine.worldPaths["安雅"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "晋升世界",
		CoreRules: "被关注的人会进入叙事前台",
		Scene: core.SceneState{
			Location:    "镇口",
			TimeOfDay:   "傍晚",
			Weather:     "阴",
			Characters:  []string{"111", "玩家"},
			Description: "镇口有个熟悉的茶摊",
		},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()

	_, err := engine.UpdatePopulationConfig(core.PopulationConfig{
		BackgroundNPCs: []core.BackgroundNPC{{
			ID:       "tea_vendor",
			Name:     "茶摊老板",
			Role:     "商贩",
			Location: "镇口",
			Traits:   []string{"健谈"},
			Hooks:    []string{"知道街上的风声"},
		}},
		Policy: core.PromotionPolicy{
			PromoteThreshold:   6,
			MajorThreshold:     12,
			InteractionWeight:  3,
			MentionWeight:      2,
			EventWeight:        2,
			RelationshipWeight: 2,
			SceneWeight:        1,
		},
	})
	if err != nil {
		t.Fatalf("UpdatePopulationConfig: %v", err)
	}

	for _, evt := range []core.Event{
		{
			ID:        "dir_pop_1",
			Type:      "dialogue",
			Actor:     "茶摊老板",
			Target:    "111",
			Payload:   map[string]interface{}{"content": "茶摊老板低声说，昨夜城门外又出事了。"},
			SceneID:   "镇口",
			Canonical: true,
			CreatedAt: time.Now().UTC(),
		},
		{
			ID:        "dir_pop_2",
			Type:      "user_message",
			Actor:     "user",
			Target:    "111",
			Payload:   map[string]interface{}{"content": "我想先听茶摊老板怎么说。"},
			SceneID:   "镇口",
			Canonical: true,
			CreatedAt: time.Now().UTC().Add(time.Second),
		},
	} {
		if err := engine.eventStore.Append(evt); err != nil {
			t.Fatalf("append event %s: %v", evt.ID, err)
		}
	}

	engine.reconcilePopulationLocked()
	engine.UpdateDirectorConfig(core.DirectorConfig{Mode: "auto_single", MaxSpeakers: 1})
	engine.stateMgr.Set(core.WorldState{
		Scene: core.SceneState{
			Location:   "镇口",
			Characters: []string{"111", "玩家"},
		},
		Relationships: map[string]core.Relationship{},
		Variables:     map[string]interface{}{},
		Flags:         map[string]bool{},
		Tension:       0.4,
	})

	ch, err := engine.ProcessTurn("茶摊老板，你先说。")
	if err != nil {
		t.Fatalf("ProcessTurn: %v", err)
	}
	for range ch {
	}

	plan := engine.GetDirectorPlan()
	if len(plan.Selected) == 0 || plan.Selected[0] != "茶摊老板" {
		t.Fatalf("director selected = %#v, want 茶摊老板", plan.Selected)
	}
	if got := engine.GetFocusCharacter(); got != "茶摊老板" {
		t.Fatalf("focus character = %q, want 茶摊老板", got)
	}
}

func TestNeonBlockPopulationPromotionLoop(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "neon-block")
	copyDir(t, filepath.Join("..", "..", "worlds", "neon_block"), worldDir)

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.worldPaths["111"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "霓虹里街区",
		CoreRules: "world first",
		Scene: core.SceneState{
			Location:    "旧街夜市",
			TimeOfDay:   "深夜",
			Weather:     "闷热有雨",
			Characters:  []string{"玩家"},
			Description: "night market",
		},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()

	now := time.Now().UTC()
	eventsToAppend := []core.Event{
		{
			ID:        "nb_evt_1",
			Type:      "dialogue",
			Actor:     "蓝姐",
			Target:    "玩家",
			Payload:   map[string]interface{}{"content": "蓝姐压低声音说，监控坏掉前最后一个进店的人神色很怪。"},
			SceneID:   "旧街夜市",
			Canonical: true,
			CreatedAt: now,
		},
		{
			ID:        "nb_evt_2",
			Type:      "user_message",
			Actor:     "user",
			Target:    "玩家",
			Payload:   map[string]interface{}{"content": "我想先问蓝姐她看见了什么。"},
			SceneID:   "旧街夜市",
			Canonical: true,
			CreatedAt: now.Add(time.Second),
		},
		{
			ID:        "nb_evt_3",
			Type:      "trust_change",
			Actor:     "蓝姐",
			Target:    "玩家",
			Payload:   map[string]interface{}{"delta": 1.8},
			SceneID:   "旧街夜市",
			Canonical: true,
			CreatedAt: now.Add(2 * time.Second),
		},
	}
	for _, evt := range eventsToAppend {
		if err := engine.eventStore.Append(evt); err != nil {
			t.Fatalf("append event %s: %v", evt.ID, err)
		}
	}

	engine.mu.Lock()
	engine.reconcilePopulationLocked()
	engine.mu.Unlock()

	cfg, err := engine.GetPopulationConfig()
	if err != nil {
		t.Fatalf("GetPopulationConfig: %v", err)
	}
	foundPromoted := false
	for _, npc := range cfg.PromotedNPCs {
		if npc.Name == "蓝姐" {
			foundPromoted = true
			if npc.Status == "" {
				t.Fatalf("promoted npc missing status: %#v", npc)
			}
			break
		}
	}
	if !foundPromoted {
		t.Fatalf("promoted npcs = %#v, want 蓝姐 promoted", cfg.PromotedNPCs)
	}

	insights, err := engine.GetPopulationInsights()
	if err != nil {
		t.Fatalf("GetPopulationInsights: %v", err)
	}
	foundHistory := false
	for _, npc := range insights.Promoted {
		if npc.Name == "蓝姐" {
			foundHistory = len(npc.History) > 0
			break
		}
	}
	if !foundHistory {
		t.Fatalf("promoted insights = %#v, want 蓝姐 with promotion history", insights.Promoted)
	}
}

func TestTurnTraceCapturesStepTraces(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	engine.SyncActiveWorldContext()
	engine.UpdateDirectorConfig(core.DirectorConfig{Mode: "auto_chain", MaxSpeakers: 2})

	engine.stateMgr.Set(core.WorldState{
		Scene: core.SceneState{
			Location:   "大观园",
			Characters: []string{"111", "安雅"},
		},
		Relationships: map[string]core.Relationship{
			"111_安雅": {Trust: 0.7, Intimacy: 0.3},
		},
		Variables: map[string]interface{}{},
		Flags:     map[string]bool{},
		Tension:   0.7,
	})

	ch, err := engine.ProcessTurn("安雅，你怎么看？")
	if err != nil {
		t.Fatalf("ProcessTurn: %v", err)
	}
	for range ch {
	}

	trace, ok := engine.GetLatestTrace()
	if !ok {
		t.Fatal("latest trace missing")
	}
	if len(trace.StepTraces) == 0 {
		t.Fatalf("step traces missing: %#v", trace)
	}
	if trace.StepTraces[0].Step.Speaker == "" {
		t.Fatalf("step trace speaker empty: %#v", trace.StepTraces[0])
	}
	if len(trace.StepTraces) > 1 && trace.StepTraces[1].Handoff == nil {
		t.Fatalf("second step missing handoff: %#v", trace.StepTraces[1])
	}
}

func TestBranchAndSaveDiffAndMerge(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	engine.ConfigurePersistence(t.TempDir(), map[string]string{}, map[string]string{})
	engine.SyncActiveWorldContext()

	engine.SeedScene(core.SceneState{
		Location:    "A",
		TimeOfDay:   "day",
		Weather:     "clear",
		Characters:  []string{"111", "用户"},
		Description: "scene A",
	})
	engine.SetTension(0.2)
	slotA, err := engine.CreateSaveSlot("slot-a", "main", "base")
	if err != nil {
		t.Fatalf("CreateSaveSlot slot-a: %v", err)
	}

	evt := events.BuildEvent("flag_set", "system", "", map[string]interface{}{"key": "alt_only"})
	evt.Branch = "alt"
	evt.Canonical = true
	if err := engine.eventStore.Append(evt); err != nil {
		t.Fatalf("append alt flag event: %v", err)
	}
	evt2 := events.BuildEvent("variable_set", "system", "", map[string]interface{}{"key": "route", "value": "alt"})
	evt2.Branch = "alt"
	evt2.Canonical = true
	if err := engine.eventStore.Append(evt2); err != nil {
		t.Fatalf("append alt variable event: %v", err)
	}

	engine.SetTension(0.9)
	slotB, err := engine.CreateSaveSlot("slot-b", "main", "changed")
	if err != nil {
		t.Fatalf("CreateSaveSlot slot-b: %v", err)
	}

	saveDiff, err := engine.CompareSaveSlots(slotA.Name, slotB.Name)
	if err != nil {
		t.Fatalf("CompareSaveSlots: %v", err)
	}
	if saveDiff.Tension == nil {
		t.Fatalf("save diff tension = nil, want difference")
	}

	branchDiff, err := engine.CompareBranchesDetailed("main", "alt", -1)
	if err != nil {
		t.Fatalf("CompareBranchesDetailed: %v", err)
	}
	if branchDiff.Flags == nil || branchDiff.Variables == nil {
		t.Fatalf("branch diff = %#v, want flags and variables diff", branchDiff)
	}

	result, err := engine.MergeBranchState("alt", "main", true, true)
	if err != nil {
		t.Fatalf("MergeBranchState: %v", err)
	}
	if result.EventsAppended == 0 {
		t.Fatalf("merge result = %#v, want appended events", result)
	}
}

func TestTurnTraceRecorded(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	engine.SyncActiveWorldContext()

	ch, err := engine.ProcessTurn("安雅，你现在在想什么？")
	if err != nil {
		t.Fatalf("ProcessTurn: %v", err)
	}
	for range ch {
	}

	trace, ok := engine.GetLatestTrace()
	if !ok {
		t.Fatal("latest trace missing")
	}
	if trace.UserInput == "" || trace.FocusCharacter == "" {
		t.Fatalf("trace = %#v, want user input and focus_character", trace)
	}
	if trace.Turn == 0 {
		t.Fatalf("trace turn = 0")
	}
	if len(trace.DirectorPlan.WorldSignals) == 0 {
		t.Fatalf("trace world signals missing: %#v", trace.DirectorPlan)
	}
	if engine.currentWorldPathLocked() != "" && len(trace.WorldMetrics.PopulationHighlights) == 0 {
		t.Fatalf("trace population highlights missing: %#v", trace.WorldMetrics)
	}
}

func TestCreateAndLoadSaveSlot(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	engine.ConfigurePersistence(t.TempDir(), map[string]string{}, map[string]string{})
	engine.SyncActiveWorldContext()
	engine.SeedScene(core.SceneState{
		Location:    "废弃地铁站",
		TimeOfDay:   "凌晨",
		Weather:     "酸雨",
		Characters:  []string{"111", "用户"},
		Description: "slot scene",
	})

	slot, err := engine.CreateSaveSlot("checkpoint-a", "main", "before switch")
	if err != nil {
		t.Fatalf("create save slot: %v", err)
	}
	if slot.Name != "checkpoint-a" {
		t.Fatalf("slot name = %q", slot.Name)
	}

	if err := engine.SwitchFocusCharacter("安雅"); err != nil {
		t.Fatalf("switch character: %v", err)
	}
	loaded, err := engine.LoadSaveSlot("checkpoint-a")
	if err != nil {
		t.Fatalf("load save slot: %v", err)
	}
	if loaded.Character != "" {
		t.Fatalf("loaded character mirror = %q, want empty on canonical save output", loaded.Character)
	}
	if got := engine.GetState().Scene.Location; got != "废弃地铁站" {
		t.Fatalf("loaded scene = %q, want 废弃地铁站", got)
	}
	if got := engine.GetState().Scene.Characters; len(got) < 1 || got[0] != "111" {
		t.Fatalf("loaded scene characters = %#v, want active character normalized", got)
	}
}

func TestCreateAndLoadSaveSlotWithoutEvents(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	engine.ConfigurePersistence(t.TempDir(), map[string]string{}, map[string]string{})
	engine.SyncActiveWorldContext()

	if _, err := engine.UpdatePlayerRole(core.PlayerRole{
		Name:           "无事件玩家",
		Description:    "快照回滚测试",
		BoundCharacter: "111",
	}); err != nil {
		t.Fatalf("UpdatePlayerRole: %v", err)
	}

	state := engine.GetState()
	state.Scene = core.SceneState{
		Location:    "未提交场景",
		TimeOfDay:   "清晨",
		Weather:     "晴",
		Characters:  []string{"111", "无事件玩家"},
		Description: "snapshot only",
	}
	engine.stateMgr.Set(state)

	slot, err := engine.CreateSaveSlot("snapshot-only", "main", "before any events")
	if err != nil {
		t.Fatalf("CreateSaveSlot without events: %v", err)
	}
	if slot.EventID != "" {
		t.Fatalf("slot.EventID = %q, want empty for snapshot-only save", slot.EventID)
	}

	mutated := engine.GetState()
	mutated.Scene.Location = "已变更场景"
	engine.stateMgr.Set(mutated)

	loaded, err := engine.LoadSaveSlot("snapshot-only")
	if err != nil {
		t.Fatalf("LoadSaveSlot without events: %v", err)
	}
	if got := loaded.WorldState.Scene.Location; got != "未提交场景" {
		t.Fatalf("loaded slot scene = %q, want 未提交场景", got)
	}
	if got := engine.GetState().Scene.Location; got != "未提交场景" {
		t.Fatalf("engine scene = %q, want 未提交场景", got)
	}
	if got := engine.GetPlayerRole().Name; got != "无事件玩家" {
		t.Fatalf("player role = %q, want 无事件玩家", got)
	}
}

func TestLoadSaveSlotPrefersFocusCharacterOverLegacyCharacter(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	engine.ConfigurePersistence(t.TempDir(), map[string]string{}, map[string]string{})
	engine.SyncActiveWorldContext()

	if err := engine.writeSaveSlots([]core.SaveSlot{{
		Name:           "focus-wins",
		Branch:         "main",
		Character:      "111",
		FocusCharacter: "安雅",
		PlayerRole:     core.PlayerRole{Name: "玩家"},
		WorldState: core.WorldState{
			Scene: core.SceneState{
				Location:    "地下安全屋",
				TimeOfDay:   "深夜",
				Weather:     "无风",
				Characters:  []string{"安雅", "玩家"},
				Description: "focus should win",
			},
		},
	}}); err != nil {
		t.Fatalf("writeSaveSlots: %v", err)
	}

	loaded, err := engine.LoadSaveSlot("focus-wins")
	if err != nil {
		t.Fatalf("LoadSaveSlot: %v", err)
	}
	if loaded.FocusCharacter != "安雅" {
		t.Fatalf("loaded.FocusCharacter = %q, want 安雅", loaded.FocusCharacter)
	}
	if got := engine.GetFocusCharacter(); got != "安雅" {
		t.Fatalf("focus character = %q, want 安雅 to win over legacy Character", got)
	}
}

func TestScenarioPresetCreateAndApply(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	engine.ConfigurePersistence(t.TempDir(), map[string]string{}, map[string]string{})
	engine.SyncActiveWorldContext()

	if _, err := engine.UpdatePlayerRole(core.PlayerRole{
		Name:           "测试玩家",
		Description:    "作者预设身份",
		BoundCharacter: "111",
	}); err != nil {
		t.Fatalf("UpdatePlayerRole: %v", err)
	}

	engine.SeedScene(core.SceneState{
		Location:    "作者控制台",
		TimeOfDay:   "黄昏",
		Weather:     "多云",
		Characters:  []string{"111", "用户"},
		Description: "preset scene",
	})

	preset, err := engine.CreateScenarioPreset("opening", "main", "初始局面")
	if err != nil {
		t.Fatalf("CreateScenarioPreset: %v", err)
	}
	if preset.Name != "opening" {
		t.Fatalf("preset.Name = %q, want opening", preset.Name)
	}

	if err := engine.SwitchFocusCharacter("安雅"); err != nil {
		t.Fatalf("SwitchCharacter 安雅: %v", err)
	}
	engine.SeedScene(core.SceneState{
		Location:    "地下安全屋",
		TimeOfDay:   "深夜",
		Weather:     "无风",
		Characters:  []string{"安雅", "用户"},
		Description: "other scene",
	})

	applied, err := engine.ApplyScenarioPreset("opening")
	if err != nil {
		t.Fatalf("ApplyScenarioPreset: %v", err)
	}
	if applied.FocusCharacter != "111" {
		t.Fatalf("applied.FocusCharacter = %q, want 111", applied.FocusCharacter)
	}
	if got := engine.GetState().Scene.Location; got != "作者控制台" {
		t.Fatalf("scene.Location = %q, want 作者控制台", got)
	}
	if got := engine.GetPlayerRole().Name; got != "测试玩家" {
		t.Fatalf("player role = %q, want 测试玩家", got)
	}
}

func TestApplyScenarioPresetPrefersFocusCharacterOverLegacyCharacter(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	engine.ConfigurePersistence(t.TempDir(), map[string]string{}, map[string]string{})
	engine.SyncActiveWorldContext()

	engine.mu.Lock()
	if err := engine.writeScenarioPresetsLocked([]core.ScenarioPreset{{
		Name:           "focus-wins",
		Branch:         "main",
		Character:      "111",
		FocusCharacter: "安雅",
		PlayerRole:     core.PlayerRole{Name: "玩家"},
		Scene: core.SceneState{
			Location:    "地下安全屋",
			TimeOfDay:   "深夜",
			Weather:     "无风",
			Characters:  []string{"安雅", "玩家"},
			Description: "preset focus should win",
		},
	}}); err != nil {
		engine.mu.Unlock()
		t.Fatalf("writeScenarioPresetsLocked: %v", err)
	}
	engine.mu.Unlock()

	applied, err := engine.ApplyScenarioPreset("focus-wins")
	if err != nil {
		t.Fatalf("ApplyScenarioPreset: %v", err)
	}
	if applied.FocusCharacter != "安雅" {
		t.Fatalf("applied.FocusCharacter = %q, want 安雅", applied.FocusCharacter)
	}
	if got := engine.GetFocusCharacter(); got != "安雅" {
		t.Fatalf("focus character = %q, want 安雅 to win over legacy Character", got)
	}
}

func TestExperimentReportCreateAndList(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	engine.ConfigurePersistence(t.TempDir(), map[string]string{}, map[string]string{})

	saved, err := engine.CreateExperimentReport(core.ExperimentReport{
		Name:              "neon-compare",
		Note:              "guard pressure divergence",
		BatchCount:        36,
		SourceInstanceID:  "default",
		CompareInstanceID: "alt-guard",
		CurrentCheckpoint: "neon-compare-current",
		CompareCheckpoint: "neon-compare-compare",
		OutcomeSummary:    []string{"default vs alt-guard"},
		Conclusion:        []string{"长期张力主导：default（gap 0.70）"},
		Current: core.ExperimentSnapshot{
			InstanceID:         "default",
			FocusCharacter:     "111",
			Participants:       []string{"111", "玩家"},
			ParticipantDetails: []core.ParticipantSummary{{Name: "111", Kind: "persona", Source: "character_definition", Loaded: true, Switchable: true, Present: true, Focus: true}},
			SceneLocation:      "外城",
			SceneDescription:   "长窗口实验",
			TickCount:          36,
			Tension:            0.8,
			TrajectorySummary:  []string{"trend a"},
			DirectorPlan:       &core.DirectorPlan{Mode: "auto_chain", Selected: []string{"111"}, WorldSignals: []string{"pressure:curfew"}},
			LatestTrace:        &core.TurnTrace{Turn: 12, FocusCharacter: "111", UserInput: "继续观察"},
		},
		Compare: &core.ExperimentSnapshot{
			InstanceID:        "alt-guard",
			FocusCharacter:    "巡夜人",
			SceneLocation:     "外城",
			TickCount:         36,
			Tension:           0.1,
			TrajectorySummary: []string{"trend b"},
		},
	})
	if err != nil {
		t.Fatalf("CreateExperimentReport: %v", err)
	}
	if saved.Name != "neon-compare" {
		t.Fatalf("saved.Name = %q, want neon-compare", saved.Name)
	}
	if saved.CreatedAt.IsZero() {
		t.Fatalf("saved.CreatedAt = zero")
	}
	if saved.CurrentCheckpoint != "neon-compare-current" || saved.CompareCheckpoint != "neon-compare-compare" {
		t.Fatalf("saved checkpoints = %q / %q, want persisted checkpoint anchors", saved.CurrentCheckpoint, saved.CompareCheckpoint)
	}

	reports, err := engine.ListExperimentReports()
	if err != nil {
		t.Fatalf("ListExperimentReports: %v", err)
	}
	if len(reports) != 1 || reports[0].Name != "neon-compare" {
		t.Fatalf("reports = %#v, want neon-compare", reports)
	}
	if reports[0].Compare == nil || reports[0].Compare.InstanceID != "alt-guard" {
		t.Fatalf("compare snapshot = %#v, want alt-guard", reports[0].Compare)
	}
	if reports[0].Current.DirectorPlan == nil || len(reports[0].Current.DirectorPlan.Selected) != 1 {
		t.Fatalf("current director plan = %#v, want persisted director evidence", reports[0].Current.DirectorPlan)
	}
	if reports[0].Current.LatestTrace == nil || reports[0].Current.LatestTrace.Turn != 12 {
		t.Fatalf("current latest trace = %#v, want persisted trace evidence", reports[0].Current.LatestTrace)
	}
	if reports[0].CurrentCheckpoint != "neon-compare-current" || reports[0].CompareCheckpoint != "neon-compare-compare" {
		t.Fatalf("listed checkpoints = %q / %q, want round-tripped checkpoint anchors", reports[0].CurrentCheckpoint, reports[0].CompareCheckpoint)
	}
}

func TestExperimentReportNormalizesFocusCompatibility(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	engine.ConfigurePersistence(t.TempDir(), map[string]string{}, map[string]string{})

	saved, err := engine.CreateExperimentReport(core.ExperimentReport{
		Name:             "compat-report",
		SourceInstanceID: "default",
		Current: core.ExperimentSnapshot{
			InstanceID: "default",
			LatestTrace: &core.TurnTrace{
				Turn:      3,
				Character: "LegacyTraceFocus",
				UserInput: "继续观察",
			},
		},
		Compare: &core.ExperimentSnapshot{
			InstanceID: "alt",
			LatestTrace: &core.TurnTrace{
				Turn:           2,
				FocusCharacter: "CompareFocus",
				Character:      "LegacyCompare",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateExperimentReport: %v", err)
	}
	if saved.Current.FocusCharacter != "LegacyTraceFocus" {
		t.Fatalf("saved.Current.FocusCharacter = %q, want LegacyTraceFocus", saved.Current.FocusCharacter)
	}
	if saved.Current.LatestTrace == nil || saved.Current.LatestTrace.FocusCharacter != "LegacyTraceFocus" {
		t.Fatalf("saved.Current.LatestTrace = %#v, want normalized focus", saved.Current.LatestTrace)
	}
	if saved.Compare == nil || saved.Compare.LatestTrace == nil || saved.Compare.LatestTrace.FocusCharacter != "CompareFocus" {
		t.Fatalf("saved.Compare.LatestTrace = %#v, want CompareFocus to win", saved.Compare)
	}

	reports, err := engine.ListExperimentReports()
	if err != nil {
		t.Fatalf("ListExperimentReports: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("len(reports) = %d, want 1", len(reports))
	}
	if reports[0].Current.FocusCharacter != "LegacyTraceFocus" {
		t.Fatalf("reports[0].Current.FocusCharacter = %q, want LegacyTraceFocus", reports[0].Current.FocusCharacter)
	}
	if reports[0].Compare == nil || reports[0].Compare.LatestTrace == nil || reports[0].Compare.LatestTrace.FocusCharacter != "CompareFocus" {
		t.Fatalf("reports[0].Compare = %#v, want normalized compare trace focus", reports[0].Compare)
	}
}

func TestListTurnTracesNewestFirst(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)

	engine.mu.Lock()
	engine.recordTraceLocked(core.TurnTrace{Turn: 1, FocusCharacter: "111", UserInput: "first"})
	engine.recordTraceLocked(core.TurnTrace{Turn: 2, FocusCharacter: "111", UserInput: "second"})
	engine.recordTraceLocked(core.TurnTrace{Turn: 3, FocusCharacter: "安雅", UserInput: "third"})
	engine.mu.Unlock()

	traces := engine.ListTurnTraces(2)
	if len(traces) != 2 {
		t.Fatalf("len(traces) = %d, want 2", len(traces))
	}
	if traces[0].Turn != 3 || traces[1].Turn != 2 {
		t.Fatalf("turns = [%d %d], want [3 2]", traces[0].Turn, traces[1].Turn)
	}
	if traces[0].FocusCharacter != "安雅" || traces[0].Character != "" {
		t.Fatalf("trace payload = %#v, want focus only and empty legacy character mirror", traces[0])
	}
}

func TestLoadSaveSlotDoesNotResetTurnCounter(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	engine.ConfigurePersistence(t.TempDir(), map[string]string{}, map[string]string{})
	engine.SyncActiveWorldContext()

	engine.mu.Lock()
	engine.recordTraceLocked(core.TurnTrace{Turn: 1, Character: "111", UserInput: "first"})
	engine.recordTraceLocked(core.TurnTrace{Turn: 2, Character: "111", UserInput: "second"})
	engine.mu.Unlock()

	if _, err := engine.CreateSaveSlot("cp-turns", "main", "keep turn count"); err != nil {
		t.Fatalf("CreateSaveSlot: %v", err)
	}
	if _, err := engine.LoadSaveSlot("cp-turns"); err != nil {
		t.Fatalf("LoadSaveSlot: %v", err)
	}
	if engine.turnCount != 2 {
		t.Fatalf("turnCount after load = %d, want 2", engine.turnCount)
	}

	engine.mu.Lock()
	engine.turnCount++
	next := engine.turnCount
	engine.mu.Unlock()
	if next != 3 {
		t.Fatalf("next turn = %d, want 3", next)
	}
}

func TestResetDialogueDoesNotResetTurnCounter(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)

	engine.mu.Lock()
	engine.recordTraceLocked(core.TurnTrace{Turn: 4, Character: "111", UserInput: "fourth"})
	engine.mu.Unlock()

	engine.ResetDialogue()
	if engine.turnCount != 4 {
		t.Fatalf("turnCount after reset = %d, want 4", engine.turnCount)
	}
}

func TestSwitchCharacterDoesNotResetTurnCounter(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)

	engine.mu.Lock()
	engine.recordTraceLocked(core.TurnTrace{Turn: 5, Character: "111", UserInput: "fifth"})
	engine.mu.Unlock()

	if err := engine.SwitchFocusCharacter("安雅"); err != nil {
		t.Fatalf("SwitchCharacter: %v", err)
	}
	if engine.turnCount != 5 {
		t.Fatalf("turnCount after switch = %d, want 5", engine.turnCount)
	}
}

func TestSwitchCharacterKeepsPersonaSceneAndDialogueAligned(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	engine.SyncActiveWorldContext()

	char, ok := engine.GetCharacter()
	if !ok || char.Identity.Name != "111" {
		t.Fatalf("initial active character = %#v, want 111", char.Identity)
	}
	if got := engine.GetState().Scene.Characters; !containsString(got, "111") {
		t.Fatalf("initial scene characters = %#v, want 111 present", got)
	}
	if msgs := engine.GetDialogueLimit(10); len(msgs) == 0 || msgs[0].Content != "111 hello" {
		t.Fatalf("initial dialogue = %#v, want 111 dialogue", msgs)
	}

	if err := engine.SwitchFocusCharacter("安雅"); err != nil {
		t.Fatalf("switch character: %v", err)
	}

	char, ok = engine.GetCharacter()
	if !ok || char.Identity.Name != "安雅" {
		t.Fatalf("switched active character = %#v, want 安雅", char.Identity)
	}
	if got := engine.GetState().Scene.Characters; !containsString(got, "安雅") {
		t.Fatalf("switched scene characters = %#v, want 安雅 present", got)
	}
	if msgs := engine.GetDialogueLimit(10); len(msgs) == 0 || msgs[0].Content != "anya hello" {
		t.Fatalf("switched dialogue = %#v, want 安雅 dialogue", msgs)
	}
}

func TestSwitchCharacterRoundTripKeepsPerCharacterWorldSceneDialogueAligned(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	world111 := filepath.Join(root, "world-111")
	worldAnya := filepath.Join(root, "world-anya")
	writeTestWorldBundle(t, world111, "夜之城 2077", "chrome first", core.SceneState{
		Location:    "废弃地铁站",
		TimeOfDay:   "凌晨 3 点",
		Weather:     "酸雨",
		Characters:  []string{"111", "用户"},
		Description: "111 scene",
	})
	writeTestWorldBundle(t, worldAnya, "安全屋", "stay hidden", core.SceneState{
		Location:    "地下安全屋",
		TimeOfDay:   "深夜",
		Weather:     "无风",
		Characters:  []string{"安雅", "用户"},
		Description: "Anya scene",
	})

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": world111,
		"安雅":  worldAnya,
	})
	engine.worldPaths["111"] = world111
	engine.worldPaths["安雅"] = worldAnya
	engine.charWorlds["111"] = CharWorld{
		WorldName: "夜之城 2077",
		CoreRules: "chrome first",
		Scene: core.SceneState{
			Location:    "废弃地铁站",
			TimeOfDay:   "凌晨 3 点",
			Weather:     "酸雨",
			Characters:  []string{"111", "用户"},
			Description: "111 scene",
		},
	}
	engine.charWorlds["安雅"] = CharWorld{
		WorldName: "安全屋",
		CoreRules: "stay hidden",
		Scene: core.SceneState{
			Location:    "地下安全屋",
			TimeOfDay:   "深夜",
			Weather:     "无风",
			Characters:  []string{"安雅", "用户"},
			Description: "Anya scene",
		},
	}
	engine.SyncActiveWorldContext()

	role, err := engine.UpdatePlayerRole(core.PlayerRole{Name: "贾宝玉", BoundCharacter: "贾宝玉"})
	if err != nil {
		t.Fatalf("UpdatePlayerRole: %v", err)
	}
	if role.Name != "贾宝玉" {
		t.Fatalf("role.Name = %q, want 贾宝玉", role.Name)
	}

	assertActive := func(wantChar, wantWorld, wantLocation, wantDialogueFirst string) {
		t.Helper()

		char, ok := engine.GetCharacter()
		if !ok || char.Identity.Name != wantChar {
			t.Fatalf("active character = %#v, want %s", char.Identity, wantChar)
		}
		if got := engine.GetWorldName(); got != wantWorld {
			t.Fatalf("world = %q, want %q", got, wantWorld)
		}
		scene := engine.GetState().Scene
		if scene.Location != wantLocation {
			t.Fatalf("scene.Location = %q, want %q", scene.Location, wantLocation)
		}
		if !containsString(scene.Characters, wantChar) || !containsString(scene.Characters, "贾宝玉") {
			t.Fatalf("scene.Characters = %#v, want %s and 贾宝玉 present", scene.Characters, wantChar)
		}
		msgs := engine.GetDialogueLimit(10)
		if len(msgs) == 0 || msgs[0].Content != wantDialogueFirst {
			t.Fatalf("dialogue = %#v, want first content %q", msgs, wantDialogueFirst)
		}
	}

	assertActive("111", "夜之城 2077", "废弃地铁站", "111 hello")

	if err := engine.SwitchFocusCharacter("安雅"); err != nil {
		t.Fatalf("switch to 安雅: %v", err)
	}
	assertActive("安雅", "安全屋", "地下安全屋", "anya hello")

	if _, err := engine.UpdateWorldConfig(core.WorldConfig{Name: "安雅新据点", CoreRules: "trust no one"}); err != nil {
		t.Fatalf("UpdateWorldConfig 安雅: %v", err)
	}
	if _, err := engine.UpdateSceneConfig(core.SceneConfig{
		Name: "default",
		Scene: core.SceneState{
			Location:    "屋顶观察哨",
			TimeOfDay:   "黎明前",
			Weather:     "薄雾",
			Characters:  []string{"安雅", "用户"},
			Description: "updated anya scene",
		},
	}); err != nil {
		t.Fatalf("UpdateSceneConfig 安雅: %v", err)
	}
	assertActive("安雅", "安雅新据点", "屋顶观察哨", "anya hello")

	if err := engine.SwitchFocusCharacter("111"); err != nil {
		t.Fatalf("switch back to 111: %v", err)
	}
	assertActive("111", "夜之城 2077", "废弃地铁站", "111 hello")

	if err := engine.SwitchFocusCharacter("安雅"); err != nil {
		t.Fatalf("switch again to 安雅: %v", err)
	}
	assertActive("安雅", "安雅新据点", "屋顶观察哨", "anya hello")
}

func TestHTTPCharacterConfigAndMemoryStayAlignedPerCharacter(t *testing.T) {
	auth.Init("")
	auth.SetSecureCookie(false)

	engine := newMultiCharacterTestEngine(t)
	charPath := filepath.Join(t.TempDir(), "anya.yml")
	if err := os.WriteFile(charPath, []byte(`identity:
  name: "安雅"
  immutable: ["alert"]
  adaptive: {trust: 3}
  forbidden: ["info_dump"]
  voice: {style: "brief", rhythm: "short"}
  writing_guide: "old"
goals:
  primary:
    - id: survive
      priority: 10
      condition: "always"
`), 0644); err != nil {
		t.Fatalf("write char file: %v", err)
	}
	engine.ConfigurePersistence(t.TempDir(), map[string]string{"安雅": charPath}, map[string]string{"安雅": "worlds/anya_world.yml"})
	engine.SyncActiveWorldContext()

	server := api.NewServer(engine)
	mux := http.NewServeMux()
	server.Register(mux)

	reqCfg := httptest.NewRequest(http.MethodPost, "/api/focus-definition-config", bytes.NewBufferString(`{"focus_character":"安雅","card":{"identity":{"name":"安雅","immutable":["guarded"],"adaptive":{"trust":4},"forbidden":["info_dump"],"voice":{"style":"spare","rhythm":"short"},"writing_guide":"updated"},"goals":[{"id":"survive","priority":10,"type":"primary","condition":"always"}]}}`))
	reqCfg.Header.Set("Content-Type", "application/json")
	recCfg := httptest.NewRecorder()
	mux.ServeHTTP(recCfg, reqCfg)
	if recCfg.Code != http.StatusOK {
		t.Fatalf("POST /api/focus-definition-config = %d", recCfg.Code)
	}

	switchReq := httptest.NewRequest(http.MethodPost, "/api/switch", bytes.NewBufferString(`{"focus_character":"安雅"}`))
	switchReq.Header.Set("Content-Type", "application/json")
	switchRec := httptest.NewRecorder()
	mux.ServeHTTP(switchRec, switchReq)
	if switchRec.Code != http.StatusOK {
		t.Fatalf("POST /api/switch = %d", switchRec.Code)
	}

	memoryReq := httptest.NewRequest(http.MethodGet, "/api/memory?dialogue=10", nil)
	memoryRec := httptest.NewRecorder()
	mux.ServeHTTP(memoryRec, memoryReq)
	if memoryRec.Code != http.StatusOK {
		t.Fatalf("GET /api/memory = %d", memoryRec.Code)
	}
	var memoryResp struct {
		FocusCharacter string         `json:"focus_character"`
		Dialogue       []core.Message `json:"dialogue"`
	}
	if err := json.NewDecoder(memoryRec.Body).Decode(&memoryResp); err != nil {
		t.Fatalf("decode memory: %v", err)
	}
	if memoryResp.FocusCharacter != "安雅" {
		t.Fatalf("memory focus_character = %q, want 安雅", memoryResp.FocusCharacter)
	}
	if len(memoryResp.Dialogue) == 0 || memoryResp.Dialogue[0].Content != "anya hello" {
		t.Fatalf("memory dialogue = %#v, want 安雅 dialogue", memoryResp.Dialogue)
	}

	charReq := httptest.NewRequest(http.MethodGet, "/api/focus-definition", nil)
	charRec := httptest.NewRecorder()
	mux.ServeHTTP(charRec, charReq)
	if charRec.Code != http.StatusOK {
		t.Fatalf("GET /api/focus-definition = %d", charRec.Code)
	}
	var charResp core.Character
	if err := json.NewDecoder(charRec.Body).Decode(&charResp); err != nil {
		t.Fatalf("decode character: %v", err)
	}
	if charResp.Identity.WritingGuide != "updated" {
		t.Fatalf("character writing guide = %q, want updated", charResp.Identity.WritingGuide)
	}

	stateReq := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	stateRec := httptest.NewRecorder()
	mux.ServeHTTP(stateRec, stateReq)
	if stateRec.Code != http.StatusOK {
		t.Fatalf("GET /api/state = %d", stateRec.Code)
	}
	var stateResp core.WorldState
	if err := json.NewDecoder(stateRec.Body).Decode(&stateResp); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if !containsString(stateResp.Scene.Characters, "安雅") {
		t.Fatalf("state scene characters = %#v, want 安雅 present", stateResp.Scene.Characters)
	}
}

func TestActionLoggerPersistenceReloadsPerInstance(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "runtime.db")

	defaultEngine := newRuntimeEngineOnDB(t, dbPath)
	defaultEngine.SetInstanceMetadata("default", time.Now().UTC())
	defaultEngine.actionLogger.Record(emotion.ActionLogEntry{
		Tick:       1,
		Character:  "111",
		Fired:      true,
		ActionType: "approach",
	})

	altEngine := newRuntimeEngineOnDB(t, dbPath)
	altEngine.SetInstanceMetadata("alt", time.Now().UTC())
	altEngine.actionLogger.Record(emotion.ActionLogEntry{
		Tick:      2,
		Character: "111",
		Fired:     false,
		BlockedBy: "cooldown",
	})

	reloadedDefault := newRuntimeEngineOnDB(t, dbPath)
	reloadedDefault.SetInstanceMetadata("default", time.Now().UTC())
	defaultLogs := reloadedDefault.QueryActionLog("111", false, false, 10)
	if len(defaultLogs) != 1 {
		t.Fatalf("default logs = %#v, want 1 entry", defaultLogs)
	}
	defaultEntry, ok := defaultLogs[0].(emotion.ActionLogEntry)
	if !ok {
		t.Fatalf("default log type = %T, want ActionLogEntry", defaultLogs[0])
	}
	if defaultEntry.Tick != 1 || !defaultEntry.Fired {
		t.Fatalf("default entry = %#v, want fired tick 1", defaultEntry)
	}

	reloadedAlt := newRuntimeEngineOnDB(t, dbPath)
	reloadedAlt.SetInstanceMetadata("alt", time.Now().UTC())
	altLogs := reloadedAlt.QueryActionLog("111", false, false, 10)
	if len(altLogs) != 1 {
		t.Fatalf("alt logs = %#v, want 1 entry", altLogs)
	}
	altEntry, ok := altLogs[0].(emotion.ActionLogEntry)
	if !ok {
		t.Fatalf("alt log type = %T, want ActionLogEntry", altLogs[0])
	}
	if altEntry.Tick != 2 || altEntry.BlockedBy != "cooldown" {
		t.Fatalf("alt entry = %#v, want blocked tick 2", altEntry)
	}
}

func TestTickLoopSurvivesScaledSeventyTicks(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	engine.SetInstanceMetadata("default", time.Now().UTC())
	engine.SyncActiveWorldContext()

	loop := simulation.NewLoop(10 * time.Millisecond)
	engine.tickLoop = loop
	loop.OnTick(engine.onTick)
	loop.Start()
	defer engine.Stop()

	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if int(engine.tickCount.Load()) >= 70 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if int(engine.tickCount.Load()) < 70 {
		t.Fatalf("engine tickCount = %d, want >= 70", int(engine.tickCount.Load()))
	}
	if loop.TickCount() < 70 {
		t.Fatalf("loop tick count = %d, want >= 70", loop.TickCount())
	}

	state := engine.GetState()
	if state.Clock.Minute == 0 && state.Clock.Hour == 0 && state.Clock.Day == 0 {
		t.Fatalf("clock did not advance: %#v", state.Clock)
	}

	events, err := engine.eventStore.GetCanonicalEvents()
	if err != nil {
		t.Fatalf("GetCanonicalEvents: %v", err)
	}
	if len(events) < 70 {
		t.Fatalf("canonical events = %d, want at least 70 clock events", len(events))
	}

	logs := engine.QueryActionLog("", false, false, 20)
	stats := engine.ActionLogStats()
	if stats["total_entries"] == nil {
		t.Fatalf("action log stats missing total_entries: %#v", stats)
	}
	if logs == nil && stats["total_entries"].(int) < 0 {
		t.Fatalf("unexpected action log state: logs=%#v stats=%#v", logs, stats)
	}
}

func TestOnTickInjectsWorldPressureEvents(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "pressure-world")
	writeTestWorldBundle(t, worldDir, "压力世界", "世界会自己演化", core.SceneState{
		Location:    "外城",
		TimeOfDay:   "深夜",
		Weather:     "阴",
		Characters:  []string{"111", "玩家"},
		Description: "外城正在收紧控制",
	})

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.worldPaths["111"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "压力世界",
		CoreRules: "世界会自己演化",
		Scene: core.SceneState{
			Location:    "外城",
			TimeOfDay:   "深夜",
			Weather:     "阴",
			Characters:  []string{"111", "玩家"},
			Description: "外城正在收紧控制",
		},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()

	if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
		Pressures: []core.WorldPressureConfig{{
			ID:          "curfew",
			Name:        "宵禁升级",
			Kind:        "security",
			Description: "夜间搜查正在扩大",
			Intensity:   0.9,
			Target:      "外城",
			Escalates:   []string{"盘查", "抓捕"},
		}},
	}); err != nil {
		t.Fatalf("UpdateWorldStructureConfig: %v", err)
	}

	engine.onTick()

	state := engine.GetState()
	if state.Tension <= 0 {
		t.Fatalf("state tension = %.2f, want positive after pulse", state.Tension)
	}
	status := engine.TickStatus()
	summary, ok := status["last_tick_summary"].([]string)
	if !ok || len(summary) == 0 {
		t.Fatalf("tick summary missing: %#v", status)
	}
	foundDelta := false
	for _, line := range summary {
		if strings.Contains(line, "tension") || strings.Contains(line, "pressure delta") {
			foundDelta = true
			break
		}
	}
	if !foundDelta {
		t.Fatalf("tick summary missing state delta: %#v", summary)
	}
	if got := state.Variables["world.pressure.curfew.last_tick"]; got == nil {
		t.Fatalf("pressure variable missing, state = %#v", state.Variables)
	}

	events, err := engine.eventStore.GetCanonicalEvents()
	if err != nil {
		t.Fatalf("GetCanonicalEvents: %v", err)
	}
	foundPressure := false
	for _, evt := range events {
		if evt.Type == "world_pressure" && evt.Payload["pressure_id"] == "curfew" {
			foundPressure = true
			break
		}
	}
	if !foundPressure {
		t.Fatalf("canonical events missing world_pressure: %#v", events)
	}
}

func TestTickStatusIncludesStructureAuthoringDiagnostics(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "diagnostic-world")
	writeTestWorldBundle(t, worldDir, "诊断世界", "作者需要看到结构影响", core.SceneState{
		Location:    "外城",
		TimeOfDay:   "深夜",
		Weather:     "阴",
		Characters:  []string{"111", "玩家"},
		Description: "外城正在收紧控制",
	})

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.worldPaths["111"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "诊断世界",
		CoreRules: "作者需要看到结构影响",
		Scene: core.SceneState{
			Location:    "外城",
			TimeOfDay:   "深夜",
			Weather:     "阴",
			Characters:  []string{"111", "玩家"},
			Description: "外城正在收紧控制",
		},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()

	if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
		Locations: []core.WorldLocationConfig{{
			ID:          "outer_city",
			Name:        "外城",
			Kind:        "district",
			Description: "夜间戒备森严",
			Controller:  "guard",
		}},
		Pressures: []core.WorldPressureConfig{{
			ID:          "curfew",
			Name:        "宵禁升级",
			Kind:        "security",
			Description: "夜间搜查正在扩大",
			Intensity:   0.8,
			Target:      "guard",
		}},
	}); err != nil {
		t.Fatalf("UpdateWorldStructureConfig: %v", err)
	}
	if _, err := engine.UpdatePopulationConfig(core.PopulationConfig{
		BackgroundNPCs: []core.BackgroundNPC{{
			ID:       "watcher",
			Name:     "巡夜人",
			Role:     "巡逻",
			Location: "外城",
			Faction:  "guard",
			Traits:   []string{"警觉"},
			Hooks:    []string{"宵禁", "搜查"},
		}},
	}); err != nil {
		t.Fatalf("UpdatePopulationConfig: %v", err)
	}

	status := engine.TickStatus()
	diagnostics, ok := status["diagnostics"].([]map[string]interface{})
	if !ok || len(diagnostics) == 0 {
		t.Fatalf("diagnostics missing: %#v", status["diagnostics"])
	}

	var foundSceneControl bool
	var foundActivePressure bool
	var foundPopulationCandidates bool
	for _, item := range diagnostics {
		metric, _ := item["metric"].(string)
		message, _ := item["message"].(string)
		switch metric {
		case "scene_control":
			foundSceneControl = strings.Contains(message, "控制区")
		case "active_pressure":
			foundActivePressure = strings.Contains(message, "pressure '宵禁升级'")
		case "scene_population_candidates":
			foundPopulationCandidates = strings.Contains(message, "巡夜人")
		}
	}
	if !foundSceneControl || !foundActivePressure || !foundPopulationCandidates {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
}

func TestWorldStructureInterventionChangesRuntimeOutputs(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "structure-compare-world")
	writeTestWorldBundle(t, worldDir, "结构对照世界", "作者干预应真实改变 runtime", core.SceneState{
		Location:    "外城",
		TimeOfDay:   "深夜",
		Weather:     "阴",
		Characters:  []string{"111", "玩家"},
		Description: "外城夜色紧绷",
	})

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.worldPaths["111"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "结构对照世界",
		CoreRules: "作者干预应真实改变 runtime",
		Scene: core.SceneState{
			Location:    "外城",
			TimeOfDay:   "深夜",
			Weather:     "阴",
			Characters:  []string{"111", "玩家"},
			Description: "外城夜色紧绷",
		},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()

	if _, err := engine.UpdatePopulationConfig(core.PopulationConfig{
		BackgroundNPCs: []core.BackgroundNPC{{
			ID:       "watcher",
			Name:     "巡夜人",
			Role:     "巡逻",
			Location: "岗亭",
			Faction:  "guard",
			Traits:   []string{"警觉"},
			Hooks:    []string{"搜查", "宵禁"},
		}},
	}); err != nil {
		t.Fatalf("UpdatePopulationConfig: %v", err)
	}

	if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
		Locations: []core.WorldLocationConfig{{
			ID:          "outer_city",
			Name:        "外城",
			Kind:        "district",
			Description: "尚未明确归属",
			Controller:  "",
		}},
	}); err != nil {
		t.Fatalf("UpdateWorldStructureConfig baseline: %v", err)
	}

	engine.onTick()
	baselineStatus := engine.TickStatus()
	engine.mu.Lock()
	baselinePlan := engine.directTurnLocked("现在情况如何？", engine.stateMgr.Get())
	engine.mu.Unlock()

	if containsString(baselinePlan.Candidates, "巡夜人") {
		t.Fatalf("baseline candidates = %#v, want no structure-driven 巡夜人", baselinePlan.Candidates)
	}
	if len(baselinePlan.WorldSignals) == 0 || baselinePlan.WorldSignals[0] != "当前主要由用户输入与在场状态驱动" {
		t.Fatalf("baseline world signals = %#v", baselinePlan.WorldSignals)
	}
	if got := baselineStatus["tension"].(float64); got != 0 {
		t.Fatalf("baseline tension = %.2f, want 0 without active structure pressure", got)
	}
	baselineDiagnostics, _ := baselineStatus["diagnostics"].([]map[string]interface{})
	for _, item := range baselineDiagnostics {
		if metric, _ := item["metric"].(string); metric == "scene_population_candidates" || metric == "active_pressure" || metric == "scene_control" {
			t.Fatalf("baseline diagnostics unexpectedly structure-driven: %#v", baselineDiagnostics)
		}
	}

	if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
		Factions: []core.WorldFactionConfig{{
			ID:            "guard",
			Name:          "巡城司",
			Role:          "law",
			Description:   "负责外城宵禁",
			Relationships: []string{"敌对 smugglers"},
		}, {
			ID:            "smugglers",
			Name:          "走私帮",
			Role:          "criminal",
			Description:   "夜里活动频繁",
			Relationships: []string{"敌对 guard"},
		}},
		Locations: []core.WorldLocationConfig{{
			ID:          "outer_city",
			Name:        "外城",
			Kind:        "district",
			Description: "巡城司控制区",
			Controller:  "guard",
		}},
		Pressures: []core.WorldPressureConfig{{
			ID:          "curfew",
			Name:        "宵禁升级",
			Kind:        "conflict",
			Description: "外城正在扩大盘查",
			Intensity:   0.9,
			Target:      "guard",
			Escalates:   []string{"checkpoint"},
		}},
	}); err != nil {
		t.Fatalf("UpdateWorldStructureConfig intervention: %v", err)
	}

	engine.onTick()
	intervenedStatus := engine.TickStatus()
	engine.mu.Lock()
	intervenedPlan := engine.directTurnLocked("现在情况如何？", engine.stateMgr.Get())
	engine.mu.Unlock()

	if !containsString(intervenedPlan.Candidates, "巡夜人") {
		t.Fatalf("intervened candidates = %#v, want 巡夜人 after structure intervention", intervenedPlan.Candidates)
	}
	foundSceneSignal := false
	for _, signal := range intervenedPlan.WorldSignals {
		if strings.Contains(signal, "当前 scene 位置相关") || strings.Contains(signal, "命中当前 pressure") || strings.Contains(signal, "命中当前 faction") {
			foundSceneSignal = true
			break
		}
	}
	if !foundSceneSignal {
		t.Fatalf("intervened world signals = %#v, want structure-driven signals", intervenedPlan.WorldSignals)
	}
	if got := intervenedStatus["tension"].(float64); got <= 0 {
		t.Fatalf("intervened tension = %.2f, want positive after structure pressure", got)
	}
	pressureStates, _ := intervenedStatus["pressure_states"].(map[string]float64)
	if pressureStates["curfew"] <= 0 {
		t.Fatalf("pressure states = %#v, want curfew intensity tracked", pressureStates)
	}
	factionTensions, _ := intervenedStatus["faction_tensions"].(map[string]float64)
	if factionTensions["guard"] <= 0 {
		t.Fatalf("faction tensions = %#v, want guard tension after structure pressure", factionTensions)
	}
	intervenedDiagnostics, _ := intervenedStatus["diagnostics"].([]map[string]interface{})
	foundSceneControl := false
	foundPressure := false
	foundPopulation := false
	for _, item := range intervenedDiagnostics {
		metric, _ := item["metric"].(string)
		message, _ := item["message"].(string)
		switch metric {
		case "scene_control":
			foundSceneControl = strings.Contains(message, "控制区")
		case "active_pressure":
			foundPressure = strings.Contains(message, "宵禁升级")
		case "scene_population_candidates":
			foundPopulation = strings.Contains(message, "巡夜人")
		}
	}
	if !foundSceneControl || !foundPressure || !foundPopulation {
		t.Fatalf("intervened diagnostics = %#v, want structure-driven authoring evidence", intervenedDiagnostics)
	}
}

func TestStructureDrivenSimulationSustainsEvolutionAcrossTicks(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "sustained-structure-world")
	writeTestWorldBundle(t, worldDir, "持续演化世界", "世界应在多 tick 下持续演化", core.SceneState{
		Location:    "外城",
		TimeOfDay:   "深夜",
		Weather:     "阴",
		Characters:  []string{"111", "玩家"},
		Description: "夜里的外城持续承压",
	})

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.worldPaths["111"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "持续演化世界",
		CoreRules: "世界应在多 tick 下持续演化",
		Scene: core.SceneState{
			Location:    "外城",
			TimeOfDay:   "深夜",
			Weather:     "阴",
			Characters:  []string{"111", "玩家"},
			Description: "夜里的外城持续承压",
		},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()

	if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
		Factions: []core.WorldFactionConfig{{
			ID:            "guard",
			Name:          "巡城司",
			Role:          "law",
			Description:   "负责外城宵禁",
			Relationships: []string{"敌对 smugglers"},
		}},
		Locations: []core.WorldLocationConfig{{
			ID:          "outer_city",
			Name:        "外城",
			Kind:        "district",
			Description: "巡城司控制区",
			Controller:  "guard",
		}},
		Pressures: []core.WorldPressureConfig{{
			ID:          "curfew",
			Name:        "宵禁升级",
			Kind:        "conflict",
			Description: "夜间搜查持续扩大",
			Intensity:   0.9,
			Target:      "guard",
		}},
	}); err != nil {
		t.Fatalf("UpdateWorldStructureConfig: %v", err)
	}

	tensionHistory := make([]float64, 0, 8)
	factionHistory := make([]float64, 0, 8)
	pressureHistory := make([]float64, 0, 8)
	summaryCount := 0
	for i := 0; i < 8; i++ {
		engine.onTick()
		status := engine.TickStatus()
		tensionHistory = append(tensionHistory, status["tension"].(float64))
		factionTensions, _ := status["faction_tensions"].(map[string]float64)
		factionHistory = append(factionHistory, factionTensions["guard"])
		pressureStates, _ := status["pressure_states"].(map[string]float64)
		pressureHistory = append(pressureHistory, pressureStates["curfew"])
		if summary, _ := status["last_tick_summary"].([]string); len(summary) > 0 {
			summaryCount++
		}
	}

	if summaryCount < 8 {
		t.Fatalf("last tick summaries observed = %d, want summary every tick", summaryCount)
	}
	distinctTensions := map[string]bool{}
	for _, v := range tensionHistory {
		distinctTensions[fmt.Sprintf("%.2f", v)] = true
	}
	if len(distinctTensions) < 2 {
		t.Fatalf("tension history = %#v, want multiple distinct states", tensionHistory)
	}
	if factionHistory[len(factionHistory)-1] <= factionHistory[0] {
		t.Fatalf("faction history = %#v, want sustained growth from structure pressure", factionHistory)
	}
	if pressureHistory[len(pressureHistory)-1] <= pressureHistory[0] {
		t.Fatalf("pressure history = %#v, want dynamic pressure state evolution", pressureHistory)
	}
	status := engine.TickStatus()
	history, ok := status["tick_history"].([]core.TickSnapshot)
	if !ok || len(history) != 8 {
		t.Fatalf("tick history = %#v, want 8 recent snapshots", status["tick_history"])
	}
	trajectorySummary, ok := status["trajectory_summary"].([]string)
	if !ok || len(trajectorySummary) == 0 {
		t.Fatalf("trajectory summary = %#v, want author-facing long-window summary", status["trajectory_summary"])
	}
	if !strings.Contains(strings.Join(trajectorySummary, " | "), "tension trend:") {
		t.Fatalf("trajectory summary = %#v, want tension trend line", trajectorySummary)
	}
	if history[0].Tick <= 0 || history[len(history)-1].Tick <= history[0].Tick {
		t.Fatalf("tick history ticks = %#v, want increasing tick snapshots", history)
	}
	if len(history[len(history)-1].Summary) == 0 {
		t.Fatalf("tick history latest summary = %#v, want latest snapshot summary", history[len(history)-1])
	}
	state := engine.GetState()
	if state.Variables["world.pressure.curfew.last_tick"] == nil {
		t.Fatalf("state variables = %#v, want pressure trace persisted", state.Variables)
	}

	events, err := engine.eventStore.GetCanonicalEvents()
	if err != nil {
		t.Fatalf("GetCanonicalEvents: %v", err)
	}
	worldPressureCount := 0
	for _, evt := range events {
		if evt.Type == "world_pressure" && evt.Payload["pressure_id"] == "curfew" {
			worldPressureCount++
		}
	}
	if worldPressureCount < 3 {
		t.Fatalf("world pressure events = %d, want repeated structure-driven evolution", worldPressureCount)
	}
}

func TestAutonomousSimulationPromotesScenePopulationAcrossLongWindow(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "autonomous-growth-world")
	writeTestWorldBundle(t, worldDir, "自治成长世界", "无人输入时人口也应自然进场并成长", core.SceneState{
		Location:    "外城",
		TimeOfDay:   "深夜",
		Weather:     "阴",
		Characters:  []string{"111", "玩家"},
		Description: "外城在宵禁与走私冲突中持续承压",
	})

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.worldPaths["111"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "自治成长世界",
		CoreRules: "无人输入时人口也应自然进场并成长",
		Scene: core.SceneState{
			Location:    "外城",
			TimeOfDay:   "深夜",
			Weather:     "阴",
			Characters:  []string{"111", "玩家"},
			Description: "外城在宵禁与走私冲突中持续承压",
		},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()

	if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
		Factions: []core.WorldFactionConfig{
			{
				ID:            "guard",
				Name:          "巡城司",
				Role:          "law",
				Description:   "负责宵禁和盘查",
				Relationships: []string{"敌对 smugglers"},
			},
			{
				ID:            "smugglers",
				Name:          "走私帮",
				Role:          "criminal",
				Description:   "夜里持续活动",
				Relationships: []string{"敌对 guard"},
			},
		},
		Locations: []core.WorldLocationConfig{{
			ID:          "outer_city",
			Name:        "外城",
			Kind:        "district",
			Description: "巡城司控制区",
			Controller:  "guard",
		}},
		Pressures: []core.WorldPressureConfig{{
			ID:          "curfew",
			Name:        "宵禁升级",
			Kind:        "conflict",
			Description: "外城盘查与走私冲突持续加剧",
			Intensity:   0.9,
			Target:      "guard",
		}},
	}); err != nil {
		t.Fatalf("UpdateWorldStructureConfig: %v", err)
	}

	if _, err := engine.UpdatePopulationConfig(core.PopulationConfig{
		BackgroundNPCs: []core.BackgroundNPC{
			{
				ID:       "watcher",
				Name:     "巡夜人",
				Role:     "guard",
				Location: "外城",
				Faction:  "guard",
				Traits:   []string{"警觉", "克制"},
				Hooks:    []string{"宵禁", "盘查"},
			},
			{
				ID:       "runner",
				Name:     "线人",
				Role:     "informant",
				Location: "外城",
				Faction:  "smugglers",
				Traits:   []string{"灵活", "谨慎"},
				Hooks:    []string{"走私", "风声"},
			},
		},
		Policy: core.PromotionPolicy{
			PromoteThreshold:   3.5,
			MajorThreshold:     8,
			InteractionWeight:  3,
			MentionWeight:      1,
			EventWeight:        2,
			RelationshipWeight: 3,
			SceneWeight:        2,
		},
	}); err != nil {
		t.Fatalf("UpdatePopulationConfig: %v", err)
	}

	for i := 0; i < 36; i++ {
		engine.onTick()
	}

	state := engine.GetState()
	if !containsString(state.Scene.Characters, "巡夜人") {
		t.Fatalf("scene participants = %#v, want autonomous tick to pull 巡夜人 into scene", state.Scene.Characters)
	}
	if _, ok := engine.agents.GetCharacter("巡夜人"); !ok {
		t.Fatalf("巡夜人 should be loaded by autonomous tick runtime")
	}
	if !containsString(engine.GetLoadedCharacters(), "巡夜人") {
		t.Fatalf("loaded characters = %#v, want 巡夜人 loaded through autonomous tick", engine.GetLoadedCharacters())
	}
	if got := engine.npcTickExposure["巡夜人"]; got < 20 {
		t.Fatalf("npc exposure = %#v, want sustained exposure for 巡夜人", engine.npcTickExposure)
	}

	insights, err := engine.GetPopulationInsights()
	if err != nil {
		t.Fatalf("GetPopulationInsights: %v", err)
	}
	var promoted *core.PopulationCharacterInsight
	for i := range insights.Promoted {
		if insights.Promoted[i].Name == "巡夜人" {
			promoted = &insights.Promoted[i]
			break
		}
	}
	if promoted == nil {
		t.Fatalf("promoted insights = %#v, want 巡夜人 promoted without directTurn/director", insights.Promoted)
	}
	if promoted.Attention.Score < 3.5 {
		t.Fatalf("promoted attention = %#v, want score above promotion threshold", promoted.Attention)
	}
	if promoted.IdentityCore == "" {
		t.Fatalf("promoted insight = %#v, want identity core persisted", promoted)
	}
	historyTypes := make(map[string]bool)
	for _, evt := range promoted.History {
		historyTypes[evt.Type] = true
	}
	if !historyTypes["population_promoted"] {
		t.Fatalf("promotion history = %#v, want population_promoted event in insights history", promoted.History)
	}

	status := engine.TickStatus()
	history, ok := status["tick_history"].([]core.TickSnapshot)
	if !ok || len(history) != 12 {
		t.Fatalf("tick history = %#v, want capped recent snapshots after long-window run", status["tick_history"])
	}
	trajectorySummary, ok := status["trajectory_summary"].([]string)
	if !ok || len(trajectorySummary) == 0 {
		t.Fatalf("trajectory summary = %#v, want long-window trajectory summary", status["trajectory_summary"])
	}
	joinedTrajectory := strings.Join(trajectorySummary, " | ")
	if !strings.Contains(joinedTrajectory, "population outcome:") || !strings.Contains(joinedTrajectory, "recent diagnostics:") {
		t.Fatalf("trajectory summary = %#v, want population and diagnostics summary lines", trajectorySummary)
	}
	diagnosticSnapshots := 0
	sceneCandidateSnapshots := 0
	for _, snapshot := range history {
		if len(snapshot.Diagnostics) > 0 {
			diagnosticSnapshots++
		}
		for _, item := range snapshot.Diagnostics {
			if metric, _ := item["metric"].(string); metric == "scene_population_candidates" {
				sceneCandidateSnapshots++
				break
			}
		}
	}
	if diagnosticSnapshots < 12 {
		t.Fatalf("tick history diagnostics = %#v, want diagnostics preserved across recent snapshots", history)
	}
	if sceneCandidateSnapshots == 0 {
		t.Fatalf("tick history diagnostics = %#v, want scene population candidate diagnostics in history", history)
	}
	highlights, _ := status["population_highlights"].([]string)
	if len(highlights) == 0 || !strings.Contains(strings.Join(highlights, " | "), "promoted:") {
		t.Fatalf("population highlights = %#v, want promoted summary after long-window growth", highlights)
	}

	events, err := engine.eventStore.GetCanonicalEvents()
	if err != nil {
		t.Fatalf("GetCanonicalEvents: %v", err)
	}
	promotedCount := 0
	worldPressureCount := 0
	for _, evt := range events {
		if evt.Type == "population_promoted" && evt.Target == "巡夜人" {
			promotedCount++
		}
		if evt.Type == "world_pressure" && evt.Payload["pressure_id"] == "curfew" {
			worldPressureCount++
		}
	}
	if promotedCount == 0 {
		t.Fatalf("canonical events = %#v, want promoted event for 巡夜人", events)
	}
	if worldPressureCount < 10 {
		t.Fatalf("world pressure events = %d, want sustained long-window world evolution", worldPressureCount)
	}
}

func TestWorldStructureInterventionDivergesLongWindowAutonomousOutcome(t *testing.T) {
	buildEngine := func(t *testing.T, worldDir, worldName, rules string, structure core.WorldStructureConfig) *Engine {
		t.Helper()
		engine := newMultiCharacterTestEngine(t)
		writeTestWorldBundle(t, worldDir, worldName, rules, core.SceneState{
			Location:    "外城",
			TimeOfDay:   "深夜",
			Weather:     "阴",
			Characters:  []string{"111", "玩家"},
			Description: "外城在夜里持续变化",
		})
		engine.ConfigurePersistence(filepath.Dir(worldDir), map[string]string{}, map[string]string{
			"111": worldDir,
		})
		engine.worldPaths["111"] = worldDir
		engine.charWorlds["111"] = CharWorld{
			WorldName: worldName,
			CoreRules: rules,
			Scene: core.SceneState{
				Location:    "外城",
				TimeOfDay:   "深夜",
				Weather:     "阴",
				Characters:  []string{"111", "玩家"},
				Description: "外城在夜里持续变化",
			},
		}
		engine.focusCharacter = "111"
		engine.SyncActiveWorldContext()

		if _, err := engine.UpdatePopulationConfig(core.PopulationConfig{
			BackgroundNPCs: []core.BackgroundNPC{
				{
					ID:       "watcher",
					Name:     "巡夜人",
					Role:     "guard",
					Location: "外城",
					Faction:  "guard",
					Traits:   []string{"警觉", "克制"},
					Hooks:    []string{"宵禁", "盘查"},
				},
				{
					ID:       "runner",
					Name:     "线人",
					Role:     "informant",
					Location: "外城",
					Faction:  "smugglers",
					Traits:   []string{"灵活", "谨慎"},
					Hooks:    []string{"走私", "风声"},
				},
			},
			Policy: core.PromotionPolicy{
				PromoteThreshold:   4.2,
				MajorThreshold:     8,
				InteractionWeight:  3,
				MentionWeight:      1,
				EventWeight:        2,
				RelationshipWeight: 3,
				SceneWeight:        2,
			},
		}); err != nil {
			t.Fatalf("UpdatePopulationConfig: %v", err)
		}
		if _, err := engine.UpdateWorldStructureConfig(structure); err != nil {
			t.Fatalf("UpdateWorldStructureConfig: %v", err)
		}
		return engine
	}

	baselineRoot := t.TempDir()
	baseline := buildEngine(t,
		filepath.Join(baselineRoot, "baseline-world"),
		"低压世界",
		"没有控制区和压力时，世界不应凭空长出主要角色",
		core.WorldStructureConfig{
			Locations: []core.WorldLocationConfig{{
				ID:          "outer_city",
				Name:        "外城",
				Kind:        "district",
				Description: "无人控制的普通街区",
				Controller:  "",
			}},
		},
	)
	intervenedRoot := t.TempDir()
	intervened := buildEngine(t,
		filepath.Join(intervenedRoot, "intervened-world"),
		"高压世界",
		"控制区和压力会改变长期世界结果",
		core.WorldStructureConfig{
			Factions: []core.WorldFactionConfig{
				{
					ID:            "guard",
					Name:          "巡城司",
					Role:          "law",
					Description:   "负责宵禁和盘查",
					Relationships: []string{"敌对 smugglers"},
				},
				{
					ID:            "smugglers",
					Name:          "走私帮",
					Role:          "criminal",
					Description:   "夜里持续活动",
					Relationships: []string{"敌对 guard"},
				},
			},
			Locations: []core.WorldLocationConfig{{
				ID:          "outer_city",
				Name:        "外城",
				Kind:        "district",
				Description: "巡城司控制区",
				Controller:  "guard",
			}},
			Pressures: []core.WorldPressureConfig{{
				ID:          "curfew",
				Name:        "宵禁升级",
				Kind:        "conflict",
				Description: "外城盘查与走私冲突持续加剧",
				Intensity:   0.9,
				Target:      "guard",
			}},
		},
	)

	for i := 0; i < 36; i++ {
		baseline.onTick()
		intervened.onTick()
	}

	baselineStatus := baseline.TickStatus()
	intervenedStatus := intervened.TickStatus()
	if !containsString(baseline.GetState().Scene.Characters, "巡夜人") || !containsString(intervened.GetState().Scene.Characters, "巡夜人") {
		t.Fatalf("scene participants baseline=%#v intervened=%#v, want scene-local npc present in both worlds before comparing long-window outcomes", baseline.GetState().Scene.Characters, intervened.GetState().Scene.Characters)
	}

	if baselineStatus["tension"].(float64) != 0 {
		t.Fatalf("baseline tension = %.2f, want calm world without pressures", baselineStatus["tension"].(float64))
	}
	if intervenedStatus["tension"].(float64) <= baselineStatus["tension"].(float64) {
		t.Fatalf("baseline tension = %.2f intervened tension = %.2f, want higher long-window tension after structure intervention", baselineStatus["tension"].(float64), intervenedStatus["tension"].(float64))
	}

	baselineInsights, err := baseline.GetPopulationInsights()
	if err != nil {
		t.Fatalf("baseline GetPopulationInsights: %v", err)
	}
	intervenedInsights, err := intervened.GetPopulationInsights()
	if err != nil {
		t.Fatalf("intervened GetPopulationInsights: %v", err)
	}
	for _, npc := range baselineInsights.Promoted {
		if npc.Name == "巡夜人" {
			t.Fatalf("baseline promoted = %#v, want no 巡夜人 promotion without structure pressure/control", baselineInsights.Promoted)
		}
	}
	intervenedPromoted := false
	for _, npc := range intervenedInsights.Promoted {
		if npc.Name == "巡夜人" {
			intervenedPromoted = true
			if npc.IdentityCore == "" || npc.Attention.Score < 4.2 {
				t.Fatalf("intervened promoted npc = %#v, want persisted identity core and promotion score", npc)
			}
		}
	}
	if !intervenedPromoted {
		t.Fatalf("intervened promoted = %#v, want 巡夜人 promoted after long-window structure intervention", intervenedInsights.Promoted)
	}

	baselineHistory, ok := baselineStatus["tick_history"].([]core.TickSnapshot)
	if !ok || len(baselineHistory) != 12 {
		t.Fatalf("baseline tick history = %#v, want capped recent history", baselineStatus["tick_history"])
	}
	intervenedHistory, ok := intervenedStatus["tick_history"].([]core.TickSnapshot)
	if !ok || len(intervenedHistory) != 12 {
		t.Fatalf("intervened tick history = %#v, want capped recent history", intervenedStatus["tick_history"])
	}

	baselineTrajectory, ok := baselineStatus["trajectory_summary"].([]string)
	if !ok || len(baselineTrajectory) == 0 {
		t.Fatalf("baseline trajectory summary = %#v, want baseline author summary", baselineStatus["trajectory_summary"])
	}
	intervenedTrajectory, ok := intervenedStatus["trajectory_summary"].([]string)
	if !ok || len(intervenedTrajectory) == 0 {
		t.Fatalf("intervened trajectory summary = %#v, want intervened author summary", intervenedStatus["trajectory_summary"])
	}
	if strings.Join(baselineTrajectory, " | ") == strings.Join(intervenedTrajectory, " | ") {
		t.Fatalf("baseline trajectory = %#v intervened trajectory = %#v, want author summaries to diverge with world outcome", baselineTrajectory, intervenedTrajectory)
	}

	baselinePressureDiagHits := 0
	baselinePopulationDiagHits := 0
	intervenedPressureDiagHits := 0
	intervenedPopulationDiagHits := 0
	for _, snapshot := range baselineHistory {
		for _, item := range snapshot.Diagnostics {
			metric, _ := item["metric"].(string)
			switch metric {
			case "active_pressure", "scene_control":
				baselinePressureDiagHits++
			case "scene_population_candidates":
				baselinePopulationDiagHits++
			}
		}
	}
	for _, snapshot := range intervenedHistory {
		for _, item := range snapshot.Diagnostics {
			metric, _ := item["metric"].(string)
			switch metric {
			case "active_pressure":
				intervenedPressureDiagHits++
			case "scene_population_candidates":
				intervenedPopulationDiagHits++
			}
		}
	}
	if baselinePressureDiagHits != 0 {
		t.Fatalf("baseline history diagnostics = %#v, want no pressure/control diagnostics in calm world", baselineHistory)
	}
	if baselinePopulationDiagHits == 0 {
		t.Fatalf("baseline history diagnostics = %#v, want scene-local population candidates to remain diagnosable", baselineHistory)
	}
	if intervenedPressureDiagHits == 0 || intervenedPopulationDiagHits == 0 {
		t.Fatalf("intervened history diagnostics = %#v, want repeated pressure/population diagnostics across long window", intervenedHistory)
	}

	baselineHighlights, _ := baselineStatus["population_highlights"].([]string)
	intervenedHighlights, _ := intervenedStatus["population_highlights"].([]string)
	if strings.Contains(strings.Join(baselineHighlights, " | "), "promoted:") {
		t.Fatalf("baseline highlights = %#v, want no promoted highlights in calm world", baselineHighlights)
	}
	if !strings.Contains(strings.Join(intervenedHighlights, " | "), "promoted: 巡夜人") {
		t.Fatalf("intervened highlights = %#v, want promoted highlight for 巡夜人", intervenedHighlights)
	}

	baselineEvents, err := baseline.eventStore.GetCanonicalEvents()
	if err != nil {
		t.Fatalf("baseline GetCanonicalEvents: %v", err)
	}
	intervenedEvents, err := intervened.eventStore.GetCanonicalEvents()
	if err != nil {
		t.Fatalf("intervened GetCanonicalEvents: %v", err)
	}
	baselineWorldPressure := 0
	intervenedWorldPressure := 0
	intervenedPromotion := 0
	for _, evt := range baselineEvents {
		if evt.Type == "world_pressure" {
			baselineWorldPressure++
		}
	}
	for _, evt := range intervenedEvents {
		if evt.Type == "world_pressure" && evt.Payload["pressure_id"] == "curfew" {
			intervenedWorldPressure++
		}
		if evt.Type == "population_promoted" && evt.Target == "巡夜人" {
			intervenedPromotion++
		}
	}
	if baselineWorldPressure != 0 {
		t.Fatalf("baseline world pressure events = %d, want no pressure-driven evolution in calm world", baselineWorldPressure)
	}
	if intervenedWorldPressure < 10 || intervenedPromotion == 0 {
		t.Fatalf("intervened events: world_pressure=%d promotion=%d, want sustained world pressure and promotion after intervention", intervenedWorldPressure, intervenedPromotion)
	}
}

func TestWorldOutcomeSampleMatrixAcrossHundredTicks(t *testing.T) {
	type sampleExpectation struct {
		name                string
		structure           core.WorldStructureConfig
		expectedPressureID  string
		expectedPromotedNPC string
		expectTensionFloor  float64
		expectCalmTension   bool
	}

	buildEngine := func(t *testing.T, sample sampleExpectation) *Engine {
		t.Helper()
		engine := newMultiCharacterTestEngine(t)
		root := t.TempDir()
		worldDir := filepath.Join(root, strings.ReplaceAll(strings.ToLower(sample.name), " ", "_"))
		writeTestWorldBundle(t, worldDir, sample.name, "多样本长窗口闭环验证", core.SceneState{
			Location:    "外城",
			TimeOfDay:   "深夜",
			Weather:     "阴",
			Characters:  []string{"111", "玩家"},
			Description: "长窗口样本矩阵",
		})
		engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
			"111": worldDir,
		})
		engine.worldPaths["111"] = worldDir
		engine.charWorlds["111"] = CharWorld{
			WorldName: sample.name,
			CoreRules: "多样本长窗口闭环验证",
			Scene: core.SceneState{
				Location:    "外城",
				TimeOfDay:   "深夜",
				Weather:     "阴",
				Characters:  []string{"111", "玩家"},
				Description: "长窗口样本矩阵",
			},
		}
		engine.focusCharacter = "111"
		engine.SyncActiveWorldContext()

		if _, err := engine.UpdatePopulationConfig(core.PopulationConfig{
			BackgroundNPCs: []core.BackgroundNPC{
				{
					ID:       "watcher",
					Name:     "巡夜人",
					Role:     "guard",
					Location: "外城",
					Faction:  "guard",
					Traits:   []string{"警觉", "克制"},
					Hooks:    []string{"宵禁", "盘查"},
				},
				{
					ID:       "runner",
					Name:     "线人",
					Role:     "informant",
					Location: "外城",
					Faction:  "smugglers",
					Traits:   []string{"灵活", "谨慎"},
					Hooks:    []string{"走私", "风声"},
				},
			},
			Policy: core.PromotionPolicy{
				PromoteThreshold:   7.3,
				MajorThreshold:     10,
				InteractionWeight:  3,
				MentionWeight:      1,
				EventWeight:        2,
				RelationshipWeight: 3,
				SceneWeight:        2,
			},
		}); err != nil {
			t.Fatalf("UpdatePopulationConfig: %v", err)
		}
		if _, err := engine.UpdateWorldStructureConfig(sample.structure); err != nil {
			t.Fatalf("UpdateWorldStructureConfig: %v", err)
		}
		return engine
	}

	samples := []sampleExpectation{
		{
			name: "Calm Sample",
			structure: core.WorldStructureConfig{
				Locations: []core.WorldLocationConfig{{
					ID:          "outer_city",
					Name:        "外城",
					Kind:        "district",
					Description: "无人控制的普通街区",
					Controller:  "",
				}},
			},
			expectCalmTension: true,
		},
		{
			name: "Guard Pressure Sample",
			structure: core.WorldStructureConfig{
				Factions: []core.WorldFactionConfig{
					{ID: "guard", Name: "巡城司", Role: "law", Relationships: []string{"敌对 smugglers"}},
					{ID: "smugglers", Name: "走私帮", Role: "criminal", Relationships: []string{"敌对 guard"}},
				},
				Locations: []core.WorldLocationConfig{{
					ID:          "outer_city",
					Name:        "外城",
					Kind:        "district",
					Description: "巡城司控制区",
					Controller:  "guard",
				}},
				Pressures: []core.WorldPressureConfig{{
					ID:          "curfew",
					Name:        "宵禁升级",
					Kind:        "conflict",
					Description: "巡城司扩大盘查",
					Intensity:   0.9,
					Target:      "guard",
				}},
			},
			expectedPressureID:  "curfew",
			expectedPromotedNPC: "巡夜人",
			expectTensionFloor:  0.4,
		},
		{
			name: "Smuggler Pressure Sample",
			structure: core.WorldStructureConfig{
				Factions: []core.WorldFactionConfig{
					{ID: "guard", Name: "巡城司", Role: "law", Relationships: []string{"敌对 smugglers"}},
					{ID: "smugglers", Name: "走私帮", Role: "criminal", Relationships: []string{"敌对 guard"}},
				},
				Locations: []core.WorldLocationConfig{{
					ID:          "outer_city",
					Name:        "外城",
					Kind:        "district",
					Description: "走私帮暗巷控制区",
					Controller:  "smugglers",
				}},
				Pressures: []core.WorldPressureConfig{{
					ID:          "smuggling",
					Name:        "走私潮上涨",
					Kind:        "criminal",
					Description: "走私帮正在快速扩张",
					Intensity:   0.88,
					Target:      "smugglers",
				}},
			},
			expectedPressureID:  "smuggling",
			expectedPromotedNPC: "线人",
			expectTensionFloor:  0.35,
		},
	}

	results := make(map[string]map[string]interface{}, len(samples))
	for _, sample := range samples {
		engine := buildEngine(t, sample)
		for i := 0; i < 120; i++ {
			engine.onTick()
		}

		status := engine.TickStatus()
		trajectory, ok := status["trajectory_summary"].([]string)
		if !ok || len(trajectory) == 0 {
			t.Fatalf("%s trajectory summary = %#v, want long-window summary after 120 ticks", sample.name, status["trajectory_summary"])
		}
		history, ok := status["tick_history"].([]core.TickSnapshot)
		if !ok || len(history) != 12 {
			t.Fatalf("%s tick history = %#v, want capped recent snapshots after 120 ticks", sample.name, status["tick_history"])
		}
		insights, err := engine.GetPopulationInsights()
		if err != nil {
			t.Fatalf("%s GetPopulationInsights: %v", sample.name, err)
		}
		promotedNames := make([]string, 0, len(insights.Promoted))
		for _, npc := range insights.Promoted {
			promotedNames = append(promotedNames, npc.Name)
		}
		topPromoted := ""
		if len(insights.Promoted) > 0 {
			topPromoted = insights.Promoted[0].Name
		}
		sort.Strings(promotedNames)

		if sample.expectCalmTension {
			if status["tension"].(float64) != 0 {
				t.Fatalf("%s tension = %.2f, want calm baseline to remain stable over 120 ticks", sample.name, status["tension"].(float64))
			}
		} else {
			if status["tension"].(float64) < sample.expectTensionFloor {
				t.Fatalf("%s tension = %.2f, want >= %.2f after 120 ticks", sample.name, status["tension"].(float64), sample.expectTensionFloor)
			}
			if !containsString(promotedNames, sample.expectedPromotedNPC) {
				t.Fatalf("%s promoted = %#v, want %s promoted after 120 ticks", sample.name, promotedNames, sample.expectedPromotedNPC)
			}
			if topPromoted != sample.expectedPromotedNPC {
				t.Fatalf("%s top promoted = %q, want %q to dominate after 120 ticks", sample.name, topPromoted, sample.expectedPromotedNPC)
			}
			if !strings.Contains(strings.Join(trajectory, " | "), sample.expectedPressureID) {
				t.Fatalf("%s trajectory = %#v, want dominant pressure %s in summary", sample.name, trajectory, sample.expectedPressureID)
			}
		}

		results[sample.name] = map[string]interface{}{
			"trajectory": strings.Join(trajectory, " | "),
			"promoted":   strings.Join(promotedNames, ","),
			"top":        topPromoted,
			"tension":    status["tension"].(float64),
		}
	}

	if results["Guard Pressure Sample"]["trajectory"] == results["Smuggler Pressure Sample"]["trajectory"] {
		t.Fatalf("guard vs smuggler trajectory = %#v vs %#v, want different long-window summaries across samples", results["Guard Pressure Sample"], results["Smuggler Pressure Sample"])
	}
	if results["Guard Pressure Sample"]["top"] == results["Smuggler Pressure Sample"]["top"] {
		t.Fatalf("guard vs smuggler top promoted = %#v vs %#v, want different promoted leaders across samples", results["Guard Pressure Sample"], results["Smuggler Pressure Sample"])
	}
}

func TestWorldOutcomeSampleMatrixAcrossTwoHundredTicks(t *testing.T) {
	type sampleExpectation struct {
		name                string
		structure           core.WorldStructureConfig
		expectedPressureID  string
		expectedPromotedNPC string
		expectTensionFloor  float64
		expectCalmTension   bool
	}

	buildEngine := func(t *testing.T, sample sampleExpectation) *Engine {
		t.Helper()
		engine := newMultiCharacterTestEngine(t)
		root := t.TempDir()
		worldDir := filepath.Join(root, strings.ReplaceAll(strings.ToLower(sample.name), " ", "_"))
		writeTestWorldBundle(t, worldDir, sample.name, "200 tick 多样本长窗口闭环验证", core.SceneState{
			Location:    "外城",
			TimeOfDay:   "深夜",
			Weather:     "阴",
			Characters:  []string{"111", "玩家"},
			Description: "200 tick 长窗口样本矩阵",
		})
		engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
			"111": worldDir,
		})
		engine.worldPaths["111"] = worldDir
		engine.charWorlds["111"] = CharWorld{
			WorldName: sample.name,
			CoreRules: "200 tick 多样本长窗口闭环验证",
			Scene: core.SceneState{
				Location:    "外城",
				TimeOfDay:   "深夜",
				Weather:     "阴",
				Characters:  []string{"111", "玩家"},
				Description: "200 tick 长窗口样本矩阵",
			},
		}
		engine.focusCharacter = "111"
		engine.SyncActiveWorldContext()

		if _, err := engine.UpdatePopulationConfig(core.PopulationConfig{
			BackgroundNPCs: []core.BackgroundNPC{
				{
					ID:       "watcher",
					Name:     "巡夜人",
					Role:     "guard",
					Location: "外城",
					Faction:  "guard",
					Traits:   []string{"警觉", "克制"},
					Hooks:    []string{"宵禁", "盘查"},
				},
				{
					ID:       "runner",
					Name:     "线人",
					Role:     "informant",
					Location: "外城",
					Faction:  "smugglers",
					Traits:   []string{"灵活", "谨慎"},
					Hooks:    []string{"走私", "风声"},
				},
				{
					ID:       "fixer",
					Name:     "修灯人",
					Role:     "utility",
					Location: "外城",
					Faction:  "operators",
					Traits:   []string{"耐心", "稳重"},
					Hooks:    []string{"停电", "供电", "断路"},
				},
			},
			Policy: core.PromotionPolicy{
				PromoteThreshold:   7.3,
				MajorThreshold:     12,
				InteractionWeight:  3,
				MentionWeight:      1,
				EventWeight:        2,
				RelationshipWeight: 3,
				SceneWeight:        2,
			},
		}); err != nil {
			t.Fatalf("UpdatePopulationConfig: %v", err)
		}
		if _, err := engine.UpdateWorldStructureConfig(sample.structure); err != nil {
			t.Fatalf("UpdateWorldStructureConfig: %v", err)
		}
		return engine
	}

	samples := []sampleExpectation{
		{
			name: "Calm 200 Tick Sample",
			structure: core.WorldStructureConfig{
				Locations: []core.WorldLocationConfig{{
					ID:          "outer_city",
					Name:        "外城",
					Kind:        "district",
					Description: "无人控制的普通街区",
					Controller:  "",
				}},
			},
			expectCalmTension: true,
		},
		{
			name: "Guard 200 Tick Sample",
			structure: core.WorldStructureConfig{
				Factions: []core.WorldFactionConfig{
					{ID: "guard", Name: "巡城司", Role: "law", Relationships: []string{"敌对 smugglers"}},
					{ID: "smugglers", Name: "走私帮", Role: "criminal", Relationships: []string{"敌对 guard"}},
				},
				Locations: []core.WorldLocationConfig{{
					ID:          "outer_city",
					Name:        "外城",
					Kind:        "district",
					Description: "巡城司控制区",
					Controller:  "guard",
				}},
				Pressures: []core.WorldPressureConfig{{
					ID:          "curfew",
					Name:        "宵禁升级",
					Kind:        "conflict",
					Description: "巡城司扩大盘查",
					Intensity:   0.92,
					Target:      "guard",
				}},
			},
			expectedPressureID:  "curfew",
			expectedPromotedNPC: "巡夜人",
			expectTensionFloor:  0.7,
		},
		{
			name: "Smuggler 200 Tick Sample",
			structure: core.WorldStructureConfig{
				Factions: []core.WorldFactionConfig{
					{ID: "guard", Name: "巡城司", Role: "law", Relationships: []string{"敌对 smugglers"}},
					{ID: "smugglers", Name: "走私帮", Role: "criminal", Relationships: []string{"敌对 guard"}},
				},
				Locations: []core.WorldLocationConfig{{
					ID:          "outer_city",
					Name:        "外城",
					Kind:        "district",
					Description: "走私帮暗巷控制区",
					Controller:  "smugglers",
				}},
				Pressures: []core.WorldPressureConfig{{
					ID:          "smuggling",
					Name:        "走私潮上涨",
					Kind:        "criminal",
					Description: "走私帮正在快速扩张",
					Intensity:   0.9,
					Target:      "smugglers",
				}},
			},
			expectedPressureID:  "smuggling",
			expectedPromotedNPC: "线人",
			expectTensionFloor:  0.65,
		},
		{
			name: "Infrastructure 200 Tick Sample",
			structure: core.WorldStructureConfig{
				Factions: []core.WorldFactionConfig{
					{ID: "operators", Name: "电网维护队", Role: "utility", Relationships: []string{"紧张 guard"}},
					{ID: "guard", Name: "巡城司", Role: "law", Relationships: []string{"依赖 operators"}},
				},
				Locations: []core.WorldLocationConfig{{
					ID:          "outer_city",
					Name:        "外城",
					Kind:        "district",
					Description: "断电频发的维护区",
					Controller:  "operators",
				}},
				Pressures: []core.WorldPressureConfig{{
					ID:          "blackout",
					Name:        "电网失稳",
					Kind:        "infrastructure",
					Description: "外城频繁断电，维护队持续抢修",
					Intensity:   0.87,
					Target:      "operators",
				}},
			},
			expectedPressureID:  "blackout",
			expectedPromotedNPC: "修灯人",
			expectTensionFloor:  0.55,
		},
	}

	results := make(map[string]map[string]interface{}, len(samples))
	for _, sample := range samples {
		engine := buildEngine(t, sample)
		for i := 0; i < 200; i++ {
			engine.onTick()
		}

		status := engine.TickStatus()
		if tickCount, ok := status["tick_count"].(int); !ok || tickCount != 0 {
			t.Fatalf("%s tick_count = %#v, want idle manual-tick status to remain 0 without loop", sample.name, status["tick_count"])
		}
		trajectory, ok := status["trajectory_summary"].([]string)
		if !ok || len(trajectory) == 0 {
			t.Fatalf("%s trajectory summary = %#v, want long-window summary after 200 ticks", sample.name, status["trajectory_summary"])
		}
		history, ok := status["tick_history"].([]core.TickSnapshot)
		if !ok || len(history) != 12 {
			t.Fatalf("%s tick history = %#v, want capped recent snapshots after 200 ticks", sample.name, status["tick_history"])
		}
		insights, err := engine.GetPopulationInsights()
		if err != nil {
			t.Fatalf("%s GetPopulationInsights: %v", sample.name, err)
		}
		promotedNames := make([]string, 0, len(insights.Promoted))
		for _, npc := range insights.Promoted {
			promotedNames = append(promotedNames, npc.Name)
		}
		topPromoted := ""
		if len(insights.Promoted) > 0 {
			topPromoted = insights.Promoted[0].Name
		}
		sort.Strings(promotedNames)

		if sample.expectCalmTension {
			if status["tension"].(float64) != 0 {
				t.Fatalf("%s tension = %.2f, want calm baseline to remain stable over 200 ticks", sample.name, status["tension"].(float64))
			}
		} else {
			if status["tension"].(float64) < sample.expectTensionFloor {
				t.Fatalf("%s tension = %.2f, want >= %.2f after 200 ticks", sample.name, status["tension"].(float64), sample.expectTensionFloor)
			}
			if !containsString(promotedNames, sample.expectedPromotedNPC) {
				t.Fatalf("%s promoted = %#v, want %s promoted after 200 ticks", sample.name, promotedNames, sample.expectedPromotedNPC)
			}
			if topPromoted != sample.expectedPromotedNPC {
				t.Fatalf("%s top promoted = %q, want %q to dominate after 200 ticks", sample.name, topPromoted, sample.expectedPromotedNPC)
			}
			if !strings.Contains(strings.Join(trajectory, " | "), sample.expectedPressureID) {
				t.Fatalf("%s trajectory = %#v, want dominant pressure %s in 200-tick summary", sample.name, trajectory, sample.expectedPressureID)
			}
		}

		results[sample.name] = map[string]interface{}{
			"trajectory": strings.Join(trajectory, " | "),
			"promoted":   strings.Join(promotedNames, ","),
			"top":        topPromoted,
			"tension":    status["tension"].(float64),
		}
	}

	if results["Guard 200 Tick Sample"]["trajectory"] == results["Smuggler 200 Tick Sample"]["trajectory"] {
		t.Fatalf("guard vs smuggler 200-tick trajectory = %#v vs %#v, want different long-window summaries across samples", results["Guard 200 Tick Sample"], results["Smuggler 200 Tick Sample"])
	}
	if results["Guard 200 Tick Sample"]["top"] == results["Smuggler 200 Tick Sample"]["top"] {
		t.Fatalf("guard vs smuggler 200-tick top promoted = %#v vs %#v, want different promoted leaders across samples", results["Guard 200 Tick Sample"], results["Smuggler 200 Tick Sample"])
	}
	if results["Infrastructure 200 Tick Sample"]["trajectory"] == results["Guard 200 Tick Sample"]["trajectory"] {
		t.Fatalf("infrastructure vs guard 200-tick trajectory = %#v vs %#v, want broader world outcome divergence", results["Infrastructure 200 Tick Sample"], results["Guard 200 Tick Sample"])
	}
}

func TestRealWorldDirectorySampleMatrixAcrossHundredTwentyTicks(t *testing.T) {
	type sampleExpectation struct {
		name                string
		sourceDir           string
		scene               core.SceneState
		expectedPromotedNPC string
		expectedPressureID  string
		expectNoPromotion   bool
		expectTensionFloor  float64
		configure           func(t *testing.T, engine *Engine, worldDir string)
	}

	buildEngine := func(t *testing.T, sample sampleExpectation) *Engine {
		t.Helper()
		engine := newMultiCharacterTestEngine(t)
		root := t.TempDir()
		worldDir := filepath.Join(root, strings.ReplaceAll(strings.ToLower(sample.name), " ", "_"))
		copyDir(t, filepath.Join("..", "..", "worlds", sample.sourceDir), worldDir)

		engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
			"111": worldDir,
		})
		engine.worldPaths["111"] = worldDir
		engine.charWorlds["111"] = CharWorld{
			WorldName: sample.name,
			CoreRules: "真实世界目录长窗口验证",
			Scene:     sample.scene,
		}
		engine.focusCharacter = "111"
		engine.SyncActiveWorldContext()

		if sample.configure != nil {
			sample.configure(t, engine, worldDir)
		}
		return engine
	}

	samples := []sampleExpectation{
		{
			name:      "Neon Block Real World",
			sourceDir: "neon_block",
			scene: core.SceneState{
				Location:    "旧街夜市",
				TimeOfDay:   "深夜",
				Weather:     "闷热有雨",
				Characters:  []string{"蓝姐", "谭叔", "玩家"},
				Description: "真实世界目录中的默认夜市场景",
			},
			expectedPromotedNPC: "蓝姐",
			expectedPressureID:  "missing_rider",
			expectTensionFloor:  0.45,
		},
		{
			name:      "Wedding Import Real World",
			sourceDir: "1_7",
			scene: core.SceneState{
				Location:    "未知地点",
				TimeOfDay:   "白天",
				Weather:     "阴雨",
				Characters:  []string{"许灵_单阶段人设", "玩家"},
				Description: "真实导入世界的默认接站场景",
			},
			expectedPromotedNPC: "婚礼管家",
			expectedPressureID:  "arrival_gossip",
			expectTensionFloor:  0.50,
			configure: func(t *testing.T, engine *Engine, worldDir string) {
				t.Helper()
				if _, err := engine.UpdatePopulationConfig(core.PopulationConfig{
					BackgroundNPCs: []core.BackgroundNPC{
						{
							ID:       "steward",
							Name:     "婚礼管家",
							Role:     "steward",
							Location: "未知地点",
							Faction:  "wedding_hosts",
							Traits:   []string{"周到", "急切"},
							Hooks:    []string{"要把迟到接站压下去", "不想婚礼前出乱子"},
						},
						{
							ID:       "driver",
							Name:     "代驾老周",
							Role:     "driver",
							Location: "车站外",
							Faction:  "station_runners",
							Traits:   []string{"疲惫", "圆滑"},
							Hooks:    []string{"谁临时改了接站安排", "想把责任甩出去"},
						},
						{
							ID:       "guard",
							Name:     "站台保安",
							Role:     "guard",
							Location: "候车厅",
							Faction:  "station_runners",
							Traits:   []string{"谨慎", "怕麻烦"},
							Hooks:    []string{"担心现场起争执", "不想事情闹大"},
						},
					},
					Policy: core.PromotionPolicy{
						PromoteThreshold:   6.8,
						MajorThreshold:     11,
						InteractionWeight:  3,
						MentionWeight:      1,
						EventWeight:        2,
						RelationshipWeight: 3,
						SceneWeight:        2,
					},
				}); err != nil {
					t.Fatalf("UpdatePopulationConfig wedding import: %v", err)
				}
				if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
					Locations: []core.WorldLocationConfig{
						{ID: "arrival_point", Name: "未知地点", Kind: "arrival", Description: "婚礼接站与临时协调点", Controller: "wedding_hosts"},
						{ID: "station_gate", Name: "车站外", Kind: "transit", Description: "接站车与代驾聚集的混乱出口", Controller: "station_runners"},
						{ID: "platform_hall", Name: "候车厅", Kind: "waiting", Description: "旅客和保安都不想久留的大厅", Controller: "station_runners"},
					},
					Factions: []core.WorldFactionConfig{
						{ID: "wedding_hosts", Name: "婚礼主家", Role: "family", Relationships: []string{"压制 station_runners"}},
						{ID: "station_runners", Name: "接站跑腿圈", Role: "logistics", Relationships: []string{"不信任 wedding_hosts"}},
					},
					Pressures: []core.WorldPressureConfig{
						{ID: "pickup_delay", Name: "接站迟到", Kind: "coordination", Description: "婚礼前的接站安排持续失序", Intensity: 0.84, Target: "wedding_hosts"},
						{ID: "arrival_gossip", Name: "站台风声", Kind: "rumor", Description: "谁被怠慢、谁在甩锅开始扩散", Intensity: 0.62, Target: "未知地点"},
					},
				}); err != nil {
					t.Fatalf("UpdateWorldStructureConfig wedding import: %v", err)
				}
			},
		},
		{
			name:      "Dream Mansion Real World",
			sourceDir: "《红楼梦》完整版、-角色卡-202604190812",
			scene: core.SceneState{
				Location:    "未知地点",
				TimeOfDay:   "未知时间",
				Weather:     "未知天气",
				Characters:  []string{"薛宝钗", "玩家"},
				Description: "真实导入世界的默认闺阁场景",
			},
			expectedPromotedNPC: "莺儿",
			expectedPressureID:  "maids_whisper",
			expectTensionFloor:  0.48,
			configure: func(t *testing.T, engine *Engine, worldDir string) {
				t.Helper()
				if _, _, err := world.EnsureSeededPopulation(worldDir); err != nil {
					t.Fatalf("EnsureSeededPopulation dream mansion: %v", err)
				}
				if _, err := engine.UpdatePopulationConfig(core.PopulationConfig{
					BackgroundNPCs: []core.BackgroundNPC{
						{
							ID:       "yinger",
							Name:     "莺儿",
							Role:     "侍女",
							Location: "未知地点",
							Faction:  "xue_house",
							Traits:   []string{"机灵", "知分寸"},
							Hooks:    []string{"替宝姑娘探听风声", "不想让诗社话头失控"},
						},
						{
							ID:       "housemaid",
							Name:     "婆子",
							Role:     "杂役",
							Location: "回廊",
							Faction:  "rong_house",
							Traits:   []string{"谨慎", "嘴碎"},
							Hooks:    []string{"最怕传错话", "担心被责罚"},
						},
						{
							ID:       "page",
							Name:     "小厮",
							Role:     "跑腿",
							Location: "书房外",
							Faction:  "poetry_circle",
							Traits:   []string{"轻快", "爱看热闹"},
							Hooks:    []string{"去回话", "把诗社消息带错边"},
						},
					},
					Policy: core.PromotionPolicy{
						PromoteThreshold:   6.8,
						MajorThreshold:     11,
						InteractionWeight:  3,
						MentionWeight:      1,
						EventWeight:        2,
						RelationshipWeight: 3,
						SceneWeight:        2,
					},
				}); err != nil {
					t.Fatalf("UpdatePopulationConfig dream mansion: %v", err)
				}
				if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
					Locations: []core.WorldLocationConfig{
						{ID: "boudoir", Name: "未知地点", Kind: "residence", Description: "闺阁内室，消息传得不快却更要紧", Controller: "xue_house"},
						{ID: "corridor", Name: "回廊", Kind: "transit", Description: "丫鬟婆子擦身而过、最容易串话", Controller: "rong_house"},
						{ID: "study_gate", Name: "书房外", Kind: "service", Description: "回话与递帖都得经过的地方", Controller: "poetry_circle"},
					},
					Factions: []core.WorldFactionConfig{
						{ID: "xue_house", Name: "薛家房内", Role: "household", Relationships: []string{"顾忌 rong_house"}},
						{ID: "rong_house", Name: "荣府杂役", Role: "household", Relationships: []string{"议论 xue_house"}},
						{ID: "poetry_circle", Name: "诗社往来圈", Role: "social", Relationships: []string{"牵动 xue_house"}},
					},
					Pressures: []core.WorldPressureConfig{
						{ID: "poetry_society", Name: "诗社风声", Kind: "social", Description: "诗社流言让宝钗身边的人先紧张起来", Intensity: 0.81, Target: "xue_house"},
						{ID: "maids_whisper", Name: "回廊私语", Kind: "rumor", Description: "回廊里关于谁该出面的话越传越偏", Intensity: 0.58, Target: "未知地点"},
					},
				}); err != nil {
					t.Fatalf("UpdateWorldStructureConfig dream mansion: %v", err)
				}
			},
		},
	}

	results := make(map[string]map[string]interface{}, len(samples))
	for _, sample := range samples {
		engine := buildEngine(t, sample)
		for i := 0; i < 120; i++ {
			engine.onTick()
		}

		status := engine.TickStatus()
		trajectory, ok := status["trajectory_summary"].([]string)
		if !ok || len(trajectory) == 0 {
			t.Fatalf("%s trajectory summary = %#v, want long-window summary from real world directory", sample.name, status["trajectory_summary"])
		}
		joinedTrajectory := strings.Join(trajectory, " | ")
		history, ok := status["tick_history"].([]core.TickSnapshot)
		if !ok || len(history) != 12 {
			t.Fatalf("%s tick history = %#v, want capped recent snapshots after 120 ticks", sample.name, status["tick_history"])
		}
		insights, err := engine.GetPopulationInsights()
		if err != nil {
			t.Fatalf("%s GetPopulationInsights: %v", sample.name, err)
		}
		promotedNames := make([]string, 0, len(insights.Promoted))
		for _, npc := range insights.Promoted {
			promotedNames = append(promotedNames, npc.Name)
		}
		sort.Strings(promotedNames)

		if sample.expectNoPromotion {
			if len(promotedNames) != 0 {
				t.Fatalf("%s promoted = %#v, want no promotion from real world sample", sample.name, promotedNames)
			}
		} else {
			if tension, _ := status["tension"].(float64); tension < sample.expectTensionFloor {
				t.Fatalf("%s tension = %#v, want >= %.2f from real world sample", sample.name, status["tension"], sample.expectTensionFloor)
			}
			if !containsString(promotedNames, sample.expectedPromotedNPC) {
				t.Fatalf("%s promoted = %#v, want %s promoted from real world sample", sample.name, promotedNames, sample.expectedPromotedNPC)
			}
			if !strings.Contains(joinedTrajectory, sample.expectedPressureID) {
				t.Fatalf("%s trajectory = %q, want dominant pressure %s from real world sample", sample.name, joinedTrajectory, sample.expectedPressureID)
			}
		}

		results[sample.name] = map[string]interface{}{
			"trajectory": joinedTrajectory,
			"promoted":   strings.Join(promotedNames, ","),
			"tension":    status["tension"].(float64),
		}
	}

	if results["Neon Block Real World"]["trajectory"] == results["Wedding Import Real World"]["trajectory"] {
		t.Fatalf("neon vs wedding real-world trajectory = %#v vs %#v, want divergent long-window summaries across real world families", results["Neon Block Real World"], results["Wedding Import Real World"])
	}
	if results["Wedding Import Real World"]["promoted"] == results["Dream Mansion Real World"]["promoted"] {
		t.Fatalf("wedding vs dream real-world promoted = %#v vs %#v, want different promoted leaders across imported world families", results["Wedding Import Real World"], results["Dream Mansion Real World"])
	}
}

func TestRealWorldDirectorySampleMatrixAcrossTwoHundredTicks(t *testing.T) {
	type sampleExpectation struct {
		name                string
		sourceDir           string
		scene               core.SceneState
		expectedPromotedNPC string
		expectedPressureID  string
		expectNoPromotion   bool
		expectTensionFloor  float64
		configure           func(t *testing.T, engine *Engine, worldDir string)
	}

	buildEngine := func(t *testing.T, sample sampleExpectation) *Engine {
		t.Helper()
		engine := newMultiCharacterTestEngine(t)
		root := t.TempDir()
		worldDir := filepath.Join(root, strings.ReplaceAll(strings.ToLower(sample.name), " ", "_"))
		copyDir(t, filepath.Join("..", "..", "worlds", sample.sourceDir), worldDir)

		engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
			"111": worldDir,
		})
		engine.worldPaths["111"] = worldDir
		engine.charWorlds["111"] = CharWorld{
			WorldName: sample.name,
			CoreRules: "真实世界目录长窗口验证",
			Scene:     sample.scene,
		}
		engine.focusCharacter = "111"
		engine.SyncActiveWorldContext()

		if sample.configure != nil {
			sample.configure(t, engine, worldDir)
		}
		return engine
	}

	samples := []sampleExpectation{
		{
			name:      "Neon Block Real World 200",
			sourceDir: "neon_block",
			scene: core.SceneState{
				Location:    "旧街夜市",
				TimeOfDay:   "深夜",
				Weather:     "闷热有雨",
				Characters:  []string{"蓝姐", "谭叔", "玩家"},
				Description: "真实世界目录中的默认夜市场景",
			},
			expectedPromotedNPC: "蓝姐",
			expectedPressureID:  "missing_rider",
			expectTensionFloor:  0.45,
		},
		{
			name:      "Wedding Import Real World 200",
			sourceDir: "1_7",
			scene: core.SceneState{
				Location:    "未知地点",
				TimeOfDay:   "白天",
				Weather:     "阴雨",
				Characters:  []string{"许灵_单阶段人设", "玩家"},
				Description: "真实导入世界的默认接站场景",
			},
			expectedPromotedNPC: "婚礼管家",
			expectedPressureID:  "arrival_gossip",
			expectTensionFloor:  0.50,
			configure: func(t *testing.T, engine *Engine, worldDir string) {
				t.Helper()
				if _, err := engine.UpdatePopulationConfig(core.PopulationConfig{
					BackgroundNPCs: []core.BackgroundNPC{
						{
							ID:       "steward",
							Name:     "婚礼管家",
							Role:     "steward",
							Location: "未知地点",
							Faction:  "wedding_hosts",
							Traits:   []string{"周到", "急切"},
							Hooks:    []string{"要把迟到接站压下去", "不想婚礼前出乱子"},
						},
						{
							ID:       "driver",
							Name:     "代驾老周",
							Role:     "driver",
							Location: "车站外",
							Faction:  "station_runners",
							Traits:   []string{"疲惫", "圆滑"},
							Hooks:    []string{"谁临时改了接站安排", "想把责任甩出去"},
						},
						{
							ID:       "guard",
							Name:     "站台保安",
							Role:     "guard",
							Location: "候车厅",
							Faction:  "station_runners",
							Traits:   []string{"谨慎", "怕麻烦"},
							Hooks:    []string{"担心现场起争执", "不想事情闹大"},
						},
					},
					Policy: core.PromotionPolicy{
						PromoteThreshold:   6.8,
						MajorThreshold:     11,
						InteractionWeight:  3,
						MentionWeight:      1,
						EventWeight:        2,
						RelationshipWeight: 3,
						SceneWeight:        2,
					},
				}); err != nil {
					t.Fatalf("UpdatePopulationConfig wedding import: %v", err)
				}
				if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
					Locations: []core.WorldLocationConfig{
						{ID: "arrival_point", Name: "未知地点", Kind: "arrival", Description: "婚礼接站与临时协调点", Controller: "wedding_hosts"},
						{ID: "station_gate", Name: "车站外", Kind: "transit", Description: "接站车与代驾聚集的混乱出口", Controller: "station_runners"},
						{ID: "platform_hall", Name: "候车厅", Kind: "waiting", Description: "旅客和保安都不想久留的大厅", Controller: "station_runners"},
					},
					Factions: []core.WorldFactionConfig{
						{ID: "wedding_hosts", Name: "婚礼主家", Role: "family", Relationships: []string{"压制 station_runners"}},
						{ID: "station_runners", Name: "接站跑腿圈", Role: "logistics", Relationships: []string{"不信任 wedding_hosts"}},
					},
					Pressures: []core.WorldPressureConfig{
						{ID: "pickup_delay", Name: "接站迟到", Kind: "coordination", Description: "婚礼前的接站安排持续失序", Intensity: 0.84, Target: "wedding_hosts"},
						{ID: "arrival_gossip", Name: "站台风声", Kind: "rumor", Description: "谁被怠慢、谁在甩锅开始扩散", Intensity: 0.62, Target: "未知地点"},
					},
				}); err != nil {
					t.Fatalf("UpdateWorldStructureConfig wedding import: %v", err)
				}
			},
		},
		{
			name:      "Dream Mansion Real World 200",
			sourceDir: "《红楼梦》完整版、-角色卡-202604190812",
			scene: core.SceneState{
				Location:    "未知地点",
				TimeOfDay:   "未知时间",
				Weather:     "未知天气",
				Characters:  []string{"薛宝钗", "玩家"},
				Description: "真实导入世界的默认闺阁场景",
			},
			expectedPromotedNPC: "莺儿",
			expectedPressureID:  "maids_whisper",
			expectTensionFloor:  0.48,
			configure: func(t *testing.T, engine *Engine, worldDir string) {
				t.Helper()
				if _, _, err := world.EnsureSeededPopulation(worldDir); err != nil {
					t.Fatalf("EnsureSeededPopulation dream mansion: %v", err)
				}
				if _, err := engine.UpdatePopulationConfig(core.PopulationConfig{
					BackgroundNPCs: []core.BackgroundNPC{
						{
							ID:       "yinger",
							Name:     "莺儿",
							Role:     "侍女",
							Location: "未知地点",
							Faction:  "xue_house",
							Traits:   []string{"机灵", "知分寸"},
							Hooks:    []string{"替宝姑娘探听风声", "不想让诗社话头失控"},
						},
						{
							ID:       "housemaid",
							Name:     "婆子",
							Role:     "杂役",
							Location: "回廊",
							Faction:  "rong_house",
							Traits:   []string{"谨慎", "嘴碎"},
							Hooks:    []string{"最怕传错话", "担心被责罚"},
						},
						{
							ID:       "page",
							Name:     "小厮",
							Role:     "跑腿",
							Location: "书房外",
							Faction:  "poetry_circle",
							Traits:   []string{"轻快", "爱看热闹"},
							Hooks:    []string{"去回话", "把诗社消息带错边"},
						},
					},
					Policy: core.PromotionPolicy{
						PromoteThreshold:   6.8,
						MajorThreshold:     11,
						InteractionWeight:  3,
						MentionWeight:      1,
						EventWeight:        2,
						RelationshipWeight: 3,
						SceneWeight:        2,
					},
				}); err != nil {
					t.Fatalf("UpdatePopulationConfig dream mansion: %v", err)
				}
				if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
					Locations: []core.WorldLocationConfig{
						{ID: "boudoir", Name: "未知地点", Kind: "residence", Description: "闺阁内室，消息传得不快却更要紧", Controller: "xue_house"},
						{ID: "corridor", Name: "回廊", Kind: "transit", Description: "丫鬟婆子擦身而过、最容易串话", Controller: "rong_house"},
						{ID: "study_gate", Name: "书房外", Kind: "service", Description: "回话与递帖都得经过的地方", Controller: "poetry_circle"},
					},
					Factions: []core.WorldFactionConfig{
						{ID: "xue_house", Name: "薛家房内", Role: "household", Relationships: []string{"顾忌 rong_house"}},
						{ID: "rong_house", Name: "荣府杂役", Role: "household", Relationships: []string{"议论 xue_house"}},
						{ID: "poetry_circle", Name: "诗社往来圈", Role: "social", Relationships: []string{"牵动 xue_house"}},
					},
					Pressures: []core.WorldPressureConfig{
						{ID: "poetry_society", Name: "诗社风声", Kind: "social", Description: "诗社流言让宝钗身边的人先紧张起来", Intensity: 0.81, Target: "xue_house"},
						{ID: "maids_whisper", Name: "回廊私语", Kind: "rumor", Description: "回廊里关于谁该出面的话越传越偏", Intensity: 0.58, Target: "未知地点"},
					},
				}); err != nil {
					t.Fatalf("UpdateWorldStructureConfig dream mansion: %v", err)
				}
			},
		},
		{
			name:      "Campus Villa Real World 200",
			sourceDir: "48111430a81be7d4",
			scene: core.SceneState{
				Location:    "别墅",
				TimeOfDay:   "白天",
				Weather:     "晴朗炎热",
				Characters:  []string{"赵小亮", "玩家", "沈佳"},
				Description: "真实世界目录中的校园别墅场景",
			},
			expectedPromotedNPC: "别墅巡守",
			expectedPressureID:  "family_secret",
			expectTensionFloor:  0.42,
			configure: func(t *testing.T, engine *Engine, worldDir string) {
				t.Helper()
				if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
					Locations: []core.WorldLocationConfig{
						{ID: "villa", Name: "别墅", Kind: "residence", Description: "三层建筑住宅，玩家与赵小亮的活动中心", Controller: "villa_family"},
						{ID: "school", Name: "明南高中", Kind: "school", Description: "半封闭式管理学校，设有高二4班", Controller: "school_faculty"},
						{ID: "village", Name: "上溪村", Kind: "rural", Description: "偏远乡下，玩家奶奶居住地", Controller: "village_locals"},
					},
					Factions: []core.WorldFactionConfig{
						{ID: "villa_family", Name: "别墅家庭", Role: "household", Relationships: []string{"顾忌 school_faculty"}},
						{ID: "school_faculty", Name: "明南高中教职工", Role: "authority", Relationships: []string{"监督 villa_family"}},
						{ID: "village_locals", Name: "上溪村村民", Role: "community", Relationships: []string{"远离 villa_family"}},
					},
					Pressures: []core.WorldPressureConfig{
						{ID: "family_secret", Name: "家庭秘密", Kind: "domestic", Description: "别墅内的异常关系引发暗流", Intensity: 0.75, Target: "villa_family"},
						{ID: "school_rumors", Name: "校园传闻", Kind: "rumor", Description: "明南高中关于玩家的传闻开始扩散", Intensity: 0.60, Target: "别墅"},
					},
				}); err != nil {
					t.Fatalf("UpdateWorldStructureConfig campus villa: %v", err)
				}
			},
		},
		{
			name:      "Streaming Penthouse Real World 200",
			sourceDir: "a0c85d27e38863a4",
			scene: core.SceneState{
				Location:    "客厅",
				TimeOfDay:   "夜晚",
				Weather:     "阴雨",
				Characters:  []string{"ANJONI小玖", "玩家"},
				Description: "真实世界目录中的直播顶层场景",
			},
			expectedPromotedNPC: "客厅巡守",
			expectedPressureID:  "platform_riot",
			expectTensionFloor:  0.42,
			configure: func(t *testing.T, engine *Engine, worldDir string) {
				t.Helper()
				if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
					Locations: []core.WorldLocationConfig{
						{ID: "penthouse", Name: "客厅", Kind: "residence", Description: "汤臣一品顶层大平层，直播与社交中心", Controller: "streamer_team"},
						{ID: "stream_room", Name: "直播间", Kind: "workspace", Description: "专业直播设备与拍摄区域", Controller: "streamer_team"},
						{ID: "backstage", Name: "后台", Kind: "logistics", Description: "运营团队与来访者等候区", Controller: "rival_streamers"},
					},
					Factions: []core.WorldFactionConfig{
						{ID: "streamer_team", Name: "主播团队", Role: "content", Relationships: []string{"防范 rival_streamers"}},
						{ID: "rival_streamers", Name: "竞品主播", Role: "competition", Relationships: []string{"觊觎 streamer_team 流量"}},
					},
					Pressures: []core.WorldPressureConfig{
						{ID: "platform_riot", Name: "平台风波", Kind: "crisis", Description: "斗鱼平台政策变动引发主播圈动荡", Intensity: 0.78, Target: "streamer_team"},
						{ID: "stream_rivalry", Name: "直播竞争", Kind: "competition", Description: "竞品主播暗中挖角与流量争夺", Intensity: 0.65, Target: "客厅"},
					},
				}); err != nil {
					t.Fatalf("UpdateWorldStructureConfig streaming penthouse: %v", err)
				}
			},
		},
	}

	results := make(map[string]map[string]interface{}, len(samples))
	for _, sample := range samples {
		engine := buildEngine(t, sample)
		for i := 0; i < 200; i++ {
			engine.onTick()
		}

		status := engine.TickStatus()
		trajectory, ok := status["trajectory_summary"].([]string)
		if !ok || len(trajectory) == 0 {
			t.Fatalf("%s trajectory summary = %#v, want long-window summary from real world directory", sample.name, status["trajectory_summary"])
		}
		joinedTrajectory := strings.Join(trajectory, " | ")
		history, ok := status["tick_history"].([]core.TickSnapshot)
		if !ok || len(history) != 12 {
			t.Fatalf("%s tick history = %#v, want capped recent snapshots after 200 ticks", sample.name, status["tick_history"])
		}
		insights, err := engine.GetPopulationInsights()
		if err != nil {
			t.Fatalf("%s GetPopulationInsights: %v", sample.name, err)
		}
		promotedNames := make([]string, 0, len(insights.Promoted))
		for _, npc := range insights.Promoted {
			promotedNames = append(promotedNames, npc.Name)
		}
		sort.Strings(promotedNames)

		if sample.expectNoPromotion {
			if len(promotedNames) != 0 {
				t.Fatalf("%s promoted = %#v, want no promotion from real world sample", sample.name, promotedNames)
			}
		} else {
			if tension, _ := status["tension"].(float64); tension < sample.expectTensionFloor {
				t.Fatalf("%s tension = %#v, want >= %.2f from real world sample", sample.name, status["tension"], sample.expectTensionFloor)
			}
			if !containsString(promotedNames, sample.expectedPromotedNPC) {
				t.Fatalf("%s promoted = %#v, want %s promoted from real world sample", sample.name, promotedNames, sample.expectedPromotedNPC)
			}
			if !strings.Contains(joinedTrajectory, sample.expectedPressureID) {
				t.Fatalf("%s trajectory = %q, want dominant pressure %s from real world sample", sample.name, joinedTrajectory, sample.expectedPressureID)
			}
		}

		results[sample.name] = map[string]interface{}{
			"trajectory": joinedTrajectory,
			"promoted":   strings.Join(promotedNames, ","),
			"tension":    status["tension"].(float64),
		}
	}

	if results["Neon Block Real World 200"]["trajectory"] == results["Wedding Import Real World 200"]["trajectory"] {
		t.Fatalf("neon vs wedding real-world 200-tick trajectory = %#v vs %#v, want divergent long-window summaries across real world families", results["Neon Block Real World 200"], results["Wedding Import Real World 200"])
	}
	if results["Wedding Import Real World 200"]["promoted"] == results["Dream Mansion Real World 200"]["promoted"] {
		t.Fatalf("wedding vs dream real-world 200-tick promoted = %#v vs %#v, want different promoted leaders across imported world families", results["Wedding Import Real World 200"], results["Dream Mansion Real World 200"])
	}
}

func writeTestWorldBundle(t *testing.T, worldDir, name, rules string, scene core.SceneState) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(worldDir, "canon"), 0755); err != nil {
		t.Fatalf("mkdir canon: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(worldDir, "scenes"), 0755); err != nil {
		t.Fatalf("mkdir scenes: %v", err)
	}
	worldYAML := "meta:\n  name: " + name + "\ncore_rules: |\n  " + rules + "\n"
	sceneYAML := "scene:\n" +
		"  location: " + scene.Location + "\n" +
		"  time_of_day: " + scene.TimeOfDay + "\n" +
		"  weather: " + scene.Weather + "\n" +
		"  present_chars:\n"
	for _, character := range scene.Characters {
		sceneYAML += "    - " + character + "\n"
	}
	sceneYAML += "  atmosphere: " + scene.Description + "\n"
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

func copyDir(t *testing.T, src, dst string) {
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

func TestRealWorldDirectoryStabilityAcrossFiveHundredTicks(t *testing.T) {
	if os.Getenv("CORERP_RUN_SLOW_PROOF_TESTS") != "1" {
		t.Skip("set CORERP_RUN_SLOW_PROOF_TESTS=1 to run 500 tick proof audit stability test")
	}

	type sampleExpectation struct {
		name                string
		sourceDir           string
		scene               core.SceneState
		expectedPromotedNPC string
		expectedPressureID  string
		expectTensionFloor  float64
		configure           func(t *testing.T, engine *Engine, worldDir string)
	}

	buildEngine := func(t *testing.T, sample sampleExpectation) *Engine {
		t.Helper()
		engine := newMultiCharacterTestEngine(t)
		root := t.TempDir()
		worldDir := filepath.Join(root, strings.ReplaceAll(strings.ToLower(sample.name), " ", "_"))
		copyDir(t, filepath.Join("..", "..", "worlds", sample.sourceDir), worldDir)

		engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
			"111": worldDir,
		})
		engine.worldPaths["111"] = worldDir
		engine.charWorlds["111"] = CharWorld{
			WorldName: sample.name,
			CoreRules: "真实世界目录 500 tick 长窗口稳定性验证",
			Scene:     sample.scene,
		}
		engine.focusCharacter = "111"
		engine.SyncActiveWorldContext()

		if sample.configure != nil {
			sample.configure(t, engine, worldDir)
		}
		return engine
	}

	samples := []sampleExpectation{
		{
			name:      "Neon Block Stability 500",
			sourceDir: "neon_block",
			scene: core.SceneState{
				Location:    "旧街夜市",
				TimeOfDay:   "深夜",
				Weather:     "闷热有雨",
				Characters:  []string{"蓝姐", "谭叔", "玩家"},
				Description: "真实世界目录中的默认夜市场景",
			},
			expectedPromotedNPC: "蓝姐",
			expectedPressureID:  "missing_rider",
			expectTensionFloor:  0.45,
		},
		{
			name:      "Wedding Import Stability 500",
			sourceDir: "1_7",
			scene: core.SceneState{
				Location:    "未知地点",
				TimeOfDay:   "白天",
				Weather:     "阴雨",
				Characters:  []string{"许灵_单阶段人设", "玩家"},
				Description: "真实导入世界的默认接站场景",
			},
			expectedPromotedNPC: "婚礼管家",
			expectedPressureID:  "arrival_gossip",
			expectTensionFloor:  0.50,
			configure: func(t *testing.T, engine *Engine, worldDir string) {
				t.Helper()
				if _, err := engine.UpdatePopulationConfig(core.PopulationConfig{
					BackgroundNPCs: []core.BackgroundNPC{
						{ID: "steward", Name: "婚礼管家", Role: "steward", Location: "未知地点", Faction: "wedding_hosts", Traits: []string{"周到", "急切"}, Hooks: []string{"要把迟到接站压下去", "不想婚礼前出乱子"}},
						{ID: "driver", Name: "代驾老周", Role: "driver", Location: "车站外", Faction: "station_runners", Traits: []string{"疲惫", "圆滑"}, Hooks: []string{"谁临时改了接站安排", "想把责任甩出去"}},
						{ID: "guard", Name: "站台保安", Role: "guard", Location: "候车厅", Faction: "station_runners", Traits: []string{"谨慎", "怕麻烦"}, Hooks: []string{"担心现场起争执", "不想事情闹大"}},
					},
					Policy: core.PromotionPolicy{PromoteThreshold: 6.8, MajorThreshold: 11, InteractionWeight: 3, MentionWeight: 1, EventWeight: 2, RelationshipWeight: 3, SceneWeight: 2},
				}); err != nil {
					t.Fatalf("UpdatePopulationConfig wedding import: %v", err)
				}
				if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
					Locations: []core.WorldLocationConfig{
						{ID: "arrival_point", Name: "未知地点", Kind: "arrival", Description: "婚礼接站与临时协调点", Controller: "wedding_hosts"},
						{ID: "station_gate", Name: "车站外", Kind: "transit", Description: "接站车与代驾聚集的混乱出口", Controller: "station_runners"},
						{ID: "platform_hall", Name: "候车厅", Kind: "waiting", Description: "旅客和保安都不想久留的大厅", Controller: "station_runners"},
					},
					Factions: []core.WorldFactionConfig{
						{ID: "wedding_hosts", Name: "婚礼主家", Role: "family", Relationships: []string{"压制 station_runners"}},
						{ID: "station_runners", Name: "接站跑腿圈", Role: "logistics", Relationships: []string{"不信任 wedding_hosts"}},
					},
					Pressures: []core.WorldPressureConfig{
						{ID: "pickup_delay", Name: "接站迟到", Kind: "coordination", Description: "婚礼前的接站安排持续失序", Intensity: 0.84, Target: "wedding_hosts"},
						{ID: "arrival_gossip", Name: "站台风声", Kind: "rumor", Description: "谁被怠慢、谁在甩锅开始扩散", Intensity: 0.62, Target: "未知地点"},
					},
				}); err != nil {
					t.Fatalf("UpdateWorldStructureConfig wedding import: %v", err)
				}
			},
		},
		{
			name:      "Dream Mansion Stability 500",
			sourceDir: "《红楼梦》完整版、-角色卡-202604190812",
			scene: core.SceneState{
				Location:    "未知地点",
				TimeOfDay:   "未知时间",
				Weather:     "未知天气",
				Characters:  []string{"薛宝钗", "玩家"},
				Description: "真实导入世界的默认闺阁场景",
			},
			expectedPromotedNPC: "莺儿",
			expectedPressureID:  "maids_whisper",
			expectTensionFloor:  0.48,
			configure: func(t *testing.T, engine *Engine, worldDir string) {
				t.Helper()
				if _, _, err := world.EnsureSeededPopulation(worldDir); err != nil {
					t.Fatalf("EnsureSeededPopulation dream mansion: %v", err)
				}
				if _, err := engine.UpdatePopulationConfig(core.PopulationConfig{
					BackgroundNPCs: []core.BackgroundNPC{
						{ID: "yinger", Name: "莺儿", Role: "侍女", Location: "未知地点", Faction: "xue_house", Traits: []string{"机灵", "知分寸"}, Hooks: []string{"替宝姑娘探听风声", "不想让诗社话头失控"}},
						{ID: "housemaid", Name: "婆子", Role: "杂役", Location: "回廊", Faction: "rong_house", Traits: []string{"谨慎", "嘴碎"}, Hooks: []string{"最怕传错话", "担心被责罚"}},
						{ID: "page", Name: "小厮", Role: "跑腿", Location: "书房外", Faction: "poetry_circle", Traits: []string{"轻快", "爱看热闹"}, Hooks: []string{"去回话", "把诗社消息带错边"}},
					},
					Policy: core.PromotionPolicy{PromoteThreshold: 6.8, MajorThreshold: 11, InteractionWeight: 3, MentionWeight: 1, EventWeight: 2, RelationshipWeight: 3, SceneWeight: 2},
				}); err != nil {
					t.Fatalf("UpdatePopulationConfig dream mansion: %v", err)
				}
				if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
					Locations: []core.WorldLocationConfig{
						{ID: "boudoir", Name: "未知地点", Kind: "residence", Description: "闺阁内室，消息传得不快却更要紧", Controller: "xue_house"},
						{ID: "corridor", Name: "回廊", Kind: "transit", Description: "丫鬟婆子擦身而过、最容易串话", Controller: "rong_house"},
						{ID: "study_gate", Name: "书房外", Kind: "service", Description: "回话与递帖都得经过的地方", Controller: "poetry_circle"},
					},
					Factions: []core.WorldFactionConfig{
						{ID: "xue_house", Name: "薛家房内", Role: "household", Relationships: []string{"顾忌 rong_house"}},
						{ID: "rong_house", Name: "荣府杂役", Role: "household", Relationships: []string{"议论 xue_house"}},
						{ID: "poetry_circle", Name: "诗社往来圈", Role: "social", Relationships: []string{"牵动 xue_house"}},
					},
					Pressures: []core.WorldPressureConfig{
						{ID: "poetry_society", Name: "诗社风声", Kind: "social", Description: "诗社流言让宝钗身边的人先紧张起来", Intensity: 0.81, Target: "xue_house"},
						{ID: "maids_whisper", Name: "回廊私语", Kind: "rumor", Description: "回廊里关于谁该出面的话越传越偏", Intensity: 0.58, Target: "未知地点"},
					},
				}); err != nil {
					t.Fatalf("UpdateWorldStructureConfig dream mansion: %v", err)
				}
			},
		},
		{
			name:      "Campus Villa Stability 500",
			sourceDir: "48111430a81be7d4",
			scene: core.SceneState{
				Location:    "别墅",
				TimeOfDay:   "白天",
				Weather:     "晴朗炎热",
				Characters:  []string{"赵小亮", "玩家", "沈佳"},
				Description: "真实世界目录中的校园别墅场景",
			},
			expectedPromotedNPC: "别墅巡守",
			expectedPressureID:  "family_secret",
			expectTensionFloor:  0.42,
			configure: func(t *testing.T, engine *Engine, worldDir string) {
				t.Helper()
				if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
					Locations: []core.WorldLocationConfig{
						{ID: "villa", Name: "别墅", Kind: "residence", Description: "三层建筑住宅，玩家与赵小亮的活动中心", Controller: "villa_family"},
						{ID: "school", Name: "明南高中", Kind: "school", Description: "半封闭式管理学校，设有高二4班", Controller: "school_faculty"},
						{ID: "village", Name: "上溪村", Kind: "rural", Description: "偏远乡下，玩家奶奶居住地", Controller: "village_locals"},
					},
					Factions: []core.WorldFactionConfig{
						{ID: "villa_family", Name: "别墅家庭", Role: "household", Relationships: []string{"顾忌 school_faculty"}},
						{ID: "school_faculty", Name: "明南高中教职工", Role: "authority", Relationships: []string{"监督 villa_family"}},
						{ID: "village_locals", Name: "上溪村村民", Role: "community", Relationships: []string{"远离 villa_family"}},
					},
					Pressures: []core.WorldPressureConfig{
						{ID: "family_secret", Name: "家庭秘密", Kind: "domestic", Description: "别墅内的异常关系引发暗流", Intensity: 0.75, Target: "villa_family"},
						{ID: "school_rumors", Name: "校园传闻", Kind: "rumor", Description: "明南高中关于玩家的传闻开始扩散", Intensity: 0.60, Target: "别墅"},
					},
				}); err != nil {
					t.Fatalf("UpdateWorldStructureConfig campus villa: %v", err)
				}
			},
		},
		{
			name:      "Streaming Penthouse Stability 500",
			sourceDir: "a0c85d27e38863a4",
			scene: core.SceneState{
				Location:    "客厅",
				TimeOfDay:   "夜晚",
				Weather:     "阴雨",
				Characters:  []string{"ANJONI小玖", "玩家"},
				Description: "真实世界目录中的直播顶层场景",
			},
			expectedPromotedNPC: "客厅巡守",
			expectedPressureID:  "platform_riot",
			expectTensionFloor:  0.42,
			configure: func(t *testing.T, engine *Engine, worldDir string) {
				t.Helper()
				if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
					Locations: []core.WorldLocationConfig{
						{ID: "penthouse", Name: "客厅", Kind: "residence", Description: "汤臣一品顶层大平层，直播与社交中心", Controller: "streamer_team"},
						{ID: "stream_room", Name: "直播间", Kind: "workspace", Description: "专业直播设备与拍摄区域", Controller: "streamer_team"},
						{ID: "backstage", Name: "后台", Kind: "logistics", Description: "运营团队与来访者等候区", Controller: "rival_streamers"},
					},
					Factions: []core.WorldFactionConfig{
						{ID: "streamer_team", Name: "主播团队", Role: "content", Relationships: []string{"防范 rival_streamers"}},
						{ID: "rival_streamers", Name: "竞品主播", Role: "competition", Relationships: []string{"觊觎 streamer_team 流量"}},
					},
					Pressures: []core.WorldPressureConfig{
						{ID: "platform_riot", Name: "平台风波", Kind: "crisis", Description: "斗鱼平台政策变动引发主播圈动荡", Intensity: 0.78, Target: "streamer_team"},
						{ID: "stream_rivalry", Name: "直播竞争", Kind: "competition", Description: "竞品主播暗中挖角与流量争夺", Intensity: 0.65, Target: "客厅"},
					},
				}); err != nil {
					t.Fatalf("UpdateWorldStructureConfig streaming penthouse: %v", err)
				}
			},
		},
	}

	results := make(map[string]map[string]interface{}, len(samples))
	for _, sample := range samples {
		engine := buildEngine(t, sample)
		for i := 0; i < 500; i++ {
			engine.onTick()
		}

		status := engine.TickStatus()
		trajectory, ok := status["trajectory_summary"].([]string)
		if !ok || len(trajectory) == 0 {
			t.Fatalf("%s trajectory summary = %#v, want long-window summary from 500-tick stability test", sample.name, status["trajectory_summary"])
		}
		joinedTrajectory := strings.Join(trajectory, " | ")
		history, ok := status["tick_history"].([]core.TickSnapshot)
		if !ok || len(history) != 12 {
			t.Fatalf("%s tick history = %#v, want capped recent snapshots after 500 ticks", sample.name, status["tick_history"])
		}
		insights, err := engine.GetPopulationInsights()
		if err != nil {
			t.Fatalf("%s GetPopulationInsights: %v", sample.name, err)
		}
		promotedNames := make([]string, 0, len(insights.Promoted))
		for _, npc := range insights.Promoted {
			promotedNames = append(promotedNames, npc.Name)
		}
		sort.Strings(promotedNames)

		if tension, _ := status["tension"].(float64); tension < sample.expectTensionFloor {
			t.Fatalf("%s tension = %#v, want >= %.2f from 500-tick stability test", sample.name, status["tension"], sample.expectTensionFloor)
		}
		if !containsString(promotedNames, sample.expectedPromotedNPC) {
			t.Fatalf("%s promoted = %#v, want %s promoted from 500-tick stability test", sample.name, promotedNames, sample.expectedPromotedNPC)
		}
		if !strings.Contains(joinedTrajectory, sample.expectedPressureID) {
			t.Fatalf("%s trajectory = %q, want dominant pressure %s from 500-tick stability test", sample.name, joinedTrajectory, sample.expectedPressureID)
		}

		results[sample.name] = map[string]interface{}{
			"trajectory": joinedTrajectory,
			"promoted":   strings.Join(promotedNames, ","),
			"tension":    status["tension"].(float64),
		}
	}

	if results["Neon Block Stability 500"]["trajectory"] == results["Wedding Import Stability 500"]["trajectory"] {
		t.Fatalf("neon vs wedding 500-tick stability trajectory = %#v vs %#v, want divergent summaries", results["Neon Block Stability 500"], results["Wedding Import Stability 500"])
	}
	if results["Wedding Import Stability 500"]["promoted"] == results["Dream Mansion Stability 500"]["promoted"] {
		t.Fatalf("wedding vs dream 500-tick stability promoted = %#v vs %#v, want different leaders", results["Wedding Import Stability 500"], results["Dream Mansion Stability 500"])
	}
}

func TestPopulationIdentityShiftAccumulates(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "shift-world")
	writeTestWorldBundle(t, worldDir, "漂移世界", "关系会随事件累积", core.SceneState{
		Location:    "酒馆",
		TimeOfDay:   "夜晚",
		Weather:     "晴",
		Characters:  []string{"111", "玩家"},
		Description: "酒馆里灯光昏暗",
	})

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.worldPaths["111"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "漂移世界",
		CoreRules: "关系会随事件累积",
		Scene: core.SceneState{
			Location:    "酒馆",
			TimeOfDay:   "夜晚",
			Weather:     "晴",
			Characters:  []string{"111", "玩家"},
			Description: "酒馆里灯光昏暗",
		},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()

	_, err := engine.UpdatePopulationConfig(core.PopulationConfig{
		BackgroundNPCs: []core.BackgroundNPC{{
			ID:       "bartender",
			Name:     "酒保",
			Role:     "服务生",
			Location: "酒馆",
			Traits:   []string{"沉默寡言"},
			Hooks:    []string{"见过太多秘密"},
		}},
		Policy: core.PromotionPolicy{
			PromoteThreshold:   5,
			MajorThreshold:     15,
			InteractionWeight:  3,
			MentionWeight:      1,
			EventWeight:        2,
			RelationshipWeight: 3,
			SceneWeight:        1,
		},
	})
	if err != nil {
		t.Fatalf("UpdatePopulationConfig: %v", err)
	}

	now := time.Now().UTC()
	events := []core.Event{
		{ID: "shift_1", Type: "dialogue", Actor: "酒保", Target: "111", Payload: map[string]interface{}{"content": "酒保低声说了一句"}, SceneID: "酒馆", Canonical: true, CreatedAt: now},
		{ID: "shift_2", Type: "trust_change", Actor: "111", Target: "酒保", Payload: map[string]interface{}{"delta": 2.0}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(time.Second)},
		{ID: "shift_3", Type: "trust_change", Actor: "111", Target: "酒保", Payload: map[string]interface{}{"delta": 1.5}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(2 * time.Second)},
		{ID: "shift_4", Type: "fear_change", Actor: "111", Target: "酒保", Payload: map[string]interface{}{"delta": 0.8}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(3 * time.Second)},
	}
	for _, evt := range events {
		if err := engine.eventStore.Append(evt); err != nil {
			t.Fatalf("append event %s: %v", evt.ID, err)
		}
	}

	engine.reconcilePopulationLocked()

	population, err := engine.GetPopulationConfig()
	if err != nil {
		t.Fatalf("GetPopulationConfig: %v", err)
	}
	if len(population.PromotedNPCs) != 1 {
		t.Fatalf("promoted npcs = %#v, want 1", population.PromotedNPCs)
	}
	if len(population.IdentityCores) != 1 {
		t.Fatalf("identity cores = %#v, want 1", population.IdentityCores)
	}

	core := population.IdentityCores[0]
	if core.Adaptive["trust"] <= 3 {
		t.Fatalf("identity core trust = %.2f, want > 3 after multiple trust_change events", core.Adaptive["trust"])
	}
	if core.Adaptive["fear"] <= 2 {
		t.Fatalf("identity core fear = %.2f, want > 2 after fear_change event", core.Adaptive["fear"])
	}

	char, ok := engine.agents.GetCharacter("酒保")
	if !ok {
		t.Fatalf("promoted runtime character not loaded")
	}
	if char.Identity.Adaptive["trust"] != core.Adaptive["trust"] {
		t.Fatalf("runtime character trust = %.2f, want %.2f (synced with identity core)", char.Identity.Adaptive["trust"], core.Adaptive["trust"])
	}
}

func TestPopulationIdentityShiftChangesFutureAllowedActions(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "shift-behavior-world")
	writeTestWorldBundle(t, worldDir, "行为漂移世界", "成长后应改变未来行为", core.SceneState{
		Location:    "酒馆",
		TimeOfDay:   "夜晚",
		Weather:     "晴",
		Characters:  []string{"111", "玩家"},
		Description: "酒馆里消息很多",
	})

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.worldPaths["111"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "行为漂移世界",
		CoreRules: "成长后应改变未来行为",
		Scene: core.SceneState{
			Location:    "酒馆",
			TimeOfDay:   "夜晚",
			Weather:     "晴",
			Characters:  []string{"111", "玩家"},
			Description: "酒馆里消息很多",
		},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()

	_, err := engine.UpdatePopulationConfig(core.PopulationConfig{
		BackgroundNPCs: []core.BackgroundNPC{{
			ID:       "bartender",
			Name:     "酒保",
			Role:     "服务生",
			Location: "酒馆",
			Traits:   []string{"沉默寡言"},
			Hooks:    []string{"见过太多秘密"},
		}},
		Policy: core.PromotionPolicy{
			PromoteThreshold:   4,
			MajorThreshold:     12,
			InteractionWeight:  3,
			MentionWeight:      1,
			EventWeight:        2,
			RelationshipWeight: 3,
			SceneWeight:        1,
		},
	})
	if err != nil {
		t.Fatalf("UpdatePopulationConfig: %v", err)
	}

	now := time.Now().UTC()
	initialEvents := []core.Event{
		{ID: "behavior_1", Type: "dialogue", Actor: "酒保", Target: "111", Payload: map[string]interface{}{"content": "酒保低声提醒你今晚别惹事"}, SceneID: "酒馆", Canonical: true, CreatedAt: now},
		{ID: "behavior_2", Type: "user_message", Actor: "user", Target: "111", Payload: map[string]interface{}{"content": "我先听酒保怎么说"}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(time.Second)},
	}
	for _, evt := range initialEvents {
		if err := engine.eventStore.Append(evt); err != nil {
			t.Fatalf("append initial event %s: %v", evt.ID, err)
		}
	}

	engine.reconcilePopulationLocked()

	beforeChar, ok := engine.agents.GetCharacter("酒保")
	if !ok {
		t.Fatalf("promoted runtime character not loaded after first reconcile")
	}
	beforeTrust := beforeChar.Identity.Adaptive["trust"]
	baseAllowed := filterAllowedActionsForStep(core.TurnStep{Kind: "tension_response"}, []string{"speak", "trust", "negotiate", "hide", "move", "threaten", "attack"})
	beforeActions := filterAllowedActionsForAdaptive(baseAllowed, beforeChar.Identity.Adaptive)
	if !containsString(beforeActions, "attack") || !containsString(beforeActions, "threaten") {
		t.Fatalf("before actions = %v, want aggressive options still available before growth", beforeActions)
	}

	shiftEvents := []core.Event{
		{ID: "behavior_3", Type: "trust_change", Actor: "111", Target: "酒保", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(2 * time.Second)},
		{ID: "behavior_4", Type: "trust_change", Actor: "111", Target: "酒保", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(3 * time.Second)},
		{ID: "behavior_5", Type: "trust_change", Actor: "111", Target: "酒保", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(4 * time.Second)},
		{ID: "behavior_6", Type: "trust_change", Actor: "111", Target: "酒保", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(5 * time.Second)},
		{ID: "behavior_7", Type: "trust_change", Actor: "111", Target: "酒保", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(6 * time.Second)},
	}
	for _, evt := range shiftEvents {
		if err := engine.eventStore.Append(evt); err != nil {
			t.Fatalf("append shift event %s: %v", evt.ID, err)
		}
	}

	engine.reconcilePopulationLocked()

	afterChar, ok := engine.agents.GetCharacter("酒保")
	if !ok {
		t.Fatalf("runtime character missing after identity shift")
	}
	if afterChar.Identity.Adaptive["trust"] <= beforeTrust || afterChar.Identity.Adaptive["trust"] < 7 {
		t.Fatalf("after trust = %.2f, want > %.2f and >= 7", afterChar.Identity.Adaptive["trust"], beforeTrust)
	}
	afterActions := filterAllowedActionsForAdaptive(baseAllowed, afterChar.Identity.Adaptive)
	if containsString(afterActions, "attack") || containsString(afterActions, "threaten") {
		t.Fatalf("after actions = %v, want aggressive actions removed after high-trust growth", afterActions)
	}

	insights, err := engine.GetPopulationInsights()
	if err != nil {
		t.Fatalf("GetPopulationInsights: %v", err)
	}
	foundShiftHistory := false
	for _, npc := range insights.Promoted {
		if npc.Name != "酒保" {
			continue
		}
		for _, item := range npc.History {
			if item.Type == "population_identity_shift" && len(item.Adaptive) > 0 {
				foundShiftHistory = true
				break
			}
		}
	}
	if !foundShiftHistory {
		t.Fatalf("promoted insights = %#v, want identity shift history for 酒保", insights.Promoted)
	}
}

func TestPopulationIdentityShiftChangesDirectorWinner(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "shift-director-world")
	writeTestWorldBundle(t, worldDir, "选人漂移世界", "成长后应改变谁来上场", core.SceneState{
		Location:    "酒馆",
		TimeOfDay:   "夜晚",
		Weather:     "晴",
		Characters:  []string{"安雅", "玩家"},
		Description: "酒馆里每个人都在观察彼此",
	})

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
		"安雅":  worldDir,
	})
	engine.worldPaths["111"] = worldDir
	engine.worldPaths["安雅"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "选人漂移世界",
		CoreRules: "成长后应改变谁来上场",
		Scene: core.SceneState{
			Location:    "酒馆",
			TimeOfDay:   "夜晚",
			Weather:     "晴",
			Characters:  []string{"111", "玩家"},
			Description: "111 的个人场景",
		},
	}
	engine.charWorlds["安雅"] = CharWorld{
		WorldName: "选人漂移世界",
		CoreRules: "成长后应改变谁来上场",
		Scene: core.SceneState{
			Location:    "酒馆",
			TimeOfDay:   "夜晚",
			Weather:     "晴",
			Characters:  []string{"安雅", "玩家"},
			Description: "安雅暂时主视角",
		},
	}
	engine.focusCharacter = "安雅"
	engine.SyncActiveWorldContext()

	_, err := engine.UpdatePopulationConfig(core.PopulationConfig{
		BackgroundNPCs: []core.BackgroundNPC{{
			ID:       "bartender",
			Name:     "酒保",
			Role:     "服务生",
			Location: "后厨",
			Traits:   []string{"沉默寡言"},
			Hooks:    []string{"知道很多秘密"},
		}},
		Policy: core.PromotionPolicy{
			PromoteThreshold:   4,
			MajorThreshold:     12,
			InteractionWeight:  3,
			MentionWeight:      1,
			EventWeight:        2,
			RelationshipWeight: 3,
			SceneWeight:        1,
		},
	})
	if err != nil {
		t.Fatalf("UpdatePopulationConfig: %v", err)
	}

	now := time.Now().UTC()
	for _, evt := range []core.Event{
		{ID: "winner_1", Type: "dialogue", Actor: "酒保", Target: "安雅", Payload: map[string]interface{}{"content": "酒保只是简短应了一声"}, SceneID: "酒馆", Canonical: true, CreatedAt: now},
		{ID: "winner_2", Type: "user_message", Actor: "user", Target: "安雅", Payload: map[string]interface{}{"content": "先听听周围人怎么想"}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(time.Second)},
	} {
		if err := engine.eventStore.Append(evt); err != nil {
			t.Fatalf("append seed event %s: %v", evt.ID, err)
		}
	}

	engine.reconcilePopulationLocked()
	engine.UpdateDirectorConfig(core.DirectorConfig{
		Mode:        "auto_single",
		MaxSpeakers: 1,
		Weights: map[string]float64{
			"trust":             10,
			"continuity":        0.01,
			"present":           0.01,
			"kind_persona":      0.01,
			"kind_npc":          0.01,
			"source_promoted":   4,
			"source_definition": 0.01,
			"loaded":            0.01,
			"location_match":    0.01,
			"faction_match":     0.01,
			"pressure_match":    0.01,
			"hook_match":        0.01,
			"mentioned":         0.01,
			"mention_order":     0.01,
			"opened_by_user":    0.01,
			"tension_switch":    0.01,
			"silence_cap":       0.01,
			"silence_divisor":   9999,
			"intimacy":          0.01,
			"fear":              0.01,
		},
	})

	engine.mu.Lock()
	beforePlan := engine.directTurnLocked("", engine.stateMgr.Get())
	engine.mu.Unlock()
	if len(beforePlan.Selected) == 0 || beforePlan.Selected[0] != "111" {
		t.Fatalf("before selected = %#v, want 111 to win before bartender growth", beforePlan.Selected)
	}
	var beforeBartender *core.DirectorCandidate
	for i := range beforePlan.CandidateDetails {
		if beforePlan.CandidateDetails[i].Name == "酒保" {
			beforeBartender = &beforePlan.CandidateDetails[i]
			break
		}
	}
	if beforeBartender == nil {
		t.Fatalf("before candidate details = %#v, want 酒保 present", beforePlan.CandidateDetails)
	}

	for _, evt := range []core.Event{
		{ID: "winner_3", Type: "trust_change", Actor: "安雅", Target: "酒保", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(2 * time.Second)},
		{ID: "winner_4", Type: "trust_change", Actor: "安雅", Target: "酒保", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(3 * time.Second)},
		{ID: "winner_5", Type: "trust_change", Actor: "安雅", Target: "酒保", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(4 * time.Second)},
		{ID: "winner_6", Type: "trust_change", Actor: "安雅", Target: "酒保", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(5 * time.Second)},
		{ID: "winner_7", Type: "trust_change", Actor: "安雅", Target: "酒保", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(6 * time.Second)},
	} {
		if err := engine.eventStore.Append(evt); err != nil {
			t.Fatalf("append trust event %s: %v", evt.ID, err)
		}
	}

	engine.reconcilePopulationLocked()
	engine.mu.Lock()
	afterPlan := engine.directTurnLocked("", engine.stateMgr.Get())
	engine.mu.Unlock()
	if len(afterPlan.Selected) == 0 || afterPlan.Selected[0] != "酒保" {
		t.Fatalf("after selected = %#v, want 酒保 to win after trust growth", afterPlan.Selected)
	}
	var afterBartender *core.DirectorCandidate
	for i := range afterPlan.CandidateDetails {
		if afterPlan.CandidateDetails[i].Name == "酒保" {
			afterBartender = &afterPlan.CandidateDetails[i]
			break
		}
	}
	if afterBartender == nil {
		t.Fatalf("after candidate details = %#v, want 酒保 present", afterPlan.CandidateDetails)
	}
	if afterBartender.Score <= beforeBartender.Score {
		t.Fatalf("bartender score before/after = %.2f/%.2f, want growth to raise director score", beforeBartender.Score, afterBartender.Score)
	}
	if afterBartender.ScoreBreakdown["trust"] <= beforeBartender.ScoreBreakdown["trust"] {
		t.Fatalf("bartender trust breakdown before/after = %.2f/%.2f, want trust to drive director shift", beforeBartender.ScoreBreakdown["trust"], afterBartender.ScoreBreakdown["trust"])
	}
	if !containsString(afterBartender.DominantFactors, "信任倾向") {
		t.Fatalf("after dominant factors = %#v, want 信任倾向 included", afterBartender.DominantFactors)
	}
}

func TestPopulationIdentityShiftRefreshesDesiresAndAutonomousIntent(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "shift-desire-world")
	writeTestWorldBundle(t, worldDir, "欲望漂移世界", "成长后欲望和自治意图应改变", core.SceneState{
		Location:    "酒馆",
		TimeOfDay:   "夜晚",
		Weather:     "晴",
		Characters:  []string{"111", "玩家"},
		Description: "酒馆里每个人都在观察彼此",
	})

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.worldPaths["111"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "欲望漂移世界",
		CoreRules: "成长后欲望和自治意图应改变",
		Scene: core.SceneState{
			Location:    "酒馆",
			TimeOfDay:   "夜晚",
			Weather:     "晴",
			Characters:  []string{"111", "玩家"},
			Description: "酒馆里每个人都在观察彼此",
		},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()

	_, err := engine.UpdatePopulationConfig(core.PopulationConfig{
		BackgroundNPCs: []core.BackgroundNPC{{
			ID:       "bartender",
			Name:     "酒保",
			Role:     "服务生",
			Location: "酒馆",
			Traits:   []string{"冷静", "独立"},
			Hooks:    []string{"知道很多秘密"},
		}},
		Policy: core.PromotionPolicy{
			PromoteThreshold:   4,
			MajorThreshold:     12,
			InteractionWeight:  3,
			MentionWeight:      1,
			EventWeight:        2,
			RelationshipWeight: 3,
			SceneWeight:        1,
		},
	})
	if err != nil {
		t.Fatalf("UpdatePopulationConfig: %v", err)
	}

	now := time.Now().UTC()
	for _, evt := range []core.Event{
		{ID: "desire_1", Type: "dialogue", Actor: "酒保", Target: "111", Payload: map[string]interface{}{"content": "酒保只是淡淡看了你一眼"}, SceneID: "酒馆", Canonical: true, CreatedAt: now},
		{ID: "desire_2", Type: "user_message", Actor: "user", Target: "111", Payload: map[string]interface{}{"content": "让酒保先说说"}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(time.Second)},
	} {
		if err := engine.eventStore.Append(evt); err != nil {
			t.Fatalf("append seed event %s: %v", evt.ID, err)
		}
	}

	engine.reconcilePopulationLocked()

	beforeChar, ok := engine.agents.GetCharacter("酒保")
	if !ok {
		t.Fatalf("promoted runtime character not loaded")
	}
	beforeDesires, err := engine.desireStore.GetByCharacter("酒保")
	if err != nil {
		t.Fatalf("GetByCharacter before: %v", err)
	}
	if len(beforeDesires) == 0 || beforeDesires[0].Type != emotion.DesireAutonomy {
		t.Fatalf("before desires = %#v, want autonomy-dominant baseline", beforeDesires)
	}
	beforeAction := emotion.GenerateAutonomousAction(
		"酒保",
		emotion.EmotionalPressure{Total: 0.9},
		beforeDesires,
		emotionVectorFromAdaptive(beforeChar.Identity.Adaptive),
		nil,
		0,
	)
	if beforeAction == nil || beforeAction.ActionType != "withdraw" {
		t.Fatalf("before autonomous action = %#v, want withdraw from autonomy baseline", beforeAction)
	}

	for _, evt := range []core.Event{
		{ID: "desire_3", Type: "trust_change", Actor: "111", Target: "酒保", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(2 * time.Second)},
		{ID: "desire_4", Type: "trust_change", Actor: "111", Target: "酒保", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(3 * time.Second)},
		{ID: "desire_5", Type: "trust_change", Actor: "111", Target: "酒保", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(4 * time.Second)},
		{ID: "desire_6", Type: "trust_change", Actor: "111", Target: "酒保", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(5 * time.Second)},
		{ID: "desire_7", Type: "trust_change", Actor: "111", Target: "酒保", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "酒馆", Canonical: true, CreatedAt: now.Add(6 * time.Second)},
	} {
		if err := engine.eventStore.Append(evt); err != nil {
			t.Fatalf("append trust event %s: %v", evt.ID, err)
		}
	}

	engine.reconcilePopulationLocked()

	afterChar, ok := engine.agents.GetCharacter("酒保")
	if !ok {
		t.Fatalf("runtime character missing after trust growth")
	}
	if afterChar.Identity.Adaptive["trust"] < 7 {
		t.Fatalf("after trust = %.2f, want >= 7", afterChar.Identity.Adaptive["trust"])
	}
	afterDesires, err := engine.desireStore.GetByCharacter("酒保")
	if err != nil {
		t.Fatalf("GetByCharacter after: %v", err)
	}
	if len(afterDesires) == 0 {
		t.Fatalf("after desires = %#v, want refreshed desires", afterDesires)
	}
	foundAffection := false
	for _, desire := range afterDesires {
		if desire.Type == emotion.DesireAffection {
			foundAffection = true
			break
		}
	}
	if !foundAffection {
		t.Fatalf("after desires = %#v, want affection desire after trust growth", afterDesires)
	}
	afterAction := emotion.GenerateAutonomousAction(
		"酒保",
		emotion.EmotionalPressure{Total: 0.9},
		afterDesires,
		emotionVectorFromAdaptive(afterChar.Identity.Adaptive),
		nil,
		0,
	)
	if afterAction == nil || afterAction.ActionType != "approach" {
		t.Fatalf("after autonomous action = %#v, want approach after affection refresh", afterAction)
	}
}

func TestIdentityShiftChangesSchedulerActionAndRelationshipOutcome(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "shift-relationship-world")
	writeTestWorldBundle(t, worldDir, "关系漂移世界", "成长后应改变关系走向", core.SceneState{
		Location:    "外城",
		TimeOfDay:   "深夜",
		Weather:     "阴",
		Characters:  []string{"111", "玩家"},
		Description: "外城对峙一触即发",
	})

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.worldPaths["111"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "关系漂移世界",
		CoreRules: "成长后应改变关系走向",
		Scene: core.SceneState{
			Location:    "外城",
			TimeOfDay:   "深夜",
			Weather:     "阴",
			Characters:  []string{"111", "玩家"},
			Description: "外城对峙一触即发",
		},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()
	engine.scheduler.SetActionInterval(0)
	engine.scheduler.SetRandomStepChance(0)

	engine.agents.LoadCharacter("smugglers", core.Character{
		WorldPath: worldDir,
		Identity: core.IdentityEnvelope{
			Name:      "smugglers",
			Adaptive:  map[string]float64{"trust": 3},
			Immutable: []string{"wary"},
		},
	})
	if !containsString(engine.loadedCharacters, "smugglers") {
		engine.loadedCharacters = append(engine.loadedCharacters, "smugglers")
	}
	engine.worldPaths["smugglers"] = worldDir
	engine.charWorlds["smugglers"] = CharWorld{
		WorldName: "关系漂移世界",
		CoreRules: "成长后应改变关系走向",
		Scene: core.SceneState{
			Location:    "外城",
			TimeOfDay:   "深夜",
			Weather:     "阴",
			Characters:  []string{"guard", "smugglers"},
			Description: "走私帮在外城活动",
		},
	}

	if _, err := engine.UpdatePopulationConfig(core.PopulationConfig{
		BackgroundNPCs: []core.BackgroundNPC{{
			ID:       "guard",
			Name:     "guard",
			Role:     "guard",
			Location: "外城",
			Faction:  "guard",
			Traits:   []string{"固执", "独立"},
			Hooks:    []string{"守住防线"},
		}},
		Policy: core.PromotionPolicy{
			PromoteThreshold:   4,
			MajorThreshold:     12,
			InteractionWeight:  3,
			MentionWeight:      1,
			EventWeight:        2,
			RelationshipWeight: 3,
			SceneWeight:        1,
		},
	}); err != nil {
		t.Fatalf("UpdatePopulationConfig: %v", err)
	}
	if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
		Factions: []core.WorldFactionConfig{
			{ID: "guard", Name: "guard"},
			{ID: "smugglers", Name: "smugglers"},
		},
		Locations: []core.WorldLocationConfig{
			{ID: "outer_city", Name: "外城", Controller: "guard"},
		},
	}); err != nil {
		t.Fatalf("UpdateWorldStructureConfig: %v", err)
	}

	now := time.Now().UTC()
	for _, evt := range []core.Event{
		{ID: "rel_1", Type: "dialogue", Actor: "guard", Target: "111", Payload: map[string]interface{}{"content": "守卫正盯着街口"}, SceneID: "外城", Canonical: true, CreatedAt: now},
		{ID: "rel_2", Type: "user_message", Actor: "user", Target: "111", Payload: map[string]interface{}{"content": "先看守卫怎么反应"}, SceneID: "外城", Canonical: true, CreatedAt: now.Add(time.Second)},
	} {
		if err := engine.eventStore.Append(evt); err != nil {
			t.Fatalf("append seed event %s: %v", evt.ID, err)
		}
	}

	engine.reconcilePopulationLocked()
	engine.agents.LoadCharacter("guard", core.Character{
		WorldPath: worldDir,
		Identity: core.IdentityEnvelope{
			Name:      "guard",
			Adaptive:  map[string]float64{"trust": 2, "aggression": 6, "fear": 2},
			Immutable: []string{"固执", "独立"},
		},
	})
	engine.charWorlds["guard"] = CharWorld{
		WorldName: "轨迹分叉世界",
		CoreRules: "成长后应持续改变关系轨迹",
		Scene: core.SceneState{
			Location:    "外城",
			TimeOfDay:   "深夜",
			Weather:     "阴",
			Characters:  []string{"guard", "smugglers"},
			Description: "守卫与走私帮持续对峙",
		},
	}
	engine.stateMgr.Set(core.WorldState{
		Scene: core.SceneState{
			Location:   "外城",
			Characters: []string{"111", "guard", "smugglers"},
		},
		Relationships: map[string]core.Relationship{},
		Variables:     map[string]interface{}{},
		Flags:         map[string]bool{},
	})

	engine.onTick()
	beforeLogs := engine.scheduler.RecentActionsForCharacter("guard", 0)
	if len(beforeLogs) == 0 || beforeLogs[len(beforeLogs)-1].Action == "trust" {
		t.Fatalf("before logs = %#v, want pre-shift action to remain non-trust", beforeLogs)
	}

	for _, evt := range []core.Event{
		{ID: "rel_3", Type: "trust_change", Actor: "111", Target: "guard", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "外城", Canonical: true, CreatedAt: now.Add(2 * time.Second)},
		{ID: "rel_4", Type: "trust_change", Actor: "111", Target: "guard", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "外城", Canonical: true, CreatedAt: now.Add(3 * time.Second)},
		{ID: "rel_5", Type: "trust_change", Actor: "111", Target: "guard", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "外城", Canonical: true, CreatedAt: now.Add(4 * time.Second)},
		{ID: "rel_6", Type: "trust_change", Actor: "111", Target: "guard", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "外城", Canonical: true, CreatedAt: now.Add(5 * time.Second)},
		{ID: "rel_7", Type: "trust_change", Actor: "111", Target: "guard", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "外城", Canonical: true, CreatedAt: now.Add(6 * time.Second)},
	} {
		if err := engine.eventStore.Append(evt); err != nil {
			t.Fatalf("append growth event %s: %v", evt.ID, err)
		}
	}

	engine.reconcilePopulationLocked()
	engine.stateMgr.Set(core.WorldState{
		Scene: core.SceneState{
			Location:   "外城",
			Characters: []string{"111", "guard", "smugglers"},
		},
		Relationships: map[string]core.Relationship{
			"guard_smugglers": {Trust: -1},
		},
		Variables: map[string]interface{}{},
		Flags:     map[string]bool{},
	})

	engine.onTick()
	afterLogs := engine.scheduler.RecentActionsForCharacter("guard", 0)
	if len(afterLogs) < 2 || afterLogs[len(afterLogs)-1].Action != "trust" {
		t.Fatalf("after logs = %#v, want trust after identity shift", afterLogs)
	}
	afterState := engine.GetState()
	if got := afterState.Relationships["guard_smugglers"].Trust; got <= -1 {
		t.Fatalf("relationship trust = %.2f, want immediate improvement after trust action", got)
	}
}

func TestIdentityShiftSustainsDivergentRelationshipTrajectoryAcrossTicks(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "trajectory-world")
	writeTestWorldBundle(t, worldDir, "轨迹分叉世界", "成长后应持续改变关系轨迹", core.SceneState{
		Location:    "外城",
		TimeOfDay:   "深夜",
		Weather:     "阴",
		Characters:  []string{"111", "玩家"},
		Description: "外城对峙持续升级",
	})

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.worldPaths["111"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "轨迹分叉世界",
		CoreRules: "成长后应持续改变关系轨迹",
		Scene: core.SceneState{
			Location:    "外城",
			TimeOfDay:   "深夜",
			Weather:     "阴",
			Characters:  []string{"111", "玩家"},
			Description: "外城对峙持续升级",
		},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()
	engine.scheduler.SetActionInterval(0)
	engine.scheduler.SetRandomStepChance(0)

	engine.agents.LoadCharacter("smugglers", core.Character{
		WorldPath: worldDir,
		Identity: core.IdentityEnvelope{
			Name:      "smugglers",
			Adaptive:  map[string]float64{"trust": 3},
			Immutable: []string{"wary"},
		},
	})
	if !containsString(engine.loadedCharacters, "smugglers") {
		engine.loadedCharacters = append(engine.loadedCharacters, "smugglers")
	}
	engine.worldPaths["smugglers"] = worldDir
	engine.charWorlds["smugglers"] = CharWorld{
		WorldName: "轨迹分叉世界",
		CoreRules: "成长后应持续改变关系轨迹",
		Scene: core.SceneState{
			Location:    "外城",
			TimeOfDay:   "深夜",
			Weather:     "阴",
			Characters:  []string{"guard", "smugglers"},
			Description: "走私帮在外城活动",
		},
	}

	if _, err := engine.UpdatePopulationConfig(core.PopulationConfig{
		BackgroundNPCs: []core.BackgroundNPC{{
			ID:       "guard",
			Name:     "guard",
			Role:     "guard",
			Location: "外城",
			Faction:  "guard",
			Traits:   []string{"固执", "独立"},
			Hooks:    []string{"守住防线"},
		}},
		Policy: core.PromotionPolicy{
			PromoteThreshold:   4,
			MajorThreshold:     12,
			InteractionWeight:  3,
			MentionWeight:      1,
			EventWeight:        2,
			RelationshipWeight: 3,
			SceneWeight:        1,
		},
	}); err != nil {
		t.Fatalf("UpdatePopulationConfig: %v", err)
	}
	if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
		Factions: []core.WorldFactionConfig{
			{ID: "guard", Name: "guard"},
			{ID: "smugglers", Name: "smugglers"},
		},
		Locations: []core.WorldLocationConfig{
			{ID: "outer_city", Name: "外城", Controller: "guard"},
		},
	}); err != nil {
		t.Fatalf("UpdateWorldStructureConfig: %v", err)
	}

	now := time.Now().UTC()
	for _, evt := range []core.Event{
		{ID: "traj_1", Type: "dialogue", Actor: "guard", Target: "111", Payload: map[string]interface{}{"content": "守卫在街口来回巡查"}, SceneID: "外城", Canonical: true, CreatedAt: now},
		{ID: "traj_2", Type: "user_message", Actor: "user", Target: "111", Payload: map[string]interface{}{"content": "继续观察守卫的行动"}, SceneID: "外城", Canonical: true, CreatedAt: now.Add(time.Second)},
		{ID: "traj_rel_0", Type: "trust_change", Actor: "guard", Target: "smugglers", Payload: map[string]interface{}{"delta": -1.0}, SceneID: "外城", Canonical: true, CreatedAt: now.Add(1500 * time.Millisecond)},
	} {
		if err := engine.eventStore.Append(evt); err != nil {
			t.Fatalf("append seed event %s: %v", evt.ID, err)
		}
	}

	engine.reconcilePopulationLocked()
	engine.agents.LoadCharacter("guard", core.Character{
		WorldPath: worldDir,
		Identity: core.IdentityEnvelope{
			Name:      "guard",
			Adaptive:  map[string]float64{"trust": 2, "aggression": 6, "fear": 2},
			Immutable: []string{"固执", "独立"},
		},
	})
	engine.stateMgr.Set(core.WorldState{
		Scene: core.SceneState{
			Location:   "外城",
			Characters: []string{"111", "guard", "smugglers"},
		},
		Relationships: map[string]core.Relationship{
			"guard_smugglers": {Trust: -1},
		},
		Variables: map[string]interface{}{},
		Flags:     map[string]bool{},
	})

	preShiftTrust := make([]float64, 0, 2)
	for i := 0; i < 2; i++ {
		engine.onTick()
		preShiftTrust = append(preShiftTrust, engine.GetState().Relationships["guard_smugglers"].Trust)
	}
	preShiftLogs := engine.scheduler.RecentActionsForCharacter("guard", 0)
	preShiftTrustActions := 0
	for _, log := range preShiftLogs {
		if log.Action == "trust" {
			preShiftTrustActions++
		}
	}

	for _, evt := range []core.Event{
		{ID: "traj_3", Type: "trust_change", Actor: "111", Target: "guard", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "外城", Canonical: true, CreatedAt: now.Add(2 * time.Second)},
		{ID: "traj_4", Type: "trust_change", Actor: "111", Target: "guard", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "外城", Canonical: true, CreatedAt: now.Add(3 * time.Second)},
		{ID: "traj_5", Type: "trust_change", Actor: "111", Target: "guard", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "外城", Canonical: true, CreatedAt: now.Add(4 * time.Second)},
		{ID: "traj_6", Type: "trust_change", Actor: "111", Target: "guard", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "外城", Canonical: true, CreatedAt: now.Add(5 * time.Second)},
		{ID: "traj_7", Type: "trust_change", Actor: "111", Target: "guard", Payload: map[string]interface{}{"delta": 3.0}, SceneID: "外城", Canonical: true, CreatedAt: now.Add(6 * time.Second)},
	} {
		if err := engine.eventStore.Append(evt); err != nil {
			t.Fatalf("append growth event %s: %v", evt.ID, err)
		}
	}
	engine.reconcilePopulationLocked()
	engine.stateMgr.Set(core.WorldState{
		Scene: core.SceneState{
			Location:   "外城",
			Characters: []string{"111", "guard", "smugglers"},
		},
		Relationships: map[string]core.Relationship{
			"guard_smugglers": {Trust: preShiftTrust[len(preShiftTrust)-1]},
		},
		Variables: map[string]interface{}{},
		Flags:     map[string]bool{},
	})

	postShiftTrust := make([]float64, 0, 3)
	for i := 0; i < 3; i++ {
		engine.onTick()
		postShiftTrust = append(postShiftTrust, engine.GetState().Relationships["guard_smugglers"].Trust)
	}
	postShiftLogs := engine.scheduler.RecentActionsForCharacter("guard", 0)
	postShiftTrustActions := 0
	for _, log := range postShiftLogs {
		if log.Action == "trust" {
			postShiftTrustActions++
		}
	}

	postShiftAddedTrustActions := 0
	postShiftThreatActions := 0
	for _, log := range postShiftLogs {
		if log.Tick > 2 && log.Action == "trust" {
			postShiftAddedTrustActions++
		}
		if log.Tick > 2 && log.Action == "threaten" {
			postShiftThreatActions++
		}
	}
	preShiftAddedTrustActions := 0
	for _, log := range preShiftLogs {
		if log.Tick <= 2 && log.Action == "trust" {
			preShiftAddedTrustActions++
		}
	}

	if preShiftAddedTrustActions == 0 && postShiftAddedTrustActions == 0 {
		t.Fatalf("pre-shift trust = %#v post-shift trust = %#v pre logs = %#v post logs = %#v, want identity-influenced trust actions in trajectory", preShiftTrust, postShiftTrust, preShiftLogs, postShiftLogs)
	}
	if postShiftThreatActions > 0 {
		t.Fatalf("post-shift logs = %#v, want grown identity to avoid renewed threats in later ticks", postShiftLogs)
	}
	if len(postShiftTrust) == 0 || len(preShiftTrust) == 0 || postShiftTrust[len(postShiftTrust)-1] < preShiftTrust[len(preShiftTrust)-1] {
		t.Fatalf("pre-shift trust = %#v post-shift trust = %#v, want identity shift to sustain or improve relationship trajectory", preShiftTrust, postShiftTrust)
	}
}

func TestIdentityShiftShapesLongWindowWorldOutcome(t *testing.T) {
	root := t.TempDir()
	buildBranch := func(t *testing.T, branch string, growTrust bool) *Engine {
		t.Helper()
		engine := newMultiCharacterTestEngine(t)
		worldDir := filepath.Join(root, branch+"-world")
		scene := core.SceneState{
			Location:    "外城",
			TimeOfDay:   "深夜",
			Weather:     "阴",
			Characters:  []string{"111", "玩家"},
			Description: "外城冲突长期运行样本",
		}
		writeTestWorldBundle(t, worldDir, "慢变量结果世界 "+branch, "人格慢变量会改变长期世界结果", scene)
		engine.ConfigurePersistence(filepath.Join(root, branch+"-data"), map[string]string{}, map[string]string{
			"111": worldDir,
		})
		engine.worldPaths["111"] = worldDir
		engine.charWorlds["111"] = CharWorld{
			WorldName: "慢变量结果世界 " + branch,
			CoreRules: "人格慢变量会改变长期世界结果",
			Scene:     scene,
		}
		engine.focusCharacter = "111"
		engine.SyncActiveWorldContext()
		engine.scheduler.SetActionInterval(0)
		engine.scheduler.SetRandomStepChance(0)

		engine.agents.LoadCharacter("smugglers", core.Character{
			WorldPath: worldDir,
			Identity: core.IdentityEnvelope{
				Name:      "smugglers",
				Adaptive:  map[string]float64{"trust": 3},
				Immutable: []string{"wary"},
			},
		})

		if _, err := engine.UpdatePopulationConfig(core.PopulationConfig{
			BackgroundNPCs: []core.BackgroundNPC{{
				ID:       "guard",
				Name:     "guard",
				Role:     "guard",
				Location: "外城",
				Faction:  "guard",
				Traits:   []string{"固执", "独立"},
				Hooks:    []string{"守住防线"},
			}},
			Policy: core.PromotionPolicy{
				PromoteThreshold:   4,
				MajorThreshold:     12,
				InteractionWeight:  3,
				MentionWeight:      1,
				EventWeight:        2,
				RelationshipWeight: 3,
				SceneWeight:        1,
			},
		}); err != nil {
			t.Fatalf("UpdatePopulationConfig %s: %v", branch, err)
		}
		if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
			Factions: []core.WorldFactionConfig{
				{ID: "guard", Name: "guard"},
				{ID: "smugglers", Name: "smugglers"},
			},
			Locations: []core.WorldLocationConfig{
				{ID: "outer_city", Name: "外城", Controller: "guard"},
			},
		}); err != nil {
			t.Fatalf("UpdateWorldStructureConfig %s: %v", branch, err)
		}

		now := time.Now().UTC()
		for _, evt := range []core.Event{
			{ID: branch + "_seed_1", Type: "dialogue", Actor: "guard", Target: "111", Payload: map[string]interface{}{"content": "守卫在街口盘查"}, SceneID: "外城", Canonical: true, CreatedAt: now},
			{ID: branch + "_seed_2", Type: "user_message", Actor: "user", Target: "111", Payload: map[string]interface{}{"content": "继续观察守卫"}, SceneID: "外城", Canonical: true, CreatedAt: now.Add(time.Second)},
		} {
			if err := engine.eventStore.Append(evt); err != nil {
				t.Fatalf("append seed event %s: %v", evt.ID, err)
			}
		}
		engine.reconcilePopulationLocked()

		engine.agents.LoadCharacter("guard", core.Character{
			WorldPath: worldDir,
			Identity: core.IdentityEnvelope{
				Name:      "guard",
				Adaptive:  map[string]float64{"trust": 2, "aggression": 6, "fear": 2},
				Immutable: []string{"固执", "独立"},
			},
		})
		engine.worldPaths["guard"] = worldDir
		engine.charWorlds["guard"] = CharWorld{
			WorldName: "慢变量结果世界 " + branch,
			CoreRules: "人格慢变量会改变长期世界结果",
			Scene: core.SceneState{
				Location:    "外城",
				TimeOfDay:   "深夜",
				Weather:     "阴",
				Characters:  []string{"guard", "smugglers"},
				Description: "守卫与走私帮持续对峙",
			},
		}

		if growTrust {
			for i := 0; i < 5; i++ {
				evt := core.Event{
					ID:        fmt.Sprintf("%s_growth_%d", branch, i),
					Type:      "trust_change",
					Actor:     "111",
					Target:    "guard",
					Payload:   map[string]interface{}{"delta": 3.0},
					SceneID:   "外城",
					Canonical: true,
					CreatedAt: now.Add(time.Duration(i+2) * time.Second),
				}
				if err := engine.eventStore.Append(evt); err != nil {
					t.Fatalf("append growth event %s: %v", evt.ID, err)
				}
			}
			engine.reconcilePopulationLocked()
			guard, ok := engine.agents.GetCharacter("guard")
			if !ok {
				t.Fatalf("guard missing after growth reconcile")
			}
			if guard.Identity.Adaptive["trust"] < 7 {
				t.Fatalf("grown guard trust = %.2f, want >= 7", guard.Identity.Adaptive["trust"])
			}
		}

		engine.stateMgr.Set(core.WorldState{
			Scene: core.SceneState{
				Location:   "外城",
				Characters: []string{"111", "guard", "smugglers"},
			},
			Relationships: map[string]core.Relationship{},
			Variables:     map[string]interface{}{},
			Flags:         map[string]bool{},
		})
		return engine
	}

	ungrown := buildBranch(t, "ungrown", false)
	grown := buildBranch(t, "grown", true)
	for i := 0; i < 8; i++ {
		ungrown.onTick()
		grown.onTick()
	}

	ungrownActions := ungrown.scheduler.RecentActionsForCharacter("guard", 0)
	grownActions := grown.scheduler.RecentActionsForCharacter("guard", 0)
	ungrownThreats := countNPCAction(ungrownActions, "threaten")
	grownTrusts := countNPCAction(grownActions, "trust")
	if ungrownThreats == 0 {
		t.Fatalf("ungrown actions = %#v, want aggressive threat actions before slow-variable growth", ungrownActions)
	}
	if grownTrusts == 0 {
		t.Fatalf("grown actions = %#v, want trust actions after slow-variable growth", grownActions)
	}

	ungrownStatus := ungrown.TickStatus()
	grownStatus := grown.TickStatus()
	ungrownTension, _ := ungrownStatus["tension"].(float64)
	grownTension, _ := grownStatus["tension"].(float64)
	if ungrownTension <= grownTension {
		t.Fatalf("ungrown tension %.2f grown tension %.2f, want slow-variable growth to change long-window world tension", ungrownTension, grownTension)
	}
	ungrownTrajectory, _ := ungrownStatus["trajectory_summary"].([]string)
	grownTrajectory, _ := grownStatus["trajectory_summary"].([]string)
	if len(ungrownTrajectory) == 0 || len(grownTrajectory) == 0 {
		t.Fatalf("trajectory summaries ungrown=%#v grown=%#v, want author-visible long-window summaries", ungrownTrajectory, grownTrajectory)
	}
	if strings.Join(ungrownTrajectory, " | ") == strings.Join(grownTrajectory, " | ") {
		t.Fatalf("trajectory summaries should diverge after identity slow-variable growth: ungrown=%#v grown=%#v", ungrownTrajectory, grownTrajectory)
	}
}

func TestIdentityShiftShapesWorldOutcomeAcrossWorldFamilies(t *testing.T) {
	type sample struct {
		name         string
		location     string
		targetNPC    string
		otherNPC     string
		targetTraits []string
		otherTraits  []string
		hooks        []string
	}

	samples := []sample{
		{
			name:         "outer-city",
			location:     "外城",
			targetNPC:    "guard",
			otherNPC:     "smugglers",
			targetTraits: []string{"固执", "独立"},
			otherTraits:  []string{"wary"},
			hooks:        []string{"守住防线"},
		},
		{
			name:         "harbor",
			location:     "码头",
			targetNPC:    "dispatcher",
			otherNPC:     "brokers",
			targetTraits: []string{"急切", "务实"},
			otherTraits:  []string{"opportunistic"},
			hooks:        []string{"维持装卸秩序"},
		},
	}

	root := t.TempDir()
	buildBranch := func(t *testing.T, sample sample, branch string, growTrust bool) *Engine {
		t.Helper()
		engine := newMultiCharacterTestEngine(t)
		worldDir := filepath.Join(root, sample.name+"-"+branch+"-world")
		scene := core.SceneState{
			Location:    sample.location,
			TimeOfDay:   "深夜",
			Weather:     "阴",
			Characters:  []string{"111", "玩家"},
			Description: sample.name + " 慢变量长期运行样本",
		}
		worldName := "慢变量矩阵世界 " + sample.name + " " + branch
		writeTestWorldBundle(t, worldDir, worldName, "人格慢变量会改变长期世界结果", scene)
		engine.ConfigurePersistence(filepath.Join(root, sample.name+"-"+branch+"-data"), map[string]string{}, map[string]string{
			"111": worldDir,
		})
		engine.worldPaths["111"] = worldDir
		engine.charWorlds["111"] = CharWorld{
			WorldName: worldName,
			CoreRules: "人格慢变量会改变长期世界结果",
			Scene:     scene,
		}
		engine.focusCharacter = "111"
		engine.SyncActiveWorldContext()
		engine.scheduler.SetActionInterval(0)
		engine.scheduler.SetRandomStepChance(0)

		engine.agents.LoadCharacter(sample.otherNPC, core.Character{
			WorldPath: worldDir,
			Identity: core.IdentityEnvelope{
				Name:      sample.otherNPC,
				Adaptive:  map[string]float64{"trust": 3},
				Immutable: append([]string(nil), sample.otherTraits...),
			},
		})

		if _, err := engine.UpdatePopulationConfig(core.PopulationConfig{
			BackgroundNPCs: []core.BackgroundNPC{{
				ID:       sample.targetNPC,
				Name:     sample.targetNPC,
				Role:     "guard",
				Location: sample.location,
				Faction:  sample.targetNPC,
				Traits:   append([]string(nil), sample.targetTraits...),
				Hooks:    append([]string(nil), sample.hooks...),
			}},
			Policy: core.PromotionPolicy{
				PromoteThreshold:   4,
				MajorThreshold:     12,
				InteractionWeight:  3,
				MentionWeight:      1,
				EventWeight:        2,
				RelationshipWeight: 3,
				SceneWeight:        1,
			},
		}); err != nil {
			t.Fatalf("UpdatePopulationConfig %s/%s: %v", sample.name, branch, err)
		}
		if _, err := engine.UpdateWorldStructureConfig(core.WorldStructureConfig{
			Factions: []core.WorldFactionConfig{
				{ID: sample.targetNPC, Name: sample.targetNPC},
				{ID: sample.otherNPC, Name: sample.otherNPC},
			},
			Locations: []core.WorldLocationConfig{
				{ID: sample.name + "_location", Name: sample.location, Controller: sample.targetNPC},
			},
		}); err != nil {
			t.Fatalf("UpdateWorldStructureConfig %s/%s: %v", sample.name, branch, err)
		}

		now := time.Now().UTC()
		for _, evt := range []core.Event{
			{ID: sample.name + "_" + branch + "_seed_1", Type: "dialogue", Actor: sample.targetNPC, Target: "111", Payload: map[string]interface{}{"content": sample.targetNPC + " 正在现场维持秩序"}, SceneID: sample.location, Canonical: true, CreatedAt: now},
			{ID: sample.name + "_" + branch + "_seed_2", Type: "user_message", Actor: "user", Target: "111", Payload: map[string]interface{}{"content": "继续观察 " + sample.targetNPC}, SceneID: sample.location, Canonical: true, CreatedAt: now.Add(time.Second)},
		} {
			if err := engine.eventStore.Append(evt); err != nil {
				t.Fatalf("append seed event %s: %v", evt.ID, err)
			}
		}
		engine.reconcilePopulationLocked()
		engine.agents.LoadCharacter(sample.targetNPC, core.Character{
			WorldPath: worldDir,
			Identity: core.IdentityEnvelope{
				Name:      sample.targetNPC,
				Adaptive:  map[string]float64{"trust": 2, "aggression": 6, "fear": 2},
				Immutable: append([]string(nil), sample.targetTraits...),
			},
		})
		engine.worldPaths[sample.targetNPC] = worldDir
		engine.charWorlds[sample.targetNPC] = CharWorld{
			WorldName: worldName,
			CoreRules: "人格慢变量会改变长期世界结果",
			Scene: core.SceneState{
				Location:    sample.location,
				TimeOfDay:   "深夜",
				Weather:     "阴",
				Characters:  []string{sample.targetNPC, sample.otherNPC},
				Description: sample.targetNPC + " 与 " + sample.otherNPC + " 持续对峙",
			},
		}

		if growTrust {
			for i := 0; i < 5; i++ {
				evt := core.Event{
					ID:        fmt.Sprintf("%s_%s_growth_%d", sample.name, branch, i),
					Type:      "trust_change",
					Actor:     "111",
					Target:    sample.targetNPC,
					Payload:   map[string]interface{}{"delta": 3.0},
					SceneID:   sample.location,
					Canonical: true,
					CreatedAt: now.Add(time.Duration(i+2) * time.Second),
				}
				if err := engine.eventStore.Append(evt); err != nil {
					t.Fatalf("append growth event %s: %v", evt.ID, err)
				}
			}
			engine.reconcilePopulationLocked()
			character, ok := engine.agents.GetCharacter(sample.targetNPC)
			if !ok {
				t.Fatalf("%s missing after growth reconcile", sample.targetNPC)
			}
			if character.Identity.Adaptive["trust"] < 7 {
				t.Fatalf("%s grown trust = %.2f, want >= 7", sample.targetNPC, character.Identity.Adaptive["trust"])
			}
		}

		engine.stateMgr.Set(core.WorldState{
			Scene: core.SceneState{
				Location:   sample.location,
				Characters: []string{"111", sample.targetNPC, sample.otherNPC},
			},
			Relationships: map[string]core.Relationship{},
			Variables:     map[string]interface{}{},
			Flags:         map[string]bool{},
		})
		return engine
	}

	for _, sample := range samples {
		ungrown := buildBranch(t, sample, "ungrown", false)
		grown := buildBranch(t, sample, "grown", true)
		for i := 0; i < 8; i++ {
			ungrown.onTick()
			grown.onTick()
		}

		ungrownActions := ungrown.scheduler.RecentActionsForCharacter(sample.targetNPC, 0)
		grownActions := grown.scheduler.RecentActionsForCharacter(sample.targetNPC, 0)
		if countNPCAction(ungrownActions, "threaten") == 0 {
			t.Fatalf("%s ungrown actions = %#v, want threat actions before slow-variable growth", sample.name, ungrownActions)
		}
		if countNPCAction(grownActions, "trust") == 0 {
			t.Fatalf("%s grown actions = %#v, want trust actions after slow-variable growth", sample.name, grownActions)
		}

		ungrownStatus := ungrown.TickStatus()
		grownStatus := grown.TickStatus()
		ungrownTension, _ := ungrownStatus["tension"].(float64)
		grownTension, _ := grownStatus["tension"].(float64)
		if ungrownTension <= grownTension {
			t.Fatalf("%s ungrown tension %.2f grown tension %.2f, want slow-variable growth to change world outcome", sample.name, ungrownTension, grownTension)
		}
		ungrownTrajectory, _ := ungrownStatus["trajectory_summary"].([]string)
		grownTrajectory, _ := grownStatus["trajectory_summary"].([]string)
		if len(ungrownTrajectory) == 0 || len(grownTrajectory) == 0 {
			t.Fatalf("%s trajectories ungrown=%#v grown=%#v, want author-visible summaries", sample.name, ungrownTrajectory, grownTrajectory)
		}
		if strings.Join(ungrownTrajectory, " | ") == strings.Join(grownTrajectory, " | ") {
			t.Fatalf("%s trajectories should diverge after slow-variable growth: ungrown=%#v grown=%#v", sample.name, ungrownTrajectory, grownTrajectory)
		}
	}
}

func countNPCAction(actions []agents.NPCActionLog, action string) int {
	count := 0
	for _, item := range actions {
		if item.Action == action {
			count++
		}
	}
	return count
}

func TestPopulationInsightsIncludesPromotionReason(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)
	root := t.TempDir()
	worldDir := filepath.Join(root, "reason-world")
	writeTestWorldBundle(t, worldDir, "原因世界", "每次晋升都有原因", core.SceneState{
		Location:    "广场",
		TimeOfDay:   "午后",
		Weather:     "晴",
		Characters:  []string{"111", "玩家"},
		Description: "广场上人来人往",
	})

	engine.ConfigurePersistence(root, map[string]string{}, map[string]string{
		"111": worldDir,
	})
	engine.worldPaths["111"] = worldDir
	engine.charWorlds["111"] = CharWorld{
		WorldName: "原因世界",
		CoreRules: "每次晋升都有原因",
		Scene: core.SceneState{
			Location:    "广场",
			TimeOfDay:   "午后",
			Weather:     "晴",
			Characters:  []string{"111", "玩家"},
			Description: "广场上人来人往",
		},
	}
	engine.focusCharacter = "111"
	engine.SyncActiveWorldContext()

	_, err := engine.UpdatePopulationConfig(core.PopulationConfig{
		BackgroundNPCs: []core.BackgroundNPC{{
			ID:       "vendor",
			Name:     "小贩",
			Role:     "商贩",
			Location: "广场",
			Traits:   []string{"热情"},
			Hooks:    []string{"知道城里所有八卦"},
		}},
		Policy: core.PromotionPolicy{
			PromoteThreshold:   5,
			MajorThreshold:     15,
			InteractionWeight:  3,
			MentionWeight:      1,
			EventWeight:        2,
			RelationshipWeight: 2,
			SceneWeight:        1,
		},
	})
	if err != nil {
		t.Fatalf("UpdatePopulationConfig: %v", err)
	}

	now := time.Now().UTC()
	events := []core.Event{
		{ID: "reason_1", Type: "dialogue", Actor: "小贩", Target: "111", Payload: map[string]interface{}{"content": "小贩说今天生意不错"}, SceneID: "广场", Canonical: true, CreatedAt: now},
		{ID: "reason_2", Type: "user_message", Actor: "user", Target: "111", Payload: map[string]interface{}{"content": "我想问问小贩有没有见过可疑的人"}, SceneID: "广场", Canonical: true, CreatedAt: now.Add(time.Second)},
	}
	for _, evt := range events {
		if err := engine.eventStore.Append(evt); err != nil {
			t.Fatalf("append event %s: %v", evt.ID, err)
		}
	}

	engine.reconcilePopulationLocked()

	insights, err := engine.GetPopulationInsights()
	if err != nil {
		t.Fatalf("GetPopulationInsights: %v", err)
	}

	found := false
	for _, npc := range insights.Promoted {
		if npc.Name == "小贩" {
			found = true
			if npc.GrowthSummary == "" || npc.GrowthSummary == "尚未被世界卷入" {
				t.Fatalf("promoted npc growth_summary = %q, want non-empty after promotion", npc.GrowthSummary)
			}
			if npc.Adaptive == nil || len(npc.Adaptive) == 0 {
				t.Fatalf("promoted npc adaptive = %v, want non-empty", npc.Adaptive)
			}
			break
		}
	}
	if !found {
		t.Fatalf("promoted insights = %#v, want 小贩 promoted", insights.Promoted)
	}
}
