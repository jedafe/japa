const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await context.newPage();

  const errors = [];
  page.on('console', msg => { if (msg.type() === 'error') errors.push(msg.text()); });
  page.on('response', r => { if (r.url().includes('/api/v1/') && r.status() >= 400) errors.push(`[${r.status()}] ${r.url().replace(/.*\/api\/v1/, '')}`); });

  // Login
  await page.goto('http://localhost:3001/login', { waitUntil: 'networkidle' });
  await page.fill('input[type="email"]', 'admin@japaconnect.com');
  await page.fill('input[type="password"]', 'Admin123!');
  await page.click('button[type="submit"]');
  await page.waitForTimeout(3000);

  // Users list
  await page.goto('http://localhost:3001/users', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);

  // Find jeff's row using locator
  const jeffRow = page.locator('tr', { hasText: 'jeff' }).first();
  console.log('Jeff row found:', await jeffRow.count() > 0);

  if (await jeffRow.count() > 0) {
    // Click the action dropdown button (last button in the row)
    const actionBtn = jeffRow.locator('button').last();
    await actionBtn.click();
    await page.waitForTimeout(800);

    // Take screenshot of the open dropdown
    await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-13-dropdown.png', fullPage: true });

    // Look for all visible menu items
    const menuItems = await page.$$('[role="menuitem"], [data-radix-collection-item], [data-state="open"] a, [data-state="open"] button');
    console.log('Menu items found:', menuItems.length);
    for (const item of menuItems) {
      const text = await item.textContent();
      const tag = await item.evaluate(el => el.tagName);
      const href = await item.getAttribute('href');
      console.log(`  ${tag}: "${text.trim()}" href="${href}"`);
    }

    // Try clicking the View option
    const viewOption = page.locator('[role="menuitem"]', { hasText: 'View' }).first();
    if (await viewOption.count() > 0) {
      errors.length = 0;
      await viewOption.click();
      await page.waitForTimeout(3000);
      console.log('\nAfter View click:');
      console.log('URL:', page.url());
      console.log('Errors:', errors.length ? errors.join(' | ') : 'none');
      await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-14-detail.png', fullPage: true });

      // Check detail page content
      const body = await page.textContent('body');
      console.log('Has jeff:', body.includes('jeff'));
      console.log('Has Overview:', body.includes('Overview'));
      console.log('Has tabs:', body.includes('Activity'));
    } else {
      console.log('No View menuitem found');
      // Try any visible link/button with "View"
      const viewBtn = page.getByText('View', { exact: true }).first();
      if (await viewBtn.count() > 0) {
        await viewBtn.click();
        await page.waitForTimeout(3000);
        console.log('URL after View btn click:', page.url());
        await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-14-detail.png', fullPage: true });
      }
    }
  }

  await browser.close();
  console.log('\n=== DONE ===');
})();
