import { chromium } from '@playwright/test';

const browser = await chromium.launch({ headless: true, channel: 'chrome' });
const context = await browser.newContext();
const page = await context.newPage();

async function tabIs(expected: string) {
  const value = await page.locator('[role="tab"][aria-selected="true"]').getAttribute('data-value');
  return value === expected;
}

async function activeTab() {
  return page.locator('[role="tab"][aria-selected="true"]').getAttribute('data-value');
}

// Test 1: deep link to hotels
await page.goto('http://localhost:3000/tools/travel?tab=hotels', { waitUntil: 'networkidle' });
console.log('deep link active tab:', await activeTab());

// Test 2: default tab
await page.goto('http://localhost:3000/tools/travel', { waitUntil: 'networkidle' });
console.log('default active tab:', await activeTab());

// Test 3: click providers and check URL
await page.locator('[role="tab"][data-value="providers"]').click();
await page.waitForTimeout(300);
console.log('after providers click active tab:', await activeTab());
console.log('after providers click URL:', page.url());

await browser.close();
