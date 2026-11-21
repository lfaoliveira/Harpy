// Package config implements the harpy YAML configuration schema (version 1),
// machine matching and profile resolution.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// KnownManagers are the package managers supported by this build.
var KnownManagers = []string{"winget", "chocolatey", "apt", "pacman"}

// Config is the root of config.yaml.
type Config struct {
	Version  int                `yaml:"version"`
	Search   SearchConfig       `yaml:"search"`
	Defaults DefaultsConfig     `yaml:"defaults"`
	Profiles map[string]Profile `yaml:"profiles"`
	Machines []Machine          `yaml:"machines"`
}

// SearchConfig controls how package search ranks results. The search command
// itself arrives in a later milestone; the fields are parsed now so the file
// format is stable.
type SearchConfig struct {
	Mode               string `yaml:"mode"` // fuzzy | literal
	IncludeDescription bool   `yaml:"include_description"`
}

// DefaultsConfig holds installation defaults.
type DefaultsConfig struct {
	Managers        []string `yaml:"managers"`
	ContinueOnError *bool    `yaml:"continue_on_error"`
}

// Continue reports whether installation should keep going after a package
// fails. The default is true.
func (d DefaultsConfig) Continue() bool {
	if d.ContinueOnError == nil {
		return true
	}
	return *d.ContinueOnError
}

// Profile is a named list of applications.
type Profile struct {
	Apps []App `yaml:"apps"`
}

// App is one package declaration. In YAML it may be a plain string
// ("git") or a mapping with per-manager IDs.
type App struct {
	Name   string `yaml:"name"`
	Winget string `yaml:"winget"`
	Choco  string `yaml:"choco"`
	Apt    string `yaml:"apt"`
	Pacman string `yaml:"pacman"`
}

// UnmarshalYAML accepts both scalar and mapping forms.
func (a *App) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		a.Name = strings.TrimSpace(node.Value)
		if a.Name == "" {
			return errors.New("app name cannot be empty")
		}
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("app must be a string or a mapping, got %v", node.Kind)
	}

	type plain App
	var p plain
	if err := node.Decode(&p); err != nil {
		return err
	}
	*a = App(p)
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" {
		a.Name = a.firstID()
	}
	if a.Name == "" {
		return errors.New("app mapping needs a name or at least one manager id")
	}
	return nil
}

func (a App) firstID() string {
	for _, id := range []string{a.Winget, a.Choco, a.Apt, a.Pacman} {
		if id != "" {
			return id
		}
	}
	return ""
}

// IDFor returns the package ID for a manager. An empty result means the app
// did not declare an explicit ID for that manager.
func (a App) IDFor(manager string) string {
	switch NormalizeManagerName(manager) {
	case "winget":
		return a.Winget
	case "chocolatey":
		return a.Choco
	case "apt":
		return a.Apt
	case "pacman":
		return a.Pacman
	default:
		return ""
	}
}

// Merge fills empty fields of a with values from other. Later profile
// declarations may override IDs, which is what Merge does for non-empty
// values.
func (a *App) Merge(other App) {
	if a.Name == "" {
		a.Name = other.Name
	}
	if other.Winget != "" {
		a.Winget = other.Winget
	}
	if other.Choco != "" {
		a.Choco = other.Choco
	}
	if other.Apt != "" {
		a.Apt = other.Apt
	}
	if other.Pacman != "" {
		a.Pacman = other.Pacman
	}
}

// Machine binds profiles to a host.
type Machine struct {
	Name     string       `yaml:"name"`
	Match    MachineMatch `yaml:"match"`
	Profiles []string     `yaml:"profiles"`
}

// MachineMatch selects a machine by OS and hostname glob.
type MachineMatch struct {
	OS       string `yaml:"os"`       // windows | linux | any
	Hostname string `yaml:"hostname"` // glob, optional
}

// NormalizeManagerName maps aliases to canonical manager names. It returns ""
// for unknown names.
func NormalizeManagerName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "winget":
		return "winget"
	case "chocolatey", "choco":
		return "chocolatey"
	case "apt", "apt-get":
		return "apt"
	case "pacman":
		return "pacman"
	default:
		return ""
	}
}

// Load reads and validates a config file. Warnings (unknown keys, unknown
// managers, newer config versions) are returned separately so the CLI can show
// them without rejecting a forward-compatible file.
func Load(filename string) (*Config, []string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, nil, err
	}
	return Parse(data)
}

// Parse decodes and validates config bytes.
func Parse(data []byte) (*Config, []string, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil, errors.New("config file is empty")
	}

	var warnings []string
	var cfg Config

	strict := yaml.NewDecoder(bytes.NewReader(data))
	strict.KnownFields(true)
	if err := strict.Decode(&cfg); err != nil {
		// Unknown keys are warnings in harpy: the file must stay usable when
		// newer fields (for example the future dev section) are present.
		var lenient Config
		if err2 := yaml.NewDecoder(bytes.NewReader(data)).Decode(&lenient); err2 != nil {
			return nil, nil, fmt.Errorf("parse config: %w", err2)
		}
		cfg = lenient
		warnings = append(warnings, fmt.Sprintf("config contains unknown or unsupported fields (ignored): %v", err))
	}

	cfg.applyDefaults()
	warnings = append(warnings, cfg.validate()...)
	if err := cfg.validateErrors(); err != nil {
		return nil, warnings, err
	}
	return &cfg, warnings, nil
}

func (c *Config) applyDefaults() {
	if c.Version == 0 {
		c.Version = 1
	}
	c.Search.Mode = strings.ToLower(strings.TrimSpace(c.Search.Mode))
	if c.Search.Mode == "" {
		c.Search.Mode = "fuzzy"
	}
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
}

func (c *Config) validate() []string {
	var warnings []string
	if c.Version != 1 {
		warnings = append(warnings, fmt.Sprintf("config version %d is not fully supported by this build (expected version 1)", c.Version))
	}
	if c.Search.Mode != "fuzzy" && c.Search.Mode != "literal" {
		warnings = append(warnings, fmt.Sprintf("search.mode %q is invalid; using fuzzy", c.Search.Mode))
		c.Search.Mode = "fuzzy"
	}

	seenManagers := map[string]bool{}
	for i, name := range c.Defaults.Managers {
		canonical := NormalizeManagerName(name)
		if canonical == "" {
			warnings = append(warnings, fmt.Sprintf("defaults.managers[%d]: unknown manager %q (ignored)", i, name))
			continue
		}
		c.Defaults.Managers[i] = canonical
		if seenManagers[canonical] {
			warnings = append(warnings, fmt.Sprintf("defaults.managers contains %q more than once (deduplicated)", canonical))
		}
		seenManagers[canonical] = true
	}
	c.Defaults.Managers = dedupeStrings(c.Defaults.Managers)

	seenMachines := map[string]bool{}
	for i := range c.Machines {
		m := &c.Machines[i]
		m.Name = strings.TrimSpace(m.Name)
		if m.Name == "" {
			warnings = append(warnings, fmt.Sprintf("machines[%d] has no name (using machine-%d)", i, i+1))
			m.Name = fmt.Sprintf("machine-%d", i+1)
		}
		if seenMachines[strings.ToLower(m.Name)] {
			warnings = append(warnings, fmt.Sprintf("machine %q is declared more than once; the first match wins", m.Name))
		}
		seenMachines[strings.ToLower(m.Name)] = true
		m.Match.OS = strings.ToLower(strings.TrimSpace(m.Match.OS))
		if m.Match.Hostname != "" {
			if _, err := path.Match(strings.ToLower(m.Match.Hostname), "probe"); err != nil {
				warnings = append(warnings, fmt.Sprintf("machine %q has invalid hostname glob %q: %v", m.Name, m.Match.Hostname, err))
			}
		}
	}

	for name, profile := range c.Profiles {
		for i := range profile.Apps {
			if strings.TrimSpace(profile.Apps[i].Name) == "" {
				warnings = append(warnings, fmt.Sprintf("profile %q apps[%d] has an empty name", name, i))
			}
		}
	}
	return warnings
}

func (c *Config) validateErrors() error {
	if c.Version < 1 {
		return fmt.Errorf("invalid config version %d", c.Version)
	}
	for _, m := range c.Machines {
		switch m.Match.OS {
		case "", "any", "windows", "linux":
		default:
			return fmt.Errorf("machine %q: match.os must be windows, linux or any (got %q)", m.Name, m.Match.OS)
		}
		if len(m.Profiles) == 0 {
			return fmt.Errorf("machine %q: no profiles selected", m.Name)
		}
		for _, p := range m.Profiles {
			if _, ok := c.Profiles[p]; !ok {
				return fmt.Errorf("machine %q references unknown profile %q", m.Name, p)
			}
		}
	}
	return nil
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// Selection is the result of resolving a machine or explicit profiles.
type Selection struct {
	Machine  string
	Profiles []string
	Apps     []App
}

// Resolve selects apps for a machine. An explicit profile list bypasses
// machine matching (useful for scripts and tests). Exactly one of machineName
// and profiles should be non-empty; when both are empty machine matching is
// used.
func (c *Config) Resolve(machineName string, profiles []string, hostOS, hostname string) (*Selection, error) {
	machineName = strings.TrimSpace(machineName)
	if machineName != "" && len(profiles) > 0 {
		return nil, errors.New("use either --machine or --profile, not both")
	}

	var selectedProfiles []string
	var machineLabel string
	if len(profiles) > 0 {
		selectedProfiles = dedupeStrings(trimAll(profiles))
	} else {
		machine, err := c.matchMachine(machineName, hostOS, hostname)
		if err != nil {
			return nil, err
		}
		machineLabel = machine.Name
		selectedProfiles = dedupeStrings(machine.Profiles)
	}
	if len(selectedProfiles) == 0 {
		return nil, errors.New("no profiles selected")
	}

	apps := make([]App, 0)
	index := map[string]int{}
	for _, profileName := range selectedProfiles {
		profile, ok := c.Profiles[profileName]
		if !ok {
			return nil, fmt.Errorf("unknown profile %q", profileName)
		}
		for _, app := range profile.Apps {
			key := strings.ToLower(strings.TrimSpace(app.Name))
			if at, exists := index[key]; exists {
				apps[at].Merge(app)
				continue
			}
			index[key] = len(apps)
			apps = append(apps, app)
		}
	}

	return &Selection{Machine: machineLabel, Profiles: selectedProfiles, Apps: apps}, nil
}

func (c *Config) matchMachine(name, hostOS, hostname string) (*Machine, error) {
	if name != "" {
		for i := range c.Machines {
			if strings.EqualFold(c.Machines[i].Name, name) {
				return &c.Machines[i], nil
			}
		}
		return nil, fmt.Errorf("machine %q not found (available: %s)", name, strings.Join(c.machineNames(), ", "))
	}

	if len(c.Machines) == 0 {
		return nil, errors.New("config has no machines; add a machines entry or use --profile NAME")
	}

	for i := range c.Machines {
		m := &c.Machines[i]
		if !osMatches(m.Match.OS, hostOS) {
			continue
		}
		if m.Match.Hostname != "" {
			ok, err := globMatch(m.Match.Hostname, hostname)
			if err != nil {
				return nil, fmt.Errorf("machine %q: invalid hostname glob: %w", m.Name, err)
			}
			if !ok {
				continue
			}
		}
		return m, nil
	}
	return nil, fmt.Errorf("no machine matched os=%s hostname=%s (available: %s); use --machine NAME or --profile NAME",
		hostOS, hostname, strings.Join(c.machineNames(), ", "))
}

func (c *Config) machineNames() []string {
	names := make([]string, 0, len(c.Machines))
	for _, m := range c.Machines {
		names = append(names, m.Name)
	}
	sort.Strings(names)
	return names
}

func osMatches(configured, hostOS string) bool {
	switch strings.ToLower(strings.TrimSpace(configured)) {
	case "", "any":
		return true
	default:
		return strings.EqualFold(configured, hostOS)
	}
}

func globMatch(pattern, value string) (bool, error) {
	ok, err := path.Match(strings.ToLower(pattern), strings.ToLower(value))
	if err != nil {
		return false, err
	}
	return ok, nil
}

func trimAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		for _, part := range strings.Split(s, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}
