package storage

import (
	"context"

	"corerp.local/backend/internal/core"
)

type StudioCreateRequest struct {
	PrincipalID         string                   `json:"principal_id"`
	AuthorityInstanceID string                   `json:"authority_instance_id"`
	AuthorityBranchID   string                   `json:"authority_branch_id"`
	InstanceID          string                   `json:"instance_id"`
	IdempotencyKey      string                   `json:"idempotency_key"`
	Spec                core.StudioWorldSpec     `json:"spec"`
	SystemPackage       core.StudioPackageBundle `json:"system_package"`
	NarrativePackage    core.StudioPackageBundle `json:"narrative_package"`
	PlayerPrincipalID   string                   `json:"player_principal_id"`
}
type StudioCreateResult struct {
	InstanceID        string `json:"instance_id"`
	BranchID          string `json:"branch_id"`
	EntityID          string `json:"entity_id"`
	PlayerPrincipalID string `json:"player_principal_id"`
	ReadyEventID      string `json:"ready_event_id"`
	EventSequence     int64  `json:"event_sequence"`
	Status            string `json:"status"`
	Replayed          bool   `json:"replayed"`
}

// CreateStudioWorld coordinates existing recoverable authority owners. Each
// stage commits separately; the last stage alone grants Play access. The full
// immutable request hash is bound into genesis before any package can install.
func (s *Store) CreateStudioWorld(ctx context.Context, r StudioCreateRequest) (StudioCreateResult, error) {
	var empty StudioCreateResult
	hash, err := core.HashJSON(r)
	if err != nil {
		return empty, err
	}
	g := StudioGenesisRequest{CreationPlanHash: hash, PrincipalID: r.PrincipalID, AuthorityInstanceID: r.AuthorityInstanceID, AuthorityBranchID: r.AuthorityBranchID, InstanceID: r.InstanceID, IdempotencyKey: r.IdempotencyKey, Spec: r.Spec}
	if err := validateStudioGenesisRequest(g); err != nil {
		return empty, err
	}
	if !studioID(r.PlayerPrincipalID) {
		return empty, core.NewError(core.CodeInvalidArgument, "bounded existing player identity required")
	}
	if r.SystemPackage.Manifest.Kind != "system" || r.NarrativePackage.Manifest.Kind != "narrative" {
		return empty, core.NewError(core.CodeInvalidArgument, "typed system and narrative packages required")
	}
	if err := core.ValidateStudioPackageSet([]core.StudioPackageBundle{r.SystemPackage, r.NarrativePackage}); err != nil {
		return empty, err
	}
	// Preflight predictable failures without creating a half-configured world.
	// Each authority owner still independently rechecks authorization on retry.
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return empty, err
	}
	if err := authorizeStudioWorldCreation(ctx, tx.conn, r.PrincipalID, r.AuthorityInstanceID, r.AuthorityBranchID); err != nil {
		tx.Rollback(ctx)
		return empty, err
	}
	var player int
	err = tx.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM principals WHERE principal_id=? AND principal_type='player' AND status='active'`, r.PlayerPrincipalID).Scan(&player)
	tx.Rollback(ctx)
	if err != nil {
		return empty, err
	}
	if player != 1 {
		return empty, core.NewError(core.CodeUnauthorized, "existing active player required")
	}
	if _, err := s.PrepareStudioWorld(ctx, g); err != nil {
		return empty, err
	}
	if _, err := s.PrepareStudioParticipants(ctx, g); err != nil {
		return empty, err
	}
	if _, err := s.PrepareStudioSpatial(ctx, g); err != nil {
		return empty, err
	}
	id := func(key string) string { v, _ := core.StudioWorldObjectID(r.InstanceID, "create_key", key); return v }
	binding := core.CareerBinding{PrincipalID: r.PrincipalID, InstanceID: r.InstanceID, BranchID: "br_main", ExpectedHead: int64(len(r.Spec.People) + 2)}
	packages := []core.StudioPackageBundle{r.SystemPackage, r.NarrativePackage}
	for _, dependency := range r.SystemPackage.Manifest.Requires {
		if dependency.ID == r.NarrativePackage.Manifest.ID {
			packages[0], packages[1] = packages[1], packages[0]
			break
		}
	}
	for _, bundle := range packages {
		binding.IdempotencyKey = id("install-" + bundle.Manifest.Kind)
		if _, err := s.InstallStudioPackage(ctx, StudioPackageInstallRequest{Genesis: g, Binding: binding, Bundle: bundle}); err != nil {
			return empty, err
		}
		binding.ExpectedHead++
	}
	binding.IdempotencyKey = id("activate")
	if _, err := s.ActivateStudioPackages(ctx, StudioPackageActivationRequest{Genesis: g, Binding: binding, SystemPackageID: r.SystemPackage.Manifest.ID, NarrativePackageID: r.NarrativePackage.Manifest.ID}); err != nil {
		return empty, err
	}
	binding.ExpectedHead++
	binding.IdempotencyKey = id("save")
	ready, err := s.SaveStudioWorld(ctx, StudioWorldSaveRequest{Genesis: g, Binding: binding, PlayerPrincipalID: r.PlayerPrincipalID})
	if err != nil {
		return empty, err
	}
	return StudioCreateResult{InstanceID: r.InstanceID, BranchID: "br_main", EntityID: ready.Fact.EntityID, PlayerPrincipalID: ready.Fact.PlayerPrincipalID, ReadyEventID: ready.EventID, EventSequence: ready.EventSequence, Status: "ready", Replayed: ready.Replayed}, nil
}
