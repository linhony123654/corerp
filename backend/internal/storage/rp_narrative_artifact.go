package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"corerp.local/backend/internal/core"
)

type rpNarrativeSource struct {
	ID, Actor, Kind, WorldTime, Payload, Parent, Batch, CommandKind, DecisionAction string
	Sequence, Index, Last                                                           int64
}

func narrativeDiverged(message string) error {
	return core.NewError(core.CodeProjectionDiverged, message)
}

func loadRPNarrativeSource(ctx context.Context, conn *sql.Conn, instance, branch, id string, head int64) (rpNarrativeSource, error) {
	var out rpNarrativeSource
	err := conn.QueryRowContext(ctx, `SELECT e.event_id,e.actor_id,e.event_type,e.world_time,e.payload,COALESCE(e.causation_event_id,''),e.batch_id,c.command_type,COALESCE(d.action,''),e.event_sequence,e.batch_index,b.last_sequence
	 FROM events e JOIN event_batches b ON b.batch_id=e.batch_id
	 JOIN commands c ON c.command_id=b.command_id
	 JOIN command_attempts a ON a.command_id=b.command_id AND a.attempt_no=b.attempt_no
	 LEFT JOIN rp_npc_decisions d ON d.event_id=e.event_id AND d.npc_entity_id=e.actor_id
	 WHERE e.event_id=? AND e.instance_id=? AND e.branch_id=?
	 AND b.instance_id=e.instance_id AND b.branch_id=e.branch_id
	 AND c.instance_id=e.instance_id AND c.branch_id=e.branch_id
	 AND c.status='committed' AND a.status='committed'
 AND b.event_count=(SELECT COUNT(*) FROM events complete WHERE complete.batch_id=b.batch_id AND complete.instance_id=b.instance_id AND complete.branch_id=b.branch_id)
 AND b.first_sequence=(SELECT MIN(complete.event_sequence) FROM events complete WHERE complete.batch_id=b.batch_id)
 AND b.last_sequence=(SELECT MAX(complete.event_sequence) FROM events complete WHERE complete.batch_id=b.batch_id)
	 AND e.event_sequence<=? AND b.last_sequence<=?`, id, instance, branch, head, head).Scan(&out.ID, &out.Actor, &out.Kind, &out.WorldTime, &out.Payload, &out.Parent, &out.Batch, &out.CommandKind, &out.DecisionAction, &out.Sequence, &out.Index, &out.Last)
	if errors.Is(err, sql.ErrNoRows) {
		return out, narrativeDiverged("narrative source lacks complete committed authority")
	}
	return out, err
}

// Historical public handles remain frozen, even after later introductions.
func validateRPNarrativePublicIdentity(ctx context.Context, conn *sql.Conn, instance, branch, observer, subject, supplied, name string, head int64) error {
	if supplied == subject {
		if subject != observer {
			var count int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_identity_familiarity f JOIN events e ON e.event_id=f.source_event_id WHERE f.instance_id=? AND f.branch_id=? AND f.observer_agent_id=? AND f.subject_agent_id=? AND e.instance_id=? AND e.branch_id=? AND e.event_sequence<=?`, instance, branch, observer, subject, instance, branch, head).Scan(&count); err != nil {
				return err
			}
			if count != 1 {
				return narrativeDiverged("narrative identity lacks historical public familiarity")
			}
		}
		var canonicalName string
		if err := conn.QueryRowContext(ctx, `SELECT json_extract(e.payload,'$.display_name') FROM materialized_entities n JOIN cohorts c ON c.cohort_id=n.source_cohort_id JOIN cohort_materializations m ON m.materialization_id=n.materialization_id JOIN events e ON e.event_id=m.materialize_event_id WHERE n.entity_id=? AND c.instance_id=? AND c.branch_id=? AND e.instance_id=c.instance_id AND e.branch_id=c.branch_id AND e.event_sequence<=?`, subject, instance, branch, head).Scan(&canonicalName); err != nil {
			return err
		}
		if canonicalName != name {
			return narrativeDiverged("narrative actor name differs from its public identity")
		}
		return nil
	}
	var known int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_identity_familiarity f JOIN events e ON e.event_id=f.source_event_id WHERE f.instance_id=? AND f.branch_id=? AND f.observer_agent_id=? AND f.subject_agent_id=? AND e.instance_id=? AND e.branch_id=? AND e.event_sequence<=?`, instance, branch, observer, subject, instance, branch, head).Scan(&known); err != nil {
		return err
	}
	if subject == observer || known != 0 {
		return narrativeDiverged("narrative anonymous handle differs from historical familiarity")
	}
	alias, err := rpAnonymousEntityID(ctx, conn, instance, branch, observer, subject)
	if err != nil {
		return err
	}
	if supplied != alias || name != "陌生人" {
		return narrativeDiverged("narrative actor is not an authorized public handle")
	}
	return nil
}

func validateRPNarrativeFact(ctx context.Context, conn *sql.Conn, instance, branch, observer string, head int64, fact core.RPNarrativeFact, source rpNarrativeSource) error {
	if source.WorldTime != fact.WorldTime {
		return narrativeDiverged("narrative time differs from its committed source")
	}
	if err := validateRPNarrativePublicIdentity(ctx, conn, instance, branch, observer, source.Actor, fact.ActorID, fact.ActorName, head); err != nil {
		return err
	}
	speech := fact.Action == "speak" || fact.Action == "respond" || fact.Action == "refuse"
	if speech {
		var event rpSpeechEvent
		if source.Kind != "RPSpeechAccepted" || json.Unmarshal([]byte(source.Payload), &event) != nil || event.SpeakerEntityID != source.Actor || event.Text != fact.Text {
			return narrativeDiverged("narrative speech differs from accepted words")
		}
		if fact.Action == "refuse" && source.DecisionAction != "refuse" || source.DecisionAction == "refuse" && fact.Action != "refuse" {
			return narrativeDiverged("narrative refusal differs from committed decision")
		}
		if source.Actor != observer {
			heard := false
			for _, id := range event.ListenerIDs {
				heard = heard || id == observer
			}
			var raw string
			err := conn.QueryRowContext(ctx, `SELECT claim_payload FROM observation_records WHERE source_event_id=? AND observer_agent_id=? AND subject_agent_id=? AND claim_key='speech:'||?`, source.ID, observer, source.Actor, source.ID).Scan(&raw)
			var claim rpSpeechClaim
			if err != nil || !heard || json.Unmarshal([]byte(raw), &claim) != nil || claim.ClaimType != "speaker_said" || claim.SpeakerEntityID != source.Actor || claim.Text != fact.Text {
				return narrativeDiverged("narrative speech lacks frozen hearing evidence")
			}
		}
		return nil
	}
	if fact.Action == "expression" {
		var event core.RPNonverbalFact
		if source.Kind != "RPNonverbalAction" || json.Unmarshal([]byte(source.Payload), &event) != nil || event.ActorEntityID != source.Actor {
			return narrativeDiverged("narrative expression differs from its source")
		}
		code := event.Action
		if code == "gesture" {
			code = event.GestureCode
		}
		if fact.ExpressionCode != code {
			return narrativeDiverged("narrative expression code differs from its source")
		}
		// An actor's own committed expression has no self-observation row.
		// Its original typed owner and complete committed source are the proof;
		// other characters still require their independently frozen witness.
		if source.Actor == observer {
			if source.CommandKind != "RPNonverbalAction" || event.PlaceID == "" {
				return narrativeDiverged("own narrative expression lacks its typed owner")
			}
			if event.TargetEntityID == "" {
				if fact.TargetActorID != "" || fact.TargetActorName != "" {
					return narrativeDiverged("own expression acquired an uncommitted target")
				}
				return nil
			}
			return validateRPNarrativePublicIdentity(ctx, conn, instance, branch, observer, event.TargetEntityID, fact.TargetActorID, fact.TargetActorName, head)
		}
		var witness *core.RPNonverbalWitness
		for i := range event.Witnesses {
			if event.Witnesses[i].ObserverEntityID == observer {
				witness = &event.Witnesses[i]
			}
		}
		if witness == nil {
			return narrativeDiverged("narrative expression lacks a frozen witness")
		}
		expected := rpNonverbalWitnessClaim(event, *witness)
		var raw string
		if err := conn.QueryRowContext(ctx, `SELECT claim_payload FROM observation_records WHERE source_event_id=? AND observer_agent_id=? AND subject_agent_id=? AND claim_key='nonverbal:'||?`, source.ID, observer, source.Actor, source.ID).Scan(&raw); err != nil {
			return narrativeDiverged("narrative expression lacks its observation")
		}
		var claim core.RPNonverbalClaim
		if json.Unmarshal([]byte(raw), &claim) != nil || !reflect.DeepEqual(claim, expected) {
			return narrativeDiverged("narrative expression witness differs from frozen event")
		}
		if expected.TargetEntityID == "" {
			if fact.TargetActorID != "" || fact.TargetActorName != "" {
				return narrativeDiverged("narrative expression reveals an unseen target")
			}
		} else if err := validateRPNarrativePublicIdentity(ctx, conn, instance, branch, observer, expected.TargetEntityID, fact.TargetActorID, fact.TargetActorName, head); err != nil {
			return err
		}
		return nil
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal([]byte(source.Payload), &payload) != nil {
		return narrativeDiverged("narrative source cannot be decoded")
	}
	field := func(key string) string { var value string; _ = json.Unmarshal(payload[key], &value); return value }
	valid := false
	switch fact.Action {
	case "silence", "wait":
		valid = source.Kind == "RPNPCDecisionRecorded" && source.DecisionAction == fact.Action
	case "act":
		valid = source.Kind == "AgentActivityStarted" && field("activity_code") == fact.ActivityCode
	case "activity_done":
		valid = source.Kind == "AgentActivityCompleted" && field("activity_code") == fact.ActivityCode
	case "activity_interrupted":
		valid = source.Kind == "AgentActivityCancelled" && field("activity_code") == fact.ActivityCode
	case "leave", "arrive", "depart":
		valid = source.Kind == "RPNPCMoved" || source.Kind == "AgentMoved"
	default:
		if strings.HasPrefix(fact.Action, "object_") {
			var observers []string
			_ = json.Unmarshal(payload["observer_entity_ids"], &observers)
			for _, id := range observers {
				valid = valid || id == observer
			}
			valid = valid && source.Kind == "RPSceneObjectStateChanged" && fact.Action == "object_"+field("action") && fact.ObjectName == field("display_name") && fact.ObjectState == field("state")
			if !valid {
				return narrativeDiverged("narrative object state lacks observed authority")
			}
			return nil
		}
	}
	if !valid {
		return narrativeDiverged("narrative action differs from its source")
	}
	at := source.Sequence
	if fact.Action == "leave" || fact.Action == "depart" {
		at--
	}
	visible, err := rpCanSeeAtSequence(ctx, conn, instance, branch, observer, source.Actor, at)
	if err != nil {
		return err
	}
	if !visible {
		return narrativeDiverged("narrative action was not observable")
	}
	return nil
}

func annotateRPNarrativeCompanions(ctx context.Context, conn *sql.Conn, instance, branch, observer string, input *core.RPNarrativeInput) error {
	sources := make(map[string]rpNarrativeSource, len(input.Facts))
	var last int64
	for i := range input.Facts {
		fact := &input.Facts[i]
		source, err := loadRPNarrativeSource(ctx, conn, instance, branch, fact.EventID, input.SourceHead)
		if err != nil {
			return err
		}
		if source.Sequence <= last {
			return narrativeDiverged("narrative facts are not unique and chronologically ordered")
		}
		last = source.Sequence
		// The window previously discarded the explicit refusal classification.
		if fact.Action == "speak" && source.DecisionAction == "refuse" {
			fact.Action = "refuse"
		}
		if err := validateRPNarrativeFact(ctx, conn, instance, branch, observer, input.SourceHead, *fact, source); err != nil {
			return err
		}
		sources[fact.EventID] = source
		fact.CompanionEventID = ""
	}
	for i := range input.Facts {
		fact := &input.Facts[i]
		if fact.Action != "expression" {
			continue
		}
		source := sources[fact.EventID]
		if source.Index != 1 || source.CommandKind != "RPNPCDecision" && source.CommandKind != "RPNPCInitiative" {
			continue
		}
		parent, err := loadRPNarrativeSource(ctx, conn, instance, branch, source.Parent, input.SourceHead)
		if err != nil {
			return err
		}
		if source.Parent == "" || parent.Batch != source.Batch || parent.Actor != source.Actor || parent.WorldTime != source.WorldTime || parent.Index != 0 || source.Sequence != parent.Sequence+1 || parent.DecisionAction == "" {
			return narrativeDiverged("NPC expression lacks exact committed parent lineage")
		}
		if parent.Kind != "RPSpeechAccepted" {
			continue
		}
		if i > 0 && input.Facts[i-1].EventID == parent.ID && input.Facts[i-1].ActorID == fact.ActorID {
			fact.CompanionEventID = parent.ID
		}
	}
	return nil
}

func validateRPNarrativeArtifactOnConn(ctx context.Context, conn *sql.Conn, instance, branch, observer, turnID string, canonical bool, artifactJSON string, view core.RPNarrativeView) (core.RPNarrativeView, error) {
	if view.CompositionVersion != core.RPFactCompositionVersionV2 {
		return view, nil
	}
	var artifact core.RPNarrativeArtifact
	decoder := json.NewDecoder(strings.NewReader(artifactJSON))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&artifact) != nil || artifact.Input.ControlledEntityID != observer || artifact.Input.SourceHead <= 0 {
		return view, narrativeDiverged("v2 narrative lacks a frozen public artifact")
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal([]byte(artifactJSON), &envelope) != nil {
		return view, narrativeDiverged("v2 narrative artifact is invalid")
	}
	parsedPlan, err := core.DecodeRPCompositionPlan(envelope["plan"], artifact.Input)
	if err != nil || !reflect.DeepEqual(parsedPlan, artifact.Plan) {
		return view, narrativeDiverged("saved narrative plan does not satisfy its strict grammar")
	}
	hash, err := core.HashJSON(artifact.Input)
	if err != nil || hash != artifact.InputSHA256 {
		return view, narrativeDiverged("frozen narrative input hash differs")
	}
	var head int64
	if err := conn.QueryRowContext(ctx, `SELECT head_sequence FROM branches WHERE instance_id=? AND branch_id=?`, instance, branch).Scan(&head); err != nil {
		return view, err
	}
	if artifact.Input.SourceHead > head {
		return view, narrativeDiverged("narrative artifact exceeds the committed head")
	}
	var headEventID string
	if err := conn.QueryRowContext(ctx, `SELECT event_id FROM events WHERE instance_id=? AND branch_id=? AND event_sequence=?`, instance, branch, artifact.Input.SourceHead).Scan(&headEventID); err != nil {
		return view, narrativeDiverged("narrative snapshot head has no committed boundary")
	}
	if _, err := loadRPNarrativeSource(ctx, conn, instance, branch, headEventID, artifact.Input.SourceHead); err != nil {
		return view, err
	}

	var sessionID, playerTurnID, playerEventID, ownerObserver, ownerInstance, ownerBranch, status string
	var settled sql.NullInt64
	if err := conn.QueryRowContext(ctx, `SELECT t.session_id,COALESCE(t.player_turn_id,t.player_event_id),t.player_event_id,s.controlled_entity_id,s.instance_id,s.branch_id,t.status,t.settled_sequence FROM rp_turn_runs t JOIN rp_sessions s ON s.session_id=t.session_id WHERE t.turn_run_id=?`, turnID).Scan(&sessionID, &playerTurnID, &playerEventID, &ownerObserver, &ownerInstance, &ownerBranch, &status, &settled); err != nil {
		return view, err
	}
	if observer != ownerObserver || instance != ownerInstance || branch != ownerBranch || (settled.Valid && settled.Int64 > 0 && canonical && artifact.Input.SourceHead != settled.Int64) || (settled.Valid && settled.Int64 > 0 && artifact.Input.SourceHead < settled.Int64) {
		return view, narrativeDiverged("narrative artifact differs from owner turn scope or head")
	}
	expected, err := readRPNarrativeInputAtHead(ctx, conn, sessionID, playerTurnID, playerEventID, artifact.Input.SourceHead)
	if err != nil {
		return view, err
	}
	if len(expected.Facts) != len(artifact.Input.Facts) || !reflect.DeepEqual(expected.ActivityLabels, artifact.Input.ActivityLabels) {
		return view, narrativeDiverged("narrative artifact differs from its complete turn input")
	}
	for i := range expected.Facts {
		// Identity is separately checked against historical public evidence.
		a, b := expected.Facts[i], artifact.Input.Facts[i]
		a.ActorID, b.ActorID = "", ""
		a.ActorName, b.ActorName = "", ""
		if a.Action == "expression" && b.Action == "expression" {
			a.TargetActorID, b.TargetActorID = "", ""
			a.TargetActorName, b.TargetActorName = "", ""
		}
		if !reflect.DeepEqual(a, b) {
			return view, narrativeDiverged("narrative typed fields differ from the source projection")
		}
	}
	cues, err := readRPPublicPresentations(ctx, conn, instance, branch)
	if err != nil {
		return view, err
	}
	seenCues := map[string]bool{}
	for _, cue := range artifact.Input.PublicPresentations {
		authorized, ok := cues[cue.ActorID]
		if !ok || seenCues[cue.ActorID] || cue.SourceEventID != authorized.SourceEventID || cue.Text != authorized.Text {
			return view, narrativeDiverged("narrative character cue differs from published source")
		}
		found := false
		for _, fact := range artifact.Input.Facts {
			if fact.ActorID == cue.ActorID && fact.ActorName == cue.ActorName && fact.ActorID != observer {
				found = true
			}
		}
		if !found {
			return view, narrativeDiverged("narrative cue lacks a public actor in this turn")
		}
		seenCues[cue.ActorID] = true
	}
	for _, fact := range artifact.Input.Facts {
		if _, exists := cues[fact.ActorID]; exists && fact.ActorID != observer && !seenCues[fact.ActorID] {
			return view, narrativeDiverged("narrative omits an applicable public character cue")
		}
	}
	// Re-derive companion links from world authority, retaining historical names.
	verified := artifact.Input
	verified.Facts = append([]core.RPNarrativeFact(nil), artifact.Input.Facts...)
	if err := annotateRPNarrativeCompanions(ctx, conn, instance, branch, observer, &verified); err != nil {
		return view, err
	}
	if !reflect.DeepEqual(verified.Facts, artifact.Input.Facts) {
		return view, narrativeDiverged("frozen narrative facts or companions differ from source authority")
	}
	expanded, err := core.RenderRPComposition(ctx, artifact.Input, artifact.Plan, nil)
	if err != nil {
		return view, narrativeDiverged("frozen narrative plan is invalid")
	}
	if expanded.CompositionVersion != view.CompositionVersion || !reflect.DeepEqual(expanded.Lines, view.Lines) || !reflect.DeepEqual(expanded.EventIDs, view.EventIDs) || !reflect.DeepEqual(expanded.FactGroups, view.FactGroups) {
		return view, narrativeDiverged("saved narrative differs from frozen plan expansion")
	}
	expanded.RenderID = view.RenderID
	expanded.FallbackReason = view.FallbackReason
	return expanded, nil
}

func encodeRPNarrativeArtifact(view core.RPNarrativeView) (string, error) {
	if view.CompositionVersion != core.RPFactCompositionVersionV2 {
		return "{}", nil
	}
	if view.Artifact == nil {
		return "", narrativeDiverged("v2 narrative has no presentation artifact")
	}
	hash, err := core.HashJSON(view.Artifact.Input)
	if err != nil || hash != view.Artifact.InputSHA256 {
		return "", narrativeDiverged("v2 narrative artifact hash differs")
	}
	expanded, err := core.RenderRPComposition(context.Background(), view.Artifact.Input, view.Artifact.Plan, nil)
	if err != nil || !reflect.DeepEqual(expanded.Lines, view.Lines) || !reflect.DeepEqual(expanded.EventIDs, view.EventIDs) || !reflect.DeepEqual(expanded.FactGroups, view.FactGroups) {
		return "", narrativeDiverged("v2 narrative is not its finite plan expansion")
	}
	encoded, err := core.CanonicalJSON(view.Artifact)
	return string(encoded), err
}

func decodeRPNarrativeReceipt(version, linesJSON, groupsJSON, idsJSON string) (core.RPNarrativeView, error) {
	view := core.RPNarrativeView{CompositionVersion: version}
	if json.Unmarshal([]byte(linesJSON), &view.Lines) != nil || json.Unmarshal([]byte(idsJSON), &view.EventIDs) != nil {
		return view, narrativeDiverged("narrative receipt cannot be decoded")
	}
	groups, err := decodeRPNarrativeComposition(version, groupsJSON, idsJSON, view.Lines)
	view.FactGroups = groups
	return view, err
}

// Callers first validate the independent ID JSON with decodeRPNarrativeComposition.
func mustRPNarrativeIDs(encoded string) []string {
	var ids []string
	_ = json.Unmarshal([]byte(encoded), &ids)
	return ids
}

func readRPNarrativePackagesAtHead(ctx context.Context, conn *sql.Conn, instance, branch string, head int64) (*studioActivePackages, error) {
	var definition string
	if err := conn.QueryRowContext(ctx, `SELECT world_definition_id FROM world_instances WHERE instance_id=?`, instance).Scan(&definition); err != nil {
		return nil, err
	}
	if definition != "corerp.studio.world" {
		return nil, nil
	}
	var epoch, hash, raw, eventID string
	var start int64
	if err := conn.QueryRowContext(ctx, `SELECT epoch_id,ruleset_hash,lock_document,COALESCE(activation_event_id,''),start_sequence FROM rule_epochs WHERE instance_id=? AND branch_id=? AND start_sequence<=? AND (end_sequence IS NULL OR ?<end_sequence)`, instance, branch, head+1, head+1).Scan(&epoch, &hash, &raw, &eventID, &start); err != nil {
		return nil, err
	}
	return readStudioPinnedPackages(ctx, conn, instance, branch, epoch, hash, raw, eventID, start)
}

// Places have a supported rename owner; replay resolves the frozen historical name.
func rpNarrativePlaceNameAtHead(ctx context.Context, conn *sql.Conn, instance, branch, place string, head int64) (string, error) {
	var name string
	err := conn.QueryRowContext(ctx, `SELECT json_extract(payload,'$.new_name') FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPLocationRenamed' AND json_extract(payload,'$.location_id')=? AND event_sequence<=? ORDER BY event_sequence DESC LIMIT 1`, instance, branch, place, head).Scan(&name)
	if err == nil {
		return name, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	var kind, raw string
	err = conn.QueryRowContext(ctx, `SELECT p.display_name,e.event_type,e.payload FROM agent_places p JOIN events e ON e.event_id=p.definition_event_id WHERE p.place_id=? AND p.instance_id=? AND p.branch_id=? AND e.event_sequence<=?`, place, instance, branch, head).Scan(&name, &kind, &raw)
	if err != nil {
		return "", err
	}
	switch kind {
	case "RPLocationMaterialized":
		var fact RPLocationFact
		if json.Unmarshal([]byte(raw), &fact) != nil || fact.LocationID != place {
			return "", narrativeDiverged("place declaration differs")
		}
		return fact.DisplayName, nil
	case "StudioSpatialPrepared":
		var genesis string
		if err := conn.QueryRowContext(ctx, `SELECT payload FROM events WHERE instance_id=? AND branch_id=? AND event_type='StudioWorldPrepared' AND event_sequence=1`, instance, branch).Scan(&genesis); err != nil {
			return "", err
		}
		var declared struct {
			Request StudioGenesisRequest `json:"request"`
		}
		if json.Unmarshal([]byte(genesis), &declared) != nil {
			return "", narrativeDiverged("place genesis is invalid")
		}
		for _, p := range declared.Request.Spec.Places {
			id, err := core.StudioWorldObjectID(instance, "place", p.Key)
			if err != nil {
				return "", err
			}
			if id == place {
				return p.Name, nil
			}
		}
		return "", narrativeDiverged("place is absent from declaration")
	default:
		// Fixed legacy bootstrap names precede supported spatial renames.
		var old string
		err := conn.QueryRowContext(ctx, `SELECT json_extract(payload,'$.old_name') FROM events WHERE instance_id=? AND branch_id=? AND event_type='RPLocationRenamed' AND json_extract(payload,'$.location_id')=? ORDER BY event_sequence LIMIT 1`, instance, branch, place).Scan(&old)
		if err == nil {
			return old, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		return name, nil
	}
}

func rpNarrativeIdentityKnownAtHead(ctx context.Context, conn *sql.Conn, instance, branch, observer, subject string, head int64) (bool, error) {
	if observer == subject {
		return true, nil
	}
	var count int
	err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_identity_familiarity f JOIN events e ON e.event_id=f.source_event_id WHERE f.instance_id=? AND f.branch_id=? AND f.observer_agent_id=? AND f.subject_agent_id=? AND e.instance_id=? AND e.branch_id=? AND e.event_sequence<=?`, instance, branch, observer, subject, instance, branch, head).Scan(&count)
	return count > 0, err
}
