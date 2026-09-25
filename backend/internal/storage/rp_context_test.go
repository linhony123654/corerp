package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPClientContextOwnEvidenceSharedSessionsAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "client-context.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	r := RPContextReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID}
	provider := rpDecisionProviderFunc(func(context.Context, core.RPDecisionInput) (core.RPDecisionProposal, error) {
		return core.RPDecisionProposal{Action: "respond", Text: "我有一百万。"}, nil
	})
	if _, err := s.RunRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "context-speech", Text: "你好。"}, provider); err != nil {
		t.Fatal(err)
	}
	gift := socialRequest(t, ctx, s, read, M2RPNPCID, "gift", "context-gift")
	gift.AmountMinor = 1
	given, err := s.SocialRP(ctx, gift)
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.ReadRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.ReadRPContext(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if view.ProtocolVersion != RPClientProtocolVersion || view.ObserverEntityID != M2RPPlayerID || view.InstanceID != M2DemoInstanceID || view.BranchID != M2DemoBranchID || view.ObservationCursor != given.EventSequence || view.Facts == nil {
		t.Fatalf("invalid binding: %+v", view)
	}
	heard, witnessed := false, false
	for _, fact := range view.Facts {
		if fact.SubjectEntityID != M2RPNPCID || fact.PlaceID != M2AgentCafeID || fact.SourceEventID == "" || fact.LearnedWorldTime == "" {
			t.Fatalf("unknown/unsourced fact: %+v", fact)
		}
		heard = heard || fact.Kind == "speaker_said" && fact.Text == "我有一百万。"
		witnessed = witnessed || fact.Kind == "interpersonal_action" && fact.Action == "gift" && fact.SourceEventID == given.EventID
	}
	if !heard || !witnessed {
		t.Fatalf("missing actual hearing/gift: %+v", view)
	}
	after, err := s.ReadRPSession(ctx, read)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("context read changed session cursor")
	}
	second, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: read.PrincipalID, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "first_person", IdempotencyKey: "second-client"})
	if err != nil {
		t.Fatal(err)
	}
	other := r
	other.SessionID = second.SessionID
	shared, err := s.ReadRPContext(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	shared.SessionID = view.SessionID
	if !reflect.DeepEqual(view, shared) {
		t.Fatal("sessions created separate character memory")
	}
	for _, subject := range []string{M2AgentBoID, "entity_unknown"} {
		unknown := r
		unknown.SubjectEntityID = subject
		if _, err := s.ReadRPContext(ctx, unknown); !core.HasCode(err, core.CodeNotFound) {
			t.Fatalf("guessed unfamiliar subject was not rejected: %s %v", subject, err)
		}
	}
	boAlias, err := rpAnonymousEntityIDForTest(ctx, s, M2DemoInstanceID, M2DemoBranchID, M2RPPlayerID, M2AgentBoID)
	if err != nil {
		t.Fatal(err)
	}
	unknown := r
	unknown.SubjectEntityID = boAlias
	out, err := s.ReadRPContext(ctx, unknown)
	if err != nil || len(out.Facts) != 0 || out.MoreFacts || out.SubjectEntityID != boAlias {
		t.Fatalf("unheard anonymous subject exposed history: %+v %v", out, err)
	}
	limited := r
	limited.Limit = 1
	page, err := s.ReadRPContext(ctx, limited)
	if err != nil || len(page.Facts) != 1 || !page.MoreFacts || page.Facts[0].SourceEventID != given.EventID {
		t.Fatal("context bound/order lost")
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := s.ReadRPContext(ctx, r)
	if err != nil || !reflect.DeepEqual(view, recovered) {
		t.Fatalf("rebuild/reopen changed context: %+v %v", recovered, err)
	}
	foreign := r
	foreign.PrincipalID = "principal_creator"
	if _, err := s.ReadRPContext(ctx, foreign); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("foreign read: %v", err)
	}
	if _, err := s.CloseRPSession(ctx, core.RPSessionReadRequest{PrincipalID: read.PrincipalID, SessionID: second.SessionID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadRPContext(ctx, other); !core.HasCode(err, core.CodeBranchConflict) {
		t.Fatalf("closed session read: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE capability_grants SET status='revoked' WHERE principal_id=? AND capability_id='world.rp.control'`, read.PrincipalID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadRPContext(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("revoked controller read: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, given.EventSequence)
}

func TestRPClientContextBoundsAndNarrowPrivateProjection(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "context-bound.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, _ := newRPWaitTestSession(t, ctx, s)
	r := RPContextReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID}
	for i := 0; i < 51; i++ {
		if _, err := s.SocialRP(ctx, socialRequest(t, ctx, s, read, M2RPNPCID, "greet", fmt.Sprintf("context-%d", i))); err != nil {
			t.Fatal(err)
		}
	}
	for _, limit := range []int{0, 1, 50} {
		r.Limit = limit
		out, err := s.ReadRPContext(ctx, r)
		want := limit
		if want == 0 {
			want = 20
		}
		if err != nil || len(out.Facts) != want || !out.MoreFacts {
			t.Fatalf("limit%d: %+v %v", limit, out, err)
		}
	}
	for _, limit := range []int{-1, 51} {
		r.Limit = limit
		if _, err := s.ReadRPContext(ctx, r); !core.HasCode(err, core.CodeInvalidArgument) {
			t.Fatalf("invalid bound: %v", err)
		}
	}
	r.Limit = 0
	// Corrupt only a disposable projection to prove extra/private payload keys
	// cannot leak through the public DTO. No fabricated authority is committed.
	if _, err := s.db.ExecContext(ctx, `UPDATE agent_knowledge SET claim_payload=json_set(claim_payload,'$.private_goal','secret-evaluation') WHERE observer_agent_id=?`, M2RPPlayerID); err != nil {
		t.Fatal(err)
	}
	out, err := s.ReadRPContext(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(out)
	if err != nil || strings.Contains(string(raw), "secret-evaluation") || strings.Contains(string(raw), "private_goal") {
		t.Fatal("raw knowledge leaked")
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE agent_knowledge SET claim_payload=json_set(claim_payload,'$.claim_type','private_evaluation') WHERE observer_agent_id=?`, M2RPPlayerID); err != nil {
		t.Fatal(err)
	}
	out, err = s.ReadRPContext(ctx, r)
	if err != nil || len(out.Facts) != 0 || out.MoreFacts {
		t.Fatal("unpermitted claim type leaked")
	}
}

func TestRPClientContextSubjectRestrictsKnownFactsNotPrivateCharacter(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "context-subject.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	first, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "context-cafe", Text: "你好。"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: first.SettledSequence, FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", IdempotencyKey: "context-visit"}); err != nil {
		t.Fatal(err)
	}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	last, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "context-home", Text: "你好。"})
	if err != nil {
		t.Fatal(err)
	}
	adaAlias, err := rpAnonymousEntityIDForTest(ctx, s, M2DemoInstanceID, M2DemoBranchID, M2RPPlayerID, M2AgentAdaID)
	if err != nil {
		t.Fatal(err)
	}
	for _, subject := range []string{M2RPNPCID, adaAlias} {
		out, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, SubjectEntityID: subject})
		if err != nil || len(out.Facts) == 0 || out.ObserverEntityID != M2RPPlayerID || out.ObservationCursor != last.SettledSequence {
			t.Fatalf("subject boundary: %+v %v", out, err)
		}
		for _, fact := range out.Facts {
			if fact.SubjectEntityID != subject {
				t.Fatal("subject restriction ignored")
			}
			if subject == M2RPNPCID && fact.PlaceID != M2AgentCafeID {
				t.Fatal("historical known place replaced by current player place")
			}
		}
	}
}
