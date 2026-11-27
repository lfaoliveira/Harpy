// Package cli is Harpy's presentation layer: flag parsing, prompts, tables and
// the versioned JSON envelope. It depends on domain use cases, never on
// package-manager internals or raw filesystem calls.
package cli

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/lfaoliveira/harpy/src/internal/domain"
	"github.com/lfaoliveira/harpy/src/internal/manager"
	"github.com/lfaoliveira/harpy/src/internal/model"
	"github.com/lfaoliveira/harpy/src/internal/platform"
)

const schemaVersion = 1

// Version is the build version, overridable with -ldflags.
var Version = "0.1.0-dev"

// Application holds the CLI's input/output streams and external adapters.
type Application struct {
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
	configs platform.ConfigStore
	logs    domain.LogStore
	// managers is injectable so command tests never need real package
	// managers.
	managers func(manager.Options) []domain.Manager
}

// NewApp builds the production application.
func NewApp(stdin io.Reader, stdout, stderr io.Writer) *Application {
	return &Application{
		stdin:    stdin,
		stdout:   stdout,
		stderr:   stderr,
		configs:  platform.NewConfigStore(),
		logs:     platform.NewLogStore(),
		managers: manager.All,
	}
}

// Run executes one CLI invocation and returns its process exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	code, err := NewApp(stdin, stdout, stderr).dispatch(args)
	if err != nil {
		fmt.Fprintf(stderr, "harpy: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	return code
}

func (a *Application) dispatch(args []string) (int, error) {
	if len(args) == 0 {
		a.printHelp()
		return 0, nil
	}

	command := args[0]
	rest := args[1:]
	switch command {
	case "-h", "--help", "help":
		a.printHelp()
		return 0, nil
	case "-v", "--version", "version":
		return a.commandVersion(rest)
	case "init":
		return a.commandInit(rest)
	case "plan":
		return a.commandPlan(rest)
	case "apply":
		return a.commandApply(rest)
	case "managers":
		return a.commandManagers(rest)
	default:
		return 1, fmt.Errorf("unknown command %q (run \"harpy help\")", command)
	}
}

func (a *Application) printHelp() {
	fmt.Fprint(a.stdout, `harpy — universal package manager CLI

Usage:
  harpy <command> [flags]

Commands:
  init        Write a starter config file
  plan        Show the diff between desired and installed packages
  apply       Download and install missing packages sequentially
  managers    Show supported package managers and what is available
  version     Print version information
  help        Show this help

Core flags (plan/apply):
  --config PATH        Config file (default: per-user config path)
  --machine NAME       Select a machine entry from the config
  --profile A,B        Bypass machine matching and use these profiles
  --json               Emit a stable, versioned JSON document

Examples:
  harpy init
  harpy plan
  harpy plan --machine thinkpad
  harpy apply --yes
  harpy managers --all
  harpy plan --profile base,dev --json

Config: `+configPathForHelp()+`
Docs:   docs/TESTING.md for the Docker test harness
`)
}

func configPathForHelp() string {
	p, err := platform.ConfigPath()
	if err != nil {
		return "Not Found"
	}
	return platform.ToSlash(p)
}

func (a *Application) commandVersion(args []string) (int, error) {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	jsonOut := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil
		}
		return 2, err
	}

	data := map[string]any{
		"version": Version,
		"os":      platform.HostOS(),
		"arch":    runtime.GOARCH,
		"go":      runtime.Version(),
	}
	if *jsonOut {
		return 0, a.writeJSON("version", data)
	}
	fmt.Fprintf(a.stdout, "harpy %s (%s/%s, %s)\n", Version, platform.HostOS(), runtime.GOARCH, runtime.Version())
	return 0, nil
}

type configFlags struct {
	path     string
	machine  string
	profiles string
}

func addConfigFlags(fs *flag.FlagSet, flags *configFlags) {
	fs.StringVar(&flags.path, "config", "", "config file path (default: per-user config)")
	fs.StringVar(&flags.machine, "machine", "", "machine name from the config")
	fs.StringVar(&flags.profiles, "profile", "", "comma-separated profiles (bypasses machine matching)")
}

func (a *Application) loadConfig(flags configFlags) (*model.Config, string, error) {
	path := flags.path
	if path == "" {
		var err error
		path, err = a.configs.Path()
		if err != nil {
			return nil, "", err
		}
	}

	cfg, warnings, err := a.configs.Load(path)
	for _, warning := range warnings {
		fmt.Fprintf(a.stderr, "warning: %s\n", warning)
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, path, fmt.Errorf("config not found at %s; run \"harpy init\" or pass --config", platform.ToSlash(path))
		}
		return nil, path, fmt.Errorf("load config %s: %w", platform.ToSlash(path), err)
	}
	return cfg, path, nil
}

func (a *Application) service(cfg *model.Config, pacmanSync string, verbose bool, progress io.Writer) *domain.Service {
	return domain.New(cfg, domain.Options{
		HostOS:   platform.HostOS(),
		Hostname: platform.Hostname(),
		Managers: a.managers(manager.Options{PacmanSync: pacmanSync}),
		Elevated: platform.IsElevated(),
		Verbose:  verbose,
		Progress: progress,
		Logs:     a.logs,
	})
}

func (a *Application) confirm(prompt string) (bool, error) {
	file, ok := a.stdin.(*os.File)
	if !ok {
		return false, errors.New("confirmation requires a terminal; re-run with --yes")
	}
	info, err := file.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return false, errors.New("confirmation requires a terminal; re-run with --yes")
	}

	fmt.Fprintf(a.stderr, "%s [y/N] ", prompt)
	line, err := bufio.NewReader(a.stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes", "s", "sim":
		return true, nil
	default:
		return false, nil
	}
}

func splitProfiles(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
