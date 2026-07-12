import { defineConfig } from '@playwright/test';

export default defineConfig({
	testDir: './tests/e2e',
	timeout: 30_000,
	retries: 1,
	workers: process.env.CI ? 2 : undefined,
	use: {
		screenshot: 'only-on-failure',
		trace: 'on-first-retry',
		video: 'retain-on-failure'
	},
	projects: [
		{
			name: 'main',
			testMatch: ['main.spec.ts', 'main-journeys.spec.ts', 'ssr-integrity.spec.ts'],
			use: { baseURL: 'http://localhost:3000' }
		},
		{
			name: 'admin',
			testMatch: 'admin.spec.ts',
			use: { baseURL: 'http://localhost:3001' }
		},
		{
			name: 'control',
			testMatch: 'control.spec.ts',
			use: { baseURL: 'http://localhost:3002' }
		},
		{
			name: 'learn',
			testMatch: 'learn.spec.ts',
			use: { baseURL: 'http://localhost:3003' }
		},
		{
			name: 'docs',
			testMatch: 'docs.spec.ts',
			use: { baseURL: 'http://localhost:3004' }
		},
		{
			name: 'connectivity',
			testMatch: 'api-connectivity.spec.ts',
			use: { baseURL: 'http://localhost:3000' }
		}
	],
	webServer: [
		{
			command: 'cd apps/main && bun run dev',
			port: 3000,
			reuseExistingServer: true,
			timeout: 30_000
		},
		{
			command: 'cd apps/admin && bun run dev',
			port: 3001,
			reuseExistingServer: true,
			timeout: 30_000
		},
		{
			command: 'cd apps/control && bun run dev',
			port: 3002,
			reuseExistingServer: true,
			timeout: 30_000
		},
		{
			command: 'cd apps/learn && bun run dev',
			port: 3003,
			reuseExistingServer: true,
			timeout: 30_000
		},
		{
			command: 'cd apps/docs && bun run dev',
			port: 3004,
			reuseExistingServer: true,
			timeout: 30_000
		}
	]
});
