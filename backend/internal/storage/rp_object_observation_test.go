package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPObjectWitnessFrozenNeutralAndRecoveredFromEvent(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "object-witness.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	source, _, _ := objectTestSource(t, ctx, s)
	stage := objectTestRequest(t, ctx, s, player, "stage", "witness-stage")
	stage.SourceID = source
	item, err := s.ObjectRP(ctx, stage)
	if err != nil {
		t.Fatal(err)
	}
	var eventRaw, observed, outbox, audience string
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=?`, item.EventID).Scan(&eventRaw); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT claim_payload FROM observation_records WHERE source_event_id=? AND observer_agent_id=?`, item.EventID, M2RPNPCID).Scan(&observed); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT payload,audience_scope FROM outbox WHERE event_id=?`, item.EventID).Scan(&outbox, &audience); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(eventRaw, `"witness_ids"`) || !strings.Contains(eventRaw, M2RPNPCID) || strings.Contains(observed, M2DemoSKUID) || strings.Contains(observed, M2RPPlayerID) || strings.Contains(observed, M2RPNPCID) || strings.Contains(outbox, M2DemoSKUID) || strings.Contains(audience, M2RPNPCID) {
		t.Fatalf("witness evidence leaked SKU/identity or did not freeze sight: Event=%s observed=%s outbox=%s audience=%s", eventRaw, observed, outbox, audience)
	}
	if observed != outbox || strings.Contains(observed, `"source_id"`) || strings.Contains(observed, `"offer_status"`) {
		t.Fatalf("public claim is more specific than witnessed action: %q %q", observed, outbox)
	}
	diffs, err := rpObjectObservationProjectionDifferences(ctx, s.db, M2DemoInstanceID, M2DemoBranchID, item.EventSequence)
	if err != nil || len(diffs) != 0 {
		t.Fatalf("untampered Event witnesses did not replay: %+v %v", diffs, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE observation_records SET claim_payload='{"claim_type":"object_interaction","sku_id":"leaked"}' WHERE source_event_id=? AND observer_agent_id=?`, item.EventID, M2RPNPCID); err != nil {
		t.Fatal(err)
	}
	diffs, err = rpObjectObservationProjectionDifferences(ctx, s.db, M2DemoInstanceID, M2DemoBranchID, item.EventSequence)
	if err != nil || len(diffs) != 1 || diffs[0].Projection != "rp_object_observation" {
		t.Fatalf("tampered witness ignored: %+v %v", diffs, err)
	}
	diffs, err = s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID)
	if err != nil || len(diffs) == 0 {
		t.Fatalf("CompareProjections accepted forged object sight: %+v %v", diffs, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatalf("Event witness could not be recovered: %v", err)
	}
	diffs, err = rpObjectObservationProjectionDifferences(ctx, s.db, M2DemoInstanceID, M2DemoBranchID, item.EventSequence)
	if err != nil || len(diffs) != 0 {
		t.Fatalf("Event witness recovery failed: %+v %v", diffs, err)
	}
	var restored string
	if err := s.db.QueryRowContext(ctx, `SELECT claim_payload FROM agent_knowledge WHERE source_event_id=? AND observer_agent_id=?`, item.EventID, M2RPNPCID).Scan(&restored); err != nil || restored != observed {
		t.Fatalf("rebuild copied tampered witness into Knowledge: %s %v", restored, err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatalf("Event witness repair left projection drift: %+v %v", diffs, err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE outbox SET audience_scope='{"kind":"rp_participants","entity_ids":["malicious"]}' WHERE event_id=?`, item.EventID); err != nil {
		t.Fatal(err)
	}
	diffs, err = rpObjectObservationProjectionDifferences(ctx, s.db, M2DemoInstanceID, M2DemoBranchID, item.EventSequence)
	if err != nil || len(diffs) != 1 || diffs[0].Projection != "rp_object_outbox" {
		t.Fatalf("forged object audience ignored: %+v %v", diffs, err)
	}
}

func TestRPObjectRefusalRequiresActualRecipientAndCancelsOffer(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "object-refusal.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, player, _ := newRPWaitTestSession(t, ctx, s)
	source, _, _ := objectTestSource(t, ctx, s)
	stage := objectTestRequest(t, ctx, s, player, "stage", "refusal-stage")
	stage.SourceID = source
	item, err := s.ObjectRP(ctx, stage)
	if err != nil {
		t.Fatal(err)
	}
	offer := objectTestRequest(t, ctx, s, player, "offer", "refusal-offer")
	offer.ObjectID, offer.TargetEntityID = item.ObjectID, M2RPNPCID
	offered, err := s.ObjectRP(ctx, offer)
	if err != nil {
		t.Fatal(err)
	}
	cai := allowFixtureControl(t, ctx, s, M2RPNPCID)
	refuse := objectTestRequest(t, ctx, s, cai, "refuse", "refusal-by-recipient")
	refuse.OfferID = offered.OfferID
	if _, err := s.ObjectRP(ctx, refuse); err != nil {
		t.Fatal(err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_objects WHERE object_id=? AND owner_actor_id=? AND holder_actor_id=?`, []any{item.ObjectID, M2RPPlayerID, M2RPPlayerID}, 1)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_object_offers WHERE offer_id=? AND status='refused'`, []any{offered.OfferID}, 1)
	failed := objectTestRequest(t, ctx, s, player, "give", "after-refusal")
	failed.OfferID = offered.OfferID
	if _, err := s.ObjectRP(ctx, failed); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("refused offer was transferable: %v", err)
	}
}
