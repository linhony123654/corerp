package decision

import (
	"testing"
	"time"
)

func TestProviderConfigurationIsExplicitAndFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		env   map[string]string
		valid bool
		mode  string
	}{
		{"default", map[string]string{}, true, "deterministic"},
		{"explicit deterministic", map[string]string{"CORERP_DECISION_PROVIDER": "deterministic"}, true, "deterministic"},
		{"partial", map[string]string{"CORERP_LLM_API_KEY": "do-not-log"}, false, ""},
		{"unknown", map[string]string{"CORERP_DECISION_PROVIDER": "anything"}, false, ""},
		{"missing endpoint", map[string]string{"CORERP_DECISION_PROVIDER": "chat_completions"}, false, ""},
		{"local", map[string]string{"CORERP_DECISION_PROVIDER": "chat_completions", "CORERP_LLM_ENDPOINT": "http://127.0.0.1:9090/v1/chat/completions", "CORERP_LLM_MODEL": "local-model"}, true, "chat_completions"},
		{"remote cleartext", map[string]string{"CORERP_DECISION_PROVIDER": "chat_completions", "CORERP_LLM_ENDPOINT": "http://example.com/v1/chat/completions", "CORERP_LLM_MODEL": "remote", "CORERP_LLM_API_KEY": "test"}, false, ""},
		{"remote no key", map[string]string{"CORERP_DECISION_PROVIDER": "chat_completions", "CORERP_LLM_ENDPOINT": "https://example.com/v1/chat/completions", "CORERP_LLM_MODEL": "remote"}, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider, mode, err := FromEnvironment(func(k string) string { return tc.env[k] })
			if (err == nil) != tc.valid || mode != tc.mode || (tc.valid && provider == nil) {
				t.Fatalf("config provider=%T mode=%s err=%v", provider, mode, err)
			}
		})
	}
	for _, endpoint := range []string{"https://user:secret@example.com/v1/chat/completions", "https://example.com/v1/chat/completions?key=secret", "https://example.com/#fragment", "file:///tmp/model"} {
		if _, err := NewChatProvider(Config{Endpoint: endpoint, Model: "model", APIKey: "key"}); err == nil {
			t.Fatal("unsafe endpoint accepted")
		}
	}
	for _, budget := range []Config{
		{Timeout: -time.Second}, {Timeout: 2 * time.Minute}, {Attempts: -1}, {Attempts: 4},
	} {
		budget.Endpoint = "http://127.0.0.1/model"
		budget.Model = "model"
		if _, err := NewChatProvider(budget); err == nil {
			t.Fatal("invalid budget accepted")
		}
	}
	for _, setting := range []struct{ key, value string }{{"CORERP_LLM_ATTEMPTS", "0"}, {"CORERP_LLM_ATTEMPTS", "ten"}, {"CORERP_LLM_TIMEOUT", "0s"}, {"CORERP_LLM_TIMEOUT", "forever"}} {
		env := map[string]string{"CORERP_DECISION_PROVIDER": "chat_completions", "CORERP_LLM_ENDPOINT": "http://127.0.0.1/model", "CORERP_LLM_MODEL": "test", setting.key: setting.value}
		if _, _, err := FromEnvironment(func(k string) string { return env[k] }); err == nil {
			t.Fatal("invalid environment budget accepted")
		}
	}
}
