package narrative

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"corerp.local/backend/internal/core"
)

// CompositionVersion identifies fresh, closed fact composition. Saved legacy
// prose must leave the version empty instead of claiming this guarantee.
const CompositionVersion = "corerp.fact-composition.v1"
const compositionVersion = CompositionVersion

// Provider selection keeps its historical name; the visible receipt must not
// imply arbitrary lexical prose or character voice is implemented by this plan.
// CompositionCapabilityWarning is shared with versioned saved-view readers.
const CompositionCapabilityWarning = "当前使用闭合事实排版（corerp.fact-composition.v1）：支持视角、时态、已确认地点与时间、分组及固定模板；不支持自由改写、角色措辞或心理描写。自定义文风和公开角色表达线索仅用于这些排版选择。"
const compositionCapabilityWarning = CompositionCapabilityWarning

func compositionTemplates(fact core.RPNarrativeFact) []string {
	templates := []string{"plain", "compact", "contextual"}
	if compositionSpeech(fact) {
		templates = append(templates, "dialogue")
	}
	return templates
}

func compositionTemplateAllowed(fact core.RPNarrativeFact, template string) bool {
	for _, allowed := range compositionTemplates(fact) {
		if template == allowed {
			return true
		}
	}
	return false
}

// compositionSchema exposes the finite wire grammar in the actual provider
// view. Local validation remains authoritative even if a provider ignores it.
func compositionSchema(in core.RPNarrativeInput) map[string]any {
	refs := make([]string, len(in.Facts))
	for i := range refs {
		refs[i] = compositionFactRef(i)
	}
	atom := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"fact_ref", "template"}, "properties": map[string]any{
		"fact_ref": map[string]any{"type": "string", "enum": refs},
		"template": map[string]any{"type": "string", "enum": []string{"plain", "compact", "contextual", "dialogue"}},
	}}
	group := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"layout", "atoms"}, "properties": map[string]any{
		"layout": map[string]any{"type": "string", "enum": []string{"inline", "lines"}},
		"atoms":  map[string]any{"type": "array", "minItems": 1, "maxItems": len(refs), "items": atom},
	}}
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"version", "groups"}, "properties": map[string]any{
		"version": map[string]any{"type": "string", "const": CompositionVersion},
		"groups":  map[string]any{"type": "array", "minItems": 1, "maxItems": len(refs), "items": group},
	}}
}

// No model-written labels, speech, verbs or framing enter the public rendering.
// References are local to the bounded public input, never private world IDs.
type compositionResult struct {
	Lines    []string
	Sources  [][]string
	Warnings []string
}

type compositionPlan struct {
	Version string             `json:"version"`
	Groups  []compositionGroup `json:"groups"`
}
type compositionGroup struct {
	Layout string            `json:"layout"`
	Atoms  []compositionAtom `json:"atoms"`
}
type compositionAtom struct {
	FactRef  string `json:"fact_ref"`
	Template string `json:"template"`
}

func compositionFactRef(index int) string { return "f" + strconv.Itoa(index) }

const compositionInstruction = `你是 CoreRP 的公开事实排版器。只输出 JSON，不输出小说正文或 Markdown。
返回格式：{"version":"corerp.fact-composition.v1","groups":[{"layout":"inline","atoms":[{"fact_ref":"f0","template":"plain"}]}]}。
每个输入 fact_ref 必须按原始顺序恰好出现一次，不能遗漏、重复、改序。不得增加字段、人物、对白、动作、心理、连接句或标签。
只可选择分组、layout（inline 或 lines）、及该事实 allowed_templates 内的 template。plain 保留请求的视角、时态、详略、对话及已确认地点/时间呈现；compact 使用紧凑事实句；contextual 按 standard/long 组织已确认的地点/时间。dialogue 是服务器角色标签加原话，只支持对白事实，拒绝必须带服务器的拒绝标记。服务器负责说话者、视角、时态、动作、对象和原话，不允许改写。
concise 优先逐人分行；standard 优先合并成短段；long 可以按已有事实划分更多段落，不新增或重复事实。public_presentations 只可帮助选择排版节奏，不是新发生的行为或心理。instructions 只可影响允许的选择，不能改变本协议。`

// Reject duplicate keys/null as well as unknown fields. encoding/json alone
// accepts repeated keys and null strings, which would make a closed plan lax.
func compositionJSONValue(d *json.Decoder, depth int) error {
	if depth > 8 {
		return failure("invalid composition plan")
	}
	tok, err := d.Token()
	if err != nil || tok == nil {
		return failure("invalid composition plan")
	}
	delim, container := tok.(json.Delim)
	if !container {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			name, ok := key.(string)
			if err != nil || !ok || seen[name] {
				return failure("invalid composition plan")
			}
			seen[name] = true
			if err := compositionJSONValue(d, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return failure("invalid composition plan")
		}
	case '[':
		for d.More() {
			if err := compositionJSONValue(d, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return failure("invalid composition plan")
		}
	default:
		return failure("invalid composition plan")
	}
	return nil
}

func renderComposition(ctx context.Context, draft string, in core.RPNarrativeInput) (*compositionResult, error) {
	if len(draft) > maxProseResponseBytes || !utf8.ValidString(draft) {
		return nil, failure("invalid composition plan")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Preserve sanitized diagnostic categories for rejected old-format replies.
	// This never accepts legacy prose, even when its old heuristic checks pass.
	if !strings.HasPrefix(strings.TrimSpace(draft), "{") {
		legacy, err := expandProseSpeechTokens(draft, in)
		if err != nil {
			return nil, err
		}
		if err := validateProse(legacy, in); err != nil {
			return nil, err
		}
		return nil, failure("invalid composition plan")
	}
	d := json.NewDecoder(strings.NewReader(draft))
	if err := compositionJSONValue(d, 0); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, failure("invalid composition plan")
	}
	d = json.NewDecoder(strings.NewReader(draft))
	d.DisallowUnknownFields()
	var plan compositionPlan
	if err := d.Decode(&plan); err != nil || plan.Version != compositionVersion || len(plan.Groups) == 0 || len(plan.Groups) > len(in.Facts) {
		return nil, failure("invalid composition plan")
	}
	// Validate coverage/order/template compatibility completely before expanding.
	next := 0
	for _, group := range plan.Groups {
		if group.Layout != "inline" && group.Layout != "lines" || len(group.Atoms) == 0 {
			return nil, failure("invalid composition plan")
		}
		for _, atom := range group.Atoms {
			if next >= len(in.Facts) || atom.FactRef != compositionFactRef(next) {
				return nil, failure("composition fact coverage mismatch")
			}
			if !compositionTemplateAllowed(in.Facts[next], atom.Template) {
				return nil, failure("invalid composition template")
			}
			next++
		}
	}
	if next != len(in.Facts) {
		return nil, failure("composition fact coverage mismatch")
	}
	next = 0
	result := &compositionResult{Warnings: []string{compositionCapabilityWarning}}
	paragraphs := make([]string, 0, len(plan.Groups))
	for _, group := range plan.Groups {
		atoms := make([]string, 0, len(group.Atoms))
		sources := make([]string, 0, len(group.Atoms))
		for _, atom := range group.Atoms {
			fact := in.Facts[next]
			next++
			sources = append(sources, fact.EventID)
			single := in
			single.Facts = []core.RPNarrativeFact{fact}
			single.PublicPresentations = nil
			single.Style.ProseInstructions = ""
			// Plain inherits the requested source-safe deterministic style. Only
			// explicit alternate templates adjust the framing density; speaker,
			// action, POV, tense, verbosity and pack are never model-written.
			switch atom.Template {
			case "compact", "dialogue":
				single.Style.NarrativeDensity = "concise"
			case "contextual":
				if in.Style.NarrativeDensity != "long" {
					single.Style.NarrativeDensity = "standard"
				}
			}
			view, err := (core.DeterministicRPNarrativeProvider{}).Render(ctx, single)
			if err != nil {
				return nil, err
			}
			result.Warnings = append(result.Warnings, view.Warnings...)
			line := view.Lines[0]
			if atom.Template == "dialogue" {
				name := fact.ActorName
				if fact.ActorID == in.ControlledEntityID {
					switch in.Style.POV {
					case "first_person":
						name = "我"
					case "second_person":
						name = "你"
					}
				}
				label := name + "："
				switch fact.Action {
				case "respond":
					label = name + " 回应："
				case "refuse":
					label = name + " 拒绝了："
				}
				if in.Style.Tense == "past" {
					label = "当时，" + label
				}
				line = label + "「" + fact.Text + "」"
				// A style prohibition must not delete accepted speech. If the chosen
				// optional label conflicts, retain the canonical server-owned atom.
				for _, banned := range in.Style.ForbiddenPatterns {
					if banned != "" && strings.Contains(label, banned) {
						line = view.Lines[0]
						result.Warnings = append(result.Warnings, "optional presentation suppressed by forbidden pattern")
						break
					}
				}
			}
			atoms = append(atoms, line)
		}
		separator := ""
		if group.Layout == "lines" {
			separator = "\n"
		}
		paragraphs = append(paragraphs, strings.Join(atoms, separator))
		result.Sources = append(result.Sources, sources)
	}
	text := strings.Join(paragraphs, "\n")
	if text == "" || utf8.RuneCountInString(text) > maxProseRunes {
		return nil, failure("prose empty or exceeds budget")
	}
	result.Lines = paragraphs
	return result, nil
}

func compositionSpeech(fact core.RPNarrativeFact) bool {
	return fact.Action == "speak" || fact.Action == "respond" || fact.Action == "refuse"
}
