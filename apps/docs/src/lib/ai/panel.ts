// §115 AI Quick Panel — wires @japaconnect/ai-panel to the Docs app's
// api/csrf/i18n. Called once from the root layout onMount.
import { env } from '$env/dynamic/public';
import { initAiPanelDeps } from '@japaconnect/ai-panel';
import { getCsrfToken, refreshCsrfToken } from '$lib/api/csrf';
import { t } from '$lib/i18n';

const API_BASE = (env.PUBLIC_JC_API_URL ?? '/api/v1').replace(/\/+$/, '');
// Same convention as GuestAuthButtons/UserDropdown/GuestHeader/GuestFooter.
const MAIN_ORIGIN = (env.PUBLIC_MAIN_ORIGIN ?? 'http://localhost:3000').replace(/\/$/, '');

let inited = false;

export function ensureAiPanelDeps(): void {
	if (inited) return;
	inited = true;
	initAiPanelDeps({
		apiBase: API_BASE,
		surface: 'docs',
		getCsrfToken,
		refreshCsrfToken,
		// Docs' t() is narrower on params (string only) — adapt to the
		// package's string|number signature.
		t: (key, fallback, params) => t(key, fallback, params as Record<string, string> | undefined),
		fullChatUrl: (c) =>
			`${MAIN_ORIGIN}${c ? `/ai?c=${encodeURIComponent(c)}` : '/ai'}`
	});
}
