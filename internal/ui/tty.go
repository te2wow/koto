package ui

import (
	"os"

	"golang.org/x/term"
)

// IsTTY reports whether f is an interactive terminal.
func IsTTY(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
