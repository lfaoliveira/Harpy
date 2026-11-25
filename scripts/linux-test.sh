#!/usr/bin/env bash
# Linux test harness for harpy. Runs inside a Debian/Arch container (see
# docker-compose.yml) or on any Linux host.
set -euo pipefail

manager="${1:-}"
if [[ -z "$manager" ]]; then
	if command -v apt-get >/dev/null 2>&1; then
		manager="apt"
	elif command -v pacman >/dev/null 2>&1; then
		manager="pacman"
	else
		echo "cannot detect a supported package manager (apt-get or pacman)" >&2
		exit 1
	fi
fi

case "$manager" in
apt | pacman) ;;
*)
	echo "unknown manager: $manager (expected apt or pacman)" >&2
	exit 2
	;;
esac

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

echo "== harpy Linux tests: manager=$manager os=$(uname -srm) =="
go version

echo "== gofmt =="
unformatted="$(gofmt -l src)"
if [[ -n "$unformatted" ]]; then
	echo "unformatted files:" >&2
	echo "$unformatted" >&2
	exit 1
fi

echo "== go vet =="
go vet ./...

echo "== go test =="
go test ./...

echo "== build =="
go build -o /tmp/harpy ./src

echo "== managers =="
/tmp/harpy managers

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

cat >"$work/config.yaml" <<EOF
version: 1
search:
  mode: fuzzy
defaults:
  managers: [$manager]
  continue_on_error: true
profiles:
  smoke:
    apps:
      - jq
      - tree
machines:
  - name: container
    match: { os: linux, hostname: '*' }
    profiles: [smoke]
EOF
export HARPY_CONFIG="$work/config.yaml"

echo "== plan (human) =="
/tmp/harpy plan

echo "== plan (json contract) =="
plan_json="$(/tmp/harpy plan --json)"
printf '%s\n' "$plan_json" | jq -e '.schema == 1 and .command == "plan"' >/dev/null
printf '%s\n' "$plan_json" | jq -e '.data.items[] | select(.app == "jq" and .status == "installed")' >/dev/null
printf '%s\n' "$plan_json" | jq -e '.data.items[] | select(.app == "tree" and .status == "to-install")' >/dev/null
echo "json contract OK"

if [[ "${SMOKE_APPLY:-1}" == "1" ]]; then
	echo "== apply smoke: install tree through harpy (json) =="
	apply_json="$(/tmp/harpy apply --yes --json)"
	printf '%s\n' "$apply_json" | jq -e '.schema == 1 and .command == "apply"' >/dev/null
	printf '%s\n' "$apply_json" | jq -e '.data.summary.failed == 0' >/dev/null
	printf '%s\n' "$apply_json" | jq -e '.data.items[] | select(.app == "tree" and .status == "installed")' >/dev/null
	log_dir="$(printf '%s\n' "$apply_json" | jq -r '.data.log_dir')"
	test -n "$log_dir"
	test -d "$log_dir"
	log_file="$(find "$log_dir" -type f -name '*tree*.log' -print -quit)"
	test -n "$log_file"
	test -f "$log_file"
	echo "apply json + logs OK: $log_dir"

	echo "== re-plan: tree must now be installed =="
	after_json="$(/tmp/harpy plan --json)"
	printf '%s\n' "$after_json" | jq -e '.data.items[] | select(.app == "tree" and .status == "installed")' >/dev/null

	if command -v tree >/dev/null 2>&1; then
		tree --version >/dev/null
	fi
	echo "apply smoke OK: tree installed"
else
	echo "SMOKE_APPLY=0; skipping real install"
fi

echo "== OK =="
