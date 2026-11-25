package model

// PackageID is the identifier a package manager uses for one package.
type PackageID string

func (id PackageID) String() string { return string(id) }

// Package is a package resolved to one concrete manager and ID.
type Package struct {
	Name    string
	Manager ManagerName
	ID      PackageID
}

// InstallState is the lifecycle state of one package in a plan.
type InstallState string

const (
	StateToInstall InstallState = "to-install"
	StateInstalled InstallState = "installed"
	StateBlocked   InstallState = "blocked"
	StateFailed    InstallState = "failed"
	StateSkipped   InstallState = "skipped"
)

// Item is one app resolved to a concrete package manager.
type Item struct {
	App     string       `json:"app"`
	Manager ManagerName  `json:"manager,omitempty"`
	ID      PackageID    `json:"id,omitempty"`
	OS      string       `json:"os,omitempty"`
	Status  InstallState `json:"status"`
	Note    string       `json:"note,omitempty"`
	Error   string       `json:"error,omitempty"`
	Log     string       `json:"log,omitempty"`
}

// Package resolves an item to the canonical manager contract.
func (i Item) Package() Package {
	return Package{Name: i.App, Manager: i.Manager, ID: i.ID}
}

// Summary aggregates item states.
type Summary struct {
	Total     int `json:"total"`
	ToInstall int `json:"to_install"`
	Installed int `json:"installed"`
	Blocked   int `json:"blocked"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

// Plan is the complete diff between desired and installed packages.
type Plan struct {
	Machine  string   `json:"machine,omitempty"`
	Profiles []string `json:"profiles,omitempty"`
	Items    []Item   `json:"items"`
	Summary  Summary  `json:"summary"`
	LogDir   string   `json:"log_dir,omitempty"`
}

// Recompute refreshes the summary after status changes.
func (p *Plan) Recompute() {
	s := Summary{Total: len(p.Items)}
	for _, item := range p.Items {
		switch item.Status {
		case StateToInstall:
			s.ToInstall++
		case StateInstalled:
			s.Installed++
		case StateBlocked:
			s.Blocked++
		case StateFailed:
			s.Failed++
		case StateSkipped:
			s.Skipped++
		}
	}
	p.Summary = s
}

// ManagerInfo describes one manager for the "managers" command.
type ManagerInfo struct {
	Name              ManagerName `json:"name"`
	OS                string      `json:"os"`
	Host              bool        `json:"host"`
	Available         bool        `json:"available"`
	ElevationRequired bool        `json:"elevation_required"`
}
