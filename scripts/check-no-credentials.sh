#!/usr/bin/env bash
#
# Fails the build if a credential is committed to source or bundled into a
# build artifact.
#
# This exists because a live Telegram bot token was once hardcoded in this
# project's source. Rotating that token fixed the incident; this script is what
# stops the next one. It is a required CI step, not a linter — treat a failure
# here as a security event, not a style nit.
#
# Usage:
#   scripts/check-no-credentials.sh                 # source scan only
#   scripts/check-no-credentials.sh build/bin dist  # also scan build outputs
#
# Exit status: 0 clean, 1 something was found.

set -euo pipefail

# Telegram bot tokens are "<numeric bot id>:<35-char secret>". The trailing
# group rejects longer alphanumeric runs so that git hashes and base64 blobs,
# which are otherwise the same alphabet, do not produce false positives.
readonly TELEGRAM_TOKEN_PATTERN='(^|[^0-9A-Za-z_-])[0-9]{8,10}:[A-Za-z0-9_-]{35}([^A-Za-z0-9_-]|$)'

# This script necessarily contains the pattern it searches for, and the docs
# describe it. Excluding them by path is safer than trying to write a regex
# that cannot match itself.
readonly SELF_EXCLUDED_PATHS=(
  'scripts/check-no-credentials.sh'
  'docs/DEVELOPMENT.md'
)

problems_found=0

report_problem() {
  echo "FAIL: $1" >&2
  problems_found=1
}

is_self_excluded() {
  local candidate_path="$1"
  local excluded_path
  for excluded_path in "${SELF_EXCLUDED_PATHS[@]}"; do
    [[ "$candidate_path" == "$excluded_path" ]] && return 0
  done
  return 1
}

# --- 1. No credential-shaped string anywhere in tracked source ---------------
#
# Deliberately scans `git ls-files` rather than the working tree: the working
# tree holds a real .env that is gitignored and must never be read by CI, and
# tracked files are exactly what an attacker would have published.

echo "Scanning tracked source for Telegram bot tokens..."

while IFS= read -r -d '' tracked_path; do
  is_self_excluded "$tracked_path" && continue

  # -I skips binary files, which cannot leak a readable token and produce
  # unusable diagnostics when they match by chance.
  if match_output=$(grep -InE "$TELEGRAM_TOKEN_PATTERN" -- "$tracked_path" 2>/dev/null); then
    report_problem "credential-shaped string in tracked file '$tracked_path'"
    # Print only the line number and a redacted marker. Echoing the match
    # would copy the secret into public CI logs.
    echo "$match_output" | cut -d: -f1 | sed 's/^/       line /' >&2
  fi
done < <(git ls-files -z)

# --- 2. .env must never be tracked -------------------------------------------

echo "Checking that .env is not tracked..."

while IFS= read -r tracked_env_path; do
  report_problem "'$tracked_env_path' is tracked by git; it must stay gitignored"
done < <(git ls-files | grep -E '(^|/)\.env$' || true)

# --- 3. No .env in any build output or bundle --------------------------------
#
# An installer that scoops up the developer's .env ships their bot token to
# every user. Each argument is a directory that ends up in a release artifact.

for output_directory in "$@"; do
  [[ -d "$output_directory" ]] || continue
  echo "Scanning build output '$output_directory' for .env files..."

  while IFS= read -r -d '' bundled_env_path; do
    report_problem "'$bundled_env_path' was bundled into a build output"
  # The parentheses are load-bearing: without them -print0 binds only to the
  # last -o branch and the plain '.env' case is found but never reported.
  done < <(find "$output_directory" \
    \( -name '.env' -o \( -name '.env.*' ! -name '.env.example' \) \) \
    -print0 2>/dev/null)
done

# --- Result ------------------------------------------------------------------

if [[ "$problems_found" -ne 0 ]]; then
  cat >&2 <<'REMEDIATION'

A credential or credential-shaped string was found.

If it is a real secret:
  1. Revoke it now. For a Telegram bot, message @BotFather and use /revoke.
  2. Remove it from source and from git history.
  3. Only then push again. A token in a public commit is compromised the
     moment it is pushed, whether or not the commit is later amended.

If it is a false positive, exclude the specific path in
scripts/check-no-credentials.sh rather than loosening the pattern.
REMEDIATION
  exit 1
fi

echo "OK: no credentials found in tracked source or build output."
