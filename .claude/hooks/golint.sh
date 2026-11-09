#!/bin/sh
# PostToolUse hook: lint the Go module the edited file belongs to.
#
# The session may run from the umbrella folder that holds several repositories,
# so the module cannot be assumed: it is found by walking up from the edited
# file to the nearest go.mod. Anything that is not Go, lives outside a module
# or outside the session's project directory leaves without a word; a go.mod
# found elsewhere could carry a linter config this hook must not execute.
#
# Findings go back as additionalContext rather than plain stdout, which a
# PostToolUse hook only writes into the transcript where the model never sees
# it. A clean run stays silent so the model is not told what it already knows.
set -u

PATH="$HOME/go/bin:$PATH"
export PATH

input=$(cat)

file=$(printf '%s' "$input" | python3 -I -c 'import json,sys
try:
    print(json.load(sys.stdin).get("tool_input", {}).get("file_path", ""))
except Exception:
    print("")' 2>/dev/null)

case "$file" in
*.go) ;;
*) exit 0 ;;
esac

root=$(cd "${CLAUDE_PROJECT_DIR:-.}" 2>/dev/null && pwd -P) || exit 0
dir=$(cd "$(dirname "$file")" 2>/dev/null && pwd -P) || exit 0
case "$dir" in
"$root" | "$root"/*) ;;
*) exit 0 ;;
esac
while [ "$dir" != "$root" ] && [ ! -f "$dir/go.mod" ]; do
	dir=$(dirname "$dir")
done
[ -f "$dir/go.mod" ] || exit 0

command -v golangci-lint >/dev/null 2>&1 || exit 0

findings=$(cd "$dir" && golangci-lint run --fast-only ./... 2>&1 | head -40)
case "$findings" in
"" | *"0 issues"*) exit 0 ;;
esac

MODULE="$dir" FINDINGS="$findings" python3 -I -c 'import json, os
print(json.dumps({"hookSpecificOutput": {
    "hookEventName": "PostToolUse",
    "additionalContext": "golangci-lint --fast-only in "
        + os.path.basename(os.environ["MODULE"]) + ":\n" + os.environ["FINDINGS"],
}}))'
