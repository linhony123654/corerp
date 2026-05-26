package events

import (
	"strings"
	"testing"
	"time"

	"corerp/internal/core"
)

func TestCausalityDoesNotLinkUserMessagesByGenericActor(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	c := NewCausalityEngine(s)
	base := time.Now()

	e1 := core.Event{
		ID:        "u1",
		Type:      "user_message",
		Actor:     "user",
		Target:    "111",
		Canonical: true,
		CreatedAt: base,
	}
	e2 := core.Event{
		ID:        "u2",
		Type:      "user_message",
		Actor:     "user",
		Target:    "111",
		Canonical: true,
		CreatedAt: base.Add(time.Second),
	}

	if err := s.Append(e1); err != nil {
		t.Fatalf("append e1: %v", err)
	}
	if err := c.LinkNewEvent(e1); err != nil {
		t.Fatalf("link e1: %v", err)
	}
	if err := s.Append(e2); err != nil {
		t.Fatalf("append e2: %v", err)
	}
	if err := c.LinkNewEvent(e2); err != nil {
		t.Fatalf("link e2: %v", err)
	}

	got, err := s.GetByID("u2")
	if err != nil {
		t.Fatalf("get e2: %v", err)
	}
	if len(got.Causes) != 0 {
		t.Fatalf("user_message should not inherit generic user causes, got %+v", got.Causes)
	}
}

func TestCausalityLinksDialogueOnlyToMatchingUserMessage(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	c := NewCausalityEngine(s)
	base := time.Now()

	events := []core.Event{
		{ID: "u111", Type: "user_message", Actor: "user", Target: "111", Canonical: true, CreatedAt: base},
		{ID: "u_hlm", Type: "user_message", Actor: "user", Target: "《红楼梦》完整版、", Canonical: true, CreatedAt: base.Add(time.Second)},
		{ID: "d111", Type: "dialogue", Actor: "111", Target: "用户", Canonical: true, SessionID: "sess_1", SceneID: "scene_a", CreatedAt: base.Add(2 * time.Second)},
	}

	for _, evt := range events {
		if err := s.Append(evt); err != nil {
			t.Fatalf("append %s: %v", evt.ID, err)
		}
		if err := c.LinkNewEvent(evt); err != nil {
			t.Fatalf("link %s: %v", evt.ID, err)
		}
	}

	got, err := s.GetByID("d111")
	if err != nil {
		t.Fatalf("get d111: %v", err)
	}
	if len(got.Causes) != 1 {
		t.Fatalf("dialogue causes = %d, want 1", len(got.Causes))
	}
	if got.Causes[0].EventID != "u111" {
		t.Fatalf("dialogue cause = %s, want u111", got.Causes[0].EventID)
	}
}

func TestCausalityRebuildAllRewritesBadLinks(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	c := NewCausalityEngine(s)
	base := time.Now()

	user111 := core.Event{ID: "u111", Type: "user_message", Actor: "user", Target: "111", Canonical: true, CreatedAt: base}
	userOther := core.Event{ID: "u_other", Type: "user_message", Actor: "user", Target: "《红楼梦》完整版、", Canonical: true, CreatedAt: base.Add(time.Second)}
	dialogue111 := core.Event{
		ID:        "d111",
		Type:      "dialogue",
		Actor:     "111",
		Target:    "用户",
		Canonical: true,
		CreatedAt: base.Add(2 * time.Second),
		Causes: []core.Cause{
			{EventID: "u111", Weight: 0.9},
			{EventID: "u_other", Weight: 0.9},
		},
	}

	for _, evt := range []core.Event{user111, userOther, dialogue111} {
		if err := s.Append(evt); err != nil {
			t.Fatalf("append %s: %v", evt.ID, err)
		}
	}

	if err := c.RebuildAll(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	got, err := s.GetByID("d111")
	if err != nil {
		t.Fatalf("get d111: %v", err)
	}
	if len(got.Causes) != 1 || got.Causes[0].EventID != "u111" {
		t.Fatalf("rebuild causes = %+v, want only u111", got.Causes)
	}
}

func TestCausalityRebuildAllKeepsInterleavedMultiCharacterDialogueSeparated(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	c := NewCausalityEngine(s)
	base := time.Now()

	events := []core.Event{
		{
			ID:        "u111",
			Type:      "user_message",
			Actor:     "user",
			Target:    "111",
			SessionID: "sess_1",
			SceneID:   "scene_a",
			Canonical: true,
			CreatedAt: base,
		},
		{
			ID:        "u_anya",
			Type:      "user_message",
			Actor:     "user",
			Target:    "安雅",
			SessionID: "sess_1",
			SceneID:   "scene_a",
			Canonical: true,
			CreatedAt: base.Add(time.Second),
		},
		{
			ID:        "d111",
			Type:      "dialogue",
			Actor:     "111",
			Target:    "用户",
			SessionID: "sess_1",
			SceneID:   "scene_a",
			Canonical: true,
			CreatedAt: base.Add(2 * time.Second),
			Causes: []core.Cause{
				{EventID: "u_anya", Weight: 0.9},
			},
		},
		{
			ID:        "d_anya",
			Type:      "dialogue",
			Actor:     "安雅",
			Target:    "用户",
			SessionID: "sess_1",
			SceneID:   "scene_a",
			Canonical: true,
			CreatedAt: base.Add(3 * time.Second),
			Causes: []core.Cause{
				{EventID: "u111", Weight: 0.9},
				{EventID: "d111", Weight: 0.7},
			},
		},
	}

	for _, evt := range events {
		if err := s.Append(evt); err != nil {
			t.Fatalf("append %s: %v", evt.ID, err)
		}
	}

	if _, err := s.db.Exec(`UPDATE events SET effects = ? WHERE id = ?`, `[{"event_id":"d_anya","weight":0.9}]`, "u111"); err != nil {
		t.Fatalf("poison u111 effects: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE events SET effects = ? WHERE id = ?`, `[{"event_id":"d111","weight":0.9}]`, "u_anya"); err != nil {
		t.Fatalf("poison u_anya effects: %v", err)
	}

	if err := c.RebuildAll(); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	d111, err := s.GetByID("d111")
	if err != nil {
		t.Fatalf("get d111: %v", err)
	}
	if len(d111.Causes) != 1 || d111.Causes[0].EventID != "u111" {
		t.Fatalf("d111 causes = %+v, want only u111", d111.Causes)
	}

	dAnya, err := s.GetByID("d_anya")
	if err != nil {
		t.Fatalf("get d_anya: %v", err)
	}
	if len(dAnya.Causes) != 1 || dAnya.Causes[0].EventID != "u_anya" {
		t.Fatalf("d_anya causes = %+v, want only u_anya", dAnya.Causes)
	}

	u111, err := s.GetByID("u111")
	if err != nil {
		t.Fatalf("get u111: %v", err)
	}
	if len(u111.Effects) != 1 || causeEffectID(u111.Effects[0]) != "d111" {
		t.Fatalf("u111 effects = %+v, want only d111", u111.Effects)
	}

	uAnya, err := s.GetByID("u_anya")
	if err != nil {
		t.Fatalf("get u_anya: %v", err)
	}
	if len(uAnya.Effects) != 1 || causeEffectID(uAnya.Effects[0]) != "d_anya" {
		t.Fatalf("u_anya effects = %+v, want only d_anya", uAnya.Effects)
	}
}

func TestCausalityNarrativeOnlySkipsNoiseEvents(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	c := NewCausalityEngine(s)
	base := time.Now()

	events := []core.Event{
		{ID: "u1", Type: "user_message", Actor: "user", Target: "111", Canonical: true, CreatedAt: base},
		{ID: "d1", Type: "dialogue", Actor: "111", Target: "用户", Canonical: true, CreatedAt: base.Add(time.Second)},
		{ID: "v1", Type: "variable_set", Actor: "111", Canonical: true, CreatedAt: base.Add(2 * time.Second)},
		{ID: "f1", Type: "fact_extracted", Actor: "111", Canonical: false, CreatedAt: base.Add(3 * time.Second)},
	}

	for _, evt := range events {
		if err := s.Append(evt); err != nil {
			t.Fatalf("append %s: %v", evt.ID, err)
		}
		if err := c.LinkNewEvent(evt); err != nil {
			t.Fatalf("link %s: %v", evt.ID, err)
		}
	}

	chain, err := c.GetChainNarrativeOnly("f1", 4)
	if err != nil {
		t.Fatalf("narrative chain: %v", err)
	}
	if chain == nil {
		t.Fatal("expected a narrative ancestor chain")
	}
	if chain.Event.ID != "d1" {
		t.Fatalf("narrative root = %s, want d1", chain.Event.ID)
	}
	if len(chain.Causes) != 1 || chain.Causes[0].Event.ID != "u1" {
		t.Fatalf("narrative causes = %+v, want only u1", chain.Causes)
	}
	if len(chain.Effects) != 0 {
		t.Fatalf("narrative effects should skip noise events, got %+v", chain.Effects)
	}
}

func TestCausalityChainDoesNotRenderBackEdgeToRoot(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	c := NewCausalityEngine(s)
	base := time.Now()

	events := []core.Event{
		{ID: "u1", Type: "user_message", Actor: "user", Target: "111", Canonical: true, CreatedAt: base, Payload: map[string]interface{}{"content": "111"}},
		{ID: "d1", Type: "dialogue", Actor: "111", Target: "用户", Canonical: true, CreatedAt: base.Add(time.Second), Payload: map[string]interface{}{"content": "收到，我来回应。"}},
	}

	for _, evt := range events {
		if err := s.Append(evt); err != nil {
			t.Fatalf("append %s: %v", evt.ID, err)
		}
		if err := c.LinkNewEvent(evt); err != nil {
			t.Fatalf("link %s: %v", evt.ID, err)
		}
	}

	chain, err := c.GetChainNarrativeOnly("d1", 4)
	if err != nil {
		t.Fatalf("narrative chain: %v", err)
	}
	if chain == nil {
		t.Fatal("expected chain")
	}
	if len(chain.Causes) != 1 || chain.Causes[0].Event.ID != "u1" {
		t.Fatalf("causes = %+v, want only u1", chain.Causes)
	}
	if len(chain.Causes[0].Effects) != 0 {
		t.Fatalf("cause node should not include back-edge to root, got %+v", chain.Causes[0].Effects)
	}

	summary, err := c.GetChainSummaryNarrativeOnly("d1", 4)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if strings.Count(summary, "[dialogue]") != 1 {
		t.Fatalf("summary should render root dialogue once, got %q", summary)
	}
	if !strings.Contains(summary, "“收到，我来回应。”") {
		t.Fatalf("summary should include dialogue snippet, got %q", summary)
	}
	if !strings.Contains(summary, "“111”") {
		t.Fatalf("summary should include user message snippet, got %q", summary)
	}
}

func TestCausalityDoesNotCrossBranchBoundaries(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()

	c := NewCausalityEngine(s)
	base := time.Now()

	mainUser := core.Event{
		ID:        "u_main",
		Type:      "user_message",
		Actor:     "user",
		Target:    "111",
		Branch:    "main",
		Canonical: true,
		CreatedAt: base,
	}
	altDialogue := core.Event{
		ID:        "d_alt",
		Type:      "dialogue",
		Actor:     "111",
		Target:    "用户",
		Branch:    "alt",
		Canonical: true,
		CreatedAt: base.Add(time.Second),
	}

	for _, evt := range []core.Event{mainUser, altDialogue} {
		if err := s.Append(evt); err != nil {
			t.Fatalf("append %s: %v", evt.ID, err)
		}
		if err := c.LinkNewEvent(evt); err != nil {
			t.Fatalf("link %s: %v", evt.ID, err)
		}
	}

	got, err := s.GetByID("d_alt")
	if err != nil {
		t.Fatalf("get d_alt: %v", err)
	}
	if len(got.Causes) != 0 {
		t.Fatalf("alt branch dialogue should not link to main branch user message, got %+v", got.Causes)
	}
}
