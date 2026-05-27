package events

import (
	"encoding/json"
	"testing"

	"corerp/internal/core"
)

// Snapshot is a point-in-time capture of projected world state.
type Snapshot struct {
	Version int             `json:"version"`
	Tick    int64           `json:"tick"`
	State   core.WorldState `json:"state"`
	Hash    string          `json:"hash"`
}

// TakeSnapshot creates a snapshot from the current projection.
func TakeSnapshot(state core.WorldState, tick int64) Snapshot {
	return Snapshot{
		Version: 1,
		Tick:    tick,
		State:   state,
		Hash:    CanonicalHashV1(state),
	}
}

// MarshalSnapshot serializes to JSON bytes.
func MarshalSnapshot(s Snapshot) ([]byte, error) {
	return json.Marshal(s)
}

// UnmarshalSnapshot deserializes from JSON bytes.
func UnmarshalSnapshot(data []byte) (Snapshot, error) {
	var s Snapshot
	err := json.Unmarshal(data, &s)
	return s, err
}

// VerifySnapshot checks that the snapshot hash matches its state.
func (s *Snapshot) Verify() bool {
	return s.Hash == CanonicalHashV1(s.State)
}

// RestoreAndReplay restores from a snapshot and replays remaining events.
// Returns the final state and whether the hash matches a full replay.
func RestoreAndReplay(snap Snapshot, remaining []core.Event) (core.WorldState, string) {
	state := snap.State
	for _, e := range remaining {
		if !e.Canonical {
			continue
		}
		state = applyEvent(state, e)
	}
	return state, CanonicalHashV1(state)
}

// === Tests ===

func TestTakeSnapshotHashMatchesState(t *testing.T) {
	state := core.WorldState{
		Scene:   core.SceneState{Location: "snapshot-test"},
		Tension: 0.42,
	}

	snap := TakeSnapshot(state, 5)
	if snap.Version != 1 {
		t.Errorf("version = %d, want 1", snap.Version)
	}
	if snap.Tick != 5 {
		t.Errorf("tick = %d, want 5", snap.Tick)
	}
	if !snap.Verify() {
		t.Error("snapshot hash must match state")
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	original := TakeSnapshot(core.WorldState{
		Scene:         core.SceneState{Location: "夜之城", TimeOfDay: "夜晚", Weather: "酸雨", Characters: []string{"V", "玩家"}, Description: "测试"},
		Tension:       0.7,
		Relationships: map[string]core.Relationship{"V_玩家": {Trust: 5.0, Fear: 2.0}},
	}, 42)

	data, err := MarshalSnapshot(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	restored, err := UnmarshalSnapshot(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !restored.Verify() {
		t.Error("restored snapshot hash must match state")
	}
	if restored.Hash != original.Hash {
		t.Error("hash must survive round-trip")
	}
	if restored.Tick != original.Tick {
		t.Error("tick must survive round-trip")
	}
}

func TestRestoreSnapshotHashMatchesFullReplay(t *testing.T) {
	// Create events that produce state with tension=0.5
	events := []core.Event{
		{ID: "e1", Type: "scene_init", Payload: map[string]interface{}{"location": "A"}, Canonical: true},
		{ID: "e2", Type: "tension_change", Payload: map[string]interface{}{"delta": 0.3}, Canonical: true},
		{ID: "e3", Type: "tension_change", Payload: map[string]interface{}{"delta": 0.2}, Canonical: true},
	}

	// Full replay
	fullState := Project(events)
	fullHash := CanonicalHashV1(fullState)

	// Take snapshot at event 1 (after scene_init)
	snapState := Project(events[:1])
	snap := TakeSnapshot(snapState, 1)

	// Restore from snapshot + replay remaining events
	restoredState, restoredHash := RestoreAndReplay(snap, events[1:])

	if restoredHash != fullHash {
		t.Errorf("SNAPSHOT RESTORE BROKEN: restore=%s full=%s", restoredHash, fullHash)
	}
	if restoredState.Tension != fullState.Tension {
		t.Errorf("tension: restore=%.2f full=%.2f", restoredState.Tension, fullState.Tension)
	}
}

func TestRestoreAndReplayWithNonCanonical(t *testing.T) {
	events := []core.Event{
		{ID: "e1", Type: "scene_init", Payload: map[string]interface{}{"location": "A"}, Canonical: true},
		{ID: "e2", Type: "tension_change", Payload: map[string]interface{}{"delta": 0.5}, Canonical: true},
		{ID: "e3_q", Type: "unknown_action", Payload: map[string]interface{}{}, Canonical: false}, // quarantined
		{ID: "e4", Type: "tension_change", Payload: map[string]interface{}{"delta": 0.3}, Canonical: true},
	}

	// Full replay skips e3_q
	fullState := Project(events)
	fullHash := CanonicalHashV1(fullState)

	// Snapshot at e1, restore + replay e2..e4
	snap := TakeSnapshot(Project(events[:1]), 1)
	_, restoredHash := RestoreAndReplay(snap, events[1:])

	if restoredHash != fullHash {
		t.Errorf("restore with quarantined events broken: restore=%s full=%s", restoredHash, fullHash)
	}
}

func TestSnapshotTamperDetection(t *testing.T) {
	snap := TakeSnapshot(core.WorldState{Tension: 0.5}, 0)

	// Tamper with state without updating hash
	snap.State.Tension = 99.9

	if snap.Verify() {
		t.Error("should detect tampered snapshot (hash mismatch)")
	}
}
