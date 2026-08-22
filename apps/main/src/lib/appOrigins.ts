// @ts-ignore — import.meta.env is Vite's typed env; available in client, SSR, and tests.
const isDev = (import.meta as any).env?.DEV === true;

/**
 * Cross-app origin helpers.
 *
 * The Main app links into the dedicated Learn / Japa University / Docs
 * SvelteKit surfaces. Origins are configurable via public env vars so a single
 * Main build can point at different deployments.
 *
 * We read `import.meta.env` rather than `$env/dynamic/public` because the
 * SvelteKit public-env virtual module is not available in unit tests, which
 * transitively import this file through dashboard view components.
 *
 * Ref: ADR-060 (cross-app deep links), ADR-063 (Japa University host detection)
 */
// @ts-ignore — import.meta.env is Vite's typed env; fallback for test/SSR.
const publicEnv: Record<string, string | undefined> =
	(typeof import.meta !== 'undefined' && (import.meta as any).env) || {};

const LEARN_BASE = isDev
	? 'http://localhost:3003'
	: (publicEnv.PUBLIC_JC_LEARN_ORIGIN ?? '');
const UNIVERSITY_BASE = isDev
	? 'http://localhost:3003'
	: (publicEnv.PUBLIC_JC_UNIVERSITY_ORIGIN ?? '');
const DOCS_BASE = isDev ? 'http://localhost:3004' : (publicEnv.PUBLIC_DOCS_ORIGIN ?? '');

export const LEARN_ORIGIN = LEARN_BASE.replace(/\/$/, '');
export const UNIVERSITY_ORIGIN = UNIVERSITY_BASE.replace(/\/$/, '');
export const DOCS_ORIGIN = DOCS_BASE.replace(/\/$/, '');

function buildUrl(origin: string, path: string): string {
	if (!origin) {
		throw new Error(
			`Cross-app origin is not configured. Set PUBLIC_JC_LEARN_ORIGIN and PUBLIC_JC_UNIVERSITY_ORIGIN.`
		);
	}
	const cleanPath = path.startsWith('/') ? path : `/${path}`;
	return `${origin}${cleanPath}`;
}

/** Build an absolute Learn app URL for a given path. */
export function learnUrl(path: string): string {
	return buildUrl(LEARN_ORIGIN, path);
}

/** Build an absolute Japa University app URL for a given path. */
export function universityUrl(path: string): string {
	return buildUrl(UNIVERSITY_ORIGIN, path);
}

/** Build an absolute Docs app URL for a given path. */
export function docsUrl(path: string): string {
	return buildUrl(DOCS_ORIGIN, path);
}
