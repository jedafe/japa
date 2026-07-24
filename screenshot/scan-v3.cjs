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

  const rows = await page.$$('tbody tr');
  console.log('Rows:', rows.length);
  console.log('Console errors:', consoleErrors.length);
  console.log('Network errors:', networkErrors.length);

  // Now navigate to user detail for "jeff" (active user, row 4)
  // We need to find jeff's row and click its View button
  consoleErrors.length = 0;
  networkErrors.length = 0;

  // Find the link to jeff by finding the row with "jeff" text
  const jeffRow = await page.locator('tbody tr', { hasText: 'jeff' }).first();
  if (await jeffRow.count() > 0) {
    console.log('Found jeff row');
    // Click the first button in that row to open dropdown
    const btn = jeffRow.locator('button').first();
    await btn.click();
    await page.waitForTimeout(500);
    await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/12-jeff-dropdown.png', fullPage: true });

    // Click "View" option
    const viewOption = page.locator('[role="menuitem"]').first();
    if (await viewOption.count() > 0) {
      await viewOption.click();
      await page.waitForTimeout(3000);
    } else {
      console.log('No menuitem found, checking for links...');
      const links = await page.$$('a[href*="/users/"]');
      for (const l of links) {
        const href = await l.getAttribute('href');
        console.log('  Link:', href);
      }
    }
  } else {
    console.log('Jeff row not found');
  }

  console.log('URL:', page.url());
  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/13-user-detail.png', fullPage: true });

  // Check detail page
  console.log('\n=== USER DETAIL CHECK ===');
  const bodyText = await page.textContent('body');
  console.log('Has "jeff":', bodyText.includes('jeff'));
  console.log('Has "@":', bodyText.includes('@'));
  console.log('Has "Overview":', bodyText.includes('Overview'));
  console.log('Has "Activity":', bodyText.includes('Activity'));

  // Check for avatar
  const avatarImg = await page.$('img[alt]');
  const avatarLetter = await page.$('[class*="rounded-2xl"][class*="h-24"]');
  console.log('Avatar image:', !!avatarImg);
  console.log('Avatar letter:', !!avatarLetter);

  // Check for badges
  const badges = await page.$$('[class*="rounded-full"][class*="px-2"]');
  console.log('Badge elements:', badges.length);

  // Check for tabs
  const tabBtns = await page.$$('[role="tablist"] button, [role="tab"]');
  console.log('Tab buttons:', tabBtns.length);

  // Check for action buttons
  const warnBtn = await page.$('button:has-text("Warn")');
  const banBtn = await page.$('button:has-text("Ban")');
  const deleteBtn = await page.$('button:has-text("Delete")');
  console.log('Warn button:', !!warnBtn);
  console.log('Ban button:', !!banBtn);
  console.log('Delete button:', !!deleteBtn);

  // Check for cards
  const cards = await page.$$('[class*="border"][class*="rounded"]');
  console.log('Card-like elements:', cards.length);

  if (consoleErrors.length > 0) {
    console.log('\nConsole errors:');
    consoleErrors.forEach(e => console.log(' -', e.substring(0, 200)));
  }
  if (networkErrors.length > 0) {
    console.log('\nNetwork errors:');
    networkErrors.forEach(e => console.log(' -', e));
  }

  // Groups page
  console.log('\n=== GROUPS PAGE ===');
  consoleErrors.length = 0;
  networkErrors.length = 0;
  await page.goto('http://localhost:3001/users/groups', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);
  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/14-groups.png', fullPage: true });
  const groupRows = await page.$$('tbody tr');
  console.log('Group rows:', groupRows.length);
  console.log('Console errors:', consoleErrors.length);
  console.log('Network errors:', networkErrors.length);

  // Roles page
  console.log('\n=== ROLES PAGE ===');
  consoleErrors.length = 0;
  networkErrors.length = 0;
  await page.goto('http://localhost:3001/users/roles', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);
  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/15-roles.png', fullPage: true });
  const roleRows = await page.$$('tbody tr');
  console.log('Role rows:', roleRows.length);
  console.log('Console errors:', consoleErrors.length);
  console.log('Network errors:', networkErrors.length);

  await browser.close();
  console.log('\n=== DONE ===');
})();
