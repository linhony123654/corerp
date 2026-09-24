package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"corerp.local/backend/internal/core"
)

// Inspector grants are independent of player control and ordinary event feeds.
// Provisioning belongs to the operator/creator setup path, never this read API.
type StudioEventRequest struct {
	PrincipalID string `json:"principal_id"`
	InstanceID  string `json:"instance_id"`
	BranchID    string `json:"branch_id"`
	EventID     string `json:"event_id"`
}

type StudioRuleEvidence struct {
	EpochID       string              `json:"epoch_id"`
	RulesetHash   string              `json:"ruleset_hash"`
	StartSequence int64               `json:"start_sequence"`
	EndSequence   *int64              `json:"end_sequence,omitempty"`
	PackageStatus string              `json:"package_status"`
	Packages      *StudioRulePackages `json:"packages,omitempty"`
}

// Package contents are creator evidence, not an ops diagnostic or an assertion
// that every rule in the installed pair caused the inspected event.
type StudioRulePackages struct {
	Lock      StudioPackageLock        `json:"lock"`
	System    core.StudioPackageBundle `json:"system"`
	Narrative core.StudioPackageBundle `json:"narrative"`
}

type StudioEventEvidence struct {
	InstanceID        string             `json:"instance_id"`
	BranchID          string             `json:"branch_id"`
	HeadSequence      int64              `json:"head_sequence"`
	AccessLevel       string             `json:"access_level"`
	EventID           string             `json:"event_id"`
	EventSequence     int64              `json:"event_sequence"`
	EventType         string             `json:"event_type"`
	WorldTime         string             `json:"world_time"`
	BatchID           string             `json:"batch_id"`
	BatchHash         string             `json:"batch_hash"`
	Rule              StudioRuleEvidence `json:"rule"`
	ActorID           string             `json:"actor_id,omitempty"`
	CommandID         string             `json:"command_id,omitempty"`
	CausationEventID  string             `json:"causation_event_id,omitempty"`
	CausationEvidence string             `json:"causation_evidence"`
	Payload           json.RawMessage    `json:"payload,omitempty"`
	Redacted          bool               `json:"redacted"`
}

// ReadStudioEvent returns a single coherent read snapshot. It does not evaluate
// rules, infer intent, re-run decisions, update observations or mutate the world.
func (s *Store) ReadStudioEvent(ctx context.Context, r StudioEventRequest) (StudioEventEvidence, error) {
	var out StudioEventEvidence
	for _, v := range []string{r.PrincipalID, r.InstanceID, r.BranchID, r.EventID} {
		if strings.TrimSpace(v) == "" || len(v) > 256 {
			return out, core.NewError(core.CodeInvalidArgument, "bounded principal, instance, branch and event IDs are required")
		}
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, core.WrapError(core.CodeStorageFailure, "begin inspector snapshot", err)
	}
	defer tx.Rollback()
	// No caller-selected role or capability. Both event and rule fields must have
	// been explicitly granted; a player cannot acquire creator access by naming it.
	role, err := authorizeStudio(ctx, tx, r.PrincipalID, r.InstanceID, r.BranchID)
	if err != nil {
		return out, err
	}
	var actor, command, payload, definition, lockDocument, activationEvent string
	var cause sql.NullString
	var end sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT b.head_sequence,e.event_id,e.event_sequence,e.event_type,e.world_time,
	 e.batch_id,x.batch_hash,x.epoch_id,r.ruleset_hash,r.start_sequence,r.end_sequence,
	 e.actor_id,x.command_id,e.causation_event_id,e.payload,w.world_definition_id,r.lock_document,COALESCE(r.activation_event_id,'')
	 FROM events e JOIN branches b ON b.instance_id=e.instance_id AND b.branch_id=e.branch_id
	 JOIN world_instances w ON w.instance_id=e.instance_id
	 JOIN event_batches x ON x.batch_id=e.batch_id AND x.instance_id=e.instance_id AND x.branch_id=e.branch_id
	 JOIN rule_epochs r ON r.instance_id=x.instance_id AND r.branch_id=x.branch_id AND r.epoch_id=x.epoch_id
	 WHERE e.instance_id=? AND e.branch_id=? AND e.event_id=?`, r.InstanceID, r.BranchID, r.EventID).
		Scan(&out.HeadSequence, &out.EventID, &out.EventSequence, &out.EventType, &out.WorldTime,
			&out.BatchID, &out.BatchHash, &out.Rule.EpochID, &out.Rule.RulesetHash, &out.Rule.StartSequence, &end,
			&actor, &command, &cause, &payload, &definition, &lockDocument, &activationEvent)
	if err != nil {
		return StudioEventEvidence{}, classifyMissing(err, "inspected event")
	}
	if out.EventSequence < out.Rule.StartSequence || (end.Valid && out.EventSequence >= end.Int64) || out.EventSequence > out.HeadSequence {
		return StudioEventEvidence{}, core.NewError(core.CodeProjectionDiverged, "event is outside recorded branch or rule epoch")
	}
	out.InstanceID, out.BranchID, out.AccessLevel = r.InstanceID, r.BranchID, role
	if end.Valid {
		out.Rule.EndSequence = &end.Int64
	}
	out.Redacted = role == "operator"
	out.Rule.PackageStatus = "redacted"
	out.CausationEvidence = "redacted"
	if !out.Redacted {
		out.Rule.PackageStatus = "not_recorded"
		if definition == "corerp.studio.world" {
			out.Rule.PackageStatus = "preparation"
			if out.Rule.EpochID != "epoch_0" {
				packages, err := readStudioPinnedPackages(ctx, tx, r.InstanceID, r.BranchID, out.Rule.EpochID, out.Rule.RulesetHash, lockDocument, activationEvent, out.Rule.StartSequence)
				if err != nil {
					return StudioEventEvidence{}, core.WrapError(core.CodeProjectionDiverged, "historical package provenance differs", err)
				}
				out.Rule.PackageStatus = "verified"
				out.Rule.Packages = &StudioRulePackages{Lock: packages.Lock, System: packages.System, Narrative: packages.Narrative}
			}
		}
		out.ActorID, out.CommandID, out.Payload = actor, command, json.RawMessage(payload)
		out.CausationEvidence = "not_recorded"
		if cause.Valid && cause.String != "" {
			// A reference alone does not authorize a lookup outside this branch.
			var count int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE event_id=? AND instance_id=? AND branch_id=?`, cause.String, r.InstanceID, r.BranchID).Scan(&count); err != nil {
				return StudioEventEvidence{}, core.WrapError(core.CodeStorageFailure, "read causal scope", err)
			}
			out.CausationEvidence = "outside_scope"
			if count == 1 {
				out.CausationEventID, out.CausationEvidence = cause.String, "recorded"
			}
		}
	}
	return out, nil
}
