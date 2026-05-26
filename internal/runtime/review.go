package runtime

import (
	"fmt"

	"corerp/internal/core"
	"corerp/internal/memory"
)

func (e *Engine) ListQuarantineEvents(character string, limit int) ([]core.Event, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if character == "" {
		character = e.activeCharacter
	}
	return e.gatekeeper.ListPending(limit, character)
}

func (e *Engine) PromoteQuarantineEvent(eventID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.gatekeeper.Promote(eventID)
}

func (e *Engine) RejectQuarantineEvent(eventID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.gatekeeper.Reject(eventID)
}

func (e *Engine) ListPendingFacts(character string, limit int) ([]core.PendingFact, map[string]interface{}, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	cp := memory.NewConfidencePipelineForInstance(e.memEngine.DB(), e.memEngine.InstanceID())
	if cp == nil {
		return nil, nil, fmt.Errorf("pending facts pipeline is not available")
	}
	if character == "" {
		character = e.activeCharacter
	}
	items, err := cp.ListPending(limit, character)
	if err != nil {
		return nil, nil, err
	}
	return items, cp.PendingStats(), nil
}

func (e *Engine) ConfirmPendingFact(eventID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	cp := memory.NewConfidencePipelineForInstance(e.memEngine.DB(), e.memEngine.InstanceID())
	if cp == nil {
		return fmt.Errorf("pending facts pipeline is not available")
	}
	return cp.ConfirmPendingByID(eventID)
}

func (e *Engine) DeletePendingFact(eventID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	cp := memory.NewConfidencePipelineForInstance(e.memEngine.DB(), e.memEngine.InstanceID())
	if cp == nil {
		return fmt.Errorf("pending facts pipeline is not available")
	}
	return cp.DeletePendingByID(eventID)
}

func (e *Engine) PromotePendingFact(eventID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	cp := memory.NewConfidencePipelineForInstance(e.memEngine.DB(), e.memEngine.InstanceID())
	if cp == nil {
		return fmt.Errorf("pending facts pipeline is not available")
	}
	_, err := cp.PromotePendingByID(eventID)
	return err
}
