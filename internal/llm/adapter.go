package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"corerp/internal/core"
)

type Adapter struct {
	endpoint   string
	apiKey     string
	model      string
	httpClient *http.Client
	promptTokens     int
	completionTokens int
}

// Usage returns the token counts from the last call.
func (a *Adapter) Usage() (prompt, completion int) {
	return a.promptTokens, a.completionTokens
}

// Model returns the model name.
func (a *Adapter) Model() string {
	return a.model
}

func NewAdapter(endpoint, apiKey, model string) *Adapter {
	return &Adapter{
		endpoint:   endpoint,
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{},
	}
}

// Generate sends a snapshot prompt and streams back the response via callback.
func (a *Adapter) Generate(prompt string, onChunk func(core.LLMStreamChunk)) error {
	reqBody := core.LLMRequest{
		Model:       a.model,
		Messages:    []core.LLMMessage{{Role: "system", Content: prompt}},
		Stream:      true,
		Temperature: 0.3,
		MaxTokens:   2048,
	}

	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", a.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("LLM error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				onChunk(core.LLMStreamChunk{Done: true})
				return nil
			}
			return err
		}

		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			onChunk(core.LLMStreamChunk{Done: true})
			return nil
		}

		var streamResp struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			continue
		}
		if streamResp.Usage != nil {
			a.promptTokens = streamResp.Usage.PromptTokens
			a.completionTokens = streamResp.Usage.CompletionTokens
		}
		if len(streamResp.Choices) > 0 {
			content := streamResp.Choices[0].Delta.Content
			if content != "" {
				onChunk(core.LLMStreamChunk{Content: content})
			}
		}
	}
}

// GenerateNonStream is used for simple completions (e.g. working memory summarization).
func (a *Adapter) GenerateNonStream(messages []core.LLMMessage) (string, error) {
	reqBody := core.LLMRequest{
		Model:       a.model,
		Messages:    messages,
		Stream:      false,
		Temperature: 0.5,
		MaxTokens:   1024,
	}

	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", a.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}
	if result.Usage != nil {
		a.promptTokens = result.Usage.PromptTokens
		a.completionTokens = result.Usage.CompletionTokens
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in LLM response")
	}
	return result.Choices[0].Message.Content, nil
}

// flexibleFrame accepts multiple effects formats from LLM.
type flexibleFrame struct {
	Actor         string            `json:"actor"`
	Action        string            `json:"action"`
	Target        string            `json:"target"`
	Intensity     int               `json:"intensity"`
	Emotion       core.EmotionState `json:"emotion"`
	Intent        string            `json:"intent"`
	SuggestedLine string            `json:"suggested_line"`
	Effects       json.RawMessage   `json:"effects"`
}

func parseFlexibleFrame(data []byte) (core.ActionFrame, error) {
	var flex flexibleFrame
	if err := json.Unmarshal(data, &flex); err != nil {
		return core.ActionFrame{}, err
	}
	frame := core.ActionFrame{
		Actor:         flex.Actor,
		Action:        flex.Action,
		Target:        flex.Target,
		Intensity:     flex.Intensity,
		Emotion:       flex.Emotion,
		Intent:        flex.Intent,
		SuggestedLine: flex.SuggestedLine,
	}
	// Try []StateEffect first
	var effects []core.StateEffect
	if err := json.Unmarshal(flex.Effects, &effects); err == nil {
		frame.Effects = effects
		return frame, nil
	}
	// Fallback: []string like ["trust-0.5"]
	var strEffects []string
	if err := json.Unmarshal(flex.Effects, &strEffects); err == nil {
		for _, s := range strEffects {
			frame.Effects = append(frame.Effects, core.StateEffect{Path: s, Delta: 0})
		}
		return frame, nil
	}
	return frame, nil
}

// ExtractActionFrame pulls the JSON Action Frame from LLM output.
// Supports fenced code blocks (```json) and inline JSON.
func ExtractActionFrame(raw string) (core.ActionFrame, string, error) {
	raw = strings.TrimSpace(raw)

	// Pre-extract narrative so we always have it even if JSON parsing fails
	narrative := extractCodeBlock(raw, "text")
	if narrative == "" {
		narrative = extractAfterCodeBlocks(raw)
	}

	// Strategy 1: Extract from ```json ... ``` block
	jsonCodeBlock := extractCodeBlock(raw, "json")
	if jsonCodeBlock != "" {
		frame, err := parseFlexibleFrame([]byte(jsonCodeBlock))
		if err == nil && frame.Action != "" {
			return frame, strings.TrimSpace(narrative), nil
		}
		// JSON found but malformed — still use the narrative if we have it
		if narrative != "" {
			if frame.Action == "" {
				frame.Action = "speak"
			}
			if frame.Actor == "" {
				frame.Actor = "unknown"
			}
			return frame, strings.TrimSpace(narrative), nil
		}
	}

	// Strategy 2: Find first well-formed JSON object
	frame, nar := extractFirstJSON(raw)
	if frame.Action != "" || frame.Actor != "" {
		if nar == "" && frame.SuggestedLine != "" {
			nar = frame.SuggestedLine
		}
		return frame, nar, nil
	}

	// Strategy 3: Treat everything as narrative
	return core.ActionFrame{}, raw, nil
}

func extractCodeBlock(raw, lang string) string {
	prefix := "```" + lang + "\n"
	start := strings.Index(raw, prefix)
	if start == -1 {
		prefix = "```" + lang
		start = strings.Index(raw, prefix)
	}
	if start == -1 {
		return ""
	}
	start += len(prefix)
	end := strings.Index(raw[start:], "\n```")
	if end == -1 {
		end = strings.Index(raw[start:], "```")
	}
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(raw[start : start+end])
}

func extractAfterCodeBlocks(raw string) string {
	// Find the last ``` block end and take everything after it
	last := strings.LastIndex(raw, "```")
	if last == -1 {
		return ""
	}
	// Skip past the closing ```
	idx := last + 3
	if idx < len(raw) {
		return strings.TrimSpace(raw[idx:])
	}
	return ""
}

func extractFirstJSON(raw string) (core.ActionFrame, string) {
	// Scan for first valid JSON object
	for i := 0; i < len(raw); i++ {
		if raw[i] == '{' {
			for j := i + 1; j <= len(raw); j++ {
				if j == len(raw) || raw[j] == '}' {
					end := j + 1
					if end > len(raw) {
						end = len(raw)
					}
					candidate := raw[i:end]
					var frame core.ActionFrame
					if err := json.Unmarshal([]byte(candidate), &frame); err == nil {
						narrative := strings.TrimSpace(raw[j+1:])
						if narrative == "" {
							narrative = strings.TrimSpace(raw[:i])
						}
						return frame, narrative
					}
					break // This { ... } wasn't valid JSON, try next
				}
			}
		}
	}
	return core.ActionFrame{}, ""
}
