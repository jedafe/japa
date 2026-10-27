<script lang="ts">
	// §122 Next-Action announcement bar — shows the highest-priority
	// unclaimed suggestion whose channels include announcement_bar, for
	// guests and members alike (the API does the filtering).
	import { onMount } from 'svelte';
	import { t } from '$lib/i18n';
	import {
		getNextActions,
		dismissNextAction,
		trackNextAction,
		claimNextAction,
		releaseNextAction,
		isNextActionClaimed,
		markNextActionPending,
		nextActionsViewer,
		nextActionsSurface,
		goToNextAction
	} from '$lib/api/nextActions';
	import type { NextAction } from '$lib/types';
	import { nextActionIcon } from '$lib/icons/nextActionIcon';

	let action = $state<NextAction | null>(null);
	let dismissed = $state(false);
	let lastViewer = '';

	async function load() {
		const list = await getNextActions(nextActionsSurface());
		const prev = action;
		action = list.find((a) => a.channels.includes('announcement_bar') && !isNextActionClaimed(a.id)) ?? null;
		if (prev && prev.id !== action?.id) releaseNextAction(prev.id);
		if (action) {
			claimNextAction(action.id);
			/* server records the impression */
		}
	}

	onMount(() => {
		lastViewer = nextActionsViewer();
		void load();
		return () => { if (action) releaseNextAction(action.id); };
	});

	// Identity change (in-app login/signup/logout) — the rendered suggestion
	// was computed for a different viewer, re-pick against the fresh list.
	$effect(() => {
		const v = nextActionsViewer();
		if (lastViewer === '' || v === lastViewer) { lastViewer = v; return; }
		lastViewer = v;
		dismissed = false;
		void load();
	});

	function cta() {
		if (!action) return;
		trackNextAction(action.id, 'clicked');
		markNextActionPending(action.id);
		goToNextAction(action.cta_url);
	}

	function dismiss() {
		if (!action) return;
		void dismissNextAction(action.id);
		dismissed = true;
	}

	function snooze() {
		if (!action) return;
		void dismissNextAction(action.id, action.snooze_default_hours || undefined);
		dismissed = true;
	}
</script>

{#if action && !dismissed}
	{@const Icon = nextActionIcon(action.icon)}
	<div
		class="flex items-center gap-3 border-b border-[var(--color-border)] bg-[var(--color-accent-subtle)] px-4 py-2 text-sm"
		role="region"
		aria-label={t('nextaction.suggestion', 'Suggestion')}
	>
		<Icon size={15} class="shrink-0 text-[var(--color-accent)]" />
		<span class="min-w-0 flex-1 truncate text-[var(--color-text)]">
			<strong class="font-medium">{action.title}</strong>
			{#if action.body}<span class="ms-1 hidden text-[var(--color-text-muted)] sm:inline">{action.body}</span>{/if}
		</span>
		<button
			onclick={cta}
			class="shrink-0 rounded-full bg-[var(--color-accent)] px-3 py-1 text-xs font-medium text-white hover:opacity-90"
		>
			{action.cta_label}
		</button>
		{#if action.snooze_default_hours > 0}
			<button
				onclick={snooze}
				class="shrink-0 rounded-full px-2 py-1 text-xs text-[var(--color-text-muted)] hover:text-[var(--color-text)]"
			>
				{t('nextaction.later', 'Later')}
			</button>
		{/if}
		<button
			onclick={dismiss}
			class="shrink-0 rounded-full p-1 text-[var(--color-text-muted)] hover:text-[var(--color-text)]"
			aria-label={t('nextaction.dismiss_suggestion', 'Dismiss suggestion')}
		>
			✕
		</button>
	</div>
{/if}
