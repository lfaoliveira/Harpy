package manager

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// CommandError carries the exit code and stderr of a failed manager command.
type CommandError struct {
	Name   string
	Args   []string
	Code   int
	Stderr string
	Err    error
}

func (e *CommandError) Error() string {
	cmd := strings.TrimSpace(e.Name + " " + strings.Join(e.Args, " "))
	switch {
	case e.Code > 255:
		return fmt.Sprintf("%s: exit code %d (0x%X)", cmd, e.Code, uint32(e.Code))
	case e.Code >= 0:
		return fmt.Sprintf("%s: exit code %d", cmd, e.Code)
	default:
		return fmt.Sprintf("%s: %v", cmd, e.Err)
	}
}

func (e *CommandError) Unwrap() error { return e.Err }

// LastLine returns the last non-empty stderr line, which is what the summary
// shows for failed packages.
func (e *CommandError) LastLine() string {
	lines := strings.Split(strings.TrimSpace(e.Stderr), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

type base struct {
	env []string
}

// run executes a command, streaming stdout and stderr to w.
func (b base) run(ctx context.Context, w io.Writer, name string, args ...string) error {
	if w == nil {
		w = io.Discard
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), b.env...)

	var stderr bytes.Buffer
	cmd.Stdout = w
	cmd.Stderr = io.MultiWriter(w, &stderr)

	err := cmd.Run()
	if err == nil {
		return nil
	}
	return newCommandError(name, args, stderr.String(), err)
}

// output executes a command and captures stdout and stderr.
func (b base) output(ctx context.Context, name string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), b.env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return stdout.String(), stderr.String(), newCommandError(name, args, stderr.String(), err)
	}
	return stdout.String(), stderr.String(), nil
}

func newCommandError(name string, args []string, stderr string, err error) *CommandError {
	code := -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	}
	return &CommandError{
		Name:   name,
		Args:   append([]string(nil), args...),
		Code:   code,
		Stderr: stderr,
		Err:    err,
	}
}

func exitCode(err error) (int, bool) {
	var cmdErr *CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Code, true
	}
	return 0, false
}

// wrapExit translates a manager-specific exit code into a sentinel error.
func wrapExit(err error, codes map[int]error) error {
	if err == nil {
		return nil
	}
	code, ok := exitCode(err)
	if !ok {
		return err
	}
	if target, found := codes[code]; found {
		return fmt.Errorf("%w: %v", target, err)
	}
	return err
}

func asCommandError(err error, target **CommandError) bool {
	return errors.As(err, target)
}
