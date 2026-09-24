package core

import (
	"crypto/sha256"
	"encoding/binary"
	"strings"
	"time"
)

// Only the actor's already-known contact source is exposed, never the stream,
// roll, policy configuration or another actor's evaluations.
type RPContactOpportunityContext struct {
	SourceEventID string `json:"source_event_id"`
	Selected      bool   `json:"selected"`
}

// A missed optional contact must not suppress independent sourced motivations.
func RPContactOpportunitySuppressed(input RPDecisionInput) bool {
	if input.Trigger == nil || input.Trigger.Kind != "elapsed_time" || input.ContactOpportunity == nil || input.ContactOpportunity.Selected {
		return false
	}
	if RPWeatherHomeDestination(input) != "" || RPSelectedStoreShortage(input) || RPSelectedWorkChange(input) || RPSelectedCommunityChange(input) || RPSelectedVisitDestination(input) != "" {
		return false
	}
	if input.Life != nil {
		for _, goal := range input.Life.Goals {
			if len(goal.SourceEventIDs) == 0 {
				continue
			}
			if goal.Code == "stabilize_income" || goal.Code == "collect_money_owed" || goal.Code == "avoid_conflict" && goal.SubjectEntityID == input.InterlocutorEntityID && len(input.ReachablePlaceIDs) > 0 {
				return false
			}
		}
	}
	return true
}

// Opportunity pressure is a derived view of a bounded authoritative history.
// Callers must supply sourced facts; this pure contract does not grant knowledge
// or declare that any proposed world event actually happened.
type RPOpportunityPressure struct {
	Busy          bool
	CoolingDown   bool
	RecentMajor   bool
	RecentChanges int
	Quiet         bool
	Seeking       bool
}

// Basis points keep probability arithmetic replayable (10000 = certainty).
// Even an eligible, quiet, actively seeking actor is never promised an event.
func RPOpportunityProbability(base int, rare bool, p RPOpportunityPressure) (int, error) {
	ceiling := 5000
	if rare {
		ceiling = 100
	}
	if base < 0 || base > ceiling || p.RecentChanges < 0 || p.RecentChanges > 10000 {
		return 0, NewError(CodeInvalidArgument, "opportunity probability/history is outside bounds")
	}
	if p.Busy || p.CoolingDown || base == 0 {
		return 0, nil
	}
	chance := base
	if !rare {
		if p.Quiet && p.RecentChanges == 0 && !p.RecentMajor {
			chance += min(base/4, 500)
		}
		if p.Seeking {
			chance += base / 2
		}
		ceiling = 7500
	}
	chance = min(chance, ceiling) / (1 + p.RecentChanges)
	if p.RecentMajor {
		chance /= 4
	}
	return chance, nil
}

type RPOpportunityDrawKey struct {
	PolicyEventID string
	StreamSeed    string
	InstanceID    string
	BranchID      string
	ActorID       string
	Kind          string
	SourceEventID string
	WorldTime     string
}

// The identity deliberately excludes session, request key, provider output and
// wall clock. Storage must persist the first evaluation per actor/source/window,
// including misses, and enforce cooldown across policy revisions.
type RPOpportunityDraw struct {
	Version           string `json:"version"`
	WindowStart       string `json:"window_start"`
	IdentityHash      string `json:"identity_hash"`
	RollBasisPoints   int    `json:"roll_basis_points"`
	ChanceBasisPoints int    `json:"chance_basis_points"`
	Selected          bool   `json:"selected"`
}

func DrawRPOpportunity(k RPOpportunityDrawKey, chance int) (RPOpportunityDraw, error) {
	var empty RPOpportunityDraw
	for _, value := range []string{k.PolicyEventID, k.StreamSeed, k.InstanceID, k.BranchID, k.ActorID, k.Kind, k.SourceEventID} {
		if strings.TrimSpace(value) == "" || len(value) > 256 {
			return empty, NewError(CodeInvalidArgument, "opportunity draw requires bounded source and stream identity")
		}
	}
	if chance < 0 || chance > 7500 {
		return empty, NewError(CodeInvalidArgument, "opportunity draw probability exceeds non-guaranteed bound")
	}
	at, err := time.Parse(time.RFC3339, k.WorldTime)
	if err != nil {
		return empty, NewError(CodeInvalidArgument, "opportunity draw requires world time")
	}
	window := at.UTC().Truncate(time.Hour).Format(time.RFC3339)
	identity, err := HashJSON([]string{"corerp.opportunity.draw.v1", k.PolicyEventID, k.StreamSeed, k.InstanceID, k.BranchID, k.ActorID, k.Kind, k.SourceEventID, window})
	if err != nil {
		return empty, err
	}
	digest := sha256.Sum256([]byte(identity))
	roll := int(binary.BigEndian.Uint64(digest[:8]) % 10000)
	return RPOpportunityDraw{Version: "corerp.opportunity.draw.v1", WindowStart: window, IdentityHash: identity, RollBasisPoints: roll, ChanceBasisPoints: chance, Selected: roll < chance}, nil
}
