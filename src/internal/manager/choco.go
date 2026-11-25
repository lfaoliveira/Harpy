package manager

import (
	"context"
	"io"
	"strings"

	"github.com/lfaoliveira/harpy/src/internal/model"
	"github.com/lfaoliveira/harpy/src/internal/platform"
)

var chocoExitCodes = map[int]error{
	3010: model.ErrRebootRequired,
	1641: model.ErrRebootRequired,
}

// Chocolatey manages packages through choco.exe on Windows.
type Chocolatey struct {
	base
}

func NewChocolatey(runner platform.Runner) *Chocolatey {
	return &Chocolatey{base: newBase(runner)}
}

func (m *Chocolatey) Name() model.ManagerName { return model.ManagerChocolatey }
func (m *Chocolatey) OS() string              { return "windows" }
func (m *Chocolatey) NeedsElevation() bool    { return true }
func (m *Chocolatey) Available() bool         { return m.lookPath("choco") }

func (m *Chocolatey) Installed(ctx context.Context, packages []model.Package) (map[model.PackageID]bool, error) {
	result := make(map[model.PackageID]bool, len(packages))
	if len(packages) == 0 {
		return result, nil
	}

	stdout, _, err := m.output(ctx, "choco", "list", "--limit-output", "--no-progress")
	if err != nil {
		// Older choco versions need --local-only, newer ones print a warning
		// but still exit 0. A non-zero exit is a real failure.
		return nil, err
	}

	installed := map[string]bool{}
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		id := strings.SplitN(line, "|", 2)[0]
		installed[strings.ToLower(strings.TrimSpace(id))] = true
	}
	for _, pkg := range packages {
		result[pkg.ID] = installed[strings.ToLower(pkg.ID.String())]
	}
	return result, nil
}

func (m *Chocolatey) Download(context.Context, []model.Package, io.Writer) error {
	return model.ErrNoDownload
}

func (m *Chocolatey) Install(ctx context.Context, pkg model.Package, w io.Writer) error {
	err := m.run(ctx, w, "choco", "install", pkg.ID.String(), "-y", "--no-progress", "--limit-output")
	return wrapExit(err, chocoExitCodes)
}
