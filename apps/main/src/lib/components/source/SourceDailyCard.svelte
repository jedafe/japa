<script lang="ts">
	import { goto } from '$app/navigation';
	import { ArrowUp, MessageSquare, Eye, ExternalLink, Share2 } from 'lucide-svelte';
	import { t, i18n } from '$lib/i18n';
	import { relativeTime } from '$lib/feed/time';
	import EntityHoverChip from '$lib/components/ui/EntityHoverChip.svelte';
	import type { SourceItem } from '$lib/types/sources';

	interface Props {
		item: SourceItem;
		sourceName?: string;
		sourceLogo?: string | null;
		sourceId?: string;
		sourceSlug?: string;
		entityLabel?: string;
	}

	let { item, sourceName, sourceLogo, sourceId, sourceSlug, entityLabel }: Props = $props();

	const time = $derived(
		item.published_at ? relativeTime(item.published_at, new Date(), i18n.locale) : ''
	);

	function navigate(e: MouseEvent) {
		const target = e.target as HTMLElement | null;
		if (target && (target.closest('a') || target.closest('button'))) return;
		goto(item.href);
	}

	function onKeyDown(e: KeyboardEvent) {
		if (e.key === 'Enter') goto(item.href);
	}

	function formatCount(n: number): string {
		if (n >= 1_000_000) return (n / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M';
		if (n >= 1_000) return (n / 1_000).toFixed(1).replace(/\.0$/, '') + 'k';
		return String(n);
	}

	async function share(e: Event) {
		e.stopPropagation();
		const url = item.original_url || item.href;
		try {
			if (navigator.share) {
				await navigator.share({ title: item.title, url });
			} else if (navigator.clipboard) {
				await navigator.clipboard.writeText(url);
			}
		} catch {
			// user cancelled or unavailable
		}
	}
</script>

<div
	class="group relative flex h-full cursor-pointer flex-col overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-all hover:-translate-y-0.5 hover:shadow-lg"
	role="link"
	tabindex="0"
	onclick={navigate}
	onkeydown={onKeyDown}
>
	<!-- Full-card anchor overlay -->
	<a
		href={item.href}
		class="absolute inset-0 z-0 rounded-[var(--radius-card)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
		aria-label={item.title}
	>
		<span class="sr-only">{item.title}</span>
	</a>

	<!-- Cover / media area -->
	<div class="relative aspect-[4/3] w-full overflow-hidden">
		<div
			class="h-full w-full bg-gradient-to-br from-[var(--color-primary)]/20 via-[var(--color-surface-subtle)] to-[var(--color-surface-subtle)]"
		></div>

		<!-- Entity-type badge overlay -->
		{#if entityLabel}
			<div class="absolute bottom-3 left-3 flex flex-wrap gap-1.5">
				<span
					class="rounded-full bg-black/60 px-2.5 py-1 text-xs font-medium text-white backdrop-blur-sm"
				>
					{entityLabel}
				</span>
			</div>
		{/if}
	</div>

	<!-- Body -->
	<div class="flex flex-1 flex-col gap-2 p-4">
		<h2 class="line-clamp-2 text-[17px] font-semibold leading-snug tracking-tight text-[var(--color-text)]">
			{item.title}
		</h2>

		{#if item.excerpt}
			<p class="line-clamp-2 text-sm leading-relaxed text-[var(--color-text-subtle)]">
				{item.excerpt}
			</p>
		{/if}

		<!-- Footer -->
		<div class="relative z-10 mt-auto flex items-center justify-between border-t border-[var(--color-border)] pt-3">
			<div class="min-w-0 flex-1">
				{#if sourceName && sourceId && sourceSlug}
					<EntityHoverChip
						type="source"
						id={sourceId}
						slug={sourceSlug}
						name={sourceName}
						avatar={sourceLogo}
						variant="row"
						secondary={time}
						class="inline-flex w-fit min-w-0 max-w-full items-center gap-3 text-left"
					/>
				{:else if sourceName}
					<span class="inline-flex items-center gap-2 text-xs text-[var(--color-text-muted)]">
						{#if sourceLogo}
							<img src={sourceLogo} alt="" class="h-5 w-5 rounded object-cover" loading="lazy" />
						{:else}
							<span class="grid h-5 w-5 place-items-center rounded bg-[var(--color-primary)] text-[10px] font-bold text-[var(--color-primary-contrast)]">
								{sourceName.charAt(0).toUpperCase()}
							</span>
						{/if}
						<span class="truncate">{sourceName}</span>
						{#if time}<span>· {time}</span>{/if}
					</span>
				{:else if time}
					<span class="text-xs text-[var(--color-text-muted)]">{time}</span>
				{/if}
			</div>

			<div class="pointer-events-auto relative z-10 flex shrink-0 items-center gap-3 text-xs text-[var(--color-muted)]">
				<span class="inline-flex items-center gap-1" title={t('sources.votes', 'Votes')}>
					<ArrowUp size={14} />
					{formatCount(item.vote_score)}
				</span>
				<span class="inline-flex items-center gap-1" title={t('sources.comments', 'Comments')}>
					<MessageSquare size={14} />
					{formatCount(item.comment_count)}
				</span>
				<span class="inline-flex items-center gap-1" title={t('sources.views', 'Views')}>
					<Eye size={14} />
					{formatCount(item.view_count)}
				</span>
				{#if item.original_url}
					<a
						href={item.original_url}
						target="_blank"
						rel="noopener noreferrer"
						class="inline-flex items-center gap-0.5 hover:text-[var(--color-primary)]"
						onclick={(e) => e.stopPropagation()}
						aria-label={t('sources.original_label', 'Original source')}
					>
						<ExternalLink size={14} />
					</a>
				{/if}
				<button
					type="button"
					onclick={share}
					class="inline-flex items-center gap-0.5 hover:text-[var(--color-primary)]"
					aria-label={t('common.share', 'Share')}
				>
					<Share2 size={14} />
				</button>
			</div>
		</div>
	</div>
</div>
