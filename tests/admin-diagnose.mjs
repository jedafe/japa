import { chromium } from 'playwright';
import { writeFileSync } from 'fs';

const BASE = 'http://localhost:3001';

// All admin routes to test
const ROUTES = [
  '/',
  '/login',
  '/users',
  '/users/roles',
  '/users/groups',
  '/content',
  '/moderation',
  '/moderation/content',
  '/moderation/reports',
  '/moderation/appeals',
  '/moderation/alerts',
  '/moderation-queue',
  '/analytics',
  '/settings',
  '/ai',
  '/ai/generate',
  '/ai/tools',
  '/blog',
  '/blog/review-queue',
  '/spaces',
  '/tags',
  '/organisations',
  '/universities',
  '/reactions',
  '/bots',
  '/tickets',
  '/ticket-types',
  '/broadcast',
  '/broadcasts',
  '/newsletter',
  '/spam',
  '/custom-fields',
  '/comparison-types',
  '/comparisons',
  '/professional-types',
  '/profiles',
  '/docs',
  '/jsonld-templates',
  '/page-builder',
  '/page-templates',
  '/translations',
  '/forms',
  '/media',
  '/reviews',
  '/reports',
  '/search',
  '/widgets',
  '/space-types',
  '/gdpr',
  '/deletions',
  '/notification-types',
  '/join-requests',
  '/learning',
  '/learn',
  '/mentorship',
];

(async () => {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await context.newPage();
  
  const results = [];
  const consoleErrors = [];
  
  // Listen for console errors globally
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      consoleErrors.push({ route: 'pending', text: msg.text() });
    }
  });
  
  page.on('pageerror', (error) => {
    consoleErrors.push({ route: 'pending', text: `PAGE ERROR: ${error.message}` });
  });

  // First try to login
  console.log('--- Attempting login ---');
  try {
    await page.goto(`${BASE}/login`, { waitUntil: 'networkidle', timeout: 15000 });
    const loginContent = await page.content();
    console.log(`Login page loaded, length: ${loginContent.length}`);
    
    // Check if there's a login form
    const hasEmailField = await page.locator('input[type="email"], input[name="email"], input[placeholder*="email" i]').count();
    const hasPasswordField = await page.locator('input[type="password"]').count();
    console.log(`Login form: email=${hasEmailField}, password=${hasPasswordField}`);
    
    // Take screenshot
    await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/test-results/admin-login.png' });
    
    // Check URL after load
    const loginUrl = page.url();
    console.log(`Login URL: ${loginUrl}`);
  } catch (e) {
    console.log(`Login page error: ${e.message}`);
  }

  // Now test each route
  console.log('\n--- Testing all routes ---');
  
  for (const route of ROUTES) {
    const routeConsoleErrors = [];
    
    // Update console error tracking for this route
    const errorHandler = (msg) => {
      if (msg.type() === 'error') {
        routeConsoleErrors.push(msg.text());
      }
    };
    const pageErrorHandler = (error) => {
      routeConsoleErrors.push(`PAGE ERROR: ${error.message}`);
    };
    
    page.on('console', errorHandler);
    page.on('pageerror', pageErrorHandler);
    
    let status = 'unknown';
    let error = null;
    let hasContent = false;
    let hasTable = false;
    let hasError = false;
    let pageTitle = '';
    let bodyText = '';
    
    try {
      const response = await page.goto(`${BASE}${route}`, { waitUntil: 'domcontentloaded', timeout: 10000 });
      
      // Wait a bit for dynamic content
      await page.waitForTimeout(2000);
      
      status = response?.status() ?? 'no-response';
      
      // Check for error states
      const bodyTextRaw = await page.locator('body').innerText({ timeout: 5000 }).catch(() => '');
      bodyText = bodyTextRaw.substring(0, 500);
      
      // Check for common error indicators
      hasError = bodyText.toLowerCase().includes('error') || 
                  bodyText.toLowerCase().includes('something went wrong') ||
                  bodyText.toLowerCase().includes('not found') ||
                  bodyText.toLowerCase().includes('unauthorized') ||
                  bodyText.toLowerCase().includes('500') ||
                  bodyText.toLowerCase().includes('404');
      
      hasContent = bodyTextRaw.trim().length > 50;
      hasTable = await page.locator('table').count() > 0;
      pageTitle = await page.title();
      
      // Take screenshot
      const screenshotName = route === '/' ? 'admin-dashboard' : `admin${route.replace(/\//g, '-')}`;
      await page.screenshot({ path: `/home/jmobile/websites/japa/japaconnect/test-results/${screenshotName}.png` });
      
    } catch (e) {
      status = 'error';
      error = e.message;
    }
    
    // Cleanup listeners
    page.removeListener('console', errorHandler);
    page.removeListener('pageerror', pageErrorHandler);
    
    const result = {
      route,
      status,
      hasContent,
      hasTable,
      hasError,
      pageTitle,
      error,
      consoleErrors: routeConsoleErrors,
      bodyPreview: bodyText.substring(0, 200)
    };
    
    results.push(result);
    
    const icon = hasError ? '❌' : (hasContent ? '✅' : '⚠️');
    console.log(`${icon} ${route} [${status}] content=${hasContent} table=${hasTable} errors=${routeConsoleErrors.length}${error ? ' err=' + error.substring(0, 80) : ''}`);
  }
  
  // Write full report
  writeFileSync(
    '/home/jmobile/websites/japa/japaconnect/test-results/admin-diagnosis.json',
    JSON.stringify(results, null, 2)
  );
  
  // Print summary
  console.log('\n=== SUMMARY ===');
  console.log(`Total routes: ${results.length}`);
  console.log(`With content: ${results.filter(r => r.hasContent && !r.hasError).length}`);
  console.log(`With errors: ${results.filter(r => r.hasError).length}`);
  console.log(`Blank/empty: ${results.filter(r => !r.hasContent && !r.hasError).length}`);
  console.log(`Failed to load: ${results.filter(r => r.status === 'error').length}`);
  
  const problems = results.filter(r => r.hasError || !r.hasContent || r.status === 'error' || r.consoleErrors.length > 0);
  if (problems.length > 0) {
    console.log('\n=== PROBLEMS ===');
    for (const p of problems) {
      console.log(`\n${p.route}:`);
      console.log(`  Status: ${p.status}`);
      console.log(`  Error: ${p.error || 'none'}`);
      console.log(`  Console errors: ${p.consoleErrors.length}`);
      if (p.bodyPreview) console.log(`  Body preview: ${p.bodyPreview.substring(0, 150)}`);
      if (p.consoleErrors.length > 0) {
        for (const ce of p.consoleErrors.slice(0, 3)) {
          console.log(`  Console: ${ce.substring(0, 200)}`);
        }
      }
    }
  }
  
  await browser.close();
  console.log('\nDone. Full report: test-results/admin-diagnosis.json');
})();
