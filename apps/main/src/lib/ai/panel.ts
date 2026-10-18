// §115 AI Quick Panel — wires the shared @japaconnect/ai-panel package to
// main's api/csrf/i18n. Called once from the (app) layout onMount on every
// page: /ai hides the FAB but JapaAIChat streams through the same factory.
import { PUBLIC_JC_API_URL } from '$env/static/public';
import { initAiPanelDeps } from '@japaconnect/ai-panel';
import { getCsrfToken, refreshCsrfToken } from '$lib/api/csrf.svelte';
import { t } from '$lib/i18n';

let inited = false;

export function ensureAiPanelDeps(): void {
	if (inited) return;
	inited = true;
	initAiPanelDeps({
		apiBase: PUBLIC_JC_API_URL,
		surface: 'main',
		getCsrfToken,
		refreshCsrfToken,
		t,
		fullChatUrl: (c) => (c ? `/ai?c=${encodeURIComponent(c)}` : '/ai')
	});
}
