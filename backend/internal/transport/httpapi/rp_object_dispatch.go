package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"corerp.local/backend/internal/core"
)

// The existing scene-object route and the newer held-object route share a
// path. Their action verbs are disjoint, so route by verb while retaining the
// normal strict decoder and principal binding for each request shape.
func (s *Server) handleRPObjectAction(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(response, request.Body, s.maxBodyBytes))
	if err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	var action struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(body, &action); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	switch action.Action {
	case "open", "close", "switch_on", "switch_off":
		handleBoundCommand(s, response, request, requestID, principalID, func(r *core.RPSceneObjectActionRequest) *string { return &r.PrincipalID }, s.service.InteractRPSceneObject)
	default:
		handleBoundCommand(s, response, request, requestID, principalID, func(r *core.RPObjectRequest) *string { return &r.PrincipalID }, s.service.ObjectRP)
	}
}
