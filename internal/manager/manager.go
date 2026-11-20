// Package manager defines the package-manager contract used by harpy and
// implements the winget, chocolatey, apt and pacman adapters.
package manager

import (
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
)

// Sentinel errors returned by adapters. The engine maps them to friendly
// summary statuses.
var (
	ErrNoDownload       = errors.New("package manager does not support pre-download")
	ErrAlreadyInstalled = errors.New("package is already installed")
	ErrNoUpdate         = errors.New("no update available")
	ErrRebootRequired   = errors.New("reboot required")
)

// Manager is the common contract for all package managers.
type Manager interface {
	Name() string
	OS() string
	Available() bool
	NeedsElevation() bool
	Installed(ctx context.Context, ids []string) (map[string]bool, error)
	Download(ctx context.Context, ids []string, w io.Writer) error
	Install(ctx context.Context, id string, w io.Writer) error
}

// Options configures adapter behavior.
type Options struct {
	// PacmanSync is "pkg" (default: pacman -Sy with a partial-upgrade warning)
	// or "full" (pacman -Syu before installing).
	PacmanSync string
}

func (o Options) pacmanSync() string {
	if strings.EqualFold(o.PacmanSync, "full") {
		return "full"
	}
	return "pkg"
}

// All returns every supported manager, regardless of host OS.
func All(opts Options) []Manager {
	return []Manager{
		NewWinget(),
		NewChocolatey(),
		NewApt(),
		NewPacman(opts.pacmanSync()),
	}
}

// Host returns managers that belong to the current OS.
func Host(opts Options) []Manager {
	hostOS := runtime.GOOS
	var out []Manager
	for _, m := range All(opts) {
		if m.OS() == hostOS {
			out = append(out, m)
		}
	}
	return out
}

// ByName finds a manager by canonical name or alias.
func ByName(name string, opts Options) (Manager, bool) {
	canonical := canonicalName(name)
	for _, m := range All(opts) {
		if m.Name() == canonical {
			return m, true
		}
	}
	return nil, false
}

// IsKnown reports whether name is a supported manager.
func IsKnown(name string) bool {
	return canonicalName(name) != ""
}

// HostOrder returns the default precedence for the current OS.
func HostOrder() []string {
	if runtime.GOOS == "windows" {
		return []string{"winget", "chocolatey"}
	}
	return []string{"apt", "pacman"}
}

func canonicalName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "winget":
		return "winget"
	case "chocolatey", "choco":
		return "chocolatey"
	case "apt", "apt-get":
		return "apt"
	case "pacman":
		return "pacman"
	default:
		return ""
	}
}
