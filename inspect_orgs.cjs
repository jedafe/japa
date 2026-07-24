const { chromium } = require('playwright');

(async () => {
    const browser = await chromium.launch({ headless: true });
    const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
    
    // Navigate to admin
    await page.goto('http://localhost:3001');
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(2000);
    
    // Screenshot landing
    await page.screenshot({ path: '/tmp/org_01_landing.png', fullPage: true });
    console.log('Landing URL:', page.url());
    console.log('Landing title:', await page.title());
    
    // Find and fill login
    const emailInput = page.locator("input[type='email'], input[name='email'], input[placeholder*='email' i]").first();
    if (await emailInput.isVisible()) {
        await emailInput.fill('admin@japaconnect.com');
        const pwdInput = page.locator("input[type='password'], input[name='password']").first();
        await pwdInput.fill('Admin123!');
        
        // Capture console errors
        const consoleErrors = [];
        page.on('console', msg => {
            if (msg.type() === 'error') consoleErrors.push(msg.text());
        });
        
        await page.locator("button[type='submit'], button:has-text('Sign in'), button:has-text('Login'), button:has-text('Log in'), button:has-text('Continue')").first().click();
        await page.waitForLoadState('networkidle');
        await page.waitForTimeout(3000);
        
        await page.screenshot({ path: '/tmp/org_02_after_login.png', fullPage: true });
        console.log('After login URL:', page.url());
    } else {
        console.log('No email input found');
        const html = await page.content();
        console.log('Page content (first 3000 chars):', html.substring(0, 3000));
    }
    
    // Navigate to organisations
    await page.goto('http://localhost:3001/organisations');
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(3000);
    
    await page.screenshot({ path: '/tmp/org_03_organisations.png', fullPage: true });
    console.log('Organisations URL:', page.url());
    
    // Check for console errors on this page
    const orgConsoleErrors = [];
    page.on('console', msg => {
        if (msg.type() === 'error') orgConsoleErrors.push(msg.text());
    });
    
    // Reload to capture errors
    await page.reload();
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(3000);
    await page.screenshot({ path: '/tmp/org_04_reload.png', fullPage: true });
    
    if (orgConsoleErrors.length > 0) {
        console.log('\nConsole errors:');
        orgConsoleErrors.forEach(e => console.log('  -', e));
    }
    
    // Inspect DOM structure
    const tableExists = await page.locator('table').count();
    console.log('\nTables found:', tableExists);
    
    if (tableExists > 0) {
        const headers = await page.locator('table thead th').allTextContents();
        console.log('Table headers:', headers);
        const rowCount = await page.locator('table tbody tr').count();
        console.log('Table rows:', rowCount);
    }
    
    // Check DataTable
    const dataTable = await page.locator('[class*="data-table"], [class*="DataTable"]').count();
    console.log('DataTable components:', dataTable);
    
    // Check pagination
    const pagination = await page.locator('[class*="pagination"], [class*="Pagination"], nav[aria-label*="pagination" i]').count();
    console.log('Pagination elements:', pagination);
    
    // Check filter bar
    const filterBar = await page.locator('[class*="filter"], [class*="Filter"]').count();
    console.log('Filter elements:', filterBar);
    
    // Check for error display
    const errorElements = await page.locator('[style*="danger"], [class*="error"], [role="alert"]').all();
    for (const el of errorElements) {
        if (await el.isVisible()) {
            const text = await el.textContent();
            console.log('Error visible:', text?.substring(0, 200));
        }
    }
    
    // Check buttons
    const buttons = await page.locator('button').allTextContents();
    console.log('\nButtons on page:', buttons.filter(b => b.trim()).map(b => b.trim()));
    
    // Check for broken images/icons
    const images = await page.locator('img').count();
    console.log('Images:', images);
    
    // Check for skeleton loaders
    const skeletons = await page.locator('[class*="skeleton"], [class*="Skeleton"], [class*="animate-pulse"]').count();
    console.log('Skeleton/pulse elements:', skeletons);
    
    // Save full HTML
    const html = await page.content();
    const fs = require('fs');
    fs.writeFileSync('/tmp/org_page.html', html);
    console.log('\nFull HTML saved to /tmp/org_page.html');
    
    await browser.close();
    console.log('\nDone!');
})();
