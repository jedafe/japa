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

  // 1. Users list
  errors.length = 0;
  await page.goto('http://localhost:3001/users', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);
  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-01-users.png', fullPage: true });
  console.log('=== USERS LIST ===');
  console.log('Errors:', errors.length || 'none');

  // 2. User detail - click jeff
  errors.length = 0;
  const jeffRow = page.locator('tbody tr', { hasText: 'jeff' }).first();
  await jeffRow.locator('button').first().click();
  await page.waitForTimeout(500);
  const viewLink = page.locator('a[href*="/users/"]').first();
  if (await viewLink.count() > 0) {
    await viewLink.click();
    await page.waitForTimeout(3000);
  }
  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-02-user-detail.png', fullPage: true });
  console.log('\n=== USER DETAIL ===');
  console.log('URL:', page.url());
  console.log('Errors:', errors.length ? errors.join(' | ') : 'none');

  // Click Activity tab
  const activityTab = page.locator('button', { hasText: 'Activity' });
  if (await activityTab.count() > 0) {
    await activityTab.click();
    await page.waitForTimeout(1500);
    await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-03-activity.png', fullPage: true });
  }

  // Click AI Context tab
  const aiTab = page.locator('button', { hasText: 'AI' });
  if (await aiTab.count() > 0) {
    await aiTab.click();
    await page.waitForTimeout(1500);
    await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-04-ai.png', fullPage: true });
  }

  // 3. Groups
  errors.length = 0;
  await page.goto('http://localhost:3001/users/groups', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);
  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-05-groups.png', fullPage: true });
  const groupRows = await page.$$('tbody tr');
  console.log('\n=== GROUPS ===');
  console.log('Rows:', groupRows.length);
  console.log('Errors:', errors.length || 'none');

  // 4. Roles
  errors.length = 0;
  await page.goto('http://localhost:3001/users/roles', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);
  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-06-roles.png', fullPage: true });
  const roleRows = await page.$$('tbody tr');
  console.log('\n=== ROLES ===');
  console.log('Rows:', roleRows.length);
  console.log('Errors:', errors.length || 'none');

  // 5. Mobile
  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto('http://localhost:3001/users', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);
  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-07-mobile.png', fullPage: true });

  await browser.close();
  console.log('\n=== DONE ===');
})();
