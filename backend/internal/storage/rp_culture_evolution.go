package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
)

func requireKnownCultureVersion(ctx context.Context, conn *sql.Conn, b core.CareerBinding, actor, eventID string) (CultureFact, error) {
	definition, err := readCultureFact(ctx, conn, b, eventID)
	if err != nil {
		return CultureFact{}, err
	}
	if definition.Kind != "definition" || definition.Definition == nil {
		return CultureFact{}, core.NewError(core.CodeInvalidArgument, "culture lineage requires a definition Event")
	}
	if definition.ActorID == actor {
		return definition, nil
	}
	var count int
	err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events e,json_each(e.payload,'$.listener_ids') listener WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPCultureFactRecorded' AND json_extract(e.payload,'$.kind')='transmission' AND json_extract(e.payload,'$.definition_event_id')=? AND listener.value=?`, b.InstanceID, b.BranchID, eventID, actor).Scan(&count)
	if err != nil {
		return CultureFact{}, err
	}
	if count == 0 {
		return CultureFact{}, core.NewError(core.CodeUnauthorized, "author does not know referenced culture version")
	}
	return definition, nil
}

func validateCultureLineage(ctx context.Context, conn *sql.Conn, r CultureDefinitionRequest) error {
	var latest string
	err := conn.QueryRowContext(ctx, `SELECT event_id FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPCultureFactRecorded' AND json_extract(payload,'$.kind')='definition' AND json_extract(payload,'$.definition.culture_id')=? ORDER BY event_sequence DESC LIMIT 1`, r.Binding.InstanceID, r.Binding.BranchID, r.Culture.CultureID).Scan(&latest)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if latest != r.PreviousDefinitionEventID {
		return core.NewError(core.CodeBranchConflict, "culture revision must reference current definition")
	}
	if latest != "" {
		previous, err := requireKnownCultureVersion(ctx, conn, r.Binding, r.AuthorID, latest)
		if err != nil {
			return err
		}
		if previous.Definition.ScopeKind != r.Culture.ScopeKind || previous.Definition.ScopeID != r.Culture.ScopeID {
			return core.NewError(core.CodeInvalidArgument, "culture evolution cannot silently replace scope")
		}
		if r.Culture.ScopeKind == "community" && previous.ActorID != r.AuthorID {
			return core.NewError(core.CodeUnauthorized, "community revision belongs to its author; others may form a subculture")
		}
	}
	if len(r.ParentDefinitionEventIDs) != len(r.Culture.SubcultureOf) || len(r.ParentDefinitionEventIDs) > 8 {
		return core.NewError(core.CodeInvalidArgument, "subculture requires bounded matching parent version sources")
	}
	seen := map[string]bool{}
	for i, id := range r.ParentDefinitionEventIDs {
		parent, err := requireKnownCultureVersion(ctx, conn, r.Binding, r.AuthorID, id)
		if err != nil {
			return err
		}
		parentID := parent.Definition.CultureID
		if parentID != r.Culture.SubcultureOf[i] || parentID == r.Culture.CultureID || seen[parentID] {
			return core.NewError(core.CodeInvalidArgument, "invalid duplicate or self subculture parent")
		}
		seen[parentID] = true
	}
	return nil
}
