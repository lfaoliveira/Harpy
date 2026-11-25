package manager

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/lfaoliveira/harpy/src/internal/model"
	"github.com/lfaoliveira/harpy/src/internal/platform"
)

type fakeRunner struct {
	commands []platform.Command
	outputFn func(platform.Command) (platform.Result, error)
	runFn    func(platform.Command, io.Writer) error
	paths    map[string]bool
}

func (f *fakeRunner) Run(_ context.Context, cmd platform.Command, w io.Writer) error {
	f.commands = append(f.commands, cmd)
	if f.runFn != nil {
		return f.runFn(cmd, w)
	}
	return nil
}

func (f *fakeRunner) Output(_ context.Context, cmd platform.Command) (platform.Result, error) {
	f.commands = append(f.commands, cmd)
	if f.outputFn != nil {
		return f.outputFn(cmd)
	}
	return platform.Result{}, nil
}

func (f *fakeRunner) LookPath(file string) (string, error) {
	if f.paths[file] {
		return "/usr/bin/" + file, nil
	}
	return "", errors.New("not found")
}

func packages(ids ...string) []model.Package {
	out := make([]model.Package, 0, len(ids))
	for _, id := range ids {
		out = append(out, model.Package{ID: model.PackageID(id)})
	}
	return out
}

func TestChocolateyInstalledParsesLimitOutput(t *testing.T) {
	runner := &fakeRunner{
		outputFn: func(cmd platform.Command) (platform.Result, error) {
			if cmd.Name != "choco" {
				t.Fatalf("command = %q", cmd.Name)
			}
			if !contains(cmd.Args, "--limit-output") {
				t.Fatalf("args = %v", cmd.Args)
			}
			return platform.Result{Stdout: "git|2.47.0\ntree|2.1.0\n"}, nil
		},
	}
	mgr := NewChocolatey(runner)
	got, err := mgr.Installed(context.Background(), packages("Git", "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if !got["Git"] || got["missing"] {
		t.Fatalf("installed = %v", got)
	}
}

func TestAptInstalledParsesDpkgStatus(t *testing.T) {
	runner := &fakeRunner{
		outputFn: func(cmd platform.Command) (platform.Result, error) {
			if cmd.Name != "dpkg-query" {
				t.Fatalf("command = %q", cmd.Name)
			}
			return platform.Result{Stdout: "git:amd64\tinstall ok installed\ntree\tdeinstall ok config-files\n"}, nil
		},
	}
	mgr := NewApt(runner)
	got, err := mgr.Installed(context.Background(), packages("git", "tree"))
	if err != nil {
		t.Fatal(err)
	}
	if !got["git"] || got["tree"] {
		t.Fatalf("installed = %v", got)
	}
}

func TestAptAvailableRequiresBothBinaries(t *testing.T) {
	runner := &fakeRunner{paths: map[string]bool{"apt-get": true, "dpkg-query": true}}
	if !NewApt(runner).Available() {
		t.Fatal("apt should be available")
	}
	runner.paths["dpkg-query"] = false
	if NewApt(runner).Available() {
		t.Fatal("apt should be unavailable without dpkg-query")
	}
}

func TestWingetInstalledMapsNoApplicationsExit(t *testing.T) {
	runner := &fakeRunner{
		outputFn: func(cmd platform.Command) (platform.Result, error) {
			id := cmd.Args[2]
			if id == "git" {
				return platform.Result{}, nil
			}
			return platform.Result{}, &platform.CommandError{Name: "winget", Code: wingetNoApplications}
		},
	}
	mgr := NewWinget(runner)
	got, err := mgr.Installed(context.Background(), packages("git", "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if !got["git"] || got["missing"] {
		t.Fatalf("installed = %v", got)
	}
}

func TestPacmanInstalledToleratesMissingQueryExit(t *testing.T) {
	runner := &fakeRunner{
		outputFn: func(platform.Command) (platform.Result, error) {
			return platform.Result{Stdout: "git 2.47.0\n"}, &platform.CommandError{Name: "pacman", Code: 1}
		},
	}
	mgr := NewPacman("pkg", runner)
	got, err := mgr.Installed(context.Background(), packages("git", "tree"))
	if err != nil {
		t.Fatal(err)
	}
	if !got["git"] || got["tree"] {
		t.Fatalf("installed = %v", got)
	}
}

func TestInstallMapsRebootExit(t *testing.T) {
	runner := &fakeRunner{
		runFn: func(platform.Command, io.Writer) error {
			return &platform.CommandError{Name: "choco", Code: 3010}
		},
	}
	err := NewChocolatey(runner).Install(context.Background(), model.Package{ID: "git"}, io.Discard)
	if !errors.Is(err, model.ErrRebootRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestNoDownloadSentinel(t *testing.T) {
	if err := NewWinget(&fakeRunner{}).Download(context.Background(), packages("git"), io.Discard); !errors.Is(err, model.ErrNoDownload) {
		t.Fatalf("winget err = %v", err)
	}
	if err := NewChocolatey(&fakeRunner{}).Download(context.Background(), packages("git"), io.Discard); !errors.Is(err, model.ErrNoDownload) {
		t.Fatalf("choco err = %v", err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}
