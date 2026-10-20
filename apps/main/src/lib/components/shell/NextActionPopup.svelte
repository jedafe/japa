<script lang="ts">
	// §122 Next-Action popup — one modal per browser session showing the
	// highest-priority suggestion whose channels include popup.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import {
		getNextActions,
		dismissNextAction,
		completeNextAction,
		trackNextAction
	} from '$lib/api/nextActions';
	import type { NextAction } from '$lib/types';

	const SESSION_KEY = 'jc_nextaction_popup_shown';

	let action = $state<NextAction | null>(null);
	let open = $state(false);

	onMount(async () => {
		if (sessionStorage.getItem(SESSION_KEY)) return;
		const list = await getNextActions('main');
		action = list.find((a) => a.channels.includes('popup')) ?? null;
		if (action) {
			sessionStorage.setItem(SESSION_KEY, '1');
			/* server records the impression */
			open = true;
		}
	});

	function cta() {
		if (!action) return;
		trackNextAction(action.id, 'clicked');
		void completeNextAction(action.id);
		open = false;
		void goto(action.cta_url);
	}

	function dismiss() {
		if (!action) return;
		void dismissNextAction(action.id);
		open = false;
	}

	function snooze() {
		if (!action) return;
		void dismissNextAction(action.id, action.snooze_default_hours);
		open = false;
	}
</script>

{#if action && open}
	<div
		class="fixed inset-0 z-[90] flex items-center justify-center bg-black/40 p-4"
		role="presentation"
	>
		<div
			class="w-full max-w-sm rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 shadow-xl"
			role="dialog"
			aria-modal="true"
			aria-label={action.title}
			tabindex="-1"
			onkeydown={(e) => e.key === 'Escape' && dismiss()}
		>
			<h2 class="text-base font-semibold text-[var(--color-text)]">{action.title}</h2>
			{#if action.body}
				<p class="mt-2 text-sm text-[var(--color-text-muted)]">{action.body}</p>
			{/if}
			<div class="mt-4 flex items-center gap-2">
				<button
					onclick={cta}
					class="flex-1 rounded-full bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white hover:opacity-90"
				>
					{action.cta_label}
				</button>
				{#if action.snooze_default_hours > 0}
					<button
						onclick={snooze}
						class="rounded-full px-4 py-2 text-sm text-[var(--color-text-muted)] hover:text-[var(--color-text)]"
					>
						Later
					</button>
				{/if}
				<button
					onclick={dismiss}
					class="rounded-full px-4 py-2 text-sm text-[var(--color-text-muted)] hover:text-[var(--color-text)]"
				>
					Dismiss
				</button>
			</div>
		</div>
	</div>
{/if}
