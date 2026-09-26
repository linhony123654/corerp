package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

type rpInformationKnowledgeRow struct {
	SubjectID, PlaceID, DeliveryEventID, ObservationID, LearnedWorldTime, ClaimKey, ClaimJSON string
	LastSequence                                                                              int64
}

// Read only the actor's delivered claims. A mutable knowledge row is never
// sufficient to put content into an external decision: both private Events
// and the exact observation/knowledge join must substantiate it first.
func readRPOwnInformation(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput) ([]core.RPInformationMemory, error) {
	rows, err := conn.QueryContext(ctx, `SELECT k.subject_agent_id,k.place_id,k.source_event_id,k.observation_id,k.learned_world_time,k.claim_key,k.claim_payload,k.last_event_sequence
 FROM agent_knowledge k LEFT JOIN events e ON e.event_id=k.source_event_id
 LEFT JOIN observation_records o ON o.observation_id=k.observation_id
 WHERE k.observer_agent_id=? AND (k.claim_key LIKE 'information:%' OR e.event_type='RPInformationDelivered' OR o.channel IN ('direct_message','rumor','organization_announcement','public_notice'))
 ORDER BY k.last_event_sequence DESC,k.claim_key LIMIT 8`, input.NPCEntityID)
	if err != nil {
		return nil, err
	}
	claims := []rpInformationKnowledgeRow{}
	for rows.Next() {
		var row rpInformationKnowledgeRow
		if err := rows.Scan(&row.SubjectID, &row.PlaceID, &row.DeliveryEventID, &row.ObservationID,
			&row.LearnedWorldTime, &row.ClaimKey, &row.ClaimJSON, &row.LastSequence); err != nil {
			rows.Close()
			return nil, err
		}
		claims = append(claims, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(claims) == 0 {
		return []core.RPInformationMemory{}, nil
	}
	_, expectedObservations, err := rpInformationExpected(ctx, conn, input.InstanceID, input.BranchID, input.HeadSequence)
	if err != nil {
		return nil, err
	}

	memories := make([]core.RPInformationMemory, 0, len(claims))
	for _, claim := range claims {
		var delivered RPInformationFact
		var deliveryRaw, deliveryWorldTime string
		var deliverySequence int64
		err := conn.QueryRowContext(ctx, `SELECT event_sequence,world_time,payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPInformationDelivered'`,
			claim.DeliveryEventID, input.InstanceID, input.BranchID).Scan(&deliverySequence, &deliveryWorldTime, &deliveryRaw)
		if err != nil {
			return nil, core.NewError(core.CodeProjectionDiverged, "information knowledge lacks delivery source")
		}
		if err := json.Unmarshal([]byte(deliveryRaw), &delivered); err != nil {
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid information delivery source")
		}
		var sent RPInformationFact
		var sendRaw, sendWorldTime string
		var sendSequence int64
		err = conn.QueryRowContext(ctx, `SELECT event_sequence,world_time,payload FROM events WHERE event_id=? AND instance_id=? AND branch_id=? AND event_type='RPInformationSent'`,
			delivered.SourceEventID, input.InstanceID, input.BranchID).Scan(&sendSequence, &sendWorldTime, &sendRaw)
		if err != nil {
			return nil, core.NewError(core.CodeProjectionDiverged, "information delivery lacks send source")
		}
		if err := json.Unmarshal([]byte(sendRaw), &sent); err != nil {
			return nil, core.NewError(core.CodeProjectionDiverged, "invalid information send source")
		}
		want := sent
		want.SourceEventID = delivered.SourceEventID
		want.RecipientPlaceID = delivered.RecipientPlaceID
		isOrganization := sent.Channel == "organization_announcement"
		isPublic := sent.Channel == "public_notice"
		isPublication := isOrganization || isPublic
		if isPublication {
			want.RecipientID = delivered.RecipientID
			want.DeliverWorldTime = deliveryWorldTime
			if isOrganization {
				want.AudienceContractID = delivered.AudienceContractID
				want.AudienceTermsEventID = delivered.AudienceTermsEventID
			}
		}
		sentAt, sentErr := time.Parse(time.RFC3339Nano, sendWorldTime)
		deliveredAt, deliveredErr := time.Parse(time.RFC3339Nano, deliveryWorldTime)
		worldAt, worldErr := time.Parse(time.RFC3339Nano, input.WorldTime)
		if sentErr != nil || deliveredErr != nil || worldErr != nil || deliveredAt.Before(sentAt) ||
			(!isPublication && !deliveredAt.After(sentAt)) || deliveredAt.After(worldAt) || sendSequence >= deliverySequence ||
			sent.Version != "corerp.information.v1" || !validRPInformationChannel(sent) ||
			(isOrganization && (sent.Visibility != "organization" || sent.ClaimedReliability != "official_statement" || sent.RecipientID != "")) ||
			(isPublic && (sent.Visibility != "public" || sent.ClaimedReliability != "official_statement" || sent.RecipientID != "")) ||
			(!isPublication && (sent.Visibility != "private" || sent.ClaimedReliability != "unverified" || sent.RecipientID == "" || deliveryWorldTime != sent.DeliverWorldTime)) ||
			sent.SourceEventID != "" || sent.RecipientPlaceID != "" || sent.MessageID == "" || sent.SenderID == "" ||
			strings.TrimSpace(sent.Text) == "" || len(sent.Text) > 2000 || sent.SenderID == delivered.RecipientID ||
			delivered.RecipientPlaceID == "" || delivered != want ||
			claim.LastSequence != deliverySequence || claim.LearnedWorldTime != deliveryWorldTime ||
			claim.SubjectID != sent.SenderID || claim.PlaceID != delivered.RecipientPlaceID ||
			input.NPCEntityID != delivered.RecipientID || claim.ClaimKey != "information:"+delivered.SourceEventID ||
			claim.ObservationID != "observation_"+claim.DeliveryEventID+"_"+delivered.RecipientID {
			return nil, core.NewError(core.CodeProjectionDiverged, "information knowledge differs from delivery source")
		}
		wantClaim, err := rpInformationClaimJSON(sent)
		if err != nil {
			return nil, err
		}
		if claim.ClaimJSON != wantClaim {
			return nil, core.NewError(core.CodeProjectionDiverged, "information knowledge claim differs from send source")
		}
		var observation rpInformationObservation
		err = conn.QueryRowContext(ctx, `SELECT observation_id,source_event_id,observer_agent_id,subject_agent_id,place_id,channel,observed_world_time,claim_key,claim_payload FROM observation_records WHERE observation_id=?`,
			claim.ObservationID).Scan(&observation.ID, &observation.SourceEventID, &observation.ObserverID,
			&observation.SubjectID, &observation.PlaceID, &observation.Channel, &observation.WorldTime,
			&observation.ClaimKey, &observation.ClaimJSON)
		if err != nil || observation != (rpInformationObservation{claim.ObservationID, claim.DeliveryEventID,
			delivered.RecipientID, sent.SenderID, delivered.RecipientPlaceID, sent.Channel, deliveryWorldTime,
			claim.ClaimKey, wantClaim}) || expectedObservations[claim.ObservationID] != observation {
			return nil, core.NewError(core.CodeProjectionDiverged, "information observation differs from delivery source")
		}
		handle, err := rpAnonymousEntityID(ctx, conn, input.InstanceID, input.BranchID, input.NPCEntityID, sent.SenderID)
		if err != nil {
			return nil, err
		}
		memory := core.RPInformationMemory{MessageID: sent.MessageID, SenderHandle: handle, Text: sent.Text, Channel: sent.Channel,
			ClaimedReliability: sent.ClaimedReliability, LearnedWorldTime: deliveryWorldTime,
			MayRelay: sent.AllowRelay, Forwarded: sent.ForwardedFromSendEventID != ""}
		memory.Stance, _, err = rpInformationCurrentStance(ctx, conn, input.InstanceID, input.BranchID,
			input.NPCEntityID, sent.MessageID, delivered.SourceEventID, claim.DeliveryEventID)
		if err != nil {
			return nil, err
		}
		known, err := rpIdentityKnown(ctx, conn, input.InstanceID, input.BranchID, input.NPCEntityID, sent.SenderID)
		if err != nil {
			return nil, err
		}
		if known {
			if err := conn.QueryRowContext(ctx, `SELECT display_name FROM materialized_entities WHERE entity_id=? AND status='active'`, sent.SenderID).Scan(&memory.SenderName); err != nil {
				return nil, core.NewError(core.CodeProjectionDiverged, "known message sender lacks identity source")
			}
		}
		memories = append(memories, memory)
	}
	return memories, nil
}
