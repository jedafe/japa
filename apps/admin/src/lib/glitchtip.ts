import { browser } from '$app/environment';

let initialized = false;

export function initGlitchTip() {
	if (initialized || !browser) return;

	const dsn = import.meta.env.VITE_GLITCHTIP_DSN;
	if (!dsn) return;

	try {
		const script = document.createElement('script');
		script.src = 'https://browser.sentry-cdn.com/7.119.0/bundle.min.js';
		script.crossOrigin = 'anonymous';
		script.onload = () => {
			const Sentry = (window as any).Sentry;
			if (Sentry) {
				Sentry.init({
					dsn,
					tracesSampleRate: 0.1,
					replaysSessionSampleRate: 0,
					replaysOnErrorSampleRate: 0,
					environment: import.meta.env.MODE ?? 'development',
					enabled: import.meta.env.MODE !== 'development'
				});
			}
		};
		document.head.appendChild(script);
		initialized = true;
	} catch {
		// silently fail — error tracking is non-critical
	}
}
