import * as Sentry from '@sentry/sveltekit';
import { dev } from '$app/environment';
import { PUBLIC_JC_GLITCHTIP_DSN } from '$env/static/public';

export function shouldInitErrorTracking(dsn: string, isDev: boolean): boolean {
	return dsn.length > 0 && !isDev;
}

if (shouldInitErrorTracking(PUBLIC_JC_GLITCHTIP_DSN, dev)) {
	Sentry.init({
		dsn: PUBLIC_JC_GLITCHTIP_DSN,
		tracesSampleRate: 0
	});
}

export const handleError = Sentry.handleErrorWithSentry();
