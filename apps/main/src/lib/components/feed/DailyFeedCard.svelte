<script lang="ts">
	/**
	 * Daily.dev-inspired feed card.
	 * Ref: japaconnect-engineering/sections/23-main-site-frontend.md
	 *      docs/superpowers/specs/2026-09-10-daily-feed-style-and-social-post-page-design.md
	 */
	import { goto } from '$app/navigation';
	import { Heart, MessageSquare, Eye, Bookmark, Play, Clock } from 'lucide-svelte';
	import { t, i18n } from '$lib/i18n';
	import { relativeTime } from '$lib/feed/time';
	import { contentTypeMeta, readPostLabel } from '$lib/feed/contentTypeMeta';
	import { contentHref, postHref } from '$lib/utils/contentLink';
	import { formatDuration } from '$lib/utils/duration';
	import EntityHoverChip from './EntityHoverChip.svelte';
	import type { FeedItem } from '$lib/types';

	interface Props {
		item: FeedItem;
	}

	let { item }: Props = $props();

	const meta = $derived(contentTypeMeta(item.content_type));
	const time = $derived(relativeTime(item.created_at, new Date(), i18n.locale));
	const duration = $derived(formatDuration(item.duration_secs));
	const isVideo = $derived(item.content_type === 'video' || !!item.video_source_id);
	const socialHref = $derived(postHref(item));
	const canonicalHref = $derived(contentHref(item));
	const actionLabel = $derived(readPostLabel(item.content_type));
	const accentVar = $derived(`var(${meta.accentVar}, var(--color-primary))`);
	const Icon = $derived(meta.icon);

	function navigate(e: MouseEvent) {
		const target = e.target as HTMLElement | null;
		if (target && (target.closest('a') || target.closest('button'))) return;
		goto(socialHref);
	}

	function onKeyDown(e: KeyboardEvent) {
		if (e.key === 'Enter') goto(socialHref);
	}
</script>

<div
	class="group relative flex cursor-pointer gap-4 px-4 py-3.5 outline-none transition-colors hover:bg-[color-mix(in_srgb,var(--color-surface-subtle)_45%,transparent)]"
	role="link"
	tabindex="0"
	onclick={navigate}
	onkeydown={onKeyDown}
>
	<div class="flex min-w-0 flex-1 flex-col gap-2">
		<!-- Source row + content-type action -->
		<div class="flex items-center gap-2">
			{#if item.organization}
				<EntityHoverChip
					type="organization"
					id={item.organization.id}
					slug={item.organization.slug}
					name={item.organization.name}
					avatar={item.organization.logo_url}
				/>
			{:else if item.space_name && item.space_slug}
				<EntityHoverChip
					type="space"
					id={item.space_id ?? item.space_slug}
					slug={item.space_slug}
					name={item.space_name}
					avatar={item.space_avatar_url}
				/>
			{/if}
			<span class="ml-auto flex items-center gap-1 text-[10.5px] font-semibold uppercase tracking-wide" style:color={accentVar}>
				<Icon size={12} />
				<a
					href={canonicalHref}
					class="hover:underline"
					onclick={(e) => e.stopPropagation()}
				>
					{actionLabel}
				</a>
			</span>
		</div>

		<!-- Title + excerpt -->
		<div class="min-w-0">
			<h2 class="text-balance text-[15.5px] font-semibold leading-snug tracking-tight text-[var(--color-text)] group-hover:text-[var(--color-primary)]">
				{item.title}
			</h2>
			{#if item.excerpt}
				<p class="mt-0.5 line-clamp-2 text-[13.5px] leading-relaxed text-[var(--color-text-subtle)]">
					{item.excerpt}
					{#if item.reading_time_minutes}
						<span class="text-[var(--color-muted)]"> · {item.reading_time_minutes} {t('feed.min_read', 'min read')}</span>
					{/if}
				</p>
			{/if}
		</div>

		<!-- Author + timestamp + reading time -->
		<div class="flex flex-wrap items-center gap-2 text-xs text-[var(--color-muted)]">
			{#if item.author_name && item.author_username}
				<EntityHoverChip
					type="user"
					id={item.author_id}
					slug={item.author_username}
					name={item.author_name}
					avatar={item.author_avatar_url}
				/>
			{:else if item.author_name}
				<span class="font-medium text-[var(--color-text-subtle)]">{item.author_name}</span>
			{/if}
			{#if time}
				<span>·</span>
				<span>{time}</span>
			{/if}
			{#if !item.excerpt && item.reading_time_minutes}
				<span>·</span>
				<span class="flex items-center gap-1"><Clock size={12} /> {item.reading_time_minutes} {t('feed.min_read', 'min read')}</span>
			{/if}
		</div>

		<!-- Metrics -->
		<div class="flex flex-wrap items-center gap-4 text-xs text-[var(--color-muted)] [font-variant-numeric:tabular-nums]">
			<span class="flex items-center gap-1" style:color={accentVar}>
				<Heart size={14} />
				{item.reaction_count ?? 0}
			</span>
			<span class="flex items-center gap-1"><MessageSquare size={14} /> {item.comment_count ?? 0}</span>
			<span class="flex items-center gap-1"><Eye size={14} /> {item.view_count ?? 0}</span>
			{#if item.bookmark_count != null}
				<span class="flex items-center gap-1"><Bookmark size={14} /> {item.bookmark_count}</span>
			{/if}
		</div>
	</div>

	<!-- Cover image (right on desktop, hidden on very small screens) -->
	{#if item.cover_image_url}
		<div class="relative hidden h-24 w-32 shrink-0 overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] sm:block">
			<img src={item.cover_image_url} alt={item.title} loading="lazy" class="h-full w-full object-cover" />
			{#if isVideo}
				<span class="pointer-events-none absolute inset-0 grid place-items-center bg-black/25">
					<Play size={22} class="ml-0.5 text-white" />
				</span>
				{#if duration}
					<span class="absolute bottom-1 right-1 rounded bg-black/75 px-1 py-0.5 text-[10px] font-semibold text-white">{duration}</span>
				{/if}
			{/if}
		</div>
	{:else if isVideo}
		<div class="relative hidden h-24 w-32 shrink-0 place-items-center rounded-[var(--radius)] border border-[var(--color-border)] bg-gradient-to-br from-[var(--color-primary)]/12 via-[var(--color-surface-subtle)] to-[var(--color-surface-subtle)] sm:grid">
			<Play size={22} class="text-[var(--color-muted)]" />
			{#if duration}
				<span class="absolute bottom-1 right-1 rounded bg-black/75 px-1 py-0.5 text-[10px] font-semibold text-white">{duration}</span>
			{/if}
		</div>
	{/if}
</div>
