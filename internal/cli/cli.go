// Package cli wires the koto command-line interface (cobra) to the engine.
package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/te2wow/koto/internal/engine"
)

// Version is set at build time via -ldflags.
var Version = "dev"

// Exit codes (documented; agents can branch on them).
const (
	ExitOK          = 0
	ExitFailure     = 1
	ExitUsage       = 2
	ExitAbort       = 3 // workflow reached ABORT
	ExitGateExhaust = 4 // gate retries exhausted (a specific ABORT cause)
)

// usageError marks an error as a CLI usage problem (exit code 2).
type usageError struct{ err error }

func (u usageError) Error() string { return u.err.Error() }

// ExitCode maps an error to a process exit code.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	if _, ok := errors.AsType[usageError](err); ok {
		return ExitUsage
	}
	if errors.Is(err, engine.ErrAbort) {
		return ExitAbort
	}
	return ExitFailure
}

// NewRootCmd builds the root command with all subcommands attached.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "koto",
		Short: "Run AI coding agents through a YAML workflow until the gates are green",
		Long: "koto drives CLI coding agents (Claude Code, Codex, Aider, ...) through a\n" +
			"declarative YAML workflow. Its defining feature is quality gates enforced by\n" +
			"exit code: a gate runs a real command (tests, lint, types) and the workflow\n" +
			"does not complete until it passes.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       Version,
	}
	root.AddCommand(
		newRunCmd(),
		newListCmd(),
		newWorkflowsCmd(),
		newValidateCmd(),
		newInitCmd(),
		newVersionCmd(),
	)
	return root
}
