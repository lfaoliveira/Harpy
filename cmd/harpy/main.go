package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/lfaoliveira/harpy/internal/config"
	"github.com/lfaoliveira/harpy/internal/engine"
	"github.com/lfaoliveira/harpy/internal/paths"
)

var version = "0.1.0-dev"

const schemaVersion = 1

type application struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func main() {
	app := &application{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr}
	code, err := app.dispatch(os.Args[1:])
	if err != nil {
		fmt.Fprintf(app.stderr, "harpy: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

func (a *application) dispatch(args []string) (int, error) {
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
	case "managers", "manager":
		return a.commandManagers(rest)
	default:
		return 1, fmt.Errorf("unknown command %q (run \"harpy help\")", command)
	}
}

func (a *application) printHelp() {
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
	p, err := paths.ConfigPath()
	if err != nil {
		return "(unresolved)"
	}
	return p
}

func (a *application) commandVersion(args []string) (int, error) {
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
		"version": version,
		"os":      runtime.GOOS,
		"arch":    runtime.GOARCH,
		"go":      runtime.Version(),
	}
	if *jsonOut {
		return 0, a.writeJSON("version", data)
	}
	fmt.Fprintf(a.stdout, "harpy %s (%s/%s, %s)\n", version, runtime.GOOS, runtime.GOARCH, runtime.Version())
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

func (a *application) loadConfig(flags configFlags) (*config.Config, string, error) {
	path := flags.path
	if path == "" {
		var err error
		path, err = paths.ConfigPath()
		if err != nil {
			return nil, "", err
		}
	}

	cfg, warnings, err := config.Load(path)
	for _, warning := range warnings {
		fmt.Fprintf(a.stderr, "warning: %s\n", warning)
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, path, fmt.Errorf("config not found at %s; run \"harpy init\" or pass --config", path)
		}
		return nil, path, fmt.Errorf("load config %s: %w", path, err)
	}
	return cfg, path, nil
}

func (a *application) writeJSON(command string, data any) error {
	envelope := struct {
		Schema  int    `json:"schema"`
		Command string `json:"command"`
		Version string `json:"version"`
		Data    any    `json:"data"`
	}{
		Schema:  schemaVersion,
		Command: command,
		Version: version,
		Data:    data,
	}
	encoder := json.NewEncoder(a.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(envelope)
}

func (a *application) confirm(prompt string) (bool, error) {
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

func profileList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return []string{raw}
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

func printPlan(w io.Writer, plan *engine.Plan) {
	header := "harpy plan — " + runtime.GOOS
	if plan.Machine != "" {
		header += " · machine: " + plan.Machine
	}
	fmt.Fprintln(w, header)
	if len(plan.Profiles) > 0 {
		fmt.Fprintf(w, "profiles: %s\n", strings.Join(plan.Profiles, ", "))
	}
	fmt.Fprintln(w)

	if len(plan.Items) == 0 {
		fmt.Fprintln(w, "No apps selected.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "STATUS\tMANAGER\tID\tAPP\tDETAIL")
	for _, item := range plan.Items {
		managerName := item.Manager
		if managerName == "" {
			managerName = "-"
		}
		id := item.ID
		if id == "" {
			id = "-"
		}
		detail := item.Note
		if item.Error != "" {
			detail = item.Error
		}
		if detail == "" {
			detail = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", item.Status, managerName, id, item.App, detail)
	}
	tw.Flush()
	fmt.Fprintln(w)
	fmt.Fprintln(w, summaryLine(plan.Summary))
}

func summaryLine(s engine.Summary) string {
	parts := []string{
		fmt.Sprintf("%d total", s.Total),
		fmt.Sprintf("%d to install", s.ToInstall),
		fmt.Sprintf("%d installed", s.Installed),
	}
	if s.Blocked > 0 {
		parts = append(parts, fmt.Sprintf("%d blocked", s.Blocked))
	}
	if s.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", s.Failed))
	}
	if s.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", s.Skipped))
	}
	return "Summary: " + strings.Join(parts, " · ")
}
