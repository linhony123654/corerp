package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func studioLifeFixture(world string) StudioCreateRequest {
	r := studioCreateFixture(world)
	r.Spec.People = []core.StudioWorldPerson{
		{Key: "lin", Name: "Lin", Place: "home", Player: true, Persona: "玩家角色"},
		{Key: "cai", Name: "Cai", Place: "home", Persona: "管家：精打细算，晨起理账。", Routine: []core.StudioWorldRoutineEntry{
			{WorldTime: "2026-09-22T06:00:00Z", Place: "square", ActivityCode: "market_stroll"},
			{WorldTime: "2026-09-22T08:00:00Z", Place: "home", ActivityCode: "tend_accounts"},
		}},
	}
	r.Spec.Acquaintances = [][2]string{{"lin", "cai"}}
	r.Spec.Relationships = []core.StudioWorldRelationship{{From: "cai", To: "lin", Role: "guardian", AddressTo: []string{"Lin"}, SelfReference: "管家"}}
	return r
}

func assertStudioString(t *testing.T, ctx context.Context, store *Store, query string, args []any, expected string) {
	t.Helper()
	var actual string
	if err := store.db.QueryRowContext(ctx, query, args...).Scan(&actual); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	if actual != expected {
		t.Fatalf("query %q: got %q want %q", query, actual, expected)
	}
}

func TestStudioCreateSeedsPersonaRoutineAndAcquaintances(t *testing.T) {
	ctx := context.Background()
	s := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "life.db"))
	defer func() { s.Close() }()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConfigureStudioAccessLocal(ctx, StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "life-grant"}, TargetPrincipalID: "principal_creator", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	r := studioLifeFixture("life-world")
	out, err := s.CreateStudioWorld(ctx, r)
	if err != nil || out.Status != "ready" {
		t.Fatal("create with life seeding", out, err)
	}
	if out.RPReadiness.Status != "READY" || len(out.RPReadiness.Characters) != 1 {
		t.Fatal("creation did not report RP configuration readiness", out.RPReadiness)
	}
	id := func(kind, key string) string { v, _ := core.StudioWorldObjectID(r.InstanceID, kind, key); return v }
	assertStudioString(t, ctx, s, `SELECT persona_text FROM agent_profiles WHERE agent_id=?`, []any{id("entity", "cai")}, "管家：精打细算，晨起理账。")
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_schedule_entries WHERE agent_id=? AND status='active'`, []any{id("entity", "cai")}, 2)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM scheduler_items WHERE instance_id=? AND status='pending'`, []any{r.InstanceID}, 2)
	assertStudioString(t, ctx, s, `SELECT activity_code FROM agent_schedule_entries WHERE agent_id=? ORDER BY world_time LIMIT 1`, []any{id("entity", "cai")}, "market_stroll")
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_identity_familiarity WHERE instance_id=? AND origin_kind='declared'`, []any{r.InstanceID}, 2)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_identity_familiarity WHERE observer_agent_id=? AND subject_agent_id=?`, []any{id("entity", "lin"), id("entity", "cai")}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_identity_familiarity WHERE observer_agent_id=? AND subject_agent_id=?`, []any{id("entity", "cai"), id("entity", "lin")}, 1)
	if diffs, err := s.CompareProjections(ctx, r.InstanceID, "br_main"); err != nil || len(diffs) != 0 {
		t.Fatal("life seeding projection replay", diffs, err)
	}
	// The declared identity event is the source every familiarity row derives from.
	assertM2Value(t, ctx, s, `SELECT COUNT(DISTINCT source_event_id) FROM rp_identity_familiarity WHERE instance_id=? AND origin_kind='declared'`, []any{r.InstanceID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE instance_id=? AND event_type='RPIdentitiesDeclared'`, []any{r.InstanceID}, 1)
	player := id("entity", "lin")
	npc := id("entity", "cai")
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: r.InstanceID, BranchID: "br_main", EntityID: player, POV: "second_person", IdempotencyKey: "life-rp-context"})
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	speech, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, Text: "早上好", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "life-rp-context-speech"})
	if err != nil {
		t.Fatal(err)
	}
	packet, err := s.BuildRPDecisionInput(ctx, core.RPDecisionRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID, TurnID: speech.TurnID, NPCEntityID: npc})
	if err != nil {
		t.Fatal(err)
	}
	if packet.ContextVersion != core.RPContextVersion || packet.Readiness.Persona != "READY" || packet.Readiness.RelationshipToInterlocutor != "READY" || packet.Readiness.AddressToInterlocutor != "READY" || len(packet.Relationships) != 1 || packet.Relationships[0].SubjectEntityID != player || packet.Relationships[0].Role != "guardian" || packet.Relationships[0].AddressTo[0] != "Lin" || packet.Relationships[0].SourceEventID == "" || packet.PersonaSourceEventID == "" {
		t.Fatalf("authored RP context lost source or readiness: %+v", packet)
	}
	if out.RPReadiness.Characters[0].Readiness != packet.Readiness {
		t.Fatal("creator receipt does not match committed character context", out.RPReadiness, packet.Readiness)
	}
	providerView, err := s.rpDecisionProviderView(ctx, packet)
	if err != nil || len(providerView.Relationships) != 1 || providerView.Relationships[0].SubjectEntityID != player {
		t.Fatalf("known relationship was masked or lost: %+v, %v", providerView.Relationships, err)
	}
}

func TestStudioLifeSpecValidation(t *testing.T) {
	bad := func(mutate func(*core.StudioWorldSpec)) {
		spec := studioLifeFixture("unused").Spec
		mutate(&spec)
		if err := spec.Validate(); err == nil {
			t.Fatal("invalid life seeding accepted")
		}
	}
	bad(func(s *core.StudioWorldSpec) { s.Acquaintances = [][2]string{{"lin", "lin"}} })
	bad(func(s *core.StudioWorldSpec) { s.Acquaintances = [][2]string{{"lin", "ghost"}} })
	bad(func(s *core.StudioWorldSpec) { s.Acquaintances = [][2]string{{"lin", "cai"}, {"cai", "lin"}} })
	bad(func(s *core.StudioWorldSpec) { s.Relationships[0].To = "ghost" })
	bad(func(s *core.StudioWorldSpec) { s.Acquaintances = nil })
	bad(func(s *core.StudioWorldSpec) { s.Relationships[0].AddressTo = []string{"Lin", "Lin"} })
	bad(func(s *core.StudioWorldSpec) {
		s.People[1].Routine = []core.StudioWorldRoutineEntry{{WorldTime: "2026-09-21T23:00:00Z", Place: "home", ActivityCode: "early"}}
	})
	bad(func(s *core.StudioWorldSpec) {
		s.People[1].Routine = []core.StudioWorldRoutineEntry{{WorldTime: "2026-09-22T08:00:00Z", Place: "home", ActivityCode: "late"}, {WorldTime: "2026-09-22T07:00:00Z", Place: "home", ActivityCode: "early"}}
	})
	bad(func(s *core.StudioWorldSpec) {
		s.People[1].Routine = []core.StudioWorldRoutineEntry{{WorldTime: "2026-09-22T08:00:00Z", Place: "home", ActivityCode: "Bad Code!"}}
	})
	bad(func(s *core.StudioWorldSpec) { s.People[1].Persona = strings.Repeat("中", 501) })
	bad(func(s *core.StudioWorldSpec) { s.People[1].Persona = "含坏\x00字符" })
	good := studioLifeFixture("unused").Spec
	if err := good.Validate(); err != nil {
		t.Fatal("valid life seeding rejected", err)
	}
	// Personas and routines are optional; plain specs still validate untouched.
	plain := studioCreateFixture("unused").Spec
	if err := plain.Validate(); err != nil {
		t.Fatal("plain spec rejected", err)
	}
}
