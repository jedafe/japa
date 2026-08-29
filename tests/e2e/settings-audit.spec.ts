import { test, expect } from '@playwright/test';
import { writeFileSync } from 'node:fs';

/**
 * Temporary runtime audit for /settings and /settings/:section pages.
 * Logs in with the super admin account, visits every settings section,
 * and records console errors and API 4xx/5xx responses.
 */

const SECTIONS = [
  'profile',
  'account',
  'privacy',
  'language',
  'location',
  'mentorship',
  'data-privacy',
  'security',
  'sessions',
  'connected',
  'devices',
  'notifications',
  'newsletter',
  'api-keys',
  'affiliate',
  'wallet',
  'subscription',
  'payments',
  'danger',
  'ai-privacy'
];

const OUTPUT = '/tmp/settings-runtime-audit.json';

interface SectionResult {
  slug: string;
  url: string;
  finalUrl: string;
  title: string;
  consoleErrors: { type: string; text: string; location?: string }[];
  apiErrors: { url: string; method: string; status: number }[];
  apiCalls: { url: string; method: string; status: number }[];
}

test('Settings pages runtime audit', async ({ page }) => {
  const results: SectionResult[] = [];

  // Login
  await page.goto('/login');
  await page.locator('#login-email').waitFor({ state: 'visible', timeout: 10000 });
  await page.locator('#login-email').fill('admin@japaconnect.com');
  await page.locator('#login-password').fill('Admin123!');

  const [loginRes] = await Promise.all([
    page.waitForResponse((res) => res.url().includes('/auth/login') && res.request().method() === 'POST', { timeout: 10000 }),
    page.locator('button[type="submit"]').click()
  ]);

  expect(loginRes.status()).toBe(200);

  // Wait for post-login navigation
  await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 15000 });
  const afterLogin = page.url();
  if (afterLogin.includes('/login')) {
    throw new Error('Login failed or stayed on /login');
  }

  for (const slug of SECTIONS) {
    const consoleErrors: SectionResult['consoleErrors'] = [];
    const apiErrors: SectionResult['apiErrors'] = [];
    const apiCalls: SectionResult['apiCalls'] = [];

    const handler = (msg: any) => {
      if (msg.type() === 'error' || msg.type() === 'warning') {
        consoleErrors.push({
          type: msg.type(),
          text: msg.text(),
          location: msg.location().url
        });
      }
    };
    const resHandler = async (res: any) => {
      const url = res.url();
      if (url.includes('/api/') || url.includes('localhost:8080')) {
        const entry = {
          url,
          method: res.request().method(),
          status: res.status()
        };
        apiCalls.push(entry);
        if (res.status() >= 400) {
          apiErrors.push(entry);
        }
      }
    };

    page.on('console', handler);
    page.on('response', resHandler);

    await page.goto(`/settings/${slug}`, { waitUntil: 'networkidle', timeout: 20000 });
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(1500);

    page.off('console', handler);
    page.off('response', resHandler);

    results.push({
      slug,
      url: `/settings/${slug}`,
      finalUrl: page.url(),
      title: await page.title(),
      consoleErrors,
      apiErrors,
      apiCalls
    });
  }

  writeFileSync(OUTPUT, JSON.stringify(results, null, 2));
});
