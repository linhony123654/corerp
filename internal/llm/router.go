package llm

import (
	"fmt"
	"log"

	"corerp/internal/core"
)

// Task constants define the types of LLM work that can be routed.
const (
	TaskNarrative  = "narrative"  // main story generation — needs strongest model
	TaskSummary    = "summary"    // working memory / compression — cheap model OK
	TaskExtraction = "extraction" // fact extraction — medium model OK
)

// Router distributes LLM calls across adapters based on task type.
// Supports fallback: if a task's assigned adapter fails, the router
// tries the fallback adapter before returning an error.
type Router struct {
	adapters        map[string]*Adapter // name → adapter
	routes          map[string]string   // task → adapter name
	fallbackAdapter string
}

// NewRouter creates a router with a single default adapter.
func NewRouter(defaultAdapter *Adapter) *Router {
	return &Router{
		adapters: map[string]*Adapter{
			"default": defaultAdapter,
		},
		routes: map[string]string{
			TaskNarrative:  "default",
			TaskSummary:    "default",
			TaskExtraction: "default",
		},
		fallbackAdapter: "default",
	}
}

// AddAdapter registers a named adapter for routing.
func (r *Router) AddAdapter(name string, adapter *Adapter) {
	r.adapters[name] = adapter
}

// UpdateAdapter replaces an existing adapter (or adds if new).
// Used for hot-swapping LLM configs without restart.
func (r *Router) UpdateAdapter(name, endpoint, apiKey, model string) {
	r.adapters[name] = NewAdapter(endpoint, apiKey, model)
}

// SetRoute maps a task to a specific adapter.
func (r *Router) SetRoute(task, adapterName string) error {
	if _, ok := r.adapters[adapterName]; !ok {
		return fmt.Errorf("adapter '%s' not registered", adapterName)
	}
	r.routes[task] = adapterName
	return nil
}

// SetFallback sets the fallback adapter name.
func (r *Router) SetFallback(name string) error {
	if _, ok := r.adapters[name]; !ok {
		return fmt.Errorf("adapter '%s' not registered", name)
	}
	r.fallbackAdapter = name
	return nil
}

// Generate routes a generation request to the appropriate adapter.
func (r *Router) Generate(task, prompt string, onChunk func(core.LLMStreamChunk)) error {
	adapter := r.adapterFor(task)

	err := adapter.Generate(prompt, onChunk)
	if err != nil {
		// Try fallback if different from primary
		fallback := r.adapters[r.fallbackAdapter]
		if fallback != nil && fallback != adapter {
			log.Printf("[router] task '%s' failed on primary, trying fallback: %v", task, err)
			err2 := fallback.Generate(prompt, onChunk)
			if err2 == nil {
				p, c := fallback.Usage()
				LogUsage(task, fallback.Model(), p, c)
			}
			return err2
		}
		return err
	}
	p, c := adapter.Usage()
	LogUsage(task, adapter.Model(), p, c)
	return nil
}

// GenerateNonStream routes a non-streaming request to the appropriate adapter.
func (r *Router) GenerateNonStream(task string, messages []core.LLMMessage) (string, error) {
	adapter := r.adapterFor(task)

	result, err := adapter.GenerateNonStream(messages)
	if err != nil {
		fallback := r.adapters[r.fallbackAdapter]
		if fallback != nil && fallback != adapter {
			log.Printf("[router] task '%s' nonstream failed on primary, trying fallback: %v", task, err)
			result2, err2 := fallback.GenerateNonStream(messages)
			if err2 == nil {
				p, c := fallback.Usage()
				LogUsage(task, fallback.Model(), p, c)
			}
			return result2, err2
		}
		return "", err
	}
	p, c := adapter.Usage()
	LogUsage(task, adapter.Model(), p, c)
	return result, nil
}

// Routes returns the current routing table for inspection.
func (r *Router) Routes() map[string]string {
	cp := make(map[string]string, len(r.routes))
	for k, v := range r.routes {
		cp[k] = v
	}
	return cp
}

// Adapters returns registered adapter names.
func (r *Router) Adapters() []string {
	var names []string
	for name := range r.adapters {
		names = append(names, name)
	}
	return names
}

func (r *Router) adapterFor(task string) *Adapter {
	name, ok := r.routes[task]
	if !ok {
		name = r.fallbackAdapter
	}
	adapter, ok := r.adapters[name]
	if !ok {
		return r.adapters["default"]
	}
	return adapter
}
