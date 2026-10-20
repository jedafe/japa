<script lang="ts">
	// §122 in-article next-action card — renders the first in_article-channel
	// suggestion as a bordered inline card inside article/reader bodies.
	// Guests and members alike (API does the filtering); hidden while empty.
	import { onMount } from 'svelte';
	import { getNextActions, dismissNextAction, completeNextAction, trackNextAction } from '$lib/api/nextActions';
	import type { NextAction } from '$lib/types';

	let action = $state<NextAction | null>(null);
	let dismissed = $state(false);

	onMount(async () => {
		const list = await getNextActions('main');
		action = list.find((a) => a.channels.includes('in_article')) ?? null;
		if (action) { /* server records the impression */ }
	});

	function click() {
		if (!action) return;
		trackNextAction(action.id, 'clicked');
		void completeNextAction(action.id);
	}

	function dismiss() {
		if (!action) return;
		void dismissNextAction(action.id);
		dismissed = true;
	}
</script>

{#if action && !dismissed}
	<aside
		class="my-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4 not-prose"
	>
		<div class="flex items-start justify-between gap-3">
			<div class="min-w-0">
				<p class="text-sm font-semibold text-[var(--color-text)]">{action.title}</p>
				{#if action.body}
					<p class="mt-1 text-sm text-[var(--color-text-muted)]">{action.body}</p>
				{/if}
				<a
					href={action.cta_url}
					onclick={click}
					class="mt-2 inline-block rounded-full bg-[var(--color-accent)] px-3 py-1 text-xs font-medium text-white hover:opacity-90"
				>{action.cta_label}</a>
			</div>
			<button
				onclick={dismiss}
				class="shrink-0 rounded-full p-1 text-[var(--color-text-muted)] hover:text-[var(--color-text)]"
				aria-label="Dismiss suggestion"
			>✕</button>
		</div>
	</aside>
{/if}
