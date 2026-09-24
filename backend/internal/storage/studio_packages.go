package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"corerp.local/backend/internal/core"
)

type StudioPackageInstallRequest struct {
	Genesis StudioGenesisRequest     `json:"genesis"`
	Binding core.CareerBinding       `json:"binding"`
	Bundle  core.StudioPackageBundle `json:"bundle"`
}

type StudioPackageFact struct {
	Version        string                   `json:"version"`
	GenesisEventID string                   `json:"genesis_event_id"`
	Bundle         core.StudioPackageBundle `json:"bundle"`
}

type studioInstalledPackage struct {
	EventID string
	Fact    StudioPackageFact
}

// Immutable installation content lives in the existing Event store. No package
// code runs, no filesystem extraction occurs, and installation is not activation.
func (s *Store) InstallStudioPackage(ctx context.Context, r StudioPackageInstallRequest) (privateFactRecord[StudioPackageFact], error) {
	var empty privateFactRecord[StudioPackageFact]
	if err := validateStudioGenesisRequest(r.Genesis); err != nil {
		return empty, err
	}
	if err := r.Bundle.Validate(); err != nil {
		return empty, err
	}
	if r.Binding.PrincipalID != r.Genesis.PrincipalID || r.Binding.InstanceID != r.Genesis.InstanceID || r.Binding.BranchID != "br_main" {
		return empty, core.NewError(core.CodeUnauthorized, "installation binding differs from saved genesis owner")
	}
	return executePrivateFactCommand(s, ctx, r.Binding, "InstallStudioPackage", r,
		privateFactDomain{"studio_package", "StudioPackageInstalled", `{"authorization":"sourced-world-create"}`},
		func(conn *sql.Conn) error { return authorizeSavedStudioGenesis(ctx, conn, r.Genesis) },
		func(conn *sql.Conn, c privateFactContext) (StudioPackageFact, func() error, error) {
			genesisEvent, _ := core.StudioWorldObjectID(r.Genesis.InstanceID, "event", "genesis")
			fact := StudioPackageFact{Version: "corerp.studio-package-install.v1", GenesisEventID: genesisEvent, Bundle: r.Bundle}
			var ready int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM world_instances w JOIN world_clocks c ON c.instance_id=w.instance_id AND c.branch_id=? WHERE w.instance_id=? AND w.lifecycle_state='paused' AND c.status='paused' AND c.current_world_time=? AND EXISTS(SELECT 1 FROM events e WHERE e.instance_id=w.instance_id AND e.branch_id=c.branch_id AND e.event_type='StudioSpatialPrepared') AND EXISTS(SELECT 1 FROM rule_epochs ep WHERE ep.instance_id=w.instance_id AND ep.branch_id=c.branch_id AND ep.end_sequence IS NULL AND json_extract(ep.lock_document,'$.status')='awaiting_package_activation')`, r.Binding.BranchID, r.Binding.InstanceID, r.Genesis.Spec.StartWorldTime).Scan(&ready); err != nil {
				return fact, nil, err
			}
			if ready != 1 || c.WorldTime != r.Genesis.Spec.StartWorldTime {
				return fact, nil, core.NewError(core.CodeBranchConflict, "package installation requires prepared paused world before activation")
			}
			installed, err := readStudioInstalledPackages(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID)
			if err != nil {
				return fact, nil, err
			}
			if len(installed) >= 32 {
				return fact, nil, core.NewError(core.CodeInvalidArgument, "world package limit reached")
			}
			if _, exists := installed[r.Bundle.Manifest.ID]; exists {
				return fact, nil, core.NewError(core.CodeBranchConflict, "package identity is already installed; exact retry required")
			}
			bundles := []core.StudioPackageBundle{r.Bundle}
			for _, p := range installed {
				bundles = append(bundles, p.Fact.Bundle)
			}
			if err := core.ValidateStudioPackageSet(bundles); err != nil {
				return fact, nil, err
			}
			// No projection to maintain: the bounded validated content itself is
			// saved by the shared Event transaction and later pinned by activation.
			return fact, nil, nil
		})
}

func readStudioInstalledPackages(ctx context.Context, q replayQuerier, instance, branch string) (map[string]studioInstalledPackage, error) {
	rows, err := q.QueryContext(ctx, `SELECT event_id,payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='StudioPackageInstalled' ORDER BY event_sequence LIMIT 33`, instance, branch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]studioInstalledPackage{}
	genesis, _ := core.StudioWorldObjectID(instance, "event", "genesis")
	for rows.Next() {
		var p studioInstalledPackage
		var raw string
		if err := rows.Scan(&p.EventID, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &p.Fact); err != nil {
			return nil, core.WrapError(core.CodeProjectionDiverged, "decode installed package", err)
		}
		if p.Fact.Version != "corerp.studio-package-install.v1" || p.Fact.GenesisEventID != genesis {
			return nil, core.NewError(core.CodeProjectionDiverged, "installed package source differs")
		}
		if err := p.Fact.Bundle.Validate(); err != nil {
			return nil, core.WrapError(core.CodeProjectionDiverged, "installed package content differs", err)
		}
		if _, ok := result[p.Fact.Bundle.Manifest.ID]; ok || len(result) >= 32 {
			return nil, core.NewError(core.CodeProjectionDiverged, "duplicate or excessive installed packages")
		}
		result[p.Fact.Bundle.Manifest.ID] = p
	}
	return result, rows.Err()
}
