import { defineConfig } from 'wxt';

// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md
// (Browser-Extension Digest) + docs/superpowers/specs/2026-09-24-browser-
// extension-design.md.
//
// The site origin drives host_permissions (MV3 host permissions bypass
// CORS for extension origins — no server CORS change) and, at runtime, the
// API base + deep links (lib/links.ts reads the same WXT_JC_SITE_URL).
const siteOrigin = process.env.WXT_JC_SITE_URL ?? 'https://japa.to';

export default defineConfig({
	modules: ['@wxt-dev/module-svelte'],
	manifest: {
		name: 'JapaConnect',
		short_name: 'JapaConnect',
		description:
			'Your migration feed — destination visa intel, safety alerts, and community activity on every new tab.',
		// 'identity' powers the §120 "Login with JapaConnect" OAuth flow
		// (launchWebAuthFlow → chromiumapp.org redirect).
		permissions: ['storage', 'identity'],
		host_permissions: [`${siteOrigin}/*`],
		// No tabs/cookies permissions, no content scripts — the install
		// prompt stays minimal and the surface stays read-only (V1).
		browser_specific_settings: {
			gecko: { id: 'newtab@japaconnect.com' },
		},
	},
});
