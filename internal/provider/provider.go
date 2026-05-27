// Package provider defines the agent provider interface and CLI-exec adapters.
// koto only ever exec's coding-agent CLIs, so it is never broken by SDK churn.
package provider

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Options configures a single agent invocation.
type Options struct {
	Model   string // provider model, empty = provider default
	WorkDir string // working directory for the agent process
	Edit    bool   // whether the step permits file edits
	Isolate bool   // skip the host's global agent config/hooks (e.g. claude --bare)
}

// Provider runs an agent with a prompt and returns its textual output.
type Provider interface {
	Name() string
	Run(ctx context.Context, prompt string, opts Options) (string, error)
}

// Get returns the provider registered under name.
func Get(name string) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "claude":
		return cliProvider{name: "claude", build: claudeArgs}, nil
	case "codex":
		return cliProvider{name: "codex", build: codexArgs}, nil
	case "aider":
		return cliProvider{name: "aider", build: aiderArgs}, nil
	case "gemini":
		return cliProvider{name: "gemini", build: geminiArgs}, nil
	case "copilot":
		return cliProvider{name: "copilot", build: copilotArgs}, nil
	case "mock":
		return NewMock(), nil
	default:
		return nil, fmt.Errorf("unknown provider %q", name)
	}
}

// argBuilder produces the binary and args for a given prompt/options.
type argBuilder func(prompt string, opts Options) (bin string, args []string, stdin string)

// cliProvider exec's a coding-agent CLI in non-interactive mode.
type cliProvider struct {
	name  string
	build argBuilder
}

func (p cliProvider) Name() string { return p.name }

func (p cliProvider) Run(ctx context.Context, prompt string, opts Options) (string, error) {
	bin, args, stdin := p.build(prompt, opts)
	if _, err := exec.LookPath(bin); err != nil {
		return "", fmt.Errorf("provider %q: %q not found in PATH (install it or set a different provider)", p.name, bin)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("provider %q failed: %w\n%s", p.name, err, stderr.String())
	}
	return stdout.String(), nil
}

// --- per-CLI argument builders (non-interactive / print modes) ---

func claudeArgs(prompt string, opts Options) (string, []string, string) {
	args := []string{"-p", prompt}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	// Edit steps need write permission; without it the agent can plan a fix but
	// cannot apply it non-interactively. Read-only steps stay in plan mode so the
	// agent cannot touch the working tree during reviews.
	if opts.Edit {
		args = append(args, "--permission-mode", "acceptEdits")
	} else {
		args = append(args, "--permission-mode", "plan")
	}
	// Isolation: skip the host user's global CLAUDE.md, hooks, and auto-memory so
	// koto's workflow prompt is the only instruction the agent follows. Without
	// this, a personal CLAUDE.md can inject unrelated behavior into every step.
	if opts.Isolate {
		args = append(args, "--bare")
	}
	return "claude", args, ""
}

func codexArgs(prompt string, opts Options) (string, []string, string) {
	args := []string{"exec", prompt}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	return "codex", args, ""
}

func aiderArgs(prompt string, opts Options) (string, []string, string) {
	args := []string{"--message", prompt, "--yes-always"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	return "aider", args, ""
}

func geminiArgs(prompt string, opts Options) (string, []string, string) {
	args := []string{"-p", prompt}
	if opts.Model != "" {
		args = append(args, "-m", opts.Model)
	}
	return "gemini", args, ""
}

func copilotArgs(prompt string, opts Options) (string, []string, string) {
	return "copilot", []string{"-p", prompt}, ""
}
