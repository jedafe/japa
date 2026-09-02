import { chromium } from '@playwright/test';

const BASE = 'http://localhost:3000';

async function login(page: any) {
	await page.goto(`${BASE}/login`);
	const res = await page.evaluate(
		async ({ email, password }) => {
			const r = await fetch('/api/auth/login', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ email, password })
			});
			return { ok: r.ok, status: r.status, body: await r.json().catch(() => ({})) };
		},
		{ email: 'admin@japaconnect.com', password: 'Admin123!' }
	);
	if (!res.ok) throw new Error(`Login failed: ${res.status} ${JSON.stringify(res.body)}`);
}

async function main() {
	const browser = await chromium.launch({ headless: true });
	const context = await browser.newContext();
	const page = await context.newPage();

	const logs: { type: string; text: string }[] = [];
	const failed: { method: string; url: string; status: number }[] = [];

	page.on('console', (msg) => {
		if (msg.type() === 'error') logs.push({ type: msg.type(), text: msg.text() });
	});
	page.on('pageerror', (err) => logs.push({ type: 'pageerror', text: String(err) }));
	page.on('response', (res) => {
		if (res.status() >= 400) failed.push({ method: res.request().method(), url: res.url(), status: res.status() });
	});

	await login(page);

	// 1. Dashboard rail is API-driven and shows all expected entries.
	await page.goto(`${BASE}/dashboard`);
	await page.waitForLoadState('networkidle');
	await page.screenshot({ path: '/tmp/audit-dashboard-rail.png', fullPage: true });
	const railLinks = await page.locator('nav[aria-label*="Dashboard"] a[href^="/dashboard/"]').all();
	const labels = await Promise.all(railLinks.map(async (a) => a.textContent()));
	const expected = ['overview', 'analytics', 'content', 'relationships', 'learning', 'certificates', 'knowledge-base', 'tickets', 'bookmarks', 'media', 'spaces', 'organizations', 'profiles'];
	const missing = expected.filter((slug) => !labels.some((l) => (l ?? '').toLowerCase().includes(slug.replace(/-/g, ' '))));

	// 2. [view] component routes still render.
	await page.goto(`${BASE}/dashboard/analytics`);
	await page.waitForLoadState('networkidle');
	await page.screenshot({ path: '/tmp/audit-dashboard-analytics.png', fullPage: true });

	await page.goto(`${BASE}/dashboard/content`);
	await page.waitForLoadState('networkidle');
	await page.screenshot({ path: '/tmp/audit-dashboard-content.png', fullPage: true });

	// 3. Route-folder organization dashboard still works inside the new shell.
	await page.goto(`${BASE}/dashboard/organizations`);
	await page.waitForLoadState('networkidle');
	await page.screenshot({ path: '/tmp/audit-dashboard-organizations.png', fullPage: true });

	// Click first organization to verify detail + tabs.
	const firstOrg = page.locator('a[href^="/dashboard/organizations/"]').first();
	if (await firstOrg.isVisible().catch(() => false)) {
		await firstOrg.click();
		await page.waitForLoadState('networkidle');
		await page.screenshot({ path: '/tmp/audit-organization-detail.png', fullPage: true });

		// Media tab
		await page.goto(`${page.url().split('?')[0]}?tab=media`);
		await page.waitForLoadState('networkidle');
		await page.screenshot({ path: '/tmp/audit-organization-media.png', fullPage: true });

		// Analytics tab
		await page.goto(`${page.url().split('?')[0]}?tab=analytics&period=7d`);
		await page.waitForLoadState('networkidle');
		await page.screenshot({ path: '/tmp/audit-organization-analytics.png', fullPage: true });
	}

	// 4. Another route-folder page (media library) works.
	await page.goto(`${BASE}/dashboard/media`);
	await page.waitForLoadState('networkidle');
	await page.screenshot({ path: '/tmp/audit-dashboard-media.png', fullPage: true });

	await browser.close();

	console.log('Dashboard rail labels:', labels.filter(Boolean));
	console.log('Missing expected:', missing);
	console.log('Console errors:', logs.length);
	for (const l of logs) console.log(' ', l.type, l.text);
	console.log('Failed requests:', failed.length);
	for (const f of failed) console.log(' ', f.method, f.status, f.url);

	if (logs.length || failed.length || missing.length) process.exit(1);
	console.log('Dashboard rail verification passed');
}

main().catch((e) => {
	console.error(e);
	process.exit(1);
});
