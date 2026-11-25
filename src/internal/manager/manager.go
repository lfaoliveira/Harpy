// Package manager implements Harpy's package-manager adapters. Each adapter
// translates raw command output into model contracts and delegates process
// execution to a platform.Runner.
package manager

import (
	"strings"

	"github.com/lfaoliveira/harpy/src/internal/domain"
	"github.com/lfaoliveira/harpy/src/internal/model"
	"github.com/lfaoliveira/harpy/src/internal/platform"
)

// Options configures adapter behavior.
type Options struct {
	// PacmanSync is "pkg" (default: pacman -Sy with a partial-upgrade warning)
	// or "full" (pacman -Syu before installing).
	PacmanSync string
	// Runner overrides process execution (useful in tests).
	Runner platform.Runner
}

func (o Options) runner() platform.Runner {
	if o.Runner != nil {
		return o.Runner
	}
	return platform.NewRunner()
}

func (o Options) pacmanSync() string {
	if strings.EqualFold(o.PacmanSync, "full") {
		return "full"
	}
	return "pkg"
}

// All returns every supported manager, regardless of host OS.
func All(opts Options) []domain.Manager {
	runner := opts.runner()
	return []domain.Manager{
		NewWinget(runner),
		NewChocolatey(runner),
		NewApt(runner),
		NewPacman(opts.pacmanSync(), runner),
	}
}

// Host returns managers that belong to the current OS.
func Host(opts Options) []domain.Manager {
	hostOS := platform.HostOS()
	var out []domain.Manager
	for _, m := range All(opts) {
		if m.OS() == hostOS {
			out = append(out, m)
		}
	}
	return out
}

// ByName finds a manager by canonical name or alias.
func ByName(name string, opts Options) (domain.Manager, bool) {
	canonical := model.NormalizeManagerName(name)
	if canonical == "" {
		return nil, false
	}
	for _, m := range All(opts) {
		if m.Name() == canonical {
			return m, true
		}
	}
	return nil, false
}

// IsKnown reports whether name is a supported manager.
func IsKnown(name string) bool { return model.IsKnownManager(name) }
