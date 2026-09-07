<script lang="ts">
	// Ref: japaconnect-engineering/sections/40-platform-analytics-reports.md
	// Owner-only content stats card — rendered on content single pages when
	// the loader resolved GET /content/{id}/analytics for the author.
	import { t } from '$lib/i18n';
	import type { ContentAnalytics } from '$lib/types/content';

	let { stats }: { stats: ContentAnalytics } = $props();

	const days = $derived((stats.daily_stats ?? []).slice(-14));
	const maxViews = $derived(Math.max(1, ...days.map((d) => d.views ?? 0)));
</script>

<section class="mt-8 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
	<h2 class="text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
		{t('content.stats.heading', 'Your content performance — last 30 days')}
	</h2>
	<div class="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-5">
		<div><p class="text-lg font-semibold text-[var(--color-text)]">{stats.views ?? 0}</p><p class="text-xs text-[var(--color-muted)]">{t('content.stats.views', 'Views')}</p></div>
		<div><p class="text-lg font-semibold text-[var(--color-text)]">{stats.unique_viewers ?? 0}</p><p class="text-xs text-[var(--color-muted)]">{t('content.stats.unique_viewers', 'Unique viewers')}</p></div>
		<div><p class="text-lg font-semibold text-[var(--color-text)]">{stats.reactions ?? 0}</p><p class="text-xs text-[var(--color-muted)]">{t('content.stats.reactions', 'Reactions')}</p></div>
		<div><p class="text-lg font-semibold text-[var(--color-text)]">{stats.comments ?? 0}</p><p class="text-xs text-[var(--color-muted)]">{t('content.stats.comments', 'Comments')}</p></div>
		<div><p class="text-lg font-semibold text-[var(--color-text)]">{stats.shares ?? 0}</p><p class="text-xs text-[var(--color-muted)]">{t('content.stats.shares', 'Shares')}</p></div>
	</div>
	{#if days.length > 0}
		<div class="mt-4 flex h-16 items-end gap-1" role="img" aria-label={t('content.stats.views_chart', 'Daily views')}>
			{#each days as d (d.day)}
				<div
					class="flex-1 rounded-sm bg-[var(--color-primary)]/70"
					style="height: {Math.max(4, ((d.views ?? 0) / maxViews) * 100)}%"
					title="{d.day}: {d.views ?? 0} {t('content.stats.views', 'Views')}"
				></div>
			{/each}
		</div>
	{/if}
</section>
