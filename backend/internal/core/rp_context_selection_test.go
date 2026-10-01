package core

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func rpSelectionFixture() RPDecisionInput {
	return RPDecisionInput{ContextVersion: RPContextVersion, NPCEntityID: "npc", InterlocutorEntityID: "player", InstanceID: "world", BranchID: "main", HeadSequence: 77, WorldTime: "2026-09-22T00:02:00Z", PlaceID: "hall", Persona: "作者声明的温和长辈，习惯认真听完再回答。", PersonaSourceEventID: "identity", Readiness: RPContextReadiness{Persona: "READY", RelationshipToInterlocutor: "READY", AddressToInterlocutor: "READY"}, Relationships: []RPCharacterRelationship{{SubjectEntityID: "player", Role: "student", AddressTo: []string{"小林"}, SourceEventID: "identity"}, {SubjectEntityID: "other", Role: "colleague", SourceEventID: "identity"}}, LegalActions: []string{"respond", "silence", "wait"}, ReachablePlaceIDs: []string{"lane"}, PlayerSpeechText: "那本蓝皮诗集，你当时怎么答应我的？", SpeechEventID: "current", PlayerSpeechWorldTime: "2026-09-22T00:02:00Z"}
}

func TestRPContextSelectionKeepsPeerExchangeAndDatedSketchUnderPressure(t *testing.T) {
	in := rpSelectionFixture()
	in.PlayerSpeechText = "先前说好的那个分寸，还算数么？"
	question := RPDecisionDialogue{SpeakerEntityID: "player", Text: "我不想解释缘由，先陪我说一会儿话。", EventID: "peer-request", WorldTime: "2026-09-21T00:00:00Z"}
	reply := RPDecisionDialogue{SpeakerEntityID: "npc", Text: "好，不追问缘由。", EventID: "peer-response", WorldTime: "2026-09-21T00:01:00Z"}
	in.RecentDialogue = []RPDecisionDialogue{reply}
	in.RelevantDialogue = []RPDecisionExchange{{PeerContext: true, Dialogue: []RPDecisionDialogue{question, reply}}}
	in.RecentPrivateDecisions = []RPDecisionPrivateMemory{{DecisionID: "peer-private", SourceEventID: reply.EventID, InterlocutorEntityID: "player", WorldTime: reply.WorldTime, Private: RPDecisionPrivate{Intent: "当时答应不追问，未发生其他行动。"}}}
	baseline, err := SelectRPDecisionContext(in, DefaultRPDecisionContextBudgetBytes)
	if err != nil {
		t.Fatal(err)
	}
	budget := baseline.ContextSelection.EncodedBytes + 100
	for i := 0; i < 12; i++ {
		in.RecentDialogue = append(in.RecentDialogue, RPDecisionDialogue{SpeakerEntityID: "other", EventID: fmt.Sprintf("other-%d", i), Text: strings.Repeat("旁人的新话题。", 150), WorldTime: in.WorldTime})
	}
	in.RecentPrivateDecisions = append(in.RecentPrivateDecisions, RPDecisionPrivateMemory{DecisionID: "other-private", SourceEventID: "other-result", InterlocutorEntityID: "other", WorldTime: in.WorldTime, Private: RPDecisionPrivate{Intent: strings.Repeat("与另一个人的交谈愿望。", 200)}})
	got, err := SelectRPDecisionContext(in, budget)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RelevantDialogue) != 1 || !got.RelevantDialogue[0].PeerContext || len(got.RecentPrivateDecisions) != 1 || got.RecentPrivateDecisions[0].DecisionID != "peer-private" {
		t.Fatalf("current peer context lost to unrelated recent traffic: %+v", got)
	}
	for _, d := range []RPDecisionDialogue{question, reply} {
		if words, ok := ResolveRPDecisionSpeech(got, d.EventID, d.SpeakerEntityID); !ok || words != d.Text {
			t.Fatal("retained peer group lost its exact authorized speech")
		}
	}
	for _, d := range got.RecentDialogue {
		if d.EventID == reply.EventID {
			t.Fatal("one retained peer member was also exposed as a standalone recent exchange")
		}
	}
	again, err := SelectRPDecisionContext(got, budget)
	a, _ := HashJSON(got)
	b, _ := HashJSON(again)
	if err != nil || a != b {
		t.Fatal("peer selection changed across repeated provider projection", err)
	}
}

func TestRPContextSelectionRestoresRecentExchangeAtomically(t *testing.T) {
	in := rpSelectionFixture()
	question := RPDecisionDialogue{SpeakerEntityID: "player", Text: "蓝皮诗集的事，只能向我说，不要告诉其他人。", EventID: "qualified-question", WorldTime: in.WorldTime}
	reply := RPDecisionDialogue{SpeakerEntityID: "npc", Text: "好，我记住这个条件。", EventID: "recent-answer", WorldTime: in.WorldTime}
	in.RecentDialogue = []RPDecisionDialogue{reply}
	in.RelevantDialogue = []RPDecisionExchange{{RecentContext: true, Dialogue: []RPDecisionDialogue{question, reply}}}
	before, _ := HashJSON(in)
	complete, err := SelectRPDecisionContext(in, DefaultRPDecisionContextBudgetBytes)
	if err != nil {
		t.Fatal(err)
	}
	budget := complete.ContextSelection.EncodedBytes + 64
	for i := 0; i < 20; i++ {
		in.RecentDialogue = append(in.RecentDialogue, RPDecisionDialogue{SpeakerEntityID: "other", Text: strings.Repeat("旁人的闲谈。", 500), EventID: fmt.Sprintf("crowded-%d", i), WorldTime: in.WorldTime})
	}
	got, err := SelectRPDecisionContext(in, budget)
	if err != nil || len(got.RelevantDialogue) != 1 || !got.RelevantDialogue[0].RecentContext {
		t.Fatalf("complete recent context did not survive pressure: %+v %v", got.RelevantDialogue, err)
	}
	for _, expected := range []RPDecisionDialogue{question, reply} {
		text, ok := ResolveRPDecisionSpeech(got, expected.EventID, expected.SpeakerEntityID)
		if !ok || text != expected.Text {
			t.Fatal("restored exchange lost its original attribution/words")
		}
	}
	for _, d := range got.RecentDialogue {
		if d.EventID == reply.EventID {
			t.Fatal("restored group was also offered as an independent recent member")
		}
	}
	repeated, err := SelectRPDecisionContext(got, budget)
	a, _ := HashJSON(got)
	b, _ := HashJSON(repeated)
	if err != nil || a != b {
		t.Fatal("second provider projection lost group metadata or changed selection", err)
	}
	base := rpSelectionFixture()
	baseView, err := SelectRPDecisionContext(base, DefaultRPDecisionContextBudgetBytes)
	if err != nil {
		t.Fatal(err)
	}
	minimal := baseView.ContextSelection.EncodedBytes + 64
	omitted, err := SelectRPDecisionContext(in, minimal)
	if err != nil || len(omitted.RelevantDialogue) != 0 || len(omitted.RecentDialogue) != 0 || omitted.ContextSelection.Omitted.RelevantExchanges != 1 {
		t.Fatalf("oversize group became an unexplained partial exchange: %+v %v", omitted.ContextSelection, err)
	}
	in.RecentDialogue = []RPDecisionDialogue{reply}
	after, _ := HashJSON(in)
	if after != before {
		t.Fatal("selection mutated the source packet")
	}
}

func TestRPContextSelectionDeduplicatesBodiesWithoutLosingMetadata(t *testing.T) {
	in := rpSelectionFixture()
	const reply = "蓝皮诗集我会带来，不过末页还要核对。"
	in.RecentDialogue = []RPDecisionDialogue{{SpeakerEntityID: "npc", Text: reply, EventID: "answer", WorldTime: "2026-09-22T00:01:00Z"}, {SpeakerEntityID: "player", Text: in.PlayerSpeechText, EventID: in.SpeechEventID, WorldTime: in.PlayerSpeechWorldTime}}
	in.Knowledge = []RPDecisionKnowledge{{ClaimType: "speaker_said", SubjectEntityID: "npc", Text: reply, SourceEventID: "answer"}, {ClaimType: "speaker_said", SubjectEntityID: "player", Text: in.PlayerSpeechText, SourceEventID: "current"}, {ClaimType: "agent_presence", SubjectEntityID: "other", PlaceID: "hall", SourceEventID: "identity"}}
	in.HeardPlayerHistory = []RPDecisionSpeechExcerpt{{Excerpt: in.PlayerSpeechText, EventID: "current", WorldTime: in.PlayerSpeechWorldTime}}
	in.OwnActions = []RPOwnAction{{EventID: "answer", Action: "speech", Text: reply, PlaceID: "former-room", WorldTime: "2026-09-22T00:01:00Z"}}
	before, _ := HashJSON(in)
	got, err := SelectRPDecisionContext(in, DefaultRPDecisionContextBudgetBytes)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(got)
	if strings.Count(string(encoded), reply) != 1 || strings.Count(string(encoded), in.PlayerSpeechText) != 1 {
		t.Fatal("same accepted words repeated in packet", string(encoded))
	}
	if len(got.OwnActions) != 1 || got.OwnActions[0].PlaceID != "former-room" || !got.OwnActions[0].TextFromEvent {
		t.Fatal("dedup discarded where the utterance actually happened", got.OwnActions)
	}
	if !got.RecentDialogue[1].TextFromEvent || got.RecentDialogue[1].WorldTime != in.PlayerSpeechWorldTime || !got.Knowledge[0].TextFromEvent {
		t.Fatal("source/time/reference lost", got)
	}
	if len(got.Relationships) != 2 || len(got.Knowledge) != 3 {
		t.Fatal("shared declaration Event erased distinct facts")
	}
	after, _ := HashJSON(in)
	if before != after {
		t.Fatal("selection mutated input")
	}
}

func TestRPContextSelectionBudgetPreservesCurrentRoleAndRelevantWholeExchange(t *testing.T) {
	in := rpSelectionFixture()
	in.RecentDialogue = []RPDecisionDialogue{{SpeakerEntityID: "npc", Text: "上次我答应核对缺页，你记得没错。", EventID: "recent-answer", WorldTime: in.WorldTime}}
	in.RelevantDialogue = []RPDecisionExchange{{Dialogue: []RPDecisionDialogue{{SpeakerEntityID: "player", Text: "明天请把蓝皮诗集带来。", EventID: "old-question", WorldTime: "2026-09-21T00:00:00Z"}, {SpeakerEntityID: "npc", Text: "我答应带来，只是还缺末页。", EventID: "old-answer", WorldTime: "2026-09-21T00:01:00Z"}}}}
	in.RecentPrivateDecisions = []RPDecisionPrivateMemory{{DecisionID: "d", SourceEventID: "recent-answer", Private: RPDecisionPrivate{Intent: "私下想提醒缺页，尚未递交诗集。"}}}
	for i := 0; i < 40; i++ {
		in.OwnActions = append(in.OwnActions, RPOwnAction{EventID: fmt.Sprintf("other-%d", i), Action: "speech", Text: strings.Repeat("很长而且无关的话", 120), PlaceID: "hall", WorldTime: in.WorldTime})
	}
	const budget = 5000
	got, err := SelectRPDecisionContext(in, budget)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(got)
	if len(encoded) > budget {
		t.Fatalf("unified budget not enforced: %d > %d", len(encoded), budget)
	}
	if got.Persona != in.Persona || got.PlayerSpeechText != in.PlayerSpeechText || !reflect.DeepEqual(got.Relationships, in.Relationships) || !reflect.DeepEqual(got.Readiness, in.Readiness) || !reflect.DeepEqual(got.ReachablePlaceIDs, in.ReachablePlaceIDs) {
		t.Fatal("essential role/state or options trimmed")
	}
	if !reflect.DeepEqual(got.RelevantDialogue, in.RelevantDialogue) || len(got.RecentPrivateDecisions) != 1 || len(got.OwnActions) >= 40 {
		t.Fatal("old noise displaced complete relevant exchange/private continuity", got)
	}
	if got.ContextSelection == nil || got.ContextSelection.EncodedBytes != len(encoded) || got.ContextSelection.Omitted.OwnActions == 0 {
		t.Fatal("omission/actual size not explicit", got.ContextSelection)
	}
	again, err := SelectRPDecisionContext(in, budget)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := HashJSON(got)
	b, _ := HashJSON(again)
	if a != b {
		t.Fatal("selection changed for same packet")
	}
}

func TestRPContextSelectionMandatoryOverflowDoesNotTruncateCanon(t *testing.T) {
	in := rpSelectionFixture()
	in.Persona = strings.Repeat("作者设定", 1000)
	if _, err := SelectRPDecisionContext(in, 2000); err == nil {
		t.Fatal("oversize essential canon silently accepted or truncated")
	}
}

func TestRPContextSelectionRejectsDanglingOrConflictingQuoteRefs(t *testing.T) {
	in := rpSelectionFixture()
	in.RecentDialogue = []RPDecisionDialogue{{SpeakerEntityID: "npc", EventID: "missing", TextFromEvent: true, WorldTime: in.WorldTime}}
	if _, err := SelectRPDecisionContext(in, DefaultRPDecisionContextBudgetBytes); err == nil {
		t.Fatal("dangling reference accepted")
	}
	in.RecentDialogue = []RPDecisionDialogue{{SpeakerEntityID: "npc", EventID: "one", Text: "原话甲", WorldTime: in.WorldTime}}
	in.Knowledge = []RPDecisionKnowledge{{ClaimType: "speaker_said", SubjectEntityID: "npc", SourceEventID: "one", Text: "原话乙"}}
	if _, err := SelectRPDecisionContext(in, DefaultRPDecisionContextBudgetBytes); err == nil {
		t.Fatal("same Event with conflicting original words accepted")
	}
}

func TestRPContextSelectionDoesNotMergePrivateOrDifferentEvents(t *testing.T) {
	in := rpSelectionFixture()
	const words = "我会等你把话说完。"
	in.RecentDialogue = []RPDecisionDialogue{{SpeakerEntityID: "npc", Text: words, EventID: "a", WorldTime: in.WorldTime}, {SpeakerEntityID: "npc", Text: words, EventID: "b", WorldTime: in.WorldTime}}
	in.RecentPrivateDecisions = []RPDecisionPrivateMemory{{DecisionID: "d", SourceEventID: "a", Private: RPDecisionPrivate{Intent: words}}}
	got, err := SelectRPDecisionContext(in, DefaultRPDecisionContextBudgetBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RecentDialogue) != 2 || got.RecentDialogue[0].Text != words || got.RecentDialogue[1].Text != words || !reflect.DeepEqual(got.RecentPrivateDecisions, in.RecentPrivateDecisions) || len(got.Relationships) != 2 {
		t.Fatal("equal wording/shared Event merged distinct facts or private", got)
	}
}

func TestRPContextSelectionSharedSourceKeepsDifferentSpeakers(t *testing.T) {
	in := rpSelectionFixture()
	in.Knowledge = []RPDecisionKnowledge{{ClaimType: "speaker_said", SubjectEntityID: "first", SourceEventID: "declared-dialogue", Text: "第一人曾说这句话。"}, {ClaimType: "speaker_said", SubjectEntityID: "second", SourceEventID: "declared-dialogue", Text: "第二人曾说另一句。"}}
	got, err := SelectRPDecisionContext(in, DefaultRPDecisionContextBudgetBytes)
	if err != nil || !reflect.DeepEqual(got.Knowledge, in.Knowledge) {
		t.Fatal("shared provenance merged distinct speakers", got, err)
	}
}

func TestRPContextSelectionKeepsExplicitExcerptWhenFullWordsDoNotFit(t *testing.T) {
	in := rpSelectionFixture()
	full := "我那时候说的话，" + strings.Repeat("很长的原话", 900)
	in.Knowledge = []RPDecisionKnowledge{{ClaimType: "speaker_said", SubjectEntityID: "player", SourceEventID: "long", Text: full}}
	in.HeardPlayerHistory = []RPDecisionSpeechExcerpt{{EventID: "long", Excerpt: "我那时候说的话，…", Truncated: true, WorldTime: in.WorldTime}}
	got, err := SelectRPDecisionContext(in, 2500)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.HeardPlayerHistory) != 1 || !got.HeardPlayerHistory[0].Truncated || got.HeardPlayerHistory[0].TextFromEvent || got.HeardPlayerHistory[0].Excerpt != in.HeardPlayerHistory[0].Excerpt {
		t.Fatal("partial excerpt disappeared or became a fake whole quote", got)
	}
}

func TestRPContextSelectionRepeatedProjectionIsStableAndReferencesStayComplete(t *testing.T) {
	in := rpSelectionFixture()
	in.RecentDialogue = []RPDecisionDialogue{{EventID: "reply", SpeakerEntityID: "npc", Text: "先把缺页说清楚，再决定。", WorldTime: in.WorldTime}}
	in.OwnActions = []RPOwnAction{{EventID: "reply", Action: "speech", Text: in.RecentDialogue[0].Text, PlaceID: "former-place", WorldTime: in.WorldTime}}
	first, err := SelectRPDecisionContext(in, DefaultRPDecisionContextBudgetBytes)
	if err != nil {
		t.Fatal(err)
	}
	second, err := SelectRPDecisionContext(first, DefaultRPDecisionContextBudgetBytes)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := HashJSON(first)
	b, _ := HashJSON(second)
	if a != b {
		t.Fatal("repeat projection changed packet/hash", first.ContextSelection, second.ContextSelection)
	}
	if !second.OwnActions[0].TextFromEvent || second.RecentDialogue[0].Text == "" {
		t.Fatal("reference lost its complete attributed words")
	}
	third, err := SelectRPDecisionContext(second, 1800)
	if err != nil {
		t.Fatal(err)
	}
	if len(third.OwnActions) > 0 && third.OwnActions[0].TextFromEvent && len(third.RecentDialogue) == 0 {
		t.Fatal("budget reduction left a dangling reference")
	}
}

func TestRPContextSelectionLifeSpeechSharesWordsButKeepsOwnerState(t *testing.T) {
	in := rpSelectionFixture()
	const reply = "诗集缺了末页，我还没有把它带来。"
	work := RPLifeMemory{Kind: "work_completed", SubjectEntityID: "npc", Text: "已完成作者设定的工作。", SourceEventID: "work", WorldTime: in.WorldTime}
	in.RecentDialogue = []RPDecisionDialogue{{EventID: "reply", SpeakerEntityID: "npc", Text: reply, WorldTime: in.WorldTime}}
	in.Life = &RPLifeContext{
		SalientMemories: []RPLifeMemory{{Kind: "speaker_said", SubjectEntityID: "npc", Text: reply, SourceEventID: "reply", WorldTime: in.WorldTime}, work},
		RecentWork:      []RPLifeMemory{work},
		Employment:      []RPOwnEmployment{{ContractID: "contract", SourceEventID: "employment", WageMinor: 37}},
	}
	before, _ := HashJSON(in)
	got, err := SelectRPDecisionContext(in, DefaultRPDecisionContextBudgetBytes)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(got)
	if strings.Count(string(encoded), reply) != 1 || len(got.Life.SalientMemories) != 2 || !got.Life.SalientMemories[0].TextFromEvent || got.Life.SalientMemories[0].WorldTime != in.WorldTime {
		t.Fatal("life speech was repeated or lost its observable source/time", got.Life)
	}
	if got.Life.SalientMemories[1] != work || !reflect.DeepEqual(got.Life.RecentWork, in.Life.RecentWork) || !reflect.DeepEqual(got.Life.Employment, in.Life.Employment) {
		t.Fatal("dedup modified owner-critical non-speech state", got.Life)
	}
	words, complete := ResolveRPDecisionSpeech(got, "reply", "npc")
	if !complete || words != reply {
		t.Fatal("life reference did not resolve to the authorized complete words")
	}
	after, _ := HashJSON(in)
	if before != after {
		t.Fatal("life selection mutated input")
	}
	in.Life.SalientMemories[1].TextFromEvent = true
	if _, err := SelectRPDecisionContext(in, DefaultRPDecisionContextBudgetBytes); err == nil {
		t.Fatal("non-speech work fact accepted an utterance reference")
	}
}

func TestRPContextSelectionSharedDeclarationKeepsDistinctStatementsBySameSpeaker(t *testing.T) {
	in := rpSelectionFixture()
	in.Knowledge = []RPDecisionKnowledge{
		{ClaimType: "speaker_said", SubjectEntityID: "other", SourceEventID: "declared-dialogue", Text: "第一条声明的原话。"},
		{ClaimType: "speaker_said", SubjectEntityID: "other", SourceEventID: "declared-dialogue", Text: "同一声明里的另一段原话。"},
	}
	got, err := SelectRPDecisionContext(in, DefaultRPDecisionContextBudgetBytes)
	if err != nil || !reflect.DeepEqual(got.Knowledge, in.Knowledge) {
		t.Fatal("provenance alone merged different declared statements", got, err)
	}
	if _, complete := ResolveRPDecisionSpeech(got, "declared-dialogue", "other"); complete {
		t.Fatal("ambiguous declaration was treated as one complete utterance")
	}
}
