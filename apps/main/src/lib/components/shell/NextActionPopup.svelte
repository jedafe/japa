<script lang="ts">
	// §122 Next-Action popup — Bits UI dialog showing the highest-priority
	// unclaimed suggestion whose channels include popup, capped at
	// nextactions.popup_max_per_session per browser session.
	import { onMount } from 'svelte';
	import { Dialog } from 'bits-ui';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import {
		getNextActions,
		getPopupMaxPerSession,
		dismissNextAction,
		trackNextAction,
		claimNextAction,
		isNextActionClaimed,
		markNextActionPending
	} from '$lib/api/nextActions';
	import type { NextAction } from '$lib/types';
	import { nextActionIcon } from '$lib/icons/nextActionIcon';

	const SESSION_KEY = 'jc_nextaction_popup_count';

	let action = $state<NextAction | null>(null);
	let open = $state(false);
	// Set when a button handler already resolved the suggestion — the
	// onOpenChange path then must not apply its soft-close (snooze) rule.
	let resolved = false;

	onMount(async () => {
		const shown = Number(sessionStorage.getItem(SESSION_KEY) ?? '0');
		const [list, popupMax] = await Promise.all([getNextActions('main'), getPopupMaxPerSession('main')]);
		if (shown >= popupMax) return;
		action = list.find((a) => a.channels.includes('popup') && !isNextActionClaimed(a.id)) ?? null;
		if (action) {
			sessionStorage.setItem(SESSION_KEY, String(shown + 1));
			claimNextAction(action.id);
			/* server records the impression */
			open = true;
		}
	});

	function cta() {
		if (!action) return;
		resolved = true;
		trackNextAction(action.id, 'clicked');
		markNextActionPending(action.id);
		open = false;
		void goto(action.cta_url);
	}

	function dismiss() {
		if (!action) return;
		resolved = true;
		void dismissNextAction(action.id);
		open = false;
	}

	function snooze() {
		if (!action) return;
		resolved = true;
		void dismissNextAction(action.id, action.snooze_default_hours || undefined);
		open = false;
	}

	// Escape / backdrop close is a soft dismissal — the user didn't reject the
	// suggestion, they just closed it. Snooze when the rule defines one;
	// otherwise treat as dismiss so it doesn't reappear next view.
	function onOpenChange(v: boolean) {
		if (v || resolved || !action) {
			open = v;
			return;
		}
		resolved = true;
		open = false;
		if (action.snooze_default_hours > 0) void dismissNextAction(action.id, action.snooze_default_hours);
		else void dismissNextAction(action.id);
	}
</script>

<Dialog.Root {open} {onOpenChange}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-[90] bg-black/40" />
		<Dialog.Content
			class="fixed start-1/2 top-1/2 z-[91] w-full max-w-sm -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 shadow-xl"
		>
			{#if action}
				{@const Icon = nextActionIcon(action.icon)}
				<div class="mb-2 grid h-9 w-9 place-items-center rounded-full bg-[var(--color-accent-subtle)] text-[var(--color-accent)]">
					<Icon size={18} />
				</div>
				<Dialog.Title class="text-base font-semibold text-[var(--color-text)]">{action.title}</Dialog.Title>
				{#if action.body}
					<Dialog.Description class="mt-2 text-sm text-[var(--color-text-muted)]">{action.body}</Dialog.Description>
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
							{t('nextaction.later', 'Later')}
						</button>
					{/if}
					<button
						onclick={dismiss}
						class="rounded-full px-4 py-2 text-sm text-[var(--color-text-muted)] hover:text-[var(--color-text)]"
					>
						{t('nextaction.dismiss', 'Dismiss')}
					</button>
				</div>
			{/if}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
