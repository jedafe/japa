#!/usr/bin/env bash
# Backfill roadmap release entries from git history.
# Usage: JC_API_BASE=https://api.japa.to/api/v1 JC_INGEST_KEY=<key> ./scripts/changelog-backfill.sh ["3 months ago"]
# JC_API_BASE matches the repo's PUBLIC_JC_API_URL variable (carries /api/v1).
#
# Emits git log in the GitHub push-event commit shape and posts batches of
# 100 to roadmap.entries.ingest via the MCP bridge. Commits are delimited
# by \x1e (record separator) so multi-line %B bodies parse cleanly —
# BREAKING CHANGE footers survive to the ingest parser. SHA dedupe makes
# re-runs idempotent. Ref: ADR-135.
set -euo pipefail
SINCE="${1:-3 months ago}"
: "${JC_API_BASE:?set JC_API_BASE}" "${JC_INGEST_KEY:?set JC_INGEST_KEY}"

cd "$(git rev-parse --show-toplevel)"

git log --since="$SINCE" --pretty=format:'%x1e%H%x1f%B%x1f%an%x1f%aI%x1f' --name-only |
python3 -c '
import sys, json
commits = []
for record in sys.stdin.read().split("\x1e"):
    record = record.strip("\n")
    if not record:
        continue
    parts = record.split("\x1f")
    # sha \x1f body \x1f author \x1f ts \x1f <file lines>
    if len(parts) < 4:
        continue
    sha, body, author, ts = parts[0].strip(), parts[1], parts[2], parts[3].strip()
    files = [f.strip() for f in "\n".join(parts[4:]).split("\n") if f.strip()]
    commits.append({
        "id": sha, "message": body, "url": "", "timestamp": ts,
        "author": {"name": author},
        "added": [], "modified": files, "removed": [],
    })
for i in range(0, len(commits), 100):
    print(json.dumps({"commits": commits[i:i+100]}))
' | while read -r batch; do
  # Batches can exceed argv limits — wrap via stdin, not --argjson.
  echo "$batch" | jq '{jsonrpc:"2.0",id:1,method:"tools/call",
    params:{name:"roadmap.entries.ingest",arguments:.}}' |
  curl -fsS -X POST "$JC_API_BASE/mcp" \
    -H "X-API-Key: $JC_INGEST_KEY" -H "Content-Type: application/json" -d @-
  echo
done
