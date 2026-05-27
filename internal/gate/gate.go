// Package gate executes a workflow gate: a shell command whose exit code
// deterministically decides whether the workflow advances or loops back.
package gate

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
)

// Result is the outcome of running a gate command.
type Result struct {
	Passed   bool   // true if the command exited 0
	ExitCode int    // process exit code (-1 if it could not start)
	Output   string // combined stdout+stderr, for feeding back to a fix step
}

// Run executes command via the system shell in workDir and reports the result.
// Combined stdout+stderr is captured so a fix step can see exactly what failed.
func Run(ctx context.Context, command, workDir string) Result {
	shell, flag := "sh", "-c"
	if runtime.GOOS == "windows" {
		shell, flag = "cmd", "/C"
	}
	cmd := exec.CommandContext(ctx, shell, flag, command)
	if workDir != "" {
		cmd.Dir = workDir
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	if err == nil {
		return Result{Passed: true, ExitCode: 0, Output: buf.String()}
	}
	exitCode := -1
	var exitErr *exec.ExitError
	if ok := asExitError(err, &exitErr); ok {
		exitCode = exitErr.ExitCode()
	}
	return Result{Passed: false, ExitCode: exitCode, Output: buf.String()}
}

// asExitError is a small wrapper around errors.As to keep Run readable.
func asExitError(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}
