package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

const RPFactCompositionVersionV3 = "corerp.fact-composition.v3"
const RPFactCompositionVersionV2 = "corerp.fact-composition.v2"
const RPFactCompositionVersionV1 = "corerp.fact-composition.v1"

// Plan fields select a finite grammar; every actor, target and quote is expanded
// from the frozen public input. No model-written prose is an accepted field.
type RPCompositionPlan struct {
	Version    string                   `json:"version"`
	Register   string                   `json:"register"`
	Paragraphs []RPCompositionParagraph `json:"paragraphs"`
}
type RPCompositionParagraph struct {
	Context string              `json:"context"`
	Beats   []RPCompositionBeat `json:"beats"`
}
type RPCompositionBeat struct {
	FactRefs []string `json:"fact_refs"`
	Form     string   `json:"form"`
	Lexical  string   `json:"lexical"`
}
type RPNarrativeArtifact struct {
	Input       RPNarrativeInput  `json:"input"`
	Plan        RPCompositionPlan `json:"plan"`
	InputSHA256 string            `json:"input_sha256"`
}

// Only unsupported arbitrary lexical/mental instructions are disclosed; v2's
// source-entailing diction and grouping no longer need the v1 capability wall.
const RPCompositionInstructionWarning = "已应用可支持的事实叙述与排版；自定义要求仅用于这些选择，未改写原话或补写心理、语气及未确认的情节。"
const RPCompositionInstruction = `只输出 schema 指定版本的有限组合 JSON：无已记录语气时为 corerp.fact-composition.v2，有已记录语气时为 corerp.fact-composition.v3。下例的 version 必须替换为 schema 指定版本。结构为 {"version":"corerp.fact-composition.v2","register":"plain","paragraphs":[{"context":"none","beats":[{"fact_refs":["f0"],"form":"subject_first","lexical":"plain"}]}]}。
按原始顺序覆盖所有事实，每个 fact_ref 恰好一次。只能选择 eligible_beats 给出的 fact_refs/form；register 默认 plain，只有明确的全局古典风格要求才可选择 classical，lexical 为 plain/varied，context 为 none/scene。scene 只能引入该段已给出的地点，同地点不重复开场。
引用后的姓名、对象、原话、动作由服务器展开；不得增加字段或任何正文。quote_first 调整引语位置，subject_first 主语在前，speech_gesture_quote_first/speech_gesture_subject_first 只合并已经证明的相邻对白和表情，不表示同时发生；action_quote 连接同主体相邻到达和对白。服务器仅依据 speech_tone 展开已记录的可听语气；不得自行添加或推断语气，不得从私密情绪、心理或公开表达线索推导语气。不得写笑着说、轻声、慈爱、等待多久或心理。公开表达线索只选择已支持的叙述措辞/节奏，不改变对白、不生成新事实。`

// LiteralRPNarrativeProvider retains the explicit old row renderer for v1
// compilation and audit/legacy compatibility. Play defaults use v2 below.
type LiteralRPNarrativeProvider struct{}

func rpCompositionError(message string) error { return NewError(CodeInvalidArgument, message) }
func rpFactRef(i int) string                  { return "f" + strconv.Itoa(i) }
func rpSpeech(f RPNarrativeFact) bool {
	return f.Action == "speak" || f.Action == "respond" || f.Action == "refuse"
}

// RPCompositionVersion selects v3 only when a frozen public utterance records delivery.
func RPCompositionVersion(in RPNarrativeInput) string {
	for _, f := range in.Facts {
		if f.SpeechTone != "" {
			return RPFactCompositionVersionV3
		}
	}
	return RPFactCompositionVersionV2
}

func validateRPCompositionInput(in RPNarrativeInput) error {
	if err := in.Style.Validate(); err != nil {
		return err
	}
	if in.SourceHead < 0 {
		return NewError(CodeProjectionDiverged, "invalid public source head")
	}
	seen := map[string]bool{}
	for i, f := range in.Facts {
		if f.EventID == "" || seen[f.EventID] {
			return NewError(CodeProjectionDiverged, "composition requires unique public sources")
		}
		seen[f.EventID] = true
		if !ValidRPSpeechTone(f.SpeechTone) || (f.SpeechTone != "" && !rpSpeech(f)) {
			return NewError(CodeProjectionDiverged, "invalid public speech delivery")
		}
		for _, text := range []string{f.ActorName, f.TargetActorName, f.Text, f.PlaceName, f.ActivityLabel, f.ObjectName, f.ObjectState} {
			if !utf8.ValidString(text) {
				return NewError(CodeProjectionDiverged, "invalid public fact text")
			}
		}
		if f.TargetActorID != "" && f.TargetActorName == "" {
			return NewError(CodeProjectionDiverged, "public target lacks identity label")
		}
		single := in
		single.Facts = []RPNarrativeFact{f}
		single.Style = DefaultRPStyle()
		if _, err := (LiteralRPNarrativeProvider{}).Render(context.Background(), single); err != nil {
			return err
		}
		if f.CompanionEventID != "" {
			if i == 0 || f.Action != "expression" || !rpSpeech(in.Facts[i-1]) || in.Facts[i-1].EventID != f.CompanionEventID || f.ActorID == "" || f.ActorID != in.Facts[i-1].ActorID {
				return NewError(CodeProjectionDiverged, "invalid public speech companion")
			}
		}
	}
	if len(in.PublicPresentations) > 16 {
		return NewError(CodeProjectionDiverged, "too many public presentation cues")
	}
	for _, cue := range in.PublicPresentations {
		found := false
		for _, f := range in.Facts {
			if f.ActorID == cue.ActorID && f.ActorName == cue.ActorName {
				found = true
			}
		}
		if !found || cue.ActorID == "" || cue.ActorID == in.ControlledEntityID || cue.SourceEventID == "" || cue.Text == "" || !utf8.ValidString(cue.Text) || utf8.RuneCountInString(cue.Text) > 500 || strings.ContainsAny(cue.Text, "\x00\r") {
			return NewError(CodeProjectionDiverged, "invalid authored public presentation")
		}
	}
	return nil
}

// RPCompositionChoices exposes eligible public-local ref tuples and forms. It
// contains no raw event/batch/command IDs or inferred temporal relation.
func RPCompositionChoices(in RPNarrativeInput) []map[string]any {
	choices := []map[string]any{}
	for i, f := range in.Facts {
		forms := []string{"action"}
		if rpSpeech(f) {
			forms = []string{"subject_first", "quote_first"}
		}
		choices = append(choices, map[string]any{"fact_refs": []string{rpFactRef(i)}, "forms": forms})
		if i+1 < len(in.Facts) && rpCompanion(in.Facts[i], in.Facts[i+1]) {
			choices = append(choices, map[string]any{"fact_refs": []string{rpFactRef(i), rpFactRef(i + 1)}, "forms": []string{"speech_gesture_quote_first", "speech_gesture_subject_first"}})
		}
		if i+1 < len(in.Facts) && rpActionQuote(in.Facts[i], in.Facts[i+1]) {
			choices = append(choices, map[string]any{"fact_refs": []string{rpFactRef(i), rpFactRef(i + 1)}, "forms": []string{"action_quote"}})
		}
	}
	return choices
}
func rpCompanion(a, b RPNarrativeFact) bool {
	return rpSpeech(a) && b.Action == "expression" && b.CompanionEventID == a.EventID && a.ActorID != "" && a.ActorID == b.ActorID
}
func rpActionQuote(a, b RPNarrativeFact) bool {
	return a.Action == "arrive" && rpSpeech(b) && a.ActorID != "" && a.ActorID == b.ActorID && a.PlaceName != "" && a.PlaceName == b.PlaceName
}
func rpDensity(style RPStyleProfile) string {
	if style.NarrativeDensity != "" {
		return style.NarrativeDensity
	}
	switch style.Verbosity {
	case "terse":
		return "concise"
	case "detailed":
		return "long"
	}
	return "standard"
}

func BuildDefaultRPComposition(in RPNarrativeInput) (RPCompositionPlan, error) {
	if err := validateRPCompositionInput(in); err != nil {
		return RPCompositionPlan{}, err
	}
	plan := RPCompositionPlan{Version: RPCompositionVersion(in), Register: "plain", Paragraphs: []RPCompositionParagraph{}}
	if rpExplicitClassical(in.Style.ProseInstructions) {
		plan.Register = "classical"
	}
	density := rpDensity(in.Style)
	seenPlaces := map[string]bool{}
	lastActor := ""
	lastPlace := ""
	for i := 0; i < len(in.Facts); {
		f := in.Facts[i]
		beat := RPCompositionBeat{FactRefs: []string{rpFactRef(i)}, Form: "action", Lexical: "plain"}
		if key := sha256.Sum256([]byte(f.EventID)); key[0]&1 == 1 {
			beat.Lexical = "varied"
		}
		if rpSpeech(f) {
			beat.Form = "subject_first"
			if density != "concise" && (f.Action == "respond" || f.Action == "refuse") {
				beat.Form = "quote_first"
			}
		}
		count := 1
		if i+1 < len(in.Facts) && rpCompanion(f, in.Facts[i+1]) {
			beat.FactRefs = append(beat.FactRefs, rpFactRef(i+1))
			beat.Form = "speech_gesture_quote_first"
			count = 2
			if density == "concise" {
				beat.Form = "speech_gesture_subject_first"
			}
		} else if i+1 < len(in.Facts) && rpActionQuote(f, in.Facts[i+1]) {
			beat.FactRefs = append(beat.FactRefs, rpFactRef(i+1))
			beat.Form = "action_quote"
			count = 2
		}
		paragraph := RPCompositionParagraph{Context: "none", Beats: []RPCompositionBeat{beat}}
		if density != "concise" && (density == "long" || in.Style.DescriptionDensity > 0 && (f.Action == "arrive" || lastPlace != "" && lastPlace != f.PlaceName)) && f.PlaceName != "" && !seenPlaces[f.PlaceName] {
			paragraph.Context = "scene"
			seenPlaces[f.PlaceName] = true
		}
		if density == "standard" && len(plan.Paragraphs) > 0 && f.ActorID != "" && lastActor == f.ActorID && rpSpeech(f) && rpSpeech(in.Facts[i-1]) && paragraph.Context == "none" {
			last := len(plan.Paragraphs) - 1
			plan.Paragraphs[last].Beats = append(plan.Paragraphs[last].Beats, beat)
		} else {
			plan.Paragraphs = append(plan.Paragraphs, paragraph)
		}
		lastActor = f.ActorID
		lastPlace = f.PlaceName
		i += count
	}
	return plan, nil
}

func validateRPCompositionPlan(in RPNarrativeInput, plan RPCompositionPlan) error {
	if plan.Paragraphs == nil || plan.Version != RPCompositionVersion(in) || (plan.Register != "plain" && plan.Register != "classical") || (len(plan.Paragraphs) == 0 && len(in.Facts) != 0) || len(plan.Paragraphs) > len(in.Facts) {
		return rpCompositionError("invalid composition plan")
	}
	if plan.Register == "classical" && !rpExplicitClassical(in.Style.ProseInstructions) {
		return rpCompositionError("unsupported global composition register")
	}
	next := 0
	scenes := map[string]bool{}
	for _, p := range plan.Paragraphs {
		if (p.Context != "none" && p.Context != "scene") || len(p.Beats) == 0 {
			return rpCompositionError("invalid composition plan")
		}
		if p.Context == "scene" {
			if next >= len(in.Facts) || in.Facts[next].PlaceName == "" || scenes[in.Facts[next].PlaceName] {
				return rpCompositionError("invalid composition scene")
			}
			scenes[in.Facts[next].PlaceName] = true
		}
		for _, b := range p.Beats {
			if b.Lexical != "plain" && b.Lexical != "varied" || len(b.FactRefs) == 0 || len(b.FactRefs) > 2 || next+len(b.FactRefs) > len(in.Facts) {
				return rpCompositionError("invalid composition template")
			}
			for j, ref := range b.FactRefs {
				if ref != rpFactRef(next+j) {
					return rpCompositionError("composition fact coverage mismatch")
				}
			}
			a := in.Facts[next]
			allowed := false
			if len(b.FactRefs) == 1 {
				allowed = rpSpeech(a) && (b.Form == "subject_first" || b.Form == "quote_first") || !rpSpeech(a) && b.Form == "action"
			} else {
				z := in.Facts[next+1]
				allowed = rpCompanion(a, z) && (b.Form == "speech_gesture_quote_first" || b.Form == "speech_gesture_subject_first") || rpActionQuote(a, z) && b.Form == "action_quote"
			}
			if !allowed {
				return rpCompositionError("invalid composition template")
			}
			next += len(b.FactRefs)
		}
	}
	if next != len(in.Facts) {
		return rpCompositionError("composition fact coverage mismatch")
	}
	return nil
}

func rpSubject(in RPNarrativeInput, f RPNarrativeFact) string {
	if f.ActorID == in.ControlledEntityID && f.ActorID != "" {
		switch in.Style.POV {
		case "first_person":
			return "我"
		case "second_person":
			return "你"
		}
	}
	return f.ActorName
}
func rpTarget(in RPNarrativeInput, f RPNarrativeFact) string {
	if f.TargetActorID == in.ControlledEntityID && f.TargetActorID != "" {
		switch in.Style.POV {
		case "first_person":
			return "我"
		case "second_person":
			return "你"
		}
	}
	return f.TargetActorName
}
func rpExplicitClassical(text string) bool {
	return strings.Contains(text, "古典") || strings.Contains(text, "古风") || strings.Contains(text, "文言")
}
func rpActorRegister(in RPNarrativeInput, fact RPNarrativeFact, register string) string {
	for _, cue := range in.PublicPresentations {
		if cue.ActorID == fact.ActorID && rpExplicitClassical(cue.Text) {
			return "classical"
		}
	}
	return register
}

func rpSpeechVerb(f RPNarrativeFact, register, lexical string) string {
	prefix := map[string]string{"gentle": "温和地", "firm": "坚定地", "teasing": "打趣地", "hesitant": "语调迟疑地", "flat": "语调平淡地"}[f.SpeechTone]
	return prefix + rpUntonedSpeechVerb(f, register, lexical)
}
func rpUntonedSpeechVerb(f RPNarrativeFact, register, lexical string) string {
	switch f.Action {
	case "refuse":
		return "拒绝道"
	case "respond":
		if register == "classical" {
			if lexical == "varied" {
				return "应道"
			}
			return "答道"
		}
		return "答"
	}
	if register == "classical" {
		return "道"
	}
	return "说"
}
func rpGesture(in RPNarrativeInput, f RPNarrativeFact, lexical string) string {
	plain := map[string]string{"smile": "笑了笑", "nod": "点了点头", "shake_head": "摇了摇头", "frown": "皱起眉头", "turn_away": "转过身", "beckon": "招了招手", "look_at": "看了看", "wave": "挥了挥手", "shrug": "耸了耸肩", "raise_hand": "举起手"}
	varied := map[string]string{"smile": "露出笑容", "nod": "点头", "shake_head": "摇头", "frown": "皱眉", "turn_away": "转身", "beckon": "招手", "look_at": "看向", "wave": "挥手", "shrug": "耸肩", "raise_hand": "举手"}
	verb := plain[f.ExpressionCode]
	if lexical == "varied" {
		verb = varied[f.ExpressionCode]
	}
	if f.TargetActorID != "" {
		if f.ExpressionCode == "look_at" {
			return verb + rpTarget(in, f)
		}
		preposition := "向"
		if f.ExpressionCode == "turn_away" {
			preposition = "背向"
		}
		verb = preposition + rpTarget(in, f) + verb
	}
	return verb
}
func rpRealizeBeat(ctx context.Context, in RPNarrativeInput, plan RPCompositionPlan, b RPCompositionBeat, index int) (string, error) {
	f := in.Facts[index]
	name := rpSubject(in, f)
	quote := "「" + f.Text + "」"
	register := rpActorRegister(in, f, plan.Register)
	verb := rpSpeechVerb(f, register, b.Lexical)
	colon := "："
	postSeparator := ""
	if in.Style.NarrativePackRef == "builtin/dialogue@1" {
		colon = "：\n"
		postSeparator = "\n"
	}
	switch b.Form {
	case "subject_first":
		return name + verb + colon + quote, nil
	case "quote_first":
		return quote + postSeparator + name + verb + "。", nil
	case "speech_gesture_quote_first":
		return quote + postSeparator + name + verb + "，" + rpGesture(in, in.Facts[index+1], b.Lexical) + "。", nil
	case "speech_gesture_subject_first":
		return name + verb + colon + quote + "\n" + name + rpGesture(in, in.Facts[index+1], b.Lexical) + "。", nil
	case "action_quote":
		speech := in.Facts[index+1]
		return name + "来到" + f.PlaceName + "，" + rpSpeechVerb(speech, rpActorRegister(in, speech, plan.Register), b.Lexical) + "：" + "「" + speech.Text + "」", nil
	case "action":
		if f.Action == "expression" {
			return name + rpGesture(in, f, b.Lexical) + "。", nil
		}
		switch f.Action {
		case "arrive":
			return name + "来到" + f.PlaceName + "。", nil
		case "depart":
			return name + "离开" + f.PlaceName + "。", nil
		case "silence":
			return name + "没有作答。", nil
		case "wait":
			return name + "选择等待。", nil
		}
		single := in
		single.Facts = []RPNarrativeFact{f}
		single.Style = DefaultRPStyle()
		single.Style.POV = in.Style.POV
		v, err := (LiteralRPNarrativeProvider{}).Render(ctx, single)
		if err != nil {
			return "", err
		}
		return v.Lines[0], nil
	}
	return "", rpCompositionError("invalid composition template")
}

func RenderRPComposition(ctx context.Context, in RPNarrativeInput, plan RPCompositionPlan, emit func(RPNarrativeChunk) error) (RPNarrativeView, error) {
	empty := RPNarrativeView{}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if err := validateRPCompositionInput(in); err != nil {
		return empty, err
	}
	if err := validateRPCompositionPlan(in, plan); err != nil {
		return empty, err
	}
	view := RPNarrativeView{Lines: []string{}, EventIDs: []string{}, Warnings: []string{}, CompositionVersion: RPCompositionVersion(in), FactGroups: [][]string{}}
	for _, f := range in.Facts {
		view.EventIDs = append(view.EventIDs, f.EventID)
	}
	next := 0
	for _, p := range plan.Paragraphs {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		segments := []string{}
		ids := []string{}
		place := in.Facts[next].PlaceName
		for _, b := range p.Beats {
			text, err := rpRealizeBeat(ctx, in, plan, b, next)
			if err != nil {
				return empty, err
			}
			segments = append(segments, text)
			for range b.FactRefs {
				ids = append(ids, in.Facts[next].EventID)
				next++
			}
		}
		line := strings.Join(segments, " ")
		prefix := ""
		if p.Context == "scene" {
			prefix = "在" + place + "，"
		}
		if in.Style.Tense == "past" {
			prefix = "当时，" + prefix
		}
		optionalBlocked := false
		for _, pattern := range in.Style.ForbiddenPatterns {
			if pattern != "" && strings.Contains(prefix, pattern) {
				optionalBlocked = true
			}
		}
		if optionalBlocked {
			view.Warnings = append(view.Warnings, "optional presentation suppressed by forbidden pattern")
		} else {
			line = prefix + line
		}
		for _, pattern := range in.Style.ForbiddenPatterns {
			if pattern != "" && strings.Contains(line, pattern) {
				view.Warnings = append(view.Warnings, "forbidden pattern conflicts with literal fact rendering; facts preserved")
			}
		}
		view.Lines = append(view.Lines, line)
		view.FactGroups = append(view.FactGroups, ids)
	}
	if in.Style.ProseInstructions != "" {
		view.Warnings = append(view.Warnings, RPCompositionInstructionWarning)
	}
	// Freeze nested maps/slices. A caller mutating its DTO after rendering must
	// not mutate the saved evidence or its hash through shared backing arrays.
	encoded, err := json.Marshal(RPNarrativeArtifact{Input: in, Plan: plan})
	if err != nil {
		return empty, err
	}
	artifact := &RPNarrativeArtifact{}
	if err = json.Unmarshal(encoded, artifact); err != nil {
		return empty, err
	}
	artifact.InputSHA256, err = HashJSON(artifact.Input)
	if err != nil {
		return empty, err
	}
	view.Artifact = artifact
	for i, line := range view.Lines {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		if emit != nil {
			if err := emit(RPNarrativeChunk{Index: i, EventIDs: append([]string(nil), view.FactGroups[i]...), Line: line}); err != nil {
				return empty, err
			}
		}
	}
	return view, nil
}

func (DeterministicRPNarrativeProvider) RenderStream(ctx context.Context, in RPNarrativeInput, emit func(RPNarrativeChunk) error) (RPNarrativeView, error) {
	plan, err := BuildDefaultRPComposition(in)
	if err != nil {
		return RPNarrativeView{}, err
	}
	return RenderRPComposition(ctx, in, plan, emit)
}
func (p DeterministicRPNarrativeProvider) Render(ctx context.Context, in RPNarrativeInput) (RPNarrativeView, error) {
	return p.RenderStream(ctx, in, nil)
}

func rpCompositionJSONValue(d *json.Decoder, depth int) error {
	if depth > 10 {
		return rpCompositionError("invalid composition plan")
	}
	token, err := d.Token()
	if err != nil || token == nil {
		return rpCompositionError("invalid composition plan")
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, e := d.Token()
			name, ok := key.(string)
			if e != nil || !ok || seen[name] {
				return rpCompositionError("invalid composition plan")
			}
			seen[name] = true
			if e = rpCompositionJSONValue(d, depth+1); e != nil {
				return e
			}
		}
	case '[':
		for d.More() {
			if e := rpCompositionJSONValue(d, depth+1); e != nil {
				return e
			}
		}
	default:
		return rpCompositionError("invalid composition plan")
	}
	end, e := d.Token()
	if e != nil || (delim == '{' && end != json.Delim('}')) || (delim == '[' && end != json.Delim(']')) {
		return rpCompositionError("invalid composition plan")
	}
	return nil
}
func DecodeRPCompositionPlan(raw []byte, in RPNarrativeInput) (RPCompositionPlan, error) {
	empty := RPCompositionPlan{}
	if len(raw) > 64<<10 || !utf8.Valid(raw) {
		return empty, rpCompositionError("invalid composition plan")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if err := rpCompositionJSONValue(d, 0); err != nil {
		return empty, err
	}
	if _, err := d.Token(); err != io.EOF {
		return empty, rpCompositionError("invalid composition plan")
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var p RPCompositionPlan
	if err := d.Decode(&p); err != nil {
		return empty, rpCompositionError("invalid composition plan")
	}
	if err := validateRPCompositionInput(in); err != nil {
		return empty, err
	}
	if err := validateRPCompositionPlan(in, p); err != nil {
		return empty, err
	}
	return p, nil
}
func RPCompositionSchema(in RPNarrativeInput) map[string]any {
	registers := []string{"plain"}
	if rpExplicitClassical(in.Style.ProseInstructions) {
		registers = append(registers, "classical")
	}
	variants := []any{}
	for _, choice := range RPCompositionChoices(in) {
		variants = append(variants, map[string]any{"type": "object", "additionalProperties": false, "required": []string{"fact_refs", "form", "lexical"}, "properties": map[string]any{
			"fact_refs": map[string]any{"type": "array", "const": choice["fact_refs"]}, "form": map[string]any{"type": "string", "enum": choice["forms"]}, "lexical": map[string]any{"type": "string", "enum": []string{"plain", "varied"}},
		}})
	}
	paragraph := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"context", "beats"}, "properties": map[string]any{"context": map[string]any{"type": "string", "enum": []string{"none", "scene"}}, "beats": map[string]any{"type": "array", "minItems": 1, "maxItems": len(in.Facts), "items": map[string]any{"oneOf": variants}}}}
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"version", "register", "paragraphs"}, "properties": map[string]any{"version": map[string]any{"type": "string", "const": RPCompositionVersion(in)}, "register": map[string]any{"type": "string", "enum": registers}, "paragraphs": map[string]any{"type": "array", "minItems": 0, "maxItems": len(in.Facts), "items": paragraph}}}
}
