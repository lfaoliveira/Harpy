// Package platform is Harpy's view/infrastructure layer: host paths, config
// files, log files, privileges and process execution. Models stay pure data;
// everything that touches the operating system lives here.
package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Command describes one external process invocation.
type Command struct {
	Name string
	Args []string
	Env  []string // appended to the host environment
	Dir  string
}

// Result is the captured output of a command.
type Result struct {
	Stdout string
	Stderr string
}

// Runner is the single process-execution port used by manager adapters.
type Runner interface {
	// Run executes a command and streams stdout and stderr to output.
	Run(ctx context.Context, command Command, output io.Writer) error
	// Output executes a command and captures stdout and stderr.
	Output(ctx context.Context, command Command) (Result, error)
	// LookPath reports whether an executable is available on PATH.
	LookPath(file string) (string, error)
}

// OSRunner runs commands on the host operating system.
type OSRunner struct{}

// NewRunner returns the process runner for the host OS.
func NewRunner() Runner { return OSRunner{} }

// Run executes a command, streaming stdout and stderr to output.
func (OSRunner) Run(ctx context.Context, command Command, output io.Writer) error {
	if output == nil {
		output = io.Discard
	}
	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	cmd.Env = mergedEnv(command.Env)
	cmd.Dir = command.Dir

	var stderr bytes.Buffer
	cmd.Stdout = output
	cmd.Stderr = io.MultiWriter(output, &stderr)

	if err := cmd.Run(); err != nil {
		return newCommandError(command, stderr.String(), err)
	}
	return nil
}

// Output executes a command and captures stdout and stderr.
func (OSRunner) Output(ctx context.Context, command Command) (Result, error) {
	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	cmd.Env = mergedEnv(command.Env)
	cmd.Dir = command.Dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		return result, newCommandError(command, result.Stderr, err)
	}
	return result, nil
}

// LookPath reports whether an executable is available on PATH.
func (OSRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func mergedEnv(extra []string) []string {
	if len(extra) == 0 {
		return nil // inherit the host environment
	}
	return append(os.Environ(), extra...)
}

// CommandError carries the exit code and stderr of a failed command.
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

// LastLine returns the last non-empty stderr line.
func (e *CommandError) LastLine() string {
	lines := strings.Split(strings.TrimSpace(e.Stderr), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

func newCommandError(command Command, stderr string, err error) *CommandError {
	code := -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	}
	return &CommandError{
		Name:   command.Name,
		Args:   append([]string(nil), command.Args...),
		Code:   code,
		Stderr: stderr,
		Err:    err,
	}
}

// ExitCode extracts the exit code from a CommandError.
func ExitCode(err error) (int, bool) {
	var cmdErr *CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.Code, true
	}
	return 0, false
}

// HostOS returns the current operating system name.
func HostOS() string { return runtime.GOOS }

// Hostname returns the host name, or "" when it cannot be resolved.
func Hostname() string {
	name, _ := os.Hostname()
	return name
}
