package narrative

import (
	"regexp"
	"strings"

	"corerp.local/backend/internal/core"
)

// A prose draft is presentation, never authority for the controlled actor.
// This guard targets explicit, consequential player actions in narration. It
// deliberately does not try to infer all Chinese semantics; the model prompt
// and fact-only rendering still carry the broader fidelity requirement.
var agencyClauseBreak = regexp.MustCompile(`[。！？；，：:\n]`)

var agencyPrefixes = []string{"就在这时", "片刻之后", "片刻后", "随后", "于是", "然后", "接着", "这时", "最终", "突然", "终于", "就这样"}

var agencyActions = []string{
	"走过去", "走向", "走到", "走进", "走出", "跑过去", "跑向", "来到", "离开", "跟着", "起身", "坐下", "坐到", "站起来", "靠过去", "靠在",
	"答应", "同意", "承诺", "保证", "签下", "签了", "接受邀请", "接受合同", "决定", "下定决心", "原谅", "爱上",
	"花了", "付了", "付款", "买下", "买了", "掏出", "拿出", "递给", "交给", "送给", "给了", "接过", "收下", "放下",
	"抱住", "拥抱", "搂住", "吻了", "亲了", "牵起", "握住", "伸手", "抚摸",
	"看着", "望向", "盯着", "注视", "打量", "打算", "不打算", "想要", "不想", "觉得", "心想", "不再",
}

var agencyMovementActions = map[string]bool{
	"走过去": true, "走向": true, "走到": true, "走进": true, "走出": true, "跑过去": true, "跑向": true,
	"来到": true, "离开": true, "跟着": true, "起身": true, "坐下": true, "坐到": true, "站起来": true, "靠过去": true,
}

var agencyModifiers = []string{"已经", "主动", "轻轻", "慢慢", "忽然", "突然", "马上", "直接", "终于", "终于还是", "便", "就", "也", "却", "还", "微微"}
var agencyHypotheticals = []string{"可以", "能够", "能不能", "要不要", "是否", "或许", "也许", "仿佛", "似乎", "没有", "未曾", "不曾", "并未", "不会"}
var agencySpeechVerbs = []string{"开口说道", "开口说", "开口问道", "开口问", "回答道", "回答说", "回答", "答道", "告诉", "说道", "说", "问道"}

func validateProsePlayerAgency(text string, in core.RPNarrativeInput) error {
	// Quoted spans were already checked against accepted utterances. A quoted
	// "你答应了" is a character's claim, not the narrator asserting consent.
	unquoted := stripProseQuotes(text)
	actors := []string{in.ControlledEntityID}
	for _, fact := range in.Facts {
		if fact.ActorID == in.ControlledEntityID && fact.ActorName != "" {
			actors = append(actors, fact.ActorName)
			break
		}
	}
	if in.Style.POV == "first_person" {
		actors = append(actors, "我")
	} else {
		actors = append(actors, "你")
	}
	for _, raw := range agencyClauseBreak.Split(unquoted, -1) {
		clause := strings.TrimSpace(raw)
		for _, prefix := range agencyPrefixes {
			if strings.HasPrefix(clause, prefix) {
				clause = strings.TrimSpace(strings.TrimPrefix(clause, prefix))
				break
			}
		}
	actorCheck:
		for _, actor := range actors {
			if actor == "" || !strings.HasPrefix(clause, actor) {
				continue
			}
			rest := strings.TrimSpace(strings.TrimPrefix(clause, actor))
			for i := 0; i < 2; i++ {
				removed := false
				for _, modifier := range agencyModifiers {
					if strings.HasPrefix(rest, modifier) {
						rest = strings.TrimSpace(strings.TrimPrefix(rest, modifier))
						removed = true
						break
					}
				}
				if !removed {
					break
				}
			}
			for _, hypothetical := range agencyHypotheticals {
				if strings.HasPrefix(rest, hypothetical) {
					continue actorCheck
				}
			}
			for _, action := range agencyActions {
				if strings.HasPrefix(rest, action) {
					if agencyMovementActions[action] && hasNarrativeMovementClaim(in, in.ControlledEntityID, action, strings.TrimPrefix(rest, action)) {
						continue actorCheck
					}
					return failure("uncommitted player agency in prose")
				}
			}
			for _, speech := range agencySpeechVerbs {
				if !strings.HasPrefix(rest, speech) {
					continue
				}
				tail := strings.TrimSpace(strings.TrimPrefix(rest, speech))
				if tail != "" && tail != "：" && tail != ":" {
					return failure("uncommitted player speech in prose")
				}
				break
			}
			if strings.HasPrefix(rest, "把") || strings.HasPrefix(rest, "将") {
				return failure("uncommitted player agency in prose")
			}
		}
	}
	return nil
}

// A place transition does not license a new posture, destination, or opposite
// transition. Both player and NPC narration use this same public-fact check.
func hasNarrativeMovementClaim(in core.RPNarrativeInput, actorID, verb, tail string) bool {
	tail = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(tail), "了"))
	tail = strings.TrimSpace(strings.TrimSuffix(tail, "了"))
	for _, fact := range in.Facts {
		if fact.EventID == "" || fact.ActorID != actorID {
			continue
		}
		arrival, departure := fact.Action == "arrive", fact.Action == "depart" || fact.Action == "leave"
		allowed := false
		switch verb {
		case "来到", "走进", "走到", "走向", "跑向":
			allowed = arrival
		case "离开", "走出":
			allowed = departure
		case "走过去", "跑过去":
			allowed = (arrival || departure) && tail == ""
		}
		if allowed && (tail == "" || fact.PlaceName != "" && (tail == fact.PlaceName || tail == "这里")) {
			return true
		}
	}
	return false
}

func stripProseQuotes(text string) string {
	quotes, err := quotedSpans(text)
	if err != nil {
		// validateProse rejects malformed quotation before invoking the claim
		// guards. A standalone guard must not hide text on a malformed input.
		return text
	}
	var out strings.Builder
	offset := 0
	for _, quote := range quotes {
		out.WriteString(text[offset:quote.start])
		offset = quote.end
	}
	out.WriteString(text[offset:])
	return out.String()
}
