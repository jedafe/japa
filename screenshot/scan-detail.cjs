const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await context.newPage();

  const consoleErrors = [];
  const consoleWarnings = [];
  const networkResponses = [];

  page.on('console', msg => {
    if (msg.type() === 'error') consoleErrors.push(msg.text());
    if (msg.type() === 'warning') consoleWarnings.push(msg.text());
  });

  page.on('pageerror', err => consoleErrors.push(`PAGEERROR: ${err.message}`));

  page.on('response', async response => {
    const url = response.url();
    if (url.includes('/api/v1/')) {
      try {
        const body = await response.text();
        networkResponses.push({
          url: url.replace(/.*\/api\/v1/, ''),
          status: response.status(),
          bodyPreview: body.substring(0, 300)
        });
      } catch {}
    }
  });

  // Login
  await page.goto('http://localhost:3001/login', { waitUntil: 'networkidle' });
  await page.fill('input[type="email"]', 'admin@japaconnect.com');
  await page.fill('input[type="password"]', 'Admin123!');
  await page.click('button[type="submit"]');
  await page.waitForTimeout(3000);

  // Clear login-related responses
  networkResponses.length = 0;
  consoleErrors.length = 0;

  // Users List
  console.log('\n=== USERS LIST ===');
  await page.goto('http://localhost:3001/users', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);

  // Check what the API returned
  const usersResp = networkResponses.find(r => r.url.includes('/admin/users') && !r.url.includes('/admin/users/'));
  if (usersResp) {
    console.log('Users API status:', usersResp.status);
    console.log('Users API response:', usersResp.bodyPreview);
  } else {
    console.log('No /admin/users API call detected!');
  }

  // Check table structure
  const thead = await page.$('thead');
  const ths = await page.$$('thead th');
  console.log('Table thead exists:', !!thead);
  console.log('Table header cells:', ths.length);
  if (ths.length > 0) {
    for (const th of ths) {
      const text = await th.textContent();
      console.log('  TH:', text.trim());
    }
  }

  // Check tbody
  const tbody = await page.$('tbody');
  const trs = await page.$$('tbody tr');
  console.log('Table tbody exists:', !!tbody);
  console.log('Table rows:', trs.length);

  // Check first row cells
  if (trs.length > 0) {
    const tds = await page.$$('tbody tr:first-child td');
    console.log('First row cells:', tds.length);
    for (let i = 0; i < Math.min(tds.length, 7); i++) {
      const text = await tds[i].textContent();
      console.log(`  TD[${i}]:`, text.trim().substring(0, 50));
    }
  }

  // Check for FilterBar
  const filterBar = await page.$('[class*="filter"], [data-testid*="filter"], input[placeholder*="Search"], input[placeholder*="search"]');
  console.log('FilterBar/Search input:', !!filterBar);

  // Check for Pagination
  const pagination = await page.$('nav, [class*="pagination"], [class*="Pagination"], button:has-text("Next"), button:has-text("Previous")');
  console.log('Pagination element:', !!pagination);

  // Check for Create User button
  const createBtn = await page.$('button:has-text("Create"), button:has-text("Add")');
  console.log('Create button:', !!createBtn);

  // Check header section
  const pageHeader = await page.$('h1, h2, [class*="page-header"]');
  if (pageHeader) {
    const headerText = await pageHeader.textContent();
    console.log('Page header:', headerText.trim().substring(0, 50));
  }

  // Console errors
  if (consoleErrors.length > 0) {
    console.log('\nConsole errors:');
    consoleErrors.forEach(e => console.log(' -', e.substring(0, 200)));
  }

  // Navigate to user detail
  console.log('\n=== USER DETAIL ===');
  networkResponses.length = 0;
  consoleErrors.length = 0;

  // Click on first user's view action
  const viewBtns = await page.$$('button');
  let clickedView = false;
  for (const btn of viewBtns) {
    const text = await btn.textContent();
    if (text.trim() === '' || text.includes('View')) {
      // Check if it has an Eye icon - try clicking first action button in first row
    }
  }

  // Try clicking the first dropdown action button
  const firstRowBtns = await page.$$('tbody tr:first-child button');
  if (firstRowBtns.length > 0) {
    await firstRowBtns[0].click();
    await page.waitForTimeout(500);
    await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/09-dropdown-open.png', fullPage: true });

    // Look for View option in dropdown
    const viewOption = await page.$('a:has-text("View"), button:has-text("View"), [role="menuitem"]:has-text("View")');
    if (viewOption) {
      await viewOption.click();
      await page.waitForTimeout(3000);
    } else {
      // Try navigating directly
      console.log('No View option found, checking dropdown content...');
      const menuItems = await page.$$('[role="menuitem"], [role="menu"] button, [role="menu"] a');
      for (const item of menuItems) {
        const text = await item.textContent();
        console.log('  Menu item:', text.trim());
      }
    }
  }

  await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/10-user-detail-v2.png', fullPage: true });

  // Check detail page structure
  const url = page.url();
  console.log('Current URL:', url);

  const userCard = await page.$('[class*="card"], [class*="Card"]');
  console.log('Card element:', !!userCard);

  const avatarEl = await page.$('img[alt], [class*="avatar"], [class*="rounded-2xl"]');
  console.log('Avatar element:', !!avatarEl);

  const tabElements = await page.$$('[role="tablist"] button, [role="tab"]');
  console.log('Tab elements:', tabElements.length);

  const badgeElements = await page.$$('[class*="badge"], [class*="Badge"]');
  console.log('Badge elements:', badgeElements.length);

  if (consoleErrors.length > 0) {
    console.log('\nDetail page errors:');
    consoleErrors.forEach(e => console.log(' -', e.substring(0, 200)));
  }

  // Check API responses
  console.log('\nDetail page API responses:');
  networkResponses.forEach(r => {
    console.log(`  ${r.status} ${r.url}: ${r.bodyPreview.substring(0, 150)}`);
  });

  await browser.close();
  console.log('\n=== DONE ===');
})();
