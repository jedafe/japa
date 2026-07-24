const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await context.newPage();

  const consoleErrors = [];
  const networkErrors = [];

  page.on('console', msg => {
    if (msg.type() === 'error') consoleErrors.push(msg.text());
  });
  page.on('pageerror', err => consoleErrors.push(`PAGEERROR: ${err.message}`));
  page.on('response', response => {
    if (response.url().includes('/api/v1/') && response.status() >= 400) {
      networkErrors.push(`[${response.status()}] ${response.url().replace(/.*\/api\/v1/, '')}`);
    }
  });

  // Login
  await page.goto('http://localhost:3001/login', { waitUntil: 'networkidle' });
  await page.fill('input[type="email"]', 'admin@japaconnect.com');
  await page.fill('input[type="password"]', 'Admin123!');
  await page.click('button[type="submit"]');
  await page.waitForTimeout(3000);

  // Users List
  console.log('\n=== USERS LIST ===');
  consoleErrors.length = 0;
  networkErrors.length = 0;
  await page.goto('http://localhost:3001/users', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);
  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/11-users-list-fixed.png', fullPage: true });

  // Find a non-deleted user to test detail page
  const rows = await page.$$('tbody tr');
  console.log('Rows:', rows.length);

  // Get all user statuses from the table
  for (let i = 0; i < Math.min(rows.length, 5); i++) {
    const cells = await rows[i].$$('td');
    if (cells.length >= 5) {
      const username = await cells[1].textContent();
      const status = await cells[4].textContent();
      console.log(`  Row ${i}: ${username.trim()} - ${status.trim()}`);
    }
  }

  // Click the first row's action button to open dropdown
  consoleErrors.length = 0;
  networkErrors.length = 0;

  // Find and click View for a non-deleted user
  let navigated = false;
  for (let i = 0; i < Math.min(rows.length, 10); i++) {
    const cells = await rows[i].$$('td');
    if (cells.length >= 5) {
      const status = (await cells[4].textContent()).trim();
      if (status !== 'deleted') {
        // Click the action button in this row
        const actionBtns = await rows[i].$$('button');
        if (actionBtns.length > 0) {
          await actionBtns[0].click();
          await page.waitForTimeout(500);

          // Look for View in the dropdown
          const viewLink = await page.$('[role="menuitem"]:first-child, [data-radix-menu-content] button:first-child, [data-radix-menu-content] a:first-child');
          if (viewLink) {
            await viewLink.click();
            await page.waitForTimeout(3000);
            navigated = true;
            break;
          } else {
            // Close dropdown and try next row
            await page.keyboard.press('Escape');
            await page.waitForTimeout(200);
          }
        }
      }
    }
  }

  if (!navigated) {
    console.log('Could not navigate to user detail via dropdown, trying direct URL...');
    // Get first user ID from API response
    await page.goto('http://localhost:3001/users', { waitUntil: 'networkidle' });
    await page.waitForTimeout(1000);
  }

  const currentUrl = page.url();
  console.log('Current URL:', currentUrl);

  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/12-user-detail-fixed.png', fullPage: true });

  // Check detail page elements
  const avatar = await page.$('img[alt]');
  const badges = await page.$$('[class*="inline-block"][class*="rounded-full"], [class*="badge"]');
  const tabs = await page.$$('[role="tablist"] button, [role="tab"]');
  const cards = await page.$$('[class*="card"], [class*="Card"]');

  console.log('Avatar:', !!avatar);
  console.log('Badges:', badges.length);
  console.log('Tabs:', tabs.length);
  console.log('Cards:', cards.length);

  if (consoleErrors.length > 0) {
    console.log('\nConsole errors:');
    consoleErrors.forEach(e => console.log(' -', e.substring(0, 200)));
  }
  if (networkErrors.length > 0) {
    console.log('\nNetwork errors:');
    networkErrors.forEach(e => console.log(' -', e));
  }

  // Check page content
  const pageText = await page.textContent('body');
  const hasUsername = pageText.includes('admin') || pageText.includes('test');
  const hasEmail = pageText.includes('@');
  const hasTabs = pageText.includes('Overview') || pageText.includes('Activity');
  console.log('\nPage has username:', hasUsername);
  console.log('Page has email:', hasEmail);
  console.log('Page has tabs:', hasTabs);

  await browser.close();
  console.log('\n=== DONE ===');
})();
