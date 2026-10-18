import { buildApiUrl } from '@japaconnect/api-url';
import type { AiPanelDeps } from './types';

let deps: AiPanelDeps | null = null;

/** Wire the host app's api/csrf/i18n into the package. Idempotent — call
 * once from the root layout's onMount on every page (§115). */
export function initAiPanelDeps(d: AiPanelDeps): void {
	deps = d;
}

export function getAiPanelDeps(): AiPanelDeps {
	if (!deps) throw new Error('ai-panel: initAiPanelDeps() not called');
	return deps;
}

/** Panel i18n — falls back to the English default when no t is injected. */
export function pt(key: string, fallback: string, params?: Record<string, string | number>): string {
	return deps?.t?.(key, fallback, params) ?? fallback;
}

function headers(extra?: Record<string, string>): Record<string, string> {
	const d = getAiPanelDeps();
	const h: Record<string, string> = {
		accept: 'application/json',
		'x-jc-surface': d.surface,
		...extra
	};
	const csrf = d.getCsrfToken();
	if (csrf) h['x-csrf-token'] = csrf;
	return h;
}

async function unwrap<T>(res: Response): Promise<T | null> {
	if (!res.ok) return null;
	const json = (await res.json().catch(() => null)) as { data?: T } | null;
	return (json?.data ?? null) as T | null;
}

/** Authenticated GET returning the {data} envelope payload (null on error). */
export async function apiGet<T>(path: string): Promise<T | null> {
	const res = await fetch(buildApiUrl(path, getAiPanelDeps().apiBase), {
		credentials: 'include',
		headers: headers()
	});
	return unwrap<T>(res);
}

/** Authenticated POST returning the {data} envelope payload (null on error). */
export async function apiPost<T>(path: string, body?: unknown): Promise<T | null> {
	const res = await fetch(buildApiUrl(path, getAiPanelDeps().apiBase), {
		method: 'POST',
		credentials: 'include',
		headers: headers({ 'content-type': 'application/json' }),
		body: JSON.stringify(body ?? {})
	});
	return unwrap<T>(res);
}
