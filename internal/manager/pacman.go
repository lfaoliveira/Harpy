package manager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Pacman manages Arch Linux packages through pacman.
type Pacman struct {
	base
	sync string // pkg | full
}

func NewPacman(sync string) *Pacman {
	if !strings.EqualFold(sync, "full") {
		sync = "pkg"
	}
	return &Pacman{sync: sync}
}

func (m *Pacman) Name() string         { return "pacman" }
func (m *Pacman) OS() string           { return "linux" }
func (m *Pacman) NeedsElevation() bool { return true }
func (m *Pacman) Available() bool      { _, err := exec.LookPath("pacman"); return err == nil }

func (m *Pacman) Installed(ctx context.Context, ids []string) (map[string]bool, error) {
	result := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	args := append([]string{"-Q"}, ids...)
	stdout, _, err := m.output(ctx, "pacman", args...)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	// pacman -Q exits 1 when at least one package is missing; installed
	// packages are still listed on stdout.
	if err != nil && stdout == "" && !isMissingQueryError(err) {
		return nil, err
	}

	found := map[string]bool{}
	for _, line := range strings.Split(stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			found[fields[0]] = true
		}
	}
	for _, id := range ids {
		result[id] = found[id]
	}
	return result, nil
}

func isMissingQueryError(err error) bool {
	code, ok := exitCode(err)
	return ok && code == 1
}

func (m *Pacman) Download(ctx context.Context, ids []string, w io.Writer) error {
	if len(ids) == 0 {
		return nil
	}
	if err := checkPacmanLock(); err != nil {
		return err
	}
	if m.sync == "full" {
		if err := m.run(ctx, w, "pacman", "-Syu", "--noconfirm"); err != nil {
			return err
		}
	} else {
		if w != nil {
			fmt.Fprintln(w, "warning: using pacman -Sy (partial upgrade risk); use --pacman-sync=full to run -Syu first")
		}
		if err := m.run(ctx, w, "pacman", "-Sy", "--noconfirm"); err != nil {
			return err
		}
	}
	args := append([]string{"-Sw", "--noconfirm"}, ids...)
	return m.run(ctx, w, "pacman", args...)
}

func (m *Pacman) Install(ctx context.Context, id string, w io.Writer) error {
	if err := checkPacmanLock(); err != nil {
		return err
	}
	return m.run(ctx, w, "pacman", "-S", "--noconfirm", id)
}

func checkPacmanLock() error {
	const lockPath = "/var/lib/pacman/db.lck"
	if _, err := os.Stat(lockPath); err == nil {
		return fmt.Errorf("pacman database is locked by another process; remove %s only if no pacman is running", lockPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check pacman lock: %w", err)
	}
	return nil
}
