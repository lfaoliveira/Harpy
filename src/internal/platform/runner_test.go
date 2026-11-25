package platform

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
)

func TestOutputCapturesStdout(t *testing.T) {
	result, err := NewRunner().Output(context.Background(), Command{
		Name: "go",
		Args: []string{"env", "GOOS"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(result.Stdout); got != runtime.GOOS {
		t.Fatalf("GOOS = %q, want %q", got, runtime.GOOS)
	}
}

func TestRunStreamsOutput(t *testing.T) {
	var out bytes.Buffer
	err := NewRunner().Run(context.Background(), Command{
		Name: "go",
		Args: []string{"version"},
	}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "go version") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestLookPath(t *testing.T) {
	if _, err := NewRunner().LookPath("go"); err != nil {
		t.Fatalf("go not on PATH: %v", err)
	}
}

func TestCommandErrorLastLineAndExitCode(t *testing.T) {
	err := &CommandError{
		Name:   "apt-get",
		Args:   []string{"install"},
		Code:   100,
		Stderr: "E: first\nE: last line\n",
	}
	if got := err.LastLine(); got != "E: last line" {
		t.Fatalf("LastLine = %q", got)
	}
	if code, ok := ExitCode(err); !ok || code != 100 {
		t.Fatalf("ExitCode = %d, %t", code, ok)
	}
	var wrapped error = err
	var target *CommandError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As failed")
	}
}
