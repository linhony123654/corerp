package core

import "strings"

// RPObjectRequest commits one physical action, never an inference from dialogue.
// A model may propose it, but the controlled actor and current world must pass
// the same checks as a direct request before any fact is committed.
type RPObjectRequest struct {
	PrincipalID    string `json:"principal_id"`
	SessionID      string `json:"session_id"`
	ExpectedCursor int64  `json:"expected_cursor"`
	IdempotencyKey string `json:"idempotency_key"`
	Action         string `json:"action"`
	SourceID       string `json:"source_id,omitempty"`
	ObjectID       string `json:"object_id,omitempty"`
	AnchorID       string `json:"anchor_id,omitempty"`
	TargetEntityID string `json:"target_entity_id,omitempty"`
	OfferID        string `json:"offer_id,omitempty"`
}

func (r RPObjectRequest) Validate() error {
	if strings.TrimSpace(r.PrincipalID) == "" || strings.TrimSpace(r.SessionID) == "" || r.ExpectedCursor < 1 || strings.TrimSpace(r.IdempotencyKey) == "" || len(r.IdempotencyKey) > 128 {
		return NewError(CodeInvalidArgument, "object action requires an active session, cursor and bounded key")
	}
	for _, value := range []string{r.SourceID, r.ObjectID, r.AnchorID, r.TargetEntityID, r.OfferID} {
		if len(value) > 256 || strings.TrimSpace(value) != value {
			return NewError(CodeInvalidArgument, "object action references must be bounded")
		}
	}
	switch r.Action {
	case "stage":
		if r.SourceID == "" || r.ObjectID != "" || r.AnchorID != "" || r.TargetEntityID != "" || r.OfferID != "" {
			return NewError(CodeInvalidArgument, "stage requires only a declared stock source")
		}
	case "take", "stow":
		if r.ObjectID == "" || r.SourceID != "" || r.AnchorID != "" || r.TargetEntityID != "" || r.OfferID != "" {
			return NewError(CodeInvalidArgument, "take or stow requires only one object")
		}
	case "place", "move":
		if r.ObjectID == "" || r.AnchorID == "" || r.SourceID != "" || r.TargetEntityID != "" || r.OfferID != "" {
			return NewError(CodeInvalidArgument, "placement requires an object and authored anchor")
		}
	case "offer":
		if r.ObjectID == "" || r.TargetEntityID == "" || r.SourceID != "" || r.AnchorID != "" || r.OfferID != "" {
			return NewError(CodeInvalidArgument, "offer requires an object and distinct recipient")
		}
	case "accept", "refuse", "give", "receive", "cancel_offer":
		if r.OfferID == "" || r.SourceID != "" || r.ObjectID != "" || r.AnchorID != "" || r.TargetEntityID != "" {
			return NewError(CodeInvalidArgument, "response and transfer require an explicit offer")
		}
	default:
		return NewError(CodeInvalidArgument, "unsupported object interaction")
	}
	return nil
}

// Each Event records the resulting state and the exact prior Event it advances.
// An offered item stays with the offerer; acceptance never means transfer.
type RPObjectEvidence struct {
	Version                 string   `json:"version"`
	ClaimType               string   `json:"claim_type"`
	SessionID               string   `json:"session_id"`
	ActorEntityID           string   `json:"actor_entity_id"`
	Action                  string   `json:"action"`
	ObjectID                string   `json:"object_id"`
	SKUID                   string   `json:"sku_id"`
	DisplayName             string   `json:"display_name"`
	UnitMinor               int64    `json:"unit_minor"`
	SourceID                string   `json:"source_id,omitempty"`
	SourceEventID           string   `json:"source_event_id,omitempty"`
	PreviousEvent           string   `json:"previous_event_id,omitempty"`
	OfferID                 string   `json:"offer_id,omitempty"`
	OfferEventID            string   `json:"offer_event_id,omitempty"`
	TargetEntityID          string   `json:"target_entity_id,omitempty"`
	OwnerEntityID           string   `json:"owner_entity_id"`
	HolderEntityID          string   `json:"holder_entity_id,omitempty"`
	AnchorID                string   `json:"anchor_id,omitempty"`
	PlaceID                 string   `json:"place_id"`
	ZoneKey                 string   `json:"zone_key"`
	OfferStatus             string   `json:"offer_status,omitempty"`
	FromLocationID          string   `json:"from_location_id,omitempty"`
	ToLocationID            string   `json:"to_location_id,omitempty"`
	Description             string   `json:"description"`
	WitnessIDs              []string `json:"witness_ids"`
	VisibleTargetWitnessIDs []string `json:"visible_target_witness_ids"`
}
