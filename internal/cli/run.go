package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/te2wow/koto/internal/config"
	"github.com/te2wow/koto/internal/engine"
	"github.com/te2wow/koto/internal/provider"
	"github.com/te2wow/koto/internal/runlog"
	"github.com/te2wow/koto/internal/ui"
	"github.com/te2wow/koto/internal/workflow"
)

func newRunCmd() *cobra.Command {
	var (
		wfName     string
		provName   string
		model      string
		jsonOut    bool
		dryRun     bool
		noInput    bool
		isolate    bool
		setVars    []string
	)

	cmd := &cobra.Command{
		Use:   "run <task>",
		Short: "Run a workflow on a task and loop until the gates pass",
		Long: "Run a workflow on the given task description. The task is the natural-language\n" +
			"goal handed to the agent. koto drives the workflow's state machine, running\n" +
			"quality gates and looping back to fix steps until they pass.\n\n" +
			"Examples:\n" +
			"  koto run \"add a /health endpoint\"\n" +
			"  koto run \"fix the failing parser\" --workflow fix-until-green --set test_cmd=\"go test ./...\"\n" +
			"  koto run \"refactor\" --dry-run",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return usageError{fmt.Errorf("a task description is required")}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			task := strings.Join(args, " ")

			wf, src, err := workflow.Resolve(wfName)
			if err != nil {
				return err
			}
			applyVarOverrides(wf, setVars)

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if provName != "" {
				cfg.Provider = provName
			}
			if model != "" {
				cfg.Model = model
			}

			prov, err := provider.Get(cfg.Provider)
			if err != nil {
				return usageError{err}
			}

			cwd, _ := os.Getwd()
			logger, err := runlog.New(cwd)
			if err != nil {
				return err
			}
			defer logger.Close()

			// Reporter: JSON to stderr if requested, else human (color iff stderr is a TTY).
			var reporter engine.Reporter
			if jsonOut {
				reporter = ui.NewJSON(os.Stderr)
			} else {
				reporter = ui.NewHuman(os.Stderr, ui.IsTTY(os.Stderr))
			}

			fmt.Fprintf(os.Stderr, "koto: workflow %q (%s), provider %q, task: %s\n",
				wf.Name, src, cfg.Provider, task)
			fmt.Fprintf(os.Stderr, "koto: run log → %s\n", logger.Dir)

			eng := &engine.Engine{
				WF:       wf,
				Provider: prov,
				Model:    cfg.Model,
				WorkDir:  cwd,
				Reporter: reporter,
				Approver: makeApprover(noInput),
				Log:      logger,
				DryRun:   dryRun,
				Isolate:  isolate,
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			outcome, runErr := eng.Run(ctx, task)
			printResult(jsonOut, wf.Name, outcome)
			if runErr != nil {
				return runErr
			}
			if outcome == engine.OutcomeMaxSteps {
				return fmt.Errorf("workflow stopped at max_steps without completing")
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&wfName, "workflow", "w", "default", "workflow name (local → user → builtin)")
	cmd.Flags().StringVarP(&provName, "provider", "p", "", "agent provider (overrides config): claude|codex|aider|gemini|copilot|mock")
	cmd.Flags().StringVarP(&model, "model", "m", "", "model passed to the provider (overrides config)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit machine-readable JSON events to stderr")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "trace the workflow without calling the agent or running gates")
	cmd.Flags().BoolVar(&noInput, "no-input", false, "never prompt; auto-approve approval steps")
	cmd.Flags().BoolVar(&isolate, "bare", false, "isolate the agent from the host's global config/hooks (claude --bare)")
	cmd.Flags().StringArrayVar(&setVars, "set", nil, "override a workflow var, e.g. --set test_cmd=\"go test ./...\"")
	return cmd
}

// applyVarOverrides merges --set key=value pairs into the workflow vars.
func applyVarOverrides(wf *workflow.Workflow, sets []string) {
	if len(sets) == 0 {
		return
	}
	if wf.Vars == nil {
		wf.Vars = map[string]string{}
	}
	for _, s := range sets {
		if k, v, ok := strings.Cut(s, "="); ok {
			wf.Vars[strings.TrimSpace(k)] = v
		}
	}
}

// makeApprover returns an Approver. With --no-input it auto-approves; otherwise
// it asks on the terminal (y/N).
func makeApprover(noInput bool) engine.Approver {
	if noInput || !ui.IsTTY(os.Stdin) {
		return func(string) bool { return true }
	}
	reader := bufio.NewReader(os.Stdin)
	return func(prompt string) bool {
		fmt.Fprintf(os.Stderr, "\n%s\nApprove? [y/N]: ", prompt)
		line, _ := reader.ReadString('\n')
		line = strings.ToLower(strings.TrimSpace(line))
		return line == "y" || line == "yes"
	}
}

// printResult writes the final outcome: JSON to stdout when --json, else a line.
func printResult(jsonOut bool, wfName string, outcome engine.Outcome) {
	if jsonOut {
		fmt.Printf(`{"event":"result","workflow":%q,"outcome":%q}`+"\n", wfName, outcome)
		return
	}
	fmt.Printf("outcome: %s\n", outcome)
}
