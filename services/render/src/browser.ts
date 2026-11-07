// Browser + context management for the render service.
//
// One chromium process serves all requests. Named sessions
// (provider_config.session_id, forwarded only under bypass_policy:"auto")
// get a persistent BrowserContext so cookies a challenge sets on the
// first render (Cloudflare cf_clearance) are present on the next — that
// is how a wall turns into a pass without solving anything. Anonymous
// requests use a fresh context each time.
//
// proxy_group resolves to RENDER_PROXY_<GROUP> env URLs; stealth adds a
// realistic UA + webdriver-evasion init script (basic — fingerprint
// spoofing stays a service-side upgrade, not a Go concern).
import { chromium, type Browser, type BrowserContext } from "playwright-core";
import { isPrivateRequestTargetAsync } from "./guard.ts";

const STEALTH_UA =
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36";

// HARDENING_INIT removes the egress paths ctx.route cannot intercept:
// WebSocket handshakes bypass request routing entirely (a page could
// open ws://169.254.169.254 past every guard), and service-worker
// fetches are only partially visible to routing. Content capture needs
// neither — DOM and XHR-rendered data arrive over plain HTTP(S).
const HARDENING_INIT = `
delete window.WebSocket;
delete window.EventSource;
Object.defineProperty(navigator, 'serviceWorker', { get: () => undefined });
`;

const STEALTH_INIT = `
Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
window.chrome = window.chrome || { runtime: {} };
Object.defineProperty(navigator, 'languages', { get: () => ['en-US', 'en'] });
Object.defineProperty(navigator, 'plugins', { get: () => [1, 2, 3] });
const origQuery = window.navigator.permissions && window.navigator.permissions.query;
if (origQuery) {
  window.navigator.permissions.query = (p) =>
    p && p.name === 'notifications'
      ? Promise.resolve({ state: Notification.permission })
      : origQuery.call(window.navigator.permissions, p);
}
`;

interface SessionEntry {
  ctx: BrowserContext;
  lastUsed: number;
}

export interface RenderOptions {
  stealth?: boolean;
  proxyGroup?: string;
  sessionId?: string;
}

export class BrowserPool {
  private browser: Browser | null = null;
  private launching: Promise<Browser> | null = null;
  private sessions = new Map<string, SessionEntry>();
  private readonly sessionTtlMs: number;
  private readonly maxSessions: number;
  private sweeper: ReturnType<typeof setInterval> | null = null;

  constructor(sessionTtlMs: number, maxSessions = 64) {
    this.sessionTtlMs = sessionTtlMs;
    this.maxSessions = maxSessions;
  }

  async start(): Promise<void> {
    this.browser = await this.launch();
    this.sweeper = setInterval(() => void this.sweepSessions(), 60_000);
    this.sweeper.unref();
  }

  private async launch() {
    const b = await chromium.launch({
      args: [
        "--disable-blink-features=AutomationControlled",
        "--no-sandbox", // containers run as non-root; sandbox needs userns
        "--disable-dev-shm-usage",
      ],
    });

    // A crashed/OOM-killed browser leaves dead session contexts — drop the
    // map so the next render re-launches instead of throwing on a closed
    // browser forever.
    b.on("disconnected", () => {
      this.sessions.clear();
      this.browser = null;
    });

    return b;
  }

  async stop(): Promise<void> {
    if (this.sweeper) clearInterval(this.sweeper);
    for (const [, s] of this.sessions) await s.ctx.close().catch(() => {});
    this.sessions.clear();
    await this.browser?.close().catch(() => {});
    this.browser = null;
  }

  private async sweepSessions(): Promise<void> {
    const now = Date.now();
    for (const [id, s] of this.sessions) {
      if (now - s.lastUsed > this.sessionTtlMs) {
        this.sessions.delete(id);
        await s.ctx.close().catch(() => {});
      }
    }
  }

  // context returns a live BrowserContext for the request. Named sessions
  // are borrowed — the caller MUST NOT close them; anonymous contexts are
  // owned by the caller (close in finally). A disconnected browser (crash,
  // OOM) is re-launched lazily here.
  async context(opts: RenderOptions): Promise<{ ctx: BrowserContext; owned: boolean }> {
    if (!this.browser || !this.browser.isConnected()) {
      // Dead browser's contexts are unusable — the disconnected handler
      // may not have fired yet, so clear here too before re-launching.
      // The launch itself is deduplicated: concurrent renders hitting a
      // dead browser must share ONE new chromium, not spawn one each.
      this.sessions.clear();

      this.launching ??= this.launch().finally(() => {
        this.launching = null;
      });

      this.browser = await this.launching;
    }

    // The session key folds in proxy_group + stealth: a config change
    // gets a fresh context rather than silently reusing a session built
    // under the old egress/persona.
    const key = opts.sessionId
      ? `${opts.sessionId}|${opts.proxyGroup || ""}|${opts.stealth ? "1" : "0"}`
      : "";

    if (opts.sessionId) {
      const existing = this.sessions.get(key);
      if (existing) {
        existing.lastUsed = Date.now();
        return { ctx: existing.ctx, owned: false };
      }

      if (this.sessions.size >= this.maxSessions) {
        // Evict the LRU session rather than reject — a stale session is
        // cheaper than a failed render.
        let oldestId = "";
        let oldest = Infinity;
        for (const [id, s] of this.sessions) {
          if (s.lastUsed < oldest) {
            oldest = s.lastUsed;
            oldestId = id;
          }
        }
        if (oldestId) {
          const s = this.sessions.get(oldestId);
          this.sessions.delete(oldestId);
          await s?.ctx.close().catch(() => {});
        }
      }

      const ctx = await this.newContext(opts);
      this.sessions.set(key, { ctx, lastUsed: Date.now() });
      return { ctx, owned: false };
    }

    return { ctx: await this.newContext(opts), owned: true };
  }

  private async newContext(opts: RenderOptions): Promise<BrowserContext> {
    if (!this.browser) throw new Error("browser not started");

    const ctx = await this.browser.newContext({
      userAgent: opts.stealth ? STEALTH_UA : undefined,
      locale: "en-US",
      viewport: { width: 1366, height: 768 },
      proxy: opts.proxyGroup ? proxyForGroup(opts.proxyGroup) : undefined,
      bypassCSP: false,
    });

    await ctx.addInitScript(HARDENING_INIT);

    if (opts.stealth) {
      await ctx.addInitScript(STEALTH_INIT);
    }

    // SSRF: the entry URL is validated upstream, but redirects and page
    // subresources (an <img src="http://169.254.169.254/"> on the rendered
    // page) are fetched unchecked — intercept every request and abort
    // private targets. The DNS layer resolves each hostname and rejects
    // any private answer (resolve-then-connect TOCTOU remains — see
    // guard.ts). Lookup results cache 30s so a resource-heavy page pays
    // one resolution per unique host, not per request.
    await ctx.route("**/*", async (route) => {
      if (await isPrivateRequestTargetAsync(route.request().url())) {
        return route.abort("blockedbyclient");
      }

      return route.continue();
    });

    return ctx;
  }
}

// proxyForGroup maps a proxy_group name to RENDER_PROXY_<GROUP>. The
// Go side forwards the group name only under bypass_policy:"auto"; the
// service resolves the actual credentials locally so pool secrets never
// travel on the render request.
export function proxyForGroup(group: string): { server: string; username?: string; password?: string } | undefined {
  const raw = process.env[`RENDER_PROXY_${group.toUpperCase().replace(/[^A-Z0-9]/g, "_")}`];
  if (!raw) return undefined;

  try {
    const u = new URL(raw);
    return {
      server: `${u.protocol}//${u.host}`,
      username: u.username || undefined,
      password: u.password || undefined,
    };
  } catch {
    return undefined;
  }
}
