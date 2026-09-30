package httpapi

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func TestRPChapterStartHTTPHidesPriorTranscript(t *testing.T) {
	ctx := context.Background()
	s, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "chapter-http.db"))
	defer s.Close()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "chapter-http-session"})
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	initial := decodeData[storage.RPObservation](t, response)
	response = performJSON(t, handler, "/api/v1/rp/turns/run", rpPlayerToken, core.RPSpeechRequest{SessionID: session.SessionID, ExpectedCursor: initial.ObservationCursor, Text: "旧篇", IdempotencyKey: "chapter-http-turn"})
	assertStatus(t, response, http.StatusOK)
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	before := decodeData[storage.RPObservation](t, response)
	if len(before.RecentTurns) != 1 {
		t.Fatalf("expected old transcript before reset: %+v", before.RecentTurns)
	}
	request := storage.RPChapterStartRequest{SessionID: session.SessionID, ExpectedCursor: before.ObservationCursor, IdempotencyKey: "chapter-http-reset"}
	response = performJSON(t, handler, "/api/v1/rp/sessions/chapter/start", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	result := decodeData[storage.RPChapterStartResult](t, response)
	if result.SessionID != session.SessionID || result.ChapterStartSequence == 0 || result.Replayed {
		t.Fatalf("invalid chapter result: %+v", result)
	}
	response = performJSON(t, handler, "/api/v1/rp/sessions/chapter/start", rpPlayerToken, request)
	assertStatus(t, response, http.StatusOK)
	if replay := decodeData[storage.RPChapterStartResult](t, response); !replay.Replayed || replay.ChapterStartSequence != result.ChapterStartSequence {
		t.Fatalf("chapter HTTP retry was not exact: %+v", replay)
	}
	response = performJSON(t, handler, "/api/v1/rp/observe", rpPlayerToken, core.RPSessionReadRequest{SessionID: session.SessionID})
	assertStatus(t, response, http.StatusOK)
	after := decodeData[storage.RPObservation](t, response)
	if len(after.RecentTurns) != 0 {
		t.Fatalf("old transcript remained after reset: %+v", after.RecentTurns)
	}
}
