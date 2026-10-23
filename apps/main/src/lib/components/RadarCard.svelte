<script lang="ts">
	// Ref: japaconnect-engineering — Japa Intelligence System spec §7
	// Dashboard widget for the Japa Radar. Mounts on the user dashboard
	// overview and shows a compact summary: a headline count of opportunities
	// in the user's followed countries, three teaser items with deep-links to
	// /radar/feed/{id}, and a click-through to the full /radar page.
	//
	// All data is fetched client-side from the live API on mount (Rule 16):
	//   - GET /intel/radar/counts        — per-country counts (lightweight total)
	//   - GET /intel/radar/followed      — the user's followed-country feed
	// No hardcoded data. Renders nothing when the intelligence system is
	// unavailable (503) or the user is not authenticated — the parent dashboard
	// is already auth-gated.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { Radar, ShieldCheck, ShieldAlert, CalendarClock, ArrowRight } from 'lucide-svelte';
	import type { RadarItem, RadarCountsResponse } from '$lib/types';

	let loading = $state(true);
	let followedItems = $state<RadarItem[]>([]);
	let followedTotal = $state(0);
	let allTotal = $state(0);
	let unavailable = $state(false);

	const teasers = $derived(followedItems.slice(0, 3));
	const hasFollowed = $derived(followedItems.length > 0);

	async function load() {
		loading = true;
		// Counts endpoint (lightweight) — total_new is the global scholarship
		// total shown as a secondary stat. Best-effort: a failure degrades to
		// 0 rather than failing the widget. Ref: HandleRadarCounts returns
		// {total_new, by_type, by_country} (not a per-country array).
		const countsRes = await api.get<RadarCountsResponse>('/intel/radar/counts');
		if (countsRes.data) {
			allTotal = countsRes.data.total_new ?? 0;
		}
		// 503 from either endpoint means the intelligence system is not
		// configured — collapse the widget entirely.
		if (countsRes.error?.status === 503) {
			unavailable = true;
			loading = false;
			return;
		}

		// Followed feed — the user's followed-country opportunities. We request
		// a generous page so the count reflects the real followed total (capped
		// at 100, which is plenty for a dashboard summary); the first three
		// rows become the teaser list.
		//
		// The radar feed uses a non-standard envelope ({items, ...} per spec
		// §739) — the array is under `items`, not `data`. Use api.getEnvelope
		// to read the raw response envelope.
		let followedUnavailable = false;
		try {
			const res = await api.getEnvelope<{ items: RadarItem[] }>('/intel/radar/followed?page_size=100');
			if (res.status === 503) {
				followedUnavailable = true;
			} else if (res.envelope?.items) {
				followedItems = res.envelope.items as RadarItem[];
			}
		} catch {
			// network error — keep empty list
		}
		if (followedUnavailable) {
			unavailable = true;
			loading = false;
			return;
		}
		followedTotal = followedItems.length;
		loading = false;
	}

	function trustVerified(item: RadarItem): boolean {
		return item.trust_level <= 2;
	}

	function deadlineLabel(item: RadarItem): string {
		if (!item.deadline) return '';
		const d = new Date(item.deadline);
		if (isNaN(d.getTime())) return '';
		const days = Math.ceil((d.getTime() - Date.now()) / 86_400_000);
		if (days < 0) return t('radar.detail.deadline_passed');
		return `${days}d`;
	}

	$effect(() => {
		load();
	});
</script>

{#if !unavailable}
	<section
		class="group rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40"
	>
		<header class="mb-3 flex items-center justify-between">
			<h2 class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<Radar size={16} class="text-[var(--color-primary)]" />
				{t('radar.widget.title')}
			</h2>
			<a
				href="/radar"
				class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)] transition-colors hover:text-[var(--color-primary)]"
				aria-label={t('radar.widget.view_all')}
			>
				{t('radar.widget.view_all')}
				<ArrowRight class="rtl:-scale-x-100" size={13} />
			</a>
		</header>

		{#if loading}
			<!-- Skeleton at final dimensions -->
			<div class="space-y-2">
				<div class="h-7 w-3/4 animate-pulse rounded-[var(--radius)] bg-[var(--color-surface-subtle)]"></div>
				<div class="h-12 animate-pulse rounded-[var(--radius)] bg-[var(--color-surface-subtle)]"></div>
				<div class="h-12 animate-pulse rounded-[var(--radius)] bg-[var(--color-surface-subtle)]"></div>
			</div>
		{:else}
			<!-- Headline count -->
			<p class="mb-3 text-sm text-[var(--color-text)]">
				{#if hasFollowed}
					<span class="text-lg font-medium text-[var(--color-primary)]">{followedTotal}</span>
					{t('radar.widget.new_opportunities')}
				{:else if allTotal > 0}
					<span class="text-lg font-medium text-[var(--color-text)]">{allTotal}</span>
					{t('radar.widget.opportunities_all')}
				{:else}
					<span class="text-[var(--color-muted)]">{t('radar.widget.empty')}</span>
				{/if}
			</p>

			<!-- Teaser items -->
			{#if teasers.length > 0}
				<ul class="space-y-2">
					{#each teasers as item (item.id)}
						<li>
							<a
								href={`/radar/feed/${item.id}?item_type=${encodeURIComponent(item.type)}`}
								class="block rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2 transition-colors hover:bg-[var(--color-primary)]/5"
							>
								<div class="flex items-center gap-2">
									<p class="min-w-0 flex-1 truncate text-sm font-medium text-[var(--color-text)]">
										{item.title}
									</p>
									<span
										class="inline-flex shrink-0 items-center gap-1 rounded-full px-1.5 py-0.5 text-[10px] font-medium {trustVerified(item)
											? 'bg-[var(--color-success)]/10 text-[var(--color-success)]'
											: 'bg-[var(--color-warning)]/10 text-[var(--color-warning)]'}"
									>
										{#if trustVerified(item)}
											<ShieldCheck size={10} />
											{t('radar.widget.teaser_verified')}
										{:else}
											<ShieldAlert size={10} />
											{t('radar.widget.teaser_needs_review')}
										{/if}
									</span>
								</div>
								<div class="mt-1 flex items-center gap-3 text-xs text-[var(--color-muted)]">
									<span class="capitalize">{item.country}</span>
									{#if deadlineLabel(item)}
										<span class="inline-flex items-center gap-1">
											<CalendarClock size={11} />
											{deadlineLabel(item)}
										</span>
									{/if}
								</div>
							</a>
						</li>
					{/each}
				</ul>
			{:else if !hasFollowed && allTotal > 0}
				<p class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-3 text-xs text-[var(--color-muted)]">
					{t('radar.widget.followed_none')}
				</p>
			{/if}
		{/if}
	</section>
{/if}
