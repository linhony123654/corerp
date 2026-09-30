package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

type RPObjectResult struct {
	EventID       string `json:"event_id"`
	EventSequence int64  `json:"event_sequence"`
	ObjectID      string `json:"object_id"`
	OfferID       string `json:"offer_id,omitempty"`
	Description   string `json:"description"`
	Replayed      bool   `json:"replayed"`
}

type rpObjectRow struct {
	ID, SourceID, SKU, Name, Owner, Escrow, State, Holder, Anchor, Place, Zone, StageEvent, LastEvent string
	UnitMinor, Sequence                                                                               int64
}

type rpObjectOfferRow struct {
	ID, ObjectID, From, Target, Status, OfferEvent, ResponseEvent, TerminalEvent string
	Sequence                                                                     int64
}

type rpObjectMutation struct {
	Object   rpObjectRow
	Offer    rpObjectOfferRow
	From     string
	To       string
	NewStock bool
}

// ObjectRP is a typed Event owner. A proposal never changes physical state;
// only this transaction can serialize a SKU unit, move an object, record the
// other actor's response or perform a later, separate consensual transfer.
func (s *Store) ObjectRP(ctx context.Context, r core.RPObjectRequest) (RPObjectResult, error) {
	var empty RPObjectResult
	if err := r.Validate(); err != nil {
		return empty, err
	}
	requestHash, err := core.HashJSON(r)
	if err != nil {
		return empty, err
	}
	tx, err := beginImmediate(ctx, s.db)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	session, err := loadRPSessionRecord(ctx, tx.conn, r.PrincipalID, r.SessionID)
	if err != nil {
		return empty, err
	}
	key := "rp_object:" + session.SessionID + ":" + r.IdempotencyKey
	if err := checkRPTypedActionRetirement(ctx, tx.conn, r.PrincipalID, "object", session.SessionID, r.IdempotencyKey); err != nil {
		return empty, err
	}
	var oldHash, oldStatus, oldCommandID string
	err = tx.conn.QueryRowContext(ctx, `SELECT request_hash,status,command_id FROM commands WHERE instance_id=? AND branch_id=? AND command_type='RPObjectInteraction' AND idempotency_key=?`, session.InstanceID, session.BranchID, key).Scan(&oldHash, &oldStatus, &oldCommandID)
	if err == nil {
		if oldHash != requestHash {
			return empty, core.NewError(core.CodeIdempotencyMismatch, "object request key has different payload")
		}
		if oldStatus != "committed" {
			return empty, core.NewError(core.CodeCommandInProgress, "object command has not committed")
		}
		var raw string
		var result RPObjectResult
		if err := tx.conn.QueryRowContext(ctx, `SELECT e.event_id,e.event_sequence,e.payload FROM event_batches b JOIN events e ON e.batch_id=b.batch_id WHERE b.command_id=? AND e.event_type='RPObjectInteracted'`, oldCommandID).Scan(&result.EventID, &result.EventSequence, &raw); err != nil {
			return empty, classifyMissing(err, "committed object action")
		}
		var fact core.RPObjectEvidence
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			return empty, err
		}
		result.ObjectID, result.OfferID, result.Description, result.Replayed = fact.ObjectID, fact.OfferID, fact.Description, true
		return result, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	if session.Status != "active" {
		return empty, core.NewError(core.CodeBranchConflict, "object action requires an active RP session")
	}
	if err := requireCurrentRPSession(ctx, tx.conn, session); err != nil {
		return empty, err
	}
	if err := authorizeRPControl(ctx, tx.conn, r.PrincipalID, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return empty, err
	}
	if err := requireNoActiveRPSharedRound(ctx, tx.conn, session.InstanceID, session.BranchID); err != nil {
		return empty, err
	}
	var pending int
	if err := tx.conn.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM rp_turn_runs t JOIN rp_sessions s ON s.session_id=t.session_id WHERE s.instance_id=? AND s.branch_id=? AND t.status<>'settled')+(SELECT COUNT(*) FROM rp_wait_intents w JOIN rp_sessions s ON s.session_id=w.session_id WHERE s.instance_id=? AND s.branch_id=? AND w.status='pending')`, session.InstanceID, session.BranchID, session.InstanceID, session.BranchID).Scan(&pending); err != nil {
		return empty, err
	}
	if pending != 0 {
		return empty, core.NewError(core.CodeCommandInProgress, "finish pending RP action before touching an object")
	}
	if err := validateRPBinding(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID); err != nil {
		return empty, err
	}
	var head int64
	var worldTime, epoch string
	if err := tx.conn.QueryRowContext(ctx, `SELECT b.head_sequence,c.current_world_time FROM branches b JOIN world_clocks c ON c.instance_id=b.instance_id AND c.branch_id=b.branch_id WHERE b.instance_id=? AND b.branch_id=?`, session.InstanceID, session.BranchID).Scan(&head, &worldTime); err != nil {
		return empty, err
	}
	if r.ExpectedCursor != head || session.ObservationCursor != head {
		return empty, core.NewError(core.CodeBranchConflict, "observe current world before an object action")
	}
	if err := ensureCohortTransitionChronology(ctx, tx.conn, session.InstanceID, session.BranchID, worldTime); err != nil {
		return empty, err
	}
	point, err := readRPActorSpatialPoint(ctx, tx.conn, session.InstanceID, session.BranchID, session.ControlledEntityID)
	if err != nil {
		return empty, err
	}
	sequence := head + 1
	if err := tx.conn.QueryRowContext(ctx, `SELECT epoch_id FROM rule_epochs WHERE instance_id=? AND branch_id=? AND start_sequence<=? AND (end_sequence IS NULL OR ?<end_sequence)`, session.InstanceID, session.BranchID, sequence, sequence).Scan(&epoch); err != nil {
		return empty, err
	}
	keyHash, err := core.HashJSON([]string{session.InstanceID, session.BranchID, key})
	if err != nil {
		return empty, err
	}
	suffix := keyHash[7:]
	commandID, attemptID, batchID, eventID := "cmd_rp_object_"+suffix, "attempt_rp_object_"+suffix, "batch_rp_object_"+suffix, "event_rp_object_"+suffix
	fact, mutation, err := prepareRPObjectAction(ctx, tx.conn, session, r, point, eventID)
	if err != nil {
		return empty, err
	}
	fact.WitnessIDs, fact.VisibleTargetWitnessIDs, err = readRPObjectWitnesses(ctx, tx.conn, session, point.PlaceID, fact.TargetEntityID)
	if err != nil {
		return empty, err
	}
	payload, err := core.CanonicalJSON(fact)
	if err != nil {
		return empty, err
	}
	batchHash, err := core.HashJSON(struct {
		RequestHash string
		Sequence    int64
		WorldTime   string
		Evidence    core.RPObjectEvidence
	}{requestHash, sequence, worldTime, fact})
	if err != nil {
		return empty, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	for _, statement := range []struct {
		name, query string
		args        []any
	}{
		{"object command", `INSERT INTO commands(command_id,instance_id,branch_id,command_type,idempotency_key,request_hash,expected_head,principal_id,command_policy,status,created_at_utc) VALUES (?,?,?,'RPObjectInteraction',?,?,?,?,'{"authorization":"rp-session-control;single-sku-item"}','pending',?)`, []any{commandID, session.InstanceID, session.BranchID, key, requestHash, head, r.PrincipalID, now}},
		{"object attempt", `INSERT INTO command_attempts(command_id,attempt_no,attempt_id,status,lease_owner,lease_until_utc,proposal_hash,created_at_utc) VALUES (?,1,?,'ready','corerp-rp-object',?,?,?)`, []any{commandID, attemptID, s.now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano), requestHash, now}},
		{"object event batch", `INSERT INTO event_batches(batch_id,command_id,attempt_no,instance_id,branch_id,epoch_id,expected_head,first_sequence,last_sequence,event_count,world_time,batch_hash,committed_at_utc) VALUES (?,?,1,?,?,?,?,?,?,1,?,?,?)`, []any{batchID, commandID, session.InstanceID, session.BranchID, epoch, head, sequence, sequence, worldTime, batchHash, now}},
		{"object fact", `INSERT INTO events(event_id,batch_id,instance_id,branch_id,event_sequence,batch_index,event_type,actor_id,world_time,payload) VALUES (?,?,?,?,?,0,'RPObjectInteracted',?,?,?)`, []any{eventID, batchID, session.InstanceID, session.BranchID, sequence, session.ControlledEntityID, worldTime, string(payload)}},
	} {
		if err := execAgentOne(ctx, tx.conn, statement.name, statement.query, statement.args...); err != nil {
			return empty, err
		}
	}
	if err := applyRPObjectAction(ctx, tx.conn, session, fact, mutation, eventID, sequence); err != nil {
		return empty, err
	}
	if err := execAgentOne(ctx, tx.conn, "object own-action evidence", `INSERT INTO rp_own_actions(agent_id,event_id,action,activity_code,text,place_id,world_time,status,instance_id,branch_id,last_event_sequence) VALUES (?,?,?,'rp_object',?,?,?,'completed',?,?,?)`, session.ControlledEntityID, eventID, r.Action, fact.Description, point.PlaceID, worldTime, session.InstanceID, session.BranchID, sequence); err != nil {
		return empty, err
	}
	// Each witnessed claim is deliberately less informative than the Event:
	// neither a SKU ledger nor somebody else's consent/identity is observable.
	neutralClaim, err := rpObjectNeutralClaim()
	if err != nil {
		return empty, err
	}
	for _, other := range fact.WitnessIDs {
		observationID := "observation_" + eventID + "_" + other
		claimKey := "object:" + eventID
		if err := execAgentOne(ctx, tx.conn, "visible object observation", `INSERT INTO observation_records(observation_id,source_event_id,observer_agent_id,subject_agent_id,place_id,channel,observed_world_time,claim_key,claim_payload) VALUES (?,?,?,?,?,'co_location',?,?,?)`, observationID, eventID, other, session.ControlledEntityID, point.PlaceID, worldTime, claimKey, neutralClaim); err != nil {
			return empty, err
		}
		if err := execAgentOne(ctx, tx.conn, "visible object knowledge", `INSERT INTO agent_knowledge(observer_agent_id,claim_key,subject_agent_id,place_id,source_event_id,observation_id,learned_world_time,claim_payload,projection_version,last_event_sequence) VALUES (?,?,?,?,?,?,?,?,0,?)`, other, claimKey, session.ControlledEntityID, point.PlaceID, eventID, observationID, worldTime, neutralClaim, sequence); err != nil {
			return empty, err
		}
	}
	if err := execAgentOne(ctx, tx.conn, "object clock lineage", `UPDATE world_clocks SET projection_version=projection_version+1,last_event_sequence=? WHERE instance_id=? AND branch_id=? AND current_world_time=?`, sequence, session.InstanceID, session.BranchID, worldTime); err != nil {
		return empty, err
	}
	if err := execAgentOne(ctx, tx.conn, "object branch head", `UPDATE branches SET head_sequence=? WHERE instance_id=? AND branch_id=? AND head_sequence=?`, sequence, session.InstanceID, session.BranchID, head); err != nil {
		return empty, err
	}
	if err := s.insertAgentAudit(ctx, tx.conn, "audit_"+commandID, "agent_decision", eventID, commandID, attemptID, worldTime, now, payload); err != nil {
		return empty, err
	}
	if err := insertRPParticipantOutbox(ctx, tx.conn, "outbox_"+commandID, eventID, "rp.object.interacted", session.InstanceID, session.BranchID, session.ControlledEntityID, nil, []byte(neutralClaim)); err != nil {
		return empty, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit object command", `UPDATE commands SET status='committed' WHERE command_id=? AND status='pending'`, commandID); err != nil {
		return empty, err
	}
	if err := execAgentOne(ctx, tx.conn, "commit object attempt", `UPDATE command_attempts SET status='committed',finished_at_utc=? WHERE attempt_id=? AND status='ready'`, now, attemptID); err != nil {
		return empty, err
	}
	if s.beforeCommit != nil {
		if err := s.beforeCommit(); err != nil {
			return empty, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return RPObjectResult{EventID: eventID, EventSequence: sequence, ObjectID: fact.ObjectID, OfferID: fact.OfferID, Description: fact.Description}, nil
}

func prepareRPObjectAction(ctx context.Context, conn *sql.Conn, session RPSession, r core.RPObjectRequest, point rpActorSpatialPoint, eventID string) (core.RPObjectEvidence, rpObjectMutation, error) {
	var fact core.RPObjectEvidence
	var mutation rpObjectMutation
	actor := session.ControlledEntityID
	fact.Version, fact.ClaimType, fact.SessionID, fact.ActorEntityID, fact.Action = "corerp.object.interaction.v1", "object_interaction", session.SessionID, actor, r.Action
	fact.PlaceID, fact.ZoneKey = point.PlaceID, point.ZoneKey
	if r.Action == "stage" {
		var sourceEvent, sourceOwner, location, anchorID, anchorPlace, anchorZone, sku, name string
		var unit int64
		err := conn.QueryRowContext(ctx, `SELECT s.definition_event_id,s.owner_actor_id,s.inventory_location_id,s.anchor_id,a.place_id,a.zone_key,s.sku_id,s.display_name,s.unit_minor FROM rp_object_sources s JOIN rp_object_anchors a ON a.anchor_id=s.anchor_id AND a.instance_id=s.instance_id AND a.branch_id=s.branch_id WHERE s.source_id=? AND s.instance_id=? AND s.branch_id=?`, r.SourceID, session.InstanceID, session.BranchID).Scan(&sourceEvent, &sourceOwner, &location, &anchorID, &anchorPlace, &anchorZone, &sku, &name, &unit)
		if err != nil {
			return fact, mutation, classifyMissing(err, "declared actor-owned object source")
		}
		if sourceOwner != actor || anchorPlace != point.PlaceID || anchorZone != point.ZoneKey {
			return fact, mutation, core.NewError(core.CodeNotFound, "object source is not actor-owned and within reach")
		}
		var sourceRaw string
		if err := conn.QueryRowContext(ctx, `SELECT payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPObjectSourceDefined'`, sourceEvent, session.InstanceID, session.BranchID).Scan(&sourceRaw); err != nil {
			return fact, mutation, classifyMissing(err, "committed object source")
		}
		var declared RPObjectSourceFact
		if err := json.Unmarshal([]byte(sourceRaw), &declared); err != nil {
			return fact, mutation, err
		}
		if declared.Version != "corerp.object.source.v1" || declared.SourceID != r.SourceID || declared.OwnerEntityID != actor || declared.AnchorID != anchorID || declared.InventoryLocationID != location || declared.SKUID != sku || declared.DisplayName != name || declared.UnitMinor != unit {
			return fact, mutation, core.NewError(core.CodeProjectionDiverged, "object source differs from its committed Event")
		}
		var actualLocation string
		if err := conn.QueryRowContext(ctx, `SELECT m.inventory_location_id FROM materialized_entities m JOIN agent_profiles a ON a.agent_id=m.entity_id AND a.instance_id=? AND a.branch_id=? AND a.status='active' JOIN stock_locations l ON l.location_id=m.inventory_location_id AND l.owner_id=m.entity_id AND l.location_kind='holder' WHERE m.entity_id=? AND m.status='active' AND m.population_count=1`, session.InstanceID, session.BranchID, actor).Scan(&actualLocation); err != nil || actualLocation != location {
			return fact, mutation, core.NewError(core.CodeProjectionDiverged, "object source no longer matches actor's own holder stock")
		}
		var scale int
		var baseUnit, skuEvent string
		if err := conn.QueryRowContext(ctx, `SELECT base_unit,quantity_scale,definition_event_id FROM product_skus WHERE sku_id=?`, sku).Scan(&baseUnit, &scale, &skuEvent); err != nil || baseUnit != "unit" || scale != 0 || skuEvent != declared.SKUEventID {
			return fact, mutation, core.NewError(core.CodeProjectionDiverged, "object SKU definition is unavailable or changed")
		}
		var authoredName sql.NullString
		if err := conn.QueryRowContext(ctx, `SELECT CASE WHEN json_extract(payload,'$.sku_id')=? THEN json_extract(payload,'$.sku_display_name') END FROM events WHERE event_id=? AND instance_id=? AND branch_id=?`, sku, skuEvent, session.InstanceID, session.BranchID).Scan(&authoredName); err != nil {
			return fact, mutation, classifyMissing(err, "branch-local object SKU definition")
		}
		skuLabel := sku
		if authoredName.Valid && authoredName.String != "" {
			skuLabel = authoredName.String
		}
		if name != skuLabel || !validRPObjectName(skuLabel) {
			return fact, mutation, core.NewError(core.CodeProjectionDiverged, "object label no longer matches its SKU definition")
		}
		actualUnit := int64(1)
		for i := 0; i < scale; i++ {
			actualUnit *= 10
		}
		if unit != actualUnit || !validRPObjectName(name) {
			return fact, mutation, core.NewError(core.CodeProjectionDiverged, "object source differs from SKU definition")
		}
		available, _, err := readRPStoreStock(ctx, conn, session.InstanceID, session.BranchID, location, sku)
		if err != nil {
			return fact, mutation, err
		}
		if available < unit {
			return fact, mutation, core.NewError(core.CodeBranchConflict, "no unescrowed single SKU unit remains")
		}
		fact.ObjectID = "rp_object_" + strings.TrimPrefix(eventID, "event_rp_object_")
		fact.SourceID, fact.SourceEventID = r.SourceID, sourceEvent
		fact.SKUID, fact.UnitMinor, fact.DisplayName, fact.OwnerEntityID, fact.HolderEntityID = sku, unit, name, actor, actor
		fact.FromLocationID, fact.ToLocationID = location, rpObjectEscrowID(fact.ObjectID, actor)
		fact.Description = "拿出一件已声明且实际持有的物品。"
		mutation.NewStock, mutation.From, mutation.To = true, fact.FromLocationID, fact.ToLocationID
		return fact, mutation, nil
	}
	if r.OfferID != "" {
		var err error
		mutation.Offer, err = loadRPObjectOffer(ctx, conn, session, r.OfferID)
		if err != nil {
			return fact, mutation, err
		}
		r.ObjectID = mutation.Offer.ObjectID
	}
	obj, err := loadRPObject(ctx, conn, session, r.ObjectID)
	if err != nil {
		return fact, mutation, err
	}
	mutation.Object = obj
	if obj.State == "stowed" {
		return fact, mutation, core.NewError(core.CodeBranchConflict, "stowed object cannot be touched again; stage a new unit")
	}
	fact.ObjectID, fact.SKUID, fact.UnitMinor, fact.DisplayName, fact.OwnerEntityID = obj.ID, obj.SKU, obj.UnitMinor, obj.Name, obj.Owner
	fact.SourceID, fact.PreviousEvent = obj.SourceID, obj.LastEvent
	if obj.State == "held" {
		fact.HolderEntityID = obj.Holder
	} else {
		fact.AnchorID = obj.Anchor
	}
	// A held object's world point follows its holder's actual spatial position;
	// an old place recorded on its last action cannot pin it to a room forever.
	if obj.State == "held" {
		ownerPoint, err := readRPActorSpatialPoint(ctx, conn, session.InstanceID, session.BranchID, obj.Holder)
		if err != nil {
			return fact, mutation, err
		}
		if point != ownerPoint {
			return fact, mutation, core.NewError(core.CodeNotFound, "held object is outside the actor's reach")
		}
	} else if obj.Place != point.PlaceID || obj.Zone != point.ZoneKey {
		return fact, mutation, core.NewError(core.CodeNotFound, "placed object is outside the actor's reach")
	}
	var activeOffer int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_object_offers WHERE object_id=? AND instance_id=? AND branch_id=? AND status IN ('offered','accepted_pending_transfer')`, obj.ID, session.InstanceID, session.BranchID).Scan(&activeOffer); err != nil {
		return fact, mutation, err
	}
	if r.Action != "offer" && r.Action != "accept" && r.Action != "refuse" && r.Action != "give" && r.Action != "receive" && r.Action != "cancel_offer" && activeOffer != 0 {
		return fact, mutation, core.NewError(core.CodeBranchConflict, "object is locked by an unresolved offer")
	}
	switch r.Action {
	case "take":
		if obj.Owner != actor || obj.State != "placed" {
			return fact, mutation, core.NewError(core.CodeNotFound, "only the owner may pick up a reachable placed object")
		}
		fact.AnchorID, fact.HolderEntityID, fact.Description = "", actor, "拿起了放在近处的物品。"
	case "stow":
		if obj.Owner != actor || obj.State != "held" || obj.Holder != actor {
			return fact, mutation, core.NewError(core.CodeNotFound, "only the owner may stow an object they hold")
		}
		var inventory string
		if err := conn.QueryRowContext(ctx, `SELECT m.inventory_location_id FROM materialized_entities m JOIN cohorts c ON c.cohort_id=m.source_cohort_id JOIN agent_profiles a ON a.agent_id=m.entity_id AND a.status='active' JOIN stock_locations l ON l.location_id=m.inventory_location_id AND l.owner_id=m.entity_id AND l.location_kind='holder' WHERE m.entity_id=? AND m.status='active' AND m.population_count=1 AND a.instance_id=? AND a.branch_id=? AND c.instance_id=a.instance_id AND c.branch_id=a.branch_id`, actor, session.InstanceID, session.BranchID).Scan(&inventory); err != nil {
			return fact, mutation, classifyMissing(err, "owner's current ordinary inventory")
		}
		fact.HolderEntityID, fact.AnchorID = "", ""
		fact.FromLocationID, fact.ToLocationID = obj.Escrow, inventory
		fact.Description = "将持有的物品收回普通库存；不再留在场景中。"
		mutation.NewStock, mutation.From, mutation.To = true, fact.FromLocationID, fact.ToLocationID
	case "place", "move":
		if obj.Owner != actor || (r.Action == "place" && obj.State != "held") || (r.Action == "move" && obj.State != "placed") {
			return fact, mutation, core.NewError(core.CodeNotFound, "only the owner may place or reposition the object")
		}
		var anchorPlace, anchorZone, near string
		if err := conn.QueryRowContext(ctx, `SELECT place_id,zone_key,COALESCE(near_entity_id,'') FROM rp_object_anchors WHERE anchor_id=? AND instance_id=? AND branch_id=?`, r.AnchorID, session.InstanceID, session.BranchID).Scan(&anchorPlace, &anchorZone, &near); err != nil {
			return fact, mutation, classifyMissing(err, "authored object reach anchor")
		}
		if anchorPlace != point.PlaceID || anchorZone != point.ZoneKey || r.AnchorID == obj.Anchor {
			return fact, mutation, core.NewError(core.CodeInvalidArgument, "destination anchor is not another reachable point")
		}
		if near != "" {
			nearPoint, err := readRPActorSpatialPoint(ctx, conn, session.InstanceID, session.BranchID, near)
			if err != nil || nearPoint != point {
				return fact, mutation, core.NewError(core.CodeNotFound, "named neighbor is not presently at the anchor")
			}
			visible, err := rpCanPerceive(ctx, conn, session.InstanceID, session.BranchID, actor, near, "visual", "")
			if err != nil || !visible {
				return fact, mutation, core.NewError(core.CodeNotFound, "named neighbor cannot be observed at the anchor")
			}
		}
		fact.AnchorID, fact.HolderEntityID = r.AnchorID, ""
		if r.Action == "place" {
			fact.Description = "把物品放在场景中可触及的位置。"
		} else {
			fact.Description = "把已放下的物品推到了另一处可触及位置。"
		}
	case "offer":
		if obj.Owner != actor || obj.State != "held" || activeOffer != 0 {
			return fact, mutation, core.NewError(core.CodeBranchConflict, "only the holder can offer an unoffered object")
		}
		target := r.TargetEntityID
		if !strings.HasPrefix(target, "person_") {
			known, err := rpIdentityKnown(ctx, conn, session.InstanceID, session.BranchID, actor, target)
			if err != nil || !known {
				return fact, mutation, core.NewError(core.CodeNotFound, "unidentified object recipient")
			}
		}
		target, err = rpResolvePublicEntityID(ctx, conn, session.InstanceID, session.BranchID, actor, target)
		if err != nil {
			return fact, mutation, err
		}
		if target == actor {
			return fact, mutation, core.NewError(core.CodeInvalidArgument, "cannot offer object to self")
		}
		if err := requireRPObjectPeer(ctx, conn, session, point, target); err != nil {
			return fact, mutation, err
		}
		fact.TargetEntityID, fact.OfferID, fact.OfferEventID, fact.OfferStatus = target, "rp_offer_"+strings.TrimPrefix(eventID, "event_rp_object_"), eventID, "offered"
		fact.Description = "向对方提出交接物品的邀请；对方尚未接受。"
	case "accept", "refuse", "give", "receive", "cancel_offer":
		offer := mutation.Offer
		fact.OfferID, fact.OfferEventID, fact.TargetEntityID = offer.ID, offer.OfferEvent, offer.Target
		if offer.ObjectID != obj.ID || offer.From != obj.Owner || obj.State != "held" || obj.Holder != offer.From || activeOffer != 1 {
			return fact, mutation, core.NewError(core.CodeBranchConflict, "offer no longer matches a held, owned object")
		}
		switch r.Action {
		case "accept", "refuse":
			if actor != offer.Target || offer.Status != "offered" {
				return fact, mutation, core.NewError(core.CodeUnauthorized, "only offered actor may answer an unresolved offer")
			}
			if r.Action == "accept" {
				if err := requireRPObjectPeer(ctx, conn, session, point, offer.From); err != nil {
					return fact, mutation, err
				}
				fact.OfferStatus, fact.Description = "accepted_pending_transfer", "对方同意之后交接，但物品仍在原持有人手中。"
			} else {
				fact.OfferStatus, fact.Description = "refused", "对方拒绝交接；物品仍在原持有人手中。"
			}
		case "cancel_offer":
			if actor != offer.From || (offer.Status != "offered" && offer.Status != "accepted_pending_transfer") {
				return fact, mutation, core.NewError(core.CodeUnauthorized, "only offerer may withdraw an active offer")
			}
			fact.OfferStatus, fact.Description = "cancelled", "提出者撤回交接邀请；物品未交出。"
		case "give", "receive":
			if offer.Status != "accepted_pending_transfer" || offer.ResponseEvent == "" || (r.Action == "give" && actor != offer.From) || (r.Action == "receive" && actor != offer.Target) {
				return fact, mutation, core.NewError(core.CodeBranchConflict, "transfer requires the recipient's own prior accepted Event")
			}
			other := offer.From
			if actor == offer.From {
				other = offer.Target
			}
			if err := requireRPObjectPeer(ctx, conn, session, point, other); err != nil {
				return fact, mutation, err
			}
			fact.OfferStatus, fact.OwnerEntityID, fact.HolderEntityID, fact.Description = "transferred", offer.Target, offer.Target, "经对方同意，物品才从原持有人实际交到接收者手中。"
			fact.FromLocationID, fact.ToLocationID = obj.Escrow, rpObjectEscrowID(obj.ID, offer.Target)
			mutation.NewStock, mutation.From, mutation.To = true, fact.FromLocationID, fact.ToLocationID
		}
	}
	return fact, mutation, nil
}

func requireRPObjectPeer(ctx context.Context, conn *sql.Conn, session RPSession, point rpActorSpatialPoint, peer string) error {
	other, err := readRPActorSpatialPoint(ctx, conn, session.InstanceID, session.BranchID, peer)
	if err != nil || other != point {
		return core.NewError(core.CodeNotFound, "object recipient is not at the same reachable point")
	}
	visible, err := rpCanPerceive(ctx, conn, session.InstanceID, session.BranchID, session.ControlledEntityID, peer, "visual", "")
	if err != nil {
		return err
	}
	if !visible {
		return core.NewError(core.CodeNotFound, "object recipient cannot be observed")
	}
	return nil
}

func rpObjectEscrowID(objectID, owner string) string {
	hash, _ := core.HashJSON([]string{objectID, owner})
	return "stock_rp_object_" + hash[7:]
}

func loadRPObject(ctx context.Context, conn *sql.Conn, session RPSession, id string) (rpObjectRow, error) {
	var row rpObjectRow
	var holder, anchor sql.NullString
	var raw string
	err := conn.QueryRowContext(ctx, `SELECT o.object_id,o.source_id,o.sku_id,o.display_name,o.owner_actor_id,o.escrow_location_id,o.physical_state,o.holder_actor_id,o.anchor_id,o.place_id,o.zone_key,o.stage_event_id,o.last_event_id,o.unit_minor,o.last_event_sequence,e.payload FROM rp_objects o JOIN events e ON e.event_id=o.last_event_id AND e.instance_id=o.instance_id AND e.branch_id=o.branch_id AND e.event_type='RPObjectInteracted' WHERE o.object_id=? AND o.instance_id=? AND o.branch_id=?`, id, session.InstanceID, session.BranchID).Scan(&row.ID, &row.SourceID, &row.SKU, &row.Name, &row.Owner, &row.Escrow, &row.State, &holder, &anchor, &row.Place, &row.Zone, &row.StageEvent, &row.LastEvent, &row.UnitMinor, &row.Sequence, &raw)
	if err != nil {
		return row, classifyMissing(err, "scoped committed object")
	}
	row.Holder, row.Anchor = holder.String, anchor.String
	var latest core.RPObjectEvidence
	if err := json.Unmarshal([]byte(raw), &latest); err != nil {
		return row, err
	}
	if latest.ObjectID != row.ID || latest.SKUID != row.SKU || latest.UnitMinor != row.UnitMinor || latest.DisplayName != row.Name || latest.OwnerEntityID != row.Owner || latest.HolderEntityID != row.Holder || latest.AnchorID != row.Anchor || latest.SourceID != row.SourceID || (row.State == "stowed" && (latest.Action != "stow" || row.Holder != "" || row.Anchor != "" || latest.FromLocationID != row.Escrow)) {
		return row, core.NewError(core.CodeProjectionDiverged, "object projection differs from last committed Event")
	}
	var sourceEvent, owner string
	if err := conn.QueryRowContext(ctx, `SELECT definition_event_id,owner_actor_id FROM rp_object_sources WHERE source_id=? AND instance_id=? AND branch_id=? AND sku_id=? AND unit_minor=? AND display_name=?`, row.SourceID, session.InstanceID, session.BranchID, row.SKU, row.UnitMinor, row.Name).Scan(&sourceEvent, &owner); err != nil {
		return row, classifyMissing(err, "object's original declared source")
	}
	if err := conn.QueryRowContext(ctx, `SELECT location_id FROM rp_object_escrows WHERE object_id=? AND owner_actor_id=? AND location_id=?`, row.ID, row.Owner, row.Escrow).Scan(new(string)); err != nil {
		return row, classifyMissing(err, "object owner escrow lineage")
	}
	var stockOwner string
	if err := conn.QueryRowContext(ctx, `SELECT owner_id FROM stock_locations WHERE location_id=? AND location_kind='holder'`, row.Escrow).Scan(&stockOwner); err != nil || stockOwner != row.Owner {
		return row, core.NewError(core.CodeProjectionDiverged, "object escrow is not actor-owned holder stock")
	}
	stock, _, err := readRPStoreStock(ctx, conn, session.InstanceID, session.BranchID, row.Escrow, row.SKU)
	if err != nil {
		return row, err
	}
	wantStock := row.UnitMinor
	if row.State == "stowed" {
		wantStock = 0
	}
	if stock != wantStock {
		return row, core.NewError(core.CodeProjectionDiverged, "object escrow differs from its physical state")
	}
	return row, nil
}

func loadRPObjectOffer(ctx context.Context, conn *sql.Conn, session RPSession, id string) (rpObjectOfferRow, error) {
	var row rpObjectOfferRow
	var response, terminal sql.NullString
	var raw string
	err := conn.QueryRowContext(ctx, `SELECT o.offer_id,o.object_id,o.from_actor_id,o.target_actor_id,o.status,o.offer_event_id,o.response_event_id,o.terminal_event_id,o.last_event_sequence,e.payload FROM rp_object_offers o JOIN rp_objects item ON item.object_id=o.object_id AND item.instance_id=o.instance_id AND item.branch_id=o.branch_id JOIN events e ON e.instance_id=o.instance_id AND e.branch_id=o.branch_id AND e.event_sequence=o.last_event_sequence AND e.event_type='RPObjectInteracted' WHERE o.offer_id=? AND o.instance_id=? AND o.branch_id=?`, id, session.InstanceID, session.BranchID).Scan(&row.ID, &row.ObjectID, &row.From, &row.Target, &row.Status, &row.OfferEvent, &response, &terminal, &row.Sequence, &raw)
	if err != nil {
		return row, classifyMissing(err, "scoped committed object offer")
	}
	row.ResponseEvent, row.TerminalEvent = response.String, terminal.String
	var latest core.RPObjectEvidence
	if err := json.Unmarshal([]byte(raw), &latest); err != nil {
		return row, err
	}
	if latest.OfferID != row.ID || latest.ObjectID != row.ObjectID || latest.OfferEventID != row.OfferEvent || latest.TargetEntityID != row.Target || latest.OfferStatus != row.Status {
		return row, core.NewError(core.CodeProjectionDiverged, "offer state differs from its last committed Event")
	}
	return row, nil
}

func readRPObjectOrdinaryStock(ctx context.Context, conn *sql.Conn, session RPSession, location, sku string) (int64, bool, error) {
	var actual int64
	var movements int
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE WHEN m.to_location_id=? AND m.movement_kind IN ('transfer','create') THEN m.quantity_minor ELSE 0 END - CASE WHEN m.from_location_id=? AND m.movement_kind IN ('transfer','consume','destroy') THEN m.quantity_minor ELSE 0 END),0),COUNT(*) FROM stock_movements m JOIN events e ON e.event_id=m.event_id WHERE e.instance_id=? AND e.branch_id=? AND m.sku_id=? AND (m.to_location_id=? OR m.from_location_id=?)`, location, location, session.InstanceID, session.BranchID, sku, location, location).Scan(&actual, &movements); err != nil {
		return 0, false, err
	}
	var projected int64
	err := conn.QueryRowContext(ctx, `SELECT quantity_minor FROM inventory_balances WHERE location_id=? AND sku_id=?`, location, sku).Scan(&projected)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	if errors.Is(err, sql.ErrNoRows) && movements == 0 {
		return 0, false, nil
	}
	if err != nil || actual < 0 || projected != actual {
		return 0, false, core.NewError(core.CodeProjectionDiverged, "ordinary inventory differs from committed stock movements")
	}
	return projected, true, nil
}

func applyRPObjectAction(ctx context.Context, conn *sql.Conn, session RPSession, fact core.RPObjectEvidence, m rpObjectMutation, eventID string, sequence int64) error {
	if m.NewStock {
		if fact.Action == "stage" {
			if err := execAgentOne(ctx, conn, "create object escrow", `INSERT INTO stock_locations(location_id,owner_id,location_kind) VALUES (?,?,'holder')`, m.To, fact.OwnerEntityID); err != nil {
				return err
			}
			if err := execAgentOne(ctx, conn, "serialize SKU unit", `INSERT INTO inventory_balances(location_id,sku_id,quantity_minor,projection_version,last_event_sequence) VALUES (?,?,?,0,?)`, m.To, fact.SKUID, fact.UnitMinor, sequence); err != nil {
				return err
			}
		} else if fact.Action == "stow" {
			balance, exists, err := readRPObjectOrdinaryStock(ctx, conn, session, m.To, fact.SKUID)
			if err != nil {
				return err
			}
			credited, ok := checkedAdd(balance, fact.UnitMinor)
			if !ok {
				return core.NewError(core.CodeIntegerOverflow, "ordinary inventory would overflow")
			}
			if exists {
				if err := execAgentOne(ctx, conn, "return physical unit to ordinary inventory", `UPDATE inventory_balances SET quantity_minor=?,projection_version=projection_version+1,last_event_sequence=? WHERE location_id=? AND sku_id=? AND quantity_minor=?`, credited, sequence, m.To, fact.SKUID, balance); err != nil {
					return err
				}
			} else if err := execAgentOne(ctx, conn, "open ordinary SKU balance for returned item", `INSERT INTO inventory_balances(location_id,sku_id,quantity_minor,projection_version,last_event_sequence) VALUES (?,?,?,0,?)`, m.To, fact.SKUID, credited, sequence); err != nil {
				return err
			}
		} else {
			var present int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM rp_object_escrows WHERE object_id=? AND owner_actor_id=? AND location_id=?`, fact.ObjectID, fact.OwnerEntityID, m.To).Scan(&present); err != nil {
				return err
			}
			if present == 0 {
				if err := execAgentOne(ctx, conn, "create recipient escrow", `INSERT INTO stock_locations(location_id,owner_id,location_kind) VALUES (?,?,'holder')`, m.To, fact.OwnerEntityID); err != nil {
					return err
				}
				if err := execAgentOne(ctx, conn, "record recipient escrow", `INSERT INTO rp_object_escrows(object_id,owner_actor_id,location_id) VALUES (?,?,?)`, fact.ObjectID, fact.OwnerEntityID, m.To); err != nil {
					return err
				}
				if err := execAgentOne(ctx, conn, "credit recipient escrow", `INSERT INTO inventory_balances(location_id,sku_id,quantity_minor,projection_version,last_event_sequence) VALUES (?,?,?,0,?)`, m.To, fact.SKUID, fact.UnitMinor, sequence); err != nil {
					return err
				}
			} else {
				available, _, err := readRPStoreStock(ctx, conn, session.InstanceID, session.BranchID, m.To, fact.SKUID)
				if err != nil {
					return err
				}
				if available != 0 {
					return core.NewError(core.CodeProjectionDiverged, "recipient item escrow already contains stock")
				}
				if err := execAgentOne(ctx, conn, "refill recipient escrow", `UPDATE inventory_balances SET quantity_minor=?,projection_version=projection_version+1,last_event_sequence=? WHERE location_id=? AND sku_id=? AND quantity_minor=0`, fact.UnitMinor, sequence, m.To, fact.SKUID); err != nil {
					return err
				}
			}
		}
		if err := execAgentOne(ctx, conn, "transfer precisely one physical SKU unit", `INSERT INTO stock_movements(movement_id,event_id,sku_id,from_location_id,to_location_id,quantity_minor,movement_kind,reason_code) VALUES (?,?,?,?,?,?,'transfer',?)`, "movement_"+eventID, eventID, fact.SKUID, m.From, m.To, fact.UnitMinor, "rp_object_"+fact.Action); err != nil {
			return err
		}
		if err := execAgentOne(ctx, conn, "debit original stock", `UPDATE inventory_balances SET quantity_minor=quantity_minor-?,projection_version=projection_version+1,last_event_sequence=? WHERE location_id=? AND sku_id=? AND quantity_minor>=?`, fact.UnitMinor, sequence, m.From, fact.SKUID, fact.UnitMinor); err != nil {
			return err
		}
	}
	if fact.Action == "stage" {
		if err := execAgentOne(ctx, conn, "stage stock-backed single item", `INSERT INTO rp_objects(object_id,instance_id,branch_id,source_id,sku_id,unit_minor,display_name,owner_actor_id,escrow_location_id,physical_state,holder_actor_id,place_id,zone_key,stage_event_id,last_event_id,last_event_sequence) VALUES (?,?,?,?,?,?,?,?,?,'held',?,?,?,?,?,?)`, fact.ObjectID, session.InstanceID, session.BranchID, fact.SourceID, fact.SKUID, fact.UnitMinor, fact.DisplayName, fact.OwnerEntityID, m.To, fact.HolderEntityID, fact.PlaceID, fact.ZoneKey, eventID, eventID, sequence); err != nil {
			return err
		}
		return execAgentOne(ctx, conn, "link staged item escrow", `INSERT INTO rp_object_escrows(object_id,owner_actor_id,location_id) VALUES (?,?,?)`, fact.ObjectID, fact.OwnerEntityID, m.To)
	}
	state := "held"
	if fact.Action == "stow" {
		state = "stowed"
	} else if fact.AnchorID != "" {
		state = "placed"
	}
	newEscrow := m.Object.Escrow
	if fact.Action == "give" || fact.Action == "receive" {
		newEscrow = m.To
	}
	if err := execAgentOne(ctx, conn, "project object Event", `UPDATE rp_objects SET owner_actor_id=?,escrow_location_id=?,physical_state=?,holder_actor_id=?,anchor_id=?,place_id=?,zone_key=?,last_event_id=?,last_event_sequence=? WHERE object_id=? AND instance_id=? AND branch_id=? AND last_event_id=?`, fact.OwnerEntityID, newEscrow, state, nullRPObjectString(fact.HolderEntityID), nullRPObjectString(fact.AnchorID), fact.PlaceID, fact.ZoneKey, eventID, sequence, fact.ObjectID, session.InstanceID, session.BranchID, fact.PreviousEvent); err != nil {
		return err
	}
	if fact.Action == "offer" {
		return execAgentOne(ctx, conn, "record distinct offer Event", `INSERT INTO rp_object_offers(offer_id,instance_id,branch_id,object_id,from_actor_id,target_actor_id,status,offer_event_id,last_event_sequence) VALUES (?,?,?,?,?,?,'offered',?,?)`, fact.OfferID, session.InstanceID, session.BranchID, fact.ObjectID, fact.ActorEntityID, fact.TargetEntityID, eventID, sequence)
	}
	if fact.OfferID != "" {
		var response, terminal any
		response, terminal = nullRPObjectString(m.Offer.ResponseEvent), nullRPObjectString(m.Offer.TerminalEvent)
		if fact.Action == "accept" || fact.Action == "refuse" {
			response = eventID
		}
		if fact.Action == "refuse" || fact.Action == "cancel_offer" || fact.Action == "give" || fact.Action == "receive" {
			terminal = eventID
		}
		return execAgentOne(ctx, conn, "project distinct offer response", `UPDATE rp_object_offers SET status=?,response_event_id=?,terminal_event_id=?,last_event_sequence=? WHERE offer_id=? AND instance_id=? AND branch_id=? AND status=? AND last_event_sequence=?`, fact.OfferStatus, response, terminal, sequence, fact.OfferID, session.InstanceID, session.BranchID, m.Offer.Status, m.Offer.Sequence)
	}
	return nil
}
