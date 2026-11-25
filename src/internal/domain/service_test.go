package domain

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lfaoliveira/harpy/src/internal/logx"
	"github.com/lfaoliveira/harpy/src/internal/model"
)

type fakeManager struct {
	name      model.ManagerName
	os        string
	available bool
	elevation bool
	installed map[string]bool
	detectErr error
	download  []string
	downErr   error
	installs  []string
	installFn func(id string) error
}

func (f *fakeManager) Name() model.ManagerName { return f.name }
func (f *fakeManager) OS() string              { return f.os }
func (f *fakeManager) Available() bool         { return f.available }
func (f *fakeManager) NeedsElevation() bool    { return f.elevation }

func (f *fakeManager) Download(_ context.Context, packages []model.Package, _ io.Writer) error {
	for _, pkg := range packages {
		f.download = append(f.download, pkg.ID.String())
	}
	return f.downErr
}

func (f *fakeManager) Installed(_ context.Context, packages []model.Package) (map[model.PackageID]bool, error) {
	if f.detectErr != nil {
		return nil, f.detectErr
	}
	out := map[model.PackageID]bool{}
	for _, pkg := range packages {
		out[pkg.ID] = f.installed[pkg.ID.String()]
	}
	return out, nil
}

func (f *fakeManager) Install(_ context.Context, pkg model.Package, _ io.Writer) error {
	f.installs = append(f.installs, pkg.ID.String())
	if f.installFn != nil {
		return f.installFn(pkg.ID.String())
	}
	return nil
}

type testLogs struct{}

func (testLogs) NewRun() (LogRun, error) {
	dir, err := os.MkdirTemp("", "harpy-logs-")
	if err != nil {
		return nil, err
	}
	return testRun{dir: dir}, nil
}

func (testLogs) OpenRun(dir string) LogRun { return testRun{dir: dir} }

type testRun struct{ dir string }

func (r testRun) Dir() string { return r.dir }

func (r testRun) Create(name string) (Log, error) {
	file, err := os.Create(filepath.Join(r.dir, logx.SafeName(name)+".log"))
	if err != nil {
		return nil, err
	}
	return file, nil
}

func testConfig(apps ...model.App) *model.Config {
	return &model.Config{
		Version: 1,
		Search:  model.SearchConfig{Mode: "fuzzy"},
		Defaults: model.DefaultsConfig{
			Managers: []model.ManagerName{model.ManagerWinget, model.ManagerChocolatey},
		},
		Profiles: map[string]model.Profile{
			"base": {Apps: apps},
		},
	}
}

func newService(cfg *model.Config, managers ...Manager) *Service {
	return New(cfg, Options{
		HostOS:   "windows",
		Hostname: "test",
		Managers: managers,
		Logs:     testLogs{},
	})
}

func TestPlanChoosesExplicitManagerByPrecedence(t *testing.T) {
	winget := &fakeManager{name: model.ManagerWinget, os: "windows", available: true}
	choco := &fakeManager{name: model.ManagerChocolatey, os: "windows", available: true}
	cfg := testConfig(model.App{Name: "vscode", Winget: "Microsoft.VisualStudioCode", Choco: "vscode"})
	cfg.Defaults.Managers = []model.ManagerName{model.ManagerChocolatey, model.ManagerWinget}

	svc := newService(cfg, winget, choco)
	selection, err := svc.Resolve("", []string{"base"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := svc.Plan(context.Background(), selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("items = %d", len(plan.Items))
	}
	item := plan.Items[0]
	if item.Manager != model.ManagerChocolatey || item.ID != "vscode" || item.Status != model.StateToInstall {
		t.Fatalf("unexpected item: %+v", item)
	}
}

func TestPlanFallsBackToAvailableManager(t *testing.T) {
	winget := &fakeManager{name: model.ManagerWinget, os: "windows", available: false}
	choco := &fakeManager{name: model.ManagerChocolatey, os: "windows", available: true}
	cfg := testConfig(model.App{Name: "vscode", Winget: "Microsoft.VisualStudioCode", Choco: "vscode"})

	svc := newService(cfg, winget, choco)
	selection, _ := svc.Resolve("", []string{"base"})
	plan, err := svc.Plan(context.Background(), selection)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Items[0].Manager != model.ManagerChocolatey {
		t.Fatalf("manager = %q", plan.Items[0].Manager)
	}
}

func TestPlanBlockedWhenManagerUnavailable(t *testing.T) {
	winget := &fakeManager{name: model.ManagerWinget, os: "windows", available: false}
	cfg := testConfig(model.App{Name: "git", Winget: "Git.Git"})
	svc := newService(cfg, winget)
	selection, _ := svc.Resolve("", []string{"base"})
	plan, err := svc.Plan(context.Background(), selection)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Items[0].Status != model.StateBlocked {
		t.Fatalf("status = %q", plan.Items[0].Status)
	}
}

func TestPlanMarksInstalled(t *testing.T) {
	choco := &fakeManager{name: model.ManagerChocolatey, os: "windows", available: true, installed: map[string]bool{"git": true}}
	cfg := testConfig(model.App{Name: "git"}, model.App{Name: "jq"})
	svc := newService(cfg, choco)
	selection, _ := svc.Resolve("", []string{"base"})
	plan, err := svc.Plan(context.Background(), selection)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Items[0].Status != model.StateInstalled || plan.Items[1].Status != model.StateToInstall {
		t.Fatalf("items = %+v", plan.Items)
	}
	if plan.Summary.Installed != 1 || plan.Summary.ToInstall != 1 {
		t.Fatalf("summary = %+v", plan.Summary)
	}
}

func TestApplySequentialAndLogs(t *testing.T) {
	choco := &fakeManager{
		name: model.ManagerChocolatey, os: "windows", available: true,
		downErr: model.ErrNoDownload,
		installFn: func(id string) error {
			if id == "bad" {
				return errors.New("boom")
			}
			return nil
		},
	}
	cfg := testConfig(model.App{Name: "one"}, model.App{Name: "bad"}, model.App{Name: "two"})
	svc := newService(cfg, choco)
	svc.Progress = io.Discard
	selection, _ := svc.Resolve("", []string{"base"})
	plan, err := svc.Plan(context.Background(), selection)
	if err != nil {
		t.Fatal(err)
	}

	logDir := t.TempDir()
	err = svc.Apply(context.Background(), plan, ApplyOptions{
		Download:        true,
		ContinueOnError: true,
		LogDir:          logDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Join(choco.installs, ",") != "one,bad,two" {
		t.Fatalf("install order = %v", choco.installs)
	}
	if plan.Items[0].Status != model.StateInstalled || plan.Items[1].Status != model.StateFailed || plan.Items[2].Status != model.StateInstalled {
		t.Fatalf("statuses = %+v", plan.Items)
	}
	if plan.Summary.Installed != 2 || plan.Summary.Failed != 1 {
		t.Fatalf("summary = %+v", plan.Summary)
	}
	for _, item := range plan.Items {
		if item.Log == "" {
			t.Fatalf("missing log for %s", item.ID)
		}
		if _, err := os.Stat(item.Log); err != nil {
			t.Fatalf("log %s: %v", item.Log, err)
		}
	}
	entries, _ := os.ReadDir(logDir)
	if len(entries) == 0 {
		t.Fatal("no log files written")
	}
}

func TestApplyStopsOnError(t *testing.T) {
	choco := &fakeManager{
		name: model.ManagerChocolatey, os: "windows", available: true,
		installFn: func(id string) error {
			if id == "bad" {
				return errors.New("boom")
			}
			return nil
		},
	}
	cfg := testConfig(model.App{Name: "one"}, model.App{Name: "bad"}, model.App{Name: "two"})
	svc := newService(cfg, choco)
	svc.Progress = io.Discard
	selection, _ := svc.Resolve("", []string{"base"})
	plan, _ := svc.Plan(context.Background(), selection)

	err := svc.Apply(context.Background(), plan, ApplyOptions{
		ContinueOnError: false,
		LogDir:          t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(choco.installs, ",") != "one,bad" {
		t.Fatalf("install order = %v", choco.installs)
	}
	if plan.Items[2].Status != model.StateSkipped {
		t.Fatalf("third status = %q", plan.Items[2].Status)
	}
}

func TestApplyDownloadGroupsPackagesPerManager(t *testing.T) {
	choco := &fakeManager{name: model.ManagerChocolatey, os: "windows", available: true}
	cfg := testConfig(model.App{Name: "one"}, model.App{Name: "two"})
	svc := newService(cfg, choco)
	svc.Progress = io.Discard
	selection, _ := svc.Resolve("", []string{"base"})
	plan, _ := svc.Plan(context.Background(), selection)

	if err := svc.Apply(context.Background(), plan, ApplyOptions{
		Download:        true,
		ContinueOnError: true,
		LogDir:          t.TempDir(),
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(choco.download, ",") != "one,two" {
		t.Fatalf("download ids = %v", choco.download)
	}
}

func TestApplyDownloadFailureMarksItemsFailed(t *testing.T) {
	choco := &fakeManager{
		name: model.ManagerChocolatey, os: "windows", available: true,
		downErr: errors.New("network down"),
	}
	cfg := testConfig(model.App{Name: "one"})
	svc := newService(cfg, choco)
	svc.Progress = io.Discard
	selection, _ := svc.Resolve("", []string{"base"})
	plan, _ := svc.Plan(context.Background(), selection)

	if err := svc.Apply(context.Background(), plan, ApplyOptions{
		Download:        true,
		ContinueOnError: true,
		LogDir:          t.TempDir(),
	}); err != nil {
		t.Fatal(err)
	}
	if plan.Items[0].Status != model.StateFailed {
		t.Fatalf("status = %q", plan.Items[0].Status)
	}
	if len(choco.installs) != 0 {
		t.Fatalf("should not install after download failure: %v", choco.installs)
	}
}

func TestApplyHandlesAlreadyInstalledExit(t *testing.T) {
	choco := &fakeManager{
		name: model.ManagerChocolatey, os: "windows", available: true,
		installFn: func(string) error {
			return errors.Join(model.ErrAlreadyInstalled, errors.New("exit code 1"))
		},
	}
	cfg := testConfig(model.App{Name: "one"})
	svc := newService(cfg, choco)
	svc.Progress = io.Discard
	selection, _ := svc.Resolve("", []string{"base"})
	plan, _ := svc.Plan(context.Background(), selection)

	if err := svc.Apply(context.Background(), plan, ApplyOptions{LogDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if plan.Items[0].Status != model.StateInstalled || plan.Items[0].Note != "already installed" {
		t.Fatalf("item = %+v", plan.Items[0])
	}
}

func TestLogsDirectoryUsed(t *testing.T) {
	dir := t.TempDir()
	choco := &fakeManager{name: model.ManagerChocolatey, os: "windows", available: true}
	cfg := testConfig(model.App{Name: "weird/name:with spaces"})
	svc := newService(cfg, choco)
	svc.Progress = io.Discard
	selection, _ := svc.Resolve("", []string{"base"})
	plan, _ := svc.Plan(context.Background(), selection)

	if err := svc.Apply(context.Background(), plan, ApplyOptions{LogDir: dir}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("entries = %v", entries)
	}
	if strings.ContainsAny(entries[0].Name(), "/:") {
		t.Fatalf("unsafe log name %q", entries[0].Name())
	}
	if filepath.Ext(entries[0].Name()) != ".log" {
		t.Fatalf("log name %q", entries[0].Name())
	}
}

func TestElevationRequired(t *testing.T) {
	choco := &fakeManager{name: model.ManagerChocolatey, os: "windows", available: true, elevation: true}
	cfg := testConfig(model.App{Name: "git", Choco: "git"})
	cfg.Defaults.Managers = []model.ManagerName{model.ManagerChocolatey}

	svc := newService(cfg, choco)
	selection, _ := svc.Resolve("", []string{"base"})
	plan, _ := svc.Plan(context.Background(), selection)

	if name, ok := svc.ElevationRequired(plan); !ok || name != model.ManagerChocolatey {
		t.Fatalf("ElevationRequired = %q, %t", name, ok)
	}

	elevated := New(cfg, Options{HostOS: "windows", Hostname: "test", Managers: []Manager{choco}, Elevated: true, Logs: testLogs{}})
	selection, _ = elevated.Resolve("", []string{"base"})
	plan, _ = elevated.Plan(context.Background(), selection)
	if name, ok := elevated.ElevationRequired(plan); ok {
		t.Fatalf("ElevationRequired on elevated process = %q, %t", name, ok)
	}
}
