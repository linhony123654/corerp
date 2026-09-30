package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"corerp.local/backend/internal/endpointpolicy"
)

type ProxyTestRequest struct {
	Endpoint       string  `json:"endpoint"`
	APIKey         string  `json:"apiKey"`
	Model          string  `json:"model"`
	TimeoutSeconds float64 `json:"timeoutSeconds"`
}

type ProxyTestResult struct {
	OK        bool   `json:"ok"`
	LatencyMs int64  `json:"latencyMs"`
	Message   string `json:"message"`
}

type ProxyModelsRequest struct {
	Endpoint       string  `json:"endpoint"`
	APIKey         string  `json:"apiKey"`
	TimeoutSeconds float64 `json:"timeoutSeconds"`
}

type ProxyModelsResult struct {
	OK      bool     `json:"ok"`
	Models  []string `json:"models"`
	Message string   `json:"message"`
}

func (s *Server) handleProxyTest(response http.ResponseWriter, request *http.Request, requestID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input ProxyTestRequest
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}

	endpoint := strings.TrimSpace(input.Endpoint)
	if endpoint == "" {
		writeData(response, http.StatusOK, ProxyTestResult{
			OK:        false,
			LatencyMs: 0,
			Message:   "请填写接口 Endpoint 地址",
		})
		return
	}
	canonicalEndpoint, canonicalErr := endpointpolicy.Canonicalize(endpoint)
	if canonicalErr != nil {
		writeData(response, http.StatusOK, ProxyTestResult{OK: false, Message: "Endpoint 格式无效"})
		return
	}
	endpoint = canonicalEndpoint
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		writeData(response, http.StatusOK, ProxyTestResult{
			OK:        false,
			LatencyMs: 0,
			Message:   "Endpoint 地址必须以 http:// 或 https:// 开头",
		})
		return
	}

	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = "deepseek-chat"
	}

	timeoutSec := input.TimeoutSeconds
	if timeoutSec <= 0 {
		timeoutSec = 15
	} else if timeoutSec < 2 {
		timeoutSec = 2
	} else if timeoutSec > 60 {
		timeoutSec = 60
	}

	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "Ping"},
		},
		"max_tokens":  5,
		"temperature": 0.1,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		writeData(response, http.StatusOK, ProxyTestResult{
			OK:      false,
			Message: fmt.Sprintf("构造请求失败: %v", err),
		})
		return
	}

	targetURL := resolveChatCompletionsURL(endpoint)
	targetURL, client, policyErr := s.endpointPolicy.PrepareChatCompletions(targetURL, time.Duration(timeoutSec*float64(time.Second)))
	if policyErr != nil {
		writeData(response, http.StatusOK, ProxyTestResult{OK: false, Message: "Endpoint 不在服务端允许范围内或地址不可安全访问"})
		return
	}
	apiKey := strings.TrimSpace(input.APIKey)

	sendPost := func(u string) (*http.Response, int64, error) {
		req, err := http.NewRequestWithContext(request.Context(), http.MethodPost, u, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		start := time.Now()
		resp, err := client.Do(req)
		latency := time.Since(start).Milliseconds()
		return resp, latency, err
	}

	resp, latency, err := sendPost(targetURL)
	if err != nil {
		writeData(response, http.StatusOK, ProxyTestResult{
			OK:        false,
			LatencyMs: latency,
			Message:   "网络连接失败，请检查服务端允许的 Endpoint 与网络配置",
		})
		return
	}

	// A 404 fallback is a new destination path and must pass the server policy too.
	if resp.StatusCode == http.StatusNotFound && strings.HasSuffix(targetURL, "/v1/chat/completions") {
		fallbackURL := strings.TrimSuffix(targetURL, "/v1/chat/completions") + "/chat/completions"
		if permittedURL, _, policyErr := s.endpointPolicy.PrepareChatCompletions(fallbackURL, time.Duration(timeoutSec*float64(time.Second))); policyErr == nil {
			if fallbackResp, fallbackLatency, fallbackErr := sendPost(permittedURL); fallbackErr == nil {
				if fallbackResp.StatusCode != http.StatusNotFound {
					resp.Body.Close()
					resp = fallbackResp
					latency = fallbackLatency
				} else {
					fallbackResp.Body.Close()
				}
			}
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		writeData(response, http.StatusOK, ProxyTestResult{
			OK:        true,
			LatencyMs: latency,
			Message:   fmt.Sprintf("连接成功 · 延时 %dms", latency),
		})
		return
	}

	writeData(response, http.StatusOK, ProxyTestResult{
		OK:        false,
		LatencyMs: latency,
		Message:   fmt.Sprintf("服务响应错误 (%d)", resp.StatusCode),
	})
}

func (s *Server) handleProxyModels(response http.ResponseWriter, request *http.Request, requestID string) {
	if !requireMethod(response, requestID, request, http.MethodPost) {
		return
	}
	var input ProxyModelsRequest
	if err := decodeJSON(response, request, s.maxBodyBytes, &input); err != nil {
		writeDecodeError(response, requestID, err)
		return
	}

	endpoint := strings.TrimSpace(input.Endpoint)
	if endpoint == "" {
		writeData(response, http.StatusOK, ProxyModelsResult{
			OK:      false,
			Models:  nil,
			Message: "请先填写接口 Endpoint 地址",
		})
		return
	}
	canonicalEndpoint, canonicalErr := endpointpolicy.Canonicalize(endpoint)
	if canonicalErr != nil {
		writeData(response, http.StatusOK, ProxyModelsResult{OK: false, Models: nil, Message: "Endpoint 格式无效"})
		return
	}
	endpoint = canonicalEndpoint
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		writeData(response, http.StatusOK, ProxyModelsResult{
			OK:      false,
			Models:  nil,
			Message: "Endpoint 地址必须以 http:// 或 https:// 开头",
		})
		return
	}

	timeoutSec := input.TimeoutSeconds
	if timeoutSec <= 0 {
		timeoutSec = 15
	} else if timeoutSec < 2 {
		timeoutSec = 2
	} else if timeoutSec > 60 {
		timeoutSec = 60
	}

	modelsURL := resolveModelsURL(endpoint)
	modelsURL, client, policyErr := s.endpointPolicy.PrepareModels(modelsURL, time.Duration(timeoutSec*float64(time.Second)))
	if policyErr != nil {
		writeData(response, http.StatusOK, ProxyModelsResult{OK: false, Models: nil, Message: "Endpoint 不在服务端允许范围内或地址不可安全访问"})
		return
	}
	apiKey := strings.TrimSpace(input.APIKey)

	fetchURL := func(targetURL string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(request.Context(), http.MethodGet, targetURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		return client.Do(req)
	}

	resp, err := fetchURL(modelsURL)
	if err != nil {
		writeData(response, http.StatusOK, ProxyModelsResult{
			OK:      false,
			Models:  nil,
			Message: "网络连接失败，请检查服务端允许的 Endpoint 与网络配置",
		})
		return
	}

	if resp.StatusCode == http.StatusNotFound && strings.HasSuffix(modelsURL, "/v1/models") {
		fallbackURL := strings.TrimSuffix(modelsURL, "/v1/models") + "/models"
		if permittedURL, _, policyErr := s.endpointPolicy.PrepareModels(fallbackURL, time.Duration(timeoutSec*float64(time.Second))); policyErr == nil {
			if fallbackResp, fallbackErr := fetchURL(permittedURL); fallbackErr == nil {
				if fallbackResp.StatusCode == http.StatusOK {
					resp.Body.Close()
					resp = fallbackResp
				} else {
					fallbackResp.Body.Close()
				}
			}
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeData(response, http.StatusOK, ProxyModelsResult{
			OK:      false,
			Models:  nil,
			Message: fmt.Sprintf("拉取失败 (%d)", resp.StatusCode),
		})
		return
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		writeData(response, http.StatusOK, ProxyModelsResult{
			OK:      false,
			Models:  nil,
			Message: "读取上游响应失败",
		})
		return
	}

	models := parseModelsFromJSON(bodyBytes)
	if len(models) == 0 {
		writeData(response, http.StatusOK, ProxyModelsResult{
			OK:      false,
			Models:  nil,
			Message: "接口返回正常，但未在返回数据中解析到模型名称",
		})
		return
	}

	writeData(response, http.StatusOK, ProxyModelsResult{
		OK:      true,
		Models:  models,
		Message: fmt.Sprintf("成功拉取到 %d 个模型", len(models)),
	})
}

func resolveChatCompletionsURL(endpoint string) string {
	clean := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if strings.HasSuffix(clean, "/chat/completions") {
		return clean
	}
	if strings.HasSuffix(clean, "/completions") {
		return clean[:len(clean)-len("/completions")] + "/chat/completions"
	}
	if strings.HasSuffix(clean, "/models") {
		return clean[:len(clean)-len("/models")] + "/chat/completions"
	}
	if strings.HasSuffix(clean, "/v1") {
		return clean + "/chat/completions"
	}
	if idx := strings.Index(clean, "/v1/"); idx != -1 {
		return clean[:idx] + "/v1/chat/completions"
	}
	return clean + "/v1/chat/completions"
}

func resolveModelsURL(endpoint string) string {
	clean := strings.TrimRight(endpoint, "/")
	if strings.HasSuffix(clean, "/chat/completions") {
		return clean[:len(clean)-len("/chat/completions")] + "/models"
	}
	if strings.HasSuffix(clean, "/completions") {
		return clean[:len(clean)-len("/completions")] + "/models"
	}
	if strings.HasSuffix(clean, "/models") {
		return clean
	}
	if strings.HasSuffix(clean, "/v1") {
		return clean + "/models"
	}
	if idx := strings.Index(clean, "/v1/"); idx != -1 {
		return clean[:idx] + "/v1/models"
	}
	return clean + "/v1/models"
}

func parseModelsFromJSON(body []byte) []string {
	var parsed struct {
		Data   []any `json:"data"`
		Models []any `json:"models"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}

	rawList := parsed.Data
	if len(rawList) == 0 {
		rawList = parsed.Models
	}

	seen := make(map[string]bool)
	var list []string

	for _, item := range rawList {
		var name string
		switch v := item.(type) {
		case string:
			name = v
		case map[string]any:
			if id, ok := v["id"].(string); ok {
				name = id
			} else if n, ok := v["name"].(string); ok {
				name = n
			}
		}
		name = strings.TrimSpace(name)
		if name != "" && !seen[name] {
			seen[name] = true
			list = append(list, name)
		}
	}

	sort.Strings(list)
	return list
}
