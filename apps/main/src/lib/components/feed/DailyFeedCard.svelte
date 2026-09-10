<script lang="ts">
	/**
	 * Daily.dev-inspired feed card.
	 * Ref: japaconnect-engineering/sections/23-main-site-frontend.md
	 *      docs/superpowers/specs/2026-09-10-daily-feed-style-and-social-post-page-design.md
	 */
	import { goto } from '$app/navigation';
	import {
		ArrowDown,
		ArrowUp,
		Bookmark,
		ExternalLink,
		MessageSquare,
		MoreVertical,
		Play,
		Share2
	} from 'lucide-svelte';
	import { t, i18n } from '$lib/i18n';
	import { relativeTime } from '$lib/feed/time';
	import { readPostLabel } from '$lib/feed/contentTypeMeta';
	import { contentHref, postHref } from '$lib/utils/contentLink';
	import { formatDuration } from '$lib/utils/duration';
	import EntityHoverChip from './EntityHoverChip.svelte';
	import type { FeedItem } from '$lib/types';

	interface Props {
		item: FeedItem;
	}

	let { item }: Props = $props();

	const time = $derived(relativeTime(item.created_at, new Date(), i18n.locale));
	const duration = $derived(formatDuration(item.duration_secs));
	const isVideo = $derived(item.content_type === 'video' || !!item.video_source_id);
	const socialHref = $derived(postHref(item));
	const canonicalHref = $derived(contentHref(item));
	const actionLabel = $derived(readPostLabel(item.content_type));

	const sourceEntity = $derived(
		item.organization
			? {
					type: 'organization' as const,
					id: item.organization.id,
					slug: item.organization.slug,
					name: item.organization.name,
					avatar: item.organization.logo_url
				}
			: item.space_name && item.space_slug
				? {
						type: 'space' as const,
						id: item.space_id ?? item.space_slug,
						slug: item.space_slug,
						name: item.space_name,
						avatar: item.space_avatar_url
					}
				: {
						type: 'user' as const,
						id: item.author_id,
						slug: item.author_username ?? '',
						name: item.author_name ?? t('feed.anonymous', 'Unknown'),
						avatar: item.author_avatar_url
					}
	);

	const secondary = $derived(
		item.organization || item.space_name
			? item.author_name
				? `${item.author_name} • ${time}`
				: item.reading_time_minutes
					? `${item.reading_time_minutes} ${t('feed.min_read', 'min read')} • ${time}`
					: time
			: item.reading_time_minutes
				? `${item.reading_time_minutes} ${t('feed.min_read', 'min read')} • ${time}`
				: time
	);

	const visibleTags = $derived(item.tags?.slice(0, 3) ?? []);
	const extraTagCount = $derived(Math.max((item.tags?.length ?? 0) - visibleTags.length, 0));

	function navigate(e: MouseEvent) {
		const target = e.target as HTMLElement | null;
		if (target && (target.closest('a') || target.closest('button'))) return;
		goto(socialHref);
	}

	function onKeyDown(e: KeyboardEvent) {
		if (e.key === 'Enter') goto(socialHref);
	}

	function sharePost(e: Event) {
		e.stopPropagation();
		if (typeof navigator !== 'undefined' && navigator.clipboard) {
			navigator.clipboard.writeText(window.location.origin + canonicalHref).catch(() => {});
		}
	}
</script>

<div
	class="group relative flex cursor-pointer gap-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 outline-none transition-colors hover:bg-[color-mix(in_srgb,var(--color-surface-subtle)_40%,transparent)]"
	role="link"
	tabindex="0"
	onclick={navigate}
	onkeydown={onKeyDown}
>
	<div class="flex min-w-0 flex-1 flex-col gap-3">
		<!-- Source row: avatar + entity name + secondary metadata -->
		<div class="flex items-center gap-2">
			<EntityHoverChip
				type={sourceEntity.type}
				id={sourceEntity.id}
				slug={sourceEntity.slug}
				name={sourceEntity.name}
				avatar={sourceEntity.avatar}
				variant="row"
				secondary={secondary}
			/>
			<div class="ml-auto flex items-center gap-3">
				<a
					href={canonicalHref}
					class="flex shrink-0 items-center gap-1 whitespace-nowrap text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
					onclick={(e) => e.stopPropagation()}
				>
					{actionLabel}
					<ExternalLink size={13} />
				</a>
				<button
					type="button"
					class="text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
					onclick={(e) => e.stopPropagation()}
					aria-label={t('feed.card.menu', 'More actions')}
				>
					<MoreVertical size={16} />
				</button>
			</div>
		</div>

		<!-- Title -->
		<h2 class="text-balance text-[17px] font-semibold leading-snug tracking-tight text-[var(--color-text)]">
			{item.title}
		</h2>

		<!-- Tags -->
		{#if item.tags && item.tags.length > 0}
			<div class="flex flex-wrap items-center gap-2">
				{#each visibleTags as tag (tag)}
					<span class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-2 py-1 text-[11px] font-medium text-[var(--color-text-subtle)]">
						#{tag}
					</span>
				{/each}
				{#if extraTagCount > 0}
					<span class="rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-2 py-1 text-[11px] font-medium text-[var(--color-muted)]">
						+{extraTagCount}
					</span>
				{/if}
			</div>
		{/if}

		<!-- Mobile cover image -->
		{#if item.cover_image_url}
			<div class="relative aspect-video w-full overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] sm:hidden">
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
		{/if}

		<!-- Metrics / actions -->
		<div class="flex flex-wrap items-center gap-5 text-xs text-[var(--color-muted)] [font-variant-numeric:tabular-nums]">
			<span class="flex items-center gap-1.5">
				<MessageSquare size={14} />
				{item.comment_count ?? 0}
			</span>
			<span class="flex items-center gap-1.5">
				<ArrowUp size={14} />
				{item.reaction_count ?? 0}
			</span>
			<span class="flex items-center gap-1.5">
				<ArrowDown size={14} />
			</span>
			<span class="flex items-center gap-1.5">
				<Bookmark size={14} />
			</span>
			<button
				type="button"
				class="flex items-center gap-1.5 transition-colors hover:text-[var(--color-text)]"
				onclick={sharePost}
				aria-label={t('feed.card.share', 'Share')}
			>
				<Share2 size={14} />
			</button>
		</div>
	</div>

	<!-- Cover image (right on desktop) -->
	{#if item.cover_image_url}
		<div class="relative hidden aspect-video w-[220px] shrink-0 overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] sm:block">
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
		<div class="relative hidden aspect-video w-[220px] shrink-0 place-items-center rounded-[var(--radius)] border border-[var(--color-border)] bg-gradient-to-br from-[var(--color-primary)]/12 via-[var(--color-surface-subtle)] to-[var(--color-surface-subtle)] sm:grid">
			<Play size={22} class="text-[var(--color-muted)]" />
			{#if duration}
				<span class="absolute bottom-1 right-1 rounded bg-black/75 px-1 py-0.5 text-[10px] font-semibold text-white">{duration}</span>
			{/if}
		</div>
	{/if}
</div>
