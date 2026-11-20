package manager

import (
	"context"
	"fmt"
	"io"
	"os/exec"
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
	wingetAlreadyInstalled:   ErrAlreadyInstalled,
	wingetAlreadyInstalled32: ErrAlreadyInstalled,
	wingetNoUpdate:           ErrNoUpdate,
	wingetNoUpdate32:         ErrNoUpdate,
	3010:                     ErrRebootRequired,
	1641:                     ErrRebootRequired,
}

// Winget manages packages through the Windows Package Manager.
type Winget struct {
	base
}

func NewWinget() *Winget { return &Winget{} }

func (m *Winget) Name() string         { return "winget" }
func (m *Winget) OS() string           { return "windows" }
func (m *Winget) NeedsElevation() bool { return false }
func (m *Winget) Available() bool      { _, err := exec.LookPath("winget"); return err == nil }

func (m *Winget) Installed(ctx context.Context, ids []string) (map[string]bool, error) {
	result := make(map[string]bool, len(ids))
	for _, id := range ids {
		_, _, err := m.output(ctx, "winget", "list",
			"--id", id,
			"--exact",
			"--accept-source-agreements",
			"--disable-interactivity",
		)
		if err == nil {
			result[id] = true
			continue
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		code, _ := exitCode(err)
		switch code {
		case wingetNoApplications, wingetNoApplications32, wingetNoUpdate, wingetNoUpdate32,
			wingetAlreadyInstalled, wingetAlreadyInstalled32:
			result[id] = false
		default:
			return nil, fmt.Errorf("winget list %s: %w", id, err)
		}
	}
	return result, nil
}

func (m *Winget) Download(ctx context.Context, ids []string, w io.Writer) error {
	return ErrNoDownload
}

func (m *Winget) Install(ctx context.Context, id string, w io.Writer) error {
	err := m.run(ctx, w, "winget", "install",
		"--id", id,
		"--exact",
		"--accept-package-agreements",
		"--accept-source-agreements",
		"--disable-interactivity",
	)
	return wrapExit(err, wingetExitCodes)
}
