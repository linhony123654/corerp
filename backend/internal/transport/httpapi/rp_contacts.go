package httpapi

import (
	"corerp.local/backend/internal/storage"
	"net/http"
)

func (s *Server) handleRPContactsRead(w http.ResponseWriter, r *http.Request, requestID, principal string) {
	if !requireMethod(w, requestID, r, http.MethodPost) {
		return
	}
	var input storage.RPContactsReadRequest
	if err := decodeJSON(w, r, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(w, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principal); err != nil {
		writeError(w, requestID, err)
		return
	}
	result, err := s.service.ReadRPContacts(r.Context(), input)
	if err != nil {
		writeError(w, requestID, err)
		return
	}
	writeData(w, http.StatusOK, result)
}
