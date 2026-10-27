#!/usr/bin/env bash
# Backfill roadmap release entries from git history.
# Usage: JC_API_BASE=https://api.japa.to JC_INGEST_KEY=<key> ./scripts/changelog-backfill.sh ["3 months ago"]
#
# Emits git log in the GitHub push-event commit shape and posts batches of
# 100 to roadmap.entries.ingest via the MCP bridge. SHA dedupe makes
# re-runs idempotent. Ref: ADR-135.
set -euo pipefail
SINCE="${1:-3 months ago}"
: "${JC_API_BASE:?set JC_API_BASE}" "${JC_INGEST_KEY:?set JC_INGEST_KEY}"

git log --since="$SINCE" --pretty=format:'%H%x1f%s%x1f%an%x1f%aI%x1f' --name-only |
python3 -c '
import sys, json
commits, cur = [], None
for line in sys.stdin:
    line = line.rstrip("\n")
    if "\x1f" in line:
        if cur: commits.append(cur)
        sha, subj, author, ts, _ = line.split("\x1f")
        cur = {"id": sha, "message": subj, "url": "", "timestamp": ts,
               "author": {"name": author}, "added": [], "modified": [], "removed": []}
    elif line and cur:
        cur["modified"].append(line)
if cur: commits.append(cur)
for i in range(0, len(commits), 100):
    print(json.dumps({"commits": commits[i:i+100]}))
' | while read -r batch; do
  jq -n --argjson c "$batch" \
    '{jsonrpc:"2.0",id:1,method:"tools/call",
      params:{name:"roadmap.entries.ingest",arguments:$c}}' |
  curl -fsS -X POST "$JC_API_BASE/api/v1/mcp" \
    -H "X-API-Key: $JC_INGEST_KEY" -H "Content-Type: application/json" -d @-
  echo
done
