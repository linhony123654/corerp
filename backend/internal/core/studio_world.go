package core

import (
	"fmt"
	"regexp"
	"strings"
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
	}
	if players != 1 {
		return bad("world specification requires exactly one controlled participant")
	}
	return nil
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
