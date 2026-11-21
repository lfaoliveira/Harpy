package manager

import (
	"context"
	"io"
	"os/exec"
	"strings"
)

var chocoExitCodes = map[int]error{
	3010: ErrRebootRequired,
	1641: ErrRebootRequired,
}

// Chocolatey manages packages through choco.exe on Windows.
type Chocolatey struct {
	base
}

func NewChocolatey() *Chocolatey { return &Chocolatey{} }

func (m *Chocolatey) Name() string         { return "chocolatey" }
func (m *Chocolatey) OS() string           { return "windows" }
func (m *Chocolatey) NeedsElevation() bool { return true }
func (m *Chocolatey) Available() bool      { _, err := exec.LookPath("choco"); return err == nil }

func (m *Chocolatey) Installed(ctx context.Context, ids []string) (map[string]bool, error) {
	result := make(map[string]bool, len(ids))
	if len(ids) == 0 {
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
	for _, id := range ids {
		result[id] = installed[strings.ToLower(id)]
	}
	return result, nil
}

func (m *Chocolatey) Download(ctx context.Context, ids []string, w io.Writer) error {
	return ErrNoDownload
}

func (m *Chocolatey) Install(ctx context.Context, id string, w io.Writer) error {
	err := m.run(ctx, w, "choco", "install", id, "-y", "--no-progress", "--limit-output")
	return wrapExit(err, chocoExitCodes)
}
