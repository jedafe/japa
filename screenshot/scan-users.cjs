const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await context.newPage();

  const consoleErrors = [];
  const networkErrors = [];

  page.on('console', msg => {
    if (msg.type() === 'error') {
      consoleErrors.push(`[console.error] ${msg.text()}`);
    }
  });

  page.on('pageerror', err => {
    consoleErrors.push(`[pageerror] ${err.message}`);
  });

  page.on('response', response => {
    const url = response.url();
    if (url.includes('/api/v1/') && response.status() >= 400) {
      networkErrors.push(`[${response.status()}] ${url}`);
    }
  });

  // Login
  console.log('\n=== LOGGING IN ===');
  await page.goto('http://localhost:3001/login', { waitUntil: 'networkidle' });
  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/01-login.png', fullPage: true });

  await page.fill('input[type="email"]', 'admin@japaconnect.com');
  await page.fill('input[type="password"]', 'Admin123!');
  await page.click('button[type="submit"]');
  await page.waitForTimeout(3000);
  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/02-after-login.png', fullPage: true });

  // Users List Page
  console.log('\n=== /users LIST PAGE ===');
  consoleErrors.length = 0;
  networkErrors.length = 0;
  await page.goto('http://localhost:3001/users', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);
  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/03-users-list.png', fullPage: true });

  console.log('Console errors:', consoleErrors.length ? consoleErrors.join('\n') : 'none');
  console.log('Network errors:', networkErrors.length ? networkErrors.join('\n') : 'none');

  // Check if table has data
  const rows = await page.$$('table tbody tr');
  console.log('Table rows found:', rows.length);

  // Check if action buttons exist
  const actionBtns = await page.$$('table tbody tr:first-child button');
  console.log('Action buttons in first row:', actionBtns.length);

  // Check filters
  const filters = await page.$$('[data-testid="filter-bar"], .filter-bar, select');
  console.log('Filter elements:', filters.length);

  // Check pagination
  const pagination = await page.$('[data-testid="pagination"], nav[aria-label*="pagination"], .pagination');
  console.log('Pagination found:', !!pagination);

  // Click first row's action button (dropdown)
  if (rows.length > 0) {
    const firstRowActions = await page.$$('table tbody tr:first-child button, table tbody tr:first-child [data-testid="action-cell"] button');
    console.log('First row action buttons:', firstRowActions.length);
    if (firstRowActions.length > 0) {
      await firstRowActions[0].click();
      await page.waitForTimeout(500);
      await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/04-users-list-actions-open.png', fullPage: true });
    }
  }

  // Click View on first user to go to detail page
  const viewLinks = await page.$$('a[href*="/users/"]');
  console.log('User links found:', viewLinks.length);

  // Navigate to first user detail
  if (rows.length > 0) {
    // Find a user ID from the first row
    const firstRowLink = await page.$('table tbody tr:first-child a[href*="/users/"]');
    if (firstRowLink) {
      const href = await firstRowLink.getAttribute('href');
      console.log('First user link:', href);
    }
  }

  // User Detail Page - find a real user
  console.log('\n=== /users/[userId] DETAIL PAGE ===');
  consoleErrors.length = 0;
  networkErrors.length = 0;

  // Get the first user's link from the table
  const userLink = await page.$('table tbody tr td a[href*="/users/"]');
  if (userLink) {
    await userLink.click();
    await page.waitForTimeout(3000);
  } else {
    // Try clicking View button
    const viewBtn = await page.$('table tbody tr:first-child button:has-text("View")');
    if (viewBtn) {
      await viewBtn.click();
      await page.waitForTimeout(3000);
    } else {
      // Navigate manually - try to find user ID from page
      console.log('No user links found, trying manual navigation...');
      await page.goto('http://localhost:3001/users', { waitUntil: 'networkidle' });
      await page.waitForTimeout(1000);
    }
  }

  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/05-user-detail.png', fullPage: true });
  console.log('Console errors:', consoleErrors.length ? consoleErrors.join('\n') : 'none');
  console.log('Network errors:', networkErrors.length ? networkErrors.join('\n') : 'none');

  // Check user card
  const avatar = await page.$('img[alt], .avatar, [class*="rounded-2xl"]');
  console.log('Avatar found:', !!avatar);

  // Check tabs
  const tabs = await page.$$('[role="tab"], button:has-text("Overview"), button:has-text("Activity")');
  console.log('Tab buttons found:', tabs.length);

  // Check action buttons
  const detailActions = await page.$$('button:has-text("Ban"), button:has-text("Warn"), button:has-text("Delete"), button:has-text("Suspend")');
  console.log('Action buttons on detail page:', detailActions.length);

  // Groups Page
  console.log('\n=== /users/groups PAGE ===');
  consoleErrors.length = 0;
  networkErrors.length = 0;
  await page.goto('http://localhost:3001/users/groups', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);
  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/06-groups.png', fullPage: true });
  console.log('Console errors:', consoleErrors.length ? consoleErrors.join('\n') : 'none');
  console.log('Network errors:', networkErrors.length ? networkErrors.join('\n') : 'none');

  const groupRows = await page.$$('table tbody tr');
  console.log('Group rows:', groupRows.length);

  // Roles Page
  console.log('\n=== /users/roles PAGE ===');
  consoleErrors.length = 0;
  networkErrors.length = 0;
  await page.goto('http://localhost:3001/users/roles', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);
  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/07-roles.png', fullPage: true });
  console.log('Console errors:', consoleErrors.length ? consoleErrors.join('\n') : 'none');
  console.log('Network errors:', networkErrors.length ? networkErrors.join('\n') : 'none');

  const roleRows = await page.$$('table tbody tr');
  console.log('Role rows:', roleRows.length);

  // Mobile view
  console.log('\n=== MOBILE VIEW (375x812) ===');
  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto('http://localhost:3001/users', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);
  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/08-users-mobile.png', fullPage: true });

  await browser.close();
  console.log('\n=== SCAN COMPLETE ===');
})();
