<script lang="ts">
	// §122 Next-Action renderer (docs surface) — announcement_bar + popup
	// (+ an in_article card slot the page can opt into via the `inline` flag
	// on tools pages that render article-style content). The API does all
	// audience/state filtering; popup cap comes from meta.popup_max_per_session.
	import { onMount } from 'svelte';
	import { env } from '$env/dynamic/public';

	const API_BASE = (env.PUBLIC_JC_API_URL ?? '/api/v1').replace(/\/+$/, '');

	interface NextAction {
		id: string;
		title: string;
		body: string;
		cta_label: string;
		cta_url: string;
		icon: string;
		channels: string[];
		snooze_default_hours: number;
	}

	let { inline = false }: { inline?: boolean } = $props();

	let bar = $state<NextAction | null>(null);
	let popup = $state<NextAction | null>(null);
	let inlineAction = $state<NextAction | null>(null);
	let open = $state(false);
	let resolved = false;
	const POPUP_KEY = 'jc_nextaction_popup_count';
	const PENDING = 'jc_na_pending';

	function post(path: string, body: unknown) {
		void fetch(`${API_BASE}${path}`, {
			method: 'POST', credentials: 'include',
			headers: { 'content-type': 'application/json', 'X-JC-Surface': 'docs' },
			body: JSON.stringify(body)
		}).catch(() => {});
	}

	function pick(list: NextAction[], ch: string) {
		return list.find((a) => a.channels.includes(ch)) ?? null;
	}

	onMount(async () => {
		// apiGet unwraps `data` but drops sibling `meta` — fetch raw here so
		// the server-configured popup cap still applies.
		let list: NextAction[] = [];
		let cap = 1;
		try {
			const res = await fetch(`${API_BASE}/next-actions?surface=docs`, {
				credentials: 'include',
				headers: { Accept: 'application/json', 'X-JC-Surface': 'docs' }
			});
			const json = await res.json();
			list = json?.data ?? [];
			cap = json?.meta?.popup_max_per_session ?? 1;
		} catch { /* suggestion bar degrades silently */ }
		bar = pick(list, 'announcement_bar');
		if (inline) inlineAction = pick(list, 'in_article');
		const shown = Number(sessionStorage.getItem(POPUP_KEY) ?? '0');
		popup = shown < cap ? pick(list, 'popup') : null;
		if (popup) {
			sessionStorage.setItem(POPUP_KEY, String(shown + 1));
			open = true;
		}
	});

	function cta(a: NextAction) {
		post(`/next-actions/${a.id}/track`, { event: 'clicked' });
		try { sessionStorage.setItem(PENDING, a.id); } catch { /* ok */ }
		if (a === popup) { resolved = true; open = false; }
	}

	function act(a: NextAction, snoozeHours?: number) {
		post(`/next-actions/${a.id}/dismiss`, snoozeHours ? { snooze_hours: snoozeHours } : {});
		if (a === bar) bar = null;
		if (a === inlineAction) inlineAction = null;
		if (a === popup) { resolved = true; open = false; }
	}

	// Escape / backdrop close = soft dismiss (snooze when defined).
	function softClose() {
		if (resolved || !popup) return;
		resolved = true;
		open = false;
		act(popup, popup.snooze_default_hours || undefined);
	}
</script>

{#if bar}
	<div class="flex items-center gap-3 border-b border-[var(--color-border)] bg-[var(--color-accent-subtle)] px-4 py-2 text-sm" role="region" aria-label="Suggestion">
		<span class="min-w-0 flex-1 truncate text-[var(--color-text)]">
			<strong class="font-medium">{bar.title}</strong>
			{#if bar.body}<span class="ml-1 hidden text-[var(--color-text-muted)] sm:inline">{bar.body}</span>{/if}
		</span>
		<a href={bar.cta_url} onclick={() => cta(bar!)} class="shrink-0 rounded-full bg-[var(--color-accent)] px-3 py-1 text-xs font-medium text-white hover:opacity-90">{bar.cta_label}</a>
		{#if bar.snooze_default_hours > 0}
			<button onclick={() => act(bar!, bar!.snooze_default_hours)} class="shrink-0 rounded-full px-2 py-1 text-xs text-[var(--color-text-muted)] hover:text-[var(--color-text)]">Later</button>
		{/if}
		<button onclick={() => act(bar!)} class="shrink-0 rounded-full p-1 text-[var(--color-text-muted)] hover:text-[var(--color-text)]" aria-label="Dismiss suggestion">✕</button>
	</div>
{/if}

{#if popup && open}
	<div class="fixed inset-0 z-[90] flex items-center justify-center bg-black/40 p-4" role="presentation" onclick={softClose} onkeydown={(e) => e.key === 'Escape' && softClose()}>
		<div class="w-full max-w-sm rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 shadow-xl" role="dialog" aria-modal="true" aria-label={popup.title} onclick={(e) => e.stopPropagation()}>
			<h2 class="text-base font-semibold text-[var(--color-text)]">{popup.title}</h2>
			{#if popup.body}<p class="mt-2 text-sm text-[var(--color-text-muted)]">{popup.body}</p>{/if}
			<div class="mt-4 flex items-center gap-2">
				<a href={popup.cta_url} onclick={() => cta(popup!)} class="flex-1 rounded-full bg-[var(--color-accent)] px-4 py-2 text-center text-sm font-medium text-white hover:opacity-90">{popup.cta_label}</a>
				{#if popup.snooze_default_hours > 0}
					<button onclick={() => act(popup!, popup!.snooze_default_hours)} class="rounded-full px-4 py-2 text-sm text-[var(--color-text-muted)] hover:text-[var(--color-text)]">Later</button>
				{/if}
				<button onclick={() => act(popup!)} class="rounded-full px-4 py-2 text-sm text-[var(--color-text-muted)] hover:text-[var(--color-text)]">Dismiss</button>
			</div>
		</div>
	</div>
{/if}

{#if inline && inlineAction}
	<aside class="my-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4">
		<div class="flex items-start justify-between gap-3">
			<div class="min-w-0">
				<p class="text-sm font-semibold text-[var(--color-text)]">{inlineAction.title}</p>
				{#if inlineAction.body}<p class="mt-1 text-sm text-[var(--color-text-muted)]">{inlineAction.body}</p>{/if}
				<a href={inlineAction.cta_url} onclick={() => cta(inlineAction!)} class="mt-2 inline-block rounded-full bg-[var(--color-accent)] px-3 py-1 text-xs font-medium text-white hover:opacity-90">{inlineAction.cta_label}</a>
			</div>
			<button onclick={() => act(inlineAction!)} class="shrink-0 rounded-full p-1 text-[var(--color-text-muted)] hover:text-[var(--color-text)]" aria-label="Dismiss suggestion">✕</button>
		</div>
	</aside>
{/if}
