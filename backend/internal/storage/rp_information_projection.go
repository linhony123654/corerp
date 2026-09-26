package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"corerp.local/backend/internal/core"
)

type rpInformationObservation struct {
	ID, SourceEventID, ObserverID, SubjectID, PlaceID, Channel, WorldTime, ClaimKey, ClaimJSON string
}

type rpInformationSource struct {
	ID, ActorID, WorldTime string
	Sequence               int64
	Fact                   RPInformationFact
}

type rpOrganizationDeliveryKey struct {
	SendID, RecipientID string
}

func validRPInformationChannel(fact RPInformationFact) bool {
	if fact.Channel == "direct_message" {
		return fact.ForwardedFromSendEventID == "" && fact.ForwardedFromDeliveryEventID == "" && fact.OrganizationID == "" && fact.SourceCareerEventID == "" && fact.InstitutionID == "" && fact.SourceLawEventID == "" && fact.AudienceContractID == "" && fact.AudienceTermsEventID == ""
	}
	if fact.Channel == "rumor" {
		return !fact.AllowRelay && fact.ForwardedFromSendEventID != "" && fact.ForwardedFromDeliveryEventID != "" && fact.OrganizationID == "" && fact.SourceCareerEventID == "" && fact.InstitutionID == "" && fact.SourceLawEventID == "" && fact.AudienceContractID == "" && fact.AudienceTermsEventID == ""
	}
	if fact.Channel == "organization_announcement" {
		return !fact.AllowRelay && fact.ForwardedFromSendEventID == "" && fact.ForwardedFromDeliveryEventID == "" &&
			fact.OrganizationID != "" && fact.SourceCareerEventID != "" && fact.InstitutionID == "" && fact.SourceLawEventID == ""
	}
	if fact.Channel == "public_notice" {
		return !fact.AllowRelay && fact.ForwardedFromSendEventID == "" && fact.ForwardedFromDeliveryEventID == "" &&
			fact.OrganizationID == "" && fact.SourceCareerEventID == "" && fact.InstitutionID != "" && fact.SourceLawEventID != "" &&
			fact.AudienceContractID == "" && fact.AudienceTermsEventID == ""
	}
	return false
}

// F7 delivery observations are derived from the two immutable Events, not
// treated as a second authority merely because generic Agent replay reads
// observation_records. This catches missing or jointly corrupted observation
// and knowledge rows before generic reconstruction.
func rpInformationExpected(ctx context.Context, q replayQuerier, instanceID, branchID string, head int64) (map[string]SchedulerItem, map[string]rpInformationObservation, error) {
	sends := map[string]rpInformationSource{}
	deliveries := map[string]rpInformationSource{}
	organizationDeliveries := map[rpOrganizationDeliveryKey]rpInformationSource{}
	messageIDs := map[string]bool{}
	rows, err := q.QueryContext(ctx, `SELECT event_id,event_sequence,event_type,actor_id,world_time,payload FROM events
	 WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type IN ('RPInformationSent','RPInformationDelivered') ORDER BY event_sequence`, instanceID, branchID, head)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var source rpInformationSource
		var kind, raw string
		if err := rows.Scan(&source.ID, &source.Sequence, &kind, &source.ActorID, &source.WorldTime, &raw); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if err := json.Unmarshal([]byte(raw), &source.Fact); err != nil {
			rows.Close()
			return nil, nil, core.NewError(core.CodeProjectionDiverged, "invalid information Event payload")
		}
		if kind == "RPInformationSent" {
			fact := source.Fact
			at, parseErr := time.Parse(time.RFC3339Nano, source.WorldTime)
			due, dueErr := time.Parse(time.RFC3339Nano, fact.DeliverWorldTime)
			isOrganization := fact.Channel == "organization_announcement"
			isPublic := fact.Channel == "public_notice"
			isPublication := isOrganization || isPublic
			if parseErr != nil || fact.Version != "corerp.information.v1" || !validRPInformationChannel(fact) ||
				fact.SourceEventID != "" || fact.RecipientPlaceID != "" || fact.MessageID == "" || fact.SenderID == "" ||
				strings.TrimSpace(fact.Text) == "" || len(fact.Text) > 2000 || messageIDs[fact.MessageID] ||
				(isOrganization && (fact.RecipientID != "" || fact.DeliverWorldTime != "" || fact.Visibility != "organization" || fact.ClaimedReliability != "official_statement" || fact.AudienceContractID != "" || fact.AudienceTermsEventID != "")) ||
				(isPublic && (fact.RecipientID != "" || fact.DeliverWorldTime != "" || fact.Visibility != "public" || fact.ClaimedReliability != "official_statement")) ||
				(!isPublication && (dueErr != nil || !due.After(at) || fact.RecipientID == "" || fact.SenderID == fact.RecipientID || fact.Visibility != "private" || fact.ClaimedReliability != "unverified")) {
				rows.Close()
				return nil, nil, core.NewError(core.CodeProjectionDiverged, "information send source differs")
			}
			messageIDs[fact.MessageID] = true
			sends[source.ID] = source
		} else {
			if source.Fact.SourceEventID == "" {
				rows.Close()
				return nil, nil, core.NewError(core.CodeProjectionDiverged, "unlinked information delivery")
			}
			if source.Fact.Channel == "organization_announcement" || source.Fact.Channel == "public_notice" {
				key := rpOrganizationDeliveryKey{source.Fact.SourceEventID, source.Fact.RecipientID}
				if key.RecipientID == "" || organizationDeliveries[key].ID != "" {
					rows.Close()
					return nil, nil, core.NewError(core.CodeProjectionDiverged, "duplicate organization notice recipient")
				}
				organizationDeliveries[key] = source
			} else {
				if deliveries[source.Fact.SourceEventID].ID != "" {
					rows.Close()
					return nil, nil, core.NewError(core.CodeProjectionDiverged, "duplicate direct information delivery")
				}
				deliveries[source.Fact.SourceEventID] = source
			}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()
	for _, sent := range sends {
		if sent.Fact.Channel != "organization_announcement" {
			continue
		}
		career, actor, sourceSequence, sourceWorldTime, noticeText, err := rpOrganizationNoticeSource(ctx, q, instanceID, branchID, sent.Fact.SourceCareerEventID)
		if err != nil {
			return nil, nil, core.NewError(core.CodeProjectionDiverged, "organization notice lacks Career source")
		}
		sourceAt, sourceErr := time.Parse(time.RFC3339Nano, sourceWorldTime)
		publishAt, publishErr := time.Parse(time.RFC3339Nano, sent.WorldTime)
		var speakerPrincipal string
		speakerErr := q.QueryRowContext(ctx, `SELECT principal_id FROM agent_profiles WHERE agent_id=? AND instance_id=? AND branch_id=?`,
			sent.Fact.SenderID, instanceID, branchID).Scan(&speakerPrincipal)
		if career.OrganizationID != sent.Fact.OrganizationID || actor != sent.ActorID ||
			speakerErr != nil || speakerPrincipal != actor ||
			sourceSequence >= sent.Sequence || sourceErr != nil || publishErr != nil || publishAt.Before(sourceAt) ||
			sent.Fact.Text != noticeText {
			return nil, nil, core.NewError(core.CodeProjectionDiverged, "organization notice lacks Career source")
		}
	}
	for _, sent := range sends {
		if sent.Fact.Channel != "public_notice" {
			continue
		}
		law, actor, sourceSequence, sourceWorldTime, err := rpPublicLawSource(ctx, q, instanceID, branchID, sent.Fact.SourceLawEventID)
		if err != nil {
			return nil, nil, core.NewError(core.CodeProjectionDiverged, "public notice lacks law source")
		}
		sourceAt, sourceErr := time.Parse(time.RFC3339Nano, sourceWorldTime)
		publishAt, publishErr := time.Parse(time.RFC3339Nano, sent.WorldTime)
		if law.InstitutionID != sent.Fact.InstitutionID || law.Enactment.LegislatorID != sent.Fact.SenderID ||
			actor != sent.ActorID || sourceSequence >= sent.Sequence || sourceErr != nil || publishErr != nil ||
			publishAt.Before(sourceAt) || sent.Fact.Text != rpPublicLawText(law) {
			return nil, nil, core.NewError(core.CodeProjectionDiverged, "public notice differs from law source")
		}
	}
	for _, sent := range sends {
		if sent.Fact.Channel != "rumor" {
			continue
		}
		parent, exists := sends[sent.Fact.ForwardedFromSendEventID]
		learned, delivered := deliveries[sent.Fact.ForwardedFromSendEventID]
		parentAt, parentTimeErr := time.Parse(time.RFC3339Nano, learned.WorldTime)
		relayAt, relayTimeErr := time.Parse(time.RFC3339Nano, sent.WorldTime)
		if !exists || !delivered || parent.Fact.Channel != "direct_message" || !parent.Fact.AllowRelay ||
			parent.Fact.RecipientID != sent.Fact.SenderID || learned.ID != sent.Fact.ForwardedFromDeliveryEventID ||
			parent.Fact.Text != sent.Fact.Text || parent.Fact.ClaimedReliability != sent.Fact.ClaimedReliability ||
			parent.Sequence >= learned.Sequence || learned.Sequence >= sent.Sequence ||
			parentTimeErr != nil || relayTimeErr != nil || relayAt.Before(parentAt) {
			return nil, nil, core.NewError(core.CodeProjectionDiverged, "rumor forwarding source chain differs")
		}
	}
	queues := map[string]SchedulerItem{}
	observations := map[string]rpInformationObservation{}
	for id, sent := range sends {
		if sent.Fact.Channel == "organization_announcement" || sent.Fact.Channel == "public_notice" {
			continue
		}
		item, _, err := rpInformationDeliveryItem(id, sent.Fact.DeliverWorldTime)
		if err != nil {
			return nil, nil, err
		}
		if delivered, ok := deliveries[id]; ok {
			want := sent.Fact
			want.SourceEventID = id
			want.RecipientPlaceID = delivered.Fact.RecipientPlaceID
			if delivered.Sequence <= sent.Sequence || delivered.WorldTime != item.WorldTime ||
				want != delivered.Fact || want.RecipientPlaceID == "" {
				return nil, nil, core.NewError(core.CodeProjectionDiverged, "information delivery lineage differs")
			}
			var placeCount int
			if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_places WHERE place_id=? AND instance_id=? AND branch_id=?`, want.RecipientPlaceID, instanceID, branchID).Scan(&placeCount); err != nil {
				return nil, nil, err
			}
			if placeCount != 1 {
				return nil, nil, core.NewError(core.CodeProjectionDiverged, "information recipient place lacks source scope")
			}
			claimJSON, err := rpInformationClaimJSON(sent.Fact)
			if err != nil {
				return nil, nil, err
			}
			observationID := "observation_" + delivered.ID + "_" + want.RecipientID
			observations[observationID] = rpInformationObservation{observationID, delivered.ID, want.RecipientID,
				want.SenderID, want.RecipientPlaceID, want.Channel, delivered.WorldTime, "information:" + id, claimJSON}
			item.Status = "completed"
		}
		queues[item.SchedulerItemID] = item
	}
	for id := range deliveries {
		if sent, ok := sends[id]; !ok || sent.Fact.Channel == "organization_announcement" || sent.Fact.Channel == "public_notice" {
			return nil, nil, core.NewError(core.CodeProjectionDiverged, "information delivery has no send source")
		}
	}
	for key, delivered := range organizationDeliveries {
		sent, exists := sends[key.SendID]
		if !exists || (sent.Fact.Channel != "organization_announcement" && sent.Fact.Channel != "public_notice") || key.RecipientID == sent.Fact.SenderID {
			return nil, nil, core.NewError(core.CodeProjectionDiverged, "organization delivery lacks publication source")
		}
		want := sent.Fact
		want.SourceEventID = key.SendID
		want.RecipientID = key.RecipientID
		want.RecipientPlaceID = delivered.Fact.RecipientPlaceID
		want.DeliverWorldTime = delivered.WorldTime
		if sent.Fact.Channel == "organization_announcement" {
			want.AudienceContractID = delivered.Fact.AudienceContractID
			want.AudienceTermsEventID = delivered.Fact.AudienceTermsEventID
		}
		at, atErr := time.Parse(time.RFC3339Nano, delivered.WorldTime)
		publishedAt, publishedErr := time.Parse(time.RFC3339Nano, sent.WorldTime)
		if delivered.Sequence <= sent.Sequence || atErr != nil || publishedErr != nil || at.Before(publishedAt) ||
			want != delivered.Fact || want.RecipientPlaceID == "" {
			return nil, nil, core.NewError(core.CodeProjectionDiverged, "organization delivery lineage differs")
		}
		var placeCount int
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_places WHERE place_id=? AND instance_id=? AND branch_id=?`, want.RecipientPlaceID, instanceID, branchID).Scan(&placeCount); err != nil {
			return nil, nil, err
		}
		if placeCount != 1 {
			return nil, nil, core.NewError(core.CodeProjectionDiverged, "organization recipient place lacks source scope")
		}
		if sent.Fact.Channel == "organization_announcement" {
			if err := validateRPOrganizationDeliveryAudience(ctx, q, instanceID, branchID, sent.Fact,
				key.RecipientID, delivered); err != nil {
				return nil, nil, err
			}
		}
		claimJSON, err := rpInformationClaimJSON(sent.Fact)
		if err != nil {
			return nil, nil, err
		}
		observationID := "observation_" + delivered.ID + "_" + key.RecipientID
		observations[observationID] = rpInformationObservation{observationID, delivered.ID, key.RecipientID,
			want.SenderID, want.RecipientPlaceID, want.Channel, delivered.WorldTime, "information:" + key.SendID, claimJSON}
	}
	if err := validateRPInformationStanceSources(ctx, q, instanceID, branchID, head, sends, deliveries, organizationDeliveries); err != nil {
		return nil, nil, err
	}
	return queues, observations, nil
}

// The access Event pins the employment terms that were effective at access
// time. Historical comparison must not ask today's mutable contract status:
// the employee may lawfully leave after learning the notice.
func validateRPOrganizationDeliveryAudience(ctx context.Context, q replayQuerier, instanceID, branchID string,
	sent RPInformationFact, recipientID string, delivered rpInformationSource) error {
	contractID, termsID := delivered.Fact.AudienceContractID, delivered.Fact.AudienceTermsEventID
	if contractID == "" || termsID == "" {
		return core.NewError(core.CodeProjectionDiverged, "organization recipient lacks employment evidence")
	}
	at, err := time.Parse(time.RFC3339Nano, delivered.WorldTime)
	if err != nil {
		return core.NewError(core.CodeProjectionDiverged, "invalid organization delivery time")
	}
	base := time.Date(2026, time.September, 22, 0, 0, 0, 0, time.UTC)
	day := int(at.Sub(base) / (24 * time.Hour))
	var latestID, raw string
	var termsSequence int64
	err = q.QueryRowContext(ctx, `SELECT event_id,event_sequence,payload FROM events WHERE instance_id=? AND branch_id=?
	 AND event_type='RPCareerFactRecorded' AND event_sequence<? AND json_extract(payload,'$.employment.contract_id')=?
	 AND CAST(json_extract(payload,'$.employment.effective_from_day') AS INTEGER)<=?
	 ORDER BY event_sequence DESC LIMIT 1`, instanceID, branchID, delivered.Sequence, contractID, day).Scan(&latestID, &termsSequence, &raw)
	if err != nil || latestID != termsID || termsSequence >= delivered.Sequence {
		return core.NewError(core.CodeProjectionDiverged, "organization recipient employment terms differ")
	}
	var fact CareerFact
	if err := json.Unmarshal([]byte(raw), &fact); err != nil || fact.Employment == nil ||
		fact.Employment.ContractID != contractID || fact.Employment.EmployeeID != recipientID ||
		fact.Employment.OrganizationID != sent.OrganizationID || fact.Employment.StartsOnDay > day ||
		fact.Employment.LifecycleStatus == "ended" || (fact.Employment.EndsOnDay != 0 && fact.Employment.EndsOnDay <= day) {
		return core.NewError(core.CodeProjectionDiverged, "organization recipient was not employed at access")
	}
	var ended int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE instance_id=? AND branch_id=? AND event_type='CareerEmploymentTermsActivated'
	 AND event_sequence<? AND json_extract(payload,'$.contract_id')=? AND json_extract(payload,'$.contract_status')='ended'
	 AND CAST(json_extract(payload,'$.effective_from_day') AS INTEGER)<=?`, instanceID, branchID, delivered.Sequence, contractID, day).Scan(&ended); err != nil {
		return err
	}
	if ended != 0 {
		return core.NewError(core.CodeProjectionDiverged, "organization recipient employment already ended")
	}
	return nil
}

// Stance has no mutable projection to rebuild. Audit its immutable correction
// chain alongside transmission source checks so Compare/Rebuild never treat a
// dangling or reassigned belief Event as valid history.
func validateRPInformationStanceSources(ctx context.Context, q replayQuerier, instanceID, branchID string, head int64, sends, deliveries map[string]rpInformationSource, organizationDeliveries map[rpOrganizationDeliveryKey]rpInformationSource) error {
	rows, err := q.QueryContext(ctx, `SELECT event_id,event_sequence,world_time,payload FROM events WHERE instance_id=? AND branch_id=? AND event_sequence<=? AND event_type='RPInformationStanceRecorded' ORDER BY event_sequence`, instanceID, branchID, head)
	if err != nil {
		return err
	}
	defer rows.Close()
	lastByClaim := map[string]string{}
	for rows.Next() {
		var id, worldTime, raw string
		var sequence int64
		if err := rows.Scan(&id, &sequence, &worldTime, &raw); err != nil {
			return err
		}
		var fact RPInformationStanceFact
		if err := json.Unmarshal([]byte(raw), &fact); err != nil {
			return core.NewError(core.CodeProjectionDiverged, "invalid information stance Event")
		}
		sent, sendOK := sends[fact.SourceSendEventID]
		delivered, deliveryOK := deliveries[fact.SourceSendEventID]
		if sendOK && (sent.Fact.Channel == "organization_announcement" || sent.Fact.Channel == "public_notice") {
			delivered, deliveryOK = organizationDeliveries[rpOrganizationDeliveryKey{fact.SourceSendEventID, fact.ObserverID}]
		}
		stanceAt, stanceTimeErr := time.Parse(time.RFC3339Nano, worldTime)
		deliveryAt, deliveryTimeErr := time.Parse(time.RFC3339Nano, delivered.WorldTime)
		claimKey := fact.SourceSendEventID + ":" + fact.ObserverID
		if !sendOK || !deliveryOK || fact.Version != "corerp.information.stance.v1" ||
			fact.SourceDeliveryEventID != delivered.ID || fact.MessageID != sent.Fact.MessageID ||
			(sent.Fact.Channel != "organization_announcement" && sent.Fact.Channel != "public_notice" && fact.ObserverID != sent.Fact.RecipientID) ||
			fact.PreviousStanceEventID != lastByClaim[claimKey] ||
			!validRPInformationStance(fact.Stance) || len(fact.Reason) > 500 ||
			sequence <= delivered.Sequence || stanceTimeErr != nil || deliveryTimeErr != nil || stanceAt.Before(deliveryAt) {
			return core.NewError(core.CodeProjectionDiverged, "information stance source chain differs")
		}
		lastByClaim[claimKey] = id
	}
	return rows.Err()
}

func rpInformationProjectionDifferences(ctx context.Context, q replayQuerier, instanceID, branchID string, head int64) ([]ProjectionDifference, error) {
	wantQueues, wantObservations, err := rpInformationExpected(ctx, q, instanceID, branchID, head)
	if err != nil {
		return nil, err
	}
	differences := []ProjectionDifference{}
	rows, err := q.QueryContext(ctx, `SELECT scheduler_item_id,world_time,phase_id,declared_priority,status,payload FROM scheduler_items WHERE instance_id=? AND branch_id=? AND phase_id=?`, instanceID, branchID, rpInformationPhase)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var got SchedulerItem
		if err := rows.Scan(&got.SchedulerItemID, &got.WorldTime, &got.PhaseID, &got.DeclaredPriority, &got.Status, &got.Payload); err != nil {
			rows.Close()
			return nil, err
		}
		want, ok := wantQueues[got.SchedulerItemID]
		if !ok || want != got {
			differences = append(differences, ProjectionDifference{Projection: "rp_information_queue", Key: got.SchedulerItemID, ExpectedText: fmt.Sprint(want), ActualText: fmt.Sprint(got)})
		}
		delete(wantQueues, got.SchedulerItemID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for id, want := range wantQueues {
		differences = append(differences, ProjectionDifference{Projection: "rp_information_queue", Key: id, ExpectedText: fmt.Sprint(want)})
	}
	rows, err = q.QueryContext(ctx, `SELECT o.observation_id,o.source_event_id,o.observer_agent_id,o.subject_agent_id,o.place_id,o.channel,o.observed_world_time,o.claim_key,o.claim_payload
	 FROM observation_records o JOIN agent_profiles a ON a.agent_id=o.observer_agent_id AND a.instance_id=? AND a.branch_id=?
	 WHERE o.channel IN ('direct_message','rumor','organization_announcement','public_notice') OR o.claim_key LIKE 'information:%'`, instanceID, branchID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var got rpInformationObservation
		if err := rows.Scan(&got.ID, &got.SourceEventID, &got.ObserverID, &got.SubjectID, &got.PlaceID, &got.Channel, &got.WorldTime, &got.ClaimKey, &got.ClaimJSON); err != nil {
			rows.Close()
			return nil, err
		}
		want, ok := wantObservations[got.ID]
		if !ok || want != got {
			differences = append(differences, ProjectionDifference{Projection: "rp_information_observation", Key: got.ID, ExpectedText: fmt.Sprint(want), ActualText: fmt.Sprint(got)})
		}
		delete(wantObservations, got.ID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for id, want := range wantObservations {
		differences = append(differences, ProjectionDifference{Projection: "rp_information_observation", Key: id, ExpectedText: fmt.Sprint(want)})
	}
	sort.Slice(differences, func(i, j int) bool {
		if differences[i].Projection != differences[j].Projection {
			return differences[i].Projection < differences[j].Projection
		}
		return differences[i].Key < differences[j].Key
	})
	return differences, nil
}

// Called before generic knowledge replay so a lost delivery observation is
// recreated from immutable Events before knowledge is reconstructed from it.
func repairRPInformationProjections(ctx context.Context, conn *sql.Conn, instanceID, branchID string, head int64) error {
	queues, observations, err := rpInformationExpected(ctx, conn, instanceID, branchID, head)
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM agent_knowledge WHERE observer_agent_id IN (SELECT agent_id FROM agent_profiles WHERE instance_id=? AND branch_id=?) AND (claim_key LIKE 'information:%' OR observation_id IN (SELECT observation_id FROM observation_records WHERE channel IN ('direct_message','rumor','organization_announcement','public_notice')))`, instanceID, branchID); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM observation_records WHERE observer_agent_id IN (SELECT agent_id FROM agent_profiles WHERE instance_id=? AND branch_id=?) AND (claim_key LIKE 'information:%' OR channel IN ('direct_message','rumor','organization_announcement','public_notice'))`, instanceID, branchID); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `DELETE FROM scheduler_items WHERE instance_id=? AND branch_id=? AND phase_id=?`, instanceID, branchID, rpInformationPhase); err != nil {
		return err
	}
	queueIDs := make([]string, 0, len(queues))
	for id := range queues {
		queueIDs = append(queueIDs, id)
	}
	sort.Strings(queueIDs)
	for _, id := range queueIDs {
		item := queues[id]
		if err := execAgentOne(ctx, conn, "rebuild information queue", `INSERT INTO scheduler_items(scheduler_item_id,instance_id,branch_id,world_time,phase_id,declared_priority,status,payload) VALUES (?,?,?,?,?,?,?,?)`, item.SchedulerItemID, instanceID, branchID, item.WorldTime, item.PhaseID, item.DeclaredPriority, item.Status, item.Payload); err != nil {
			return err
		}
	}
	observationIDs := make([]string, 0, len(observations))
	for id := range observations {
		observationIDs = append(observationIDs, id)
	}
	sort.Strings(observationIDs)
	for _, id := range observationIDs {
		want := observations[id]
		if err := execAgentOne(ctx, conn, "rebuild information observation", `INSERT INTO observation_records(observation_id,source_event_id,observer_agent_id,subject_agent_id,place_id,channel,observed_world_time,claim_key,claim_payload) VALUES (?,?,?,?,?,?,?,?,?)`, want.ID, want.SourceEventID, want.ObserverID, want.SubjectID, want.PlaceID, want.Channel, want.WorldTime, want.ClaimKey, want.ClaimJSON); err != nil {
			return err
		}
	}
	return nil
}
