<script lang="ts">
	import { goto } from '$app/navigation';
	import { ExternalLink } from 'lucide-svelte';
	import { t, i18n } from '#lib/i18n/index.js';
	import { relativeTime } from '#lib/feed/time.js';
	import EntityHoverChip from '#lib/components/ui/EntityHoverChip.svelte';
	import ReactionButton from '#lib/components/ui/ReactionButton.svelte';
	import ShareButton from '#lib/components/ui/ShareButton.svelte';
	import SourceActions from './SourceActions.svelte';
	import { remoteImageUrl } from '#lib/utils/remoteImage.js';
	import type { Source, SourceItem } from '#lib/types/sources.js';

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
		const target = e.target as HTMLElement | null;
		if (target && (target.closest('a') || target.closest('button'))) return;
		if (e.key === 'Enter') goto(item.href);
	}

	const viewHref = $derived(item.original_url);
	const viewLabel = $derived(
		t('sources.view_content_type', 'View {type}').replace('{type}', entityLabel || item.entity_type)
	);

	function onEdit(updated: Source) {
		Object.assign(source, updated);
	}

	// Share links to the in-app page (the card itself navigates there);
	// the external publisher URL stays on the "View" chip.
	const shareUrl = $derived(item.href || item.original_url);
</script>

<div
	class="group relative flex h-full cursor-pointer flex-row overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-all hover:-translate-y-0.5 hover:shadow-lg sm:flex-col"
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
	     the fallback. On mobile the card is a horizontal row: the cover is
	     a fixed-width rail (hidden without a thumbnail — a bare gradient
	     column reads as a broken image). -->
	<div
		class="relative shrink-0 overflow-hidden sm:aspect-[4/3] sm:w-full {item.thumbnail_url
			? 'w-28'
			: 'hidden'} sm:block"
	>
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
		<div class="absolute end-1.5 top-1.5 z-20 flex items-center gap-2 sm:end-3 sm:top-3">
			{#if viewHref}
				<a
					href={viewHref}
					target="_blank"
					rel="noopener noreferrer"
					class="invisible flex translate-x-2 items-center gap-1.5 rounded-full bg-black/50 px-3 py-1.5 text-xs font-medium text-white opacity-0 backdrop-blur-sm transition-all group-hover:visible group-focus-within:visible group-hover:translate-x-0 group-focus-within:translate-x-0 group-hover:opacity-100 group-focus-within:opacity-100 hover:bg-black/70"
					onclick={(e) => e.stopPropagation()}
				>
					<ExternalLink class="rtl:-scale-x-100" size={14} />
					{viewLabel}
				</a>
			{/if}

			<SourceActions
				{source}
				showBlock={false}
				showShare={false}
				triggerClass="grid h-8 w-8 place-items-center rounded-full border-0 bg-black/40 text-white opacity-90 backdrop-blur-sm transition hover:bg-black/60 hover:opacity-100 focus:opacity-100 sm:h-9 sm:w-9"
				onEdit={onEdit}
			/>
		</div>

		<!-- Entity-type badge overlay -->
		{#if entityLabel}
			<div class="absolute bottom-1.5 start-1.5 flex flex-wrap gap-1.5 sm:bottom-3 sm:start-3">
				<span
					class="rounded-full bg-black/60 px-2.5 py-1 text-xs font-medium text-white backdrop-blur-sm"
				>
					{entityLabel}
				</span>
			</div>
		{/if}
	</div>

	<!-- Body -->
	<div class="flex min-w-0 flex-1 flex-col gap-1.5 p-3 sm:gap-2 sm:p-4">
		<h2 class="line-clamp-2 text-[15px] font-semibold leading-snug tracking-tight text-[var(--color-text)] sm:text-[17px]">
			<!-- relative z-10 keeps the title link above the full-card overlay anchor. -->
			<a href={item.href} class="relative z-10 transition-colors hover:text-[var(--color-primary-text)]">{item.title}</a>
		</h2>

		{#if item.excerpt}
			<p class="line-clamp-2 text-sm leading-relaxed text-[var(--color-text-subtle)]">
				{item.excerpt}
			</p>
		{/if}

		<!-- Footer -->
		<div
			class="relative z-10 mt-auto flex items-center gap-3 border-t border-[var(--color-border)] pt-2 sm:gap-6 sm:pt-3"
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
						class="inline-flex w-fit min-w-0 max-w-full items-center gap-3 text-start"
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
				<!-- The hover-reveal "View" chip over the cover never shows on
				     touch — mirror it as an icon button in the footer on <sm. -->
				{#if viewHref}
					<a
						href={viewHref}
						target="_blank"
						rel="noopener noreferrer"
						aria-label={viewLabel}
						title={viewLabel}
						class="grid h-8 w-8 place-items-center rounded-full text-[var(--color-text-muted)] transition hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)] sm:hidden"
						onclick={(e) => e.stopPropagation()}
					>
						<ExternalLink size={16} class="rtl:-scale-x-100" />
					</a>
				{/if}

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
