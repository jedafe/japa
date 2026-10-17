import { chromium, type ConsoleMessage, type Page } from '@playwright/test';

const BASE = 'http://localhost:3005';
const API = 'http://localhost:8080/api/v1';
const OUT = '/tmp/tools-audit';

// --- login to get a jc_token cookie ---
const loginRes = await fetch(`${API}/auth/login`, {
	method: 'POST',
	headers: { 'content-type': 'application/json' },
	body: JSON.stringify({ email: 'admin@japa.to', password: 'Admin123!' })
});
const loginBody = await loginRes.json();
const token = loginBody?.data?.access_token as string;
console.log('login:', loginRes.status, token ? 'token ok' : 'NO TOKEN');

const browser = await chromium.launch({ headless: true, channel: 'chrome' });

interface PageResult {
	status: number;
	title: string;
	consoleErrors: string[];
	pageErrors: string[];
	failedApi: Array<{ url: string; status: number }>;
}

const results: Record<string, PageResult> = {};

async function newPage(auth: boolean) {
	const context = await browser.newContext();
	if (auth && token) {
		await context.addCookies([
			{ name: 'jc_token', value: token, domain: 'localhost', path: '/' }
		]);
	}
	const page = await context.newPage();
	const consoleErrors: string[] = [];
	const pageErrors: string[] = [];
	const failedApi: Array<{ url: string; status: number }> = [];
	page.on('console', (msg: ConsoleMessage) => {
		if (msg.type() === 'error' || msg.type() === 'warning') {
			consoleErrors.push(`[${msg.type()}] ${msg.text()}`);
		}
	});
	page.on('pageerror', (err) => pageErrors.push(err.message));
	page.on('response', (res) => {
		if (res.url().includes('/api/v1/') && res.status() >= 400) {
			failedApi.push({ url: res.url().replace(/^https?:\/\/[^/]+/, ''), status: res.status() });
		}
	});
	return { context, page, consoleErrors, pageErrors, failedApi };
}

async function visit(path: string, auth = false, after?: (page: Page) => Promise<void>) {
	const { context, page, consoleErrors, pageErrors, failedApi } = await newPage(auth);
	const key = `${auth ? 'auth:' : ''}${path}`;
	try {
		const resp = await page.goto(`${BASE}${path}`, { waitUntil: 'networkidle', timeout: 30000 });
		await page.waitForTimeout(700);
		if (after) await after(page);
		await page.waitForTimeout(400);
		await page.screenshot({ path: `${OUT}/${key.replace(/[^a-z0-9]+/gi, '_')}.png`, fullPage: false });
		results[key] = {
			status: resp?.status() ?? 0,
			title: await page.title(),
			consoleErrors: [...consoleErrors],
			pageErrors: [...pageErrors],
			failedApi: [...failedApi]
		};
		console.log(`${key} -> ${results[key].status} "${results[key].title}" errs:${consoleErrors.length + pageErrors.length} apiFails:${failedApi.length}`);
	} catch (e) {
		console.log(`${key} -> NAV FAIL: ${(e as Error).message.slice(0, 120)}`);
		results[key] = { status: 0, title: 'NAV FAIL', consoleErrors, pageErrors: [...pageErrors, String(e)], failedApi };
	}
	await context.close();
}

// pick the first option in a DestinationSelector autocomplete
async function pickFirstDestination(page: Page) {
	const input = page.locator('#dest-selector');
	if (await input.count()) {
		await input.click();
		await page.waitForTimeout(700);
		const opt = page.locator('.autocomplete-option').first();
		if (await opt.count()) {
			await opt.click();
			await page.waitForTimeout(1500);
		}
	}
}

// ── Hub ──
await visit('/');
// ── V1 tools ──
await visit('/visa-checklist');
await visit('/visa-checklist', false, async (page) => {
	// select origin + destination + visa type
	const inputs = page.locator('.autocomplete-input');
	if (await inputs.count()) {
		await inputs.nth(0).click();
		await page.waitForTimeout(600);
		await inputs.nth(0).fill('Nigeria');
		await page.waitForTimeout(800);
		const opt = page.locator('.autocomplete-option').first();
		if (await opt.count()) await opt.click();
		await page.waitForTimeout(500);
	}
});
await visit('/readiness');
await visit('/cost-of-living');
await visit('/scholarships');
await visit('/scholarships/e6c6a3fb-9491-48f8-af01-b8777663f3ef');
await visit('/scholarships/saved'); // anonymous -> redirect expected
await visit('/scholarships/saved', true);
await visit('/remittance');
await visit('/japabot'); // anonymous
await visit('/japabot', true);
// ── V2 tools ──
await visit('/visa-routes');
await visit('/visa-routes', false, pickFirstDestination);
await visit('/embassy-alerter');
await visit('/embassy-alerter', false, pickFirstDestination);
await visit('/embassy-alerter', true, pickFirstDestination);
await visit('/arrival-checklist');
await visit('/arrival-checklist', false, pickFirstDestination);
await visit('/processing-tracker');
await visit('/processing-tracker', false, pickFirstDestination);
await visit('/credential-checker');
await visit('/cv-formatter');
await visit('/housing-guide');
await visit('/housing-guide', false, pickFirstDestination);
await visit('/attestation-guide');
await visit('/attestation-guide', false, pickFirstDestination);
await visit('/language-test');
await visit('/language-test', false, pickFirstDestination);
await visit('/tax-calculator');
await visit('/community-locator');
await visit('/community-locator', false, pickFirstDestination);
// ── 404 + misc ──
await visit('/nonexistent-tool');
await visit('/fr/');

await browser.close();
await Bun.write(`${OUT}/results.json`, JSON.stringify(results, null, 2));
console.log('\nDONE — results at /tmp/tools-audit/results.json');
