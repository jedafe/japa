// §115 AI Quick Panel — wires @japaconnect/ai-panel to the Academy app's
// api/csrf/i18n. Called once from the root layout onMount.
import { PUBLIC_JC_API_URL } from '$env/static/public';
import { initAiPanelDeps } from '@japaconnect/ai-panel';
import { getCsrfToken, refreshCsrfToken } from '$lib/api/csrf.svelte';
import { t } from '$lib/i18n';
import { MAIN_ORIGIN } from '$lib/appOrigins';

let inited = false;

export function ensureAiPanelDeps(): void {
	if (inited) return;
	inited = true;
	initAiPanelDeps({
		apiBase: PUBLIC_JC_API_URL,
		surface: 'academy',
		getCsrfToken,
		refreshCsrfToken,
		t,
		fullChatUrl: (c) =>
			`${MAIN_ORIGIN}${c ? `/ai?c=${encodeURIComponent(c)}` : '/ai'}`
	});
}
