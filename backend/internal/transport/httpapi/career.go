package httpapi

import (
	"context"
	"net/http"

	"corerp.local/backend/internal/storage"
)

func handleCareerCommand[T any](s *Server, response http.ResponseWriter, request *http.Request, requestID, principalID string, principal func(*T) *string, execute func(context.Context, T) (storage.CareerRecord, error)) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input T
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(principal(&input), principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := execute(request.Context(), input)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

type careerMarketRead struct {
	PrincipalID   string `json:"principal_id"`
	InstanceID    string `json:"instance_id"`
	BranchID      string `json:"branch_id"`
	CandidateID   string `json:"candidate_id"`
	AfterSequence int64  `json:"after_sequence"`
}

func (s *Server) handleCareerMarket(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input careerMarketRead
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.DiscoverCareerPositions(request.Context(), input.PrincipalID, input.InstanceID, input.BranchID, input.CandidateID, input.AfterSequence)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

type careerApplicationRead struct {
	PrincipalID   string `json:"principal_id"`
	InstanceID    string `json:"instance_id"`
	BranchID      string `json:"branch_id"`
	ApplicationID string `json:"application_id"`
}

func (s *Server) handleCareerApplicationRead(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input careerApplicationRead
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.ReadCareerApplication(request.Context(), input.PrincipalID, input.InstanceID, input.BranchID, input.ApplicationID)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

type careerRecordRead struct {
	PrincipalID string `json:"principal_id"`
	InstanceID  string `json:"instance_id"`
	BranchID    string `json:"branch_id"`
	Kind        string `json:"kind"`
	RecordID    string `json:"record_id"`
}

type careerAttendanceRead struct {
	PrincipalID string `json:"principal_id"`
	InstanceID  string `json:"instance_id"`
	BranchID    string `json:"branch_id"`
	ContractID  string `json:"contract_id"`
	Day         int    `json:"day"`
}

func (s *Server) handleCareerAttendanceRead(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input careerAttendanceRead
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.ReadCareerAttendance(request.Context(), input.PrincipalID, input.InstanceID, input.BranchID, input.ContractID, input.Day)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}

func (s *Server) handleCareerRecordRead(response http.ResponseWriter, request *http.Request, requestID, principalID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input careerRecordRead
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}
	if err := bindPrincipal(&input.PrincipalID, principalID); err != nil {
		writeError(response, requestID, err)
		return
	}
	result, err := s.service.ReadCareerRecruitmentRecord(request.Context(), input.PrincipalID, input.InstanceID, input.BranchID, input.Kind, input.RecordID)
	if err != nil {
		writeError(response, requestID, err)
		return
	}
	writeData(response, http.StatusOK, result)
}
