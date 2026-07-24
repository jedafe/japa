const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.launch({
    executablePath: '/usr/bin/google-chrome',
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox']
  });

  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await context.newPage();

  // Capture network requests
  const apiCalls = [];
  page.on('response', async (response) => {
    if (response.url().includes('/api/v1/blog') && response.request().method() === 'POST') {
      const body = await response.text().catch(() => '');
      apiCalls.push({
        url: response.url(),
        status: response.status(),
        body: body.substring(0, 500)
      });
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
  console.log('Filling form...');
  await page.locator('#post-title').fill('Test Blog Post');
  await page.waitForTimeout(300);
  const slug = await page.locator('#post-slug').inputValue();
  console.log('Slug:', slug);

  // Click Create Post button
  console.log('Clicking Create Post...');
  await page.locator('button[type="submit"]').click();
  await page.waitForTimeout(5000);

  // Check API calls
  console.log('\nAPI Calls:');
  for (const call of apiCalls) {
    console.log('  Status:', call.status);
    console.log('  Body:', call.body);
  }

  // Check current URL
  console.log('\nCurrent URL:', page.url());
  
  // Check for error messages
  const errorMsg = await page.locator('[style*="color-danger"]').textContent().catch(() => '');
  console.log('Error message:', errorMsg?.trim());

  // Check page content
  const bodyText = (await page.locator('body').textContent()) || '';
  console.log('Has "Creating":', bodyText.includes('Creating'));
  console.log('Has "Failed":', bodyText.includes('Failed'));

  await page.screenshot({ path: '/tmp/test-create.png', fullPage: true });

  await browser.close();
  console.log('\n=== Done ===');
})();
