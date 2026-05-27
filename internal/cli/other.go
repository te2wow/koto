package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/te2wow/koto/internal/runlog"
	"github.com/te2wow/koto/internal/workflow"
)

func newWorkflowsCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "workflows",
		Short: "List available workflows (local → user → builtin)",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries := workflow.List()
			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(entries)
			}
			if len(entries) == 0 {
				fmt.Println("no workflows found")
				return nil
			}
			for _, e := range entries {
				fmt.Printf("%-24s %s\n", e.Name, e.Source)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	return cmd
}

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <file.yaml>",
		Short: "Validate a workflow YAML file",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return usageError{fmt.Errorf("exactly one workflow file is required")}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			wf, err := workflow.Load(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("ok: %q is valid (%d steps, initial %q)\n", wf.Name, len(wf.Steps), wf.Initial)
			return nil
		},
	}
}

func newListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List previous runs in this project",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, _ := os.Getwd()
			base := runlog.RunsBaseDir(cwd)
			dirs, err := filepath.Glob(filepath.Join(base, "*"))
			if err != nil {
				return err
			}
			sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
			type run struct {
				ID   string `json:"id"`
				Path string `json:"path"`
			}
			var runs []run
			for _, d := range dirs {
				info, err := os.Stat(d)
				if err != nil || !info.IsDir() {
					continue
				}
				runs = append(runs, run{ID: filepath.Base(d), Path: d})
			}
			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(runs)
			}
			if len(runs) == 0 {
				fmt.Println("no runs yet")
				return nil
			}
			for _, r := range runs {
				fmt.Printf("%s  %s\n", r.ID, r.Path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "output as JSON")
	return cmd
}

func newInitCmd() *cobra.Command {
	var wfName string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold .koto/ with a starter workflow",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := filepath.Join(".koto", "workflows")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			data, err := workflow.BuiltinBytes(wfName)
			if err != nil {
				return usageError{fmt.Errorf("unknown builtin workflow %q", wfName)}
			}
			dest := filepath.Join(dir, wfName+".yaml")
			if _, err := os.Stat(dest); err == nil {
				return fmt.Errorf("%s already exists; refusing to overwrite", dest)
			}
			if err := os.WriteFile(dest, data, 0o644); err != nil {
				return err
			}
			fmt.Printf("created %s\n", dest)
			fmt.Println("edit the workflow, set vars.test_cmd, then: koto run \"<task>\"")
			return nil
		},
	}
	cmd.Flags().StringVarP(&wfName, "workflow", "w", "default", "builtin workflow to copy (default|fix-until-green)")
	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the koto version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("koto %s\n", Version)
			return nil
		},
	}
}
