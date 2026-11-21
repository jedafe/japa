#!/bin/sh
# acme-entrypoint.sh — issue the gate hostname's cert via acme.sh DNS-01
# (Cloudflare API), then hand off to the image's `daemon` entrypoint for
# cron-based renewal.
#
# DNS-01 is required: the host CFLOCK4 lockdown drops the inbound HTTP-01 /
# TLS-ALPN-01 validation traffic for a DNS-only hostname. api.cloudflare.com
# is outbound-only, which the lockdown doesn't touch.
#
# Idempotent: acme.sh detects an existing unexpired cert and skips issuance.
# RSA 4096 keeps the cert dir at /acme.sh/<domain>/ (no _ecc suffix) — the
# layout the compose file's SFTPGO_*_CERTIFICATE_* paths expect.
set -e

DOMAIN="${FILES_DOMAIN:?FILES_DOMAIN env required}"

if [ -n "${ACME_EMAIL:-}" ]; then
    acme.sh --register-account -m "$ACME_EMAIL" --server letsencrypt || true
fi

# Exit code 2 = "domains not changed — skip" — not a failure. Any other
# failure is left for the renewal cron to retry.
acme.sh --issue --dns dns_cf -d "$DOMAIN" --keylength 4096 --server letsencrypt || \
    echo "acme.sh --issue for $DOMAIN non-fatal (skip or retry via cron)"

exec /entry.sh daemon
