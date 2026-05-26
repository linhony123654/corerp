package runtime

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
)

func TestSyncActiveWorldContextUpdatesSceneAndMetadata(t *testing.T) {
	engine := &Engine{
		stateMgr:        state.New(),
		activeCharacter: "V",
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
	if len(scene.Characters) != 2 || scene.Characters[0] != "V" {
		t.Fatalf("scene.Characters = %#v, want active world characters", scene.Characters)
	}
}

func TestNormalizeSceneForCharacterReplacesStaleLead(t *testing.T) {
	scene := normalizeSceneForCharacter(core.SceneState{
		Location:    "废弃地铁站",
		Characters:  []string{"安雅", "用户"},
		Description: "test",
	}, "111", "贾宝玉")

	if len(scene.Characters) != 2 {
		t.Fatalf("scene.Characters len = %d, want 2", len(scene.Characters))
	}
	if scene.Characters[0] != "111" || scene.Characters[1] != "贾宝玉" {
		t.Fatalf("scene.Characters = %#v, want [111 贾宝玉]", scene.Characters)
	}
}

func TestSyncActiveWorldContextIgnoresUnknownCharacter(t *testing.T) {
	engine := &Engine{
		stateMgr:        state.New(),
		activeCharacter: "Unknown",
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

func TestComposeTurnPromptIncludesRoleDirective(t *testing.T) {
	base := "=== 指令 ===\nbase prompt"
	step := core.TurnStep{
		Index:      0,
		Speaker:    "安雅",
		Kind:       "addressed_reply",
		BudgetMode: "full_load",
	}

	prompt := composeTurnPrompt(base, step, "玩家", nil)
	if !strings.Contains(prompt, "当前 step: #1 | speaker=安雅 | kind=addressed_reply | budget=full_load") {
		t.Fatalf("prompt missing step header: %s", prompt)
	}
	if !strings.Contains(prompt, "被 玩家 明确点名") {
		t.Fatalf("prompt missing addressed reply directive: %s", prompt)
	}
}

func TestStepPromptDirectivesDifferByKind(t *testing.T) {
	lead := strings.Join(stepPromptDirectives(core.TurnStep{Kind: "lead"}, "玩家"), "\n")
	support := strings.Join(stepPromptDirectives(core.TurnStep{Kind: "support_response"}, "玩家"), "\n")
	tension := strings.Join(stepPromptDirectives(core.TurnStep{Kind: "tension_response"}, "玩家"), "\n")

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

	prompt := composeTurnPrompt(base, step, "玩家", handoff)
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

	body := bytes.NewBufferString(`{"character":"安雅"}`)
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
	cfg, err := engine.GetCharacterConfig("安雅")
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if cfg.Path != charPath {
		t.Fatalf("config path = %q, want %q", cfg.Path, charPath)
	}

	updated, err := engine.UpdateCharacterConfig("安雅", core.Character{
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

	if err := engine.SwitchCharacter("安雅"); err != nil {
		t.Fatalf("switch to updated character: %v", err)
	}
	updatedChar, ok := engine.GetCharacter()
	if !ok {
		t.Fatalf("updated character should exist")
	}
	if updatedChar.Identity.WritingGuide != "new guide" {
		t.Fatalf("updated writing guide = %q, want new guide", updatedChar.Identity.WritingGuide)
	}
	if got := engine.GetState().Scene.Characters; len(got) == 0 || got[0] != "安雅" {
		t.Fatalf("scene characters after switch = %#v, want active character first", got)
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
	engine.activeCharacter = "111"
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

	if got := engine.GetCharacterName(); got != "安雅" {
		t.Fatalf("active speaker = %q, want 安雅", got)
	}
	plan := engine.GetDirectorPlan()
	if plan.Mode != "auto_single" || len(plan.Selected) == 0 || plan.Selected[0] != "安雅" {
		t.Fatalf("director plan = %#v, want 安雅 selected", plan)
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
	if trace.UserInput == "" || trace.Character == "" {
		t.Fatalf("trace = %#v, want user input and character", trace)
	}
	if trace.Turn == 0 {
		t.Fatalf("trace turn = 0")
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

	if err := engine.SwitchCharacter("安雅"); err != nil {
		t.Fatalf("switch character: %v", err)
	}
	loaded, err := engine.LoadSaveSlot("checkpoint-a")
	if err != nil {
		t.Fatalf("load save slot: %v", err)
	}
	if loaded.Character != "111" {
		t.Fatalf("loaded character = %q, want 111", loaded.Character)
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

	if err := engine.SwitchCharacter("安雅"); err != nil {
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
	if applied.Character != "111" {
		t.Fatalf("applied.Character = %q, want 111", applied.Character)
	}
	if got := engine.GetState().Scene.Location; got != "作者控制台" {
		t.Fatalf("scene.Location = %q, want 作者控制台", got)
	}
	if got := engine.GetPlayerRole().Name; got != "测试玩家" {
		t.Fatalf("player role = %q, want 测试玩家", got)
	}
}

func TestListTurnTracesNewestFirst(t *testing.T) {
	engine := newMultiCharacterTestEngine(t)

	engine.mu.Lock()
	engine.recordTraceLocked(core.TurnTrace{Turn: 1, Character: "111", UserInput: "first"})
	engine.recordTraceLocked(core.TurnTrace{Turn: 2, Character: "111", UserInput: "second"})
	engine.recordTraceLocked(core.TurnTrace{Turn: 3, Character: "安雅", UserInput: "third"})
	engine.mu.Unlock()

	traces := engine.ListTurnTraces(2)
	if len(traces) != 2 {
		t.Fatalf("len(traces) = %d, want 2", len(traces))
	}
	if traces[0].Turn != 3 || traces[1].Turn != 2 {
		t.Fatalf("turns = [%d %d], want [3 2]", traces[0].Turn, traces[1].Turn)
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

	if err := engine.SwitchCharacter("安雅"); err != nil {
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
	if got := engine.GetState().Scene.Characters; len(got) == 0 || got[0] != "111" {
		t.Fatalf("initial scene characters = %#v, want 111 first", got)
	}
	if msgs := engine.GetDialogueLimit(10); len(msgs) == 0 || msgs[0].Content != "111 hello" {
		t.Fatalf("initial dialogue = %#v, want 111 dialogue", msgs)
	}

	if err := engine.SwitchCharacter("安雅"); err != nil {
		t.Fatalf("switch character: %v", err)
	}

	char, ok = engine.GetCharacter()
	if !ok || char.Identity.Name != "安雅" {
		t.Fatalf("switched active character = %#v, want 安雅", char.Identity)
	}
	if got := engine.GetState().Scene.Characters; len(got) == 0 || got[0] != "安雅" {
		t.Fatalf("switched scene characters = %#v, want 安雅 first", got)
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
		if len(scene.Characters) < 2 || scene.Characters[0] != wantChar || scene.Characters[1] != "贾宝玉" {
			t.Fatalf("scene.Characters = %#v, want [%s 贾宝玉 ...]", scene.Characters, wantChar)
		}
		msgs := engine.GetDialogueLimit(10)
		if len(msgs) == 0 || msgs[0].Content != wantDialogueFirst {
			t.Fatalf("dialogue = %#v, want first content %q", msgs, wantDialogueFirst)
		}
	}

	assertActive("111", "夜之城 2077", "废弃地铁站", "111 hello")

	if err := engine.SwitchCharacter("安雅"); err != nil {
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

	if err := engine.SwitchCharacter("111"); err != nil {
		t.Fatalf("switch back to 111: %v", err)
	}
	assertActive("111", "夜之城 2077", "废弃地铁站", "111 hello")

	if err := engine.SwitchCharacter("安雅"); err != nil {
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

	reqCfg := httptest.NewRequest(http.MethodPost, "/api/character-config", bytes.NewBufferString(`{"character":"安雅","card":{"identity":{"name":"安雅","immutable":["guarded"],"adaptive":{"trust":4},"forbidden":["info_dump"],"voice":{"style":"spare","rhythm":"short"},"writing_guide":"updated"},"goals":[{"id":"survive","priority":10,"type":"primary","condition":"always"}]}}`))
	reqCfg.Header.Set("Content-Type", "application/json")
	recCfg := httptest.NewRecorder()
	mux.ServeHTTP(recCfg, reqCfg)
	if recCfg.Code != http.StatusOK {
		t.Fatalf("POST /api/character-config = %d", recCfg.Code)
	}

	switchReq := httptest.NewRequest(http.MethodPost, "/api/switch", bytes.NewBufferString(`{"character":"安雅"}`))
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
	var memoryResp core.MemorySnapshot
	if err := json.NewDecoder(memoryRec.Body).Decode(&memoryResp); err != nil {
		t.Fatalf("decode memory: %v", err)
	}
	if memoryResp.Character != "安雅" {
		t.Fatalf("memory character = %q, want 安雅", memoryResp.Character)
	}
	if len(memoryResp.Dialogue) == 0 || memoryResp.Dialogue[0].Content != "anya hello" {
		t.Fatalf("memory dialogue = %#v, want 安雅 dialogue", memoryResp.Dialogue)
	}

	charReq := httptest.NewRequest(http.MethodGet, "/api/character", nil)
	charRec := httptest.NewRecorder()
	mux.ServeHTTP(charRec, charReq)
	if charRec.Code != http.StatusOK {
		t.Fatalf("GET /api/character = %d", charRec.Code)
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
	if len(stateResp.Scene.Characters) == 0 || stateResp.Scene.Characters[0] != "安雅" {
		t.Fatalf("state scene characters = %#v, want 安雅 first", stateResp.Scene.Characters)
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
		if engine.tickCount >= 70 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if engine.tickCount < 70 {
		t.Fatalf("engine tickCount = %d, want >= 70", engine.tickCount)
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
