<script lang="ts">
	// §122 Next-Action announcement bar — shows the highest-priority
	// suggestion whose channels include announcement_bar, for guests and
	// members alike (the API does the filtering).
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import {
		getNextActions,
		dismissNextAction,
		completeNextAction,
		trackNextAction
	} from '$lib/api/nextActions';
	import type { NextAction } from '$lib/types';

	let action = $state<NextAction | null>(null);
	let dismissed = $state(false);

	onMount(async () => {
		const list = await getNextActions('main');
		action = list.find((a) => a.channels.includes('announcement_bar')) ?? null;
		if (action) { /* server records the impression */ }
	});

	function cta() {
		if (!action) return;
		trackNextAction(action.id, 'clicked');
		void completeNextAction(action.id);
		void goto(action.cta_url);
	}

	function dismiss() {
		if (!action) return;
		void dismissNextAction(action.id);
		dismissed = true;
	}
</script>

{#if action && !dismissed}
	<div
		class="flex items-center gap-3 border-b border-[var(--color-border)] bg-[var(--color-accent-subtle)] px-4 py-2 text-sm"
		role="region"
		aria-label="Suggestion"
	>
		<span class="min-w-0 flex-1 truncate text-[var(--color-text)]">
			<strong class="font-medium">{action.title}</strong>
			{#if action.body}<span class="ml-1 hidden text-[var(--color-text-muted)] sm:inline">{action.body}</span>{/if}
		</span>
		<button
			onclick={cta}
			class="shrink-0 rounded-full bg-[var(--color-accent)] px-3 py-1 text-xs font-medium text-white hover:opacity-90"
		>
			{action.cta_label}
		</button>
		<button
			onclick={dismiss}
			class="shrink-0 rounded-full p-1 text-[var(--color-text-muted)] hover:text-[var(--color-text)]"
			aria-label="Dismiss suggestion"
		>
			✕
		</button>
	</div>
{/if}
