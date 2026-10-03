import { chromium } from '@playwright/test';

const browser = await chromium.launch({ headless: true, channel: 'chrome' });
const context = await browser.newContext();
const page = await context.newPage();

async function activeTab() {
  return page.locator('[role="tab"][aria-selected="true"]').getAttribute('data-value');
}

// Test 1: deep link to hotels
await page.goto('http://localhost:3000/tools/travel?tab=hotels', { waitUntil: 'networkidle' });
console.log('deep link active tab:', await activeTab());

// Test 2: default tab
await page.goto('http://localhost:3000/tools/travel', { waitUntil: 'networkidle' });
console.log('default active tab:', await activeTab());

// Test 3: legacy providers tab must be gone — ?tab=providers falls back to flights
await page.goto('http://localhost:3000/tools/travel?tab=providers', { waitUntil: 'networkidle' });
console.log('legacy ?tab=providers lands on:', await activeTab());
console.log('providers tab present:', await page.locator('[role="tab"][data-value="providers"]').count());

// Test 4: click airlines and check URL
await page.locator('[role="tab"][data-value="airlines"]').click();
await page.waitForTimeout(300);
console.log('after airlines click active tab:', await activeTab());
console.log('after airlines click URL:', page.url());

// Test 5: click directory and check URL
await page.locator('[role="tab"][data-value="directory"]').click();
await page.waitForTimeout(300);
console.log('after directory click active tab:', await activeTab());
console.log('after directory click URL:', page.url());

await browser.close();
