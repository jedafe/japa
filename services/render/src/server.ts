// Render service — minimal headless renderer implementing the Go
// RenderFetcher's POST /render contract
// (internal/intelligence/fetcher/render.go):
//
//   POST /render  {url, wait_selector?, timeout_ms?, a11y?, stealth?,
//                  solve?, proxy_group?, session_id?}
//   → 200 {html, a11y_text?, final_url, wall_detected}
//   → 400 bad request            (caller does not count the circuit)
//   → 500/503/504 render failure (feeds jc:intel:render:circuit)
//   GET /healthz → 200
//
// Anti-bot semantics (mirrors the Go contract):
//   - stealth / solve / proxy_group / session_id arrive only when the
//     channel's bypass_policy is "auto" — the service honors them as
//     given but never escalates on its own.
//   - solve:"auto" grants a bounded challenge-settle wait
//     (RENDER_SOLVE_WAIT_MS): non-interactive challenges (Cloudflare
//     managed, DataDome) often pass on their own. If the page is still a
//     wall afterward the response carries wall_detected:true — the
//     service never fabricates a pass, and real CAPTCHA solving needs an
//     external provider (not wired here).
//
// Env: RENDER_PORT (8300), RENDER_AUTH_TOKEN (Bearer, unset = open —
// internal network only), RENDER_MAX_CONCURRENCY (4), RENDER_MAX_SESSIONS
// (64), RENDER_SESSION_TTL_MS (900000), RENDER_SOLVE_WAIT_MS (12000),
// RENDER_PROXY_<GROUP> (proxy URL per group).
//
// Ref: docs/superpowers/specs/2026-09-20-intel-channels-curation-design.md
import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { BrowserPool } from "./browser.ts";
import { isPrivateLiteral, resolvesToPrivate } from "./guard.ts";
import { detectWall } from "./wall.ts";

const PORT = intEnv("RENDER_PORT", 8300);
const AUTH_TOKEN = process.env.RENDER_AUTH_TOKEN || "";
const MAX_CONCURRENCY = intEnv("RENDER_MAX_CONCURRENCY", 4);
const SESSION_TTL_MS = intEnv("RENDER_SESSION_TTL_MS", 15 * 60 * 1000);
const SOLVE_WAIT_MS = intEnv("RENDER_SOLVE_WAIT_MS", 12 * 1000);
const MAX_SESSIONS = intEnv("RENDER_MAX_SESSIONS", 64);
const MAX_TIMEOUT_MS = 120_000;
const MAX_BODY_BYTES = 64 * 1024;

interface RenderRequest {
  url: string;
  wait_selector?: string;
  timeout_ms?: number;
  a11y?: boolean;
  stealth?: boolean;
  solve?: string;
  proxy_group?: string;
  session_id?: string;
}

// Semaphore — render slots are bounded so a crawl burst cannot fork-bomb
// chromium contexts.
class Semaphore {
  private held = 0;
  private queue: (() => void)[] = [];

  constructor(private readonly max: number) {}

  // acquire resolves a release func, or null when the wait queue itself
  // is saturated — an unbounded queue would let a crawl burst pile
  // dead-on-arrival requests in memory.
  async acquire(): Promise<(() => void) | null> {
    if (this.held < this.max) {
      this.held++;
      return () => this.release();
    }

    if (this.queue.length >= this.max * 8) {
      return null;
    }

    return new Promise((resolve) => {
      this.queue.push(() => {
        this.held++;
        resolve(() => this.release());
      });
    });
  }

  private release(): void {
    this.held--;
    const next = this.queue.shift();
    if (next) next();
  }
}

const pool = new BrowserPool(SESSION_TTL_MS, MAX_SESSIONS);
const sem = new Semaphore(MAX_CONCURRENCY);

function intEnv(name: string, fallback: number): number {
  const v = parseInt(process.env[name] || "", 10);
  return Number.isFinite(v) && v > 0 ? v : fallback;
}

function json(res: ServerResponse, status: number, body: unknown): void {
  const data = JSON.stringify(body);
  res.writeHead(status, { "content-type": "application/json" });
  res.end(data);
}

// isPrivateLiteral lives in guard.ts — shared with the route interceptor
// that guards subresources/redirects inside the browser.

async function render(req: RenderRequest): Promise<{ status: number; body: unknown }> {
  let target: URL;
  try {
    target = new URL(req.url);
    if (target.protocol !== "http:" && target.protocol !== "https:") throw new Error("scheme");
    if (target.username || target.password) throw new Error("credentials");
  } catch {
    return { status: 400, body: { error: "url must be an http(s) URL without credentials" } };
  }

  if (isPrivateLiteral(target.hostname)) {
    return { status: 400, body: { error: "url must not target a private address" } };
  }

  // DNS layer: a public hostname resolving to a private address is
  // rejected before a render slot is spent (route interception applies
  // the same check to redirects/subresources).
  if (await resolvesToPrivate(target.hostname)) {
    return { status: 400, body: { error: "url must not target a private address" } };
  }

  const timeoutMs = Math.min(Math.max(req.timeout_ms || 30_000, 1_000), MAX_TIMEOUT_MS);
  const release = await sem.acquire();

  if (!release) {
    return { status: 503, body: { error: "render queue saturated" } };
  }

  try {
    const { ctx, owned } = await pool.context({
      stealth: req.stealth === true,
      proxyGroup: req.proxy_group || undefined,
      sessionId: req.session_id || undefined,
    });

    try {
      const page = await ctx.newPage();
      try {
        await page.goto(req.url, { waitUntil: "domcontentloaded", timeout: timeoutMs });

        if (req.wait_selector) {
          await page.waitForSelector(req.wait_selector, { timeout: timeoutMs }).catch(() => {});
        }

        // Let late-loading JS settle briefly — SPA shells paint after
        // domcontentloaded.
        await page.waitForTimeout(500).catch(() => {});

        let html = await page.content();
        let wall = detectWall(html);

        // solve:"auto" — bounded settle wait for non-interactive
        // challenges to pass on their own.
        if (wall && req.solve === "auto") {
          const deadline = Date.now() + SOLVE_WAIT_MS;
          while (Date.now() < deadline) {
            await page.waitForTimeout(1000);
            html = await page.content().catch(() => html);
            wall = detectWall(html);
            if (!wall) break;
          }
        }

        const out: Record<string, unknown> = {
          final_url: page.url(),
          wall_detected: wall !== null,
        };

        if (req.a11y) {
          out.a11y_text = await a11yText(page);
        } else {
          out.html = html;
        }

        return { status: 200, body: out };
      } finally {
        await page.close().catch(() => {});
      }
    } finally {
      if (owned) await ctx.close().catch(() => {});
    }
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    const timeout = /timeout/i.test(msg);
    return { status: timeout ? 504 : 500, body: { error: msg } };
  } finally {
    release();
  }
}

// a11yText serializes the accessibility tree when available, falling back
// to rendered innerText — the contract's a11y_text is a text extraction
// surface, not a strict AX audit.
async function a11yText(page: import("playwright-core").Page): Promise<string> {
  try {
    const snapshot = await page.accessibility.snapshot();
    if (snapshot) return serializeAx(snapshot).trim();
  } catch {
    // fall through to innerText
  }

  return page.locator("body").innerText().catch(() => "");
}

interface AxNode {
  role?: string;
  name?: string;
  value?: string | number;
  children?: AxNode[];
}

function serializeAx(node: AxNode, depth = 0): string {
  const parts: string[] = [];
  const label = [node.role, node.name].filter(Boolean).join(": ");
  if (label) parts.push("  ".repeat(depth) + label);
  if (node.value !== undefined) parts.push("  ".repeat(depth) + String(node.value));
  for (const c of node.children || []) parts.push(serializeAx(c, depth + 1));
  return parts.filter(Boolean).join("\n");
}

function readBody(req: IncomingMessage): Promise<string> {
  return new Promise((resolve, reject) => {
    let size = 0;
    const chunks: Buffer[] = [];
    req.on("data", (c: Buffer) => {
      size += c.length;
      if (size > MAX_BODY_BYTES) {
        reject(new Error("body too large"));
        req.destroy();
        return;
      }
      chunks.push(c);
    });
    req.on("end", () => resolve(Buffer.concat(chunks).toString("utf8")));
    req.on("error", reject);
  });
}

const server = createServer(async (req, res) => {
  try {
    if (req.method === "GET" && req.url === "/healthz") {
      json(res, 200, { ok: true });
      return;
    }

    if (AUTH_TOKEN && req.headers.authorization !== `Bearer ${AUTH_TOKEN}`) {
      json(res, 401, { error: "unauthorized" });
      return;
    }

    if (req.method === "POST" && req.url === "/render") {
      let parsed: RenderRequest;
      try {
        parsed = JSON.parse(await readBody(req));
      } catch {
        json(res, 400, { error: "invalid JSON body" });
        return;
      }

      if (!parsed || typeof parsed.url !== "string") {
        json(res, 400, { error: "url is required" });
        return;
      }

      const out = await render(parsed);
      json(res, out.status, out.body);
      return;
    }

    json(res, 404, { error: "not found" });
  } catch (err) {
    json(res, 500, { error: err instanceof Error ? err.message : "internal error" });
  }
});

async function main(): Promise<void> {
  await pool.start();
  server.listen(PORT, () => {
    console.log(`render service listening on :${PORT}`);
  });

  const shutdown = async () => {
    server.close();
    await pool.stop();
    process.exit(0);
  };

  process.on("SIGTERM", shutdown);
  process.on("SIGINT", shutdown);
}

main().catch((err) => {
  console.error("render service failed to start:", err);
  process.exit(1);
});
