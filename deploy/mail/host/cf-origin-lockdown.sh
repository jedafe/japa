#!/bin/bash
# cf-origin-lockdown.sh — restrict inbound web ports on the public interface
# to Cloudflare egress ranges (origin lockdown, ADR-134).
#
# Mechanism: a dedicated CFLOCK4/CFLOCK6 chain jumped to from the top of
# DOCKER-USER. Ports 80, 443 (tcp+udp), and 8080 arriving on the public
# interface from non-Cloudflare source addresses are dropped. All other
# traffic (SSH, docker-internal flows, other ports) is untouched.
#
# Rebuilds are idempotent: only the CFLOCK* chains are flushed/repopulated;
# DOCKER-USER itself is never flushed. Safe to re-run anytime.
#
# CF ranges are fetched from cloudflare.com/ips (fresh on each run) with a
# baked-in fallback list so a fetch failure can never empty the allowlist.

set -euo pipefail

PUB_IF="${PUB_IF:-$(ip route show default | awk '/default/ {print $5; exit}')}"
PORTS_TCP="80,443,8080"
PORTS_UDP="443"

# Non-Cloudflare exceptions live in the EXTRA4/EXTRA6 chains, jumped to just
# before the DROPs below. They are populated by cf-admin-allow.sh (static
# CIDRs + DDNS-resolved hostnames), which also runs on a timer — so this
# script never needs IP values of its own.

FALLBACK_V4="173.245.48.0/20 103.21.244.0/22 103.22.200.0/22 103.31.4.0/22 141.101.64.0/18 108.162.192.0/18 190.93.240.0/20 188.114.96.0/20 197.234.240.0/22 198.41.128.0/17 162.158.0.0/15 104.16.0.0/13 104.24.0.0/14 172.64.0.0/13 131.0.72.0/22"
FALLBACK_V6="2400:cb00::/32 2606:4700::/32 2803:f800::/32 2405:b500::/32 2405:8100::/32 2a06:98c0::/29 2c0f:f248::/32"

fetch_ranges() {
    local url="$1" fallback="$2" out
    out="$(curl -fsS --max-time 15 "$url" 2>/dev/null | tr '\n' ' ')" || true
    if [ -z "${out// /}" ]; then out="$fallback"; fi
    echo "$out"
}

if [ -z "$PUB_IF" ]; then
    echo "cf-lockdown: could not determine public interface" >&2
    exit 1
fi

apply_family() {
    local bin="$1" chain="$2" ranges="$3" extra_chain="$4"

    if ! "$bin" -t filter -L DOCKER-USER -n >/dev/null 2>&1; then
        echo "cf-lockdown: DOCKER-USER absent for $bin — docker not ready, skipping"
        return 0
    fi

    "$bin" -N "$chain" 2>/dev/null || true
    "$bin" -F "$chain"

    local cidr
    for cidr in $ranges; do
        "$bin" -A "$chain" -i "$PUB_IF" -s "$cidr" -j RETURN
    done

    # Exceptions (static + DDNS-resolved) are consulted before the DROPs; the
    # chain may be empty until cf-admin-allow.sh first populates it.
    "$bin" -N "$extra_chain" 2>/dev/null || true
    "$bin" -A "$chain" -j "$extra_chain"
    "$bin" -A "$chain" -i "$PUB_IF" -p tcp -m multiport --dports "$PORTS_TCP" -j DROP
    "$bin" -A "$chain" -i "$PUB_IF" -p udp --dport "$PORTS_UDP" -j DROP
    # Anything else falls off the end of the chain = implicit RETURN = allowed.

    if ! "$bin" -C DOCKER-USER -j "$chain" 2>/dev/null; then
        "$bin" -I DOCKER-USER 1 -j "$chain"
    fi
}

V4="$(fetch_ranges https://www.cloudflare.com/ips-v4 "$FALLBACK_V4")"
V6="$(fetch_ranges https://www.cloudflare.com/ips-v6 "$FALLBACK_V6")"

apply_family iptables  CFLOCK4 "$V4" EXTRA4
apply_family ip6tables CFLOCK6 "$V6" EXTRA6

# Populate the exception chains (static + DDNS-resolved); also runs standalone
# on cf-admin-allow.timer for mid-cycle IP changes.
if [ -x /usr/local/sbin/cf-admin-allow.sh ]; then
    /usr/local/sbin/cf-admin-allow.sh || echo "cf-lockdown: cf-admin-allow failed" >&2
fi

echo "cf-lockdown: applied on $PUB_IF (v4: $(echo $V4 | wc -w) ranges, v6: $(echo $V6 | wc -w) ranges)"
