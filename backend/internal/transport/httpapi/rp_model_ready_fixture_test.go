package httpapi

import (
	"context"
	"net/http"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func openAuthoredRPModelHTTPSession(t *testing.T, ctx context.Context, store *storage.Store, handler http.Handler) storage.RPSession {
	t.Helper()
	prepared, err := store.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ConfigureStudioAccessLocal(ctx, storage.StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: prepared.EventSequence, IdempotencyKey: "http-model-author-grant"}, TargetPrincipalID: "principal_creator", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	declaration := storage.StudioCreateRequest{AuthorityInstanceID: storage.M2DemoInstanceID, AuthorityBranchID: storage.M2DemoBranchID, InstanceID: "http-authored-model-world", IdempotencyKey: "http-authored-model-world", PlayerPrincipalID: storage.M2RPPlayerPrincipal, SystemPackage: httpStudioPackage("system"), NarrativePackage: httpStudioPackage("narrative"),
		Spec: core.StudioWorldSpec{Version: core.StudioWorldSpecVersion, Name: "模型接口测试世界", StartWorldTime: core.StudioWorldStart, Population: 2, OpeningMoneyMinor: 20, OpeningStockMinor: 2, Places: []core.StudioWorldPlace{{Key: "home", Name: "家", Kind: "home"}, {Key: "square", Name: "广场", Kind: "public"}}, Links: []core.StudioWorldLink{{From: "home", To: "square", Minutes: 5}}, People: []core.StudioWorldPerson{{Key: "lin", Name: "Lin", Place: "home", Player: true}, {Key: "cai", Name: "Cai", Place: "home", Persona: "说话简短，先听清对方的问题，礼貌回应。不知道的事明确说不知道。"}}}}
	response := performJSON(t, handler, "/api/v1/studio/worlds/create", creatorToken, declaration)
	assertStatus(t, response, http.StatusCreated)
	created := decodeData[storage.StudioCreateResult](t, response)
	response = performJSON(t, handler, "/api/v1/rp/sessions/open", rpPlayerToken, core.RPSessionOpenRequest{InstanceID: created.InstanceID, BranchID: created.BranchID, EntityID: created.EntityID, POV: "second_person", IdempotencyKey: "http-authored-model-session"})
	assertStatus(t, response, http.StatusOK)
	return decodeData[storage.RPSession](t, response)
}
