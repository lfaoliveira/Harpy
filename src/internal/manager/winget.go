package manager

import (
	"context"
	"fmt"
	"io"

	"github.com/lfaoliveira/harpy/src/internal/model"
	"github.com/lfaoliveira/harpy/src/internal/platform"
)

const (
	wingetAlreadyInstalled   = 0x8A150061
	wingetNoUpdate           = 0x8A15002B
	wingetNoApplications     = 0x8A150014
	wingetAlreadyInstalled32 = -1978335135 // int32 representation of 0x8A150061
	wingetNoUpdate32         = -1978335189 // int32 representation of 0x8A15002B
	wingetNoApplications32   = -1978335212 // int32 representation of 0x8A150014
)

var wingetExitCodes = map[int]error{
	wingetAlreadyInstalled:   model.ErrAlreadyInstalled,
	wingetAlreadyInstalled32: model.ErrAlreadyInstalled,
	wingetNoUpdate:           model.ErrNoUpdate,
	wingetNoUpdate32:         model.ErrNoUpdate,
	3010:                     model.ErrRebootRequired,
	1641:                     model.ErrRebootRequired,
}

// Winget manages packages through the Windows Package Manager.
type Winget struct {
	base
}

func NewWinget(runner platform.Runner) *Winget {
	return &Winget{base: newBase(runner)}
}

func (m *Winget) Name() model.ManagerName { return model.ManagerWinget }
func (m *Winget) OS() string              { return "windows" }
func (m *Winget) NeedsElevation() bool    { return false }
func (m *Winget) Available() bool         { return m.lookPath("winget") }

func (m *Winget) Installed(ctx context.Context, packages []model.Package) (map[model.PackageID]bool, error) {
	result := make(map[model.PackageID]bool, len(packages))
	for _, pkg := range packages {
		_, _, err := m.output(ctx, "winget", "list",
			"--id", pkg.ID.String(),
			"--exact",
			"--accept-source-agreements",
			"--disable-interactivity",
		)
		if err == nil {
			result[pkg.ID] = true
			continue
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		code, _ := exitCode(err)
		switch code {
		case wingetNoApplications, wingetNoApplications32, wingetNoUpdate, wingetNoUpdate32,
			wingetAlreadyInstalled, wingetAlreadyInstalled32:
			result[pkg.ID] = false
		default:
			return nil, fmt.Errorf("winget list %s: %w", pkg.ID, err)
		}
	}
	return result, nil
}

func (m *Winget) Download(context.Context, []model.Package, io.Writer) error {
	return model.ErrNoDownload
}

func (m *Winget) Install(ctx context.Context, pkg model.Package, w io.Writer) error {
	err := m.run(ctx, w, "winget", "install",
		"--id", pkg.ID.String(),
		"--exact",
		"--accept-package-agreements",
		"--accept-source-agreements",
		"--disable-interactivity",
	)
	return wrapExit(err, wingetExitCodes)
}
