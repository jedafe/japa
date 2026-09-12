<script lang="ts">
	import { t } from '$lib/i18n';
	import { ExternalLink, Eye, MessageSquare, ArrowBigUp } from 'lucide-svelte';
	import type { SourceItem } from '$lib/types/sources';

	interface Props {
		item: SourceItem;
		sourceName?: string;
		sourceLogo?: string | null;
		entityLabel?: string;
	}

	let { item, sourceName, sourceLogo, entityLabel }: Props = $props();

	function formatCount(n: number): string {
		if (n >= 1_000_000) return (n / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M';
		if (n >= 1_000) return (n / 1_000).toFixed(1).replace(/\.0$/, '') + 'k';
		return String(n);
	}

	const publishedAt = $derived(
		item.published_at ? new Date(item.published_at).toLocaleDateString() : ''
	);
</script>

<article
	class="group relative flex h-full flex-col gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-shadow hover:shadow-md"
>
	<a
		href={item.href}
		class="absolute inset-0 z-0 rounded-[var(--radius-card)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
		aria-label={item.title}
	>
		<span class="sr-only">{item.title}</span>
	</a>

	<!-- Source attribution row -->
	{#if sourceName}
		<div class="pointer-events-none relative z-10 flex items-center gap-2 text-xs text-[var(--color-text-muted)]">
			{#if sourceLogo}
				<img src={sourceLogo} alt="" class="h-5 w-5 rounded object-cover" loading="lazy" />
			{:else}
				<span class="grid h-5 w-5 place-items-center rounded bg-[var(--color-primary)] text-[10px] font-bold text-[var(--color-primary-contrast)]">
					{sourceName.charAt(0).toUpperCase()}
				</span>
			{/if}
			<span class="truncate">{sourceName}</span>
		</div>
	{/if}

	<!-- Entity type badge -->
	<div class="pointer-events-none relative z-10 flex items-center gap-2">
		<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10.5px] font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]">
			{entityLabel ?? item.entity_type.replace(/_/g, ' ')}
		</span>
		<span class="text-xs text-[var(--color-text-muted)]">{publishedAt}</span>
	</div>

	<!-- Title + excerpt -->
	<div class="pointer-events-none relative z-10 flex min-w-0 flex-1 flex-col gap-1.5">
		<h3 class="line-clamp-2 text-base font-semibold leading-snug text-[var(--color-text)] group-hover:text-[var(--color-primary)]">
			{item.title}
		</h3>
		{#if item.excerpt}
			<p class="line-clamp-2 text-sm text-[var(--color-text-subtle)]">{item.excerpt}</p>
		{/if}
	</div>

	<!-- Metrics + original link -->
	<div class="relative z-10 mt-auto flex flex-wrap items-center justify-between gap-2 pt-2 text-xs text-[var(--color-text-muted)]">
		<div class="pointer-events-none flex flex-wrap items-center gap-3">
			<span class="inline-flex items-center gap-1" title={t('sources.views', 'Views')}>
				<Eye class="h-3.5 w-3.5" />
				{formatCount(item.view_count)}
			</span>
			<span class="inline-flex items-center gap-1" title={t('sources.comments', 'Comments')}>
				<MessageSquare class="h-3.5 w-3.5" />
				{formatCount(item.comment_count)}
			</span>
			<span class="inline-flex items-center gap-1" title={t('sources.votes', 'Votes')}>
				<ArrowBigUp class="h-3.5 w-3.5" />
				{formatCount(item.vote_score)}
			</span>
		</div>
		{#if item.original_url}
			<a
				href={item.original_url}
				target="_blank"
				rel="noopener noreferrer"
				class="pointer-events-auto relative z-20 inline-flex items-center gap-0.5 hover:text-[var(--color-primary)] hover:underline"
				aria-label={t('sources.original_label', 'Original source')}
			>
				{t('sources.original', 'Original')}
				<ExternalLink class="h-3 w-3" />
			</a>
		{/if}
	</div>
</article>
