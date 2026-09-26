package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"corerp.local/backend/internal/core"
)

type StudioPackageActivationRequest struct {
	Genesis            StudioGenesisRequest `json:"genesis"`
	Binding            core.CareerBinding   `json:"binding"`
	SystemPackageID    string               `json:"system_package_id"`
	NarrativePackageID string               `json:"narrative_package_id"`
	ContentPackageIDs  []string             `json:"content_package_ids,omitempty"`
}
type StudioPackagePin struct {
	ID             string `json:"id"`
	Version        string `json:"version"`
	ContentHash    string `json:"content_hash"`
	ManifestHash   string `json:"manifest_hash"`
	InstallEventID string `json:"install_event_id"`
}
type StudioPackageLock struct {
	Version           string             `json:"version"`
	ActivationEventID string             `json:"activation_event_id"`
	System            StudioPackagePin   `json:"system"`
	Narrative         StudioPackagePin   `json:"narrative"`
	Content           []StudioPackagePin `json:"content,omitempty"`
	Algorithm         string             `json:"algorithm"`
	Phase             string             `json:"phase"`
	ConflictOrder     string             `json:"conflict_order"`
}
type StudioActivationFact struct {
	Version       string            `json:"version"`
	EpochID       string            `json:"epoch_id"`
	StartSequence int64             `json:"start_sequence"`
	RulesetHash   string            `json:"ruleset_hash"`
	Lock          StudioPackageLock `json:"lock"`
}
type studioActivePackages struct {
	System    core.StudioPackageBundle
	Narrative core.StudioPackageBundle
	Content   []core.StudioPackageBundle
	Lock      StudioPackageLock
}

func studioPackagePin(p studioInstalledPackage) (StudioPackagePin, error) {
	hash, err := core.HashJSON(p.Fact.Bundle.Manifest)
	return StudioPackagePin{ID: p.Fact.Bundle.Manifest.ID, Version: p.Fact.Bundle.Manifest.Version, ContentHash: p.Fact.Bundle.Manifest.ContentHash, ManifestHash: hash, InstallEventID: p.EventID}, err
}

// ActivateStudioPackages pins rules, not player credentials or world readiness.
// Activation is only allowed once on prepared paused genesis; no hot upgrade.
func (s *Store) ActivateStudioPackages(ctx context.Context, r StudioPackageActivationRequest) (privateFactRecord[StudioActivationFact], error) {
	var empty privateFactRecord[StudioActivationFact]
	if err := validateStudioGenesisRequest(r.Genesis); err != nil {
		return empty, err
	}
	if r.Binding.PrincipalID != r.Genesis.PrincipalID || r.Binding.InstanceID != r.Genesis.InstanceID || r.Binding.BranchID != "br_main" {
		return empty, core.NewError(core.CodeUnauthorized, "activation binding differs")
	}
	if !studioID(r.SystemPackageID) || !studioID(r.NarrativePackageID) || r.SystemPackageID == r.NarrativePackageID {
		return empty, core.NewError(core.CodeInvalidArgument, "distinct installed system and narrative packages required")
	}
	if len(r.ContentPackageIDs) > 30 {
		return empty, core.NewError(core.CodeInvalidArgument, "too many content packages selected")
	}
	selected := map[string]bool{r.SystemPackageID: true, r.NarrativePackageID: true}
	for _, id := range r.ContentPackageIDs {
		if !studioID(id) || selected[id] {
			return empty, core.NewError(core.CodeInvalidArgument, "duplicate or invalid content package identity")
		}
		selected[id] = true
	}
	return executePrivateFactCommand(s, ctx, r.Binding, "ActivateStudioPackages", r, privateFactDomain{"studio_activation", "StudioPackagesActivated", `{"authorization":"sourced-world-create"}`},
		func(conn *sql.Conn) error { return authorizeSavedStudioGenesis(ctx, conn, r.Genesis) },
		func(conn *sql.Conn, c privateFactContext) (StudioActivationFact, func() error, error) {
			var fact StudioActivationFact
			var ready int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM world_instances w JOIN world_clocks c ON c.instance_id=w.instance_id AND c.branch_id=? JOIN rule_epochs ep ON ep.instance_id=c.instance_id AND ep.branch_id=c.branch_id AND ep.epoch_id='epoch_0' AND ep.end_sequence IS NULL WHERE w.instance_id=? AND w.lifecycle_state='paused' AND c.status='paused' AND c.current_world_time=? AND json_extract(ep.lock_document,'$.status')='awaiting_package_activation'`, r.Binding.BranchID, r.Binding.InstanceID, r.Genesis.Spec.StartWorldTime).Scan(&ready); err != nil {
				return fact, nil, err
			}
			if ready != 1 || c.WorldTime != r.Genesis.Spec.StartWorldTime {
				return fact, nil, core.NewError(core.CodeBranchConflict, "activation requires initial paused pending epoch")
			}
			installed, err := readStudioInstalledPackages(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID)
			if err != nil {
				return fact, nil, err
			}
			system, okSystem := installed[r.SystemPackageID]
			narrative, okNarrative := installed[r.NarrativePackageID]
			if !okSystem || !okNarrative || system.Fact.Bundle.Manifest.Kind != "system" || narrative.Fact.Bundle.Manifest.Kind != "narrative" {
				return fact, nil, core.NewError(core.CodeInvalidArgument, "installed system/narrative pair missing")
			}
			bundles := []core.StudioPackageBundle{system.Fact.Bundle, narrative.Fact.Bundle}
			contentPins := make([]StudioPackagePin, 0, len(r.ContentPackageIDs))
			for _, id := range r.ContentPackageIDs {
				content, present := installed[id]
				if !present || content.Fact.Bundle.Manifest.Kind != "content" {
					return fact, nil, core.NewError(core.CodeInvalidArgument, "selected content package missing")
				}
				bundles = append(bundles, content.Fact.Bundle)
				pin, err := studioPackagePin(content)
				if err != nil {
					return fact, nil, err
				}
				contentPins = append(contentPins, pin)
			}
			if err := core.ValidateStudioPackageSet(bundles); err != nil {
				return fact, nil, err
			}
			systemPin, err := studioPackagePin(system)
			if err != nil {
				return fact, nil, err
			}
			narrativePin, err := studioPackagePin(narrative)
			if err != nil {
				return fact, nil, err
			}
			lock := StudioPackageLock{Version: "corerp.studio-lock.v1", ActivationEventID: c.EventID, System: systemPin, Narrative: narrativePin, Content: contentPins, Algorithm: "corerp.scoped-agent.v1", Phase: m2AgentPhaseID, ConflictOrder: "world_time,phase_id,declared_priority,scheduler_item_id"}
			hash, err := core.HashJSON(lock)
			if err != nil {
				return fact, nil, err
			}
			raw, err := core.CanonicalJSON(lock)
			if err != nil {
				return fact, nil, err
			}
			epoch, _ := core.StudioWorldObjectID(r.Binding.InstanceID, "epoch", "packages")
			fact = StudioActivationFact{Version: "corerp.studio-activation.v1", EpochID: epoch, StartSequence: c.Sequence + 1, RulesetHash: hash, Lock: lock}
			return fact, func() error {
				if err := execAgentOne(ctx, conn, "close preparation epoch", `UPDATE rule_epochs SET end_sequence=? WHERE instance_id=? AND branch_id=? AND epoch_id='epoch_0' AND end_sequence IS NULL`, fact.StartSequence, r.Binding.InstanceID, r.Binding.BranchID); err != nil {
					return err
				}
				return execAgentOne(ctx, conn, "activate pinned Studio epoch", `INSERT INTO rule_epochs(instance_id,branch_id,epoch_id,start_sequence,ruleset_hash,lock_document,activation_event_id) VALUES (?,?,?,?,?,?,?)`, r.Binding.InstanceID, r.Binding.BranchID, epoch, fact.StartSequence, hash, string(raw), c.EventID)
			}, nil
		})
}

// Nil means a legacy world, not missing Studio packages. Consumers must not
// silently downgrade a Studio world whose pins or installed source are missing.
func readStudioActivePackages(ctx context.Context, conn *sql.Conn, instance, branch string) (*studioActivePackages, error) {
	var definition string
	if err := conn.QueryRowContext(ctx, `SELECT world_definition_id FROM world_instances WHERE instance_id=?`, instance).Scan(&definition); err != nil {
		return nil, err
	}
	if definition != "corerp.studio.world" {
		return nil, nil
	}
	var epoch, hash, raw, eventID string
	var start int64
	if err := conn.QueryRowContext(ctx, `SELECT ep.epoch_id,ep.ruleset_hash,ep.lock_document,COALESCE(ep.activation_event_id,''),ep.start_sequence FROM rule_epochs ep JOIN branches b ON b.instance_id=ep.instance_id AND b.branch_id=ep.branch_id WHERE ep.instance_id=? AND ep.branch_id=? AND ep.start_sequence<=b.head_sequence+1 AND (ep.end_sequence IS NULL OR b.head_sequence+1<ep.end_sequence)`, instance, branch).Scan(&epoch, &hash, &raw, &eventID, &start); err != nil {
		return nil, err
	}
	return readStudioPinnedPackages(ctx, conn, instance, branch, epoch, hash, raw, eventID, start)
}

// Validate the explicitly selected epoch. Historical inspection must not resolve
// the branch's current epoch or silently use today's package settings.
func readStudioPinnedPackages(ctx context.Context, conn replayQuerier, instance, branch, epoch, hash, raw, eventID string, start int64) (*studioActivePackages, error) {
	bad := func(message string) (*studioActivePackages, error) {
		return nil, core.NewError(core.CodeProjectionDiverged, message)
	}
	var lock StudioPackageLock
	if err := json.Unmarshal([]byte(raw), &lock); err != nil {
		return nil, err
	}
	if lock.Version != "corerp.studio-lock.v1" || lock.ActivationEventID != eventID || eventID == "" || lock.Algorithm != "corerp.scoped-agent.v1" || lock.Phase != m2AgentPhaseID || lock.ConflictOrder != "world_time,phase_id,declared_priority,scheduler_item_id" {
		return bad("Studio active package lock unavailable or unsupported")
	}
	computed, err := core.HashJSON(lock)
	if err != nil {
		return nil, err
	}
	if computed != hash {
		return bad("Studio epoch lock hash differs")
	}
	var source string
	var sequence int64
	if err := conn.QueryRowContext(ctx, `SELECT payload,event_sequence FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='StudioPackagesActivated'`, eventID, instance, branch).Scan(&source, &sequence); err != nil {
		return nil, err
	}
	var fact StudioActivationFact
	if err := json.Unmarshal([]byte(source), &fact); err != nil {
		return nil, err
	}
	sourceHash, err := core.HashJSON(fact.Lock)
	if err != nil {
		return nil, err
	}
	if fact.Version != "corerp.studio-activation.v1" || fact.EpochID != epoch || fact.StartSequence != start || start != sequence+1 || fact.RulesetHash != hash || sourceHash != hash {
		return bad("Studio epoch differs from activation source")
	}
	installed, err := readStudioInstalledPackages(ctx, conn, instance, branch)
	if err != nil {
		return nil, err
	}
	active := studioActivePackages{Lock: lock}
	selections := []struct {
		pin    StudioPackagePin
		kind   string
		target *core.StudioPackageBundle
	}{{lock.System, "system", &active.System}, {lock.Narrative, "narrative", &active.Narrative}}
	if len(lock.Content) > 30 {
		return bad("too many pinned content packages")
	}
	active.Content = make([]core.StudioPackageBundle, len(lock.Content))
	for i, pin := range lock.Content {
		selections = append(selections, struct {
			pin    StudioPackagePin
			kind   string
			target *core.StudioPackageBundle
		}{pin, "content", &active.Content[i]})
	}
	for _, selection := range selections {
		p, ok := installed[selection.pin.ID]
		if !ok || p.Fact.Bundle.Manifest.Kind != selection.kind {
			return bad("active package content missing")
		}
		pin, err := studioPackagePin(p)
		if err != nil {
			return nil, err
		}
		if pin != selection.pin {
			return bad("active package pin differs from installed content")
		}
		var installedAt int64
		if err := conn.QueryRowContext(ctx, `SELECT event_sequence FROM events WHERE instance_id=? AND branch_id=? AND event_id=? AND event_type='StudioPackageInstalled'`, instance, branch, pin.InstallEventID).Scan(&installedAt); err != nil {
			return nil, err
		}
		if installedAt >= sequence {
			return bad("package installation does not precede epoch activation")
		}
		*selection.target = p.Fact.Bundle
	}
	bundles := append([]core.StudioPackageBundle{active.System, active.Narrative}, active.Content...)
	if err := core.ValidateStudioPackageSet(bundles); err != nil {
		return nil, err
	}
	return &active, nil
}
