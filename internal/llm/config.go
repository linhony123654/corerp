package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// APIConfig holds one LLM API configuration.
type APIConfig struct {
	Name           string  `json:"name"`
	Endpoint       string  `json:"endpoint"`
	APIKey         string  `json:"api_key"`
	Model          string  `json:"model"`
	PromptPrice    float64 `json:"prompt_price"`    // ¥ per 1M tokens
	CompletionPrice float64 `json:"completion_price"` // ¥ per 1M tokens
}

// ConfigStore manages persisted LLM API configurations.
type ConfigStore struct {
	mu     sync.RWMutex
	path   string
	configs []APIConfig
}

var globalConfigStore *ConfigStore
var activeConfig APIConfig

// SetActiveConfig records the CLI/env-provided LLM config.
func SetActiveConfig(name, endpoint, apiKey, model string) {
	activeConfig = APIConfig{
		Name: name, Endpoint: endpoint, APIKey: apiKey, Model: model,
		PromptPrice: 1.0, CompletionPrice: 4.0, // DeepSeek defaults
	}
	// Also save to store for persistence
	if globalConfigStore != nil {
		globalConfigStore.Add(activeConfig)
	}
}

// GetActiveConfig returns the currently active LLM config.
func GetActiveConfig() APIConfig { return activeConfig }

// GetPricing returns prompt/completion prices for the active model.
func GetPricing() (prompt, completion float64) {
	if activeConfig.PromptPrice > 0 {
		return activeConfig.PromptPrice, activeConfig.CompletionPrice
	}
	return 1.0, 4.0
}

// InitConfigStore loads or creates the config file.
func InitConfigStore(path string) error {
	store := &ConfigStore{path: path, configs: []APIConfig{}}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &store.configs); err != nil {
			store.configs = []APIConfig{}
		}
	}
	globalConfigStore = store
	return nil
}

// GetConfigStore returns the global config store.
func GetConfigStore() *ConfigStore {
	if globalConfigStore == nil {
		globalConfigStore = &ConfigStore{configs: []APIConfig{}}
	}
	return globalConfigStore
}

// List returns all saved configs (API keys masked).
func (s *ConfigStore) List() []APIConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]APIConfig, len(s.configs))
	copy(result, s.configs)
	for i := range result {
		if len(result[i].APIKey) > 8 {
			result[i].APIKey = result[i].APIKey[:4] + "****" + result[i].APIKey[len(result[i].APIKey)-4:]
		} else if len(result[i].APIKey) > 0 {
			result[i].APIKey = "****"
		}
	}
	return result
}

// Get returns the full config by name (unmasked key).
func (s *ConfigStore) Get(name string) (*APIConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.configs {
		if c.Name == name {
			return &c, nil
		}
	}
	return nil, fmt.Errorf("config '%s' not found", name)
}

// Add adds or updates a config and persists.
func (s *ConfigStore) Add(cfg APIConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.configs {
		if c.Name == cfg.Name {
			s.configs[i] = cfg
			return s.save()
		}
	}
	s.configs = append(s.configs, cfg)
	return s.save()
}

// Remove deletes a config by name.
func (s *ConfigStore) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.configs {
		if c.Name == name {
			s.configs = append(s.configs[:i], s.configs[i+1:]...)
			return s.save()
		}
	}
	return fmt.Errorf("config '%s' not found", name)
}

func (s *ConfigStore) save() error {
	data, err := json.MarshalIndent(s.configs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}
