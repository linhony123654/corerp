package agents

import (
	"math/rand"
	"testing"

	"corerp/internal/core"
)

type schedulerTestExecutor struct {
	frames []core.ActionFrame
}

func (e *schedulerTestExecutor) Execute(frame core.ActionFrame, state core.WorldState) ([]core.Event, error) {
	e.frames = append(e.frames, frame)
	return nil, nil
}

type schedulerTestSubmitter struct{}

func (s *schedulerTestSubmitter) Submit(evt core.Event, source string) error {
	return nil
}

func TestSelectAdaptiveBestStep(t *testing.T) {
	steps := []PlanStep{
		{Action: "trust", Target: "smugglers", Priority: 7, Reason: "relationship_repair"},
		{Action: "threaten", Target: "smugglers", Priority: 6, Reason: "faction_conflict"},
	}

	aggressive := selectAdaptiveBestStep(steps, map[string]float64{
		"trust":      2,
		"aggression": 6,
	})
	if aggressive.Action != "threaten" {
		t.Fatalf("aggressive best = %#v, want threaten", aggressive)
	}

	trusting := selectAdaptiveBestStep(steps, map[string]float64{
		"trust":    8,
		"intimacy": 6,
	})
	if trusting.Action != "trust" {
		t.Fatalf("trusting best = %#v, want trust", trusting)
	}
}

func TestSchedulerTickFollowsAdaptiveShift(t *testing.T) {
	rand.Seed(1)

	scheduler := NewScheduler()
	scheduler.actionInterval = 0
	scheduler.randomStepChance = 0

	agentsMgr := NewEnvelopeManager()
	agentsMgr.LoadCharacter("guard", core.Character{
		Identity: core.IdentityEnvelope{
			Name:      "guard",
			Adaptive:  map[string]float64{"trust": 2, "aggression": 6},
			Immutable: []string{"stubborn"},
		},
	})
	agentsMgr.LoadCharacter("smugglers", core.Character{
		Identity: core.IdentityEnvelope{
			Name:      "smugglers",
			Adaptive:  map[string]float64{"trust": 3},
			Immutable: []string{"wary"},
		},
	})

	worldState := core.WorldState{
		Scene: core.SceneState{
			Location:   "外城",
			Characters: []string{"guard", "smugglers"},
		},
		Relationships: map[string]core.Relationship{
			"guard_smugglers": {Trust: -1},
		},
		Variables: map[string]interface{}{},
		Flags:     map[string]bool{},
	}
	charWorlds := map[string]core.SceneState{
		"guard":     {Location: "外城", Characters: []string{"guard", "smugglers"}},
		"smugglers": {Location: "外城", Characters: []string{"guard", "smugglers"}},
	}
	structure := core.WorldStructureConfig{
		Factions: []core.WorldFactionConfig{
			{ID: "guard", Name: "guard"},
			{ID: "smugglers", Name: "smugglers"},
		},
		Locations: []core.WorldLocationConfig{
			{ID: "outer_city", Name: "外城", Controller: "guard"},
		},
	}
	executor := &schedulerTestExecutor{}
	submitter := &schedulerTestSubmitter{}

	scheduler.Tick([]string{"guard", "smugglers"}, "focus", charWorlds, worldState, agentsMgr, executor, submitter, 1, structure)
	logs := scheduler.RecentActionsForCharacter("guard", 0)
	if len(logs) == 0 || logs[len(logs)-1].Action != "threaten" {
		t.Fatalf("first guard logs = %#v, want threaten before trust growth", logs)
	}

	agentsMgr.LoadCharacter("guard", core.Character{
		Identity: core.IdentityEnvelope{
			Name:      "guard",
			Adaptive:  map[string]float64{"trust": 8, "intimacy": 6},
			Immutable: []string{"stubborn"},
		},
	})

	scheduler.Tick([]string{"guard", "smugglers"}, "focus", charWorlds, worldState, agentsMgr, executor, submitter, 2, structure)
	logs = scheduler.RecentActionsForCharacter("guard", 0)
	if len(logs) < 2 || logs[len(logs)-1].Action != "trust" {
		t.Fatalf("guard logs after adaptive shift = %#v, want trust", logs)
	}
}
