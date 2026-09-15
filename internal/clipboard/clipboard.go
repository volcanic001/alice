package clipboard

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Write copies text using the native clipboard command for the current
// supported platform. CommandContext terminates a backend that stops responding.
func Write(ctx context.Context, text string) error {
	var command string
	switch runtime.GOOS {
	case "android":
		command = "termux-clipboard-set"
	case "darwin":
		command = "pbcopy"
	default:
		return fmt.Errorf("clipboard unsupported on %s", runtime.GOOS)
	}

	cmd := exec.CommandContext(ctx, command)
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", command, err)
	}
	return nil
}
