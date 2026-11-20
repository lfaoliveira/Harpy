package engine

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lfaoliveira/harpy/internal/config"
	"github.com/lfaoliveira/harpy/internal/manager"
)

type fakeManager struct {
	name      string
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

func (f *fakeManager) Name() string         { return f.name }
func (f *fakeManager) OS() string           { return f.os }
func (f *fakeManager) Available() bool      { return f.available }
func (f *fakeManager) NeedsElevation() bool { return f.elevation }
func (f *fakeManager) Download(_ context.Context, ids []string, _ io.Writer) error {
	f.download = append(f.download, ids...)
	return f.downErr
}
func (f *fakeManager) Installed(_ context.Context, ids []string) (map[string]bool, error) {
	if f.detectErr != nil {
		return nil, f.detectErr
	}
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = f.installed[id]
	}
	return out, nil
}
func (f *fakeManager) Install(_ context.Context, id string, _ io.Writer) error {
	f.installs = append(f.installs, id)
	if f.installFn != nil {
		return f.installFn(id)
	}
	return nil
}

func testConfig(apps ...config.App) *config.Config {
	return &config.Config{
		Version: 1,
		Search:  config.SearchConfig{Mode: "fuzzy"},
		Defaults: config.DefaultsConfig{
			Managers: []string{"winget", "chocolatey"},
		},
		Profiles: map[string]config.Profile{
			"base": {Apps: apps},
		},
	}
}

func newEngine(cfg *config.Config, managers ...manager.Manager) *Engine {
	return New(cfg, Options{
		HostOS:   "windows",
		Hostname: "test",
		Managers: managers,
	})
}

func TestPlanChoosesExplicitManagerByPrecedence(t *testing.T) {
	winget := &fakeManager{name: "winget", os: "windows", available: true}
	choco := &fakeManager{name: "chocolatey", os: "windows", available: true}
	cfg := testConfig(config.App{Name: "vscode", Winget: "Microsoft.VisualStudioCode", Choco: "vscode"})
	cfg.Defaults.Managers = []string{"chocolatey", "winget"}

	eng := newEngine(cfg, winget, choco)
	selection, err := eng.Resolve("", []string{"base"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := eng.Plan(context.Background(), selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("items = %d", len(plan.Items))
	}
	item := plan.Items[0]
	if item.Manager != "chocolatey" || item.ID != "vscode" || item.Status != StatusToInstall {
		t.Fatalf("unexpected item: %+v", item)
	}
}

func TestPlanFallsBackToAvailableManager(t *testing.T) {
	winget := &fakeManager{name: "winget", os: "windows", available: false}
	choco := &fakeManager{name: "chocolatey", os: "windows", available: true}
	cfg := testConfig(config.App{Name: "vscode", Winget: "Microsoft.VisualStudioCode", Choco: "vscode"})

	eng := newEngine(cfg, winget, choco)
	selection, _ := eng.Resolve("", []string{"base"})
	plan, err := eng.Plan(context.Background(), selection)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Items[0].Manager != "chocolatey" {
		t.Fatalf("manager = %q", plan.Items[0].Manager)
	}
}

func TestPlanBlockedWhenManagerUnavailable(t *testing.T) {
	winget := &fakeManager{name: "winget", os: "windows", available: false}
	cfg := testConfig(config.App{Name: "git", Winget: "Git.Git"})
	eng := newEngine(cfg, winget)
	selection, _ := eng.Resolve("", []string{"base"})
	plan, err := eng.Plan(context.Background(), selection)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Items[0].Status != StatusBlocked {
		t.Fatalf("status = %q", plan.Items[0].Status)
	}
}

func TestPlanMarksInstalled(t *testing.T) {
	choco := &fakeManager{name: "chocolatey", os: "windows", available: true, installed: map[string]bool{"git": true}}
	cfg := testConfig(config.App{Name: "git"}, config.App{Name: "jq"})
	eng := newEngine(cfg, choco)
	selection, _ := eng.Resolve("", []string{"base"})
	plan, err := eng.Plan(context.Background(), selection)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Items[0].Status != StatusInstalled || plan.Items[1].Status != StatusToInstall {
		t.Fatalf("items = %+v", plan.Items)
	}
	if plan.Summary.Installed != 1 || plan.Summary.ToInstall != 1 {
		t.Fatalf("summary = %+v", plan.Summary)
	}
}

func TestApplySequentialAndLogs(t *testing.T) {
	choco := &fakeManager{
		name: "chocolatey", os: "windows", available: true,
		downErr: manager.ErrNoDownload,
		installFn: func(id string) error {
			if id == "bad" {
				return errors.New("boom")
			}
			return nil
		},
	}
	cfg := testConfig(config.App{Name: "one"}, config.App{Name: "bad"}, config.App{Name: "two"})
	eng := newEngine(cfg, choco)
	eng.Progress = io.Discard
	selection, _ := eng.Resolve("", []string{"base"})
	plan, err := eng.Plan(context.Background(), selection)
	if err != nil {
		t.Fatal(err)
	}

	logDir := t.TempDir()
	err = eng.Apply(context.Background(), plan, ApplyOptions{
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
	if plan.Items[0].Status != StatusInstalled || plan.Items[1].Status != StatusFailed || plan.Items[2].Status != StatusInstalled {
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
		name: "chocolatey", os: "windows", available: true,
		installFn: func(id string) error {
			if id == "bad" {
				return errors.New("boom")
			}
			return nil
		},
	}
	cfg := testConfig(config.App{Name: "one"}, config.App{Name: "bad"}, config.App{Name: "two"})
	eng := newEngine(cfg, choco)
	eng.Progress = io.Discard
	selection, _ := eng.Resolve("", []string{"base"})
	plan, _ := eng.Plan(context.Background(), selection)

	err := eng.Apply(context.Background(), plan, ApplyOptions{
		ContinueOnError: false,
		LogDir:          t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(choco.installs, ",") != "one,bad" {
		t.Fatalf("install order = %v", choco.installs)
	}
	if plan.Items[2].Status != StatusSkipped {
		t.Fatalf("third status = %q", plan.Items[2].Status)
	}
}

func TestApplyDownloadGroupsPackagesPerManager(t *testing.T) {
	choco := &fakeManager{name: "chocolatey", os: "windows", available: true}
	cfg := testConfig(config.App{Name: "one"}, config.App{Name: "two"})
	eng := newEngine(cfg, choco)
	eng.Progress = io.Discard
	selection, _ := eng.Resolve("", []string{"base"})
	plan, _ := eng.Plan(context.Background(), selection)

	if err := eng.Apply(context.Background(), plan, ApplyOptions{
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
		name: "chocolatey", os: "windows", available: true,
		downErr: errors.New("network down"),
	}
	cfg := testConfig(config.App{Name: "one"})
	eng := newEngine(cfg, choco)
	eng.Progress = io.Discard
	selection, _ := eng.Resolve("", []string{"base"})
	plan, _ := eng.Plan(context.Background(), selection)

	if err := eng.Apply(context.Background(), plan, ApplyOptions{
		Download:        true,
		ContinueOnError: true,
		LogDir:          t.TempDir(),
	}); err != nil {
		t.Fatal(err)
	}
	if plan.Items[0].Status != StatusFailed {
		t.Fatalf("status = %q", plan.Items[0].Status)
	}
	if len(choco.installs) != 0 {
		t.Fatalf("should not install after download failure: %v", choco.installs)
	}
}

func TestApplyHandlesAlreadyInstalledExit(t *testing.T) {
	choco := &fakeManager{
		name: "chocolatey", os: "windows", available: true,
		installFn: func(string) error {
			return errors.Join(manager.ErrAlreadyInstalled, errors.New("exit code 1"))
		},
	}
	cfg := testConfig(config.App{Name: "one"})
	eng := newEngine(cfg, choco)
	eng.Progress = io.Discard
	selection, _ := eng.Resolve("", []string{"base"})
	plan, _ := eng.Plan(context.Background(), selection)

	if err := eng.Apply(context.Background(), plan, ApplyOptions{LogDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if plan.Items[0].Status != StatusInstalled || plan.Items[0].Note != "already installed" {
		t.Fatalf("item = %+v", plan.Items[0])
	}
}

func TestLogsDirectoryUsed(t *testing.T) {
	dir := t.TempDir()
	choco := &fakeManager{name: "chocolatey", os: "windows", available: true}
	cfg := testConfig(config.App{Name: "weird/name:with spaces"})
	eng := newEngine(cfg, choco)
	eng.Progress = io.Discard
	selection, _ := eng.Resolve("", []string{"base"})
	plan, _ := eng.Plan(context.Background(), selection)

	if err := eng.Apply(context.Background(), plan, ApplyOptions{LogDir: dir}); err != nil {
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
