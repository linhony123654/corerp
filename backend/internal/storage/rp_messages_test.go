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

func TestRPMessagesOwnCareerNotPrivateEvaluation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "messages.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	offer := prepareCareerEmploymentOffer(t, s)
	read := allowFixtureControl(t, ctx, s, M2AgentAdaID)
	r := RPMessagesReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID}
	view, err := s.ReadRPMessages(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Messages) != 3 || view.Messages[0].MessageID != offer.EventID || view.Messages[1].Title != "面试答复回执" || view.Messages[2].Body != "Explain safe opening checks." {
		t.Fatalf("actual invitation/response/offer missing: %+v", view)
	}
	for _, m := range view.Messages {
		if m.MessageID == "" || m.Sequence <= 0 || m.WorldTime == "" {
			t.Fatal("missing sourced message metadata")
		}
	}
	encoded, _ := json.Marshal(view)
	for _, private := range []string{"Manager's assessment", "An advisory system recommends", "evaluation", "assessments", "capabilities", "account_id"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("internal employer data leaked: %s", private)
		}
	}
	r.BeforeSequence = view.Messages[0].Sequence
	older, err := s.ReadRPMessages(ctx, r)
	if err != nil || !reflect.DeepEqual(older.Messages, view.Messages[1:]) {
		t.Fatalf("exclusive descending cursor: %+v %v", older, err)
	}
	r.BeforeSequence = view.Messages[2].Sequence
	end, err := s.ReadRPMessages(ctx, r)
	if err != nil || end.Messages == nil || len(end.Messages) != 0 || end.NextBeforeSequence != 0 {
		t.Fatalf("end page: %+v %v", end, err)
	}
	r.BeforeSequence = 0
	accepted, err := s.AcceptCareerOffer(ctx, core.CareerOfferAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "messages-accept"), OfferID: offer.Fact.RecordID, AfterWorkPlaceID: M2AgentCafeID})
	if err != nil {
		t.Fatal(err)
	}
	leave, err := s.RequestCareerLeave(ctx, core.CareerLeaveRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "messages-leave"), LeaveID: "messages-leave", ContractID: accepted.Fact.Employment.ContractID, StartDay: 1, EndDay: 3, Reason: "Private family appointment"})
	if err != nil {
		t.Fatal(err)
	}
	approved, err := s.ReviewCareerLeave(ctx, core.CareerLeaveReviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "messages-approval"), LeaveID: leave.Fact.RecordID, Decision: "approve", Notice: "Two days approved with guaranteed base pay."})
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.ReadRPMessages(ctx, r)
	if err != nil || len(view.Messages) != 5 || view.Messages[0].MessageID != approved.EventID || !strings.Contains(view.Messages[0].Body, "Two days approved") {
		t.Fatalf("own approved notice not projected: %+v %v", view, err)
	}
	other, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "messages-other"})
	if err != nil {
		t.Fatal(err)
	}
	foreignMessages, err := s.ReadRPMessages(ctx, RPMessagesReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: other.SessionID})
	if err != nil || len(foreignMessages.Messages) != 0 {
		t.Fatalf("other candidate read notices: %+v %v", foreignMessages, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := s.ReadRPMessages(ctx, r)
	if err != nil || !reflect.DeepEqual(view, reopened) {
		t.Fatalf("reopen changed message history: %v", err)
	}
	spoof := r
	spoof.PrincipalID = "principal_creator"
	if _, err := s.ReadRPMessages(ctx, spoof); !core.HasCode(err, core.CodeNotFound) {
		t.Fatalf("foreign session read: %v", err)
	}
	bad := r
	bad.BeforeSequence = -1
	if _, err := s.ReadRPMessages(ctx, bad); !core.HasCode(err, core.CodeInvalidArgument) {
		t.Fatalf("negative cursor: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE capability_grants SET status='revoked' WHERE principal_id=? AND subject_id=? AND capability_id='world.rp.control'`, r.PrincipalID, M2AgentAdaID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadRPMessages(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("revoked grant: %v", err)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, view.ObservationCursor)
}

func TestRPMessagesPagesActualInvitations(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "message-pages.db"))
	defer s.Close()
	app := prepareCareerApplicant(t, s)
	read := allowFixtureControl(t, ctx, s, M2AgentAdaID)
	for i := 0; i < 51; i++ {
		id := fmt.Sprintf("message-invite-%02d", i)
		if _, err := s.InviteCareerInterview(ctx, core.CareerInterviewRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, id), InterviewID: id, ApplicationID: app.Fact.RecordID, Question: "Please describe your experience."}); err != nil {
			t.Fatal(err)
		}
	}
	r := RPMessagesReadRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID}
	first, err := s.ReadRPMessages(ctx, r)
	if err != nil || len(first.Messages) != 50 || first.NextBeforeSequence != first.Messages[49].Sequence {
		t.Fatalf("first page: %+v %v", first, err)
	}
	r.BeforeSequence = first.NextBeforeSequence
	second, err := s.ReadRPMessages(ctx, r)
	if err != nil || len(second.Messages) != 1 || second.NextBeforeSequence != 0 || second.Messages[0].Sequence >= r.BeforeSequence {
		t.Fatalf("last page: %+v %v", second, err)
	}
	assertM2Value(t, ctx, s, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, []any{M2DemoInstanceID, M2DemoBranchID}, first.ObservationCursor)
}
