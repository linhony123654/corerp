package emotion

import (
	"testing"
)

func TestActionLoggerRecordAndRecent(t *testing.T) {
	l := NewActionLogger(10)

	l.Record(ActionLogEntry{Tick: 0, Character: "V", Fired: true, ActionType: "approach", Target: "玩家", Urgency: 0.85, Reason: "想靠近", PressureTotal: 0.85, DominantEmotion: "attachment"})
	l.Record(ActionLogEntry{Tick: 1, Character: "Jackie", Fired: false, BlockedBy: "cooldown", PressureTotal: 0.75, DominantEmotion: "fear"})
	l.Record(ActionLogEntry{Tick: 2, Character: "V", Fired: false, BlockedBy: "below_threshold", PressureTotal: 0.4, DominantEmotion: "joy"})

	recent := l.Recent(3)
	if len(recent) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(recent))
	}
	if recent[0].Character != "V" {
		t.Errorf("first entry character = %s, want V", recent[0].Character)
	}
	if recent[1].Character != "Jackie" {
		t.Errorf("second entry character = %s, want Jackie", recent[1].Character)
	}
}

func TestActionLoggerRingBuffer(t *testing.T) {
	l := NewActionLogger(3)

	// Write 5 entries → only 3 kept
	for i := 0; i < 5; i++ {
		l.Record(ActionLogEntry{Tick: i, Character: "V", Fired: true, ActionType: "approach"})
	}

	recent := l.Recent(10)
	if len(recent) != 3 {
		t.Errorf("ring buffer should hold 3, got %d", len(recent))
	}
	// Entries 2, 3, 4 should remain (oldest 0, 1 evicted)
	if recent[0].Tick != 2 {
		t.Errorf("oldest retained tick = %d, want 2", recent[0].Tick)
	}
	if recent[2].Tick != 4 {
		t.Errorf("newest tick = %d, want 4", recent[2].Tick)
	}
}

func TestActionLoggerByCharacter(t *testing.T) {
	l := NewActionLogger(10)

	l.Record(ActionLogEntry{Tick: 0, Character: "V", Fired: true, ActionType: "approach"})
	l.Record(ActionLogEntry{Tick: 1, Character: "Jackie", Fired: true, ActionType: "confront"})
	l.Record(ActionLogEntry{Tick: 2, Character: "V", Fired: true, ActionType: "withdraw"})

	vEntries := l.ByCharacter("V", 10)
	if len(vEntries) != 2 {
		t.Fatalf("V entries = %d, want 2", len(vEntries))
	}
	if vEntries[0].Tick != 0 || vEntries[1].Tick != 2 {
		t.Errorf("V entries out of order: ticks %d, %d", vEntries[0].Tick, vEntries[1].Tick)
	}

	jEntries := l.ByCharacter("Jackie", 10)
	if len(jEntries) != 1 {
		t.Errorf("Jackie entries = %d, want 1", len(jEntries))
	}
}

func TestActionLoggerFiredOnly(t *testing.T) {
	l := NewActionLogger(10)

	l.Record(ActionLogEntry{Tick: 0, Character: "V", Fired: true, ActionType: "approach"})
	l.Record(ActionLogEntry{Tick: 1, Character: "V", Fired: false, BlockedBy: "cooldown"})
	l.Record(ActionLogEntry{Tick: 2, Character: "V", Fired: true, ActionType: "confront"})

	fired := l.FiredOnly(10)
	if len(fired) != 2 {
		t.Errorf("fired = %d, want 2", len(fired))
	}
	for _, f := range fired {
		if !f.Fired {
			t.Error("all entries in FiredOnly should have Fired=true")
		}
	}
}

func TestActionLoggerBlockedOnly(t *testing.T) {
	l := NewActionLogger(10)

	l.Record(ActionLogEntry{Tick: 0, Character: "V", Fired: false, BlockedBy: "cooldown"})
	l.Record(ActionLogEntry{Tick: 1, Character: "V", Fired: false, BlockedBy: "below_threshold"})
	l.Record(ActionLogEntry{Tick: 2, Character: "V", Fired: false, BlockedBy: "scene_cap"})

	blocked := l.BlockedOnly(10)
	// below_threshold is excluded from BlockedOnly
	if len(blocked) != 2 {
		t.Errorf("blocked (excl below_threshold) = %d, want 2", len(blocked))
	}
}

func TestActionLoggerStats(t *testing.T) {
	l := NewActionLogger(10)

	l.Record(ActionLogEntry{Tick: 0, Character: "V", Fired: true, ActionType: "approach"})
	l.Record(ActionLogEntry{Tick: 1, Character: "Jackie", Fired: false, BlockedBy: "cooldown"})
	l.Record(ActionLogEntry{Tick: 2, Character: "V", Fired: false, BlockedBy: "below_threshold"})
	l.Record(ActionLogEntry{Tick: 3, Character: "V", Fired: false, BlockedBy: "scene_cap"})

	stats := l.Stats()
	if stats["fired"].(int) != 1 {
		t.Errorf("fired = %v, want 1", stats["fired"])
	}
	if stats["blocked"].(int) != 2 {
		t.Errorf("blocked = %v, want 2", stats["blocked"])
	}
	if stats["below_threshold"].(int) != 1 {
		t.Errorf("below_threshold = %v, want 1", stats["below_threshold"])
	}
	byBlock := stats["by_block_reason"].(map[string]int)
	if byBlock["cooldown"] != 1 || byBlock["scene_cap"] != 1 {
		t.Errorf("block reasons = %v", byBlock)
	}
}

func TestActionLoggerTotal(t *testing.T) {
	l := NewActionLogger(10)
	for i := 0; i < 7; i++ {
		l.Record(ActionLogEntry{Tick: i, Character: "V"})
	}
	if l.Total() != 7 {
		t.Errorf("total = %d, want 7", l.Total())
	}
}

func TestActionLoggerConcurrent(t *testing.T) {
	l := NewActionLogger(50)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				l.Record(ActionLogEntry{Tick: id*10 + j, Character: "V", Fired: true})
			}
			done <- true
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	recent := l.Recent(50)
	if len(recent) != 50 {
		t.Errorf("concurrent entries = %d, want 50", len(recent))
	}
	_ = l.Stats() // should not race
}

func TestActionLoggerPersistenceRoundTrip(t *testing.T) {
	e := newTestEngine(t)
	l := NewActionLogger(10)
	l.EnablePersistence(e.DB())

	// Write some entries
	l.Record(ActionLogEntry{Tick: 0, Character: "V", Fired: true, ActionType: "approach", Target: "玩家", Urgency: 0.8, PressureTotal: 0.8, DominantEmotion: "attachment"})
	l.Record(ActionLogEntry{Tick: 1, Character: "V", Fired: false, BlockedBy: "cooldown", PressureTotal: 0.7, DominantEmotion: "fear"})

	// Query from DB
	entries, err := l.QueryDB("V", false, false, 10)
	if err != nil {
		t.Fatalf("query db: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("query returned %d entries, want 2", len(entries))
	}
	if entries[0].ActionType != "approach" {
		t.Errorf("first action = %s, want approach", entries[0].ActionType)
	}
	if entries[1].BlockedBy != "cooldown" {
		t.Errorf("second blocked_by = %s, want cooldown", entries[1].BlockedBy)
	}
}

func TestActionLoggerQueryDBFiltered(t *testing.T) {
	e := newTestEngine(t)
	l := NewActionLogger(10)
	l.EnablePersistence(e.DB())

	l.Record(ActionLogEntry{Tick: 0, Character: "V", Fired: true, ActionType: "approach"})
	l.Record(ActionLogEntry{Tick: 1, Character: "Jackie", Fired: true, ActionType: "confront"})
	l.Record(ActionLogEntry{Tick: 2, Character: "V", Fired: false, BlockedBy: "cooldown"})
	l.Record(ActionLogEntry{Tick: 3, Character: "V", Fired: false, BlockedBy: "scene_cap"})

	// Filter by character
	vEntries, _ := l.QueryDB("V", false, false, 10)
	if len(vEntries) != 3 {
		t.Errorf("V entries = %d, want 3", len(vEntries))
	}

	// Fired only
	fired, _ := l.QueryDB("", true, false, 10)
	if len(fired) != 2 {
		t.Errorf("fired = %d, want 2", len(fired))
	}
	for _, f := range fired {
		if !f.Fired {
			t.Error("fired-only should return only fired entries")
		}
	}

	// Blocked only
	blocked, _ := l.QueryDB("", false, true, 10)
	if len(blocked) != 2 {
		t.Errorf("blocked = %d, want 2", len(blocked))
	}
}

func TestActionLoggerLoadFromDB(t *testing.T) {
	e := newTestEngine(t)

	// Logger 1: write to DB
	l1 := NewActionLogger(50)
	l1.EnablePersistence(e.DB())
	l1.Record(ActionLogEntry{Tick: 0, Character: "V", Fired: true, ActionType: "approach"})
	l1.Record(ActionLogEntry{Tick: 1, Character: "Jackie", Fired: true, ActionType: "confront"})
	l1.Record(ActionLogEntry{Tick: 2, Character: "V", Fired: false, BlockedBy: "cooldown"})

	// Logger 2: simulates restart, loads from DB
	l2 := NewActionLogger(50)
	l2.EnablePersistence(e.DB())
	l2.LoadFromDB(10)

	recent := l2.Recent(10)
	if len(recent) != 3 {
		t.Fatalf("after restart: %d entries, want 3", len(recent))
	}
	if recent[0].Character != "V" || recent[1].Character != "Jackie" {
		t.Error("restored entries out of order")
	}
}

func TestActionLoggerInstanceScopedPersistence(t *testing.T) {
	e := newTestEngine(t)

	alpha := NewActionLogger(20)
	if err := alpha.EnablePersistence(e.DB()); err != nil {
		t.Fatalf("alpha EnablePersistence: %v", err)
	}
	alpha.SetInstanceID("alpha")
	alpha.Record(ActionLogEntry{Tick: 1, Character: "V", Fired: true, ActionType: "approach"})

	beta := NewActionLogger(20)
	if err := beta.EnablePersistence(e.DB()); err != nil {
		t.Fatalf("beta EnablePersistence: %v", err)
	}
	beta.SetInstanceID("beta")
	beta.Record(ActionLogEntry{Tick: 2, Character: "V", Fired: false, BlockedBy: "cooldown"})

	alphaReload := NewActionLogger(20)
	if err := alphaReload.EnablePersistence(e.DB()); err != nil {
		t.Fatalf("alphaReload EnablePersistence: %v", err)
	}
	alphaReload.SetInstanceID("alpha")
	if err := alphaReload.LoadFromDB(10); err != nil {
		t.Fatalf("alphaReload LoadFromDB: %v", err)
	}
	alphaRecent := alphaReload.Recent(10)
	if len(alphaRecent) != 1 || alphaRecent[0].Tick != 1 {
		t.Fatalf("alpha recent = %#v, want only tick 1", alphaRecent)
	}

	betaEntries, err := beta.QueryDB("V", false, false, 10)
	if err != nil {
		t.Fatalf("beta QueryDB: %v", err)
	}
	if len(betaEntries) != 1 || betaEntries[0].Tick != 2 {
		t.Fatalf("beta entries = %#v, want only tick 2", betaEntries)
	}
}

func TestActionLoggerFullContext(t *testing.T) {
	l := NewActionLogger(10)

	entry := ActionLogEntry{
		Tick:                  42,
		Character:             "V",
		Fired:                 true,
		ActionType:            "confront",
		Target:                "betrayer",
		Urgency:               0.92,
		Reason:                "被背叛的愤怒 (pressure=0.92)",
		PressureTotal:         0.92,
		PressureLoneliness:    0.3,
		PressureJealousy:      0.5,
		PressureGuilt:         0.1,
		PressureAnxiety:       0.8,
		StrongestDesire:       "revenge",
		StrongestDesireType:   "revenge",
		StrongestDesireTarget: "betrayer",
		DominantEmotion:       "anger",
	}
	l.Record(entry)

	recent := l.Recent(1)
	if len(recent) != 1 {
		t.Fatal("entry not recorded")
	}
	e := recent[0]
	if e.ActionType != "confront" {
		t.Errorf("action = %s, want confront", e.ActionType)
	}
	if e.PressureAnxiety != 0.8 {
		t.Errorf("anxiety = %.2f, want 0.80", e.PressureAnxiety)
	}
	if e.StrongestDesire != "revenge" {
		t.Errorf("desire = %s, want revenge", e.StrongestDesire)
	}
}
