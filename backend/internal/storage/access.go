package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"corerp.local/backend/internal/core"
)

type PrivateDiagnostics struct {
	ProjectionVersion int64 `json:"projection_version"`
	LastEventSequence int64 `json:"last_event_sequence"`
}

type PrivateEconomicView struct {
	SubjectID        string              `json:"subject_id"`
	AccountID        string              `json:"account_id,omitempty"`
	BalanceMinor     *int64              `json:"balance_minor,omitempty"`
	OwnerID          string              `json:"owner_id,omitempty"`
	EvidenceRefs     []string            `json:"evidence_refs,omitempty"`
	Diagnostics      *PrivateDiagnostics `json:"diagnostics,omitempty"`
	OperatorRedacted bool                `json:"operator_redacted"`
}

func (s *Store) ReadPrivateEconomy(ctx context.Context, request core.PrivateEconomicRead) (PrivateEconomicView, error) {
	if err := request.Validate(); err != nil {
		return PrivateEconomicView{}, err
	}
	var principalType, fieldScope string
	err := s.db.QueryRowContext(ctx, `
		SELECT p.principal_type, g.field_scope
		FROM capability_grants g JOIN principals p ON p.principal_id = g.principal_id
		WHERE g.principal_id = ? AND p.status = 'active' AND g.capability_id = ?
		  AND g.instance_id = ? AND g.branch_id = ? AND g.subject_id IN (?, '*') AND g.status = 'active'
		ORDER BY CASE WHEN g.subject_id = ? THEN 0 ELSE 1 END, g.grant_id LIMIT 1`,
		request.PrincipalID, request.CapabilityID, request.InstanceID, request.BranchID,
		request.SubjectID, request.SubjectID,
	).Scan(&principalType, &fieldScope)
	if errors.Is(err, sql.ErrNoRows) {
		return PrivateEconomicView{}, core.NewError(core.CodeUnauthorized, "principal lacks private-read scope for this instance, branch, and subject")
	}
	if err != nil {
		return PrivateEconomicView{}, core.WrapError(core.CodeStorageFailure, "read private economy grant", err)
	}
	if (principalType == "operator") != (request.CapabilityID == "diagnostics.private.read") {
		return PrivateEconomicView{}, core.NewError(core.CodeUnauthorized, "principal type and private-read capability do not match")
	}
	var allowedFields []string
	if err := json.Unmarshal([]byte(fieldScope), &allowedFields); err != nil {
		return PrivateEconomicView{}, core.WrapError(core.CodeStorageFailure, "decode private-read field scope", err)
	}
	allowed := make(map[string]bool, len(allowedFields))
	for _, field := range allowedFields {
		allowed[field] = true
	}
	for _, field := range request.Fields {
		if !allowed[field] {
			return PrivateEconomicView{}, core.NewError(core.CodeUnauthorized, "requested private field is outside the grant: "+field)
		}
	}

	var accountID, ownerID string
	var balance, projectionVersion, lastEventSequence int64
	err = s.db.QueryRowContext(ctx, `
		SELECT e.account_id, a.owner_id, b.balance_minor, b.projection_version, b.last_event_sequence
		FROM economic_entities e
		JOIN accounts a ON a.account_id = e.account_id
		JOIN account_balances b ON b.account_id = a.account_id
		WHERE e.entity_id = ? AND e.private_finances = 1`, request.SubjectID,
	).Scan(&accountID, &ownerID, &balance, &projectionVersion, &lastEventSequence)
	if err != nil {
		return PrivateEconomicView{}, classifyMissing(err, "private economic subject")
	}
	view := PrivateEconomicView{SubjectID: request.SubjectID, OperatorRedacted: principalType == "operator"}
	for _, field := range request.Fields {
		switch field {
		case "account_id":
			view.AccountID = accountID
		case "balance_minor":
			value := balance
			view.BalanceMinor = &value
		case "owner_id":
			if principalType == "operator" {
				return PrivateEconomicView{}, core.NewError(core.CodeUnauthorized, "operator diagnostics do not expose owner identity")
			}
			view.OwnerID = ownerID
		case "evidence":
			if principalType != "creator" {
				return PrivateEconomicView{}, core.NewError(core.CodeUnauthorized, "full private evidence requires creator scope")
			}
			evidence, err := s.privateEconomicEvidence(ctx, request.SubjectID)
			if err != nil {
				return PrivateEconomicView{}, err
			}
			view.EvidenceRefs = evidence
		case "diagnostics":
			if principalType != "operator" {
				return PrivateEconomicView{}, core.NewError(core.CodeUnauthorized, "diagnostic metadata requires operator scope")
			}
			view.Diagnostics = &PrivateDiagnostics{ProjectionVersion: projectionVersion, LastEventSequence: lastEventSequence}
		default:
			return PrivateEconomicView{}, core.NewError(core.CodeInvalidArgument, "unsupported private economic field: "+field)
		}
	}
	return view, nil
}

func (s *Store) privateEconomicEvidence(ctx context.Context, subjectID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT 'wage:' || o.obligation_id
		FROM wage_obligations o JOIN employment_contracts c ON c.contract_id = o.contract_id
		WHERE c.employee_entity_id = ?
		UNION ALL
		SELECT 'rent:' || o.obligation_id
		FROM rent_obligations o JOIN rent_contracts c ON c.contract_id = o.contract_id
		WHERE c.tenant_entity_id = ?`, subjectID, subjectID)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read private economic evidence", err)
	}
	defer rows.Close()
	var evidence []string
	for rows.Next() {
		var reference string
		if err := rows.Scan(&reference); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "scan private economic evidence", err)
		}
		evidence = append(evidence, reference)
	}
	if err := rows.Err(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "iterate private economic evidence", err)
	}
	sort.Strings(evidence)
	return evidence, nil
}
