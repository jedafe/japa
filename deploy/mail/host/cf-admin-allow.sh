#!/bin/bash
# cf-admin-allow.sh — reconcile the EXTRA4/EXTRA6 exception chains that
# CFLOCK4/CFLOCK6 jump to just before their DROP rules. Static CIDRs plus
# A/AAAA-resolved DDNS hostnames are allowed to reach TCP :443 directly —
# used for the Stalwart admin gate on mail.japa.to (MX target cannot be
# Cloudflare-proxied, so the CF origin lockdown would otherwise hide it).
#
# Run by cf-admin-allow.timer every couple of minutes and once by
# cf-origin-lockdown.sh after each lockdown rebuild. Only the EXTRA* chains
# are flushed/repopulated; CFLOCK* and DOCKER-USER are never touched.

set -euo pipefail

PUB_IF="${PUB_IF:-$(ip route show default | awk '/default/ {print $5; exit}')}"

STATIC_V4=""
STATIC_V6=""
HOSTS_V4="gate-admin.japa.to"
HOSTS_V6=""

resolve_v4() { getent ahostsv4 "$1" 2>/dev/null | awk '{print $1}' | sort -u; }
resolve_v6() { getent ahostsv6 "$1" 2>/dev/null | awk '{print $1}' | sort -u; }

populate() {
    local bin="$1" chain="$2" static="$3" hosts="$4" resolver="$5"

    "$bin" -N "$chain" 2>/dev/null || true
    "$bin" -F "$chain"

    local src ip
    for src in $static; do
        "$bin" -A "$chain" -i "$PUB_IF" -p tcp --dport 443 -s "$src" -j ACCEPT
    done
    for src in $hosts; do
        for ip in $($resolver "$src"); do
            "$bin" -A "$chain" -i "$PUB_IF" -p tcp --dport 443 -s "$ip" -j ACCEPT
        done
    done
}

populate iptables  EXTRA4 "$STATIC_V4" "$HOSTS_V4" resolve_v4
populate ip6tables EXTRA6 "$STATIC_V6" "$HOSTS_V6" resolve_v6
