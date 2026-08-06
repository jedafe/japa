<script lang="ts">
	// TrackCardWidget — admin-managed sidebar widget (Section 59, ADR-066).
	// Shows a featured track card with thumbnail, title, level, duration, and
	// a link to the track. The track slug comes from the widget's props
	// (configured by an admin in the Sidebar Management System). If no slug is
	// configured, falls back to the first track from GET /learn/tracks so the
	// widget is never stuck at "Loading track…".
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { BookOpen, Clock, BarChart3 } from 'lucide-svelte';
	import LevelBadge from '$lib/components/learn/LevelBadge.svelte';
	import { formatDuration } from '$lib/format';
	import type { TrackSummary } from '$lib/types';

	interface Props {
		title?: string;
		trackSlug?: string;
	}
	let { title = '', trackSlug = '' }: Props = $props();

	let track = $state<TrackSummary | null>(null);
	let loading = $state(true);
	let error = $state<string | null>(null);

	$effect(() => {
		(async () => {
			loading = true;
			error = null;
			// If a track slug is configured, fetch that specific track.
			// Otherwise, fall back to the first track from the list so the
			// widget shows something useful instead of "Loading track…".
			if (trackSlug) {
				const r = await api.get<TrackSummary>(`/learn/university/tracks/${trackSlug}`);
				loading = false;
				if (r.data) {
					track = r.data;
				} else if (r.error) {
					error = r.error.message;
				}
			} else {
				const r = await api.get<TrackSummary[]>('/learn/university/catalog');
				loading = false;
				if (r.data && r.data.length > 0) {
					track = r.data[0];
				}
			}
		})();
	});
</script>

<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
	<div class="mb-3 flex items-center gap-2">
		<BookOpen size={18} class="text-[var(--color-primary)]" />
		<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('learn.featured_track', 'Featured Track')}</h2>
	</div>
	{#if loading}
		<div class="space-y-2">
			<div class="jc-skeleton h-20 w-full rounded"></div>
			<div class="jc-skeleton h-4 w-3/4 rounded"></div>
			<div class="jc-skeleton h-3 w-1/2 rounded"></div>
		</div>
	{:else if track}
		<a
			href="/tracks/{track.slug}"
			class="block rounded-[var(--radius)] p-2 hover:bg-[var(--color-surface-subtle)]"
		>
			{#if track.thumbnail_url}
				<img
					src={track.thumbnail_url}
					alt={track.title}
					class="mb-2 h-20 w-full rounded-[var(--radius)] object-cover"
				/>
			{/if}
			<p class="truncate text-sm font-medium text-[var(--color-text)]">{track.title}</p>
			<div class="mt-1.5 flex items-center gap-2 text-xs text-[var(--color-muted)]">
				{#if track.level}
					<LevelBadge level={track.level} />
				{/if}
				{#if track.duration_label}
					<span class="flex items-center gap-1">
						<Clock size={12} />
						{track.duration_label}
					</span>
				{/if}
			</div>
			{#if track.subtitle}
				<p class="mt-2 line-clamp-2 text-xs text-[var(--color-text-subtle)]">{track.subtitle}</p>
			{/if}
		</a>
	{:else}
		<p class="text-xs text-[var(--color-muted)]">{t('common.empty', 'No tracks available')}</p>
	{/if}
</section>
