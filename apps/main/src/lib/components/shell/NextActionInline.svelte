<script lang="ts">
	// §122 in-article next-action card — renders the first unclaimed
	// in_article-channel suggestion as a bordered inline card inside
	// article/reader bodies. Guests and members alike (API does the
	// filtering); hidden while empty.
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import {
		getNextActions,
		dismissNextAction,
		trackNextAction,
		claimNextAction,
		isNextActionClaimed,
		markNextActionPending
	} from '$lib/api/nextActions';
	import type { NextAction } from '$lib/types';
	import { nextActionIcon } from '$lib/icons/nextActionIcon';

	let action = $state<NextAction | null>(null);
	let dismissed = $state(false);

	onMount(async () => {
		const list = await getNextActions('main');
		action = list.find((a) => a.channels.includes('in_article') && !isNextActionClaimed(a.id)) ?? null;
		if (action) {
			claimNextAction(action.id);
			/* server records the impression */
		}
	});

	function click(e: MouseEvent) {
		if (!action) return;
		trackNextAction(action.id, 'clicked');
		markNextActionPending(action.id);
		// Internal paths navigate through the router — a full reload for a
		// same-origin CTA throws away the SPA session.
		if (action.cta_url.startsWith('/')) {
			e.preventDefault();
			void goto(action.cta_url);
		}
	}

	function dismiss() {
		if (!action) return;
		void dismissNextAction(action.id);
		dismissed = true;
	}
</script>

{#if action && !dismissed}
	{@const Icon = nextActionIcon(action.icon)}
	<aside
		class="my-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4 not-prose"
	>
		<div class="flex items-start justify-between gap-3">
			<div class="min-w-0">
				<p class="flex items-center gap-1.5 text-sm font-semibold text-[var(--color-text)]">
					<Icon size={15} class="shrink-0 text-[var(--color-accent)]" />{action.title}
				</p>
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
				aria-label={t('nextaction.dismiss_suggestion', 'Dismiss suggestion')}
			>✕</button>
		</div>
	</aside>
{/if}
