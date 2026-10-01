package narrative

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"corerp.local/backend/internal/core"
)

// ChatProseProvider is the full-prose provider anticipated by docs/f1: the
// model selects a closed composition of already-committed public facts.
// The server owns every speech/action atom and its speaker or actor label.
// Fresh raw prose is rejected; legacy saved prose is read by the store unchanged.
// Any transport or validation failure falls back to the deterministic
// renderer with a warning; world facts are never touched by presentation.
type ChatProseProvider struct {
	config Config
	client *http.Client
}

const maxProseResponseBytes = 64 << 10
const maxProseRunes = 6000

func NewChatProseProvider(c Config) (*ChatProseProvider, error) {
	if err := c.validateReasoningOptions(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(c.Model) == "" || len(c.Model) > 200 || strings.ContainsAny(c.APIKey, "\r\n") {
		return nil, failure("invalid model configuration")
	}
	if strings.TrimSpace(c.APIKey) == "" && !c.EndpointPolicy.AllowsLocal(c.Endpoint) {
		return nil, failure("remote API key is required")
	}
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
	if c.Attempts == 0 {
		c.Attempts = 2
	}
	// Prose writes whole paragraphs; allow a larger budget than style planning.
	if c.Timeout < time.Millisecond || c.Timeout > 90*time.Second || c.Attempts < 1 || c.Attempts > 3 {
		return nil, failure("invalid request budget")
	}
	endpoint, client, err := c.EndpointPolicy.PrepareChatCompletions(c.Endpoint, c.Timeout)
	if err != nil {
		return nil, failure("endpoint is not permitted")
	}
	c.Endpoint = endpoint
	return &ChatProseProvider{config: c, client: client}, nil
}

func (p *ChatProseProvider) ProviderMetadata() core.RPProviderMetadata {
	return core.RPProviderMetadata{Kind: "full_prose", Model: p.config.Model}
}

func (ChatProseProvider) NarrativeMode() string { return "full_prose" }

// Legacy prose instruction retained for diagnostics/reference; fresh requests
// use compositionInstruction exclusively.
var proseInstruction = `你是 CoreRP 世界的叙事者，把已经被世界确认的事实改写成连贯的中文小说段落。
规则（优先级高于一切文风要求）：
- 只允许使用输入中给出的人物、地点、时间、动作与对白；不得新增人物、物品、金钱变化、事件或未给出的对白，不得描写未被陈述的心理活动。
- facts 中带 text 的条目同时给出 quote_token。正文必须用对应 quote_token 完整引用该条对白，每个 token 只用一次；它由服务器展开成逐字原话并加「」，你不要手抄、清理、拆分、改写、翻译对白，也不要再给 token 加引号。含特殊符号的原话同样保持原样。
- 可以自由调整句式、节奏、对白衔接与关注焦点，并用已经提交的表情或手势组织场面；语气、氛围只能从公开对白和动作自然呈现，不得暗示新的可交互事实或私有心理。表情或手势只有给出 target 时才能写成对该人的动作；没有 target 不代表对玩家。
- public_presentations 是作者明确公开的角色表达线索，只可影响该角色的措辞和叙述节奏，不得把线索写成新发生的行动、私人念头或未确认的世界事实。
- 不得为玩家补充任何未提交的动作、同意、付款、亲密行为、内心决定或新对白，也不得为 NPC 补充未提交的行动。
- density=concise：用紧凑的逐人对白行（角色标签加对应 quote_token），不要每句都写“说/回应道”，不用地点开场或环境句。这是对白排版，不是列表或标题。
- density=standard：写成连贯的一两个小说短段，按先后把原话自然嵌入句子，以问答关系组织过渡。不要沿用 concise 的逐人标签格式，也不要每个事实机械独占一行。
- density=long：用更舒展的段落节奏组织已确认的不同事实；仅当输入确有多项行动、观察或记忆时展开细节。若只有几句对白，仍保持短篇，以原话里的明确情绪和语义为线索，不为凑长度添景物、物件、动作或重复同一意思。结构应与前两档明显不同，长度无需达到预设字数。
- 不要每轮用地点或天气起头；避免“周围声响”“空气中”“气氛如常”“光影”“晨光”等通用装饰句。先呈现人物本轮实际回答，别让文学填充淹没对话。
- 遵守输入中的视角（controlled 角色按视角称为「你/我/其名」）、时态与详略要求；分段输出，每段单独一行，不要使用列表或标题。
- 输入 forbidden 中的任何内容都不得出现。`

var exactTimePrefix = regexp.MustCompile(`^\s*(?:(?:公元\s*)?\d{4}\s*(?:年|[-/.])\s*\d{1,2}\s*(?:月|[-/.])\s*\d{1,2}\s*(?:日|号)?|\d{1,2}\s*[:：]\s*\d{2})`)
var proseMetaLanguage = regexp.MustCompile(`(?i)(?:facts?|输入中|输出中|根据事实|已提交(?:的)?(?:事实|对白)|两句对白|这段场景|没有(?:别的|新的)?(?:事|事情|人物|物品|事件|动静)(?:被|发生|出现|展开|带入)|未新增(?:人物|物品|事件|对白))`)

type proseFact struct {
	// Legacy test/decoder field; fresh plans reference entire fact atoms.
	QuoteToken       string   `json:"quote_token,omitempty"`
	FactRef          string   `json:"fact_ref"`
	AllowedTemplates []string `json:"allowed_templates"`
	Actor            string   `json:"actor"`
	Action           string   `json:"action"`
	Target           string   `json:"target,omitempty"`
	Text             string   `json:"text,omitempty"`
	Object           string   `json:"object,omitempty"`
	State            string   `json:"state,omitempty"`
	When             string   `json:"when,omitempty"`
	Where            string   `json:"where,omitempty"`
	POV              bool     `json:"controlled,omitempty"`
}

type prosePresentation struct {
	Actor string `json:"actor"`
	Style string `json:"style"`
}

// actionLabel renders activity facts with the world-declared prose label
// (fact label, then input labels, then the raw code) and plain actions with
// their fixed verb.
func actionLabel(fact core.RPNarrativeFact, labels map[string]string) string {
	switch fact.Action {
	case "expression":
		return map[string]string{"smile": "微笑", "nod": "点头", "shake_head": "摇头", "turn_away": "转身", "frown": "皱眉", "beckon": "招手"}[fact.ExpressionCode]
	case "act", "activity_done", "activity_interrupted":
		if label := fact.ActivityLabel; label != "" {
			return label
		}
		if label := labels[fact.ActivityCode]; label != "" {
			return label
		}
		return fact.ActivityCode
	default:
		return proseActionLabel(fact.Action)
	}
}

func proseActionLabel(action string) string {
	switch action {
	case "speak":
		return "说"
	case "respond":
		return "回应"
	case "refuse":
		return "拒绝"
	case "silence":
		return "保持沉默"
	case "wait":
		return "等待"
	case "leave":
		return "离开"
	case "object_open":
		return "打开"
	case "object_close":
		return "关上"
	case "object_switch_on":
		return "开启"
	case "object_switch_off":
		return "关闭"
	default:
		return action
	}
}

func (p *ChatProseProvider) Render(ctx context.Context, in core.RPNarrativeInput) (core.RPNarrativeView, error) {
	return p.RenderStream(ctx, in, nil)
}

func (p *ChatProseProvider) RenderStream(ctx context.Context, in core.RPNarrativeInput, emit func(core.RPNarrativeChunk) error) (core.RPNarrativeView, error) {
	if err := ctx.Err(); err != nil {
		return core.RPNarrativeView{}, err
	}
	if err := in.ValidateReadBudget(); err != nil {
		return core.RPNarrativeView{}, err
	}
	// A synthetic paragraph cites its bounded input fact set, never a single
	// fabricated event. Reject missing provenance before sending any context.
	seenSources := make(map[string]bool, len(in.Facts))
	for _, fact := range in.Facts {
		if fact.EventID == "" || seenSources[fact.EventID] {
			return core.RPNarrativeView{}, core.NewError(core.CodeProjectionDiverged, "prose input lacks unique committed source")
		}
		seenSources[fact.EventID] = true
	}
	// write validates the closed plan and expands only server-owned atoms.
	// Legacy free-prose heuristics must not reinterpret canonical speech data.
	composition, err := p.write(ctx, in)
	if err != nil {
		// Presentation-only fallback: facts stay authoritative, and the
		// sanitized reason travels with the view so the caller can persist
		// it — a fallback must always be queryable, never silent.
		fallback := in
		// Detailed deterministic rendering prefixes every fact with the exact
		// world timestamp. That is useful for an audit view but especially
		// mechanical as a prose fallback, so keep POV/tense while dropping only
		// the repeated timestamp presentation.
		if fallback.Style.Verbosity == "detailed" {
			fallback.Style.Verbosity = "normal"
		}
		view, derr := (core.DeterministicRPNarrativeProvider{}).RenderStream(ctx, fallback, emit)
		if derr != nil {
			return core.RPNarrativeView{}, derr
		}
		view.FallbackReason = "prose_" + failureKind(err)
		view.Warnings = append(view.Warnings, "小说式呈现暂不可用或正文未通过事实校验，已回退为标准叙述。")
		return view, nil
	}
	view := core.RPNarrativeView{Lines: []string{}, EventIDs: []string{}, Warnings: []string{}}
	// View references retain exact committed-fact order. The render store
	// records any authored public-style sources separately from turn facts.
	for _, fact := range in.Facts {
		view.EventIDs = append(view.EventIDs, fact.EventID)
	}
	view.CompositionVersion = CompositionVersion
	view.FactGroups = composition.Sources
	view.Warnings = append(view.Warnings, composition.Warnings...)
	// Plan groups, not raw newline splitting, define emission boundaries.
	// Newlines and spaces inside accepted speech remain byte-for-byte intact.
	for index, paragraph := range composition.Lines {
		view.Lines = append(view.Lines, paragraph)
		if emit != nil {
			if err := emit(core.RPNarrativeChunk{Index: index, EventIDs: composition.Sources[index], Line: paragraph}); err != nil {
				return core.RPNarrativeView{}, err
			}
		}
	}
	return view, nil
}

func (p *ChatProseProvider) write(ctx context.Context, in core.RPNarrativeInput) (*compositionResult, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()
	facts := make([]proseFact, 0, len(in.Facts))
	for index, fact := range in.Facts {
		target := fact.TargetActorName
		if fact.TargetActorID == in.ControlledEntityID && fact.TargetActorID != "" {
			switch in.Style.POV {
			case "first_person":
				target = "我"
			case "second_person":
				target = "你"
			}
		}
		templates := compositionTemplates(fact)
		facts = append(facts, proseFact{
			FactRef: compositionFactRef(index), AllowedTemplates: templates,
			Actor: fact.ActorName, Action: actionLabel(fact, in.ActivityLabels), Target: target, Text: fact.Text, Object: fact.ObjectName, State: fact.ObjectState,
			When: fact.WorldTime, Where: fact.PlaceName, POV: fact.ActorID == in.ControlledEntityID,
		})
	}
	presentations := make([]prosePresentation, 0, len(in.PublicPresentations))
	for _, cue := range in.PublicPresentations {
		presentations = append(presentations, prosePresentation{Actor: cue.ActorName, Style: cue.Text})
	}
	density := in.Style.NarrativeDensity
	if density == "" {
		switch in.Style.Verbosity {
		case "terse":
			density = "concise"
		case "detailed":
			density = "long"
		default:
			density = "standard"
		}
	}
	// Silence/wait is a public fact, but it does not supply scene beats that
	// can support an expanded paragraph any more than a short reply does.
	if sparseProseEvidence(in) {
		density = "concise"
	}
	payload := map[string]any{
		"composition_version": compositionVersion, "capability_limit": compositionCapabilityWarning, "output_schema": compositionSchema(in), "pov": in.Style.POV, "tense": in.Style.Tense, "verbosity": in.Style.Verbosity,
		"density": density, "dialogue_ratio": in.Style.DialogueRatio, "description_density": in.Style.DescriptionDensity,
		"instructions": in.Style.ProseInstructions, "forbidden": in.Style.ForbiddenPatterns, "facts": facts,
	}
	if len(presentations) > 0 {
		payload["public_presentations"] = presentations
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, failure("request encoding failed")
	}
	var last error
	var previousDraft, revisionInstruction string
	for attempt := 0; attempt < p.config.Attempts; attempt++ {
		messages := []map[string]string{{"role": "system", "content": compositionInstruction}, {"role": "user", "content": string(encoded)}}
		if previousDraft != "" {
			messages = append(messages, map[string]string{"role": "assistant", "content": previousDraft}, map[string]string{"role": "user", "content": revisionInstruction})
		}
		request := map[string]any{
			"model": p.config.Model, "stream": false, "store": false, "max_completion_tokens": 4096,
			"messages": messages,
		}
		p.config.applyReasoningOptions(request)
		body, err := json.Marshal(request)
		if err != nil {
			return nil, failure("request encoding failed")
		}
		text, retry, delay, err := p.attempt(ctx, body)
		if err == nil {
			draft := text
			composition, compositionErr := renderComposition(ctx, draft, in)
			err = compositionErr
			verr := err
			if verr == nil {
				return composition, nil
			} else {
				// A fact-breaking draft is worth one explicit repair,
				// not an identical retry that tends to reproduce the same defect.
				err, retry, delay = verr, true, 0
				debugProseValidation(verr)
				previousDraft = draft
				revisionInstruction = "上次输出不是有效的闭合排版计划。只返回 " + compositionVersion + " JSON；每个 fact_ref 按原序恰好一次，使用 allowed_templates，禁止所有额外字段及正文。"
			}
		}
		last = err
		if !retry || attempt+1 == p.config.Attempts {
			break
		}
		if delay <= 0 {
			delay = time.Duration(100*(1<<attempt)) * time.Millisecond
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, failure("timeout or cancellation")
		case <-timer.C:
		}
	}
	if last == nil {
		last = failure("attempt budget exhausted")
	}
	return nil, last
}

// Classify from fixed validator errors only. Unknown kinds stay "other";
// response text, accepted speech, IDs, URLs and credentials are never logged.
func proseValidationCategory(err error) string {
	switch failureKind(err) {
	case "invalid composition plan", "composition fact coverage mismatch", "invalid composition template":
		return "composition_invalid"
	case "accepted speech was rewritten or dropped":
		return "speech_changed"
	case "prose introduced uncommitted dialogue":
		return "new_dialogue"
	case "invalid prose speech token", "missing or repeated prose speech token":
		return "speech_token_invalid"
	case "unbalanced prose quotation":
		return "quotation_mismatch"
	case "uncommitted expression in prose":
		return "expression_not_committed"
	case "uncommitted expression target in prose":
		return "expression_target_not_committed"
	case "uncommitted player agency in prose":
		return "player_action_not_committed"
	case "uncommitted player speech in prose":
		return "player_speech_not_committed"
	case "uncommitted NPC action in prose":
		return "npc_action_not_committed"
	case "uncommitted gaze in prose":
		return "gaze_not_committed"
	case "uncommitted money claim in prose", "uncommitted time change in prose", "uncommitted object in prose", "uncommitted person in prose", "uncommitted relationship in prose", "uncommitted name in prose":
		return "world_claim_not_committed"
	case "prose empty or exceeds budget", "prose exceeds sparse public evidence":
		return "output_budget"
	case "prose contains generation-rule or fact-audit language":
		return "audit_language"
	case "prose begins a paragraph with an exact date or clock time":
		return "timestamp_prefix"
	case "forbidden pattern in prose":
		return "forbidden_pattern"
	default:
		return "other"
	}
}

func debugProseValidation(err error) {
	if os.Getenv("CORERP_NARRATIVE_DEBUG") != "" {
		fmt.Fprintln(os.Stderr, "corerp_prose_validation category="+proseValidationCategory(err))
	}
}

func proseRevisionInstruction(cause error) string {
	reason := failureKind(cause)
	if reason == "uncommitted expression target in prose" {
		return "上一个草稿为表情或手势新增或改变了互动对象。请从头重写，保留每个 quote_token 一次；仅当对应 fact 给出了 target 才能向该人点头、微笑或招手。没有 target 就只描写该 actor 的表情，不增加对象。不要新增动作、对白或心理事实。"
	}
	if strings.Contains(reason, "expression in prose") {
		return "上一个草稿把表情写给了没有对应可观察事件的人，或在多人场景中用了无法确定主语的他/她。请从头重写，逐字保留 facts 中的对白；只描写 facts 中已有的表情，并明确写出该 expression 的 actor 名字。若无法确定主语，直接省略表情。不要新增动作或对白。"
	}
	return "上一个草稿未通过 CoreRP 呈现校验（" + reason + "）。请从头重写，只输出小说正文；每条对白使用 facts 中对应的 quote_token 一次，由服务器逐字保留原话，不要清理或重抄 text，删除任何未提交对白；不得新增人物、物品、事件、知识、心理事实或玩家行动。不要解释规则，不要靠重复或编造事实凑长度。"
}

const proseSpeechTokenPrefix = "[[corerp-speech:"

var proseSpeechTokens = regexp.MustCompile(`\[\[corerp-speech:[^\]\n]{1,80}\]\]`)

func proseSpeechToken(index int) string {
	return proseSpeechTokenPrefix + strconv.Itoa(index) + "]]"
}

// Speech belongs to the world renderer, not to a model's copy/edit operation.
// Expand only spans in the original draft in one pass; a literal token inside
// committed speech must not be interpreted recursively. Legacy direct quotes
// still undergo the same exact-speech and observable checks after this step.
func expandProseSpeechTokens(draft string, in core.RPNarrativeInput) (string, error) {
	if !strings.Contains(draft, proseSpeechTokenPrefix) {
		return draft, nil
	}
	// Already-valid legacy prose may quote a literal token as spoken data.
	// Keep it unchanged instead of promoting the quoted text to an instruction.
	if validateProse(draft, in) == nil {
		return draft, nil
	}
	quotes := map[string]string{}
	terminalSpeech := map[string]bool{}
	for index, fact := range in.Facts {
		if fact.Text != "" {
			token := proseSpeechToken(index)
			quotes[token] = "「" + fact.Text + "」"
			last, _ := utf8.DecodeLastRuneInString(strings.TrimSpace(fact.Text))
			terminalSpeech[token] = strings.ContainsRune("。！？.!?…", last)
		}
	}
	matches := proseSpeechTokens.FindAllStringIndex(draft, -1)
	if len(matches) != strings.Count(draft, proseSpeechTokenPrefix) {
		return "", failure("invalid prose speech token")
	}
	var out strings.Builder
	used := map[string]bool{}
	offset := 0
	for _, match := range matches {
		token := draft[match[0]:match[1]]
		quote, known := quotes[token]
		if !known {
			return "", failure("invalid prose speech token")
		}
		if used[token] {
			return "", failure("missing or repeated prose speech token")
		}
		used[token] = true
		out.WriteString(draft[offset:match[0]])
		out.WriteString(quote)
		offset = match[1]
		// The model owns punctuation outside this inserted quotation only.
		// Avoid 「already-ended。」。 without cleaning any accepted speech,
		// including literal nested quotes, tokens, or punctuation in that speech.
		if terminalSpeech[token] && strings.HasPrefix(draft[offset:], "。") {
			offset += len("。")
		}
	}
	if len(used) != len(quotes) {
		return "", failure("missing or repeated prose speech token")
	}
	out.WriteString(draft[offset:])
	return out.String(), nil
}

func (p *ChatProseProvider) attempt(ctx context.Context, body []byte) (string, bool, time.Duration, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", false, 0, failure("invalid request")
	}
	r.Header.Set("Content-Type", "application/json")
	if p.config.APIKey != "" {
		r.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	}
	core.RecordRPProviderHTTPAttempt(ctx)
	response, err := p.client.Do(r)
	if err != nil {
		if ctx.Err() != nil {
			return "", false, 0, failure("timeout or cancellation")
		}
		return "", true, 0, failure("transport unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		retry := response.StatusCode == 429 || response.StatusCode == 408 || response.StatusCode >= 500
		return "", retry, retryDelay(response.Header.Get("Retry-After")), failure("HTTP " + strconv.Itoa(response.StatusCode))
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxProseResponseBytes+1))
	if err != nil {
		return "", ctx.Err() == nil, 0, failure("response interrupted")
	}
	if len(raw) > maxProseResponseBytes {
		return "", false, 0, failure("response exceeds budget")
	}
	var envelope struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content   string            `json:"content"`
				Refusal   *string           `json:"refusal"`
				ToolCalls []json.RawMessage `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &envelope) != nil || len(envelope.Choices) != 1 {
		return "", false, 0, failure("invalid response envelope")
	}
	choice := envelope.Choices[0]
	if choice.FinishReason != "stop" || choice.Message.Refusal != nil || len(choice.Message.ToolCalls) != 0 {
		return "", false, 0, failure("incomplete or refused response")
	}
	return strings.TrimSpace(choice.Message.Content), false, 0, nil
}

// failureKind extracts the sanitized Kind from a package error (never URLs,
// credentials or model text) so a fallback stays auditable.
func failureKind(err error) string {
	var e *Error
	if errors.As(err, &e) && e.Kind != "" {
		return e.Kind
	}
	return "unknown"
}

// validateProse enforces the full-prose contract: every accepted utterance is
// present verbatim, no quoted span invents dialogue, forbidden patterns stay
// absent, and the text stays within a bounded size.
func validateProse(text string, in core.RPNarrativeInput) error {
	if text == "" || utf8.RuneCountInString(text) > maxProseRunes {
		return failure("prose empty or exceeds budget")
	}
	paragraphs := proseParagraphs(text)
	for _, paragraph := range paragraphs {
		if exactTimePrefix.MatchString(paragraph) {
			return failure("prose begins a paragraph with an exact date or clock time")
		}
	}
	if proseMetaLanguage.MatchString(text) {
		return failure("prose contains generation-rule or fact-audit language")
	}
	if sparseProseEvidence(in) {
		speechRunes := 0
		for _, fact := range in.Facts {
			speechRunes += utf8.RuneCountInString(fact.Text)
		}
		if utf8.RuneCountInString(text) > 64+2*speechRunes {
			return failure("prose exceeds sparse public evidence")
		}
	}
	committed := map[string]bool{}
	for _, fact := range in.Facts {
		if fact.Text == "" {
			continue
		}
		committed[fact.Text] = true
		if !strings.Contains(text, fact.Text) {
			return failure("accepted speech was rewritten or dropped")
		}
	}
	for _, pattern := range in.Style.ForbiddenPatterns {
		if pattern != "" && strings.Contains(text, pattern) {
			return failure("forbidden pattern in prose")
		}
	}
	quotes, err := quotedSpans(text)
	if err != nil {
		return err
	}
	// A quoted phrase inside an exact accepted utterance is part of that
	// utterance, not a second line of dialogue invented by the narrator.
	acceptedRanges := make([]proseTextRange, 0, len(in.Facts))
	for _, fact := range in.Facts {
		if fact.Text == "" {
			continue
		}
		for offset := 0; offset < len(text); {
			index := strings.Index(text[offset:], fact.Text)
			if index < 0 {
				break
			}
			start := offset + index
			acceptedRanges = append(acceptedRanges, proseTextRange{start: start, end: start + len(fact.Text)})
			offset = start + len(fact.Text)
		}
	}
	for _, quoted := range quotes {
		if committed[quoted.text] || quoteInsideAcceptedSpeech(quoted, acceptedRanges) {
			continue
		}
		return failure("prose introduced uncommitted dialogue")
	}
	if err := validateProsePlayerAgency(text, in); err != nil {
		return err
	}
	if err := validateProseSceneClaims(text, in); err != nil {
		return err
	}
	return nil
}

// Only speech and a bounded no-response beat were observed. Without a
// gesture, activity, or authored public style there is no source for a wider
// scene, regardless of the configured verbosity.
func sparseProseEvidence(in core.RPNarrativeInput) bool {
	if len(in.Facts) == 0 || len(in.Facts) > 2 || len(in.PublicPresentations) != 0 {
		return false
	}
	for _, fact := range in.Facts {
		switch fact.Action {
		case "speak", "respond", "refuse", "silence", "wait":
		default:
			return false
		}
	}
	return true
}

type proseTextRange struct{ start, end int }
type proseQuoteSpan struct {
	proseTextRange
	text string
}

func quoteInsideAcceptedSpeech(quoted proseQuoteSpan, ranges []proseTextRange) bool {
	for _, accepted := range ranges {
		if quoted.start >= accepted.start && quoted.end <= accepted.end {
			return true
		}
	}
	return false
}

func proseParagraphs(text string) []string {
	paragraphs := make([]string, 0, 8)
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph = strings.TrimSpace(paragraph); paragraph != "" {
			paragraphs = append(paragraphs, paragraph)
		}
	}
	return paragraphs
}

// Scan matching delimiters rather than stopping at the first closing quote.
// An accepted utterance may quote somebody else using the same marks as the
// narrator's outer quotation. Return only complete outer spans, in text order,
// so all claim guards agree on which text is attributed speech.
func quotedSpans(text string) ([]proseQuoteSpan, error) {
	var spans []proseQuoteSpan
	type opening struct {
		index int
		mark  rune
	}
	var stack []opening
	for index, mark := range text {
		switch mark {
		case '「', '“':
			stack = append(stack, opening{index: index, mark: mark})
		case '」', '”':
			if len(stack) == 0 {
				return nil, failure("unbalanced prose quotation")
			}
			last := stack[len(stack)-1]
			if last.mark == '「' && mark != '」' || last.mark == '“' && mark != '”' {
				return nil, failure("unbalanced prose quotation")
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				spans = append(spans, proseQuoteSpan{
					proseTextRange: proseTextRange{start: last.index, end: index + utf8.RuneLen(mark)},
					text:           text[last.index+utf8.RuneLen(last.mark) : index],
				})
			}
		}
	}
	if len(stack) != 0 {
		return nil, failure("unbalanced prose quotation")
	}
	return spans, nil
}
