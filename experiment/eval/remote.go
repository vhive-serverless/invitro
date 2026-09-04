package eval

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	sshTargetPattern  = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9.-]+$`)
	resultRootPattern = regexp.MustCompile(`^/mnt/resources/nexus-evaluation/[A-Za-z0-9._/-]+$`)
)

func ValidateResultRoot(path string) error {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(path) || path != clean || !resultRootPattern.MatchString(clean) {
		return fmt.Errorf("result root must be a clean absolute path below /mnt/resources/nexus-evaluation")
	}
	return nil
}

func RemoteHome(target string) (string, error) {
	if !sshTargetPattern.MatchString(target) {
		return "", fmt.Errorf("invalid SSH target %q", target)
	}
	return "/users/" + strings.SplitN(target, "@", 2)[0], nil
}

func SSHCommand(ctx context.Context, target string, command ...string) (*exec.Cmd, error) {
	if _, err := RemoteHome(target); err != nil {
		return nil, err
	}
	args := []string{"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=10", target}
	quoted := make([]string, 0, len(command))
	for _, value := range command {
		quoted = append(quoted, shellQuote(value))
	}
	args = append(args, strings.Join(quoted, " "))
	return exec.CommandContext(ctx, "ssh", args...), nil
}

func RemoteAbsent(ctx context.Context, target, path string) error {
	command, err := SSHCommand(ctx, target, "test", "!", "-e", path)
	if err != nil {
		return err
	}
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("remote result root exists or is inaccessible: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func CopyRemoteTree(ctx context.Context, target, path string, output io.Writer) error {
	if _, err := RemoteHome(target); err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "scp", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=10", "-r", target+":"+path, filepath.Dir(path))
	command.Stdout, command.Stderr = output, output
	if err := command.Run(); err != nil {
		return fmt.Errorf("copy remote result: %w", err)
	}
	return nil
}
