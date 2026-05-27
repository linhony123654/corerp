package events

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"corerp/internal/core"
)

// CanonicalHashV1 produces a deterministic, versioned SHA-256 of a WorldState.
// This is the root of trust for the entire replay system.
//
// Contract:
//   - Same events in same order → same hash. Always.
//   - Map keys are sorted before marshaling.
//   - Float values are rounded to 6 decimal places to avoid platform variance.
//   - The version prefix ("v1:") means future format changes can coexist.
func CanonicalHashV1(s core.WorldState) string {
	canon := canonicalState{
		Scene: canonicalScene{
			Location:    s.Scene.Location,
			TimeOfDay:   s.Scene.TimeOfDay,
			Weather:     s.Scene.Weather,
			Characters:  sortedStrings(s.Scene.Characters),
			Description: s.Scene.Description,
		},
		Clock: canonicalClock{
			Hour:   s.Clock.Hour,
			Minute: s.Clock.Minute,
			Day:    s.Clock.Day,
		},
		Tension:       round6(s.Tension),
		Flags:         sortedFlags(s.Flags),
		Relationships: sortedRelationships(s.Relationships),
		Variables:     sortedVariables(s.Variables),
	}
	data, _ := json.Marshal(canon)
	h := sha256.Sum256(data)
	return "v1:" + fmt.Sprintf("%x", h)
}

// EventHash produces a chained hash for an event.
// Each event's hash = SHA256(prevHash + eventID + eventType + canonicalPayload).
func EventHash(prevHash string, e core.Event) string {
	payload := canonicalPayload(e.Payload)
	input := prevHash + e.ID + e.Type + payload
	h := sha256.Sum256([]byte(input))
	return "ev1:" + fmt.Sprintf("%x", h)
}

// VerifyEventChain checks that each event's hash follows from the previous.
// Returns the index of the first invalid event, or -1 if all valid.
func VerifyEventChain(events []core.Event) int {
	if len(events) == 0 {
		return -1
	}
	prev := "genesis"
	for i, e := range events {
		expected := EventHash(prev, e)
		if e.Hash != expected {
			return i
		}
		prev = expected
	}
	return -1
}

// AppendEventToChain computes and sets the hash for an event, given the prior hash.
func AppendEventToChain(prevHash string, e *core.Event) {
	e.Hash = EventHash(prevHash, *e)
}

// === internal canonical types (stable serialization format) ===

type canonicalState struct {
	Scene         canonicalScene          `json:"scene"`
	Clock         canonicalClock          `json:"clock"`
	Tension       float64                 `json:"tension"`
	Flags         []canonicalFlag         `json:"flags"`
	Relationships []canonicalRelationship `json:"relationships"`
	Variables     []canonicalVariable     `json:"variables"`
}

type canonicalScene struct {
	Location    string   `json:"location"`
	TimeOfDay   string   `json:"time_of_day"`
	Weather     string   `json:"weather"`
	Characters  []string `json:"characters"`
	Description string   `json:"description"`
}

type canonicalClock struct {
	Hour   int `json:"hour"`
	Minute int `json:"minute"`
	Day    int `json:"day"`
}

type canonicalFlag struct {
	Key   string `json:"key"`
	Value bool   `json:"value"`
}

type canonicalRelationship struct {
	Key      string  `json:"key"`
	Trust    float64 `json:"trust"`
	Intimacy float64 `json:"intimacy"`
	Fear     float64 `json:"fear"`
	Respect  float64 `json:"respect"`
	Debt     float64 `json:"debt"`
}

type canonicalVariable struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

func canonicalPayload(p map[string]interface{}) string {
	if p == nil {
		return "{}"
	}
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var pairs []string
	for _, k := range keys {
		v, _ := json.Marshal(p[k])
		pairs = append(pairs, `"`+k+`":`+string(v))
	}
	return "{" + joinStrings(pairs, ",") + "}"
}

// === helpers ===

func sortedStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	out := make([]string, len(in))
	copy(out, in)
	sort.Strings(out)
	return out
}

func sortedFlags(flags map[string]bool) []canonicalFlag {
	var out []canonicalFlag
	for k, v := range flags {
		out = append(out, canonicalFlag{Key: k, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func sortedRelationships(rels map[string]core.Relationship) []canonicalRelationship {
	var out []canonicalRelationship
	for k, r := range rels {
		out = append(out, canonicalRelationship{
			Key: k, Trust: round6(r.Trust), Intimacy: round6(r.Intimacy),
			Fear: round6(r.Fear), Respect: round6(r.Respect), Debt: round6(r.Debt),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func sortedVariables(vars map[string]interface{}) []canonicalVariable {
	var out []canonicalVariable
	for k, v := range vars {
		out = append(out, canonicalVariable{Key: k, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func round6(f float64) float64 {
	return float64(int(f*1e6+0.5)) / 1e6
}

func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	r := ss[0]
	for _, s := range ss[1:] {
		r += sep + s
	}
	return r
}
