package core

import (
	"fmt"
	"reflect"
	"testing"
)

func TestRPHotInitiativeBoundAndFairRotation(t *testing.T) {
	var actors []string
	for i := 999; i >= 0; i-- {
		actors = append(actors, fmt.Sprintf("actor-%04d", i))
	}
	history := map[string]int64{}
	visited := map[string]bool{}
	for turn := 0; turn < 63; turn++ {
		roster, err := SelectRPHotInitiatives(actors, history)
		if err != nil || len(roster) != 16 {
			t.Fatalf("unbounded scene: %d %v", len(roster), err)
		}
		copyRoster, err := SelectRPHotInitiatives(actors, history)
		if err != nil || !reflect.DeepEqual(roster, copyRoster) {
			t.Fatal("same history changed selection")
		}
		for _, actor := range roster {
			visited[actor] = true
			history[actor] = int64(turn*16 + 1)
		}
	}
	if len(visited) != 1000 {
		t.Fatalf("starved actors: visited %d", len(visited))
	}
	if actors[0] != "actor-0999" {
		t.Fatal("selection mutated caller roster")
	}
	for _, invalid := range [][]string{{""}, {"ada", "ada"}} {
		if _, err := SelectRPHotInitiatives(invalid, nil); !HasCode(err, CodeInvalidArgument) {
			t.Fatalf("invalid identity accepted: %v", err)
		}
	}
	if _, err := SelectRPHotInitiatives([]string{"ada"}, map[string]int64{"ada": -1}); !HasCode(err, CodeInvalidArgument) {
		t.Fatal("invalid history accepted")
	}
	// Departed/off-scene history cannot introduce a candidate.
	got, err := SelectRPHotInitiatives([]string{"bo"}, map[string]int64{"ada": 0, "bo": 12})
	if err != nil || !reflect.DeepEqual(got, []string{"bo"}) {
		t.Fatalf("off-scene selection: %v %v", got, err)
	}
}
