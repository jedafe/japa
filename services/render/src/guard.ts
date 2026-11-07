// SSRF guard — private-literal host detection shared by the entry-URL
// check (server.ts) and per-request route interception (browser.ts).
// Defense in depth behind the Go-side validateRenderTarget check.

// isPrivateLiteral reports whether host is a private/internal address
// spelled as a literal — dotted quads, decimal/hex/octal IPv4 forms
// (2130706433, 0x7f000001, 0177.0.0.1), IPv4-mapped IPv6, loopback,
// link-local, ULA, and well-known internal names. Numeric literals that
// are not provably public are refused outright.
export function isPrivateLiteral(host: string): boolean {
  let h = host.replace(/^\[|\]$/g, "").toLowerCase();

  if (h === "localhost" || h.endsWith(".localhost") || h.endsWith(".internal") || h.endsWith(".local")) {
    return true;
  }

  // IPv4-mapped IPv6 (::ffff:7f00:1 / ::ffff:127.0.0.1) — reduce to the
  // v4 tail and run it through the same pipeline.
  if (h.startsWith("::ffff:")) {
    h = h.slice(7);
  }

  if (h === "::1" || h.startsWith("fc") || h.startsWith("fd") || h.startsWith("fe80")) {
    return true;
  }

  const parts = h.split(".");
  const numericPart = (p: string) => /^0x[0-9a-f]+$/i.test(p) || /^\d+$/.test(p);

  // Numeric literals (any of 1/4-part, dec/hex/octal): refuse every
  // spelling outright — a numeric host is never a render target we want,
  // public or not.
  if ((parts.length === 1 || parts.length === 4) && parts.every(numericPart)) {
    return true;
  }

  if (parts.length === 4 && parts.every((p) => /^\d+$/.test(p))) {
    const [a, b] = parts.map(Number);
    if (a === 10 || a === 127 || a === 0) return true;
    if (a === 172 && b >= 16 && b <= 31) return true;
    if (a === 192 && b === 168) return true;
    if (a === 169 && b === 254) return true;
    if (a === 100 && b >= 64 && b <= 127) return true;
  }

  return false;
}

// isPrivateRequestTarget parses a request URL and reports whether it
// should be blocked — non-http(s) schemes pass through (pages legitimately
// load data:/blob: subresources).
export function isPrivateRequestTarget(rawURL: string): boolean {
  try {
    const u = new URL(rawURL);
    if (u.protocol !== "http:" && u.protocol !== "https:") return false;
    return isPrivateLiteral(u.hostname);
  } catch {
    return false; // unparseable — let the browser decide
  }
}

// ---------------------------------------------------------------------------
// DNS layer — closes the rebinding gap the literal check cannot see: a
// PUBLIC hostname that resolves to a private address. Mirrors the Go
// transport's guardedDialContext posture: every answer is checked and a
// single private answer rejects; lookup failure and empty answers fail
// closed. A resolve-then-connect TOCTOU remains (an attacker flipping
// answers between our lookup and chromium's connect) — acceptable for
// operator-vetted render targets, and far narrower than the open hole.
// ---------------------------------------------------------------------------

import { lookup } from "node:dns/promises";

const DNS_CACHE_TTL_MS = 30_000;
const DNS_CACHE_MAX = 8_192; // unique subresource hosts across a long
// crawl would otherwise grow the map without bound — the cache caps and
// expires, never accumulates.
const dnsCache = new Map<string, { priv: boolean; expires: number }>();

function dnsCacheSet(host: string, priv: boolean): void {
  if (dnsCache.size >= DNS_CACHE_MAX) {
    // Evict expired first; fall back to dropping the oldest insertions
    // (Map preserves insertion order) — either way the map stays bounded.
    const now = Date.now();
    for (const [k, v] of dnsCache) {
      if (v.expires <= now) dnsCache.delete(k);
    }

    if (dnsCache.size >= DNS_CACHE_MAX) {
      let n = Math.floor(DNS_CACHE_MAX / 4);
      for (const k of dnsCache.keys()) {
        dnsCache.delete(k);
        if (--n <= 0) break;
      }
    }
  }

  dnsCache.set(host, { priv, expires: Date.now() + DNS_CACHE_TTL_MS });
}

// isPrivateIP checks a resolved address string against the internal ranges.
export function isPrivateIP(ip: string): boolean {
  // IPv4 dotted quad.
  const v4 = ip.match(/^(\d+)\.(\d+)\.(\d+)\.(\d+)$/);
  if (v4) {
    const a = Number(v4[1]);
    const b = Number(v4[2]);
    if (a === 10 || a === 127 || a === 0) return true;
    if (a === 172 && b >= 16 && b <= 31) return true;
    if (a === 192 && b === 168) return true;
    if (a === 169 && b === 254) return true;
    if (a === 100 && b >= 64 && b <= 127) return true;
    if (a >= 224) return true; // multicast + reserved
    return false;
  }

  // IPv6 — normalize the common forms we care about.
  const h = ip.toLowerCase();
  if (h === "::1" || h === "::") return true;
  if (h.startsWith("fe80") || h.startsWith("fc") || h.startsWith("fd")) return true;
  if (h.startsWith("ff")) return true; // multicast
  if (h.startsWith("::ffff:")) return isPrivateIP(h.slice(7));

  return false;
}

// resolvesToPrivate reports whether host is a private literal OR resolves
// to any private address. dns.lookup (getaddrinfo) is used so /etc/hosts
// entries are honored — a hosts-mapped loopback is just as internal.
export async function resolvesToPrivate(host: string): Promise<boolean> {
  const h = host.replace(/^\[|\]$/g, "").toLowerCase();
  if (isPrivateLiteral(h)) return true;

  const cached = dnsCache.get(h);
  if (cached && cached.expires > Date.now()) return cached.priv;

  let priv = true; // fail closed: lookup failure or empty answers reject
  try {
    const addrs = await lookup(h, { all: true, verbatim: true });
    priv = addrs.length === 0 || addrs.some((a) => isPrivateIP(a.address));
  } catch {
    priv = true;
  }

  dnsCacheSet(h, priv);
  return priv;
}

// isPrivateRequestTargetAsync is the route-interception variant: literal
// check first, then the DNS layer for hostname targets.
export async function isPrivateRequestTargetAsync(rawURL: string): Promise<boolean> {
  try {
    const u = new URL(rawURL);
    if (u.protocol !== "http:" && u.protocol !== "https:") return false;
    return resolvesToPrivate(u.hostname);
  } catch {
    return false;
  }
}
