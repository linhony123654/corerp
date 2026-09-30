package narrative

import (
	"regexp"
	"strings"

	"corerp.local/backend/internal/core"
)

var proseMoney = regexp.MustCompile(`(?:[0-9]+|[零一二三四五六七八九十百千万两]+)(?:元|块钱|金币|银币)`)
var proseTimeLeap = regexp.MustCompile(`(?:过了|过去了|转眼过了)(?:[0-9]+|[零一二三四五六七八九十百千万两]+)(?:分钟|小时|天|日)`)
var proseNewObject = regexp.MustCompile(`(?:桌上|地上|手里|口袋里|房间里)(?:忽然|突然)?(?:出现|多出|凭空多出|摆着|放着)\S+`)
var proseNewPerson = regexp.MustCompile(`(?:来了一|出现一|走进一)(?:名|位|个)(?:服务员|店员|行人|陌生人|客人)`)
var proseLatinName = regexp.MustCompile(`[A-Z][a-z]{1,}`)
var proseRelationship = regexp.MustCompile(`(?:你们|两人)(?:就这样|从此|终于)?(?:成了恋人|确定了关系|结为伴侣|结了婚)`)
var proseGazeAtPlayer = regexp.MustCompile(`(?:看着|望向|注视着?|打量着?)(?:你|我)`)
var proseSentenceBreak = regexp.MustCompile(`[。！？；\n]`)
var proseCommaBreak = regexp.MustCompile(`[，,：:]`)

// The model receives only committed fact rows. These checks veto common
// explicit extra scene facts even when the draft preserves every quotation.
// They are a bounded safety net, not a full natural-language proof system.
func validateProseSceneClaims(text string, in core.RPNarrativeInput) error {
	unquoted := stripProseQuotes(text)
	// A directed look is an observable NPC beat, including when the actor is
	// in the preceding comma-delimited clause (e.g. "她听后，看着你"). The
	// current typed expression vocabulary has no gaze event to license it.
	if proseGazeAtPlayer.MatchString(unquoted) {
		return failure("uncommitted gaze in prose")
	}
	// The accepted speech can mention a gesture without making it true. In
	// narration, even small expressions require a witnessed committed event.
	for code, forms := range map[string][]string{
		"smile": {"笑了笑", "微笑"}, "nod": {"点头", "点了点头"},
		"shake_head": {"摇头", "摇了摇头"}, "frown": {"皱眉", "皱了皱眉"},
		"beckon": {"招手", "招了招手"}, "turn_away": {"转过身", "转身"},
	} {
		for _, sentence := range proseSentenceBreak.Split(unquoted, -1) {
			carriedActor := ""
			for _, clause := range proseCommaBreak.Split(sentence, -1) {
				if actorID, explicit := proseLeadingExpressionActor(clause, in); explicit {
					carriedActor = actorID
				}
				for _, form := range forms {
					for start := 0; start < len(clause); {
						index := strings.Index(clause[start:], form)
						if index < 0 {
							break
						}
						start += index
						if carriedActor == "" || proseExpressionMentionsOtherActor(clause[:start], in, carriedActor) || !hasNarrativeExpression(in, code, carriedActor) {
							return failure("uncommitted expression in prose")
						}
						if target, directed := proseExpressionRecipient(clause[:start], in, carriedActor); directed && (target == "" || !hasNarrativeExpressionTarget(in, code, carriedActor, target)) {
							return failure("uncommitted expression target in prose")
						}
						start += len(form)
					}
				}
			}
		}
	}
	if proseMoney.MatchString(unquoted) && !hasNarrativeFactAction(in, "money") {
		return failure("uncommitted money claim in prose")
	}
	// NPC wait records a stance, not a clock advance. The current public
	// projection has timestamps but no sourced elapsed-duration claim.
	if proseTimeLeap.MatchString(unquoted) {
		return failure("uncommitted time change in prose")
	}
	if proseNewObject.MatchString(unquoted) && !hasNarrativeFactAction(in, "object") {
		return failure("uncommitted object in prose")
	}
	if proseNewPerson.MatchString(unquoted) && !hasNarrativeFactAction(in, "arrive") {
		return failure("uncommitted person in prose")
	}
	if proseRelationship.MatchString(unquoted) && !hasNarrativeFactAction(in, "social") {
		return failure("uncommitted relationship in prose")
	}
	allowedNames := map[string]bool{}
	for _, fact := range in.Facts {
		for _, match := range proseLatinName.FindAllString(fact.ActorName+" "+fact.TargetActorName+" "+fact.PlaceName, -1) {
			allowedNames[match] = true
		}
	}
	for _, candidate := range proseLatinName.FindAllString(unquoted, -1) {
		if !allowedNames[candidate] {
			return failure("uncommitted name in prose")
		}
	}
	actors := map[string]bool{"她": true, "他": true, "陌生人": true}
	for _, fact := range in.Facts {
		if fact.ActorID != in.ControlledEntityID && fact.ActorName != "" {
			actors[fact.ActorName] = true
		}
	}
	for _, raw := range agencyClauseBreak.Split(unquoted, -1) {
		clause := strings.TrimSpace(raw)
		for _, prefix := range agencyPrefixes {
			if strings.HasPrefix(clause, prefix) {
				clause = strings.TrimSpace(strings.TrimPrefix(clause, prefix))
				break
			}
		}
		for actor := range actors {
			if !strings.HasPrefix(clause, actor) {
				continue
			}
			rest := strings.TrimSpace(strings.TrimPrefix(clause, actor))
			for _, action := range agencyActions {
				if strings.HasPrefix(rest, action) {
					if agencyMovementActions[action] && hasNPCMovementFact(in, actor, action, strings.TrimPrefix(rest, action)) {
						continue
					}
					return failure("uncommitted NPC action in prose")
				}
			}
			if strings.HasPrefix(rest, "把") || strings.HasPrefix(rest, "将") {
				return failure("uncommitted NPC action in prose")
			}
		}
	}
	return nil
}

// A gesture's subject is the clause's leading actor, or an unambiguous actor
// carried across a comma in the same sentence. Names in objects such as
// "对你点头" are never treated as subjects. A pronoun referring to multiple
// NPCs clears the carry and stays unlicensed.
func proseLeadingExpressionActor(clause string, in core.RPNarrativeInput) (string, bool) {
	type candidate struct{ name, id string }
	actors := make([]candidate, 0, len(in.Facts)+3)
	npcID := ""
	multipleNPCs := false
	for _, fact := range in.Facts {
		if fact.ActorID == "" {
			continue
		}
		if fact.ActorName != "" {
			actors = append(actors, candidate{fact.ActorName, fact.ActorID})
		}
		if fact.ActorID != in.ControlledEntityID {
			if npcID != "" && npcID != fact.ActorID {
				multipleNPCs = true
			}
			npcID = fact.ActorID
		}
	}
	if in.Style.POV == "first_person" {
		actors = append(actors, candidate{"我", in.ControlledEntityID})
	} else {
		actors = append(actors, candidate{"你", in.ControlledEntityID})
	}
	pronounID := ""
	if !multipleNPCs {
		pronounID = npcID
	}
	actors = append(actors, candidate{"她", pronounID}, candidate{"他", pronounID})
	clause = strings.TrimSpace(clause)
	for _, intro := range agencyPrefixes {
		if strings.HasPrefix(clause, intro) {
			clause = strings.TrimSpace(strings.TrimPrefix(clause, intro))
			break
		}
	}
	best, actorID := 0, ""
	for _, actor := range actors {
		if actor.name == "" || !strings.HasPrefix(clause, actor.name) {
			continue
		}
		if len(actor.name) > best {
			best, actorID = len(actor.name), actor.id
		} else if len(actor.name) == best && actor.id != actorID {
			actorID = "" // Several anonymous actors share the same label.
		}
	}
	return actorID, best > 0
}

// Another named actor inside the verb phrase makes attribution uncertain,
// except when that name is explicitly the recipient ("对宝钗点头"). This
// keeps a carried subject from licensing somebody else's gesture.
func proseExpressionMentionsOtherActor(prefix string, in core.RPNarrativeInput, subjectID string) bool {
	type actor struct{ name, id string }
	others := make([]actor, 0, len(in.Facts)+1)
	for _, fact := range in.Facts {
		if fact.ActorID != subjectID && fact.ActorName != "" {
			others = append(others, actor{fact.ActorName, fact.ActorID})
		}
	}
	playerPronoun := "你"
	if in.Style.POV == "first_person" {
		playerPronoun = "我"
	}
	if subjectID != in.ControlledEntityID {
		others = append(others, actor{playerPronoun, in.ControlledEntityID})
	}
	for _, other := range others {
		for offset := 0; offset < len(prefix); {
			index := strings.Index(prefix[offset:], other.name)
			if index < 0 {
				break
			}
			index += offset
			before := prefix[:index]
			if !proseRecipientPrefix(before) {
				return true
			}
			offset = index + len(other.name)
		}
	}
	return false
}

func hasNarrativeExpression(in core.RPNarrativeInput, code, actorID string) bool {
	for _, fact := range in.Facts {
		if fact.Action == "expression" && fact.ExpressionCode == code && fact.ActorID == actorID && fact.EventID != "" {
			return true
		}
	}
	return false
}

func hasNarrativeExpressionTarget(in core.RPNarrativeInput, code, actorID, targetID string) bool {
	for _, fact := range in.Facts {
		if fact.EventID != "" && fact.Action == "expression" && fact.ExpressionCode == code && fact.ActorID == actorID && fact.TargetActorID == targetID {
			return true
		}
	}
	return false
}

var proseRecipientMarkers = []string{"对着", "向着", "朝着", "冲着", "对", "向", "朝", "给", "冲"}

func proseRecipientPrefix(before string) bool {
	before = strings.TrimSpace(before)
	for _, marker := range proseRecipientMarkers {
		if strings.HasSuffix(before, marker) {
			return true
		}
	}
	return false
}

// Resolve explicit recipients against public identities, including target-only
// identities. An absent, hidden or ambiguous recipient cannot become a new
// interaction target. This covers bounded verb phrases, not all Chinese syntax.
func proseExpressionRecipient(prefix string, in core.RPNarrativeInput, subjectID string) (string, bool) {
	prefix = strings.TrimSpace(prefix)
	for _, intro := range agencyPrefixes {
		prefix = strings.TrimSpace(strings.TrimPrefix(prefix, intro))
	}
	leading := ""
	for _, fact := range in.Facts {
		if fact.ActorID == subjectID && fact.ActorName != "" && strings.HasPrefix(prefix, fact.ActorName) && len(fact.ActorName) > len(leading) {
			leading = fact.ActorName
		}
	}
	prefix = strings.TrimSpace(strings.TrimPrefix(prefix, leading))
	start, markerLength := -1, 0
	for _, marker := range proseRecipientMarkers {
		if index := strings.LastIndex(prefix, marker); index > start || index == start && len(marker) > markerLength {
			start, markerLength = index, len(marker)
		}
	}
	if start < 0 {
		return "", false
	}
	recipient := strings.TrimSpace(prefix[start+markerLength:])
	type candidate struct{ name, id string }
	var actors []candidate
	npcIDs := map[string]bool{}
	for _, fact := range in.Facts {
		actors = append(actors, candidate{fact.ActorName, fact.ActorID}, candidate{fact.TargetActorName, fact.TargetActorID})
		if fact.ActorID != "" && fact.ActorID != in.ControlledEntityID {
			npcIDs[fact.ActorID] = true
		}
	}
	playerPronoun := "你"
	if in.Style.POV == "first_person" {
		playerPronoun = "我"
	}
	actors = append(actors, candidate{playerPronoun, in.ControlledEntityID})
	if len(npcIDs) == 1 {
		for id := range npcIDs {
			actors = append(actors, candidate{"他", id}, candidate{"她", id})
		}
	}
	best, target := 0, ""
	for _, actor := range actors {
		if actor.name == "" || actor.id == "" || !strings.HasPrefix(recipient, actor.name) || strings.HasPrefix(strings.TrimPrefix(recipient, actor.name), "们") {
			continue
		}
		if len(actor.name) > best {
			best, target = len(actor.name), actor.id
		} else if len(actor.name) == best && target != actor.id {
			target = ""
		}
	}
	return target, true
}

func hasNPCMovementFact(in core.RPNarrativeInput, actorName, verb, tail string) bool {
	if actorName == "她" || actorName == "他" || actorName == "陌生人" {
		return false // Pronouns and anonymized names may refer to several actors.
	}
	actorID := ""
	for _, fact := range in.Facts {
		if fact.ActorID != in.ControlledEntityID && fact.ActorName == actorName {
			if actorID != "" && actorID != fact.ActorID {
				return false
			}
			actorID = fact.ActorID
		}
	}
	return actorID != "" && hasNarrativeMovementClaim(in, actorID, verb, tail)
}

func hasNarrativeFactAction(in core.RPNarrativeInput, actions ...string) bool {
	for _, fact := range in.Facts {
		for _, action := range actions {
			if fact.Action == action {
				return true
			}
		}
	}
	return false
}
