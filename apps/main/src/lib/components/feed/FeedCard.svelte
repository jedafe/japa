<script lang="ts">
	import { ArrowUp, ArrowDown, MessageSquare, Eye, Bookmark, Play, ExternalLink, Maximize2 } from 'lucide-svelte';
	import { openLightbox } from '$lib/lightbox.svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { relativeTime } from '$lib/feed/time';
	import { t, i18n } from '$lib/i18n';
	import { contentHref } from '$lib/utils/contentLink';
	import { formatDuration } from '$lib/utils/duration';
	import type { CardStyle } from '$lib/feed/cardStyle';
	import type { FeedItem } from '$lib/types';
	import DailyFeedCard from './DailyFeedCard.svelte';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import EntityHoverChip from '$lib/components/ui/EntityHoverChip.svelte';
	import VoteRail from '$lib/components/forum/VoteRail.svelte';

	interface Props {
		item: FeedItem;
		style: CardStyle;
		layout: 'stream' | 'grid-cell';
	}
	let { item, style, layout }: Props = $props();

	// Content-type pill label. Resolved via i18n with a content_type.{value}
	// key; falls back to a humanized form of the raw type for unknown types.
	// Ref: Guide §7 (i18n), Rule 16 — no hardcoded English in components.
	const typeLabel = (ct: string) => t(`content_type.${ct}`, ct.replace(/_/g, ' '));
	const href = (i: FeedItem) => contentHref(i);
	const time = $derived(relativeTime(item.created_at, new Date(), i18n.locale));
	const initials = $derived((item.author_name ?? '?').slice(0, 2).toUpperCase());
	// §106 video — thumbnail play overlay + duration badge on video cards.
	const isVideo = $derived(item.content_type === 'video' || !!item.video_source_id);
	const duration = $derived(formatDuration(item.duration_secs));

	const itemHref = $derived(href(item));
	// Forum threads vote (Section 08) rather than react — the card renders a
	// functional VoteRail wherever a static score would otherwise show.
	const isForum = $derived(item.content_type === 'forum_thread' || item.content_type === 'discussion');
	const authed = $derived(!!page.data.isAuthed);
	const authorId = $derived(item.author_id ?? '');
	const authorSlug = $derived(item.author_username ?? null);
	const authorName = $derived(item.author_name ?? null);
	const authorAvatar = $derived(item.author_avatar_url ?? null);

	function navigate() {
		if (style !== 'daily') goto(itemHref);
	}
	function onKeyDown(e: KeyboardEvent) {
		if (style !== 'daily' && e.key === 'Enter') {
			e.preventDefault();
			e.stopPropagation();
			goto(itemHref);
		}
	}

	function zoomCover(e: MouseEvent) {
		e.preventDefault();
		e.stopPropagation();
		if (item.cover_image_url) {
			openLightbox([{ id: `cover-${item.id}`, url: item.cover_image_url, alt_text: item.title }]);
		}
	}
</script>

{#snippet coverZoom(sm = false)}
	{#if item.cover_image_url}
		<button
			type="button"
			class="absolute right-1.5 top-1.5 z-10 grid place-items-center rounded-full bg-black/55 text-white opacity-0 transition-opacity hover:bg-black/75 focus-visible:opacity-100 group-hover:opacity-100 {sm ? 'h-6 w-6' : 'h-8 w-8'}"
			onclick={zoomCover}
			onkeydown={(e) => e.stopPropagation()}
			aria-label={t('feed.view_image', 'View image')}
		>
			<Maximize2 size={sm ? 12 : 15} />
		</button>
	{/if}
{/snippet}

{#snippet avatar(size: number)}
	{#if authorAvatar}
		<img src={authorAvatar} alt="" loading="lazy" class="shrink-0 rounded-full object-cover" style="width:{size}px;height:{size}px" />
	{:else}
		<span class="grid shrink-0 place-items-center rounded-full bg-[var(--color-primary)] font-semibold text-[var(--color-primary-contrast)]" style="width:{size}px;height:{size}px;font-size:{Math.round(size * 0.4)}px">{initials}</span>
	{/if}
{/snippet}

{#snippet badge()}
	<span class="inline-block rounded border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10.5px] font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]">{typeLabel(item.content_type)}</span>
	{#if item.content_type === 'guide' && item.guide_category}
		<span class="inline-block rounded border border-[var(--color-primary)]/30 bg-[var(--color-primary)]/10 px-1.5 py-0.5 text-[10.5px] font-semibold uppercase tracking-wide text-[var(--color-primary)]">{item.guide_category.replace(/_/g, ' ')}</span>
	{/if}
{/snippet}

{#snippet tags()}
	{#if item.tags?.length}
		<div class="flex flex-wrap gap-1.5">
			{#each item.tags.slice(0, 3) as tag, i (i)}
				<span class="text-xs font-medium text-[var(--color-primary)]">#{tag}</span>
			{/each}
		</div>
	{/if}
{/snippet}

{#snippet metrics()}
	<div class="flex items-center gap-4 text-xs text-[var(--color-muted)] [font-variant-numeric:tabular-nums]">
		{#if isForum}
			<!-- Reddit style keeps the score in the left rail; other styles put
			     the functional vote control in the metrics row. -->
			{#if style !== 'reddit'}
				<VoteRail
					entityType="thread"
					entityId={item.forum_thread_id ?? ''}
					slug={item.slug}
					score={item.vote_score ?? 0}
					layout="horizontal"
					{authed}
				/>
			{/if}
		{:else}
			<span class="flex items-center gap-1"><ArrowUp size={14} /> {item.vote_score ?? 0}</span>
		{/if}
		<span class="flex items-center gap-1"><MessageSquare size={14} /> {item.comment_count ?? 0}</span>
		<span class="flex items-center gap-1"><Eye size={14} /> {item.view_count ?? 0}</span>
		{#if item.bookmark_count != null}<span class="flex items-center gap-1"><Bookmark size={14} /> {item.bookmark_count}</span>{/if}
	</div>
{/snippet}

{#snippet journeyBadges()}
	{#if item.target_country || item.visa_route || item.journey_stage}
		<div class="flex flex-wrap items-center gap-1.5">
			{#if item.target_country}<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10.5px] font-medium uppercase tracking-wide text-[var(--color-text-subtle)]">{t('feed.card.to', '→')} {item.target_country}</span>{/if}
			{#if item.visa_route}<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10.5px] font-medium uppercase tracking-wide text-[var(--color-text-subtle)]">{item.visa_route}</span>{/if}
			{#if item.journey_stage}<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10.5px] font-medium uppercase tracking-wide text-[var(--color-text-subtle)]">{item.journey_stage}</span>{/if}
		</div>
	{/if}
{/snippet}

{#snippet videoOverlay()}
	{#if isVideo}
		<span class="pointer-events-none absolute inset-0 grid place-items-center bg-black/25">
			<span class="grid h-11 w-11 place-items-center rounded-full bg-black/60 text-white">
				<Play size={20} class="ml-0.5" />
			</span>
		</span>
		{#if duration}
			<span class="absolute bottom-1.5 right-1.5 rounded bg-black/75 px-1.5 py-0.5 text-[10.5px] font-semibold text-white [font-variant-numeric:tabular-nums]">{duration}</span>
		{/if}
	{/if}
{/snippet}

{#snippet metaLine()}
	<div class="flex flex-wrap items-center gap-1.5 text-xs text-[var(--color-muted)]">
		{@render badge()}
		{#if item.space_name}
			{#if item.space_slug}
				<span
					class="font-semibold text-[var(--color-text-subtle)] hover:text-[var(--color-primary)] hover:underline cursor-pointer"
					onclick={(e) => { e.preventDefault(); e.stopPropagation(); goto('/spaces/' + item.space_slug); }}
					onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); e.stopPropagation(); goto('/spaces/' + item.space_slug); } }}
					role="link"
					tabindex="0"
				>{item.space_name}</span>
			{:else}
				<span class="font-semibold text-[var(--color-text-subtle)]">{item.space_name}</span>
			{/if}
		{/if}
		{#if authorName}
			<span>·</span>
			<UserHoverCard
				id={authorId}
				slug={authorSlug}
				name={authorName}
				avatar={authorAvatar}
				variant="row"
				class="inline-flex max-w-full items-center text-left"
			>
				{#snippet trigger()}
					<span>{authorName}</span>
				{/snippet}
			</UserHoverCard>
		{/if}
		{#if time}<span>·</span><span>{time}</span>{/if}
		{#if item.source}
			{@const source = item.source}
			<span>·</span>
			<EntityHoverChip
				type="source"
				id={source.id}
				slug={source.slug}
				name={source.name}
				avatar={source.logo_url}
				class="inline-flex items-center gap-0.5 font-medium text-[var(--color-primary)] hover:underline"
			>
				{#snippet trigger()}
					{t('feed.source', 'Source')}: {source.name}
				{/snippet}
			</EntityHoverChip>
			{#if source.source_url}
				<a
					href={source.source_url}
					target="_blank"
					rel="noopener noreferrer"
					class="inline-flex items-center gap-0.5 text-[var(--color-muted)] hover:text-[var(--color-primary)]"
					onclick={(e) => { e.stopPropagation(); }}
					aria-label={t('feed.original_source', 'View original source')}
				>
					<ExternalLink size={12} />
				</a>
			{/if}
		{/if}
		{#if item.target_country || item.visa_route || item.journey_stage}
			{#if item.author_name || time}<span>·</span>{/if}
			{@render journeyBadges()}
		{/if}
	</div>
{/snippet}

{#if style === 'daily'}
	<DailyFeedCard {item} />
{:else if layout === 'grid-cell' || style === 'grid'}
	<!-- grid card (2-col cell) OR full-width grid item in a stream -->
	<div
		role="link"
		tabindex="0"
		onclick={navigate}
		onkeydown={onKeyDown}
		class="flex flex-col overflow-hidden cursor-pointer {layout === 'grid-cell' ? 'rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]' : ''} hover:bg-[color-mix(in_srgb,var(--color-surface-subtle)_45%,transparent)]"
	>
		{#if item.cover_image_url}
			<span class="group relative block">
				<img src={item.cover_image_url} alt={item.title} loading="lazy" class="aspect-video w-full object-cover" />
				{@render coverZoom()}
				{@render videoOverlay()}
			</span>
		{:else}
			<div class="relative aspect-video w-full bg-gradient-to-br from-[var(--color-primary)]/12 via-[var(--color-surface-subtle)] to-[var(--color-surface-subtle)]">{@render videoOverlay()}</div>
		{/if}
		<div class="flex flex-1 flex-col gap-2 p-3">
			{@render badge()}
			<h2 class="line-clamp-2 text-[18px] font-semibold leading-snug tracking-tight text-[var(--color-text)]">{item.title}</h2>
			{#if item.space_name}
				<span class="text-xs font-medium text-[var(--color-text-subtle)]">{item.space_name}</span>
			{/if}
			{@render journeyBadges()}
			<div class="mt-auto flex items-center gap-2 text-xs text-[var(--color-muted)]">
				{#if authorName}
					<UserHoverCard
						id={authorId}
						slug={authorSlug}
						name={authorName}
						avatar={authorAvatar}
						variant="row"
						class="inline-flex max-w-full items-center gap-2 text-left"
					>
						{#snippet trigger()}
							{@render avatar(20)}
							<span>{authorName}{#if time} · {time}{/if}</span>
						{/snippet}
					</UserHoverCard>
				{:else if time}
					<span>{time}</span>
				{/if}
			</div>
			{@render metrics()}
		</div>
	</div>
{:else if style === 'compact'}
	<div
		role="link"
		tabindex="0"
		onclick={navigate}
		onkeydown={onKeyDown}
		class="flex cursor-pointer items-center gap-3 px-4 py-2 hover:bg-[color-mix(in_srgb,var(--color-surface-subtle)_45%,transparent)]"
	>
		{#if isForum}
			<span class="w-16 shrink-0">
				<VoteRail
					entityType="thread"
					entityId={item.forum_thread_id ?? ''}
					slug={item.slug}
					score={item.vote_score ?? 0}
					layout="horizontal"
					{authed}
				/>
			</span>
		{:else}
			<span class="flex w-11 shrink-0 items-center gap-1 text-xs font-semibold text-[var(--color-text-subtle)] [font-variant-numeric:tabular-nums]"><ArrowUp size={13} class="text-[var(--color-muted)]" /> {item.vote_score ?? 0}</span>
		{/if}
		<span class="min-w-0 flex-1 flex flex-col justify-center">
			<span class="truncate text-sm font-medium text-[var(--color-text)]">{item.title}</span>
			{@render journeyBadges()}
		</span>
		{#if item.space_name}<span class="hidden shrink-0 text-xs text-[var(--color-muted)] sm:inline">{item.space_name}{#if time} · {time}{/if}</span>{/if}
		<span class="flex shrink-0 items-center gap-3 text-xs text-[var(--color-muted)] [font-variant-numeric:tabular-nums]">
			<span class="flex items-center gap-1"><MessageSquare size={13} /> {item.comment_count ?? 0}</span>
			<span class="flex items-center gap-1"><Eye size={13} /> {item.view_count ?? 0}</span>
		</span>
	</div>
{:else if style === 'magazine'}
	<div
		role="link"
		tabindex="0"
		onclick={navigate}
		onkeydown={onKeyDown}
		class="block cursor-pointer px-5 py-5 hover:bg-[color-mix(in_srgb,var(--color-surface-subtle)_35%,transparent)]"
	>
		{#if authorName}
			<UserHoverCard
				id={authorId}
				slug={authorSlug}
				name={authorName}
				avatar={authorAvatar}
				variant="row"
				class="mb-3.5 inline-flex w-full min-w-0 max-w-full items-center gap-2.5 text-left"
			>
				{#snippet trigger()}
					{@render avatar(34)}
					<span class="flex flex-col leading-tight">
						<span class="text-[13.5px] font-semibold text-[var(--color-text)]">{authorName}</span>
						<span class="text-xs text-[var(--color-muted)]">{#if item.space_name}{item.space_name} · {/if}{time}{#if item.reading_time_minutes} · {item.reading_time_minutes} {t('feed.min_read', 'min read')}{/if}</span>
					</span>
				{/snippet}
			</UserHoverCard>
		{/if}
		{#if item.cover_image_url}
			<span class="group relative mb-3.5 block">
				<img src={item.cover_image_url} alt={item.title} loading="lazy" class="aspect-video w-full rounded-[var(--radius-card)] border border-[var(--color-border)] object-cover" />
				{@render coverZoom()}
				{@render videoOverlay()}
			</span>
		{:else}
			<div class="relative mb-3.5 aspect-video w-full rounded-[var(--radius-card)] border border-[var(--color-border)] bg-gradient-to-br from-[var(--color-primary)]/12 via-[var(--color-surface-subtle)] to-[var(--color-surface-subtle)]">{@render videoOverlay()}</div>
		{/if}
		{@render badge()}
		<h2 class="mt-2 mb-2 text-balance font-serif text-[25px] font-medium leading-tight tracking-tight text-[var(--color-text)]">{item.title}</h2>
		{#if item.excerpt}<p class="mb-3.5 line-clamp-2 text-[15px] leading-relaxed text-[var(--color-text-subtle)]">{item.excerpt}</p>{/if}
		{@render journeyBadges()}
		<div class="flex flex-wrap items-center justify-between gap-3.5">{@render tags()}{@render metrics()}</div>
	</div>
{:else}
	<!-- reddit (default): vote rail + body + thumbnail -->
	<div
		role="link"
		tabindex="0"
		onclick={navigate}
		onkeydown={onKeyDown}
		class="flex cursor-pointer gap-3 px-4 py-3.5 hover:bg-[color-mix(in_srgb,var(--color-surface-subtle)_45%,transparent)]"
	>
		{#if isForum}
			<VoteRail
				entityType="thread"
				entityId={item.forum_thread_id ?? ''}
				slug={item.slug}
				score={item.vote_score ?? 0}
				layout="vertical"
				{authed}
			/>
		{:else}
			<span class="flex w-7 shrink-0 flex-col items-center gap-0.5 pt-0.5 text-[var(--color-muted)]">
				<ArrowUp size={18} />
				<span class="text-xs font-semibold text-[var(--color-text-subtle)] [font-variant-numeric:tabular-nums]">{item.vote_score ?? 0}</span>
				<ArrowDown size={18} />
			</span>
		{/if}
		<span class="min-w-0 flex-1">
			{@render metaLine()}
			<h2 class="mb-0.5 mt-1 text-[15.5px] font-semibold leading-snug tracking-tight text-[var(--color-text)]">{item.title}</h2>
			{#if item.excerpt}<p class="mb-2 line-clamp-1 text-[13.5px] text-[var(--color-text-subtle)]">{item.excerpt}{#if item.reading_time_minutes} · {item.reading_time_minutes} {t('feed.min_read', 'min read')}{/if}</p>{/if}
			<div class="flex items-center justify-between gap-3">{@render tags()}{@render metrics()}</div>
		</span>
		{#if item.cover_image_url}
			<span class="group relative block h-[84px] w-[84px] shrink-0">
				<img src={item.cover_image_url} alt={item.title} loading="lazy" class="h-full w-full rounded-[var(--radius)] object-cover" />
				{@render coverZoom(true)}
				{#if isVideo}
					<span class="pointer-events-none absolute inset-0 grid place-items-center rounded-[var(--radius)] bg-black/25">
						<Play size={18} class="text-white" />
					</span>
				{/if}
			</span>
		{/if}
	</div>
{/if}
