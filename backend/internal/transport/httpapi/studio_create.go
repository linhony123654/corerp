package httpapi

import (
	"net/http"

	"corerp.local/backend/internal/storage"
)

func (s *Server) handleStudioCreate(w http.ResponseWriter, r *http.Request, requestID, principal string) {
	if !requireMethod(w, requestID, r, http.MethodPost) {
		return
	}
	var input storage.StudioCreateRequest
	if err := decodeJSON(w, r, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(w, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principal); err != nil {
		writeError(w, requestID, err)
		return
	}
	out, err := s.service.CreateStudioWorld(r.Context(), input)
	if err != nil {
		writeError(w, requestID, err)
		return
	}
	status := http.StatusCreated
	if out.Replayed {
		status = http.StatusOK
	}
	writeData(w, status, out)
}
