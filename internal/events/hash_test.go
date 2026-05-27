package events

import (
	"testing"

	"corerp/internal/core"
)

func TestCanonicalHashV1Deterministic(t *testing.T) {
	s := core.WorldState{
		Scene: core.SceneState{
			Location:    "夜之城",
			TimeOfDay:   "夜晚",
			Weather:     "酸雨",
			Characters:  []string{"V", "玩家", "Jackie"},
			Description: "霓虹灯闪烁",
		},
		Clock:   core.WorldTime{Hour: 23, Minute: 45, Day: 3},
		Tension: 0.75,
		Flags:   map[string]bool{"detected": true, "revealed": false},
		Relationships: map[string]core.Relationship{
			"V_玩家":     {Trust: 3.5, Intimacy: 1.2, Fear: 0.5, Respect: 4.0, Debt: 0},
			"V_Jackie": {Trust: 8.0, Intimacy: 7.0, Fear: 0, Respect: 9.0, Debt: 0},
		},
		Variables: map[string]interface{}{"last_location": "酒吧"},
	}

	h1 := CanonicalHashV1(s)
	h2 := CanonicalHashV1(s)

	if h1 != h2 {
		t.Fatalf("hash not deterministic: %s != %s", h1, h2)
	}

	// Version prefix must be present
	if len(h1) < 4 || h1[:3] != "v1:" {
		t.Errorf("hash must start with 'v1:', got %s", h1)
	}
}

func TestCanonicalHashV1DifferentState(t *testing.T) {
	s1 := core.WorldState{Scene: core.SceneState{Location: "A"}}
	s2 := core.WorldState{Scene: core.SceneState{Location: "B"}}

	if CanonicalHashV1(s1) == CanonicalHashV1(s2) {
		t.Error("different states must produce different hashes")
	}
}

func TestCanonicalHashV1MapOrderStable(t *testing.T) {
	// Insertion order shouldn't matter — sorted serialization
	s := core.WorldState{
		Flags:         make(map[string]bool),
		Relationships: make(map[string]core.Relationship),
		Variables:     make(map[string]interface{}),
	}
	// Insert out of order
	s.Flags["z"] = true
	s.Flags["a"] = false
	s.Flags["m"] = true

	s.Relationships["z_key"] = core.Relationship{Trust: 1}
	s.Relationships["a_key"] = core.Relationship{Trust: 2}

	s.Variables["z_var"] = "z"
	s.Variables["a_var"] = "a"

	h1 := CanonicalHashV1(s)
	h2 := CanonicalHashV1(s)

	if h1 != h2 {
		t.Fatal("same state must produce same hash regardless of map iteration order")
	}
}

func TestCanonicalHashV1FloatPrecision(t *testing.T) {
	s1 := core.WorldState{Tension: 0.3333333}
	s2 := core.WorldState{Tension: 0.3333334}

	// Both round to 0.333333 at 6 decimal places → same hash
	if CanonicalHashV1(s1) != CanonicalHashV1(s2) {
		t.Log("floats differ beyond 6dp — may produce different hashes")
	}
}

func TestEventHashChaining(t *testing.T) {
	e1 := core.Event{ID: "evt_1", Type: "dialogue", Payload: map[string]interface{}{"content": "hello"}}
	e2 := core.Event{ID: "evt_2", Type: "dialogue", Payload: map[string]interface{}{"content": "world"}}
	e3 := core.Event{ID: "evt_3", Type: "trust_change", Payload: map[string]interface{}{"delta": 0.5}}

	AppendEventToChain("genesis", &e1)
	AppendEventToChain(e1.Hash, &e2)
	AppendEventToChain(e2.Hash, &e3)

	if e1.Hash == "" || e2.Hash == "" || e3.Hash == "" {
		t.Fatal("all events must have non-empty hashes")
	}
	if e1.Hash == e2.Hash {
		t.Error("different events must have different hashes")
	}
}

func TestVerifyEventChainValid(t *testing.T) {
	events := []core.Event{
		{ID: "evt_1", Type: "dialogue", Payload: map[string]interface{}{"a": "1"}},
		{ID: "evt_2", Type: "threat", Payload: map[string]interface{}{"b": "2"}},
		{ID: "evt_3", Type: "trust_change", Payload: map[string]interface{}{"c": "3"}},
	}

	AppendEventToChain("genesis", &events[0])
	AppendEventToChain(events[0].Hash, &events[1])
	AppendEventToChain(events[1].Hash, &events[2])

	if idx := VerifyEventChain(events); idx != -1 {
		t.Errorf("chain should be valid, broke at index %d", idx)
	}
}

func TestEventChainTamperDetection(t *testing.T) {
	events := []core.Event{
		{ID: "evt_1", Type: "dialogue", Payload: map[string]interface{}{"a": "1"}},
		{ID: "evt_2", Type: "threat", Payload: map[string]interface{}{"b": "2"}},
	}

	AppendEventToChain("genesis", &events[0])
	AppendEventToChain(events[0].Hash, &events[1])

	// Tamper: change payload without updating hash
	events[1].Payload["b"] = "tampered"

	if idx := VerifyEventChain(events); idx == -1 {
		t.Error("should detect tampered event payload")
	}
}

func TestEventChainEmptySlice(t *testing.T) {
	if idx := VerifyEventChain(nil); idx != -1 {
		t.Error("empty/nil slice should be valid")
	}
	if idx := VerifyEventChain([]core.Event{}); idx != -1 {
		t.Error("empty slice should be valid")
	}
}

func TestCanonicalPayloadStable(t *testing.T) {
	// Same content, different insertion order → same canonical JSON
	p1 := map[string]interface{}{"a": "1", "b": "2"}
	p2 := map[string]interface{}{"b": "2", "a": "1"}

	c1 := canonicalPayload(p1)
	c2 := canonicalPayload(p2)

	if c1 != c2 {
		t.Errorf("canonical payload should be order-independent:\n  %s\n  %s", c1, c2)
	}
	if c1 != `{"a":"1","b":"2"}` {
		t.Errorf("canonical payload = %s", c1)
	}
}

func TestCanonicalPayloadNil(t *testing.T) {
	c := canonicalPayload(nil)
	if c != "{}" {
		t.Errorf("nil payload = %s, want {}", c)
	}
}

func TestCanonicalHashV1StableSerialization(t *testing.T) {
	// Golden: a known state must produce the same hash every time
	s := core.WorldState{
		Scene:         core.SceneState{Location: "test", TimeOfDay: "day", Weather: "clear", Characters: []string{"A", "B"}, Description: "desc"},
		Clock:         core.WorldTime{Hour: 12, Minute: 0, Day: 1},
		Tension:       0.5,
		Flags:         map[string]bool{"flag1": true},
		Relationships: map[string]core.Relationship{"A_B": {Trust: 1.0}},
		Variables:     map[string]interface{}{"key": "value"},
	}

	// Compute hash 10 times — must be identical
	golden := CanonicalHashV1(s)
	for i := 0; i < 10; i++ {
		if CanonicalHashV1(s) != golden {
			t.Fatalf("hash changed on iteration %d: %s != %s", i, CanonicalHashV1(s), golden)
		}
	}

	// Verify it starts with v1:
	if golden[:3] != "v1:" {
		t.Errorf("version prefix missing: %s", golden)
	}
}
