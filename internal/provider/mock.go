package provider

import (
	"context"
	"sync"
)

// Mock is a deterministic, network-free provider for tests and dry runs.
// It returns scripted responses keyed by call order, falling back to Default.
type Mock struct {
	mu        sync.Mutex
	calls     int
	Responses []string // returned in order; index = call count
	Default   string   // returned when Responses is exhausted
	Prompts   []string // records every prompt it received
}

// NewMock returns a Mock that emits __DONE__ by default.
func NewMock() *Mock {
	return &Mock{Default: "__DONE__"}
}

func (m *Mock) Name() string { return "mock" }

// Run records the prompt and returns the next scripted response.
func (m *Mock) Run(_ context.Context, prompt string, _ Options) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Prompts = append(m.Prompts, prompt)
	i := m.calls
	m.calls++
	if i < len(m.Responses) {
		return m.Responses[i], nil
	}
	return m.Default, nil
}

// Calls reports how many times Run was invoked.
func (m *Mock) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}
