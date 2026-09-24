package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

type rpEventsEnvelope struct {
	storage.RPClientEvents
	NextCursor string `json:"next_cursor"`
}

func (s *Server) handleRPEvents(w http.ResponseWriter, r *http.Request, requestID, principal string, stream bool) {
	if !requireMethod(w, requestID, r, http.MethodGet) {
		return
	}
	query := r.URL.Query()
	for key, values := range query {
		if (key != "session_id" && key != "cursor" && key != "limit") || len(values) != 1 {
			writeError(w, requestID, core.NewError(core.CodeInvalidArgument, "events accept only session_id, cursor and limit"))
			return
		}
	}
	input := storage.RPEventsReadRequest{PrincipalID: principal, SessionID: query.Get("session_id")}
	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 50 {
			writeError(w, requestID, core.NewError(core.CodeInvalidArgument, "event limit must be 1–50"))
			return
		}
		input.Limit = limit
	}
	session, err := s.service.ReadRPSession(r.Context(), core.RPSessionReadRequest{PrincipalID: principal, SessionID: input.SessionID})
	if err != nil {
		writeError(w, requestID, err)
		return
	}
	cursor := query.Get("cursor")
	lastID := r.Header.Get("Last-Event-ID")
	if cursor != "" && lastID != "" && cursor != lastID {
		writeError(w, requestID, core.NewError(core.CodeInvalidArgument, "conflicting event cursors"))
		return
	}
	if cursor == "" {
		cursor = lastID
	}
	if cursor != "" {
		input.After, err = s.cursors.DecodeRP(cursor, principal, session.InstanceID, session.BranchID, session.SessionID, session.ControlledEntityID)
		if err != nil {
			writeError(w, requestID, err)
			return
		}
	}
	page, err := s.service.ReadRPEvents(r.Context(), input)
	if err != nil {
		writeError(w, requestID, err)
		return
	}
	encodeCursor := func(sequence int64) (string, error) {
		return s.cursors.EncodeRP(principal, page.InstanceID, page.BranchID, page.SessionID, page.ObserverEntityID, sequence)
	}
	if !stream {
		next, err := encodeCursor(page.NextSequence)
		if err != nil {
			writeError(w, requestID, err)
			return
		}
		writeData(w, http.StatusOK, rpEventsEnvelope{RPClientEvents: page, NextCursor: next})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, requestID, core.NewError(core.CodeStorageFailure, "streaming is unavailable"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	controller := http.NewResponseController(w)
	deadline := func() error {
		err := controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if errors.Is(err, http.ErrNotSupported) {
			return nil
		}
		return err
	}
	frame := func(kind string, sequence int64, value any) error {
		if err := deadline(); err != nil {
			return err
		}
		cursor, err := encodeCursor(sequence)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", cursor, kind, payload)
		return err
	}
	writePage := func() error {
		for _, event := range page.Events {
			if err := frame("rp_event", event.Sequence, event); err != nil {
				return err
			}
		}
		// A checkpoint also advances over invisible events without exposing them.
		// It never jumps beyond the final delivered event while more pages remain.
		if err := frame("rp_checkpoint", page.NextSequence, struct {
			ProtocolVersion string `json:"protocol_version"`
			WorldTime       string `json:"world_time"`
			MoreEvents      bool   `json:"more_events"`
			HistoryRevision int64  `json:"history_revision"`
		}{page.ProtocolVersion, page.WorldTime, page.MoreEvents, page.HistoryRevision}); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	if err := writePage(); err != nil {
		return
	}
	input.After = page.NextSequence
	historyRevision := page.HistoryRevision
	poll, heartbeat := time.NewTicker(s.eventPollInterval), time.NewTicker(s.heartbeatInterval)
	defer poll.Stop()
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-poll.C:
			// Each poll reauthorizes; no transaction remains open during writes.
			page, err = s.service.ReadRPEvents(r.Context(), input)
			if err != nil {
				return
			}
			if page.NextSequence != input.After || len(page.Events) > 0 || page.HistoryRevision != historyRevision {
				if err := writePage(); err != nil {
					return
				}
			}
			input.After = page.NextSequence
			historyRevision = page.HistoryRevision
		case <-heartbeat.C:
			if err := deadline(); err != nil {
				return
			}
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
