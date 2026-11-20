package manager

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Apt manages Debian/Ubuntu packages through apt-get and dpkg-query.
type Apt struct {
	base
}

func NewApt() *Apt {
	return &Apt{base: base{env: []string{
		"DEBIAN_FRONTEND=noninteractive",
		"APT_LISTCHANGES_FRONTEND=none",
	}}}
}

func (m *Apt) Name() string         { return "apt" }
func (m *Apt) OS() string           { return "linux" }
func (m *Apt) NeedsElevation() bool { return true }

func (m *Apt) Available() bool {
	_, aptErr := exec.LookPath("apt-get")
	_, dpkgErr := exec.LookPath("dpkg-query")
	return aptErr == nil && dpkgErr == nil
}

func (m *Apt) Installed(ctx context.Context, ids []string) (map[string]bool, error) {
	result := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	args := []string{"-W", "-f=${binary:Package}\\t${Status}\\n"}
	args = append(args, ids...)
	stdout, _, err := m.output(ctx, "dpkg-query", args...)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	// dpkg-query exits non-zero when any requested package is unknown. The
	// known packages are still printed, so the output is authoritative.
	if err != nil && stdout == "" {
		return result, nil
	}

	statusByPackage := map[string]string{}
	for _, line := range strings.Split(stdout, "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), "\t", 2)
		if len(fields) != 2 {
			continue
		}
		pkg := strings.TrimSpace(fields[0])
		statusByPackage[strings.ToLower(pkg)] = fields[1]
		statusByPackage[strings.ToLower(strings.SplitN(pkg, ":", 2)[0])] = fields[1]
	}

	for _, id := range ids {
		status := statusByPackage[strings.ToLower(id)]
		result[id] = strings.Contains(status, "install ok installed")
	}
	return result, nil
}

func (m *Apt) Download(ctx context.Context, ids []string, w io.Writer) error {
	if len(ids) == 0 {
		return nil
	}
	if err := m.run(ctx, w, "apt-get", "update"); err != nil {
		return aptLockError(err)
	}
	args := []string{
		"-d", "install", "-y", "--no-install-recommends",
		"-o", "DPkg::Lock::Timeout=120",
	}
	args = append(args, ids...)
	return aptLockError(m.run(ctx, w, "apt-get", args...))
}

func (m *Apt) Install(ctx context.Context, id string, w io.Writer) error {
	err := m.run(ctx, w, "apt-get", "install", "-y", "--no-install-recommends",
		"-o", "DPkg::Lock::Timeout=120", id)
	return aptLockError(err)
}

func aptLockError(err error) error {
	if err == nil {
		return nil
	}
	var cmdErr *CommandError
	if !asCommandError(err, &cmdErr) {
		return err
	}
	lower := strings.ToLower(cmdErr.Stderr)
	switch {
	case strings.Contains(lower, "could not get lock"),
		strings.Contains(lower, "unable to acquire the dpkg frontend lock"),
		strings.Contains(lower, "unable to lock directory"):
		return fmt.Errorf("package database is locked; wait for another apt/dpkg process to finish (details: %s)", cmdErr.LastLine())
	}
	return err
}
