package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"corerp.local/backend/internal/core"
)

var rpLocationSlotKey = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// A candidate describes an empty ordinary place. It cannot create people,
// property, inventory, money, housing, links or control grants.
type RPLocationCandidate struct {
	DisplayName      string `json:"display_name"`
	GeneratorVersion string `json:"generator_version"`
}

type RPLocationMaterializeRequest struct {
	Binding          core.CareerBinding  `json:"binding"`
	ParentLocationID string              `json:"parent_location_id"`
	SlotKey          string              `json:"slot_key"`
	Candidate        RPLocationCandidate `json:"candidate"`
}

type RPLocationFact struct {
	Version          string `json:"version"`
	LocationID       string `json:"location_id"`
	ParentLocationID string `json:"parent_location_id"`
	SlotKey          string `json:"slot_key"`
	DisplayName      string `json:"display_name"`
	ReadablePath     string `json:"readable_path"`
	GeneratorVersion string `json:"generator_version"`
}

type RPLocationRecord = privateFactRecord[RPLocationFact]

func validateRPLocationSlotRequest(r RPLocationMaterializeRequest) error {
	if err := r.Binding.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.ParentLocationID) != r.ParentLocationID || r.ParentLocationID == "" || len(r.ParentLocationID) > 256 || !rpLocationSlotKey.MatchString(r.SlotKey) {
		return core.NewError(core.CodeInvalidArgument, "bounded parent and slot required")
	}
	return nil
}

func validateRPLocationCandidate(candidate RPLocationCandidate) error {
	if !rpLocationSlotKey.MatchString(candidate.GeneratorVersion) {
		return core.NewError(core.CodeInvalidArgument, "bounded generator version required")
	}
	name := candidate.DisplayName
	if strings.TrimSpace(name) != name || name == "" || len(name) > 128 || !utf8.ValidString(name) {
		return core.NewError(core.CodeInvalidArgument, "bounded location display name required")
	}
	for _, ch := range name {
		if unicode.IsControl(ch) {
			return core.NewError(core.CodeInvalidArgument, "location display name contains control characters")
		}
	}
	return nil
}

func authorizeRPLocationBuilder(ctx context.Context, conn *sql.Conn, b core.CareerBinding) error {
	if b.InstanceID == M2DemoInstanceID && b.BranchID == M2DemoBranchID {
		_, err := authorizeCultureBuilder(ctx, conn, b)
		return err
	}
	var count int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events e JOIN principals p ON p.principal_id=e.actor_id
		WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence=1 AND e.event_type='StudioWorldPrepared'
		AND e.actor_id=? AND p.principal_type='creator' AND p.status='active'`, b.InstanceID, b.BranchID, b.PrincipalID).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return core.NewError(core.CodeUnauthorized, "location materialization requires the scoped world creator")
	}
	return nil
}

func readRPLocationSlot(ctx context.Context, conn *sql.Conn, b core.CareerBinding, parent, slot string) (RPLocationRecord, bool, error) {
	var out RPLocationRecord
	var raw string
	var locationID, generatorVersion, readablePath string
	err := conn.QueryRowContext(ctx, `SELECT e.event_id,e.event_sequence,e.world_time,e.payload,n.location_id,n.generator_version,n.readable_path
		FROM rp_location_nodes n JOIN events e ON e.event_id=n.definition_event_id
		WHERE n.instance_id=? AND n.branch_id=? AND n.parent_location_id=? AND n.slot_key=?
		AND e.instance_id=n.instance_id AND e.branch_id=n.branch_id AND e.event_type='RPLocationMaterialized'`, b.InstanceID, b.BranchID, parent, slot).Scan(&out.EventID, &out.EventSequence, &out.WorldTime, &raw, &locationID, &generatorVersion, &readablePath)
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	if err := json.Unmarshal([]byte(raw), &out.Fact); err != nil {
		return out, false, err
	}
	if out.Fact.ParentLocationID != parent || out.Fact.SlotKey != slot || out.Fact.LocationID != locationID || out.Fact.GeneratorVersion != generatorVersion || out.Fact.ReadablePath != readablePath {
		return out, false, core.NewError(core.CodeProjectionDiverged, "materialized slot differs from source Event")
	}
	out.Replayed = true
	return out, true, nil
}

func (s *Store) readRPLocationSlotAuthorized(ctx context.Context, r RPLocationMaterializeRequest) (RPLocationRecord, bool, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return RPLocationRecord{}, false, err
	}
	defer conn.Close()
	if err := authorizeRPLocationBuilder(ctx, conn, r.Binding); err != nil {
		return RPLocationRecord{}, false, err
	}
	// The request key retains exact payload identity even if another key has
	// already won the slot; a changed exact retry cannot inherit that receipt.
	hash, err := core.HashJSON(r)
	if err != nil {
		return RPLocationRecord{}, false, err
	}
	key, err := core.HashJSON([]string{r.Binding.PrincipalID, r.Binding.IdempotencyKey})
	if err != nil {
		return RPLocationRecord{}, false, err
	}
	var oldHash string
	err = conn.QueryRowContext(ctx, `SELECT request_hash FROM commands WHERE instance_id=? AND branch_id=? AND command_type='MaterializeRPLocation' AND idempotency_key=?`, r.Binding.InstanceID, r.Binding.BranchID, key).Scan(&oldHash)
	if err == nil && oldHash != hash {
		return RPLocationRecord{}, false, core.NewError(core.CodeIdempotencyMismatch, "location materialization key was used with another request")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return RPLocationRecord{}, false, err
	}
	return readRPLocationSlot(ctx, conn, r.Binding, r.ParentLocationID, r.SlotKey)
}

// MaterializeRPLocation performs a read-before-lock and accepts at most one
// sourced location per scoped parent/slot. Candidate generation/validation
// happens outside the write transaction; no model can hold its lock.
func (s *Store) MaterializeRPLocation(ctx context.Context, r RPLocationMaterializeRequest) (RPLocationRecord, error) {
	if err := validateRPLocationSlotRequest(r); err != nil {
		return RPLocationRecord{}, err
	}
	if existing, found, err := s.readRPLocationSlotAuthorized(ctx, r); err != nil || found {
		return existing, err
	}
	if err := validateRPLocationCandidate(r.Candidate); err != nil {
		return RPLocationRecord{}, err
	}
	identity, err := core.HashJSON([]string{r.Binding.InstanceID, r.Binding.BranchID, r.ParentLocationID, r.SlotKey})
	if err != nil {
		return RPLocationRecord{}, err
	}
	locationID := "location_" + identity[7:]
	record, err := executePrivateFactCommand(s, ctx, r.Binding, "MaterializeRPLocation", r,
		privateFactDomain{"rp_location", "RPLocationMaterialized", `{"authorization":"scoped-world-creator"}`},
		func(conn *sql.Conn) error { return authorizeRPLocationBuilder(ctx, conn, r.Binding) },
		func(conn *sql.Conn, c privateFactContext) (RPLocationFact, func() error, error) {
			if _, found, err := readRPLocationSlot(ctx, conn, r.Binding, r.ParentLocationID, r.SlotKey); err != nil {
				return RPLocationFact{}, nil, err
			} else if found {
				return RPLocationFact{}, nil, core.NewError(core.CodeBranchConflict, "location slot already materialized")
			}
			var path string
			err := conn.QueryRowContext(ctx, `SELECT n.readable_path FROM rp_location_nodes n JOIN agent_places p ON p.place_id=n.location_id
				WHERE n.location_id=? AND n.instance_id=? AND n.branch_id=? AND p.instance_id=n.instance_id AND p.branch_id=n.branch_id AND p.status='active'`, r.ParentLocationID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&path)
			if err != nil {
				return RPLocationFact{}, nil, classifyMissing(err, "active parent location")
			}
			fact := RPLocationFact{Version: "corerp.spatial.location.v1", LocationID: locationID, ParentLocationID: r.ParentLocationID, SlotKey: r.SlotKey, DisplayName: r.Candidate.DisplayName, ReadablePath: path + "/" + r.SlotKey, GeneratorVersion: r.Candidate.GeneratorVersion}
			return fact, func() error {
				if err := execAgentOne(ctx, conn, "materialize empty place", `INSERT INTO agent_places(place_id,instance_id,branch_id,display_name,place_kind,status,definition_event_id) VALUES (?,?,?,?, 'public','active',?)`, locationID, r.Binding.InstanceID, r.Binding.BranchID, fact.DisplayName, c.EventID); err != nil {
					return err
				}
				return execAgentOne(ctx, conn, "materialize location slot", `INSERT INTO rp_location_nodes(location_id,instance_id,branch_id,parent_location_id,slot_key,readable_path,generator_version,definition_event_id) VALUES (?,?,?,?,?,?,?,?)`, locationID, r.Binding.InstanceID, r.Binding.BranchID, r.ParentLocationID, r.SlotKey, fact.ReadablePath, fact.GeneratorVersion, c.EventID)
			}, nil
		})
	if err == nil {
		return record, nil
	}
	// Another serialized caller may have won the slot while this candidate
	// waited for the write lock. Only a proved winner converts that conflict.
	if core.HasCode(err, core.CodeBranchConflict) {
		if winner, found, readErr := s.readRPLocationSlotAuthorized(ctx, r); readErr != nil {
			return RPLocationRecord{}, readErr
		} else if found {
			return winner, nil
		}
	}
	return RPLocationRecord{}, err
}

type RPLocationRenameRequest struct {
	Binding    core.CareerBinding `json:"binding"`
	LocationID string             `json:"location_id"`
	NewName    string             `json:"new_name"`
}

type RPLocationRenameRecord = privateFactRecord[RPLocationRenameFact]

type RPLocationRenameFact struct {
	Version    string `json:"version"`
	LocationID string `json:"location_id"`
	OldName    string `json:"old_name"`
	NewName    string `json:"new_name"`
}

// Rename changes a readable attribute only. The stable location key, parent,
// logical slot and original materialization Event are untouched.
func (s *Store) RenameRPLocation(ctx context.Context, r RPLocationRenameRequest) (privateFactRecord[RPLocationRenameFact], error) {
	if err := r.Binding.Validate(); err != nil {
		return privateFactRecord[RPLocationRenameFact]{}, err
	}
	if strings.TrimSpace(r.LocationID) != r.LocationID || r.LocationID == "" || len(r.LocationID) > 256 || strings.TrimSpace(r.NewName) != r.NewName || r.NewName == "" || len(r.NewName) > 128 || !utf8.ValidString(r.NewName) {
		return privateFactRecord[RPLocationRenameFact]{}, core.NewError(core.CodeInvalidArgument, "bounded location and new name required")
	}
	for _, ch := range r.NewName {
		if unicode.IsControl(ch) {
			return privateFactRecord[RPLocationRenameFact]{}, core.NewError(core.CodeInvalidArgument, "location name contains control characters")
		}
	}
	return executePrivateFactCommand(s, ctx, r.Binding, "RenameRPLocation", r,
		privateFactDomain{"rp_location_rename", "RPLocationRenamed", `{"authorization":"scoped-world-creator"}`},
		func(conn *sql.Conn) error { return authorizeRPLocationBuilder(ctx, conn, r.Binding) },
		func(conn *sql.Conn, _ privateFactContext) (RPLocationRenameFact, func() error, error) {
			var old string
			err := conn.QueryRowContext(ctx, `SELECT p.display_name FROM agent_places p JOIN rp_location_nodes n ON n.location_id=p.place_id
				WHERE n.location_id=? AND n.instance_id=? AND n.branch_id=? AND p.instance_id=n.instance_id AND p.branch_id=n.branch_id AND p.status='active'`, r.LocationID, r.Binding.InstanceID, r.Binding.BranchID).Scan(&old)
			if err != nil {
				return RPLocationRenameFact{}, nil, classifyMissing(err, "active location to rename")
			}
			if old == r.NewName {
				return RPLocationRenameFact{}, nil, core.NewError(core.CodeBranchConflict, "location already has this name")
			}
			fact := RPLocationRenameFact{Version: "corerp.spatial.rename.v1", LocationID: r.LocationID, OldName: old, NewName: r.NewName}
			return fact, func() error {
				return execAgentOne(ctx, conn, "rename spatial location", `UPDATE agent_places SET display_name=? WHERE place_id=? AND instance_id=? AND branch_id=? AND display_name=?`, r.NewName, r.LocationID, r.Binding.InstanceID, r.Binding.BranchID, old)
			}, nil
		})
}
