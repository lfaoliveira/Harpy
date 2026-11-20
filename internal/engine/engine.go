// Package engine implements the headless plan -> download -> install
// pipeline. Installation is strictly sequential; only package-manager
// internals are parallel.
package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lfaoliveira/harpy/internal/config"
	"github.com/lfaoliveira/harpy/internal/logx"
	"github.com/lfaoliveira/harpy/internal/manager"
)

// Status is the lifecycle state of one package in a plan.
type Status string

const (
	StatusToInstall Status = "to-install"
	StatusInstalled Status = "installed"
	StatusBlocked   Status = "blocked"
	StatusFailed    Status = "failed"
	StatusSkipped   Status = "skipped"
)

// Item is one app resolved to a concrete package manager.
type Item struct {
	App     string `json:"app"`
	Manager string `json:"manager,omitempty"`
	ID      string `json:"id,omitempty"`
	OS      string `json:"os,omitempty"`
	Status  Status `json:"status"`
	Note    string `json:"note,omitempty"`
	Error   string `json:"error,omitempty"`
	Log     string `json:"log,omitempty"`
}

// Summary aggregates item statuses.
type Summary struct {
	Total     int `json:"total"`
	ToInstall int `json:"to_install"`
	Installed int `json:"installed"`
	Blocked   int `json:"blocked"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

// Plan is the complete diff between desired and installed packages.
type Plan struct {
	Machine  string   `json:"machine,omitempty"`
	Profiles []string `json:"profiles,omitempty"`
	Items    []Item   `json:"items"`
	Summary  Summary  `json:"summary"`
	LogDir   string   `json:"log_dir,omitempty"`
}

// Recompute refreshes the summary after status changes.
func (p *Plan) Recompute() {
	s := Summary{Total: len(p.Items)}
	for _, item := range p.Items {
		switch item.Status {
		case StatusToInstall:
			s.ToInstall++
		case StatusInstalled:
			s.Installed++
		case StatusBlocked:
			s.Blocked++
		case StatusFailed:
			s.Failed++
		case StatusSkipped:
			s.Skipped++
		}
	}
	p.Summary = s
}

// Options configures an Engine.
type Options struct {
	HostOS     string
	Hostname   string
	PacmanSync string
	Verbose    bool
	Progress   io.Writer
	// Managers overrides detection for tests and future plug-ins. When nil,
	// host managers are used.
	Managers []manager.Manager
}

// Engine resolves configs and executes plans.
type Engine struct {
	Config   *config.Config
	HostOS   string
	Hostname string
	Managers []manager.Manager
	Verbose  bool
	Progress io.Writer
	byName   map[string]manager.Manager
}

// New builds an Engine for the given config and host.
func New(cfg *config.Config, opts Options) *Engine {
	hostOS := opts.HostOS
	if hostOS == "" {
		hostOS = runtime.GOOS
	}
	managers := opts.Managers
	if managers == nil {
		managers = manager.Host(manager.Options{PacmanSync: opts.PacmanSync})
	}
	e := &Engine{
		Config:   cfg,
		HostOS:   hostOS,
		Hostname: opts.Hostname,
		Managers: managers,
		Verbose:  opts.Verbose,
		Progress: opts.Progress,
		byName:   map[string]manager.Manager{},
	}
	for _, m := range managers {
		if m.OS() == hostOS {
			e.byName[m.Name()] = m
		}
	}
	return e
}

// Resolve selects apps using --machine or --profile.
func (e *Engine) Resolve(machine string, profiles []string) (*config.Selection, error) {
	return e.Config.Resolve(machine, profiles, e.HostOS, e.Hostname)
}

// Plan computes installed state and the desired diff.
func (e *Engine) Plan(ctx context.Context, selection *config.Selection) (*Plan, error) {
	plan := &Plan{Machine: selection.Machine, Profiles: selection.Profiles, Items: make([]Item, 0, len(selection.Apps))}
	detect := map[string][]int{}

	for _, app := range selection.Apps {
		mgr, id := e.selectManager(app)
		item := Item{App: app.Name, ID: id}
		if mgr == nil {
			item.Status = StatusBlocked
			item.Error = fmt.Sprintf("no supported package manager for %s", e.HostOS)
			plan.Items = append(plan.Items, item)
			continue
		}

		item.Manager = mgr.Name()
		item.OS = mgr.OS()
		if id == "" {
			item.Status = StatusBlocked
			item.Error = fmt.Sprintf("app %q has no id for %s", app.Name, mgr.Name())
			plan.Items = append(plan.Items, item)
			continue
		}
		if !mgr.Available() {
			item.Status = StatusBlocked
			item.Error = fmt.Sprintf("%s is not available on PATH", mgr.Name())
			plan.Items = append(plan.Items, item)
			continue
		}

		detect[mgr.Name()] = append(detect[mgr.Name()], len(plan.Items))
		plan.Items = append(plan.Items, item)
	}

	for _, mgr := range e.orderedManagers() {
		indexes := detect[mgr.Name()]
		if len(indexes) == 0 {
			continue
		}
		ids := uniqueIDs(plan.Items, indexes)
		installed, err := mgr.Installed(ctx, ids)
		if err != nil {
			for _, idx := range indexes {
				plan.Items[idx].Status = StatusFailed
				plan.Items[idx].Error = err.Error()
			}
			continue
		}
		for _, idx := range indexes {
			if installed[plan.Items[idx].ID] {
				plan.Items[idx].Status = StatusInstalled
				plan.Items[idx].Note = "already installed"
			} else {
				plan.Items[idx].Status = StatusToInstall
			}
		}
	}

	plan.Recompute()
	return plan, ctx.Err()
}

func uniqueIDs(items []Item, indexes []int) []string {
	seen := map[string]bool{}
	ids := make([]string, 0, len(indexes))
	for _, idx := range indexes {
		id := items[idx].ID
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

// selectManager applies defaults.managers precedence, preferring an available
// manager that has an explicit ID.
func (e *Engine) selectManager(app config.App) (manager.Manager, string) {
	ordered := e.orderedManagers()
	for _, m := range ordered {
		if id := app.IDFor(m.Name()); id != "" && m.Available() {
			return m, id
		}
	}
	for _, m := range ordered {
		if id := app.IDFor(m.Name()); id != "" {
			return m, id
		}
	}
	for _, m := range ordered {
		if m.Available() {
			return m, app.Name
		}
	}
	if len(ordered) > 0 {
		return ordered[0], app.Name
	}
	return nil, ""
}

func (e *Engine) orderedManagers() []manager.Manager {
	remaining := map[string]manager.Manager{}
	for name, m := range e.byName {
		remaining[name] = m
	}

	var ordered []manager.Manager
	add := func(name string) {
		if m, ok := remaining[name]; ok {
			ordered = append(ordered, m)
			delete(remaining, name)
		}
	}
	for _, name := range e.Config.Defaults.Managers {
		add(config.NormalizeManagerName(name))
	}
	for _, name := range manager.HostOrder() {
		add(name)
	}
	// Stable fallback for injected managers that are not in the host order.
	for _, m := range e.Managers {
		add(m.Name())
	}
	return ordered
}

// ApplyOptions configures Apply.
type ApplyOptions struct {
	// Download runs the optional pre-download phase.
	Download bool
	// ContinueOnError keeps installing after a package fails.
	ContinueOnError bool
	// LogDir overrides the generated run directory (useful in tests).
	LogDir string
}

// Apply downloads and installs pending packages sequentially. Fatal errors
// (elevation, cancelled context) are returned; per-package failures are
// recorded in the plan items.
func (e *Engine) Apply(ctx context.Context, plan *Plan, opts ApplyOptions) error {
	var pending []int
	for i := range plan.Items {
		if plan.Items[i].Status == StatusToInstall {
			pending = append(pending, i)
		}
	}
	if len(pending) == 0 {
		return nil
	}

	if err := e.checkElevation(plan, pending); err != nil {
		return err
	}

	logDir := opts.LogDir
	if logDir == "" {
		run, err := logx.NewRun()
		if err != nil {
			return err
		}
		logDir = run.Dir
	}
	plan.LogDir = logDir

	downloadFailed := map[string]bool{}
	if opts.Download {
		for _, mgr := range e.orderedManagers() {
			var indexes []int
			for _, idx := range pending {
				if plan.Items[idx].Manager == mgr.Name() && plan.Items[idx].Status == StatusToInstall {
					indexes = append(indexes, idx)
				}
			}
			if len(indexes) == 0 {
				continue
			}

			logPath := filepath.Join(logDir, logx.SafeName("download-"+mgr.Name())+".log")
			file, err := os.Create(logPath)
			if err != nil {
				for _, idx := range indexes {
					plan.Items[idx].Status = StatusFailed
					plan.Items[idx].Error = fmt.Sprintf("create download log: %v", err)
				}
				downloadFailed[mgr.Name()] = true
				continue
			}
			writeLogHeader(file, "download", mgr.Name(), fmt.Sprintf("%d packages", len(indexes)))
			w := e.logWriter(file)

			e.progressf("download  %-12s %d package(s)\n", mgr.Name(), len(indexes))
			ids := uniqueIDs(plan.Items, indexes)
			err = mgr.Download(ctx, ids, w)
			_ = file.Close()

			if errors.Is(err, manager.ErrNoDownload) {
				e.progressf("download  %-12s not supported; packages download during install\n", mgr.Name())
				continue
			}
			if err != nil {
				downloadFailed[mgr.Name()] = true
				for _, idx := range indexes {
					plan.Items[idx].Status = StatusFailed
					plan.Items[idx].Error = err.Error()
					plan.Items[idx].Log = logPath
				}
				e.progressf("download  %-12s failed: %v\n", mgr.Name(), err)
				if !opts.ContinueOnError {
					plan.Recompute()
					return nil
				}
			}
		}
	}

	stopped := false
	for _, idx := range pending {
		if err := ctx.Err(); err != nil {
			plan.Recompute()
			return err
		}
		item := &plan.Items[idx]
		if item.Status != StatusToInstall {
			continue
		}
		if stopped || downloadFailed[item.Manager] {
			if stopped {
				item.Status = StatusSkipped
				item.Error = "not attempted after a previous failure"
			}
			continue
		}

		mgr := e.byName[item.Manager]
		if mgr == nil {
			item.Status = StatusFailed
			item.Error = fmt.Sprintf("manager %q is not available", item.Manager)
			continue
		}

		logPath := filepath.Join(logDir, logx.SafeName(item.Manager+"-"+item.ID)+".log")
		file, err := os.Create(logPath)
		if err != nil {
			item.Status = StatusFailed
			item.Error = fmt.Sprintf("create log: %v", err)
			continue
		}
		writeLogHeader(file, "install", item.Manager, item.ID)
		tail := &tailWriter{w: e.logWriter(file)}

		e.progressf("install   %-12s %s\n", item.Manager, item.ID)
		err = mgr.Install(ctx, item.ID, tail)
		_ = file.Close()
		item.Log = logPath

		switch {
		case err == nil:
			item.Status = StatusInstalled
			item.Note = "installed"
		case errors.Is(err, manager.ErrAlreadyInstalled):
			item.Status = StatusInstalled
			item.Note = "already installed"
		case errors.Is(err, manager.ErrNoUpdate):
			item.Status = StatusInstalled
			item.Note = "no update available"
		case errors.Is(err, manager.ErrRebootRequired):
			item.Status = StatusInstalled
			item.Note = "installed; reboot required"
		default:
			if ctx.Err() != nil {
				plan.Recompute()
				return ctx.Err()
			}
			item.Status = StatusFailed
			item.Error = err.Error()
			if last := tail.lastLine(); last != "" && !strings.Contains(item.Error, last) {
				item.Error += " (" + last + ")"
			}
			e.progressf("failed    %-12s %s: %v\n", item.Manager, item.ID, err)
			if !opts.ContinueOnError {
				stopped = true
			}
		}
	}

	plan.Recompute()
	return nil
}

func (e *Engine) checkElevation(plan *Plan, pending []int) error {
	seen := map[string]bool{}
	for _, idx := range pending {
		name := plan.Items[idx].Manager
		if seen[name] {
			continue
		}
		seen[name] = true
		mgr := e.byName[name]
		if mgr == nil || !mgr.NeedsElevation() || manager.IsElevated() {
			continue
		}
		if e.HostOS == "windows" {
			return fmt.Errorf("%s requires administrator privileges; re-run harpy from an elevated terminal", name)
		}
		return fmt.Errorf("%s requires root privileges; re-run with sudo", name)
	}
	return nil
}

func (e *Engine) logWriter(file *os.File) io.Writer {
	if e.Verbose && e.Progress != nil {
		return io.MultiWriter(file, e.Progress)
	}
	return file
}

func (e *Engine) progressf(format string, args ...any) {
	if e.Progress != nil {
		fmt.Fprintf(e.Progress, format, args...)
	}
}

func writeLogHeader(w io.Writer, phase, subject, detail string) {
	fmt.Fprintf(w, "# harpy %s\n# subject: %s\n# detail: %s\n# started: %s\n\n",
		phase, subject, detail, time.Now().Format(time.RFC3339))
}

type tailWriter struct {
	w    io.Writer
	last string
}

func (t *tailWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(string(p), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > 500 {
			line = line[:500]
		}
		t.last = line
	}
	return t.w.Write(p)
}

func (t *tailWriter) lastLine() string { return t.last }
