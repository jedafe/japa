<script lang="ts">
	// Badge-earned celebration modal (gamification v2). Polls /me/badges and
	// diffs earned IDs against a localStorage snapshot — any badge earned
	// since the last check pops a congratulatory modal. First run only
	// records the snapshot (no celebration for pre-existing awards).
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { api } from '$lib/api/client';
	import { remoteImageUrl } from '$lib/utils/remoteImage';
	import { realtime } from '$lib/realtime/realtime.svelte';
	import { t } from '$lib/i18n';
	import { Award, X } from 'lucide-svelte';
	import type { BadgeCollection, EarnedBadge, Notification } from '$lib/types';

	const SEEN_KEY = 'jc.seen_badges.v1';
	const POLL_MS = 45_000;

	interface ModalBadge extends EarnedBadge {
		iconUrl?: string;
		points_reward?: number;
	}

	let queue = $state<ModalBadge[]>([]);
	let current = $state<ModalBadge | null>(null);
	let pollTimer: ReturnType<typeof setInterval> | undefined;
	let continueBtn = $state<HTMLButtonElement | null>(null);

	// Move focus into the dialog when it opens (accessibility).
	$effect(() => {
		if (current) continueBtn?.focus();
	});

	const rarityColors: Record<string, string> = {
		common: 'var(--chart-2)',
		uncommon: 'var(--chart-4)',
		rare: 'var(--chart-1)',
		epic: 'var(--chart-5)',
		legendary: '#d97706',
		mythic: '#7c3aed'
	};

	function accent(b: ModalBadge): string {
		return rarityColors[b.rarity ?? ''] ?? 'var(--color-primary)';
	}

	function seenIds(): Set<string> {
		try {
			return new Set(JSON.parse(localStorage.getItem(SEEN_KEY) ?? '[]'));
		} catch {
			return new Set();
		}
	}

	function saveSeen(ids: Set<string>) {
		try {
			localStorage.setItem(SEEN_KEY, JSON.stringify([...ids]));
		} catch {
			/* private mode / quota — the next poll just re-differs */
		}
	}

	async function resolveIcon(b: ModalBadge) {
		// The API now emits icon_url on earned badges — prefer it. The
		// /media/{id} fallback is ownership-gated (uploader or media.view_all
		// only), so it only resolves for the admin who uploaded the artwork.
		if (b.icon_url) {
			b.iconUrl = remoteImageUrl(b.icon_url);
			return;
		}
		if (!b.icon_media_id) return;
		try {
			const res = await api.get<{ url?: string; file_url?: string; cdn_url?: string }>(`/media/${b.icon_media_id}`);
			b.iconUrl = res.data?.url ?? res.data?.file_url ?? res.data?.cdn_url;
		} catch {
			/* icon stays as the Award glyph */
		}
	}

	let inflight = false;

	async function check() {
		if (inflight) return; // overlapping polls could double-queue an award
		inflight = true;
		try {
			const res = await api.get<EarnedBadge[]>('/me/badges');
			const badges = res.data ?? [];
			if (res.error) return;

		const seen = seenIds();
		const firstRun = !localStorage.getItem(SEEN_KEY);

		const fresh = badges.filter((b: EarnedBadge) => !seen.has(b.id));
		for (const b of fresh) seen.add(b.id);
		saveSeen(seen);

		// First-ever snapshot: record, don't celebrate past awards.
		if (firstRun || fresh.length === 0) return;

		// Merge points_reward from the collection (earned list doesn't carry it).
		const rewards = new Map<string, number>();
		try {
			const col = await api.get<BadgeCollection>('/me/badge-collection');
			for (const cat of col.data?.categories ?? [])
				for (const b of cat.badges) rewards.set(b.id, b.points_reward);
		} catch { /* points just won't show */ }

		for (const b of fresh) {
			const mb = { ...b, points_reward: rewards.get(b.id) };
			await resolveIcon(mb);
			queue = [...queue, mb];
		}
		showNext();
		} finally {
			inflight = false;
		}
	}

	function showNext() {
		if (current || queue.length === 0) return;
		const [next, ...rest] = queue;
		queue = rest;
		current = next;
	}

	function dismiss() {
		current = null;
		showNext();
	}

	function onVisible() {
		if (document.visibilityState === 'visible') void check();
	}

	// Instant path — the backend emits gamification.badge_awarded over WS on
	// every award; re-diff on receipt instead of waiting for the poll.
	const onNotif = (n: Partial<Notification> & { id?: string }) => {
		if (n?.notification_type === 'gamification.badge_awarded') void check();
	};

	onMount(() => {
		if (!browser) return;
		void check();
		pollTimer = setInterval(check, POLL_MS);
		document.addEventListener('visibilitychange', onVisible);
		realtime.onNotification(onNotif);
		return () => {
			clearInterval(pollTimer);
			document.removeEventListener('visibilitychange', onVisible);
			realtime.offNotification(onNotif);
		};
	});
</script>

<style>
	@keyframes badge-pop {
		from { transform: scale(0.85); opacity: 0; }
		to { transform: scale(1); opacity: 1; }
	}
</style>

{#if current}
	{@const c = accent(current)}
	<div
		class="fixed inset-0 z-[100] flex items-center justify-center bg-black/50 p-4"
		role="dialog"
		aria-modal="true"
		tabindex="-1"
		onkeydown={(e) => e.key === 'Escape' && dismiss()}
	>
		<button class="absolute inset-0 h-full w-full cursor-default" onclick={dismiss} aria-label={t('common.close', 'Close')}></button>
		<div class="relative w-full max-w-sm animate-[badge-pop_.3s_ease-out] rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center shadow-xl">
			<button onclick={dismiss} class="absolute top-3 right-3 rounded-full p-1 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]" aria-label={t('common.close', 'Close')}>
				<X size={16} />
			</button>

			<div
				class="mx-auto grid h-20 w-20 place-items-center overflow-hidden rounded-full border-2"
				style="border-color: {c}; background: color-mix(in srgb, {c} 14%, transparent)"
			>
				{#if current.iconUrl}
					<img src={current.iconUrl} alt="" class="h-3/4 w-3/4 object-contain" />
				{:else}
					<Award size={38} style="color: {c}" />
				{/if}
			</div>

			<p class="mt-4 text-xs font-bold tracking-widest uppercase" style="color: {c}">
				{t('badges.celebration.title', 'Badge earned!')}
			</p>
			<h2 class="mt-1 text-xl font-extrabold text-[var(--color-text)]">{current.name}</h2>
			{#if current.description}
				<p class="mt-2 text-sm text-[var(--color-text-subtle)]">{current.description}</p>
			{/if}
			{#if current.points_reward}
				<p class="mt-2 text-xs font-semibold" style="color: {c}">+{current.points_reward} {t('badges.points', 'points')}</p>
			{/if}

			<div class="mt-6 flex items-center justify-center gap-2">
				<a href="/badges" class="rounded-full bg-[var(--color-primary)] px-5 py-2 text-sm font-semibold text-white hover:opacity-90">
					{t('badges.celebration.view', 'View collection')}
				</a>
				<button bind:this={continueBtn} onclick={dismiss} class="rounded-full border border-[var(--color-border)] px-5 py-2 text-sm font-semibold text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
					{t('badges.celebration.continue', 'Continue')}
				</button>
			</div>
		</div>
	</div>
{/if}
