// Runes-based translator lives in a .svelte.ts module (required for $state).
export { i18n, t, siteName, type Locale } from './store.svelte';
export { DEFAULT_SITE_NAME, substituteSiteName } from './dict';
