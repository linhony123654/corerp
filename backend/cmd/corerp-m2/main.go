package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func main() {
	databasePath := flag.String("db", "corerp-m2.db", "SQLite database path")
	action := flag.String("action", "bootstrap", "action: bootstrap, materialize, roundtrip, agents, rp-prepare, rp-travel-prepare, rp-life-prepare, rp-final-cohorts-prepare, agents30-prepare, agents30, agents30-drive, economy-day1, or economy30")
	interval := flag.Duration("interval", time.Second, "opt-in Agent driver interval (agents30-drive only)")
	batch := flag.Int("batch", 4, "Agent movements per driver tick (agents30-drive only)")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	if *action == "agents30-drive" {
		err = runAgentDriver(ctx, *databasePath, *interval, *batch, os.Stdout)
	} else {
		err = run(ctx, *databasePath, *action, os.Stdout)
	}
	if err != nil {
		encoded, _ := json.Marshal(struct {
			Error string `json:"error"`
		}{Error: err.Error()})
		fmt.Fprintln(os.Stderr, string(encoded))
		os.Exit(1)
	}
}

func run(ctx context.Context, databasePath, action string, output io.Writer) error {
	store, err := storage.Open(ctx, databasePath)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.BootstrapM2Demo(ctx); err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if action == "bootstrap" {
		return encoder.Encode(map[string]string{
			"status": "ready", "instance_id": storage.M2DemoInstanceID,
			"branch_id": storage.M2DemoBranchID, "cohort_id": storage.M2DemoCohortID,
		})
	}
	if action == "rp-life-prepare" {
		setup, err := store.PrepareRPLifeDemo(ctx)
		if err != nil {
			return err
		}
		return encoder.Encode(setup)
	}
	if action == "rp-final-cohorts-prepare" {
		if _, err := store.PrepareRPLifeDemo(ctx); err != nil {
			return err
		}
		cohorts, err := store.PrepareRPFinalCohorts(ctx)
		if err != nil {
			return err
		}
		return encoder.Encode(cohorts)
	}
	if action == "rp-prepare" {
		setup, err := store.BootstrapRPPlayDemo(ctx)
		if err != nil {
			return err
		}
		return encoder.Encode(setup)
	}
	if action == "rp-travel-prepare" {
		setup, err := store.PrepareRPTravel(ctx)
		if err != nil {
			return err
		}
		return encoder.Encode(setup)
	}
	if action == "agents" {
		setup, err := store.BootstrapM2AgentDemo(ctx)
		if err != nil {
			return err
		}
		run, err := store.RunAgentLife(ctx, storage.M2AgentNoonTime, 4)
		if err != nil {
			return err
		}
		knowledge, err := store.ReadAgentKnowledge(ctx, core.AgentKnowledgeRead{
			PrincipalID: "principal_creator", CapabilityID: "world.agent.knowledge.read",
			InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
			ObserverAgentID: storage.M2AgentAdaID,
			Fields:          []string{"claim_key", "subject_agent_id", "place_id", "learned_world_time", "source_event_id", "observation_id"},
		})
		if err != nil {
			return err
		}
		encounter, err := store.ResolveEncounter(ctx, core.EncounterRead{
			PrincipalID: "principal_creator", CapabilityID: "world.encounter.read",
			InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
			ObserverAgentID: storage.M2AgentAdaID,
			Fields:          []string{"place_id", "world_time", "participants", "activity", "evidence"},
		})
		if err != nil {
			return err
		}
		return encoder.Encode(struct {
			Setup     storage.AgentSetupResult   `json:"setup"`
			Run       storage.AgentLifeRunResult `json:"run"`
			Knowledge storage.AgentKnowledgeView `json:"knowledge"`
			Encounter storage.EncounterView      `json:"encounter"`
		}{setup, run, knowledge, encounter})
	}
	if action == "economy-day1" {
		setup, err := store.BootstrapM2AgentDemo(ctx)
		if err != nil {
			return err
		}
		definition, err := store.PrepareM2EconomicDemo(ctx)
		if err != nil {
			return err
		}
		run, err := store.RunAgentLife(ctx, "2026-09-23T07:01:00Z", 2)
		if err != nil {
			return err
		}
		return encoder.Encode(struct {
			Setup      storage.AgentSetupResult      `json:"setup"`
			Definition storage.M2EconomicSetupResult `json:"definition"`
			Run        storage.AgentLifeRunResult    `json:"run"`
		}{setup, definition, run})
	}
	if action == "economy30" {
		setup, err := store.BootstrapM2AgentDemo(ctx)
		if err != nil {
			return err
		}
		economy, err := store.PrepareM2EconomicDemo(ctx)
		if err != nil {
			return err
		}
		routine, err := store.DefineM2AgentRoutine(ctx, core.AgentRoutineRequest{
			PrincipalID: "principal_creator", CapabilityID: "world.agent.run",
			InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, Days: 30,
		})
		if err != nil {
			return err
		}
		run, err := store.RunAgentLife(ctx, storage.M2AgentDay30NoonTime, 393)
		if err != nil {
			return err
		}
		return encoder.Encode(struct {
			Setup   storage.AgentSetupResult      `json:"setup"`
			Economy storage.M2EconomicSetupResult `json:"economy"`
			Routine storage.AgentRoutineResult    `json:"routine"`
			Run     storage.AgentLifeRunResult    `json:"run"`
		}{setup, economy, routine, run})
	}
	if action == "agents30" || action == "agents30-prepare" {
		setup, err := store.BootstrapM2AgentDemo(ctx)
		if err != nil {
			return err
		}
		definition, err := store.DefineM2AgentRoutine(ctx, core.AgentRoutineRequest{
			PrincipalID: "principal_creator", CapabilityID: "world.agent.run",
			InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, Days: 30,
		})
		if err != nil {
			return err
		}
		if action == "agents30-prepare" {
			return encoder.Encode(struct {
				Setup      storage.AgentSetupResult   `json:"setup"`
				Definition storage.AgentRoutineResult `json:"definition"`
			}{setup, definition})
		}
		result, err := store.RunAgentLife(ctx, storage.M2AgentDay30NoonTime, 120)
		if err != nil {
			return err
		}
		return encoder.Encode(struct {
			Setup      storage.AgentSetupResult   `json:"setup"`
			Definition storage.AgentRoutineResult `json:"definition"`
			Run        storage.AgentLifeRunResult `json:"run"`
		}{setup, definition, result})
	}
	if action != "materialize" && action != "roundtrip" {
		return core.NewError(core.CodeInvalidArgument, "action must be bootstrap, materialize, roundtrip, agents, agents30-prepare, agents30, agents30-drive, economy-day1, or economy30")
	}
	materialized, err := store.MaterializeCohort(ctx, demoMaterializeCommand())
	if err != nil {
		return err
	}
	if action == "materialize" {
		return encoder.Encode(materialized)
	}
	dematerialized, err := store.DematerializeCohort(ctx, demoDematerializeCommand())
	if err != nil {
		return err
	}
	return encoder.Encode(struct {
		Materialized   core.CohortTransitionResult `json:"materialized"`
		Dematerialized core.CohortTransitionResult `json:"dematerialized"`
	}{materialized, dematerialized})
}

func runAgentDriver(ctx context.Context, databasePath string, interval time.Duration, batch int, output io.Writer) error {
	store, err := storage.Open(ctx, databasePath)
	if err != nil {
		return err
	}
	defer store.Close()
	result, err := store.DriveM2AgentRoutine(ctx, interval, batch)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func demoMaterializeCommand() core.MaterializeCohortCommand {
	return core.MaterializeCohortCommand{
		CommandID: "cmd_m2_demo_materialize", MaterializationID: "mat_m2_demo_person",
		InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
		PrincipalID: "principal_creator", CapabilityID: "world.cohort.materialize",
		IdempotencyKey: "idem_m2_demo_materialize", ExpectedHead: 1,
		WorldTime: "2026-09-22T01:00:00Z", SourceCohortID: storage.M2DemoCohortID,
		EntityID: "entity_m2_demo_person", DisplayName: "M2 Demo Person", PopulationCount: 1,
		AssetMinor: 1000, InventoryMinor: 10, ReceivableMinor: 200, LiabilityMinor: 150,
		AllocationAlgorithmVersion: "equal-share-v1",
	}
}

func demoDematerializeCommand() core.DematerializeCohortCommand {
	return core.DematerializeCohortCommand{
		CommandID: "cmd_m2_demo_dematerialize", MaterializationID: "mat_m2_demo_person",
		InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID,
		PrincipalID: "principal_creator", CapabilityID: "world.cohort.dematerialize",
		IdempotencyKey: "idem_m2_demo_dematerialize", ExpectedHead: 2,
		WorldTime: "2026-09-22T02:00:00Z", ReasonCode: "demo_round_trip",
	}
}
