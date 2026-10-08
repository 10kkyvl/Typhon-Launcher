#!/usr/bin/env bash
set -u

try=0
if [ "${1:-}" = "--try" ]; then
  try=1
  shift
fi
[ $# -gt 0 ] || { echo "usage: .claude/heavy.sh [--try] <command...>" >&2; exit 2; }

common=$(git rev-parse --path-format=absolute --git-common-dir 2>/dev/null) || { echo "heavy.sh: not inside the repository" >&2; exit 2; }
lock="$common/typhon-heavy.lock"

announced=0
until mkdir "$lock" 2>/dev/null; do
  owner=$(cat "$lock/pid" 2>/dev/null || true)
  # A holder killed without running its trap (agent stopped, terminal closed) leaves the
  # directory behind; its pid no longer exists, so the lock is taken over.
  if [ -n "$owner" ] && ! kill -0 "$owner" 2>/dev/null; then
    rm -rf "$lock"
    continue
  fi
  if [ $try -eq 1 ]; then
    echo "heavy.sh: queue busy, skipped: $*" >&2
    exit 0
  fi
  if [ $announced -eq 0 ]; then
    echo "heavy.sh: waiting for: $(cat "$lock/cmd" 2>/dev/null)" >&2
    announced=1
  fi
  sleep 3
done
echo $$ >"$lock/pid"
printf '%s  %s\n' "$(pwd)" "$*" >"$lock/cmd"
trap 'rm -rf "$lock"' EXIT INT TERM HUP

cores=$(getconf _NPROCESSORS_ONLN 2>/dev/null || nproc 2>/dev/null || echo 4)
half=$(( cores / 2 > 1 ? cores / 2 : 1 ))
export GOMAXPROCS="${GOMAXPROCS:-$half}"
case " ${GOFLAGS:-} " in
  *" -p="*) ;;
  *) export GOFLAGS="${GOFLAGS:-} -p=$half" ;;
esac

"$@"
