package model

import "strings"

// ManagerName is the canonical name of a package manager.
type ManagerName string

const (
	ManagerWinget     ManagerName = "winget"
	ManagerChocolatey ManagerName = "chocolatey"
	ManagerApt        ManagerName = "apt"
	ManagerPacman     ManagerName = "pacman"
)

// KnownManagers lists the package managers supported by this build.
var KnownManagers = []ManagerName{
	ManagerWinget,
	ManagerChocolatey,
	ManagerApt,
	ManagerPacman,
}

func (m ManagerName) String() string { return string(m) }

// NormalizeManagerName maps aliases to canonical manager names. It returns ""
// for unknown names.
func NormalizeManagerName(name string) ManagerName {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "winget":
		return ManagerWinget
	case "chocolatey", "choco":
		return ManagerChocolatey
	case "apt", "apt-get":
		return ManagerApt
	case "pacman":
		return ManagerPacman
	default:
		return ""
	}
}

// IsKnownManager reports whether name (or an alias) is supported.
func IsKnownManager(name string) bool {
	return NormalizeManagerName(name) != ""
}

// HostOrder returns the default manager precedence for an OS.
func HostOrder(hostOS string) []ManagerName {
	if strings.EqualFold(hostOS, "windows") {
		return []ManagerName{ManagerWinget, ManagerChocolatey}
	}
	return []ManagerName{ManagerApt, ManagerPacman}
}
