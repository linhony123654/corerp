package httpapi

import (
	"net/http"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func (s *Server) handleRPInteractionRun(w http.ResponseWriter, r *http.Request, requestID, principal string) {
	if !requireMethod(w, requestID, r, http.MethodPost) {
		return
	}
	var input struct {
		core.RPInteractionRequest
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
	provider, err := resolveRPDecisionOverride(input.Model, s.endpointPolicy)
	if err != nil {
		writeError(w, requestID, err)
		return
	}
	result, err := s.service.RunRPInteractionWith(r.Context(), input.RPInteractionRequest, provider)
	if err != nil {
		writeError(w, requestID, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (s *Server) handleRPInteractionResume(w http.ResponseWriter, r *http.Request, requestID, principal string) {
	if !requireMethod(w, requestID, r, http.MethodPost) {
		return
	}
	var input struct {
		storage.RPInteractionResumeRequest
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
	provider, err := resolveRPDecisionOverride(input.Model, s.endpointPolicy)
	if err != nil {
		writeError(w, requestID, err)
		return
	}
	result, err := s.service.ResumeRPInteractionWith(r.Context(), input.RPInteractionResumeRequest, provider)
	if err != nil {
		writeError(w, requestID, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (s *Server) handleRPInteractionStop(w http.ResponseWriter, r *http.Request, requestID, principal string) {
	if !requireMethod(w, requestID, r, http.MethodPost) {
		return
	}
	var input storage.RPInteractionResumeRequest
	if err := decodeJSON(w, r, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(w, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principal); err != nil {
		writeError(w, requestID, err)
		return
	}
	result, err := s.service.StopRPInteraction(r.Context(), input)
	if err != nil {
		writeError(w, requestID, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (s *Server) handleRPInteractionModeRead(w http.ResponseWriter, r *http.Request, requestID, principal string) {
	if !requireMethod(w, requestID, r, http.MethodPost) {
		return
	}
	var input core.RPSessionReadRequest
	if err := decodeJSON(w, r, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(w, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principal); err != nil {
		writeError(w, requestID, err)
		return
	}
	result, err := s.service.ReadRPInteractionMode(r.Context(), input)
	if err != nil {
		writeError(w, requestID, err)
		return
	}
	writeData(w, http.StatusOK, result)
}

func (s *Server) handleRPInteractionModeSet(w http.ResponseWriter, r *http.Request, requestID, principal string) {
	if !requireMethod(w, requestID, r, http.MethodPost) {
		return
	}
	var input storage.RPInteractionModeSetRequest
	if err := decodeJSON(w, r, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(w, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principal); err != nil {
		writeError(w, requestID, err)
		return
	}
	result, err := s.service.SetRPInteractionMode(r.Context(), input)
	if err != nil {
		writeError(w, requestID, err)
		return
	}
	writeData(w, http.StatusOK, result)
}
