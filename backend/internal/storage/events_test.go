package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
)

func TestVisibleEventsRespectPrincipalSubjectAndRedaction(t *testing.T) {
	ctx := context.Background()
	store := openBootstrappedStore(t, ctx, filepath.Join(t.TempDir(), "visible-events.db"))
	defer store.Close()
	run, err := store.RunStrictWorld(ctx, 65, 1000)
	if err != nil {
		t.Fatal(err)
	}

	creatorPage, err := store.ListVisibleEvents(ctx, core.VisibleEventRequest{
		PrincipalID: "principal_creator", CapabilityID: "world.events.read",
		InstanceID: DemoInstanceID, BranchID: DemoBranchID, SubjectID: DemoBranchID, Limit: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(creatorPage.Events) != int(run.HeadSequence) || creatorPage.Events[0].EventType != "WorldInitialized" {
		t.Fatalf("creator did not receive full event authority: count=%d head=%d", len(creatorPage.Events), run.HeadSequence)
	}

	operatorPage, err := store.ListVisibleEvents(ctx, core.VisibleEventRequest{
		PrincipalID: "principal_operator", CapabilityID: "diagnostics.events.read",
		InstanceID: DemoInstanceID, BranchID: DemoBranchID, SubjectID: DemoBranchID, Limit: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(operatorPage.Events) != len(creatorPage.Events) {
		t.Fatalf("operator diagnostic count differs: %d != %d", len(operatorPage.Events), len(creatorPage.Events))
	}
	for _, event := range operatorPage.Events {
		if !event.Redacted || string(event.Payload) != `{"redacted":true}` {
			t.Fatalf("operator event payload was not redacted: %+v", event)
		}
	}

	playerRequest := core.VisibleEventRequest{
		PrincipalID: "principal_buyer", CapabilityID: "world.events.read",
		InstanceID: DemoInstanceID, BranchID: DemoBranchID, SubjectID: DemoEmployee1EntityID, Limit: 100,
	}
	playerPage, err := store.ListVisibleEvents(ctx, playerRequest)
	if err != nil {
		t.Fatal(err)
	}
	if len(playerPage.Events) == 0 || len(playerPage.Events) >= len(creatorPage.Events) {
		t.Fatalf("player filtering is ineffective: player=%d creator=%d", len(playerPage.Events), len(creatorPage.Events))
	}
	for _, event := range playerPage.Events {
		if event.EventType == "WorldInitialized" || event.EventType == "StoreRestocked" {
			t.Fatalf("player received hidden event type: %+v", event)
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if raw, ok := payload["buyer_account_id"]; ok {
			var accountID string
			if err := json.Unmarshal(raw, &accountID); err != nil || accountID != DemoBuyerAccountID {
				t.Fatalf("player received another buyer event: %s", event.Payload)
			}
		}
	}

	playerRequest.SubjectID = DemoEmployee2EntityID
	if _, err := store.ListVisibleEvents(ctx, playerRequest); !core.HasCode(err, core.CodeUnauthorized) {
		t.Fatalf("player cross-subject event read should be denied, got %v", err)
	}

	last := playerPage.Events[len(playerPage.Events)-1].Sequence
	playerRequest.SubjectID = DemoEmployee1EntityID
	playerRequest.AfterSequence = last
	next, err := store.ListVisibleEvents(ctx, playerRequest)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range next.Events {
		if event.Sequence <= last {
			t.Fatalf("cursor replayed an already visible event: %+v", event)
		}
	}
}
