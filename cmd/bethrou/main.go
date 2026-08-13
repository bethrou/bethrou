package main

import (
	"fmt"
	"os"

	"github.com/bethrou/bethrou/internal/cmd"
)

func main() {
	// cmd.Execute() loads .env (if present) itself, before registering any
	// flags — see client/cmd/root.go.
	//
	// Deliberately fmt.Fprintln, not logging.Logger: client/tui redirects
	// the package-level logging.Logger to a log file for the duration of
	// the TUI session (see tui.Run), so routing this final fatal error
	// through it risks silently writing the one message the user most
	// needs to see into a file instead of the terminal.
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
