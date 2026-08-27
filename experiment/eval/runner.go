package eval

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type Command struct {
	Name string
	Args []string
	Dir  string
	Env  []string
}
type Runner struct {
	DryRun         bool
	Stdout, Stderr io.Writer
}

func (r Runner) Run(ctx context.Context, command Command) error {
	out, errOut := r.Stdout, r.Stderr
	if out == nil {
		out = os.Stdout
	}
	if errOut == nil {
		errOut = os.Stderr
	}
	fmt.Fprintln(out, "COMMAND", renderCommand(command))
	if r.DryRun {
		return nil
	}
	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	cmd.Dir = command.Dir
	cmd.Stdout = out
	cmd.Stderr = errOut
	cmd.Env = append(os.Environ(), command.Env...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s: %w", command.Name, err)
	}
	return nil
}

func renderCommand(command Command) string {
	parts := make([]string, 0, len(command.Args)+1)
	for _, value := range append([]string{command.Name}, command.Args...) {
		parts = append(parts, shellQuote(value))
	}
	return strings.Join(parts, " ")
}
func shellQuote(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !(r == '_' || r == '-' || r == '.' || r == '/' || r == ':' || r == '=' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z')
	}) < 0 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
