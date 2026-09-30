package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"corerp.local/backend/internal/core"
	"corerp.local/backend/internal/storage"
)

func httpStudioPackage(kind string) core.StudioPackageBundle {
	m := core.PackageManifest{SchemaVersion: core.StudioPackageManifestVersion, ID: "http." + kind, Kind: kind, Version: "1.0.0", EngineAPI: core.StudioPackageManifestVersion, Requires: []core.PackageDependency{}, Optional: []core.PackageDependency{}, SchemaHash: core.StudioPackageSchemaHash}
	c := core.StudioPackageContent{Version: core.StudioPackageContentVersion}
	if kind == "system" {
		c.SystemRules = &core.StudioSystemRules{NPCDailyActionBudget: 2}
		m.Capabilities = []string{"rules.npc.daily_budget"}
		m.ContentFiles = []string{"system.json"}
	} else {
		style := core.DefaultRPStyle()
		style.Verbosity = "terse"
		c.NarrativeStyle = &style
		m.Capabilities = []string{"narrative.style"}
		m.ContentFiles = []string{"narrative.json"}
	}
	m.ContentHash, _ = core.HashJSON(c)
	return core.StudioPackageBundle{Manifest: m, Content: c}
}

func TestStudioCreateHTTPIdentityPackagesPlayAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "create-http.db")
	s, h := openHTTPTestServer(t, ctx, path)
	defer func() { s.Close() }()
	setup, err := s.PrepareRPTravel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r := storage.StudioCreateRequest{AuthorityInstanceID: storage.M2DemoInstanceID, AuthorityBranchID: storage.M2DemoBranchID, InstanceID: "http-created-world", IdempotencyKey: "http-create", PlayerPrincipalID: storage.M2RPPlayerPrincipal, SystemPackage: httpStudioPackage("system"), NarrativePackage: httpStudioPackage("narrative"),
		Spec: core.StudioWorldSpec{Version: core.StudioWorldSpecVersion, Name: "HTTP 世界", StartWorldTime: core.StudioWorldStart, Population: 2, OpeningMoneyMinor: 20, OpeningStockMinor: 2, Places: []core.StudioWorldPlace{{Key: "home", Name: "家", Kind: "home"}, {Key: "square", Name: "广场", Kind: "public"}}, Links: []core.StudioWorldLink{{From: "home", To: "square", Minutes: 5}}, People: []core.StudioWorldPerson{{Key: "lin", Name: "Lin", Place: "home", Player: true}, {Key: "cai", Name: "Cai", Place: "home"}}}}
	const route = "/api/v1/studio/worlds/create"
	assertStatus(t, performJSON(t, h, route, "", r), http.StatusUnauthorized)
	assertStatus(t, performJSON(t, h, route, creatorToken, r), http.StatusForbidden)
	_, err = s.ConfigureStudioAccessLocal(ctx, storage.StudioAccessRequest{Purpose: "create_world", Binding: core.CareerBinding{PrincipalID: "principal_operator", InstanceID: storage.M2DemoInstanceID, BranchID: storage.M2DemoBranchID, ExpectedHead: setup.EventSequence, IdempotencyKey: "http-creation-grant"}, TargetPrincipalID: "principal_creator", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{operatorToken, rpPlayerToken, buyerToken} {
		assertStatus(t, performJSON(t, h, route, token, r), http.StatusForbidden)
	}
	spoof := r
	spoof.PrincipalID = "principal_creator"
	assertStatus(t, performJSON(t, h, route, rpPlayerToken, spoof), http.StatusForbidden)
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var unknown map[string]any
	if err := json.Unmarshal(raw, &unknown); err != nil {
		t.Fatal(err)
	}
	unknown["system_package"].(map[string]any)["content"].(map[string]any)["code"] = "arbitrary"
	assertStatus(t, performJSON(t, h, route, creatorToken, unknown), http.StatusBadRequest)
	assertStatus(t, performRequest(t, h, http.MethodGet, route, creatorToken, nil), http.StatusMethodNotAllowed)
	response := performJSON(t, h, route, creatorToken, r)
	assertStatus(t, response, http.StatusCreated)
	created := decodeData[storage.StudioCreateResult](t, response)
	if created.Status != "ready" || created.Replayed || created.InstanceID != r.InstanceID || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("invalid creation receipt", created)
	}
	if created.RPReadiness.Status != "INCOMPLETE" || len(created.RPReadiness.Characters) != 1 || created.RPReadiness.Characters[0].Readiness.Persona != "MISSING" || created.RPReadiness.Characters[0].Readiness.RelationshipToInterlocutor != "UNKNOWN" {
		t.Fatal("world readiness concealed missing RP data", created.RPReadiness)
	}
	assertStatus(t, performJSON(t, h, route, creatorToken, r), http.StatusOK)
	changed := r
	changed.Spec.Name = "different"
	assertAPIError(t, performJSON(t, h, route, creatorToken, changed), http.StatusConflict, core.CodeIdempotencyMismatch)
	response = performJSON(t, h, "/api/v1/rp/bindings/list", rpPlayerToken, storage.RPDiscoverRequest{})
	assertStatus(t, response, http.StatusOK)
	bindings := decodeData[storage.RPDiscovery](t, response)
	found := false
	for _, b := range bindings.Bindings {
		found = found || (b.InstanceID == created.InstanceID && b.EntityID == created.EntityID)
	}
	if !found {
		t.Fatal("created binding missing", bindings)
	}
	open := core.RPSessionOpenRequest{InstanceID: created.InstanceID, BranchID: created.BranchID, EntityID: created.EntityID, POV: "second_person", IdempotencyKey: "open-created"}
	assertStatus(t, performJSON(t, h, "/api/v1/rp/sessions/open", creatorToken, open), http.StatusForbidden)
	response = performJSON(t, h, "/api/v1/rp/sessions/open", rpPlayerToken, open)
	assertStatus(t, response, http.StatusOK)
	session := decodeData[storage.RPSession](t, response)
	read := core.RPSessionReadRequest{SessionID: session.SessionID}
	response = performJSON(t, h, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	view := decodeData[storage.RPObservation](t, response)
	response = performJSON(t, h, "/api/v1/rp/turns/run", rpPlayerToken, core.RPSpeechRequest{SessionID: session.SessionID, ExpectedCursor: view.ObservationCursor, IdempotencyKey: "hello-created", Text: "你好。"})
	assertStatus(t, response, http.StatusOK)
	turn := decodeData[storage.RPTurnResult](t, response)
	if turn.Status != "settled" || turn.NarrativeStyle.Verbosity != "terse" || len(turn.NarrativeLines) == 0 || len(turn.NPCEventIDs) == 0 {
		t.Fatal("new-world turn not settled with installed style", turn)
	}
	s.Close()
	s, h = openHTTPTestServer(t, ctx, path)
	response = performJSON(t, h, route, creatorToken, r)
	assertStatus(t, response, http.StatusOK)
	retry := decodeData[storage.StudioCreateResult](t, response)
	if !retry.Replayed || retry.ReadyEventID != created.ReadyEventID {
		t.Fatal("restart created another world", retry)
	}
	if retry.RPReadiness.Status != created.RPReadiness.Status || len(retry.RPReadiness.Characters) != 1 || retry.RPReadiness.Characters[0] != created.RPReadiness.Characters[0] {
		t.Fatal("restart changed pinned setup readiness", retry.RPReadiness, created.RPReadiness)
	}
	response = performJSON(t, h, "/api/v1/rp/observe", rpPlayerToken, read)
	assertStatus(t, response, http.StatusOK)
	reopened := decodeData[storage.RPObservation](t, response)
	if reopened.ControlledEntity.EntityID != created.EntityID || len(reopened.RecentTurns) == 0 {
		t.Fatal("created Play state lost after restart", reopened)
	}
}
