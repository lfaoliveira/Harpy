package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/lfaoliveira/harpy/src/internal/config"
	"github.com/lfaoliveira/harpy/src/internal/engine"
	"github.com/lfaoliveira/harpy/src/internal/manager"
	"github.com/lfaoliveira/harpy/src/internal/paths"
)

func (a *application) commandInit(args []string) (int, error) {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	var path string
	var force, toStdout, jsonOut bool
	fs.StringVar(&path, "path", "", "write the config to this path")
	fs.BoolVar(&force, "force", false, "overwrite an existing config")
	fs.BoolVar(&toStdout, "stdout", false, "print the template instead of writing a file")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil
		}
		return 2, err
	}
	if fs.NArg() > 0 {
		return 2, fmt.Errorf("init takes no positional arguments (got %v)", fs.Args())
	}
	if toStdout {
		if jsonOut {
			return 2, errors.New("--stdout and --json cannot be combined")
		}
		fmt.Fprint(a.stdout, config.DefaultTemplate)
		return 0, nil
	}

	if path == "" {
		var err error
		path, err = paths.ConfigPath()
		if err != nil {
			return 1, err
		}
	}
	if _, err := os.Stat(path); err == nil && !force {
		return 1, fmt.Errorf("config already exists at %s (use --force to overwrite)", path)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 1, fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(config.DefaultTemplate), 0o644); err != nil {
		return 1, fmt.Errorf("write config: %w", err)
	}

	if jsonOut {
		return 0, a.writeJSON("init", map[string]any{"path": path})
	}
	fmt.Fprintf(a.stdout, "wrote %s\n\nNext steps:\n  harpy plan\n  harpy apply --yes\n", path)
	return 0, nil
}

type managerInfo struct {
	Name              string `json:"name"`
	OS                string `json:"os"`
	Host              bool   `json:"host"`
	Available         bool   `json:"available"`
	ElevationRequired bool   `json:"elevation_required"`
}

func (a *application) commandManagers(args []string) (int, error) {
	fs := flag.NewFlagSet("managers", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	var all, jsonOut bool
	fs.BoolVar(&all, "all", false, "show managers for every OS")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil
		}
		return 2, err
	}
	if fs.NArg() > 0 {
		return 2, fmt.Errorf("managers takes no positional arguments (got %v)", fs.Args())
	}

	var infos []managerInfo
	for _, m := range manager.All(manager.Options{}) {
		host := m.OS() == runtime.GOOS
		if !all && !host {
			continue
		}
		infos = append(infos, managerInfo{
			Name:              m.Name(),
			OS:                m.OS(),
			Host:              host,
			Available:         m.Available(),
			ElevationRequired: m.NeedsElevation(),
		})
	}

	elevated := manager.IsElevated()
	if jsonOut {
		return 0, a.writeJSON("managers", map[string]any{
			"os":       runtime.GOOS,
			"elevated": elevated,
			"managers": infos,
		})
	}

	tw := tabwriter.NewWriter(a.stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tOS\tHOST\tAVAILABLE\tELEVATION")
	for _, info := range infos {
		host := "no"
		if info.Host {
			host = "yes"
		}
		available := "no"
		if info.Available {
			available = "yes"
		}
		elevation := "not required"
		if info.ElevationRequired {
			elevation = "required"
			if info.Host && elevated {
				elevation = "required (active)"
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", info.Name, info.OS, host, available, elevation)
	}
	tw.Flush()
	fmt.Fprintf(a.stdout, "\nhost: %s · elevated: %t\n", runtime.GOOS, elevated)
	return 0, nil
}

func (a *application) commandPlan(args []string) (int, error) {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	var flags configFlags
	var jsonOut bool
	addConfigFlags(fs, &flags)
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil
		}
		return 2, err
	}
	if fs.NArg() > 0 {
		return 2, fmt.Errorf("plan takes no positional arguments (got %v)", fs.Args())
	}

	cfg, _, err := a.loadConfig(flags)
	if err != nil {
		return 1, err
	}
	hostname, _ := os.Hostname()
	eng := engine.New(cfg, engine.Options{
		HostOS:   runtime.GOOS,
		Hostname: hostname,
	})
	selection, err := eng.Resolve(flags.machine, profileList(flags.profiles))
	if err != nil {
		return 1, err
	}
	plan, err := eng.Plan(context.Background(), selection)
	if err != nil {
		return 1, err
	}

	if jsonOut {
		return 0, a.writeJSON("plan", plan)
	}
	printPlan(a.stdout, plan)
	return 0, nil
}

func (a *application) commandApply(args []string) (int, error) {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	var flags configFlags
	var jsonOut, yes bool
	var download, continueOnError, verbose bool
	var pacmanSync string
	addConfigFlags(fs, &flags)
	fs.BoolVar(&jsonOut, "json", false, "emit JSON")
	fs.BoolVar(&yes, "yes", false, "do not ask for confirmation")
	fs.BoolVar(&download, "download", true, "pre-download packages where the manager supports it")
	fs.BoolVar(&continueOnError, "continue-on-error", true, "keep installing after a package fails")
	fs.BoolVar(&verbose, "verbose", false, "stream package-manager output")
	fs.StringVar(&pacmanSync, "pacman-sync", "pkg", "pacman sync policy: pkg or full")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil
		}
		return 2, err
	}
	if fs.NArg() > 0 {
		return 2, fmt.Errorf("apply takes no positional arguments (got %v)", fs.Args())
	}
	if pacmanSync != "pkg" && pacmanSync != "full" {
		return 2, fmt.Errorf("--pacman-sync must be pkg or full (got %q)", pacmanSync)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cfg, _, err := a.loadConfig(flags)
	if err != nil {
		return 1, err
	}
	if !flagWasSet(fs, "continue-on-error") {
		continueOnError = cfg.Defaults.Continue()
	}
	hostname, _ := os.Hostname()
	progress := a.stdout
	if jsonOut {
		progress = a.stderr
	}
	eng := engine.New(cfg, engine.Options{
		HostOS:     runtime.GOOS,
		Hostname:   hostname,
		PacmanSync: pacmanSync,
		Verbose:    verbose,
		Progress:   progress,
	})
	selection, err := eng.Resolve(flags.machine, profileList(flags.profiles))
	if err != nil {
		return 1, err
	}
	plan, err := eng.Plan(ctx, selection)
	if err != nil {
		return 1, err
	}

	if !jsonOut {
		printPlan(a.stdout, plan)
	}

	if plan.Summary.ToInstall == 0 {
		if jsonOut {
			if err := a.writeJSON("apply", plan); err != nil {
				return 1, err
			}
		} else if plan.Summary.Blocked > 0 || plan.Summary.Failed > 0 {
			fmt.Fprintln(a.stdout, "Nothing installable; see blocked/failed items above.")
		} else {
			fmt.Fprintln(a.stdout, "Everything is already installed.")
		}
		if plan.Summary.Blocked > 0 || plan.Summary.Failed > 0 {
			return 1, nil
		}
		return 0, nil
	}

	if name, ok := elevationRequired(plan); ok {
		if runtime.GOOS == "windows" {
			return 1, fmt.Errorf("%s requires administrator privileges; re-run from an elevated terminal: %s", name, strings.Join(os.Args, " "))
		}
		return 1, fmt.Errorf("%s requires root privileges; re-run: sudo %s", name, strings.Join(os.Args, " "))
	}

	if !yes {
		fmt.Fprintln(a.stderr)
		ok, err := a.confirm(fmt.Sprintf("Install %d package(s)?", plan.Summary.ToInstall))
		if err != nil {
			return 1, err
		}
		if !ok {
			fmt.Fprintln(a.stderr, "aborted; nothing changed")
			return 1, nil
		}
	}

	if err := eng.Apply(ctx, plan, engine.ApplyOptions{
		Download:        download,
		ContinueOnError: continueOnError,
	}); err != nil {
		return 1, err
	}

	if jsonOut {
		if err := a.writeJSON("apply", plan); err != nil {
			return 1, err
		}
	} else {
		fmt.Fprintln(a.stdout)
		fmt.Fprintln(a.stdout, summaryLine(plan.Summary))
		if plan.LogDir != "" {
			fmt.Fprintf(a.stdout, "logs: %s\n", plan.LogDir)
		}
	}

	if plan.Summary.Failed > 0 {
		return 1, nil
	}
	return 0, nil
}

func elevationRequired(plan *engine.Plan) (string, bool) {
	if manager.IsElevated() {
		return "", false
	}
	for _, item := range plan.Items {
		if item.Status != engine.StatusToInstall {
			continue
		}
		mgr, ok := manager.ByName(item.Manager, manager.Options{})
		if ok && mgr.NeedsElevation() {
			return mgr.Name(), true
		}
	}
	return "", false
}
