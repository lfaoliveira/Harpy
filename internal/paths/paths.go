// Package paths resolves harpy's per-user config, state and log locations.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ConfigPath returns the config file path.
// Override: HARPY_CONFIG.
func ConfigPath() (string, error) {
	if v := os.Getenv("HARPY_CONFIG"); v != "" {
		return expandHome(v)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, "harpy", "config.yaml"), nil
}

// DBPath returns the SQLite database path (used from milestone M2 on).
// Override: HARPY_DB.
func DBPath() (string, error) {
	if v := os.Getenv("HARPY_DB"); v != "" {
		return expandHome(v)
	}
	base := os.Getenv("LOCALAPPDATA")
	if runtime.GOOS != "windows" {
		base = os.Getenv("XDG_DATA_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("resolve home dir: %w", err)
			}
			base = filepath.Join(home, ".local", "share")
		}
	} else if base == "" {
		var err error
		base, err = os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("resolve local app data dir: %w", err)
		}
	}
	return filepath.Join(base, "harpy", "harpy.db"), nil
}

// LogsDir returns the directory that holds per-run log directories.
func LogsDir() (string, error) {
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			var err error
			base, err = os.UserCacheDir()
			if err != nil {
				return "", fmt.Errorf("resolve local app data dir: %w", err)
			}
		}
		return filepath.Join(base, "harpy", "logs"), nil
	}

	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "harpy", "logs"), nil
}

func expandHome(p string) (string, error) {
	if p == "~" || len(p) > 1 && p[:2] == "~/" || len(p) > 1 && p[:2] == `~\` {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home dir: %w", err)
		}
		if len(p) == 1 {
			return home, nil
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}
