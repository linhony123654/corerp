package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

// Transport-only notification control. The actual late-settlement/storage
// invariant is separately exercised with an interrupted real turn in storage.
type lateHistoryService struct {
	Service
	revision atomic.Int64
}

func (s *lateHistoryService) ReadRPEvents(ctx context.Context, r storage.RPEventsReadRequest) (storage.RPClientEvents, error) {
	page, err := s.Service.ReadRPEvents(ctx, r)
	page.HistoryRevision += s.revision.Load()
	return page, err
}

func TestRPClientEventsHTTPLateHistoryCheckpoint(t *testing.T) {
	ctx := context.Background()
	s, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "late-checkpoint.db"))
	defer s.Close()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	session, err := s.OpenRPSession(ctx, core.RPSessionOpenRequest{PrincipalID: storage.M2RPPlayerPrincipal, InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "late-checkpoint"})
	if err != nil {
		t.Fatal(err)
	}
	api := handler.(*Server)
	service := &lateHistoryService{Service: api.service}
	api.service = service
	server := httptest.NewServer(api)
	defer server.Close()
	streamCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(streamCtx, http.MethodGet, server.URL+"/api/v1/rp/events/stream?session_id="+url.QueryEscape(session.SessionID), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+rpPlayerToken)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("stream status %d", response.StatusCode)
	}
	scanner := bufio.NewScanner(response.Body)
	checkpoint := func() (int64, int64) {
		t.Helper()
		var kind, cursor, payload string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "id: ") {
				cursor = strings.TrimPrefix(line, "id: ")
			}
			if strings.HasPrefix(line, "event: ") {
				kind = strings.TrimPrefix(line, "event: ")
			}
			if strings.HasPrefix(line, "data: ") {
				payload = strings.TrimPrefix(line, "data: ")
			}
			if line == "" && kind == "rp_checkpoint" {
				var data struct {
					HistoryRevision int64 `json:"history_revision"`
				}
				if err := json.Unmarshal([]byte(payload), &data); err != nil {
					t.Fatal(err)
				}
				sequence, err := api.cursors.DecodeRP(cursor, storage.M2RPPlayerPrincipal, session.InstanceID, session.BranchID, session.SessionID, session.ControlledEntityID)
				if err != nil {
					t.Fatal(err)
				}
				return sequence, data.HistoryRevision
			}
		}
		t.Fatalf("late checkpoint missing: %v", scanner.Err())
		return 0, 0
	}
	head, revision := checkpoint()
	if revision != 0 {
		t.Fatalf("initial revision %d", revision)
	}
	service.revision.Store(1)
	laterHead, laterRevision := checkpoint()
	if laterHead != head || laterRevision != 1 {
		t.Fatalf("same-head refresh missing: %d/%d -> %d/%d", head, revision, laterHead, laterRevision)
	}
}

func TestRPClientEventsHTTPReconnectAndSessionClose(t *testing.T) {
	ctx := context.Background()
	s, handler := openHTTPTestServer(t, ctx, filepath.Join(t.TempDir(), "events-http.db"))
	defer s.Close()
	if _, err := s.PrepareRPTravel(ctx); err != nil {
		t.Fatal(err)
	}
	open := core.RPSessionOpenRequest{InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, EntityID: storage.M2RPPlayerID, POV: "second_person", IdempotencyKey: "events-http"}
	response := performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, open)
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	read := core.RPSessionReadRequest{PrincipalID: storage.M2RPPlayerPrincipal, SessionID: session.SessionID}
	initial, err := s.ObserveRPSession(ctx, read)
	if err != nil {
		t.Fatal(err)
	}
	response = performJSON(t, handler, "/api/v1/rp/turns/run", rpPlayerToken, core.RPSpeechRequest{SessionID: session.SessionID, ExpectedCursor: initial.ObservationCursor, IdempotencyKey: "events-http-turn", Text: "你好。"})
	assertStatus(t, response, http.StatusOK)
	base := "/api/v1/rp/events?session_id=" + url.QueryEscape(session.SessionID)
	get := func(route, token, cursor string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, route, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if cursor != "" {
			r.Header.Set("Last-Event-ID", cursor)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	response = get(base+"&limit=1", rpPlayerToken, "")
	assertStatus(t, response, http.StatusOK)
	first := decodeData[rpEventsEnvelope](t, response)
	if len(first.Events) != 1 || !first.MoreEvents || first.NextCursor == "" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("invalid first page: %+v", first)
	}
	response = get(base, rpPlayerToken, first.NextCursor)
	assertStatus(t, response, http.StatusOK)
	rest := decodeData[rpEventsEnvelope](t, response)
	if len(rest.Events) == 0 || rest.Events[0].Sequence <= first.NextSequence {
		t.Fatal("reconnect skipped or duplicated events")
	}
	assertStatus(t, get(base, "", ""), http.StatusUnauthorized)
	assertStatus(t, get(base, creatorToken, ""), http.StatusNotFound)
	assertStatus(t, get(base+"&observer_entity_id=hidden", rpPlayerToken, ""), http.StatusBadRequest)
	assertStatus(t, get(base+"&limit=0", rpPlayerToken, ""), http.StatusBadRequest)
	assertStatus(t, get(base+"&cursor=bad", rpPlayerToken, ""), http.StatusBadRequest)
	assertStatus(t, get(base+"&cursor=bad", rpPlayerToken, first.NextCursor), http.StatusBadRequest)
	open.IdempotencyKey = "events-http-other"
	response = performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, open)
	assertStatus(t, response, http.StatusOK)
	other := decodeData[storage.RPSession](t, response)
	assertStatus(t, get("/api/v1/rp/events?session_id="+url.QueryEscape(other.SessionID), rpPlayerToken, first.NextCursor), http.StatusForbidden)
	server := httptest.NewServer(handler)
	defer server.Close()
	streamCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(streamCtx, http.MethodGet, server.URL+"/api/v1/rp/events/stream?session_id="+url.QueryEscape(session.SessionID), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+rpPlayerToken)
	request.Header.Set("Last-Event-ID", first.NextCursor)
	stream, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	if stream.StatusCode != http.StatusOK || !strings.HasPrefix(stream.Header.Get("Content-Type"), "text/event-stream") || stream.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("stream headers: %d %+v", stream.StatusCode, stream.Header)
	}
	scanner := bufio.NewScanner(stream.Body)
	var kind, cursor string
	var events []storage.RPClientEvent
	checkpoint := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "id: ") {
			cursor = strings.TrimPrefix(line, "id: ")
		}
		if strings.HasPrefix(line, "event: ") {
			kind = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") && kind == "rp_event" {
			var event storage.RPClientEvent
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
				t.Fatal(err)
			}
			events = append(events, event)
		}
		if line == "" && kind == "rp_checkpoint" {
			checkpoint = true
			break
		}
	}
	if !checkpoint || scanner.Err() != nil || len(events) != len(rest.Events) || cursor == "" {
		t.Fatalf("stream continuation: events=%d checkpoint=%v err=%v", len(events), checkpoint, scanner.Err())
	}
	for i := range events {
		if events[i].EventID != rest.Events[i].EventID {
			t.Fatal("stream/JSON divergence")
		}
	}
	response = get(base, rpPlayerToken, cursor)
	assertStatus(t, response, http.StatusOK)
	if tail := decodeData[rpEventsEnvelope](t, response); len(tail.Events) != 0 {
		t.Fatal("checkpoint replayed delivered events")
	}
	if _, err := s.CloseRPSession(ctx, read); err != nil {
		t.Fatal(err)
	}
	// The live poll must terminate rather than trusting the old cursor forever.
	if _, err := io.Copy(io.Discard, stream.Body); err != nil {
		t.Fatalf("closed session did not terminate stream: %v", err)
	}
	if streamCtx.Err() != nil {
		t.Fatal("stream ended only because test timed out")
	}
	assertStatus(t, get(base, rpPlayerToken, cursor), http.StatusConflict)
}
