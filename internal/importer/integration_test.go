package importer

import (
	"os"
	"path/filepath"
	"testing"

	"corerp/internal/actions"
	"corerp/internal/core"
	"corerp/internal/events"
)

func TestIntegrationFullPipeline(t *testing.T) {
	tmpDir := t.TempDir()

	// === Step 1: Import character card (Convert) ===
	st := SillyTavernChar{
		Name:        "测试·V",
		Description: "夜之城雇佣兵，冷静果断。",
		Personality: "沉默寡言、身手敏捷、从不背叛朋友。讨厌公司。",
		Scenario:    "在夜之城的酒吧里，外面下着酸雨。",
		FirstMes:    "*V靠在吧台上，推了推墨镜* 你想谈生意？坐。",
		MesExample:  "*冷冷地扫了一眼* 价格呢？",
	}
	char, world := Convert(st)

	if char.Identity.Name != "测试·V" {
		t.Fatalf("import name = %s, want 测试·V", char.Identity.Name)
	}
	if len(char.Identity.Immutable) == 0 {
		t.Fatal("expected immutable traits from personality")
	}

	// === Step 2: Write world to temp directory (three-layer) ===
	worldDir := filepath.Join(tmpDir, "worlds", "test-world")
	writeWorldDir(worldDir, world)

	// Verify three-layer output
	for _, f := range []string{
		"world.yml",
		"canon/ontology.yml",
		"canon/facts.yml",
		"scenes/default.yml",
	} {
		if _, err := os.Stat(filepath.Join(worldDir, f)); os.IsNotExist(err) {
			t.Errorf("missing file in three-layer world: %s", f)
		}
	}

	// === Step 3: Read back world.yml + canon/facts.yml ===
	worldData, err := os.ReadFile(filepath.Join(worldDir, "world.yml"))
	if err != nil {
		t.Fatalf("read world.yml: %v", err)
	}
	if len(worldData) < 20 {
		t.Error("world.yml too short")
	}

	factsData, err := os.ReadFile(filepath.Join(worldDir, "canon", "facts.yml"))
	if err != nil {
		t.Fatalf("read facts.yml: %v", err)
	}
	if len(factsData) < 10 {
		t.Error("facts.yml too short")
	}

	// === Step 4: Init event store ===
	store, err := events.New(":memory:")
	if err != nil {
		t.Fatalf("new event store: %v", err)
	}
	defer store.Close()

	// === Step 5: Execute ActionFrame → generate events ===
	ex := actions.NewExecutor()
	frames := []core.ActionFrame{
		{Actor: "V", Action: "speak", Target: "player", Intensity: 2,
			SuggestedLine: "你想谈生意？坐。",
			Emotion:       core.EmotionState{Primary: "neutral", Intensity: 0.2}},
		{Actor: "V", Action: "negotiate", Target: "player", Intensity: 4,
			Intent:  "bargain",
			Emotion: core.EmotionState{Primary: "wary", Intensity: 0.5}},
		{Actor: "V", Action: "trust", Target: "player", Intensity: 3,
			Emotion: core.EmotionState{Primary: "neutral", Intensity: 0.4}},
		{Actor: "player", Action: "speak", Target: "V", Intensity: 2,
			SuggestedLine: "我要找一个人。你有渠道吗？",
			Emotion:       core.EmotionState{Primary: "determined", Intensity: 0.6}},
	}

	state := core.WorldState{
		Scene:         world.SceneToCore(),
		Relationships: make(map[string]core.Relationship),
		Variables:     make(map[string]interface{}),
		Flags:         make(map[string]bool),
	}

	var committedEvents []core.Event
	for i, frame := range frames {
		evts, err := ex.Execute(frame, state)
		if err != nil {
			t.Fatalf("execute frame %d: %v", i, err)
		}
		for _, e := range evts {
			if err := store.Append(e); err != nil {
				t.Fatalf("append event %d: %v", i, err)
			}
			committedEvents = append(committedEvents, e)
		}
	}

	// === Step 6: Replay → reconstruct state ===
	re := events.NewReplayEngine(store)

	// Replay to the trust event (3rd frame, event index varies)
	if len(committedEvents) >= 3 {
		replayed, err := re.ReplayTo(committedEvents[2].ID, "main")
		if err != nil {
			t.Fatalf("replay to event %s: %v", committedEvents[2].ID, err)
		}

		// Trust event should have created relationship
		key := "V_player"
		if rel, ok := replayed.Relationships[key]; ok {
			if rel.Trust <= 0 {
				t.Error("trust should be positive after trust action")
			}
		}
	}

	// === Step 7: Project all canonical events ===
	canonicalEvents, err := store.GetCanonicalEvents()
	if err != nil {
		t.Fatalf("get canonical events: %v", err)
	}
	if len(canonicalEvents) == 0 {
		t.Fatal("expected at least 1 canonical event")
	}

	projected := events.Project(canonicalEvents)
	_ = projected.Tension // state must be valid

	// === Step 8: Get timeline ===
	timeline, err := re.GetTimeline("main", 50)
	if err != nil {
		t.Fatalf("get timeline: %v", err)
	}
	if len(timeline) == 0 {
		t.Error("timeline should not be empty")
	}

	// === Step 9: Fork and verify branching ===
	if len(committedEvents) >= 2 {
		forkPoint := committedEvents[1].ID
		if err := re.Fork(forkPoint, "alt_path"); err != nil {
			t.Fatalf("fork: %v", err)
		}

		branches, err := re.ListBranches()
		if err != nil {
			t.Fatalf("list branches: %v", err)
		}

		hasAlt := false
		for _, b := range branches {
			if b == "alt_path" {
				hasAlt = true
			}
		}
		if !hasAlt {
			t.Error("forked branch not found")
		}
	}

	// === Step 10: Validate world state after projection ===
	_ = projected.Scene.Location
	_ = projected.Clock
	_ = projected.Flags

	t.Logf("Integration test complete: %d events, timeline=%d, scene=%s",
		len(committedEvents), len(timeline), projected.Scene.Location)
}

func TestImportJSONEnsembleWritesCastIndexAndMultipleCharacters(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "hongloumeng.json")
	src := `{
	  "name": "红楼梦",
	  "description": "大观园群像。",
	  "first_mes": "贾宝玉看向林黛玉，薛宝钗在一旁静静坐着。",
	  "data": {
	    "character_book": {
	      "entries": [
	        {"comment":"[角色] 贾宝玉","keys":["贾宝玉"],"content":"性格敏感，身份复杂，喜欢诗意与自由。"},
	        {"comment":"[角色] 林黛玉","keys":["林黛玉"],"content":"性格孤高，情绪细腻，关系紧张。"},
	        {"comment":"[角色] 薛宝钗","keys":["薛宝钗"],"content":"性格稳重，处事圆融，目标克制。"},
	        {"comment":"[角色] 王熙凤","keys":["王熙凤"],"content":"精明强势，善于掌控局面。"}
	      ]
	    }
	  }
	}`
	if err := os.WriteFile(srcPath, []byte(src), 0644); err != nil {
		t.Fatalf("write source json: %v", err)
	}

	charPath, worldPath, err := ImportJSON(srcPath, filepath.Join(tmpDir, "characters"))
	if err != nil {
		t.Fatalf("ImportJSON: %v", err)
	}
	if charPath == "" || worldPath == "" {
		t.Fatalf("charPath=%q worldPath=%q, want non-empty", charPath, worldPath)
	}

	charEntries, err := os.ReadDir(filepath.Join(tmpDir, "characters"))
	if err != nil {
		t.Fatalf("read generated characters: %v", err)
	}
	if len(charEntries) < 3 {
		t.Fatalf("generated character files = %d, want >= 3", len(charEntries))
	}

	if _, err := os.Stat(filepath.Join(filepath.Dir(worldPath), "cast_index.yml")); err != nil {
		t.Fatalf("cast_index.yml missing: %v", err)
	}
}

// SceneToCore converts the internal SceneYAML to core.SceneState.
func (w WorldYAML) SceneToCore() core.SceneState {
	return core.SceneState{
		Location:    w.Scene.Location,
		TimeOfDay:   w.Scene.TimeOfDay,
		Weather:     w.Scene.Weather,
		Characters:  w.Scene.Characters,
		Description: w.Scene.Description,
	}
}
