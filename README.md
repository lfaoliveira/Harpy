# Harpy: All-in-one App Manager

<img src="./assets/harpy_logo.png" width=300px>

## Multi-Machine, Multi-OS

Ever got tired of switching to a new machine and having to remember all your
previous apps? Harpy solves it. Describe your apps once in a YAML profile,
bind profiles to machines, and let Harpy install the missing packages.

- One config for Windows and Linux
- winget, chocolatey, apt and pacman
- Machine matching by OS and hostname
- Fuzzy searching and a TUI are on the roadmap
- Sequential, logged installs (no installer races)
- Headless CLI with a stable `--json` contract

> **Status:** early CLI core (`init`, `plan`, `apply`, `managers`). Search,
> SQLite index and the TUI are the next milestones. See
> [`PLAN.md`](PLAN.md) and [`docs/TODO.md`](docs/TODO.md).

## Install

Build from source (Go 1.23+):

```bash
git clone https://github.com/lfaoliveira/harpy.git
cd harpy
go build -o harpy ./cmd/harpy
```

Or, with [mise](https://mise.jdx.dev/) installed, from the repository root:

```bash
mise install
go install ./cmd/harpy
```

Put the resulting `harpy` binary on your `PATH`.

## Quick start

```bash
harpy init                       # writes the per-user config
harpy managers                   # detects available package managers
harpy plan                       # diff: desired vs installed
harpy apply --yes                # downloads and installs sequentially
```

Useful variations:

```bash
harpy plan --machine thinkpad
harpy plan --profile base,dev --json
harpy apply --yes --machine desktop-casa
harpy apply --yes --continue-on-error=false
harpy apply --yes --pacman-sync=full
harpy managers --all
```

`plan` never changes the system. `apply` asks for confirmation unless `--yes`
is passed.

## Commands

| Command          | Description                                                     |
| ---------------- | --------------------------------------------------------------- |
| `harpy init`     | Write a starter config (or print it with `--stdout`)            |
| `harpy plan`     | Show `to-install`, `installed`, `blocked` and `failed` items    |
| `harpy apply`    | Pre-download when supported, then install one package at a time |
| `harpy managers` | Show each manager, availability, host OS and elevation          |
| `harpy version`  | Version, OS, architecture and Go runtime                        |
| `harpy help`     | Command overview                                                |

All commands accept `--json`. The envelope is stable:

```json
{
    "schema": 1,
    "command": "plan",
    "version": "0.1.0-dev",
    "data": {
        "machine": "thinkpad",
        "profiles": ["base", "dev"],
        "items": [
            {
                "app": "ripgrep",
                "manager": "apt",
                "id": "ripgrep",
                "os": "linux",
                "status": "to-install"
            }
        ],
        "summary": {
            "total": 1,
            "to_install": 1,
            "installed": 0,
            "blocked": 0,
            "failed": 0,
            "skipped": 0
        }
    }
}
```

## Configuration

Paths:

| Item       | Linux                           | Windows                         |
| ---------- | ------------------------------- | ------------------------------- |
| Config     | `~/.config/harpy/config.yaml`   | `%APPDATA%\harpy\config.yaml`   |
| Logs       | `~/.local/state/harpy/logs/`    | `%LOCALAPPDATA%\harpy\logs\`    |
| Index (M2) | `~/.local/share/harpy/harpy.db` | `%LOCALAPPDATA%\harpy\harpy.db` |

Overrides: `HARPY_CONFIG` and `HARPY_DB`.

A ready-to-edit example lives at [`config.example.yaml`](config.example.yaml).
Apps may be plain strings or mappings with explicit IDs per manager:

```yaml
version: 1

defaults:
    managers: [winget, chocolatey] # Linux falls back to apt, pacman
    continue_on_error: true

profiles:
    base:
        apps:
            - git
            - name: ripgrep
              winget: BurntSushi.ripgrep.MSVC
              choco: ripgrep
              apt: ripgrep
              pacman: ripgrep

machines:
    - name: laptop
      match: { os: linux, hostname: 'thinkpad-*' }
      profiles: [base]
```

Manager precedence is filtered by the current OS. If an app declares an
explicit ID, that manager wins; otherwise the app name is used as the ID.

## Supported managers

| Manager    | OS      | Pre-download                            | Elevation |
| ---------- | ------- | --------------------------------------- | --------- |
| winget     | Windows | no (downloads during install)           | no        |
| chocolatey | Windows | no (downloads during install)           | yes       |
| apt        | Linux   | `apt-get update` + `apt-get -d install` | root      |
| pacman     | Linux   | `-Sy`/`-Syu` + `-Sw`                    | root      |

## Testing

Unit tests:

```bash
go test ./...
```

Linux integration in disposable Docker containers (Debian/apt and Arch/pacman):

```powershell
.\scripts\test-linux.ps1 -Distro debian
.\scripts\test-linux.ps1 -Distro arch
.\scripts\test-linux.ps1 -Distro both
```

The harness runs `gofmt`, `go vet`, unit tests, builds the binary, exercises
`harpy managers`, asserts the `plan --json` contract and performs a real
install of `tree` through harpy. See [`docs/TESTING.md`](docs/TESTING.md) for
details, the manual Docker commands and troubleshooting.

## Roadmap

- **M2:** SQLite index, search, cross-OS indexes (`--os`), export/import
- **M3:** bubbletea TUI (search, filters, live logs, add-to-config)
- **M4:** Windows auto-elevation, goreleaser, release hardening

Full execution plan: [`PLAN.md`](PLAN.md).

