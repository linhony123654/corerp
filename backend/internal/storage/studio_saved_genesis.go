package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"

	"corerp.local/backend/internal/core"
)

var studioCreationPlanHash = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func validateStudioGenesisRequest(r StudioGenesisRequest) error {
	if r.CreationPlanHash != "" && !studioCreationPlanHash.MatchString(r.CreationPlanHash) {
		return core.NewError(core.CodeInvalidArgument, "invalid creation plan hash")
	}
	if err := r.Spec.Validate(); err != nil {
		return err
	}
	if !studioID(r.IdempotencyKey) {
		return core.NewError(core.CodeInvalidArgument, "bounded genesis request key required")
	}
	_, err := core.StudioWorldObjectID(r.InstanceID, "world", "root")
	return err
}

// Reauthorize before retry reads and bind all preparatory operations to the
// exact accepted immutable declaration, including its original creator/source.
func authorizeSavedStudioGenesis(ctx context.Context, conn *sql.Conn, r StudioGenesisRequest) error {
	if err := validateStudioGenesisRequest(r); err != nil {
		return err
	}
	requestHash, err := core.HashJSON(r)
	if err != nil {
		return err
	}
	event, _ := core.StudioWorldObjectID(r.InstanceID, "event", "genesis")
	cohort, _ := core.StudioWorldObjectID(r.InstanceID, "cohort", "population")
	// Retry is also a privileged operation, so reauthorize before reading receipts.
	if err := authorizeStudioWorldCreation(ctx, conn, r.PrincipalID, r.AuthorityInstanceID, r.AuthorityBranchID); err != nil {
		return err
	}
	var payload, savedHash string
	if err := conn.QueryRowContext(ctx, `SELECT e.payload,c.request_hash FROM events e JOIN event_batches b ON b.batch_id=e.batch_id JOIN commands c ON c.command_id=b.command_id WHERE e.event_id=? AND e.instance_id=? AND e.branch_id='br_main' AND e.event_sequence=1 AND e.event_type='StudioWorldPrepared' AND e.actor_id=? AND c.principal_id=? AND c.command_type='PrepareStudioWorld' AND c.status='committed'`,
		event, r.InstanceID, r.PrincipalID, r.PrincipalID).Scan(&payload, &savedHash); err != nil {
		return classifyMissing(err, "saved Studio genesis")
	}
	var fact struct {
		Request  StudioGenesisRequest `json:"request"`
		CohortID string               `json:"cohort_id"`
	}
	if err := json.Unmarshal([]byte(payload), &fact); err != nil {
		return core.WrapError(core.CodeProjectionDiverged, "decode saved Studio genesis", err)
	}
	factHash, err := core.HashJSON(fact.Request)
	if err != nil {
		return err
	}
	if savedHash != requestHash || factHash != requestHash || fact.CohortID != cohort {
		return core.NewError(core.CodeIdempotencyMismatch, "preparation must match the saved genesis request")
	}

	return nil
}
