<script lang="ts">
	/**
	 * Daily.dev-inspired feed card.
	 * Ref: japaconnect-engineering/sections/23-main-site-frontend.md
	 *      docs/superpowers/specs/2026-09-10-daily-feed-style-and-social-post-page-design.md
	 */
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import {
		Bookmark,
		Flag,
		Heart,
		Link,
		MessageSquare,
		MoreVertical,
		Play,
		Share2
	} from 'lucide-svelte';
	import { DropdownMenu } from 'bits-ui';
	import { t, i18n } from '$lib/i18n';
	import { relativeTime } from '$lib/feed/time';
	import { contentTypeMeta, readPostLabel } from '$lib/feed/contentTypeMeta';
	import { contentHref, postHref } from '$lib/utils/contentLink';
	import { formatDuration } from '$lib/utils/duration';
	import { api } from '$lib/api/client';
	import EntityHoverChip from './EntityHoverChip.svelte';
	import type { FeedItem } from '$lib/types';

	interface Props {
		item: FeedItem;
	}

	let { item }: Props = $props();

	let copied = $state(false);
	let reported = $state(false);

	const menuItem =
		'flex w-full items-center gap-2 rounded-[var(--radius)] px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]';

	const time = $derived(relativeTime(item.created_at, new Date(), i18n.locale));
	const duration = $derived(formatDuration(item.duration_secs));
	const isVideo = $derived(item.content_type === 'video' || !!item.video_source_id);
	const socialHref = $derived(postHref(item));
	const canonicalHref = $derived(contentHref(item));
	const actionLabel = $derived(readPostLabel(item.content_type));
	const meta = $derived(contentTypeMeta(item.content_type));
	const Icon = $derived(meta.icon);
	const accentVar = $derived(`var(${meta.accentVar})`);

	const sourceEntity = $derived({
		type: 'user' as const,
		id: item.author_id,
		slug: item.author_username ?? '',
		name: item.author_name ?? t('feed.anonymous', 'Unknown'),
		avatar: item.author_avatar_url
	});

	const regionNames = $derived(
		(() => {
			try {
				return new Intl.DisplayNames([i18n.locale], { type: 'region' });
			} catch {
				return null;
			}
		})()
	);
	function countryName(code?: string | null) {
		if (!code) return '';
		const upper = code.toUpperCase();
		return regionNames?.of(upper) ?? upper;
	}
	const route = $derived(
		item.source_country && item.target_country
			? `${countryName(item.source_country)} → ${countryName(item.target_country)}`
			: item.target_country
				? countryName(item.target_country)
				: item.source_country
					? countryName(item.source_country)
					: null
	);

	const secondary = $derived(
		item.organization
			? `${item.organization.name} • ${time}`
			: route
				? `${route} • ${time}`
				: item.space_name
					? `${item.space_name} • ${time}`
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

	function openPost(e: Event) {
		e.stopPropagation();
		goto(socialHref);
	}

	const shareUrl = $derived(`${page.url.origin}${canonicalHref}`);

	async function sharePost(e: Event) {
		e.stopPropagation();
		const data = { title: item.title, text: item.excerpt ?? '', url: shareUrl };
		if (navigator.share) {
			try {
				await navigator.share(data);
			} catch {
				// User cancelled or share failed — ignore.
			}
		} else {
			await copyPostLink(e);
		}
	}

	async function copyPostLink(e: Event) {
		e.stopPropagation();
		try {
			await navigator.clipboard.writeText(shareUrl);
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch {
			// Clipboard not available — ignore.
		}
	}

	async function reportPost(e: Event) {
		e.stopPropagation();
		const reason = window.prompt(t('feed.report_prompt', 'Why are you reporting this post?'));
		if (!reason) return;
		const res = await api.post<void>(`/content/${item.id}/report`, { reason });
		if (res.error) {
			window.alert(res.error.message ?? t('error.500_body'));
		} else {
			reported = true;
		}
	}
</script>

<div
	class="group relative flex cursor-pointer flex-col gap-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 outline-none transition-colors hover:bg-[color-mix(in_srgb,var(--color-surface-subtle)_40%,transparent)]"
	role="link"
	tabindex="0"
	onclick={navigate}
	onkeydown={onKeyDown}
>
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
		<div class="ml-auto flex shrink-0 items-center gap-3">
			<a
				href={canonicalHref}
				class="flex shrink-0 items-center gap-1 whitespace-nowrap text-xs font-semibold uppercase tracking-wide transition-colors hover:opacity-80"
				style:color={accentVar}
				aria-label={actionLabel}
			>
				<Icon size={13} />
				{actionLabel}
			</a>
			<DropdownMenu.Root>
				<DropdownMenu.Trigger
					class="flex shrink-0 items-center text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
					onclick={(e) => e.stopPropagation()}
					aria-label={t('feed.card.menu', 'More actions')}
				>
					<MoreVertical size={16} />
				</DropdownMenu.Trigger>
				<DropdownMenu.Portal>
					<DropdownMenu.Content
						align="end"
						sideOffset={8}
						class="z-50 min-w-44 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg"
					>
						<DropdownMenu.Item class={menuItem}>
							{#snippet child({ props })}
								<button type="button" onclick={sharePost} {...props}>
									<Share2 size={14} /> {t('feed.card.share', 'Share')}
								</button>
							{/snippet}
						</DropdownMenu.Item>
						<DropdownMenu.Item class={menuItem}>
							{#snippet child({ props })}
								<button type="button" onclick={copyPostLink} {...props}>
									<Link size={14} /> {copied ? t('feed.copied', 'Link copied') : t('feed.card.copy_link', 'Copy link')}
								</button>
							{/snippet}
						</DropdownMenu.Item>
						<DropdownMenu.Item class={menuItem} disabled={reported}>
							{#snippet child({ props })}
								<button type="button" onclick={reportPost} disabled={reported} {...props}>
									<Flag size={14} /> {reported ? t('feed.reported', 'Reported') : t('feed.card.report', 'Report')}
								</button>
							{/snippet}
						</DropdownMenu.Item>
					</DropdownMenu.Content>
				</DropdownMenu.Portal>
			</DropdownMenu.Root>
		</div>
	</div>

	<div class="flex gap-4">
		<div class="flex min-w-0 flex-1 flex-col gap-3">
			<!-- Title -->
			<h2 class="text-balance text-[17px] font-semibold leading-snug tracking-tight text-[var(--color-text)]">
				{item.title}
			</h2>

			{#if item.excerpt}
				<p class="line-clamp-2 text-sm text-[var(--color-text-subtle)]">{item.excerpt}</p>
			{/if}

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
				<button
					type="button"
					class="flex items-center gap-1.5 transition-colors hover:text-[var(--color-text)]"
					onclick={openPost}
					aria-label={t('feed.card.comments', 'Comments')}
				>
					<MessageSquare size={14} />
					{item.comment_count ?? 0}
				</button>
				<button
					type="button"
					class="flex items-center gap-1.5 transition-colors hover:text-[var(--color-primary)]"
					onclick={openPost}
					aria-label={t('feed.card.reactions', 'Reactions')}
				>
					<Heart size={14} />
					{item.reaction_count ?? 0}
				</button>
				<button
					type="button"
					class="flex items-center gap-1.5 transition-colors hover:text-[var(--color-text)]"
					onclick={openPost}
					aria-label={t('feed.card.bookmark', 'Bookmark')}
				>
					<Bookmark size={14} />
				</button>
				<button
					type="button"
					class="flex items-center gap-1.5 transition-colors hover:text-[var(--color-text)]"
					onclick={openPost}
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
</div>
