<script lang="ts">
	// §122 Next-Action popup — Bits UI dialog showing the highest-priority
	// unclaimed suggestion whose channels include popup, capped at
	// nextactions.popup_max_per_session per browser session.
	import { onMount } from 'svelte';
	import { Dialog } from 'bits-ui';
	import { ArrowRight, Clock, X } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import {
		getNextActions,
		getPopupMaxPerSession,
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

	const SESSION_KEY = 'jc_nextaction_popup_count';

	let action = $state<NextAction | null>(null);
	let open = $state(false);
	// Set when a button handler already resolved the suggestion — the
	// onOpenChange path then must not apply its soft-close (snooze) rule.
	let resolved = false;
	let lastViewer = '';

	async function load() {
		const surface = nextActionsSurface();
		const shown = Number(sessionStorage.getItem(SESSION_KEY) ?? '0');
		const [list, popupMax] = await Promise.all([getNextActions(surface), getPopupMaxPerSession(surface)]);
		// Identity changed mid-display — the rendered suggestion was computed
		// for a different viewer, close it before re-picking.
		if (open) {
			resolved = true;
			open = false;
			if (action) releaseNextAction(action.id);
			action = null;
		}
		if (shown >= popupMax) return;
		action = list.find((a) => a.channels.includes('popup') && !isNextActionClaimed(a.id)) ?? null;
		if (action) {
			sessionStorage.setItem(SESSION_KEY, String(shown + 1));
			claimNextAction(action.id);
			/* server records the impression */
			resolved = false; // re-arm soft-close for the new suggestion
			open = true;
		}
	}

	onMount(() => {
		lastViewer = nextActionsViewer();
		void load();
		return () => { if (action) releaseNextAction(action.id); };
	});

	// In-app login/signup/logout — re-pick against the new identity's list.
	$effect(() => {
		const v = nextActionsViewer();
		if (lastViewer === '' || v === lastViewer) { lastViewer = v; return; }
		lastViewer = v;
		void load();
	});

	function cta() {
		if (!action) return;
		resolved = true;
		trackNextAction(action.id, 'clicked');
		markNextActionPending(action.id);
		open = false;
		goToNextAction(action.cta_url);
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
		<Dialog.Overlay class="jc-na-overlay fixed inset-0 z-[90] bg-black/55 backdrop-blur-[2px]" />
		<Dialog.Content
			class="jc-na-content fixed left-1/2 top-1/2 z-[91] w-[calc(100vw-2rem)] max-w-sm -translate-x-1/2 -translate-y-1/2 overflow-hidden rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-2xl focus:outline-none"
		>
			{#if action}
				{@const Icon = nextActionIcon(action.icon)}
				<!-- Brand band — gradient header with a frosted icon tile and a
				     faint watermark of the same icon so the suggestion reads as a
				     deliberate "next step" moment, not a generic alert. -->
				<div class="relative h-24 overflow-hidden bg-gradient-to-br from-[var(--color-primary)] via-[var(--color-primary-hover)] to-[var(--color-secondary)]">
					<div class="jc-na-dots absolute inset-0" aria-hidden="true"></div>
					<Icon
						size={150}
						strokeWidth={1}
						class="absolute -end-7 -top-9 rotate-12 text-white/10"
						aria-hidden="true"
					/>
					<div class="absolute bottom-3 start-5 grid h-11 w-11 place-items-center rounded-xl bg-white/15 text-white ring-1 ring-white/30 backdrop-blur-sm">
						<Icon size={22} />
					</div>
					<Dialog.Close
						class="absolute end-3 top-3 grid h-7 w-7 place-items-center rounded-full text-white/75 transition hover:bg-white/15 hover:text-white"
						aria-label={t('nextaction.dismiss_suggestion', 'Dismiss suggestion')}
					>
						<X size={15} />
					</Dialog.Close>
				</div>

				<div class="px-5 pb-5 pt-4">
					<p class="inline-flex items-center rounded-full bg-[var(--color-primary)]/10 px-2.5 py-0.5 text-[11px] font-semibold uppercase tracking-wider text-[var(--color-primary)]">
						{action.badge || t('nextaction.next_step', 'Your next step')}
					</p>
					<Dialog.Title class="mt-2 text-lg font-semibold leading-snug text-[var(--color-text)]">
						{action.title}
					</Dialog.Title>
					{#if action.body}
						<Dialog.Description class="mt-1.5 text-sm leading-relaxed text-[var(--color-text-subtle)]">
							{action.body}
						</Dialog.Description>
					{/if}

					<button
						onclick={cta}
						class="group mt-4 flex w-full items-center justify-center gap-2 rounded-full bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-white transition hover:bg-[var(--color-primary-hover)]"
					>
						{action.cta_label}
						<ArrowRight size={15} class="transition-transform group-hover:translate-x-0.5 rtl:-scale-x-100 rtl:group-hover:-translate-x-0.5" />
					</button>

					<div class="mt-3 flex items-center justify-center gap-1.5">
						{#if action.snooze_default_hours > 0}
							<button
								onclick={snooze}
								class="inline-flex items-center gap-1 rounded-full px-3 py-1.5 text-xs text-[var(--color-muted)] transition hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
							>
								<Clock size={12} />
								{t('nextaction.later', 'Later')}
							</button>
						{/if}
						<button
							onclick={dismiss}
							class="rounded-full px-3 py-1.5 text-xs text-[var(--color-muted)] transition hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
						>
							{t('nextaction.dismiss', 'Dismiss')}
						</button>
					</div>
				</div>
			{/if}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<style>
	/* Entrance — the overlay fades in while the card rises and settles.
	   Scoped keyframes (no animate plugin in this app); reduced-motion users
	   get an instant render. */
	/* data-state is set at runtime by Bits UI — :global keeps Svelte from
	   tree-shaking these selectors. */
	:global(.jc-na-overlay[data-state='open']) {
		animation: jc-na-fade 160ms ease-out;
	}
	:global(.jc-na-content[data-state='open']) {
		animation: jc-na-pop 220ms cubic-bezier(0.16, 1, 0.3, 1);
	}
	.jc-na-dots {
		background-image: radial-gradient(rgba(255, 255, 255, 0.22) 1px, transparent 1px);
		background-size: 14px 14px;
		mask-image: linear-gradient(to bottom right, black, transparent 70%);
	}
	@keyframes jc-na-fade {
		from { opacity: 0; }
	}
	/* Tailwind v4 positions via the standalone translate/scale properties,
	   so the keyframe animates those — not transform. */
	@keyframes jc-na-pop {
		from {
			opacity: 0;
			translate: -50% -46%;
			scale: 0.96;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		:global(.jc-na-overlay[data-state='open']),
		:global(.jc-na-content[data-state='open']) {
			animation: none;
		}
	}
</style>
