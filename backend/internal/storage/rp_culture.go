package storage

import (
	"context"
	"corerp.local/backend/internal/core"
	"database/sql"
	"encoding/json"
)

// Communities are actor-founded; organization cultures additionally require
// existing scoped managerial authority. Other layers remain separately gated.
type CultureDefinitionRequest struct {
	PreviousDefinitionEventID string             `json:"previous_definition_event_id,omitempty"`
	ParentDefinitionEventIDs  []string           `json:"parent_definition_event_ids,omitempty"`
	Binding                   core.CareerBinding `json:"binding"`
	AuthorID                  string             `json:"author_id"`
	Culture                   core.RPCulture     `json:"culture"`
}
type CultureTransmissionRequest struct {
	Binding           core.CareerBinding `json:"binding"`
	SpeakerID         string             `json:"speaker_id"`
	DefinitionEventID string             `json:"definition_event_id"`
}
type CultureStanceRequest struct {
	DefinitionEventID   string             `json:"definition_event_id,omitempty"`
	Binding             core.CareerBinding `json:"binding"`
	EntityID            string             `json:"entity_id"`
	TransmissionEventID string             `json:"transmission_event_id"`
	Stance              string             `json:"stance"`
}
type CultureFact struct {
	PreviousDefinitionEventID string                         `json:"previous_definition_event_id,omitempty"`
	ParentDefinitionEventIDs  []string                       `json:"parent_definition_event_ids,omitempty"`
	Territory                 *CultureTerritoryFact          `json:"territory,omitempty"`
	Family                    *CultureFamilyFact             `json:"family,omitempty"`
	ScopeSourceEventID        string                         `json:"scope_source_event_id,omitempty"`
	Affiliation               *core.RPCultureAffiliation     `json:"affiliation,omitempty"`
	Version                   string                         `json:"version"`
	Kind                      string                         `json:"kind"`
	ActorID                   string                         `json:"actor_id"`
	Definition                *core.RPCulture                `json:"definition,omitempty"`
	DefinitionEventID         string                         `json:"definition_event_id,omitempty"`
	PlaceID                   string                         `json:"place_id,omitempty"`
	ListenerIDs               []string                       `json:"listener_ids,omitempty"`
	Internalization           *core.RPCultureInternalization `json:"internalization,omitempty"`
}
type CultureRecord = privateFactRecord[CultureFact]

func (s *Store) executeCultureCommand(ctx context.Context, b core.CareerBinding, command string, request any, actor string, prepare func(*sql.Conn, privateFactContext) (CultureFact, func() error, error)) (CultureRecord, error) {
	return s.executeScopedCultureCommand(ctx, b, command, request, actor, nil, prepare)
}

func (s *Store) executeScopedCultureCommand(ctx context.Context, b core.CareerBinding, command string, request any, actor string, authorizeScope func(*sql.Conn) error, prepare func(*sql.Conn, privateFactContext) (CultureFact, func() error, error)) (CultureRecord, error) {
	return executePrivateFactCommand(s, ctx, b, command, request, privateFactDomain{"culture", "RPCultureFactRecorded", `{"authorization":"own-culture-v1"}`}, func(conn *sql.Conn) error {
		if err := authorizeCareerCandidate(ctx, conn, b, actor); err != nil {
			return err
		}
		if authorizeScope != nil {
			return authorizeScope(conn)
		}
		return nil
	}, func(conn *sql.Conn, c privateFactContext) (CultureFact, func() error, error) {
		fact, apply, err := prepare(conn, c)
		fact.Version = "corerp.culture.v1"
		return fact, apply, err
	})
}

func (s *Store) DefineRPCulture(ctx context.Context, r CultureDefinitionRequest) (CultureRecord, error) {
	if r.Culture.VersionEventID != "" || (r.Culture.ScopeKind == "community" && r.Culture.ScopeID != r.Culture.CultureID) {
		return CultureRecord{}, core.NewError(core.CodeInvalidArgument, "culture definition requires supported scope and server-assigned version")
	}
	check := r.Culture
	check.VersionEventID = "pending"
	if err := check.Validate(); err != nil {
		return CultureRecord{}, err
	}
	return s.executeScopedCultureCommand(ctx, r.Binding, "DefineRPCulture", r, r.AuthorID, func(conn *sql.Conn) error {
		if r.Culture.ScopeKind == "world" || r.Culture.ScopeKind == "region" {
			return authorizeCultureTerritory(ctx, conn, r.Binding, r.Culture.ScopeKind, r.Culture.ScopeID)
		}
		if r.Culture.ScopeKind == "organization" {
			return authorizeCareerManager(ctx, conn, r.Binding, r.Culture.ScopeID)
		}
		if r.Culture.ScopeKind == "family" {
			_, err := requireCulturalFamilyMember(ctx, conn, r.Binding, r.Culture.ScopeID, r.AuthorID)
			return err
		}
		return nil
	}, func(conn *sql.Conn, c privateFactContext) (CultureFact, func() error, error) {
		if err := validateCultureLineage(ctx, conn, r); err != nil {
			return CultureFact{}, nil, err
		}
		definition := r.Culture
		definition.VersionEventID = c.EventID
		fact := CultureFact{Kind: "definition", ActorID: r.AuthorID, Definition: &definition, PreviousDefinitionEventID: r.PreviousDefinitionEventID, ParentDefinitionEventIDs: append([]string(nil), r.ParentDefinitionEventIDs...)}
		if definition.ScopeKind == "world" || definition.ScopeKind == "region" {
			_, source, err := readCultureTerritory(ctx, conn, r.Binding, definition.ScopeKind, definition.ScopeID)
			if err != nil {
				return CultureFact{}, nil, err
			}
			fact.ScopeSourceEventID = source
		}
		if definition.ScopeKind == "family" {
			source, err := requireCulturalFamilyMember(ctx, conn, r.Binding, definition.ScopeID, r.AuthorID)
			if err != nil {
				return CultureFact{}, nil, err
			}
			fact.ScopeSourceEventID = source
		}
		if definition.ScopeKind == "organization" {
			org, err := readCareerRecord(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, "organization", definition.ScopeID)
			if err != nil {
				return CultureFact{}, nil, err
			}
			if org.Fact.Organization == nil {
				return CultureFact{}, nil, core.NewError(core.CodeProjectionDiverged, "culture lacks actual organization")
			}
			fact.ScopeSourceEventID = org.EventID
		}
		return fact, nil, nil
	})
}

func readCultureFact(ctx context.Context, conn *sql.Conn, b core.CareerBinding, eventID string) (CultureFact, error) {
	var raw string
	var fact CultureFact
	err := conn.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPCultureFactRecorded'`, eventID, b.InstanceID, b.BranchID).Scan(&raw)
	if err != nil {
		return fact, classifyMissing(err, "culture source")
	}
	if err := json.Unmarshal([]byte(raw), &fact); err != nil {
		return fact, core.WrapError(core.CodeProjectionDiverged, "culture source", err)
	}
	return fact, nil
}

func (s *Store) TransmitRPCulture(ctx context.Context, r CultureTransmissionRequest) (CultureRecord, error) {
	return s.executeCultureCommand(ctx, r.Binding, "TransmitRPCulture", r, r.SpeakerID, func(conn *sql.Conn, c privateFactContext) (CultureFact, func() error, error) {
		definition, err := readCultureFact(ctx, conn, r.Binding, r.DefinitionEventID)
		if err != nil {
			return CultureFact{}, nil, err
		}
		if definition.Kind != "definition" || definition.Definition == nil {
			return CultureFact{}, nil, core.NewError(core.CodeInvalidArgument, "transmission requires actual definition")
		}
		if definition.ActorID != r.SpeakerID {
			var heard int
			err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events e,json_each(e.payload,'$.listener_ids') l WHERE e.instance_id=? AND e.branch_id=? AND e.event_type='RPCultureFactRecorded' AND json_extract(e.payload,'$.kind')='transmission' AND json_extract(e.payload,'$.definition_event_id')=? AND l.value=?`, r.Binding.InstanceID, r.Binding.BranchID, r.DefinitionEventID, r.SpeakerID).Scan(&heard)
			if err != nil {
				return CultureFact{}, nil, err
			}
			if heard == 0 {
				return CultureFact{}, nil, core.NewError(core.CodeUnauthorized, "speaker has not learned this culture version")
			}
		}
		var place string
		if err := conn.QueryRowContext(ctx, `SELECT place_id FROM agent_positions WHERE agent_id=?`, r.SpeakerID).Scan(&place); err != nil {
			return CultureFact{}, nil, err
		}
		listeners, err := rpPerceivedEntityIDs(ctx, conn, r.Binding.InstanceID, r.Binding.BranchID, place, r.SpeakerID, "audio", "voice")
		if err != nil {
			return CultureFact{}, nil, err
		}
		encoded, err := core.CanonicalJSON(*definition.Definition)
		if err != nil {
			return CultureFact{}, nil, err
		}
		text := "Culture statement: " + string(encoded)
		if len(text) > 2000 {
			return CultureFact{}, nil, core.NewError(core.CodeInvalidArgument, "culture statement exceeds bounded speech; split definition before transmission")
		}
		fact := CultureFact{Kind: "transmission", ActorID: r.SpeakerID, DefinitionEventID: r.DefinitionEventID, PlaceID: place, ListenerIDs: listeners}
		return fact, func() error {
			return insertRPSpeechHearings(ctx, conn, c.EventID, c.Sequence, r.SpeakerID, place, c.WorldTime, "utterance_"+c.EventID, text, "statement", listeners)
		}, nil
	})
}

func (s *Store) InternalizeRPCulture(ctx context.Context, r CultureStanceRequest) (CultureRecord, error) {
	if (r.DefinitionEventID == "") == (r.TransmissionEventID == "") {
		return CultureRecord{}, core.NewError(core.CodeInvalidArgument, "choose received transmission or own authored definition")
	}
	if r.DefinitionEventID != "" {
		return s.executeCultureCommand(ctx, r.Binding, "InternalizeRPCulture", r, r.EntityID, func(conn *sql.Conn, c privateFactContext) (CultureFact, func() error, error) {
			definition, err := readCultureFact(ctx, conn, r.Binding, r.DefinitionEventID)
			if err != nil {
				return CultureFact{}, nil, err
			}
			if definition.Kind != "definition" || definition.Definition == nil || definition.ActorID != r.EntityID {
				return CultureFact{}, nil, core.NewError(core.CodeUnauthorized, "self-authored stance requires actual authorship")
			}
			stance := core.RPCultureInternalization{KnowledgeKind: "authored", EntityID: r.EntityID, CultureID: definition.Definition.CultureID, VersionEventID: r.DefinitionEventID, StanceEventID: c.EventID, Stance: r.Stance}
			if _, err := core.EvaluateRPCulture(*definition.Definition, stance, r.EntityID, "gift"); err != nil {
				return CultureFact{}, nil, err
			}
			return CultureFact{Kind: "internalization", ActorID: r.EntityID, DefinitionEventID: r.DefinitionEventID, Internalization: &stance}, nil, nil
		})
	}
	return s.executeCultureCommand(ctx, r.Binding, "InternalizeRPCulture", r, r.EntityID, func(conn *sql.Conn, c privateFactContext) (CultureFact, func() error, error) {
		transmission, err := readCultureFact(ctx, conn, r.Binding, r.TransmissionEventID)
		if err != nil {
			return CultureFact{}, nil, err
		}
		known := false
		if transmission.Kind == "transmission" {
			for _, id := range transmission.ListenerIDs {
				if id == r.EntityID {
					known = true
				}
			}
		}
		if !known {
			return CultureFact{}, nil, core.NewError(core.CodeUnauthorized, "internalization requires actual heard transmission")
		}
		definition, err := readCultureFact(ctx, conn, r.Binding, transmission.DefinitionEventID)
		if err != nil {
			return CultureFact{}, nil, err
		}
		if definition.Definition == nil {
			return CultureFact{}, nil, core.NewError(core.CodeProjectionDiverged, "missing transmitted definition")
		}
		stance := core.RPCultureInternalization{EntityID: r.EntityID, CultureID: definition.Definition.CultureID, VersionEventID: transmission.DefinitionEventID, TransmissionEventID: r.TransmissionEventID, StanceEventID: c.EventID, Stance: r.Stance}
		if _, err := core.EvaluateRPCulture(*definition.Definition, stance, r.EntityID, "gift"); err != nil {
			return CultureFact{}, nil, err
		}
		return CultureFact{Kind: "internalization", ActorID: r.EntityID, DefinitionEventID: transmission.DefinitionEventID, Internalization: &stance}, nil, nil
	})
}
