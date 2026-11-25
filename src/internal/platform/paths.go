package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	base, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "harpy", "harpy.db"), nil
}

// LogsDir returns the directory that holds per-run log directories.
func LogsDir() (string, error) {
	base, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "harpy", "logs"), nil
}

// ToSlash normalizes a path for display and serialization. Physical paths used
// for filesystem access must stay host-native.
func ToSlash(p string) string { return filepath.ToSlash(p) }

func dataDir() (string, error) {
	if runtime.GOOS == "windows" {
		if base := os.Getenv("LOCALAPPDATA"); base != "" {
			return base, nil
		}
		if base, err := os.UserCacheDir(); err == nil {
			return base, nil
		}
		return underHome("AppData", "Local")
	}
	if base := os.Getenv("XDG_DATA_HOME"); base != "" {
		return base, nil
	}
	return underHome(".local", "share")
}

func stateDir() (string, error) {
	if runtime.GOOS == "windows" {
		if base := os.Getenv("LOCALAPPDATA"); base != "" {
			return base, nil
		}
		if base, err := os.UserCacheDir(); err == nil {
			return base, nil
		}
		return underHome("AppData", "Local")
	}
	if base := os.Getenv("XDG_STATE_HOME"); base != "" {
		return base, nil
	}
	return underHome(".local", "state")
}

func underHome(parts ...string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	all := append([]string{home}, parts...)
	return filepath.Join(all...), nil
}

func expandHome(p string) (string, error) {
	if p != "~" && !strings.HasPrefix(p, "~/") && !strings.HasPrefix(p, `~\`) {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	if p == "~" {
		return home, nil
	}
	return filepath.Join(home, p[2:]), nil
}
