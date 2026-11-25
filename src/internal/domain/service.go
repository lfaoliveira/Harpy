package domain

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"time"

	"github.com/lfaoliveira/harpy/src/internal/model"
)

// Options configures a Service.
type Options struct {
	HostOS   string
	Hostname string
	Managers []Manager
	Elevated bool
	Verbose  bool
	Progress io.Writer
	Logs     LogStore
}

// Service resolves configs and executes plans.
type Service struct {
	Config   *model.Config
	HostOS   string
	Hostname string
	Managers []Manager
	Verbose  bool
	Progress io.Writer

	elevated bool
	logs     LogStore
	byName   map[model.ManagerName]Manager
}

// New builds a Service for the given config and host.
func New(cfg *model.Config, opts Options) *Service {
	hostOS := opts.HostOS
	if hostOS == "" {
		hostOS = runtime.GOOS
	}

	s := &Service{
		Config:   cfg,
		HostOS:   hostOS,
		Hostname: opts.Hostname,
		Verbose:  opts.Verbose,
		Progress: opts.Progress,
		elevated: opts.Elevated,
		logs:     opts.Logs,
		byName:   map[model.ManagerName]Manager{},
	}
	for _, m := range opts.Managers {
		if m.OS() != hostOS {
			continue
		}
		s.Managers = append(s.Managers, m)
		s.byName[m.Name()] = m
	}
	return s
}

// Resolve selects apps using --machine or --profile.
func (s *Service) Resolve(machine string, profiles []string) (*model.Selection, error) {
	return s.Config.Resolve(machine, profiles, s.HostOS, s.Hostname)
}

// Plan computes installed state and the desired diff.
func (s *Service) Plan(ctx context.Context, selection *model.Selection) (*model.Plan, error) {
	plan := &model.Plan{
		Machine:  selection.Machine,
		Profiles: selection.Profiles,
		Items:    make([]model.Item, 0, len(selection.Apps)),
	}
	detect := map[model.ManagerName][]int{}

	for _, app := range selection.Apps {
		mgr, id := s.selectManager(app)
		item := model.Item{App: app.Name, ID: model.PackageID(id)}
		if mgr == nil {
			item.Status = model.StateBlocked
			item.Error = fmt.Sprintf("no supported package manager for %s", s.HostOS)
			plan.Items = append(plan.Items, item)
			continue
		}

		item.Manager = mgr.Name()
		item.OS = mgr.OS()
		if id == "" {
			item.Status = model.StateBlocked
			item.Error = fmt.Sprintf("app %q has no id for %s", app.Name, mgr.Name())
			plan.Items = append(plan.Items, item)
			continue
		}
		if !mgr.Available() {
			item.Status = model.StateBlocked
			item.Error = fmt.Sprintf("%s is not available on PATH", mgr.Name())
			plan.Items = append(plan.Items, item)
			continue
		}

		detect[mgr.Name()] = append(detect[mgr.Name()], len(plan.Items))
		plan.Items = append(plan.Items, item)
	}

	for _, mgr := range s.orderedManagers() {
		indexes := detect[mgr.Name()]
		if len(indexes) == 0 {
			continue
		}
		installed, err := mgr.Installed(ctx, uniquePackages(plan.Items, indexes))
		if err != nil {
			for _, idx := range indexes {
				plan.Items[idx].Status = model.StateFailed
				plan.Items[idx].Error = err.Error()
			}
			continue
		}
		for _, idx := range indexes {
			if installed[plan.Items[idx].ID] {
				plan.Items[idx].Status = model.StateInstalled
				plan.Items[idx].Note = "already installed"
			} else {
				plan.Items[idx].Status = model.StateToInstall
			}
		}
	}

	plan.Recompute()
	return plan, ctx.Err()
}

// selectManager applies defaults.managers precedence, preferring an available
// manager that has an explicit ID.
func (s *Service) selectManager(app model.App) (Manager, string) {
	ordered := s.orderedManagers()
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

func (s *Service) orderedManagers() []Manager {
	remaining := make(map[model.ManagerName]Manager, len(s.byName))
	for name, m := range s.byName {
		remaining[name] = m
	}

	var ordered []Manager
	add := func(name model.ManagerName) {
		if m, ok := remaining[name]; ok {
			ordered = append(ordered, m)
			delete(remaining, name)
		}
	}
	for _, name := range s.Config.Defaults.Managers {
		add(model.NormalizeManagerName(name.String()))
	}
	for _, name := range model.HostOrder(s.HostOS) {
		add(name)
	}
	// Stable fallback for injected managers that are not in the host order.
	for _, m := range s.Managers {
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
// (missing logs, cancelled context) are returned; per-package failures are
// recorded in the plan items.
func (s *Service) Apply(ctx context.Context, plan *model.Plan, opts ApplyOptions) error {
	var pending []int
	for i := range plan.Items {
		if plan.Items[i].Status == model.StateToInstall {
			pending = append(pending, i)
		}
	}
	if len(pending) == 0 {
		return nil
	}
	if s.logs == nil {
		return errors.New("log store is not configured")
	}

	if err := s.checkElevation(plan, pending); err != nil {
		return err
	}

	var run LogRun
	var err error
	if opts.LogDir != "" {
		run = s.logs.OpenRun(opts.LogDir)
	} else {
		run, err = s.logs.NewRun()
		if err != nil {
			return err
		}
	}
	plan.LogDir = run.Dir()

	downloadFailed := map[model.ManagerName]bool{}
	if opts.Download {
		for _, mgr := range s.orderedManagers() {
			var indexes []int
			for _, idx := range pending {
				if plan.Items[idx].Manager == mgr.Name() && plan.Items[idx].Status == model.StateToInstall {
					indexes = append(indexes, idx)
				}
			}
			if len(indexes) == 0 {
				continue
			}

			file, err := run.Create("download-" + mgr.Name().String())
			if err != nil {
				for _, idx := range indexes {
					plan.Items[idx].Status = model.StateFailed
					plan.Items[idx].Error = fmt.Sprintf("create download log: %v", err)
				}
				downloadFailed[mgr.Name()] = true
				continue
			}
			writeLogHeader(file, "download", mgr.Name().String(), fmt.Sprintf("%d packages", len(indexes)))
			w := s.logWriter(file)

			s.progressf("download  %-12s %d package(s)\n", mgr.Name(), len(indexes))
			err = mgr.Download(ctx, uniquePackages(plan.Items, indexes), w)
			_ = file.Close()

			if errors.Is(err, model.ErrNoDownload) {
				s.progressf("download  %-12s not supported; packages download during install\n", mgr.Name())
				continue
			}
			if err != nil {
				downloadFailed[mgr.Name()] = true
				for _, idx := range indexes {
					plan.Items[idx].Status = model.StateFailed
					plan.Items[idx].Error = err.Error()
					plan.Items[idx].Log = file.Name()
				}
				s.progressf("download  %-12s failed: %v\n", mgr.Name(), err)
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
		if item.Status != model.StateToInstall {
			continue
		}
		if stopped || downloadFailed[item.Manager] {
			if stopped {
				item.Status = model.StateSkipped
				item.Error = "not attempted after a previous failure"
			}
			continue
		}

		mgr := s.byName[item.Manager]
		if mgr == nil {
			item.Status = model.StateFailed
			item.Error = fmt.Sprintf("manager %q is not available", item.Manager)
			continue
		}

		file, err := run.Create(item.Manager.String() + "-" + item.ID.String())
		if err != nil {
			item.Status = model.StateFailed
			item.Error = fmt.Sprintf("create log: %v", err)
			continue
		}
		writeLogHeader(file, "install", item.Manager.String(), item.ID.String())
		tail := &tailWriter{w: s.logWriter(file)}

		s.progressf("install   %-12s %s\n", item.Manager, item.ID)
		err = mgr.Install(ctx, item.Package(), tail)
		_ = file.Close()
		item.Log = file.Name()

		switch {
		case err == nil:
			item.Status = model.StateInstalled
			item.Note = "installed"
		case errors.Is(err, model.ErrAlreadyInstalled):
			item.Status = model.StateInstalled
			item.Note = "already installed"
		case errors.Is(err, model.ErrNoUpdate):
			item.Status = model.StateInstalled
			item.Note = "no update available"
		case errors.Is(err, model.ErrRebootRequired):
			item.Status = model.StateInstalled
			item.Note = "installed; reboot required"
		default:
			if ctx.Err() != nil {
				plan.Recompute()
				return ctx.Err()
			}
			item.Status = model.StateFailed
			item.Error = err.Error()
			if last := tail.lastLine(); last != "" && !strings.Contains(item.Error, last) {
				item.Error += " (" + last + ")"
			}
			s.progressf("failed    %-12s %s: %v\n", item.Manager, item.ID, err)
			if !opts.ContinueOnError {
				stopped = true
			}
		}
	}

	plan.Recompute()
	return nil
}

// ElevationRequired reports the first pending manager that needs elevated
// privileges when the current process is not elevated.
func (s *Service) ElevationRequired(plan *model.Plan) (model.ManagerName, bool) {
	if s.elevated {
		return "", false
	}
	for _, item := range plan.Items {
		if item.Status != model.StateToInstall {
			continue
		}
		if mgr, ok := s.byName[item.Manager]; ok && mgr.NeedsElevation() {
			return mgr.Name(), true
		}
	}
	return "", false
}

func (s *Service) checkElevation(plan *model.Plan, pending []int) error {
	seen := map[model.ManagerName]bool{}
	for _, idx := range pending {
		name := plan.Items[idx].Manager
		if seen[name] {
			continue
		}
		seen[name] = true
		mgr := s.byName[name]
		if mgr == nil || !mgr.NeedsElevation() || s.elevated {
			continue
		}
		if s.HostOS == "windows" {
			return fmt.Errorf("%s requires administrator privileges; re-run harpy from an elevated terminal", name)
		}
		return fmt.Errorf("%s requires root privileges; re-run with sudo", name)
	}
	return nil
}

func uniquePackages(items []model.Item, indexes []int) []model.Package {
	seen := map[model.PackageID]bool{}
	packages := make([]model.Package, 0, len(indexes))
	for _, idx := range indexes {
		item := items[idx]
		if item.ID == "" || seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		packages = append(packages, item.Package())
	}
	return packages
}

func (s *Service) logWriter(w io.Writer) io.Writer {
	if s.Verbose && s.Progress != nil {
		return io.MultiWriter(w, s.Progress)
	}
	return w
}

func (s *Service) progressf(format string, args ...any) {
	if s.Progress != nil {
		fmt.Fprintf(s.Progress, format, args...)
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
