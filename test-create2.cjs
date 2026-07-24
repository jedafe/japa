const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({
    executablePath: '/usr/bin/google-chrome',
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox']
  });

  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await context.newPage();

  // Capture ALL network requests
  const apiCalls = [];
  page.on('request', (request) => {
    if (request.url().includes('/api/v1/')) {
      apiCalls.push({
        method: request.method(),
        url: request.url(),
        headers: request.headers()
      });
    }
  });
  page.on('response', async (response) => {
    if (response.url().includes('/api/v1/')) {
      const body = await response.text().catch(() => '');
      console.log('Response:', response.status(), response.url(), body.substring(0, 200));
    }
  });

  // Login
  await page.goto('http://localhost:3001/login', { waitUntil: 'networkidle', timeout: 30000 });
  await page.waitForTimeout(1500);
  await page.locator('input[type="email"]').first().fill('admin@japaconnect.com');
  await page.locator('input[type="password"]').first().fill('Admin123!');
  await page.locator('button[type="submit"]').first().click();
  try { await page.waitForURL(function(u) { return !u.toString().includes('/login'); }, { timeout: 15000 }); } catch(e) {}
  await page.waitForTimeout(2000);

  // Go to create page
  console.log('=== Testing Create Post ===');
  await page.goto('http://localhost:3001/blog/new', { waitUntil: 'networkidle', timeout: 30000 });
  await page.waitForTimeout(2000);

  // Fill form
  await page.locator('#post-title').fill('Test Blog Post 123');
  await page.waitForTimeout(300);

  // Click Create Post button
  console.log('\nClicking Create Post...');
  await page.locator('button[type="submit"]').click();
  await page.waitForTimeout(5000);

  // Check current URL
  console.log('\nCurrent URL:', page.url());

  await browser.close();
  console.log('=== Done ===');
})();
