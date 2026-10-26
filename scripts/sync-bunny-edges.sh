#!/usr/bin/env bash
# Sync BunnyCDN edge IPs into the $is_cdn_edge geo block in docker/nginx.conf
# (ADR-134). The CDN country header (CDN-RequestCountryCode) is only forwarded
# to the app when the request's true peer is a CDN edge — this regenerates the
# marked bunny section inside that geo block from Bunny's published list.
#
# Usage: ./scripts/sync-bunny-edges.sh [path/to/nginx.conf]
set -euo pipefail

CONF="${1:-docker/nginx.conf}"
BEGIN="# --- bunny-edge-begin"
END="# --- bunny-edge-end"

ips=$(curl -fsSL --max-time 15 "https://api.bunny.net/system/edgeserverlist" | tr ',[]" \n' ' ')

[ -n "${ips// }" ] || { echo "sync-bunny-edges: empty edge list — aborting" >&2; exit 1; }

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

awk -v begin="$BEGIN" -v end="$END" -v ips="$ips" '
    $0 ~ begin {
        print
        split(ips, a, " ")
        for (i in a) if (a[i] != "") printf "    %s 1;\n", a[i]
        skip = 1
        next
    }
    $0 ~ end { skip = 0; print; next }
    !skip { print }
' "$CONF" > "$tmp"

mv "$tmp" "$CONF"
echo "sync-bunny-edges: updated $CONF"
