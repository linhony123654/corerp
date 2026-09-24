package storage

import (
	"context"
	"crypto/sha256"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

var finalClientDatabase = flag.String("final-client-db", "", "create a fresh local Final client acceptance database (test binary only)")
var finalClientLong = flag.Bool("final-client-long", false, "include sourced separated friends and fixed rare policy")
var finalClientAudit = flag.String("final-client-audit-db", "", "audit and rebuild an existing completed local Final client acceptance database")

// This explicit test-only exporter reuses source-owned declarations. It is not
// compiled into the production server and refuses every existing target file.
func TestFinalClientFixtureExport(t *testing.T) {
	if *finalClientDatabase == "" {
		t.Log("opt-in external-client fixture export not requested")
		return
	}
	if !filepath.IsAbs(*finalClientDatabase) {
		t.Fatal("Final client database path must be absolute")
	}
	f, err := os.OpenFile(*finalClientDatabase, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	s := openCareerTestWorld(t, *finalClientDatabase)
	defer s.Close()
	prepareFinalWorld(t, s, careerTime(2, 12, 0))
	if *finalClientLong {
		prepareFinalFriends(t, s)
		if _, err := s.DefineRPOpportunityPolicy(context.Background(), OpportunityPolicyRequest{Binding: careerTestBinding(t, s, "principal_creator", "final-separated-policy"), Policy: RPOpportunityPolicy{StreamSeed: "final-separated-friends-v1", WarmEnabled: true, WarmVisitsEnabled: true, RareVisitBasisPoints: 100, CooldownHours: 6, HistoryHours: 720}}); err != nil {
			t.Fatal(err)
		}
	}
}

// Post-run verification of the actual client's database, not a second simulated
// story. Invoke only after the server has stopped; never create a missing world.
func TestFinalClientWorldAudit(t *testing.T) {
	if *finalClientAudit == "" {
		t.Log("opt-in external-client world audit not requested")
		return
	}
	info, err := os.Stat(*finalClientAudit)
	if err != nil || !filepath.IsAbs(*finalClientAudit) || !info.Mode().IsRegular() {
		t.Fatalf("existing absolute database required: %v", err)
	}
	ctx := context.Background()
	s, err := Open(ctx, *finalClientAudit)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs WHERE status='settled'`, nil, 308)
	eventsBefore := finalClientEventDigest(t, s)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_profiles WHERE status='active'`, nil, 10)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM cohorts WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, 3)
	assertFinalClientSourceBoundaries(t, s)
	before := readCareerTestContext(t, s, "entity_final_nora")
	trust := 0
	for _, relation := range before.Life.Relationships {
		if relation.SubjectEntityID == M2RPPlayerID {
			trust = relation.Trust
		}
	}
	if trust < 10 || before.Life.Background == nil || before.Life.Background.MaterializationEventID == "" || len(before.Life.CultureExperiences) < 2 {
		t.Fatal("actual client run lacks sourced long-term relationship/background/culture")
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatalf("before rebuild: %+v %v", diffs, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatalf("after rebuild: %+v %v", diffs, err)
	}
	after := readCareerTestContext(t, s, "entity_final_nora")
	if after.Life.Background.MaterializationEventID != before.Life.Background.MaterializationEventID {
		t.Fatal("rebuild changed emergent identity")
	}
	if !reflect.DeepEqual(after.Life.Relationships, before.Life.Relationships) || !reflect.DeepEqual(after.Life.CultureExperiences, before.Life.CultureExperiences) {
		t.Fatal("rebuild changed sourced relationship or historical culture experience")
	}
	assertFinalClientSourceBoundaries(t, s)
	if finalClientEventDigest(t, s) != eventsBefore {
		t.Fatal("read/audit/rebuild changed authoritative event rows")
	}
	t.Logf("actual client database:308settled/10individuals/3cohorts; Nora sourced trust%d; before/after projection recovery matches", trust)
	t.Logf("authoritative event rows unchanged through audit/rebuild: sha256:%x", eventsBefore)
}

func finalClientEventDigest(t *testing.T, s *Store) [32]byte {
	t.Helper()
	var snapshot string
	// Include every current events column, with deterministic row order and
	// unmodified payload text. Equal projections alone cannot prove source
	// history was not rewritten to make those projections agree.
	if err := s.db.QueryRowContext(context.Background(), `SELECT json_group_array(json_array(
		event_id,batch_id,instance_id,branch_id,event_sequence,batch_index,event_type,
		actor_id,world_time,causation_event_id,payload))
		FROM (SELECT * FROM events ORDER BY instance_id,branch_id,event_sequence)`).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256([]byte(snapshot))
}

// These checks cover this deterministic client's recorded facts, not arbitrary
// prose semantics or live-model quality. Quote provenance is restricted to the
// same player turn and its committed NPC effects, never a matching older quote.
func assertFinalClientSourceBoundaries(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs t,json_each(t.narrative_json) n
		WHERE n.value LIKE '%：「%」' AND NOT EXISTS (
			SELECT 1 FROM rp_utterances u WHERE instr(n.value,'「'||u.speech_text||'」')>0
			AND (u.turn_id=t.player_turn_id OR EXISTS (
				SELECT 1 FROM rp_npc_decisions d WHERE d.parent_turn_id=t.player_turn_id
				AND d.session_id=t.session_id AND d.event_id=u.event_id)))`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_turn_runs t
		LEFT JOIN rp_turn_styles p ON p.turn_run_id=t.turn_run_id
		WHERE t.status='settled' AND (p.turn_run_id IS NULL OR json_array_length(t.narrative_json)=0)`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge k
		LEFT JOIN events e ON e.event_id=k.source_event_id
		LEFT JOIN observation_records o ON o.observation_id=k.observation_id
		WHERE e.event_id IS NULL OR o.observation_id IS NULL`, nil, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge k
		JOIN events e ON e.event_id=k.source_event_id
		WHERE k.observer_agent_id=? AND e.event_type='RPWarmDecisionRecorded'
		AND json_extract(e.payload,'$.decision.reason')='sourced_visit'`, []any{M2RPPlayerID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*)>0 FROM events e
		JOIN agent_movements m ON m.event_id=e.event_id
		WHERE e.event_type='RPWarmDecisionRecorded'
		AND json_extract(e.payload,'$.decision.reason')='sourced_visit'
		AND json_extract(e.payload,'$.visit_opportunity.rare')=1`, nil, 1)
}
