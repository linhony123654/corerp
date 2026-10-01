package storage

import (
	"context"
	"database/sql"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"corerp.local/backend/internal/core"
)

const rpDialogueCandidateLimit = 512
const rpPeerDialogueCandidateLimit = 128
const rpPeerExchangeLimit = 2
const rpRelevantExchangeLimit = 4
const rpRelevantDialogueRuneBudget = 6000

type rpDialogueCandidate struct {
	dialogue  core.RPDecisionDialogue
	sequence  int64
	session   string
	turn      string
	groupSize int
	peer      bool
}

// Retrieval is a read projection over the same accepted/heard speech as the
// recent window. No summary, model call, unobserved turn content or new canon.
func readRPRelevantDialogue(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput) ([]core.RPDecisionExchange, error) {
	candidates, err := readRPDialogueCandidates(ctx, conn, input)
	if err != nil {
		return nil, err
	}
	return selectRPRelevantDialogue(input, candidates), nil
}

// Reserve a bounded pool for this directed peer before global recency can
// crowd it out. Counts and peer membership use only the same authorized rows;
// unheard siblings never qualify a group or complete a truncated exchange.
func readRPDialogueCandidates(ctx context.Context, conn *sql.Conn, input core.RPDecisionInput) ([]rpDialogueCandidate, error) {
	rows, err := conn.QueryContext(ctx, `WITH authorized AS (
		SELECT u.speaker_entity_id,u.speech_text,u.event_id,u.world_time,e.event_sequence,
		       u.session_id,COALESCE(NULLIF(json_extract(e.payload,'$.parent_turn_id'),''),u.turn_id) AS exchange_turn,
		       e.payload AS speech_source,COALESCE(hearing.claim_payload,'') AS hearing_source
		FROM rp_utterances u JOIN events e ON e.event_id=u.event_id
		LEFT JOIN observation_records hearing ON hearing.source_event_id=e.event_id
		  AND hearing.observer_agent_id=? AND hearing.subject_agent_id=u.speaker_entity_id AND hearing.claim_key='speech:'||u.event_id
		WHERE e.instance_id=? AND e.branch_id=? AND e.event_sequence<=?
		  AND (u.speaker_entity_id=? OR EXISTS (
		    SELECT 1 FROM observation_records o
		    WHERE o.source_event_id=u.event_id AND o.observer_agent_id=?
		      AND o.subject_agent_id=u.speaker_entity_id
		      AND o.claim_key='speech:' || u.event_id
		      AND json_extract(o.claim_payload,'$.claim_type')='speaker_said'))
	), grouped AS (
		SELECT *,COUNT(*) OVER exchange_scope AS group_size,
		       (MAX(speaker_entity_id=?) OVER exchange_scope
		        AND MAX(speaker_entity_id=? AND ?<>'') OVER exchange_scope) AS peer
		FROM authorized WINDOW exchange_scope AS (PARTITION BY session_id,exchange_turn)
	), peer_candidates AS (
		SELECT * FROM grouped WHERE peer ORDER BY event_sequence DESC LIMIT ?
	), general_candidates AS (
		SELECT * FROM grouped WHERE event_id NOT IN (SELECT event_id FROM peer_candidates)
		ORDER BY event_sequence DESC LIMIT (?-(SELECT COUNT(*) FROM peer_candidates))
	)
	SELECT * FROM peer_candidates UNION ALL SELECT * FROM general_candidates ORDER BY event_sequence DESC`, input.NPCEntityID, input.InstanceID, input.BranchID, input.HeadSequence,
		input.NPCEntityID, input.NPCEntityID, input.NPCEntityID, input.InterlocutorEntityID, input.InterlocutorEntityID, rpPeerDialogueCandidateLimit, rpDialogueCandidateLimit)
	if err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "read NPC relevant dialogue candidates", err)
	}
	defer rows.Close()
	var candidates []rpDialogueCandidate
	for rows.Next() {
		var candidate rpDialogueCandidate
		d := &candidate.dialogue
		var sourceJSON, hearingJSON string
		if err := rows.Scan(&d.SpeakerEntityID, &d.Text, &d.EventID, &d.WorldTime, &candidate.sequence, &candidate.session, &candidate.turn, &sourceJSON, &hearingJSON, &candidate.groupSize, &candidate.peer); err != nil {
			return nil, core.WrapError(core.CodeStorageFailure, "scan NPC relevant dialogue", err)
		}
		d.SpeechTone, err = recordedRPSpeechTone(sourceJSON, hearingJSON, d.SpeakerEntityID, input.NPCEntityID)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, core.WrapError(core.CodeStorageFailure, "iterate NPC relevant dialogue", err)
	}
	return candidates, nil
}

// Repair split recent exchanges first, then rank older exchanges by shared
// words/Han bigrams. Lexical retrieval alone cannot match every paraphrase.
// Sort every ranking input/tie so a reread at the same head has the same hash.
func selectRPRelevantDialogue(input core.RPDecisionInput, candidates []rpDialogueCandidate) []core.RPDecisionExchange {
	type group struct {
		dialogue []rpDialogueCandidate
		terms    map[string]bool
		sequence int64
		missing  bool
		recent   bool
		current  bool
		peer     bool
		size     int
		score    float64
	}
	recent := map[string]bool{}
	for _, d := range input.RecentDialogue {
		recent[d.EventID] = true
	}
	groups := map[[2]string]*group{}
	for _, c := range candidates {
		key := [2]string{c.session, c.turn}
		g := groups[key]
		if g == nil {
			g = &group{terms: map[string]bool{}}
			groups[key] = g
		}
		g.dialogue = append(g.dialogue, c)
		g.peer = g.peer || c.peer
		if c.sequence > g.sequence {
			g.sequence = c.sequence
		}
		if c.groupSize > g.size {
			g.size = c.groupSize
		}
		if c.dialogue.EventID == input.SpeechEventID {
			g.current = true
		} else if recent[c.dialogue.EventID] {
			g.recent = true
		} else {
			g.missing = true
		}
		for term := range rpDialogueTerms(c.dialogue.Text) {
			g.terms[term] = true
		}
	}
	query := rpDialogueTerms(input.PlayerSpeechText)
	terms := make([]string, 0, len(query))
	for term := range query {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	frequency := map[string]int{}
	for _, g := range groups {
		for _, term := range terms {
			if g.terms[term] {
				frequency[term]++
			}
		}
	}
	var ranked []*group
	var peerGroups []*group
	for _, g := range groups {
		if g.peer && !g.current && len(g.dialogue) == g.size {
			peerGroups = append(peerGroups, g)
		}
	}
	sort.Slice(peerGroups, func(i, j int) bool { return peerGroups[i].sequence > peerGroups[j].sequence })
	retainedPeers := map[*group]bool{}
	for _, g := range peerGroups[:min(len(peerGroups), rpPeerExchangeLimit)] {
		retainedPeers[g] = true
	}
	for _, g := range groups {
		// A recent reply must not hide its older question or qualification.
		// Keep the authorized group together, including overlapping recent words.
		// The SQL count excludes unheard/out-of-head siblings. Reject a group cut
		// by the candidate window rather than presenting it as a full exchange.
		if (!g.missing && !retainedPeers[g]) || g.current || len(g.dialogue) < g.size {
			continue
		}
		for _, term := range terms {
			if g.terms[term] {
				// Common conversational fragments should not displace a distinct topic.
				if len(groups) > 3 && frequency[term]*2 > len(groups) {
					continue
				}
				g.score += math.Log(1 + float64(len(groups))/float64(frequency[term]))
			}
		}
		if g.recent || retainedPeers[g] || g.score > 0 {
			ranked = append(ranked, g)
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		repairI, repairJ := ranked[i].recent && ranked[i].missing, ranked[j].recent && ranked[j].missing
		if repairI != repairJ {
			return repairI
		}
		if repairI {
			return ranked[i].sequence > ranked[j].sequence
		}
		if retainedPeers[ranked[i]] != retainedPeers[ranked[j]] {
			return retainedPeers[ranked[i]]
		}
		if retainedPeers[ranked[i]] {
			return ranked[i].sequence > ranked[j].sequence
		}
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].sequence > ranked[j].sequence
	})
	var selected []*group
	remaining := rpRelevantDialogueRuneBudget
	for _, g := range ranked {
		size := 0
		for _, c := range g.dialogue {
			size += utf8.RuneCountInString(c.dialogue.Text)
		}
		if size > remaining {
			continue
		} // Preserve complete authorized groups and original utterances.
		remaining -= size
		selected = append(selected, g)
		if len(selected) == rpRelevantExchangeLimit {
			break
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].sequence < selected[j].sequence })
	var result []core.RPDecisionExchange
	for _, g := range selected {
		sort.Slice(g.dialogue, func(i, j int) bool { return g.dialogue[i].sequence < g.dialogue[j].sequence })
		exchange := core.RPDecisionExchange{RecentContext: g.recent && g.missing, PeerContext: retainedPeers[g]}
		for _, c := range g.dialogue {
			exchange.Dialogue = append(exchange.Dialogue, c.dialogue)
		}
		result = append(result, exchange)
	}
	return result
}

func rpDialogueTerms(text string) map[string]bool {
	terms := map[string]bool{}
	var word []rune
	var previousHan rune
	flush := func() {
		if len(word) > 1 {
			terms[string(word)] = true
		}
		word = word[:0]
	}
	for _, r := range strings.ToLower(text) {
		if unicode.Is(unicode.Han, r) {
			flush()
			if previousHan != 0 {
				terms[string([]rune{previousHan, r})] = true
			}
			previousHan = r
		} else {
			previousHan = 0
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				word = append(word, r)
			} else {
				flush()
			}
		}
	}
	flush()
	return terms
}
