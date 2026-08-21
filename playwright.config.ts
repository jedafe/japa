import { defineConfig } from '@playwright/test';

export default defineConfig({
	testDir: './tests/e2e',
	timeout: 60_000,
	retries: 1,
	workers: 1,
	use: {
		screenshot: 'only-on-failure',
		trace: 'on-first-retry',
		video: 'retain-on-failure'
	},
	projects: [
		{
			name: 'main',
			testMatch: ['main.spec.ts', 'main-journeys.spec.ts', 'ssr-integrity.spec.ts'],
			use: { baseURL: 'http://localhost:3000', channel: 'chrome' }
		},
		{
			name: 'admin',
			testMatch: 'admin.spec.ts',
			use: { baseURL: 'http://localhost:3001', channel: 'chrome' }
		},
		{
			name: 'control',
			testMatch: 'control.spec.ts',
			use: { baseURL: 'http://localhost:3002', channel: 'chrome' }
		},
		{
			name: 'connectivity',
			testMatch: 'api-connectivity.spec.ts',
			use: { baseURL: 'http://localhost:3000', channel: 'chrome' }
		},
		{
			name: 'docs',
			testMatch: 'docs.spec.ts',
			use: { baseURL: 'http://localhost:3004', channel: 'chrome' }
		}
	]
});
