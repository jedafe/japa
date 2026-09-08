<script lang="ts">
	import { ArrowUp, ArrowDown, MessageSquare, Eye, Bookmark, Play } from 'lucide-svelte';
	import { goto } from '$app/navigation';
	import { relativeTime } from '$lib/feed/time';
	import { t, i18n } from '$lib/i18n';
	import { contentHref } from '$lib/utils/contentLink';
	import { formatDuration } from '$lib/utils/duration';
	import type { CardStyle } from '$lib/feed/cardStyle';
	import type { FeedItem } from '$lib/types';

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
</script>

{#snippet avatar(size: number)}
	{#if item.author_avatar_url}
		<img src={item.author_avatar_url} alt="" loading="lazy" class="shrink-0 rounded-full object-cover" style="width:{size}px;height:{size}px" />
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
		<span class="flex items-center gap-1"><ArrowUp size={14} /> {item.vote_score ?? 0}</span>
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
		{#if item.author_name}<span>·</span><span>{item.author_name}</span>{/if}
		{#if time}<span>·</span><span>{time}</span>{/if}
		{#if item.target_country || item.visa_route || item.journey_stage}
			{#if item.author_name || time}<span>·</span>{/if}
			{@render journeyBadges()}
		{/if}
	</div>
{/snippet}

{#if layout === 'grid-cell' || style === 'grid'}
	<!-- grid card (2-col cell) OR full-width grid item in a stream -->
	<a href={href(item)} class="flex flex-col overflow-hidden {layout === 'grid-cell' ? 'rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]' : ''} hover:bg-[color-mix(in_srgb,var(--color-surface-subtle)_45%,transparent)]">
		{#if item.cover_image_url}
			<span class="relative block">
				<img src={item.cover_image_url} alt={item.title} loading="lazy" class="aspect-video w-full object-cover" />
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
				{@render avatar(20)}
				<span>{item.author_name ?? ''}{#if time} · {time}{/if}</span>
			</div>
			{@render metrics()}
		</div>
	</a>
{:else if style === 'compact'}
	<a href={href(item)} class="flex items-center gap-3 px-4 py-2 hover:bg-[color-mix(in_srgb,var(--color-surface-subtle)_45%,transparent)]">
		<span class="flex w-11 shrink-0 items-center gap-1 text-xs font-semibold text-[var(--color-text-subtle)] [font-variant-numeric:tabular-nums]"><ArrowUp size={13} class="text-[var(--color-muted)]" /> {item.vote_score ?? 0}</span>
		<span class="min-w-0 flex-1 flex flex-col justify-center">
			<span class="truncate text-sm font-medium text-[var(--color-text)]">{item.title}</span>
			{@render journeyBadges()}
		</span>
		{#if item.space_name}<span class="hidden shrink-0 text-xs text-[var(--color-muted)] sm:inline">{item.space_name}{#if time} · {time}{/if}</span>{/if}
		<span class="flex shrink-0 items-center gap-3 text-xs text-[var(--color-muted)] [font-variant-numeric:tabular-nums]">
			<span class="flex items-center gap-1"><MessageSquare size={13} /> {item.comment_count ?? 0}</span>
			<span class="flex items-center gap-1"><Eye size={13} /> {item.view_count ?? 0}</span>
		</span>
	</a>
{:else if style === 'magazine'}
	<a href={href(item)} class="block px-5 py-5 hover:bg-[color-mix(in_srgb,var(--color-surface-subtle)_35%,transparent)]">
		<div class="mb-3.5 flex items-center gap-2.5">
			{@render avatar(34)}
			<span class="flex flex-col leading-tight">
				<span class="text-[13.5px] font-semibold text-[var(--color-text)]">{item.author_name ?? ''}</span>
				<span class="text-xs text-[var(--color-muted)]">{#if item.space_name}{item.space_name} · {/if}{time}{#if item.reading_time_minutes} · {item.reading_time_minutes} {t('feed.min_read', 'min read')}{/if}</span>
			</span>
		</div>
		{#if item.cover_image_url}
			<span class="relative mb-3.5 block">
				<img src={item.cover_image_url} alt={item.title} loading="lazy" class="aspect-video w-full rounded-[var(--radius-card)] border border-[var(--color-border)] object-cover" />
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
	</a>
{:else}
	<!-- reddit (default): vote rail + body + thumbnail -->
	<a href={href(item)} class="flex gap-3 px-4 py-3.5 hover:bg-[color-mix(in_srgb,var(--color-surface-subtle)_45%,transparent)]">
		<span class="flex w-7 shrink-0 flex-col items-center gap-0.5 pt-0.5 text-[var(--color-muted)]">
			<ArrowUp size={18} />
			<span class="text-xs font-semibold text-[var(--color-text-subtle)] [font-variant-numeric:tabular-nums]">{item.vote_score ?? 0}</span>
			<ArrowDown size={18} />
		</span>
		<span class="min-w-0 flex-1">
			{@render metaLine()}
			<h2 class="mb-0.5 mt-1 text-[15.5px] font-semibold leading-snug tracking-tight text-[var(--color-text)]">{item.title}</h2>
			{#if item.excerpt}<p class="mb-2 line-clamp-1 text-[13.5px] text-[var(--color-text-subtle)]">{item.excerpt}{#if item.reading_time_minutes} · {item.reading_time_minutes} {t('feed.min_read', 'min read')}{/if}</p>{/if}
			<div class="flex items-center justify-between gap-3">{@render tags()}{@render metrics()}</div>
		</span>
		{#if item.cover_image_url}
			<span class="relative block h-[84px] w-[84px] shrink-0">
				<img src={item.cover_image_url} alt={item.title} loading="lazy" class="h-full w-full rounded-[var(--radius)] object-cover" />
				{#if isVideo}
					<span class="pointer-events-none absolute inset-0 grid place-items-center rounded-[var(--radius)] bg-black/25">
						<Play size={18} class="text-white" />
					</span>
				{/if}
			</span>
		{/if}
	</a>
{/if}
