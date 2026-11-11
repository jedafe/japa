import * as Sentry from '@sentry/sveltekit';
import { dev } from '$app/env';
import { PUBLIC_JC_GLITCHTIP_DSN } from '$app/env/public';

export function shouldInitErrorTracking(dsn: string | undefined, isDev: boolean): boolean {
	return !!dsn && !isDev;
}

if (shouldInitErrorTracking(PUBLIC_JC_GLITCHTIP_DSN, dev)) {
	Sentry.init({
		dsn: PUBLIC_JC_GLITCHTIP_DSN,
		tracesSampleRate: 0,
		// Without the sentrySvelteKit vite plugin the SDK bundles its Svelte-4
		// tracing variant, which subscribes to $app/stores — removed in
		// SvelteKit 3 and throws app_stores_removed on init. Tracing is off
		// anyway, so drop the integration.
		integrations: (integrations) =>
			integrations.filter((i) => i.name !== 'BrowserTracing')
	});
}

export const handleError = Sentry.handleErrorWithSentry();
