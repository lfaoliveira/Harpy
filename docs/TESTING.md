# Testing harpy

harpy's first milestone is the headless CLI core. The test suite has three
layers:

| Layer | Where | What it proves |
| --- | --- | --- |
| Go unit tests | Any OS | config parsing, machine/profile resolution, manager selection, sequential apply, logs, exit-code mapping |
| Linux integration | Docker: Debian 12 (apt) and Arch (pacman) | real detection, `plan`, JSON contract, download/install, re-plan after install |
| Windows integration | Host (optional) | winget detection and install; chocolatey needs an elevated terminal |

## Prerequisites

- Go 1.23+ (`mise install` picks up `mise.toml`)
- Docker Desktop (Linux containers) for the Linux harness
- On Windows, run PowerShell from the repository root

## Quick start: Linux in Docker

From Windows PowerShell:

```powershell
# Debian 12 / apt
.\scripts\test-linux.ps1 -Distro debian

# Arch Linux / pacman
.\scripts\test-linux.ps1 -Distro arch

# Both, one after the other
.\scripts\test-linux.ps1 -Distro both
```

The script builds the image, mounts the repository at `/src`, runs the checks
and removes the container. Named volumes keep the Go module and build caches
between runs.

Skip the real package installation (unit tests + `plan` only):

```powershell
.\scripts\test-linux.ps1 -Distro debian -SkipInstall
```

Reuse an existing image:

```powershell
.\scripts\test-linux.ps1 -Distro arch -NoBuild
```

### Manual Docker commands

```bash
docker compose build debian
docker compose run --rm debian

docker compose build arch
docker compose run --rm arch

# Skip the install smoke test
docker compose run --rm -e SMOKE_APPLY=0 debian
```

On Linux/macOS the same script runs directly:

```bash
bash scripts/linux-test.sh apt
bash scripts/linux-test.sh pacman
```

## What the Linux harness runs

1. `gofmt -l cmd internal` must be empty.
2. `go vet ./...`.
3. `go test ./...`.
4. `go build -o /tmp/harpy ./src/cmd/harpy`.
5. `harpy managers` (detects apt or pacman in the container).
6. Generates a temporary config with `jq` (already in the image) and `tree`
   (not installed).
7. `harpy plan --json` and asserts the stable contract:
   - `schema == 1`, `command == "plan"`
   - `data.items[] | select(.app == "jq").status == "installed"`
   - `data.items[] | select(.app == "tree").status == "to-install"`
8. Unless `SMOKE_APPLY=0`, runs `harpy apply --yes --json`, which:
   - downloads with `apt-get -d install` or `pacman -Sw`
   - installs `tree` sequentially
   - writes per-package logs under `$XDG_STATE_HOME/harpy/logs/<run-id>/`
   - asserts the `apply` JSON contract (`schema`, `command`, summary and
     `log_dir`) and that a `*tree*.log` file exists
9. Re-runs `harpy plan --json` and asserts that `tree` is now `installed`.

The containers are disposable: the installed `tree` package disappears with
the container.

## Windows host tests

```powershell
go test ./...
go run ./src/cmd/harpy managers
go run ./src/cmd/harpy plan --profile dev
```

`winget` is the default manager on Windows. `chocolatey` requires an elevated
terminal, and harpy refuses to start an install without elevation:

```powershell
# In an Administrator terminal
go run ./src/cmd/harpy apply --profile base --yes
```

Do not run `apply` against a profile you do not want installed. Use `plan`
first; it never changes the system.

## Test matrix

| Target | Manager | Command |
| --- | --- | --- |
| Debian 12 container | apt | `.\scripts\test-linux.ps1 -Distro debian` |
| Arch container | pacman | `.\scripts\test-linux.ps1 -Distro arch` |
| Windows 11 host | winget | `go run ./src/cmd/harpy plan --profile dev` |
| Windows 11 host (admin) | chocolatey | `go run ./src/cmd/harpy apply --profile base --yes` |
| Final release smoke | all | VM/Windows Sandbox + both containers |

## Troubleshooting

**`docker compose` cannot connect.** Start Docker Desktop and wait for the
engine. Check with `docker version`.

**`bash\r: No such file or directory`.** The shell script was checked out with
CRLF. `.gitattributes` forces LF for `*.sh`; re-checkout the file or run:

```bash
git config core.autocrlf input
git rm --cached scripts/linux-test.sh
git checkout -- scripts/linux-test.sh
```

**Stale images or Go caches.** Rebuild and drop the cache volumes:

```powershell
docker compose build --no-cache
docker compose down -v
```

**apt lock inside the container.** Containers are ephemeral; stop the run and
start a new one. On a host, wait for `unattended-upgrades` or the other apt
process to finish.

**pacman partial upgrade warning.** The default policy is `-Sy` with a warning.
Use `--pacman-sync=full` to run `-Syu` before installing:

```bash
/tmp/harpy apply --yes --pacman-sync=full
```

**winget output is localized.** The search parser arrives with milestone M2 and
will use positional parsing plus pt-BR/en-US fixtures. The current `plan` path
uses `winget list --id ... --exact`, which does not parse the table.

## Scope of this milestone

Covered: config schema v1, machine matching, profiles, detection of the four
managers, `init`, `managers`, `plan`, `apply`, per-package logs, elevation
checks and the JSON envelope.

Not covered yet: search/index (M2), TUI (M3), automatic Windows elevation
relaunch and goreleaser (M4). See `docs/TODO.md`.
