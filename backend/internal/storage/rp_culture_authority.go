package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
)

func authorizeCultureBuilder(ctx context.Context, conn *sql.Conn, b core.CareerBinding) (string, error) {
	var source string
	err := conn.QueryRowContext(ctx, `SELECT g.definition_event_id FROM capability_grants g JOIN principals p ON p.principal_id=g.principal_id JOIN events e ON e.event_id=g.definition_event_id AND e.instance_id=g.instance_id AND e.branch_id=g.branch_id JOIN cohorts c ON c.cohort_id=g.subject_id AND c.instance_id=g.instance_id AND c.branch_id=g.branch_id AND c.definition_event_id=e.event_id WHERE g.principal_id=? AND p.principal_type='creator' AND p.status='active' AND g.instance_id=? AND g.branch_id=? AND g.capability_id='world.cohort.materialize' AND g.status='active' AND e.event_sequence=1 ORDER BY g.grant_id LIMIT 1`, b.PrincipalID, b.InstanceID, b.BranchID).Scan(&source)
	if err == sql.ErrNoRows {
		return "", core.NewError(core.CodeUnauthorized, "culture territory requires original scoped creator construction authority")
	}
	return source, err
}

type CultureAuthorityRequest struct {
	Binding   core.CareerBinding `json:"binding"`
	ScopeKind string             `json:"scope_kind"`
	ScopeID   string             `json:"scope_id"`
	Decision  string             `json:"decision"`
	StewardID string             `json:"steward_id,omitempty"`
}

func (s *Store) ChangeRPCultureAuthority(ctx context.Context, r CultureAuthorityRequest) (CultureRecord, error) {
	if (r.ScopeKind != "world" && r.ScopeKind != "region") || (r.Decision != "revoke" && r.Decision != "appoint") || (r.Decision == "revoke" && r.StewardID != "") || (r.Decision == "appoint" && r.StewardID == "") {
		return CultureRecord{}, core.NewError(core.CodeInvalidArgument, "invalid culture authority transition")
	}
	var builder string
	return executePrivateFactCommand(s, ctx, r.Binding, "ChangeRPCultureAuthority", r, privateFactDomain{"culture", "RPCultureFactRecorded", `{"authorization":"culture-territory-v1"}`}, func(conn *sql.Conn) error {
		var err error
		builder, err = authorizeCultureBuilder(ctx, conn, r.Binding)
		return err
	}, func(conn *sql.Conn, c privateFactContext) (CultureFact, func() error, error) {
		territory, _, err := readCultureTerritory(ctx, conn, r.Binding, r.ScopeKind, r.ScopeID)
		if err != nil {
			return CultureFact{}, nil, err
		}
		if r.Decision == "revoke" {
			if territory.GrantStatus == "revoked" {
				return CultureFact{}, nil, core.NewError(core.CodeBranchConflict, "authority already revoked")
			}
			territory.GrantStatus = "revoked"
		} else {
			if err := validateRPBinding(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, r.StewardID); err != nil {
				return CultureFact{}, nil, err
			}
			if territory.StewardID == r.StewardID && territory.GrantStatus != "revoked" {
				return CultureFact{}, nil, core.NewError(core.CodeBranchConflict, "same steward already appointed")
			}
			territory.StewardID = r.StewardID
			territory.GrantStatus = "active"
			if err := conn.QueryRowContext(ctx, `SELECT principal_id FROM agent_profiles WHERE agent_id=?`, r.StewardID).Scan(&territory.StewardPrincipalID); err != nil {
				return CultureFact{}, nil, err
			}
		}
		territory.BuilderSourceEventID = builder
		return CultureFact{Version: "corerp.culture.v1", Kind: "territory_authority", ActorID: r.Binding.PrincipalID, Territory: &territory}, func() error {
			// Apply only this scope's latest sourced grant in the same transaction.
			differences, err := cultureGrantProjectionDifferences(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, c.Sequence)
			if err != nil {
				return err
			}
			var scoped []ProjectionDifference
			for _, d := range differences {
				if d.Key == territory.GrantID {
					scoped = append(scoped, d)
				}
			}
			return repairCultureGrantProjections(ctx, conn, scoped)
		}, nil
	})
}
