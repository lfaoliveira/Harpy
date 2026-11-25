package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/lfaoliveira/harpy/src/internal/domain"
	"github.com/lfaoliveira/harpy/src/internal/manager"
	"github.com/lfaoliveira/harpy/src/internal/model"
	"github.com/lfaoliveira/harpy/src/internal/platform"
)

func (a *Application) commandInit(args []string) (int, error) {
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
		fmt.Fprint(a.stdout, model.DefaultTemplate)
		return 0, nil
	}

	if path == "" {
		var err error
		path, err = a.configs.Path()
		if err != nil {
			return 1, err
		}
	}
	if exists, _ := a.configs.Exists(path); exists && !force {
		return 1, fmt.Errorf("config already exists at %s (use --force to overwrite)", platform.ToSlash(path))
	}

	if err := a.configs.Write(path, []byte(model.DefaultTemplate)); err != nil {
		return 1, err
	}

	if jsonOut {
		return 0, a.writeJSON("init", map[string]any{"path": platform.ToSlash(path)})
	}
	fmt.Fprintf(a.stdout, "wrote %s\n\nNext steps:\n  harpy plan\n  harpy apply --yes\n", platform.ToSlash(path))
	return 0, nil
}

func (a *Application) commandManagers(args []string) (int, error) {
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

	elevated := platform.IsElevated()
	infos := domain.ManagerInfos(a.managers(manager.Options{}), platform.HostOS(), elevated, all)
	if jsonOut {
		return 0, a.writeJSON("managers", map[string]any{
			"os":       platform.HostOS(),
			"elevated": elevated,
			"managers": infos,
		})
	}

	printManagers(a.stdout, infos, elevated)
	return 0, nil
}

func (a *Application) commandPlan(args []string) (int, error) {
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
	svc := a.service(cfg, "", false, nil)
	selection, err := svc.Resolve(flags.machine, splitProfiles(flags.profiles))
	if err != nil {
		return 1, err
	}
	plan, err := svc.Plan(context.Background(), selection)
	if err != nil {
		return 1, err
	}

	if jsonOut {
		return 0, a.writeJSON("plan", planPayload(plan))
	}
	printPlan(a.stdout, plan)
	return 0, nil
}

func (a *Application) commandApply(args []string) (int, error) {
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
	progress := a.stdout
	if jsonOut {
		progress = a.stderr
	}
	svc := a.service(cfg, pacmanSync, verbose, progress)
	selection, err := svc.Resolve(flags.machine, splitProfiles(flags.profiles))
	if err != nil {
		return 1, err
	}
	plan, err := svc.Plan(ctx, selection)
	if err != nil {
		return 1, err
	}

	if !jsonOut {
		printPlan(a.stdout, plan)
	}

	if plan.Summary.ToInstall == 0 {
		if jsonOut {
			if err := a.writeJSON("apply", planPayload(plan)); err != nil {
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

	if name, ok := svc.ElevationRequired(plan); ok {
		if platform.HostOS() == "windows" {
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

	if err := svc.Apply(ctx, plan, domain.ApplyOptions{
		Download:        download,
		ContinueOnError: continueOnError,
	}); err != nil {
		return 1, err
	}

	if jsonOut {
		if err := a.writeJSON("apply", planPayload(plan)); err != nil {
			return 1, err
		}
	} else {
		fmt.Fprintln(a.stdout)
		fmt.Fprintln(a.stdout, summaryLine(plan.Summary))
		if plan.LogDir != "" {
			fmt.Fprintf(a.stdout, "logs: %s\n", platform.ToSlash(plan.LogDir))
		}
	}

	if plan.Summary.Failed > 0 {
		return 1, nil
	}
	return 0, nil
}
