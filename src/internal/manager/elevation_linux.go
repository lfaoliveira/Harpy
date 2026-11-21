//go:build linux

package manager

import "os"

// IsElevated reports whether the process has root privileges.
func IsElevated() bool {
	return os.Geteuid() == 0
}
