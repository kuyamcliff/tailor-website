#!/usr/bin/env bash
# PostToolUse hook for Edit/Write/MultiEdit: format the changed file, then block em dashes in
# customer-facing code and anything that looks like a committed secret.
# Exit code 2 sends the message back to Claude so it fixes the problem.
set -uo pipefail

file=$(python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("tool_input",{}).get("file_path",""))' 2>/dev/null)
[ -z "$file" ] || [ ! -f "$file" ] && exit 0
root=$(git -C "$(dirname "$file")" rev-parse --show-toplevel 2>/dev/null || pwd)

case "$file" in
  *.go) gofmt -w "$file" 2>/dev/null ;;
  "$root"/frontend/*.ts|"$root"/frontend/*.tsx|"$root"/frontend/*.css|"$root"/frontend/*.mjs|"$root"/frontend/*.json)
    (cd "$root/frontend" && npx --no-install prettier --log-level=silent --write "$file" >/dev/null 2>&1) ;;
esac

problems=""
case "$file" in
  "$root"/frontend/app/*|"$root"/frontend/features/*|"$root"/frontend/components/*|"$root"/frontend/lib/*|"$root"/backend/internal/seed/*|"$root"/backend/internal/notifications/*)
    if grep -n $'—' "$file" >/dev/null 2>&1; then
      problems+="Em dash found in customer-facing code ($file). Rewrite the sentence without it.\n"
    fi ;;
esac

# Secret patterns: private keys, cloud keys, common API key formats, and long values assigned to
# secret-like names. .env.example is allowed to name variables but must keep values empty.
if grep -nE -- '-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----|AKIA[0-9A-Z]{16}|sk-ant-[A-Za-z0-9_-]{20,}|sk_live_[0-9a-zA-Z]{20,}|ghp_[A-Za-z0-9]{30,}|xox[baprs]-[A-Za-z0-9-]{10,}' "$file" >/dev/null 2>&1; then
  problems+="Possible secret in $file. Move it to an environment variable.\n"
fi
if grep -nE '(SECRET|PASSWORD|API_KEY|TOKEN|PIN)[A-Z_]*[[:space:]]*[:=][[:space:]]*["'"'"']?[A-Za-z0-9+/_=-]{16,}' "$file" 2>/dev/null | grep -vE 'os\.Getenv|process\.env|example|placeholder|test-callback-secret|dev' >/dev/null; then
  problems+="A secret-like value is assigned in $file. Use an environment variable instead.\n"
fi

if [ -n "$problems" ]; then
  printf "%b" "$problems" >&2
  exit 2
fi
exit 0
