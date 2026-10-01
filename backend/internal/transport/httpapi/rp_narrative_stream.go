package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func (s *Server) handleRPNarrativeStream(w http.ResponseWriter, r *http.Request, requestID, principal string) {
	if !requireMethod(w, requestID, r, http.MethodPost) {
		return
	}
	var input struct {
		storage.RPNarrativeReadRequest
		Model *core.RPModelOverride `json:"model,omitempty"`
	}
	if err := decodeJSON(w, r, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(w, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principal); err != nil {
		writeError(w, requestID, err)
		return
	}
	narrative, err := resolveRPNarrativeOverride(input.Model, s.endpointPolicy)
	if err != nil {
		writeError(w, requestID, err)
		return
	}
	controller := http.NewResponseController(w)
	started := false
	writeFrame := func(frame any) error {
		if err := r.Context().Err(); err != nil {
			return err
		}
		if err := controller.SetWriteDeadline(time.Now().Add(15 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return err
		}
		if !started {
			w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Accel-Buffering", "no")
			started = true
		}
		if err := json.NewEncoder(w).Encode(frame); err != nil {
			return err
		}
		return controller.Flush()
	}
	result, err := s.service.StreamRPNarrativeWith(r.Context(), input.RPNarrativeReadRequest, func(chunk core.RPNarrativeChunk) error {
		return writeFrame(struct {
			Type  string                `json:"type"`
			Chunk core.RPNarrativeChunk `json:"chunk"`
		}{"line", chunk})
	}, narrative)
	if err != nil {
		if !started {
			writeError(w, requestID, err)
		} else {
			// Never append a normal API envelope to a partial NDJSON response or
			// leak an internal storage/provider error to the presentation stream.
			_ = writeFrame(map[string]string{"type": "error", "message": "叙述传输未完成，请重试同一段叙述。"})
		}
		return
	}
	_ = writeFrame(struct {
		Type               string     `json:"type"`
		Count              int        `json:"count"`
		EventIDs           []string   `json:"event_ids"`
		Warnings           []string   `json:"warnings"`
		FallbackReason     string     `json:"fallback_reason,omitempty"`
		RenderID           string     `json:"render_id,omitempty"`
		CompositionVersion string     `json:"composition_version,omitempty"`
		FactGroups         [][]string `json:"fact_groups,omitempty"`
	}{"done", len(result.View.Lines), result.View.EventIDs, result.View.Warnings, result.View.FallbackReason, result.View.RenderID, result.View.CompositionVersion, result.View.FactGroups})
}
