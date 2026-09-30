package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"corerp.local/backend/internal/core"
)

func (s *Store) RunStrictWorldAuthorized(ctx context.Context, request core.StrictSimulationRequest) (StrictRunResult, error) {
	if err := request.Validate(); err != nil {
		return StrictRunResult{}, err
	}
	if err := s.authorizeExactScope(ctx, request.PrincipalID, request.CapabilityID, request.InstanceID, request.BranchID, request.BranchID); err != nil {
		return StrictRunResult{}, err
	}
	if request.InstanceID != DemoInstanceID || request.BranchID != DemoBranchID {
		return StrictRunResult{}, core.NewError(core.CodeNotFound, "strict demo world not found")
	}
	return s.RunStrictWorld(ctx, request.TargetDay, request.Budget)
}

func (s *Store) ReadDemoStateAuthorized(ctx context.Context, request core.StateReadRequest) (State, error) {
	if err := request.Validate(); err != nil {
		return State{}, err
	}
	if err := s.authorizeExactScope(ctx, request.PrincipalID, request.CapabilityID, request.InstanceID, request.BranchID, request.BranchID); err != nil {
		return State{}, err
	}
	if request.InstanceID != DemoInstanceID || request.BranchID != DemoBranchID {
		return State{}, core.NewError(core.CodeNotFound, "demo world not found")
	}
	return s.ReadDemoState(ctx)
}

func (s *Store) Ready(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return core.WrapError(core.CodeStorageFailure, "ping readiness database", err)
	}
	versions := make([]any, 0, len(schemaMigrations)+1)
	versions = append(versions, BaseSchemaVersion)
	for _, migration := range schemaMigrations {
		versions = append(versions, migration.version)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(versions)), ",")
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_meta WHERE schema_version IN (`+placeholders+`)`, versions...).Scan(&count); err != nil {
		return core.WrapError(core.CodeStorageFailure, "read readiness schema", err)
	}
	if count != len(versions) {
		return core.NewError(core.CodeStorageFailure, "required schema versions are not all applied")
	}
	return nil
}

func (s *Store) authorizeExactScope(ctx context.Context, principalID, capabilityID, instanceID, branchID, subjectID string) error {
	var grantID string
	err := s.db.QueryRowContext(ctx, `
		SELECT g.grant_id
		FROM capability_grants g JOIN principals p ON p.principal_id = g.principal_id
		WHERE g.principal_id = ? AND p.status = 'active' AND g.capability_id = ?
		  AND g.instance_id = ? AND g.branch_id = ? AND g.subject_id IN (?, '*') AND g.status = 'active'
		ORDER BY CASE WHEN g.subject_id = ? THEN 0 ELSE 1 END, g.grant_id LIMIT 1`,
		principalID, capabilityID, instanceID, branchID, subjectID, subjectID,
	).Scan(&grantID)
	if errors.Is(err, sql.ErrNoRows) {
		return core.NewError(core.CodeUnauthorized, "principal lacks capability for the requested scope")
	}
	if err != nil {
		return core.WrapError(core.CodeStorageFailure, "read scoped capability grant", err)
	}
	return nil
}
