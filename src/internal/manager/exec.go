package manager

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/lfaoliveira/harpy/src/internal/model"
	"github.com/lfaoliveira/harpy/src/internal/platform"
)

// CommandError is kept as an alias so adapter tests and callers keep a stable
// name while the implementation lives in platform.
type CommandError = platform.CommandError

type base struct {
	runner platform.Runner
	env    []string
}

func newBase(runner platform.Runner, env ...string) base {
	if runner == nil {
		runner = platform.NewRunner()
	}
	return base{runner: runner, env: env}
}

// run executes a command, streaming stdout and stderr to w.
func (b base) run(ctx context.Context, w io.Writer, name string, args ...string) error {
	return b.runner.Run(ctx, platform.Command{Name: name, Args: args, Env: b.env}, w)
}

// output executes a command and captures stdout and stderr.
func (b base) output(ctx context.Context, name string, args ...string) (string, string, error) {
	result, err := b.runner.Output(ctx, platform.Command{Name: name, Args: args, Env: b.env})
	return result.Stdout, result.Stderr, err
}

func (b base) lookPath(name string) bool {
	_, err := b.runner.LookPath(name)
	return err == nil
}

func packageIDs(packages []model.Package) []string {
	ids := make([]string, 0, len(packages))
	for _, pkg := range packages {
		ids = append(ids, pkg.ID.String())
	}
	return ids
}

func exitCode(err error) (int, bool) { return platform.ExitCode(err) }

func asCommandError(err error, target **CommandError) bool {
	return errors.As(err, target)
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
