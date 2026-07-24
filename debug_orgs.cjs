const { chromium } = require('playwright');

(async () => {
    const browser = await chromium.launch({ headless: true });
    const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
    
    // Capture network requests
    const requests = [];
    page.on('request', req => {
        if (req.url().includes('/api/')) {
            requests.push({ url: req.url(), method: req.method() });
        }
    });
    
    const responses = [];
    page.on('response', res => {
        if (res.url().includes('/api/')) {
            responses.push({ url: res.url(), status: res.status() });
        }
    });
    
    const consoleMessages = [];
    page.on('console', msg => {
        consoleMessages.push({ type: msg.type(), text: msg.text() });
    });
    
    // Login
    await page.goto('http://localhost:3001/login');
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(1000);
    
    await page.locator("input[type='email'], input[name='email'], input[placeholder*='email' i]").first().fill('admin@japaconnect.com');
    await page.locator("input[type='password'], input[name='password']").first().fill('Admin123!');
    await page.locator("button[type='submit'], button:has-text('Sign in'), button:has-text('Login'), button:has-text('Log in'), button:has-text('Continue')").first().click();
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(2000);
    console.log('Logged in, URL:', page.url());
    
    // Clear request tracking
    requests.length = 0;
    responses.length = 0;
    consoleMessages.length = 0;
    
    // Navigate to organisations
    await page.goto('http://localhost:3001/organisations');
    
    // Wait for network to settle
    await page.waitForTimeout(8000);
    
    console.log('\n=== API Requests ===');
    requests.forEach(r => console.log(`${r.method} ${r.url}`));
    
    console.log('\n=== API Responses ===');
    responses.forEach(r => console.log(`${r.status} ${r.url}`));
    
    console.log('\n=== Console Messages ===');
    consoleMessages.forEach(m => console.log(`[${m.type}] ${m.text.substring(0, 300)}`));
    
    // Check the DOM for error messages
    const errorText = await page.locator('[style*="danger"]').allTextContents();
    console.log('\n=== Error Elements ===');
    errorText.forEach(t => console.log(t));
    
    // Check loading state
    const loadingVisible = await page.locator('.animate-pulse, [class*="skeleton"]').count();
    console.log('\n=== Loading Elements ===', loadingVisible);
    
    // Check if DataTable rendered
    const tableRows = await page.locator('table tbody tr').count();
    console.log('=== Table Rows ===', tableRows);
    
    // Try to intercept and get the actual API response
    const apiResult = await page.evaluate(async () => {
        try {
            const res = await fetch('/api/v1/organizations?page=0&limit=25', {
                credentials: 'include'
            });
            const text = await res.text();
            return { status: res.status, body: text.substring(0, 1000) };
        } catch (e) {
            return { error: e.message };
        }
    });
    console.log('\n=== Direct API Call from Browser ===');
    console.log(JSON.stringify(apiResult, null, 2));
    
    await browser.close();
})();
