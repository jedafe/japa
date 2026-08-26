import type { PlaywrightTestConfig } from '@playwright/test';

const config: PlaywrightTestConfig = {
	testDir: 'tests/e2e',
	use: {
		baseURL: 'http://localhost:3010',
		trace: 'on-first-retry'
	},
	webServer: {
		command: 'bun run preview',
		url: 'http://localhost:3010',
		reuseExistingServer: !process.env.CI,
		timeout: 120000
	},
	fullyParallel: true,
	reporter: 'list'
};

export default config;
