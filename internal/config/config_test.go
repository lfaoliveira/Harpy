package config

import (
	"os"
	"strings"
	"testing"
)

const sampleConfig = `version: 1
search:
  mode: fuzzy
defaults:
  managers: [winget, chocolatey]
profiles:
  base:
    apps:
      - git
      - name: vscode
        winget: Microsoft.VisualStudioCode
        choco: vscode
  dev:
    apps: [neovim, jq]
  shared:
    apps:
      - name: ripgrep
        apt: ripgrep
      - name: ripgrep
        pacman: ripgrep
machines:
  - name: desktop
    match: { os: windows, hostname: 'DESKTOP-*' }
    profiles: [base, dev]
  - name: laptop
    match: { os: linux, hostname: 'thinkpad-*' }
    profiles: [base]
  - name: fallback
    match: { os: any }
    profiles: [dev]
`

func mustParse(t *testing.T, data string) (*Config, []string) {
	t.Helper()
	cfg, warnings, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return cfg, warnings
}

func TestParseAppForms(t *testing.T) {
	cfg, warnings := mustParse(t, sampleConfig)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	apps := cfg.Profiles["base"].Apps
	if len(apps) != 2 {
		t.Fatalf("want 2 apps, got %d", len(apps))
	}
	if apps[0].Name != "git" || apps[0].Apt != "" {
		t.Fatalf("scalar app parsed wrong: %+v", apps[0])
	}
	if apps[1].Name != "vscode" || apps[1].Winget != "Microsoft.VisualStudioCode" || apps[1].Choco != "vscode" {
		t.Fatalf("mapping app parsed wrong: %+v", apps[1])
	}
	if got := apps[1].IDFor("chocolatey"); got != "vscode" {
		t.Fatalf("IDFor(chocolatey) = %q", got)
	}
	if got := apps[1].IDFor("apt"); got != "" {
		t.Fatalf("IDFor(apt) = %q, want empty", got)
	}
}

func TestResolveMachine(t *testing.T) {
	cfg, _ := mustParse(t, sampleConfig)

	selection, err := cfg.Resolve("", nil, "windows", "desktop-abc")
	if err != nil {
		t.Fatal(err)
	}
	if selection.Machine != "desktop" || strings.Join(selection.Profiles, ",") != "base,dev" {
		t.Fatalf("unexpected selection: %+v", selection)
	}
	if len(selection.Apps) != 4 {
		t.Fatalf("want 4 apps, got %d", len(selection.Apps))
	}

	selection, err = cfg.Resolve("", nil, "linux", "desktop-abc")
	if err != nil {
		t.Fatal(err)
	}
	if selection.Machine != "fallback" {
		t.Fatalf("want fallback, got %q", selection.Machine)
	}

	selection, err = cfg.Resolve("laptop", nil, "linux", "whatever")
	if err != nil {
		t.Fatal(err)
	}
	if selection.Machine != "laptop" {
		t.Fatalf("want laptop, got %q", selection.Machine)
	}

	if _, err := cfg.Resolve("missing", nil, "linux", "x"); err == nil {
		t.Fatal("expected error for unknown machine")
	}
}

func TestResolveProfilesBypassesMachine(t *testing.T) {
	cfg, _ := mustParse(t, sampleConfig)
	selection, err := cfg.Resolve("", []string{"base,dev"}, "linux", "x")
	if err != nil {
		t.Fatal(err)
	}
	if selection.Machine != "" {
		t.Fatalf("machine should be empty, got %q", selection.Machine)
	}
	if strings.Join(selection.Profiles, ",") != "base,dev" {
		t.Fatalf("profiles = %v", selection.Profiles)
	}
	if len(selection.Apps) != 4 {
		t.Fatalf("want 4 apps, got %d", len(selection.Apps))
	}

	if _, err := cfg.Resolve("desktop", []string{"base"}, "windows", "x"); err == nil {
		t.Fatal("expected --machine/--profile conflict error")
	}
}

func TestProfileMergeDeduplicatesByName(t *testing.T) {
	cfg, _ := mustParse(t, sampleConfig)
	selection, err := cfg.Resolve("", []string{"base", "shared"}, "linux", "x")
	if err != nil {
		t.Fatal(err)
	}
	var found *App
	for i := range selection.Apps {
		if selection.Apps[i].Name == "ripgrep" {
			found = &selection.Apps[i]
			break
		}
	}
	if found == nil {
		t.Fatal("ripgrep missing")
	}
	if found.Apt != "ripgrep" || found.Pacman != "ripgrep" {
		t.Fatalf("ripgrep merge lost ids: %+v", *found)
	}
}

func TestUnknownKeysAreWarnings(t *testing.T) {
	data := `version: 1
future_section:
  enabled: true
profiles:
  p:
    apps:
      - name: x
        brew: x
machines:
  - name: any
    match: { os: any }
    profiles: [p]
`
	_, warnings := mustParse(t, data)
	if len(warnings) == 0 {
		t.Fatal("expected a warning for unknown keys")
	}
	if !strings.Contains(strings.Join(warnings, " "), "unknown") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestInvalidMachineOS(t *testing.T) {
	_, _, err := Parse([]byte(`version: 1
profiles: {p: {apps: [git]}}
machines:
  - name: bad
    match: { os: plan9 }
    profiles: [p]
`))
	if err == nil {
		t.Fatal("expected invalid os error")
	}
}

func TestNewerVersionIsWarning(t *testing.T) {
	_, warnings := mustParse(t, `version: 2
profiles: {p: {apps: [git]}}
machines:
  - name: any
    match: { os: any }
    profiles: [p]
`)
	if len(warnings) == 0 || !strings.Contains(warnings[0], "version 2") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestDefaultTemplateParses(t *testing.T) {
	cfg, warnings := mustParse(t, DefaultTemplate)
	if len(warnings) != 0 {
		t.Fatalf("template warnings: %v", warnings)
	}
	if cfg.Version != 1 {
		t.Fatalf("version = %d", cfg.Version)
	}
	if _, err := cfg.Resolve("fallback", nil, "linux", "x"); err != nil {
		t.Fatalf("fallback machine: %v", err)
	}
}

func TestDefaultTemplateMatchesExampleFile(t *testing.T) {
	examplePath := "../../config.example.yaml"
	data, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("read %s: %v", examplePath, err)
	}
	normalize := func(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }
	if normalize(string(data)) != normalize(DefaultTemplate) {
		t.Fatalf("%s drifted from config.DefaultTemplate; regenerate it with \"harpy init --stdout\"", examplePath)
	}
}
