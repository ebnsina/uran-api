package build

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

// runStream executes a command, streaming its combined stdout/stderr to logs,
// and returns an error if it exits non-zero. The command line is echoed first
// so build logs are self-describing.
func runStream(ctx context.Context, logs io.Writer, dir, name string, args ...string) error {
	fmt.Fprintf(logs, "$ %s %s\n", name, joinArgs(args))

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = logs
	cmd.Stderr = logs

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func joinArgs(args []string) string {
	out := ""
	for i, a := range args {
		if i > 0 {
			out += " "
		}
		out += a
	}
	return out
}
