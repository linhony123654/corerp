package httpapi

import (
	"net/http"

	"corerp.local/backend/internal/core"
)

func (s *Server) handleRPNonverbal(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input struct {
		core.RPNonverbalRequest
		Model *core.RPModelOverride `json:"model,omitempty"`
	}
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	provider, err := resolveRPDecisionOverride(input.Model, s.endpointPolicy)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.NonverbalRPWith(request.Context(), input.RPNonverbalRequest, provider)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}
