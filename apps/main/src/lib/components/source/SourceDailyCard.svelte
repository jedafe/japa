<script lang="ts">
	import { goto } from '$app/navigation';
	import { ExternalLink } from 'lucide-svelte';
	import { t, i18n } from '$lib/i18n';
	import { relativeTime } from '$lib/feed/time';
	import EntityHoverChip from '$lib/components/ui/EntityHoverChip.svelte';
	import ReactionButton from '$lib/components/ui/ReactionButton.svelte';
	import ShareButton from '$lib/components/ui/ShareButton.svelte';
	import SourceActions from './SourceActions.svelte';
	import { remoteImageUrl } from '$lib/utils/remoteImage';
	import type { Source, SourceItem } from '$lib/types/sources';

	interface Props {
		item: SourceItem;
		source: Source;
		sourceName?: string;
		sourceLogo?: string | null;
		sourceId?: string;
		sourceSlug?: string;
		entityLabel?: string;
	}

	let {
		item,
		source,
		sourceName,
		sourceLogo,
		sourceId,
		sourceSlug,
		entityLabel
	}: Props = $props();

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

	const viewHref = $derived(item.original_url);
	const viewLabel = $derived(
		t('sources.view_content_type', 'View {type}').replace('{type}', entityLabel || item.entity_type)
	);

	function onEdit(updated: Source) {
		Object.assign(source, updated);
	}

	const shareUrl = $derived(item.original_url || item.href);
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

	<!-- Cover / media area — shows the item's thumbnail when the backend
	     provides one (video sources, posts with og_image); the gradient is
	     the fallback. -->
	<div class="relative aspect-[4/3] w-full overflow-hidden">
		<div
			class="h-full w-full bg-gradient-to-br from-[var(--color-primary)]/20 via-[var(--color-surface-subtle)] to-[var(--color-surface-subtle)]"
		></div>
		{#if item.thumbnail_url}
			<img
				src={remoteImageUrl(item.thumbnail_url)}
				alt=""
				loading="lazy"
				class="pointer-events-none absolute inset-0 h-full w-full object-cover"
			/>
		{/if}

		<!-- Top-right action cluster -->
		<div class="absolute right-3 top-3 z-20 flex items-center gap-2">
			{#if viewHref}
				<a
					href={viewHref}
					target="_blank"
					rel="noopener noreferrer"
					class="invisible flex translate-x-2 items-center gap-1.5 rounded-full bg-black/50 px-3 py-1.5 text-xs font-medium text-white opacity-0 backdrop-blur-sm transition-all group-hover:visible group-focus-within:visible group-hover:translate-x-0 group-focus-within:translate-x-0 group-hover:opacity-100 group-focus-within:opacity-100 hover:bg-black/70"
					onclick={(e) => e.stopPropagation()}
				>
					<ExternalLink size={14} />
					{viewLabel}
				</a>
			{/if}

			<SourceActions
				{source}
				showBlock={false}
				showDelete={true}
				triggerClass="grid h-9 w-9 place-items-center rounded-full border-0 bg-black/40 text-white opacity-90 backdrop-blur-sm transition hover:bg-black/60 hover:opacity-100 focus:opacity-100"
				onEdit={onEdit}
			/>
		</div>

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
		<div
			class="relative z-10 mt-auto flex items-center gap-6 border-t border-[var(--color-border)] pt-3"
		>
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

			<div class="pointer-events-auto relative z-10 flex shrink-0 items-center gap-3">
				<!-- Items without a backing content row (remittance records, video
				     source items, wiki pages, comparisons) have no reaction endpoint —
				     rendering the button would fake a toggle that never persists. -->
				{#if item.content_id}
					<ReactionButton
						contentId={item.content_id}
						count={item.vote_score}
						side="top"
						align="end"
						ariaLabel={t('sources.love', 'Love')}
					/>
				{/if}

				<ShareButton contentId={item.content_id} url={shareUrl} title={item.title} />
			</div>
		</div>
	</div>
</div>
