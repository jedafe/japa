#!/usr/bin/env bash
# Rule 17 cleanup — strips AI-attribution trailers from commit history.
# DESTRUCTIVE: rewrites every commit SHA. Requires force-push afterward.
# Run on a mirror or coordinated branch; all clones must re-sync.
#
#   ./scripts/scrub-ai-trailers.sh            # dry run — prints matches
#   ./scripts/scrub-ai-trailers.sh --apply    # rewrite history
#
# After --apply:
#   git push --force-with-lease origin main
#   (and the same for the .jeos submodule if its history is also affected)
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

PATTERN='Co-[Aa]uthored-[Bb]y:.*(Claude|Devin|CommandCodeBot|Cursor|noreply@anthropic\.com|devin-ai-integration)'

if [ "${1:-}" != "--apply" ]; then
  echo "== trailers that would be stripped =="
  git log --format="%H %s%n%b" | grep -E "$PATTERN" | sort | uniq -c
  echo
  echo "run with --apply to rewrite history"
  exit 0
fi

# git filter-repo preferred; falls back to filter-branch if unavailable.
if command -v git-filter-repo >/dev/null 2>&1; then
  git filter-repo --force --message-callback '
import re
msg = message.decode("utf-8", "replace")
msg = re.sub(r"\n*Co-[Aa]uthored-[Bb]y:[^\n]*(Claude|Devin|CommandCodeBot|Cursor|anthropic|devin-ai-integration)[^\n]*", "", msg)
msg = re.sub(r"\n{3,}", "\n\n", msg)
return msg.encode()'
else
  git filter-branch --force --msg-filter '
sed -E "/Co-[Aa]uthored-[Bb]y:.*(Claude|Devin|CommandCodeBot|Cursor|anthropic|devin-ai-integration)/d"
' --tag-name-filter cat -- --all
fi

echo "Done. Verify with: git log --format=%b | grep -iE 'claude|devin|commandcode'"
echo "Then: git push --force-with-lease origin main"
