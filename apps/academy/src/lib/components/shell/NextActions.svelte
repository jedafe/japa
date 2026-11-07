<script lang="ts">
	import { t } from '$lib/i18n';
	// §122 Next-Action renderer (academy surface) — announcement_bar + popup
	// (+ an in_article card slot the page can opt into via the `inline` flag
	// on tools pages that render article-style content). The API does all
	// audience/state filtering; popup cap comes from meta.popup_max_per_session.
	import { onMount } from 'svelte';
	import { api } from '$lib/api/client';
	import { Dialog } from 'bits-ui';
	import { ArrowRight, Clock, X } from 'lucide-svelte';
	import { nextActionIcon } from '$lib/icons/nextActionIcon';

	interface NextAction {
		id: string;
		title: string;
		body: string;
		cta_label: string;
		cta_url: string;
		icon: string;
		badge?: string;
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
		void api.post(path, body).catch(() => {});
	}

	// Cross-channel dedup — a rule carrying multiple channels must not render
	// on bar + inline + popup simultaneously.
	const taken = new Set<string>();
	function pick(list: NextAction[], ch: string) {
		const a = list.find((a) => a.channels.includes(ch) && !taken.has(a.id)) ?? null;
		if (a) taken.add(a.id);
		return a;
	}

	onMount(async () => {
		// getBody returns the raw envelope — sibling top-level `meta` carries
		// the server-configured popup cap.
		const res = await api.getBody<{ data: NextAction[]; meta?: { popup_max_per_session?: number } }>(
			'/next-actions?surface=academy'
		);
		const list = res.data?.data ?? [];
		bar = pick(list, 'announcement_bar');
		const shown = Number(sessionStorage.getItem(POPUP_KEY) ?? '0');
		const cap = res.data?.meta?.popup_max_per_session ?? 1;
		popup = shown < cap ? pick(list, 'popup') : null;
		if (popup) {
			sessionStorage.setItem(POPUP_KEY, String(shown + 1));
			open = true;
		}
		if (inline) inlineAction = pick(list, 'in_article');
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
	{@const BarIcon = nextActionIcon(bar.icon)}
	<div class="flex items-center gap-3 border-b border-[var(--color-border)] bg-[var(--color-primary)]/8 px-4 py-2 text-sm" role="region" aria-label={t('a11y.suggestion', 'Suggestion')}>
		<BarIcon size={15} class="shrink-0 text-[var(--color-primary)]" />
		<span class="min-w-0 flex-1 truncate text-[var(--color-text)]">
			<strong class="font-medium">{bar.title}</strong>
			{#if bar.body}<span class="ms-1 hidden text-[var(--color-text-muted)] sm:inline">{bar.body}</span>{/if}
		</span>
		<a href={bar.cta_url} onclick={() => cta(bar!)} class="shrink-0 rounded-full bg-[var(--color-primary)] px-3 py-1 text-xs font-medium text-white transition hover:bg-[var(--color-primary-hover)]">{bar.cta_label}</a>
		{#if bar.snooze_default_hours > 0}
			<button onclick={() => act(bar!, bar!.snooze_default_hours)} class="shrink-0 rounded-full px-2 py-1 text-xs text-[var(--color-text-muted)] hover:text-[var(--color-text)]">{t('nextaction.later', 'Later')}</button>
		{/if}
		<button onclick={() => act(bar!)} class="shrink-0 rounded-full p-1 text-[var(--color-text-muted)] hover:text-[var(--color-text)]" aria-label={t('nextaction.dismiss_suggestion', 'Dismiss suggestion')}>✕</button>
	</div>
{/if}

<Dialog.Root {open} onOpenChange={(v) => { if (!v) softClose(); else open = true; }}>
	<Dialog.Portal>
		<Dialog.Overlay class="jc-na-overlay fixed inset-0 z-[90] bg-black/55 backdrop-blur-[2px]" />
		<Dialog.Content class="jc-na-content fixed left-1/2 top-1/2 z-[91] w-[calc(100vw-2rem)] max-w-sm -translate-x-1/2 -translate-y-1/2 overflow-hidden rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-2xl focus:outline-none">
			{#if popup}
				{@const Icon = nextActionIcon(popup.icon)}
				<div class="relative h-24 overflow-hidden bg-gradient-to-br from-[var(--color-primary)] via-[var(--color-primary-hover)] to-[var(--color-secondary)]">
					<div class="jc-na-dots absolute inset-0" aria-hidden="true"></div>
					<Icon size={150} strokeWidth={1} class="absolute -end-7 -top-9 rotate-12 text-white/10" aria-hidden="true" />
					<div class="absolute bottom-3 start-5 grid h-11 w-11 place-items-center rounded-xl bg-white/15 text-white ring-1 ring-white/30 backdrop-blur-sm">
						<Icon size={22} />
					</div>
					<Dialog.Close class="absolute end-3 top-3 grid h-7 w-7 place-items-center rounded-full text-white/75 transition hover:bg-white/15 hover:text-white" aria-label={t('nextaction.dismiss_suggestion', 'Dismiss suggestion')}>
						<X size={15} />
					</Dialog.Close>
				</div>
				<div class="px-5 pb-5 pt-4">
					<p class="inline-flex items-center rounded-full bg-[var(--color-primary)]/10 px-2.5 py-0.5 text-[11px] font-semibold uppercase tracking-wider text-[var(--color-primary)]">
						{popup.badge || t('nextaction.next_step', 'Your next step')}
					</p>
					<Dialog.Title class="mt-2 text-lg font-semibold leading-snug text-[var(--color-text)]">{popup.title}</Dialog.Title>
					{#if popup.body}<Dialog.Description class="mt-1.5 text-sm leading-relaxed text-[var(--color-text-muted)]">{popup.body}</Dialog.Description>{/if}
					<a
						href={popup.cta_url}
						onclick={() => cta(popup!)}
						class="group mt-4 flex w-full items-center justify-center gap-2 rounded-full bg-[var(--color-primary)] px-4 py-2.5 text-sm font-medium text-white transition hover:bg-[var(--color-primary-hover)]"
					>
						{popup.cta_label}
						<ArrowRight size={15} class="transition-transform group-hover:translate-x-0.5 rtl:-scale-x-100 rtl:group-hover:-translate-x-0.5" />
					</a>
					<div class="mt-3 flex items-center justify-center gap-1.5">
						{#if popup.snooze_default_hours > 0}
							<button onclick={() => act(popup!, popup!.snooze_default_hours)} class="inline-flex items-center gap-1 rounded-full px-3 py-1.5 text-xs text-[var(--color-muted)] transition hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]">
								<Clock size={12} />
								{t('nextaction.later', 'Later')}
							</button>
						{/if}
						<button onclick={() => act(popup!)} class="rounded-full px-3 py-1.5 text-xs text-[var(--color-muted)] transition hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]">{t('nextaction.dismiss', 'Dismiss')}</button>
					</div>
				</div>
			{/if}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

{#if inline && inlineAction}
	{@const InlineIcon = nextActionIcon(inlineAction.icon)}
	<aside class="my-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4">
		<div class="flex items-start justify-between gap-3">
			<div class="min-w-0">
				<p class="flex items-center gap-1.5 text-sm font-semibold text-[var(--color-text)]">
					<InlineIcon size={15} class="shrink-0 text-[var(--color-primary)]" />{inlineAction.title}
				</p>
				{#if inlineAction.body}<p class="mt-1 text-sm text-[var(--color-text-muted)]">{inlineAction.body}</p>{/if}
				<a href={inlineAction.cta_url} onclick={() => cta(inlineAction!)} class="mt-2 inline-block rounded-full bg-[var(--color-primary)] px-3 py-1 text-xs font-medium text-white transition hover:bg-[var(--color-primary-hover)]">{inlineAction.cta_label}</a>
			</div>
			<button onclick={() => act(inlineAction!)} class="shrink-0 rounded-full p-1 text-[var(--color-text-muted)] hover:text-[var(--color-text)]" aria-label={t('nextaction.dismiss_suggestion', 'Dismiss suggestion')}>✕</button>
		</div>
	</aside>
{/if}

<style>
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
