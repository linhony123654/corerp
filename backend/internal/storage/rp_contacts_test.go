package storage

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestRPContactsOwnHearingPersistencePrivacyAndCursor(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "contacts.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	_, read, initial := newRPWaitTestSession(t, ctx, s)
	r := RPContactsReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID}
	before, err := s.ReadRPContacts(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	for _, contact := range before.Contacts {
		if contact.EntityID == M2AgentAdaID || contact.EntityID == M2AgentBoID || contact.EntityID == M2RPPlayerID {
			t.Fatalf("unknown/self entity exposed: %+v", contact)
		}
	}
	turn, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "contacts-cafe", Text: "你好。"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: turn.SettledSequence, FromPlaceID: M2AgentCafeID, ToPlaceID: "place_m2_home_ada", IdempotencyKey: "contacts-move"})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	turn, err = s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: observation.ObservationCursor, IdempotencyKey: "contacts-ada", Text: "你好。"})
	if err != nil {
		t.Fatal(err)
	}
	unintroduced, err := s.ReadRPContacts(ctx, r)
	if err != nil || len(unintroduced.Contacts) != 1 || unintroduced.Contacts[0].EntityID != M2RPNPCID {
		t.Fatalf("hearing an unfamiliar person revealed their name: %+v %v", unintroduced, err)
	}
	grantRPControlForTest(t, ctx, s, M2AgentAdaID)
	ada, err := s.OpenRPSession(ctx, rpTestOpenRequest())
	if err != nil {
		t.Fatal(err)
	}
	adaRead := core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: ada.SessionID}
	adaView, err := s.ObserveRPSession(ctx, adaRead)
	if err != nil {
		t.Fatal(err)
	}
	intro, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: adaRead.PrincipalID, SessionID: adaRead.SessionID, ExpectedCursor: adaView.ObservationCursor, IdempotencyKey: "contacts-ada-intro", Text: "我叫 Ada。", IntroduceSelf: true})
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.ReadRPContacts(ctx, r)
	if err != nil || len(after.Contacts) != 2 {
		t.Fatalf("actual hearing not reflected: %+v %v", after, err)
	}
	known := map[string]bool{}
	for _, contact := range after.Contacts {
		known[contact.EntityID] = true
		if contact.DisplayName == "" || contact.LastKnownWorldTime == "" {
			t.Fatal("contact lacks known evidence metadata")
		}
	}
	if !known[M2RPNPCID] || !known[M2AgentAdaID] || known[M2AgentBoID] {
		t.Fatalf("contact privacy boundary wrong: %+v", after)
	}
	r.AfterEntityID = after.Contacts[0].EntityID
	page, err := s.ReadRPContacts(ctx, r)
	if err != nil || len(page.Contacts) != 1 || page.Contacts[0] != after.Contacts[1] {
		t.Fatalf("stable seek cursor wrong: %+v %v", page, err)
	}
	r.AfterEntityID = after.Contacts[1].EntityID
	page, err = s.ReadRPContacts(ctx, r)
	if err != nil || len(page.Contacts) != 0 || page.NextAfterEntityID != "" {
		t.Fatal("last contact does not terminate paging")
	}
	r.AfterEntityID = ""
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := s.ReadRPContacts(ctx, r)
	if err != nil || !reflect.DeepEqual(reopened, after) {
		t.Fatalf("contact knowledge changed on reopen: %+v %v", reopened, err)
	}
	foreign := r
	foreign.PrincipalID = "principal_creator"
	if _, err := s.ReadRPContacts(ctx, foreign); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("foreign session accepted: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE capability_grants SET status='revoked' WHERE principal_id=? AND capability_id='world.rp.control'`, r.PrincipalID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadRPContacts(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("revoked controller read contacts: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, intro.EventSequence)
}
