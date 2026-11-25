# harpy — TODO

Status of the execution plan in [`PLAN.md`](../PLAN.md).

## Done (M0 + M1 CLI core)

- Go module `github.com/lfaoliveira/harpy`, static-friendly (no CGO).
- Config schema v1: scalars/mappings for apps, per-manager IDs, machine match
  by OS + hostname glob, profile merge, unknown keys as warnings.
- `harpy init`, `harpy managers`, `harpy plan`, `harpy apply`,
  `harpy version`, stable `--json` envelope (`schema: 1`).
- Managers: winget, chocolatey, apt, pacman (`Installed`, `Download`,
  `Install`; download sentinel for winget/choco).
- Sequential install use case with optional manager-level pre-download,
  continue-on-error, per-package logs and summary.
- Elevation detection (root on Linux, token elevation on Windows) with a clear
  refusal before changing the system.
- Layered architecture: pure `model` contracts, `domain` use cases, `manager`
  adapters, host I/O in `platform`, and a thin `cli` presentation layer.
- Linux Docker harness: Debian 12 + Arch, unit tests and a real `apply` smoke
  test.

## Next: M2 — index and search

- SQLite index (`modernc.org/sqlite`), schema and TTL from PLAN §4.
- Search core: config → index → live query, fuzzy/literal, 25-per-page /
  200 ceiling, dedupe and installed boost.
- `harpy search`, `harpy index refresh|status|export|import`.
- winget positional parser with pt-BR/en-US fixtures.
- Cross-OS indexes via WSL/Docker/import; read-only results for other OSes.

## Next: M3 — TUI

- bubbletea list/search/log/summary screens, filters, pagination, add-to-config
  via `yaml.Node` with `.bak` backup.

## Next: M4 — release hardening

- Windows auto-relaunch via `ShellExecuteEx "runas"`.
- goreleaser: Windows + Linux, static binaries, checksums.
- Full `--json` coverage for every command.

## Explicit non-goals in v1

harpy does **not** manage developer dependencies. Runtimes, npm/pip/cargo
packages and dotfiles belong to `upkg`; harpy will only delegate to it later.

```yaml
# FUTURE — not implemented
version: 2
dev:
    runtimes:
        node: '24'
        python: '3.13'
        go: 'latest'
    managers: [npm, pip, cargo]
    dotfiles:
        tool: chezmoi
        source: git@github.com:user/dotfiles.git
sync:
    provider: git | gdrive | server
```

## Experiments / future ideas

- FTS5 for the local index (only if LIKE + Go fuzzy is not enough).
- winget catalog on Linux (MSIX `index.db` extraction — unsupported hack).
- AUR support.
- Code signing.
- Cross-check winget ↔ chocolatey install detection.
- Automatic best-effort `index refresh` when the index is stale.
- For mobile devices, create a mobile-first interface (Web or Native, we'll see) that allows to manage all official AND Side-loaded apps. Maybe have soft-uninstall? meaning that side-loaded apps stay inactive for 30 days instead of hard uninstallation.
- In the mobile devices case, focus on Android and use Android's native APIs for installation management (use unofficial APIs for cataloguing and searching Official Play Store Apps ).
