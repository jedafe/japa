import { chromium } from 'playwright';
import { writeFileSync, mkdirSync } from 'fs';
import { join } from 'path';

const BASE = 'http://localhost:3001';
const ADMIN_EMAIL = 'admin@japaconnect.com';
const ADMIN_PASSWORD = 'Admin123!';
const SCREENSHOT_DIR = '/home/jmobile/websites/japa/japaconnect/test-results/admin-screenshots';

mkdirSync(SCREENSHOT_DIR, { recursive: true });

const ROUTES = [
  '/', '/users', '/users/roles', '/users/groups',
  '/content', '/moderation', '/moderation/content', '/moderation/reports',
  '/moderation/appeals', '/moderation/alerts', '/moderation-queue',
  '/analytics', '/settings', '/ai', '/ai/generate', '/ai/tools',
  '/blog', '/blog/review-queue', '/spaces', '/tags', '/organisations',
  '/universities', '/reactions', '/bots', '/tickets', '/ticket-types',
  '/broadcast', '/broadcasts', '/newsletter', '/spam',
  '/custom-fields', '/comparison-types', '/comparisons',
  '/professional-types', '/profiles', '/docs', '/jsonld-templates',
  '/page-builder', '/page-templates', '/translations', '/forms',
  '/media', '/reviews', '/reports', '/search', '/widgets',
  '/space-types', '/gdpr', '/deletions', '/notification-types',
  '/join-requests', '/learning', '/learn', '/mentorship',
];

(async () => {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await context.newPage();

  // Collect all console messages
  const allConsoleMessages = [];
  page.on('console', msg => {
    allConsoleMessages.push({ type: msg.type(), text: msg.text() });
  });
  page.on('pageerror', err => {
    allConsoleMessages.push({ type: 'pageerror', text: err.message });
  });

  // ─── LOGIN ──────────────────────────────────────────────────
  console.log('=== LOGGING IN ===');
  try {
    await page.goto(`${BASE}/login`, { waitUntil: 'networkidle', timeout: 15000 });
    await page.fill('input[name="email"]', ADMIN_EMAIL);
    await page.fill('input[name="password"]', ADMIN_PASSWORD);
    await page.click('button[type="submit"]');
    await page.waitForURL('**/');
    await page.waitForTimeout(2000);
    console.log(`Logged in successfully. URL: ${page.url()}`);
  } catch (e) {
    console.log(`Login failed: ${e.message}`);
    // Take screenshot of login page state
    await page.screenshot({ path: join(SCREENSHOT_DIR, 'login-failed.png') });
  }

  // ─── TEST EACH ROUTE ────────────────────────────────────────
  console.log('\n=== TESTING ALL ROUTES ===\n');

  const results = [];

  for (const route of ROUTES) {
    const routeErrors = [];
    const errorHandler = (msg) => {
      if (msg.type() === 'error') routeErrors.push(msg.text());
    };
    const pageErrorHandler = (err) => {
      routeErrors.push(`PAGE_ERROR: ${err.message}`);
    };

    page.on('console', errorHandler);
    page.on('pageerror', pageErrorHandler);

    let status = 'ok';
    let bodyText = '';
    let hasTable = false;
    let hasErrorText = false;
    let hasLoginForm = false;
    let screenshotPath = '';

    try {
      const resp = await page.goto(`${BASE}${route}`, { waitUntil: 'domcontentloaded', timeout: 12000 });
      await page.waitForTimeout(2500);

      status = resp?.status() ?? 'no-response';

      bodyText = await page.locator('body').innerText({ timeout: 5000 }).catch(() => '');

      hasTable = await page.locator('table').count() > 0;
      hasLoginForm = await page.locator('input[type="email"], input[type="password"]').count() > 0;
      hasErrorText = bodyText.toLowerCase().includes('something went wrong') ||
                     bodyText.toLowerCase().includes('internal server error') ||
                     bodyText.toLowerCase().includes('cannot read') ||
                     bodyText.toLowerCase().includes('undefined is not') ||
                     bodyText.toLowerCase().includes('failed to fetch');

      // Check if redirected to login (auth issue)
      const currentUrl = page.url();
      if (currentUrl.includes('/login') && route !== '/login') {
        status = 'redirected-to-login';
      }

      const filename = route === '/' ? 'dashboard' : route.replace(/\//g, '-').replace(/^-/, '');
      screenshotPath = join(SCREENSHOT_DIR, `${filename}.png`);
      await page.screenshot({ path: screenshotPath });

    } catch (e) {
      status = 'error';
      routeErrors.push(e.message);
    }

    page.removeListener('console', errorHandler);
    page.removeListener('pageerror', pageErrorHandler);

    const hasContent = bodyText.trim().length > 80 && !hasLoginForm;
    const isBlank = bodyText.trim().length < 30 && !hasLoginForm;
    const isError = hasErrorText || routeErrors.some(e => e.includes('PAGE_ERROR'));
    const isRedirected = status === 'redirected-to-login';

    const result = {
      route,
      status,
      hasContent,
      hasTable,
      isBlank,
      isError,
      isRedirected,
      errors: routeErrors,
      bodyPreview: bodyText.substring(0, 300)
    };
    results.push(result);

    let icon;
    if (isRedirected) icon = '🔒';
    else if (isError) icon = '❌';
    else if (isBlank) icon = '⚠️';
    else if (hasTable) icon = '📊';
    else icon = '✅';

    console.log(`${icon} ${route.padEnd(25)} status=${status} content=${hasContent} table=${hasTable} errors=${routeErrors.length}${isRedirected ? ' REDIRECTED' : ''}`);
  }

  // ─── SAVE REPORT ────────────────────────────────────────────
  writeFileSync(
    join(SCREENSHOT_DIR, '..', 'admin-auth-report.json'),
    JSON.stringify(results, null, 2)
  );

  // ─── PRINT SUMMARY ──────────────────────────────────────────
  console.log('\n=== SUMMARY ===');
  console.log(`Total routes: ${results.length}`);
  console.log(`OK with content: ${results.filter(r => r.hasContent && !r.isError && !r.isBlank && !r.isRedirected).length}`);
  console.log(`Has data table: ${results.filter(r => r.hasTable).length}`);
  console.log(`Blank: ${results.filter(r => r.isBlank).length}`);
  console.log(`Errors in page: ${results.filter(r => r.isError).length}`);
  console.log(`Redirected to login: ${results.filter(r => r.isRedirected).length}`);

  const problems = results.filter(r => r.isRedirected || r.isBlank || r.isError);
  if (problems.length > 0) {
    console.log('\n=== PROBLEMATIC ROUTES ===');
    for (const p of problems) {
      console.log(`\n  ${p.route}:`);
      if (p.isRedirected) console.log('    -> Redirected to login page (auth issue)');
      if (p.isBlank) console.log('    -> Blank page (no meaningful content)');
      if (p.isError) console.log('    -> Error detected on page');
      if (p.errors.length) {
        for (const e of p.errors.slice(0, 5)) {
          console.log(`    Console: ${e.substring(0, 200)}`);
        }
      }
    }
  }

  await browser.close();
  console.log('\nDone. Report: test-results/admin-auth-report.json');
  console.log(`Screenshots: ${SCREENSHOT_DIR}/`);
})();
