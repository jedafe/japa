const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await context.newPage();

  const errors = [];
  page.on('console', msg => { if (msg.type() === 'error') errors.push(msg.text()); });
  page.on('pageerror', err => errors.push(`PAGEERROR: ${err.message}`));
  page.on('response', r => { if (r.url().includes('/api/v1/') && r.status() >= 400) errors.push(`[${r.status()}] ${r.url().replace(/.*\/api\/v1/, '')}`); });

  // Login
  await page.goto('http://localhost:3001/login', { waitUntil: 'networkidle' });
  await page.fill('input[type="email"]', 'admin@japaconnect.com');
  await page.fill('input[type="password"]', 'Admin123!');
  await page.click('button[type="submit"]');
  await page.waitForTimeout(3000);

  // Navigate directly to jeff's detail page
  errors.length = 0;
  await page.goto('http://localhost:3001/users/553d2ec0-3049-4268-a242-f48e33c69c70', { waitUntil: 'networkidle' });
  await page.waitForTimeout(3000);
  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-15-jeff.png', fullPage: true });

  console.log('URL:', page.url());
  console.log('Errors:', errors.length ? errors.join(' | ') : 'none');

  const body = await page.textContent('body');
  console.log('Has "jeff":', body.includes('jeff'));
  console.log('Has "Overview":', body.includes('Overview'));
  console.log('Has "Warn":', body.includes('Warn'));
  console.log('Has "Ban":', body.includes('Ban'));

  // Click Activity tab
  errors.length = 0;
  const actTab = page.locator('button', { hasText: 'Activity' });
  if (await actTab.count() > 0) {
    await actTab.click();
    await page.waitForTimeout(1500);
    await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-16-activity.png', fullPage: true });
    console.log('\nActivity errors:', errors.length || 'none');
  }

  // Click AI Context tab
  errors.length = 0;
  const aiTab = page.locator('button', { hasText: 'AI' });
  if (await aiTab.count() > 0) {
    await aiTab.click();
    await page.waitForTimeout(1500);
    await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-17-ai.png', fullPage: true });
    console.log('AI errors:', errors.length || 'none');
  }

  // Click Warnings tab
  errors.length = 0;
  const warnTab = page.locator('button', { hasText: 'Warnings' });
  if (await warnTab.count() > 0) {
    await warnTab.click();
    await page.waitForTimeout(1500);
    await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-18-warnings.png', fullPage: true });
    console.log('Warnings errors:', errors.length || 'none');
  }

  // Test mobile view of detail page
  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto('http://localhost:3001/users/553d2ec0-3049-4268-a242-f48e33c69c70', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);
  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-19-mobile-detail.png', fullPage: true });

  await browser.close();
  console.log('\n=== DONE ===');
})();
