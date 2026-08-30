import { chromium, type ConsoleMessage, type Request } from '@playwright/test';

const BASE = 'http://localhost:3000';
const urls = [
  '/tools',
  '/tools/cv-formatter',
  '/tools/remittance',
  '/tools/embassy-alerter',
  '/tools/tax-calculator',
  '/tools/arrival-checklist',
  '/tools/credential-checker',
  '/tools/travel',
  '/tools/travel?tab=hotels',
];

const results: Record<string, { status: number; title: string; console: string[]; failed: Array<{ url: string; status: number }> }> = {};

const browser = await chromium.launch({ headless: true, channel: 'chrome' });
const context = await browser.newContext();
const page = await context.newPage();

const failed: Array<{ url: string; status: number }> = [];
const consoleLogs: string[] = [];

page.on('console', (msg: ConsoleMessage) => consoleLogs.push(`[${msg.type()}] ${msg.text()}`));
page.on('pageerror', (err) => consoleLogs.push(`[pageerror] ${err.message}`));
page.on('requestfinished', async (request: Request) => {
  const response = await request.response().catch(() => null);
  if (response && response.status() >= 400 && request.url().includes('/api/v1/')) {
    failed.push({ url: request.url(), status: response.status() });
  }
});

for (const path of urls) {
  consoleLogs.length = 0; failed.length = 0;
  const resp = await page.goto(`${BASE}${path}`, { waitUntil: 'networkidle', timeout: 30000 });
  const status = resp?.status() ?? 0;
  const title = await page.title();
  await page.waitForTimeout(500);
  results[path] = { status, title, console: [...consoleLogs], failed: [...failed] };
  console.log(`${path} -> ${status} "${title}"`);
}

await browser.close();
await Bun.write('/home/jmobile/websites/japa/japaconnect/.playwright-audit-tools.json', JSON.stringify(results, null, 2));
