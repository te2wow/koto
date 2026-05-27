// Package workflow defines the koto YAML workflow schema, parsing, and validation.
package workflow

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Reserved transition targets.
const (
	TargetComplete = "COMPLETE" // workflow finished successfully
	TargetAbort    = "ABORT"    // workflow failed
)

// StepType is the kind of a workflow step.
type StepType string

const (
	StepAgent   StepType = "agent"   // invoke a coding agent with a prompt
	StepGate    StepType = "gate"    // run a shell command; exit code decides routing
	StepApprove StepType = "approve" // pause for human approval
)

// Workflow is a declarative, finite-state-machine definition of an agent process.
type Workflow struct {
	Name     string            `yaml:"name"`
	Initial  string            `yaml:"initial"`
	MaxSteps int               `yaml:"max_steps"`
	Vars     map[string]string `yaml:"vars"`
	Steps    []Step            `yaml:"steps"`
}

// Step is a single node in the workflow state machine.
type Step struct {
	Name    string   `yaml:"name"`
	Type    StepType `yaml:"type"`
	Persona string   `yaml:"persona"` // prompt for agent steps
	Edit    bool     `yaml:"edit"`    // whether the agent is allowed to edit files

	// gate-step fields
	Run        string `yaml:"run"`         // shell command to execute
	MaxRetries int    `yaml:"max_retries"` // attempts before ABORT
	OnPass     string `yaml:"on_pass"`     // target when command exits 0
	OnFail     string `yaml:"on_fail"`     // target when command exits non-zero

	// agent-step transitions
	Rules []Rule `yaml:"rules"`

	// approve-step fields
	Prompt string `yaml:"prompt"` // message shown to the human
}

// Rule maps a marker found in agent output to the next step.
type Rule struct {
	On string `yaml:"on"` // substring marker to look for in agent output
	To string `yaml:"to"` // target step name (or reserved target)
}

// Load reads and parses a workflow YAML file, then validates it.
func Load(path string) (*Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read workflow %s: %w", path, err)
	}
	return Parse(data)
}

// Parse unmarshals workflow YAML bytes and validates the result.
func Parse(data []byte) (*Workflow, error) {
	var wf Workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("parse workflow yaml: %w", err)
	}
	if wf.MaxSteps == 0 {
		wf.MaxSteps = 20 // sensible default stopping condition
	}
	if err := wf.Validate(); err != nil {
		return nil, err
	}
	return &wf, nil
}

// Step returns the step with the given name, or nil.
func (w *Workflow) Step(name string) *Step {
	for i := range w.Steps {
		if w.Steps[i].Name == name {
			return &w.Steps[i]
		}
	}
	return nil
}

// isReserved reports whether target is a reserved terminal target.
func isReserved(target string) bool {
	return target == TargetComplete || target == TargetAbort
}
