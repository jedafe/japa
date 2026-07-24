import { defineConfig } from '@playwright/test';

export default defineConfig({
	testDir: './tests/e2e',
	timeout: 60_000,
	retries: 0,
	workers: 1,
	use: {
		screenshot: 'only-on-failure',
		trace: 'on-first-retry',
		channel: 'chrome',
	},
	projects: [
		{
			name: 'admin',
			testMatch: 'page-builder-wiring.spec.ts',
			use: { baseURL: 'http://localhost:3001' }
		}
	]
});
