// Package domain implements Harpy's use cases (resolve, plan, apply). It owns
// business rules and depends only on model contracts and small ports.
package domain

import (
	"context"
	"io"

	"github.com/lfaoliveira/harpy/src/internal/model"
)

// Manager is the port implemented by package-manager adapters.
type Manager interface {
	Name() model.ManagerName
	OS() string
	Available() bool
	NeedsElevation() bool

	Installed(ctx context.Context, packages []model.Package) (map[model.PackageID]bool, error)
	Download(ctx context.Context, packages []model.Package, w io.Writer) error
	Install(ctx context.Context, pkg model.Package, w io.Writer) error
}

// Log is one open log file.
type Log interface {
	io.WriteCloser
	Name() string
}

// LogRun is one per-invocation log directory.
type LogRun interface {
	Dir() string
	Create(name string) (Log, error)
}

// LogStore creates log runs. Platform supplies the filesystem implementation.
type LogStore interface {
	NewRun() (LogRun, error)
	OpenRun(dir string) LogRun
}

// ManagerInfos describes managers for the "managers" command.
func ManagerInfos(managers []Manager, hostOS string, elevated, all bool) []model.ManagerInfo {
	infos := make([]model.ManagerInfo, 0, len(managers))
	for _, m := range managers {
		host := m.OS() == hostOS
		if !all && !host {
			continue
		}
		infos = append(infos, model.ManagerInfo{
			Name:              m.Name(),
			OS:                m.OS(),
			Host:              host,
			Available:         m.Available(),
			ElevationRequired: m.NeedsElevation(),
		})
	}
	return infos
}
