// Bot-wall signature scan — TypeScript port of the Go detector
// (internal/intelligence/fetcher/wall.go). Kept deliberately compact:
// the Go side re-scans every rendered body with the full detector, so a
// miss here only delays the flag until the response lands, never poisons
// a channel baseline.

const WALL_SIGNATURES: { name: string; allOf: string[] }[] = [
  { name: "cloudflare-challenge", allOf: ["_cf_chl"] },
  { name: "cloudflare-challenge", allOf: ["challenge-platform"] },
  { name: "cloudflare-wait", allOf: ["just a moment", "cloudflare"] },
  { name: "cloudflare-wait", allOf: ["checking your browser"] },
  { name: "cloudflare-block", allOf: ["attention required", "cloudflare"] },
  { name: "datadome-captcha", allOf: ["captcha-delivery.com"] },
  { name: "datadome", allOf: ["datadome", "captcha"] },
  { name: "perimeterx", allOf: ["px-captcha"] },
  { name: "perimeterx", allOf: ["_pxoncontextloaded"] },
  { name: "perimeterx-block", allOf: ["pardon our interruption"] },
  { name: "human-challenge", allOf: ["human challenge", "captcha"] },
  { name: "incapsula-denied", allOf: ["incapsula incident"] },
  { name: "incapsula-denied", allOf: ["request unsuccessful", "incapsula"] },
  { name: "akamai-sensor", allOf: ["akamai", "sensor"] },
  { name: "akamai-denied", allOf: ["access denied", "reference #"] },
  { name: "distil", allOf: ["distil", "captcha"] },
  { name: "sucuri", allOf: ["sucuri website firewall"] },
  { name: "sucuri", allOf: ["access denied - sucuri"] },
  { name: "aws-waf", allOf: ["awswaf"] },
  { name: "verify-human", allOf: ["verify you are human"] },
  { name: "verify-human", allOf: ["verify that you are human"] },
  { name: "are-you-robot", allOf: ["are you a robot", "captcha"] },
  { name: "ddos-guard", allOf: ["ddos protection by"] },
  { name: "bot-check", allOf: ["checking if you are a bot"] },
];

const WALL_SCAN_BYTES = 64 * 1024;

// detectWall scans the head of an HTML-ish body for challenge markers.
// Every listed substring must appear (multi-token requirements keep
// ambiguous words from false-flagging pages that merely mention them).
export function detectWall(body: string): string | null {
  if (!body) return null;

  const head = body.slice(0, WALL_SCAN_BYTES).toLowerCase();

  for (const sig of WALL_SIGNATURES) {
    if (sig.allOf.every((needle) => head.includes(needle))) {
      return sig.name;
    }
  }

  return null;
}
