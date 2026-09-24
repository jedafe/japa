import { defineConfig } from 'wxt';

// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md
// (Browser-Extension Digest) + docs/superpowers/specs/2026-09-24-browser-
// extension-design.md.
//
// The API origin drives both host_permissions (MV3 host permissions bypass
// CORS for extension origins — no server CORS change) and the runtime base
// URL. Override with WXT_JC_API_ORIGIN for dev/staging builds.
const apiOrigin = process.env.WXT_JC_API_ORIGIN ?? 'https://japa.to';

export default defineConfig({
	modules: ['@wxt-dev/module-svelte'],
	manifest: {
		name: 'JapaConnect',
		short_name: 'JapaConnect',
		description:
			'Your migration feed — destination visa intel, safety alerts, and community activity on every new tab.',
		permissions: ['storage'],
		host_permissions: [`${apiOrigin}/*`],
		// No tabs/cookies permissions, no content scripts — the install
		// prompt stays minimal and the surface stays read-only (V1).
		browser_specific_settings: {
			gecko: { id: 'newtab@japaconnect.com' },
		},
	},
});
