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

  // Get jeff's user ID from the API
  const token = await page.evaluate(() => {
    // Access the token from the page context
    return document.cookie;
  });

  // Navigate directly to jeff's detail page
  // First, get the user list to find jeff's ID
  await page.goto('http://localhost:3001/users', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);

  // Find jeff's row and get the user ID from the link
  const jeffLink = await page.evaluate(() => {
    const rows = document.querySelectorAll('tbody tr');
    for (const row of rows) {
      const cells = row.querySelectorAll('td');
      for (const cell of cells) {
        if (cell.textContent.includes('jeff')) {
          // Find the link in this row's action cell
          const links = row.querySelectorAll('a[href*="/users/"]');
          for (const link of links) {
            if (link.href.includes('/users/') && !link.href.includes('/users/groups')) {
              return link.href;
            }
          }
        }
      }
    }
    return null;
  });

  console.log('Jeff link:', jeffLink);

  if (jeffLink) {
    errors.length = 0;
    await page.goto(jeffLink, { waitUntil: 'networkidle' });
    await page.waitForTimeout(3000);
    await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-08-jeff-detail.png', fullPage: true });

    console.log('URL:', page.url());
    console.log('Errors:', errors.length ? errors.join(' | ') : 'none');

    // Check content
    const body = await page.textContent('body');
    console.log('Has jeff:', body.includes('jeff'));
    console.log('Has Overview:', body.includes('Overview'));
    console.log('Has Activity:', body.includes('Activity'));
    console.log('Has AI Context:', body.includes('AI Context'));

    // Click Activity tab
    const actTab = page.locator('button', { hasText: 'Activity' });
    if (await actTab.count() > 0) {
      errors.length = 0;
      await actTab.click();
      await page.waitForTimeout(1500);
      await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-09-activity.png', fullPage: true });
      console.log('Activity tab errors:', errors.length || 'none');
    }

    // Click AI Context tab
    const aiTab = page.locator('button', { hasText: 'AI' });
    if (await aiTab.count() > 0) {
      errors.length = 0;
      await aiTab.click();
      await page.waitForTimeout(1500);
      await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-10-ai.png', fullPage: true });
      console.log('AI tab errors:', errors.length || 'none');
    }

    // Click Sessions tab
    const sessTab = page.locator('button', { hasText: 'Sessions' });
    if (await sessTab.count() > 0) {
      errors.length = 0;
      await sessTab.click();
      await page.waitForTimeout(1500);
      await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-11-sessions.png', fullPage: true });
      console.log('Sessions tab errors:', errors.length || 'none');
    }

    // Click Warnings tab
    const warnTab = page.locator('button', { hasText: 'Warnings' });
    if (await warnTab.count() > 0) {
      errors.length = 0;
      await warnTab.click();
      await page.waitForTimeout(1500);
      await page.screenshot({ path: '/home/jmobile/websites/japa/japaconnect/screenshot/final-12-warnings.png', fullPage: true });
      console.log('Warnings tab errors:', errors.length || 'none');
    }
  } else {
    console.log('Could not find jeff link');
  }

  await browser.close();
  console.log('\n=== DONE ===');
})();
