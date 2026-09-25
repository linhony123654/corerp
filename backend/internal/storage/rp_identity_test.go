package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"corerp.local/backend/internal/core"
)

func rpAnonymousEntityIDForTest(ctx context.Context, s *Store, instance, branch, observer, subject string) (string, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return rpAnonymousEntityID(ctx, conn, instance, branch, observer, subject)
}

func rpAnonymousEvidenceIDForTest(ctx context.Context, s *Store, instance, branch, observer, source string) (string, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return rpAnonymousEvidenceID(ctx, conn, instance, branch, observer, source)
}

func TestRPAnonymousHandlesNeedPrivatePersistentDatabaseKey(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "keyed-handles.db")
	first, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	alias, err := rpAnonymousEntityIDForTest(ctx, first, M2DemoInstanceID, M2DemoBranchID, M2RPPlayerID, M2AgentAdaID)
	if err != nil || !strings.HasPrefix(alias, "person_") {
		t.Fatal("missing keyed alias", alias, err)
	}
	publicHash, err := core.HashJSON([]string{"rp-anonymous-v1", "person", M2DemoInstanceID, M2DemoBranchID, M2RPPlayerID, M2AgentAdaID})
	if err != nil || alias == "person_"+strings.TrimPrefix(publicHash, "sha256:")[:24] {
		t.Fatal("alias can be enumerated from public inputs", alias, err)
	}
	if _, err := first.db.ExecContext(ctx, `UPDATE rp_identity_alias_secret SET secret=randomblob(32) WHERE singleton=1`); err == nil {
		t.Fatal("presentation key was mutable")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	stable, err := rpAnonymousEntityIDForTest(ctx, reopened, M2DemoInstanceID, M2DemoBranchID, M2RPPlayerID, M2AgentAdaID)
	if err != nil || stable != alias {
		t.Fatal("restart changed anonymous handle", stable, alias, err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	other, err := Open(ctx, filepath.Join(t.TempDir(), "other-key.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	unrelated, err := rpAnonymousEntityIDForTest(ctx, other, M2DemoInstanceID, M2DemoBranchID, M2RPPlayerID, M2AgentAdaID)
	if err != nil || unrelated == alias {
		t.Fatal("independent DB reused another private handle", unrelated, alias, err)
	}
}

func TestRPIdentityNeedsKnownSourceOrHeardIntroduction(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "identity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, playerRead, playerView := newRPWaitTestSession(t, ctx, s)
	if len(playerView.PresentEntities) != 1 || !playerView.PresentEntities[0].Identified || playerView.PresentEntities[0].EntityID != M2RPNPCID {
		t.Fatal("declared demo acquaintance missing", playerView.PresentEntities)
	}
	grantRPControlForTest(t, ctx, s, M2AgentAdaID)
	ada, err := s.OpenRPSession(ctx, rpTestOpenRequest())
	if err != nil {
		t.Fatal(err)
	}
	adaRead := core.RPSessionReadRequest{PrincipalID: rpTestPrincipal, SessionID: ada.SessionID}
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: playerRead.PrincipalID, SessionID: playerRead.SessionID, FromPlaceID: playerView.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: playerView.ObservationCursor, IdempotencyKey: "meet-unknown-ada"}); err != nil {
		t.Fatal(err)
	}
	playerView, err = s.ObserveRPSession(ctx, playerRead)
	alias, err := rpAnonymousEntityIDForTest(ctx, s, M2DemoInstanceID, M2DemoBranchID, M2RPPlayerID, M2AgentAdaID)
	if err != nil {
		t.Fatal(err)
	}
	if len(playerView.PresentEntities) != 1 || playerView.PresentEntities[0].EntityID != alias || playerView.PresentEntities[0].Identified || playerView.PresentEntities[0].DisplayName != "陌生人" {
		t.Fatal("sight incorrectly granted identity", playerView.PresentEntities, err)
	}
	if _, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: playerRead.PrincipalID, SessionID: playerRead.SessionID, SubjectEntityID: M2AgentAdaID}); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("guessed authoritative ID resolved an unfamiliar context subject", err)
	}
	if _, err := s.SocialRP(ctx, core.RPSocialRequest{PrincipalID: playerRead.PrincipalID, SessionID: playerRead.SessionID, TargetEntityID: M2AgentAdaID, Action: "greet", ExpectedCursor: playerView.ObservationCursor, IdempotencyKey: "guess-unknown-ada"}); !core.HasCode(err, core.CodeNotFound) {
		t.Fatal("guessed authoritative ID targeted an unfamiliar person", err)
	}
	contextView, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: playerRead.PrincipalID, SessionID: playerRead.SessionID, SubjectEntityID: alias})
	if err != nil || contextView.SubjectEntityID != alias || len(contextView.Facts) == 0 {
		t.Fatal("anonymous context is not readable by its public handle", contextView, err)
	}
	action, err := s.SocialRP(ctx, core.RPSocialRequest{PrincipalID: playerRead.PrincipalID, SessionID: playerRead.SessionID, TargetEntityID: alias, Action: "greet", ExpectedCursor: playerView.ObservationCursor, IdempotencyKey: "greet-unknown-ada"})
	if err != nil || strings.Contains(action.Description, "Ada") {
		t.Fatal("anonymous target was not interactable or social text named them", action, err)
	}
	contextView, err = s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: playerRead.PrincipalID, SessionID: playerRead.SessionID, SubjectEntityID: alias})
	if err != nil {
		t.Fatal(err)
	}
	eventsView, err := s.ReadRPEvents(ctx, RPEventsReadRequest{PrincipalID: playerRead.PrincipalID, SessionID: playerRead.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{playerView, contextView, eventsView, action} {
		encoded, err := json.Marshal(value)
		if err != nil || strings.Contains(string(encoded), M2AgentAdaID) {
			t.Fatal("anonymous client response exposed the authoritative ID", string(encoded), err)
		}
	}
	adaView, err := s.ObserveRPSession(ctx, adaRead)
	if err != nil {
		t.Fatal(err)
	}
	intro, err := s.SpeakRP(ctx, core.RPSpeechRequest{PrincipalID: adaRead.PrincipalID, SessionID: adaRead.SessionID, ExpectedCursor: adaView.ObservationCursor, IdempotencyKey: "ada-introduces-self", Text: "我叫 Ada。", IntroduceSelf: true})
	if err != nil || len(intro.ListenerIDs) != 1 || intro.ListenerIDs[0] != M2RPPlayerID {
		t.Fatal("introduction was not heard", intro, err)
	}
	playerView, err = s.ObserveRPSession(ctx, playerRead)
	if err != nil || len(playerView.PresentEntities) != 1 || !playerView.PresentEntities[0].Identified || playerView.PresentEntities[0].DisplayName != "Ada" || playerView.PresentEntities[0].EntityID != M2AgentAdaID {
		t.Fatal("heard introduction did not grant identity", playerView.PresentEntities, err)
	}
	knownContext, err := s.ReadRPContext(ctx, RPContextReadRequest{PrincipalID: playerRead.PrincipalID, SessionID: playerRead.SessionID, SubjectEntityID: alias})
	if err != nil || knownContext.SubjectEntityID != M2AgentAdaID {
		t.Fatal("old public handle did not resolve to newly known identity", knownContext, err)
	}
	assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_identity_familiarity WHERE observer_agent_id=? AND subject_agent_id=? AND source_event_id=?`, []any{M2RPPlayerID, M2AgentAdaID, intro.EventID}, 1)
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("identity did not preserve replay", diffs, err)
	}
}

func TestRPIdentityMigrationPreservesOldDemoAcquaintance(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "identity-upgrade.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	// Recreate the pre-035 state in this disposable test database, then reopen
	// through the actual migration runner with a committed setup Event.
	for _, query := range []string{
		`DROP TABLE rp_identity_alias_secret`,
		`DROP TABLE rp_identity_legacy_sources`,
		`DROP TABLE rp_identity_cutovers`,
		`DROP TABLE rp_identity_familiarity`,
		`DELETE FROM schema_meta WHERE schema_version='corerp-f2-spatial-identity-035-2026-09-25'`,
	} {
		if _, err := s.db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, pair := range [][2]string{{M2RPPlayerID, M2RPNPCID}, {M2RPNPCID, M2RPPlayerID}} {
		assertM2Value(t, ctx, s, `SELECT COUNT(*) FROM rp_identity_familiarity WHERE observer_agent_id=? AND subject_agent_id=? AND origin_kind='demo' AND source_event_id=?`, []any{pair[0], pair[1], m2RPSetupEventID}, 1)
	}
	if diffs, err := s.CompareProjections(ctx, M2DemoInstanceID, M2DemoBranchID); err != nil || len(diffs) != 0 {
		t.Fatal("upgraded identity differs from sources", diffs, err)
	}
}

func TestRPNarrativeDoesNotNameUnintroducedRespondent(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "anonymous-narrative.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, read, view := newRPWaitTestSession(t, ctx, s)
	if _, err := s.MoveRP(ctx, core.RPMoveRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, FromPlaceID: view.PlaceID, ToPlaceID: "place_m2_home_ada", ExpectedCursor: view.ObservationCursor, IdempotencyKey: "narrative-anonymous-move"}); err != nil {
		t.Fatal(err)
	}
	view, err = s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := s.PlayRPTurn(ctx, core.RPSpeechRequest{PrincipalID: read.PrincipalID, SessionID: read.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "narrative-anonymous-speech", Text: "你好，你是谁？"})
	if err != nil || len(turn.NPCEventIDs) == 0 {
		t.Fatal("unintroduced respondent did not participate", turn, err)
	}
	input, err := s.readRPNarrativeInput(ctx, read.SessionID, turn.PlayerTurnID, turn.PlayerEventID)
	if err != nil || len(input.Facts) < 2 {
		t.Fatal("narrative lacked source-backed respondent", input, err)
	}
	for _, fact := range input.Facts[1:] {
		if fact.ActorID == M2AgentAdaID || fact.ActorName != "陌生人" {
			t.Fatal("narrative input disclosed unintroduced identity", fact)
		}
	}
}
