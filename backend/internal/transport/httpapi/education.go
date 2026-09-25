package httpapi

import (
	"context"
	"net/http"

	"corerp.local/backend/internal/storage"
)

type educationReceipt struct {
	Kind          string `json:"kind"`
	RecordID      string `json:"record_id"`
	EventSequence int64  `json:"event_sequence"`
	WorldTime     string `json:"world_time"`
	Replayed      bool   `json:"replayed"`
}

type educationQualificationRead struct {
	PrincipalID string `json:"principal_id"`
	InstanceID  string `json:"instance_id"`
	BranchID    string `json:"branch_id"`
	LearnerID   string `json:"learner_id"`
}

type educationProgramRead struct {
	PrincipalID   string `json:"principal_id"`
	InstanceID    string `json:"instance_id"`
	BranchID      string `json:"branch_id"`
	LearnerID     string `json:"learner_id"`
	Code          string `json:"code"`
	IssuerID      string `json:"issuer_id"`
	AfterSequence int64  `json:"after_sequence"`
}

// External model/controllers receive business IDs and status only. Immutable
// evidence Event IDs, exercise answers and learner provenance stay in storage.
func handleEducationCommand[T any](s *Server, response http.ResponseWriter, request *http.Request, requestID, principalID string, principal func(*T) *string, execute func(context.Context, T) (storage.EducationRecord, error)) {
	handleBoundCommand(s, response, request, requestID, principalID, principal, func(ctx context.Context, input T) (educationReceipt, error) {
		record, err := execute(ctx, input)
		if err != nil {
			return educationReceipt{}, err
		}
		return educationReceipt{Kind: record.Fact.Kind, RecordID: record.Fact.RecordID, EventSequence: record.EventSequence, WorldTime: record.WorldTime, Replayed: record.Replayed}, nil
	})
}
