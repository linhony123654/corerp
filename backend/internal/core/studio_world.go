package core

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const StudioWorldSpecVersion = "corerp.studio-world.v1"

// The first creator template uses the calendar already consumed by career rules.
// Arbitrary calendar migration is not implied by a user-editable timestamp.
const StudioWorldStart = "2026-09-22T00:00:00Z"

type StudioWorldPlace struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}
type StudioWorldLink struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Minutes int    `json:"minutes"`
}
type StudioWorldPerson struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Place  string `json:"place"`
	Player bool   `json:"player"`
	// Persona is the character's own background, seeded onto their agent
	// profile and surfaced in RP decision inputs. Empty means RP-not-ready;
	// the model must not invent a biography from the character's name.
	Persona string `json:"persona,omitempty"`
	// PublicPresentation is explicitly safe for the player's narrator to use
	// as a style cue after the actor is identified. Private persona is never
	// promoted into this field by default.
	PublicPresentation string `json:"public_presentation,omitempty"`
	// Routine declares scheduled daily life (move + activity at a place).
	// Times are absolute world times on or after StartWorldTime.
	Routine []StudioWorldRoutineEntry `json:"routine,omitempty"`
}

// StudioWorldRelationship is a directional creator declaration. It says what
// From is to To and how From may address To; the reverse direction is never
// inferred. It is canon only when committed by the Studio identity command.
type StudioWorldRelationship struct {
	From          string   `json:"from"`
	To            string   `json:"to"`
	Role          string   `json:"role"`
	AddressTo     []string `json:"address_to,omitempty"`
	SelfReference string   `json:"self_reference,omitempty"`
}

type StudioWorldRoutineEntry struct {
	WorldTime    string `json:"world_time"`
	Place        string `json:"place"`
	ActivityCode string `json:"activity_code"`
}

// StudioWorldObject is a bounded declaration, never executable content. Each
// kind has a fixed engine-owned state machine and action vocabulary.
type StudioWorldObject struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	Place        string `json:"place"`
	Kind         string `json:"kind"`
	InitialState string `json:"initial_state"`
}

// A declaration to be validated before genesis, not a projection or permission.
// Population/resources begin in the existing Cohort owner; named participants
// must subsequently be materialized through its conserved source records.
type StudioWorldSpec struct {
	Version           string              `json:"version"`
	Name              string              `json:"name"`
	StartWorldTime    string              `json:"start_world_time"`
	Population        int64               `json:"population"`
	OpeningMoneyMinor int64               `json:"opening_money_minor"`
	OpeningStockMinor int64               `json:"opening_stock_minor"`
	Places            []StudioWorldPlace  `json:"places"`
	Links             []StudioWorldLink   `json:"links"`
	People            []StudioWorldPerson `json:"people"`
	Objects           []StudioWorldObject `json:"objects,omitempty"`
	// Acquaintances declares person-key pairs who mutually recognize each
	// other at world start, committed as an RPIdentitiesDeclared event.
	Acquaintances [][2]string `json:"acquaintances,omitempty"`
	// Relationships refine declared acquaintances with authored role and
	// address rules. Omitted or incomplete declarations stay unknown/missing.
	Relationships []StudioWorldRelationship `json:"relationships,omitempty"`
}

var studioLocalKey = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,47}$`)

func studioDisplayName(s string) bool {
	return utf8.ValidString(s) && strings.TrimSpace(s) == s && utf8.RuneCountInString(s) >= 1 && utf8.RuneCountInString(s) <= 100 && !strings.ContainsAny(s, "\x00\r\n")
}

func (s StudioWorldSpec) Validate() error {
	bad := func(message string) error { return NewError(CodeInvalidArgument, message) }
	if s.Version != StudioWorldSpecVersion || s.StartWorldTime != StudioWorldStart || !studioDisplayName(s.Name) {
		return bad("unsupported world specification version, calendar or name")
	}
	if s.Population < 2 || s.Population > 1000000 {
		return bad("population must be between 2 and 1000000")
	}
	if s.OpeningMoneyMinor < 0 || s.OpeningMoneyMinor > MaxJSONSafeInteger || s.OpeningStockMinor < 0 || s.OpeningStockMinor > MaxJSONSafeInteger {
		return bad("opening resources must be nonnegative safe integers")
	}
	if len(s.Places) < 2 || len(s.Places) > 32 || len(s.People) < 2 || len(s.People) > 16 || int64(len(s.People)) > s.Population || len(s.Links) < 1 || len(s.Links) > 64 {
		return bad("world topology or named population exceeds supported bounds")
	}
	places := map[string]bool{}
	for _, p := range s.Places {
		if !studioLocalKey.MatchString(p.Key) || places[p.Key] || !studioDisplayName(p.Name) || (p.Kind != "home" && p.Kind != "work" && p.Kind != "public") {
			return bad("invalid or duplicate place")
		}
		places[p.Key] = true
	}
	graph := map[string][]string{}
	pairs := map[string]bool{}
	for _, l := range s.Links {
		if !places[l.From] || !places[l.To] || l.From == l.To || l.Minutes < 1 || l.Minutes > 120 {
			return bad("invalid world link")
		}
		a, b := l.From, l.To
		if a > b {
			a, b = b, a
		}
		pair := a + ":" + b
		if pairs[pair] {
			return bad("duplicate bidirectional world link")
		}
		pairs[pair] = true
		graph[l.From] = append(graph[l.From], l.To)
		graph[l.To] = append(graph[l.To], l.From)
	}
	seen := map[string]bool{s.Places[0].Key: true}
	queue := []string{s.Places[0].Key}
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		for _, next := range graph[key] {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	if len(seen) != len(places) {
		return bad("world places must be connected")
	}
	if len(s.Objects) > 32 {
		return bad("too many scene objects")
	}
	objects := map[string]bool{}
	for _, object := range s.Objects {
		if !studioLocalKey.MatchString(object.Key) || objects[object.Key] || !studioDisplayName(object.Name) || !places[object.Place] || !validStudioObjectState(object.Kind, object.InitialState) {
			return bad("invalid or duplicate scene object")
		}
		objects[object.Key] = true
	}
	people := map[string]bool{}
	players := 0
	for _, p := range s.People {
		if !studioLocalKey.MatchString(p.Key) || people[p.Key] || !studioDisplayName(p.Name) || !places[p.Place] {
			return bad("invalid participant or initial place")
		}
		people[p.Key] = true
		if p.Player {
			players++
		}
		if utf8.RuneCountInString(p.Persona) > 500 || strings.ContainsAny(p.Persona, "\x00\r") || utf8.RuneCountInString(p.PublicPresentation) > 500 || strings.ContainsAny(p.PublicPresentation, "\x00\r") {
			return bad("invalid participant persona")
		}
		last, _ := time.Parse(time.RFC3339, s.StartWorldTime)
		for _, r := range p.Routine {
			at, err := time.Parse(time.RFC3339, r.WorldTime)
			if err != nil {
				return bad("invalid participant routine time")
			}
			if !studioLocalKey.MatchString(r.ActivityCode) || !places[r.Place] || at.Before(last) {
				return bad("invalid participant routine entry")
			}
			last = at
		}
	}
	if players != 1 {
		return bad("world specification requires exactly one controlled participant")
	}
	seenPairs := map[[2]string]bool{}
	for _, pair := range s.Acquaintances {
		a, b := pair[0], pair[1]
		if !people[a] || !people[b] || a == b {
			return bad("invalid acquaintance pair")
		}
		key := [2]string{a, b}
		if key[0] > key[1] {
			key = [2]string{b, a}
		}
		if seenPairs[key] {
			return bad("duplicate acquaintance pair")
		}
		seenPairs[key] = true
	}
	if len(s.Relationships) > 64 {
		return bad("too many authored relationships")
	}
	seenDirections := map[[2]string]bool{}
	for _, relation := range s.Relationships {
		if !people[relation.From] || !people[relation.To] || relation.From == relation.To ||
			!studioDisplayName(relation.Role) || utf8.RuneCountInString(relation.Role) > 60 || len(relation.AddressTo) > 4 {
			return bad("invalid authored relationship")
		}
		pair := [2]string{relation.From, relation.To}
		if seenDirections[pair] {
			return bad("duplicate authored relationship")
		}
		seenDirections[pair] = true
		undirected := pair
		if undirected[0] > undirected[1] {
			undirected = [2]string{undirected[1], undirected[0]}
		}
		if !seenPairs[undirected] {
			return bad("authored relationship requires declared acquaintance")
		}
		addresses := map[string]bool{}
		for _, address := range relation.AddressTo {
			if !studioDisplayName(address) || utf8.RuneCountInString(address) > 40 || addresses[address] {
				return bad("invalid authored address")
			}
			addresses[address] = true
		}
		if relation.SelfReference != "" && (!studioDisplayName(relation.SelfReference) || utf8.RuneCountInString(relation.SelfReference) > 40) {
			return bad("invalid authored self-reference")
		}
	}
	return nil
}

func validStudioObjectState(kind, state string) bool {
	switch kind {
	case "door", "container":
		return state == "open" || state == "closed"
	case "light":
		return state == "on" || state == "off"
	default:
		return false
	}
}

// Stable namespaced identities prevent collisions with another world's local keys.
// This is identity construction only; callers still must check world ownership.
func StudioWorldObjectID(instance, kind, key string) (string, error) {
	if strings.TrimSpace(instance) != instance || instance == "" || len(instance) > 128 || !utf8.ValidString(instance) || strings.ContainsAny(instance, "\x00\r\n") || !studioLocalKey.MatchString(kind) || !studioLocalKey.MatchString(key) {
		return "", NewError(CodeInvalidArgument, "invalid world object identity")
	}
	hash, err := HashJSON([]string{StudioWorldSpecVersion, instance, kind, key})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("studio_%s_%s", kind, hash[7:]), nil
}
