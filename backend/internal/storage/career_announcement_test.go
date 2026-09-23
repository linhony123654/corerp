package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestCareerAnnouncementHearingPrivacyAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "announcement.db")
	s := openCareerTestWorld(t, path)
	defer func() { s.Close() }()
	initial, proposal := prepareCareerPositionOffer(t, s, "senior")
	post := careerTestPosting(t, s)
	post.Binding.IdempotencyKey = "manager-post"
	post.Posting.PositionID, post.Posting.Grade, post.Posting.DailyWageMinor = "manager-position", "senior", 30
	post.Posting.RequiredQualifications = []string{"team_coordination"}
	post.Posting.Capabilities = []string{core.CareerPositionManageCapability}
	if _, err := s.PostCareerPosition(ctx, post); err != nil {
		t.Fatal(err)
	}
	proposal.Binding = careerTestBinding(t, s, M2AgentBoPrincipal, "manager-offer")
	proposal.PositionID = post.Posting.PositionID
	if _, err := s.OfferCareerPositionChange(ctx, proposal); err != nil {
		t.Fatal(err)
	}
	changed, err := s.AcceptCareerPositionChange(ctx, core.CareerPositionAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "manager-accept"), ChangeID: proposal.ChangeID})
	if err != nil {
		t.Fatal(err)
	}
	r := core.CareerAnnouncementRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "announce"), AnnouncementID: "promotion-news", ContractID: initial.Fact.Employment.ContractID, SpeakerID: M2AgentAdaID}
	if _, err := s.SpeakCareerAnnouncement(ctx, r); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("not-yet-manager announcement: %v", err)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(3, 12, 0), 100); err != nil {
		t.Fatal(err)
	}
	// Move a coworker away using the real RP path. Being employed in this
	// organization is not permission to hear a conversation in another place.
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: M2RPPlayerPrincipal, InstanceID: M2DemoInstanceID, BranchID: M2DemoBranchID, EntityID: M2RPPlayerID, POV: "second_person", IdempotencyKey: "announcement-session"})
	if err != nil {
		t.Fatal(err)
	}
	read := core.RPSessionReadRequest{PrincipalID: M2RPPlayerPrincipal, SessionID: session.SessionID}
	view, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if view.PlaceID != initial.Fact.Employment.WorkplaceID {
		if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: initial.Fact.Employment.WorkplaceID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "away-from-announcement"}); err != nil {
			t.Fatal(err)
		}
	}
	r.Binding = careerTestBinding(t, s, M2AgentAdaPrincipal, "announce")
	bad := r
	bad.SpeakerID = M2AgentBoID
	if _, err := s.SpeakCareerAnnouncement(ctx, bad); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("manager impersonated another speaker: %v", err)
	}
	before := readCareerTestContext(t, s, M2AgentBoID)
	for _, m := range before.Life.SalientMemories {
		if strings.Contains(m.Text, "Career-management authority") {
			t.Fatal("unannounced promotion became common knowledge")
		}
	}
	s.beforeCommit = func() error { return core.NewError(core.CodeInjectedFailure, "announcement rollback") }
	if _, err := s.SpeakCareerAnnouncement(ctx, r); !core.HasCode(err, core.CodeInjectedFailure) {
		t.Fatalf("rollback: %v", err)
	}
	s.beforeCommit = nil
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM events WHERE json_extract(payload,'$.kind')='announcement'`, nil, 0)
	news, err := s.SpeakCareerAnnouncement(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	a := news.Fact.Announcement
	if a.TermsEventID != changed.EventID || a.PositionID != post.Posting.PositionID || !strings.Contains(a.Text, "senior") {
		t.Fatalf("announcement lacks effective role source: %+v", a)
	}
	boHeard := false
	for _, id := range a.ListenerIDs {
		if id == M2AgentBoID {
			boHeard = true
		}
		if id == M2RPPlayerID || id == M2AgentAdaID {
			t.Fatal("offsite listener or speaker counted as hearer")
		}
	}
	if !boHeard {
		t.Fatalf("colocated manager did not hear: %+v", a.ListenerIDs)
	}
	if strings.Contains(a.Text, "30") || strings.Contains(a.Text, proposal.Assessments[0].Reason) || strings.Contains(a.Text, "review") {
		t.Fatal("private wage/evaluation disclosed")
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM outbox WHERE event_id=?`, []any{news.EventID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE source_event_id=? AND observer_agent_id=?`, []any{news.EventID, M2RPPlayerID}, 0)
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM employment_contracts WHERE contract_id=? AND gross_wage_minor=30 AND position_id=?`, []any{initial.Fact.Employment.ContractID, changed.Fact.Employment.PositionKey}, 1)
	assertMemory := func(observer, source, text string, want bool) {
		t.Helper()
		input := readCareerTestContext(t, s, observer)
		found := false
		for _, m := range input.Life.SalientMemories {
			if m.SourceEventID == source && m.Kind == "speaker_said" && m.Text == text {
				found = true
			}
		}
		if found != want {
			t.Fatalf("heard memory for %s: found=%v want=%v", observer, found, want)
		}
	}
	assertMemory(M2AgentBoID, news.EventID, a.Text, true)
	assertMemory(M2RPPlayerID, news.EventID, a.Text, false)
	// Late arrival does not retroactively acquire the first statement.
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: M2AgentCafeID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "arrive-after-announcement"}); err != nil {
		t.Fatal(err)
	}
	assertMemory(M2RPPlayerID, news.EventID, a.Text, false)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if retry, err := s.SpeakCareerAnnouncement(ctx, r); err != nil || !retry.Replayed || retry.EventID != news.EventID {
		t.Fatalf("announcement retry: %+v %v", retry, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM agent_knowledge WHERE source_event_id=? AND observer_agent_id=?`, []any{news.EventID, M2RPPlayerID}, 0)
	if _, err := s.db.Exec(`DELETE FROM agent_knowledge WHERE source_event_id=?`, news.EventID); err != nil {
		t.Fatal(err)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != len(a.ListenerIDs) {
		t.Fatalf("missing hearing projection: %+v %v", diffs, err)
	}
	if err := s.RebuildProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil {
		t.Fatal(err)
	}
	assertMemory(M2AgentBoID, news.EventID, a.Text, true)
	assertMemory(M2RPPlayerID, news.EventID, a.Text, false)
	again := r
	again.Binding, again.AnnouncementID = careerTestBinding(t, s, M2AgentAdaPrincipal, "announce-again"), "news-again"
	second, err := s.SpeakCareerAnnouncement(ctx, again)
	if err != nil {
		t.Fatal(err)
	}
	assertMemory(M2RPPlayerID, second.EventID, second.Fact.Announcement.Text, true)
	var raw string
	if err := s.db.QueryRow(`SELECT claim_payload FROM agent_knowledge WHERE observer_agent_id=? AND source_event_id=?`, M2RPPlayerID, second.EventID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var claim rpSpeechClaim
	if err := json.Unmarshal([]byte(raw), &claim); err != nil || claim.SpeakerEntityID != M2AgentAdaID || claim.UtteranceID != second.Fact.Announcement.UtteranceID {
		t.Fatalf("speaker attribution: %+v %v", claim, err)
	}
}

func TestCareerAnnouncementUsesCurrentNotPendingPosition(t *testing.T) {
	ctx := context.Background()
	s := openCareerTestWorld(t, filepath.Join(t.TempDir(), "pending-announcement.db"))
	defer s.Close()
	initial, proposal := prepareCareerPositionOffer(t, s, "senior")
	if _, err := s.OfferCareerPositionChange(ctx, proposal); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AcceptCareerPositionChange(ctx, core.CareerPositionAcceptRequest{Binding: careerTestBinding(t, s, M2AgentAdaPrincipal, "accept"), ChangeID: proposal.ChangeID}); err != nil {
		t.Fatal(err)
	}
	r := core.CareerAnnouncementRequest{Binding: careerTestBinding(t, s, M2AgentBoPrincipal, "news"), AnnouncementID: "news", ContractID: initial.Fact.Employment.ContractID, SpeakerID: M2AgentBoID}
	news, err := s.SpeakCareerAnnouncement(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if news.Fact.Announcement.TermsEventID != initial.EventID || news.Fact.Announcement.PositionID != initial.Fact.Employment.PositionID || strings.Contains(news.Fact.Announcement.Text, "senior") {
		t.Fatalf("future promotion announced as current: %+v", news)
	}
	if _, err := s.RunAgentLife(ctx, careerTime(3, 0, 1), 100); err != nil {
		t.Fatal(err)
	}
	// An exact retry remains the old dated statement; a fresh announcement
	// uses current terms, without rewriting what earlier listeners heard.
	if old, err := s.SpeakCareerAnnouncement(ctx, r); err != nil || old.Fact.Announcement.Text != news.Fact.Announcement.Text || !old.Replayed {
		t.Fatalf("historical statement changed: %+v %v", old, err)
	}
	r.Binding, r.AnnouncementID = careerTestBinding(t, s, M2AgentBoPrincipal, "new-news"), "new-news"
	if now, err := s.SpeakCareerAnnouncement(ctx, r); err != nil || !strings.Contains(now.Fact.Announcement.Text, "senior") {
		t.Fatalf("new announcement not current: %+v %v", now, err)
	}
}
