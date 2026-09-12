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
		ChevronRight,
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
	import { contentHref } from '$lib/utils/contentLink';
	import { formatDuration } from '$lib/utils/duration';
	import { api } from '$lib/api/client';
	import EntityHoverChip from '$lib/components/ui/EntityHoverChip.svelte';
	import ShareMenu from './ShareMenu.svelte';
	import ShareMenuItems from './ShareMenuItems.svelte';
	import type { FeedItem } from '$lib/types';

	interface Props {
		item: FeedItem;
	}

	let { item }: Props = $props();

	let copied = $state(false);
	let reported = $state(false);
	let liked = $state(false);
	let bookmarked = $state(false);
	let reactionCount = $state(0);
	let bookmarkCount = $state(0);
	let likePending = $state(false);
	let bookmarkPending = $state(false);
	let engagementInitialized = $state(false);

	const authed = $derived(page.data.isAuthed as boolean);

	const time = $derived(relativeTime(item.created_at, new Date(), i18n.locale));

	$effect(() => {
		reactionCount = item.reaction_count ?? 0;
		bookmarkCount = item.bookmark_count ?? 0;
		liked = false;
		bookmarked = false;
		engagementInitialized = false;
	});

	$effect(() => {
		if (engagementInitialized || !authed) return;
		engagementInitialized = true;
		(async () => {
			const [reactionRes, bookmarkRes] = await Promise.all([
				api.get<{ reaction_key: string }[]>(`/content/${item.id}/reactions/me`),
				api.get<{ bookmarked: boolean }>(`/content/${item.id}/bookmark`)
			]);
			if (reactionRes.data) {
				liked = reactionRes.data.some((r) => r.reaction_key === 'like');
			}
			if (bookmarkRes.data) {
				bookmarked = bookmarkRes.data.bookmarked;
				bookmarkCount = item.bookmark_count ?? 0;
			}
		})();
	});
	const duration = $derived(formatDuration(item.duration_secs));
	const isVideo = $derived(item.content_type === 'video' || !!item.video_source_id);
	const canonicalHref = $derived(contentHref(item));
	const actionLabel = $derived(readPostLabel(item.content_type));
	const meta = $derived(contentTypeMeta(item.content_type));
	const Icon = $derived(meta.icon);

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
		goto(canonicalHref);
	}

	function onKeyDown(e: KeyboardEvent) {
		if (e.key === 'Enter') goto(canonicalHref);
	}

	function openPost(e: Event) {
		e.stopPropagation();
		goto(canonicalHref);
	}

	const shareUrl = $derived(`${page.url.origin}${canonicalHref}`);

	function requireAuth(): boolean {
		if (!authed) {
			goto(`/login?redirect=${encodeURIComponent(page.url.pathname + page.url.search)}`);
			return false;
		}
		return true;
	}

	async function toggleLike(e: Event) {
		e.stopPropagation();
		if (!requireAuth() || likePending) return;
		likePending = true;
		const next = !liked;
		liked = next;
		reactionCount += next ? 1 : -1;
		try {
			if (next) {
				const res = await api.post(`/content/${item.id}/react`, { reaction_key: 'like' });
				if (res.error) throw new Error(res.error.message);
			} else {
				const res = await api.delete(`/content/${item.id}/react?key=like`);
				if (res.error) throw new Error(res.error.message);
			}
		} catch {
			liked = !next;
			reactionCount += next ? -1 : 1;
		} finally {
			likePending = false;
		}
	}

	async function toggleBookmark(e: Event) {
		e.stopPropagation();
		if (!requireAuth() || bookmarkPending) return;
		bookmarkPending = true;
		const next = !bookmarked;
		bookmarked = next;
		bookmarkCount += next ? 1 : -1;
		try {
			if (next) {
				const res = await api.post(`/content/${item.id}/bookmark`, {});
				if (res.error) throw new Error(res.error.message);
			} else {
				const res = await api.delete(`/content/${item.id}/bookmark`);
				if (res.error) throw new Error(res.error.message);
			}
		} catch {
			bookmarked = !next;
			bookmarkCount += next ? -1 : 1;
		} finally {
			bookmarkPending = false;
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
				class="flex shrink-0 items-center gap-1.5 whitespace-nowrap rounded-full bg-[var(--color-primary)]/10 px-2.5 py-1 text-[11px] font-semibold text-[var(--color-primary)] transition-colors hover:bg-[var(--color-primary)]/20"
				aria-label={actionLabel}
			>
				<Icon size={12} />
				{actionLabel}
			</a>
			<DropdownMenu.Root>
				<DropdownMenu.Trigger
					class="flex shrink-0 items-center text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
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
						<DropdownMenu.Sub>
							<DropdownMenu.SubTrigger
								class="flex w-full cursor-pointer items-center gap-2 rounded-[var(--radius)] px-3 py-2 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
							>
								<Share2 size={14} /> {t('feed.card.share', 'Share')}
								<ChevronRight size={14} class="ml-auto" />
							</DropdownMenu.SubTrigger>
							<DropdownMenu.Portal>
								<DropdownMenu.SubContent
									sideOffset={8}
									class="z-50 min-w-48 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-lg"
								>
									<div class="mb-1 px-3 py-1.5 text-xs font-semibold uppercase text-[var(--color-muted)]">
										{t('feed.share_to', 'Share to')}
									</div>
									<ShareMenuItems contentId={item.id} url={shareUrl} title={item.title} />
								</DropdownMenu.SubContent>
							</DropdownMenu.Portal>
						</DropdownMenu.Sub>
						<DropdownMenu.Item
							class="flex w-full cursor-pointer items-center gap-2 rounded-[var(--radius)] px-3 py-2 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)]"
							onclick={copyPostLink}
						>
							<Link size={14} /> {copied ? t('feed.copied', 'Link copied') : t('feed.card.copy_link', 'Copy link')}
						</DropdownMenu.Item>
						<DropdownMenu.Item
							class="flex w-full cursor-pointer items-center gap-2 rounded-[var(--radius)] px-3 py-2 text-sm text-[var(--color-text)] data-[highlighted]:bg-[var(--color-surface-subtle)] data-[disabled]:opacity-50"
							onclick={reportPost}
							disabled={reported}
						>
							<Flag size={14} /> {reported ? t('feed.reported', 'Reported') : t('feed.card.report', 'Report')}
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
			<div class="flex flex-wrap items-center gap-6 text-sm text-[var(--color-muted)] [font-variant-numeric:tabular-nums]">
				<button
					type="button"
					class="group/btn flex items-center gap-2 transition-colors"
					onclick={openPost}
					aria-label={t('feed.card.comments', 'Comments')}
				>
					<span class="rounded-full p-2 transition-colors group-hover/btn:bg-[var(--color-primary)]/10 group-hover/btn:text-[var(--color-primary)]">
						<MessageSquare size={16} />
					</span>
					<span class="transition-colors group-hover/btn:text-[var(--color-primary)]">{item.comment_count ?? 0}</span>
				</button>
				<button
					type="button"
					class="group/btn flex items-center gap-2 transition-colors {liked ? 'text-[var(--color-primary)]' : ''}"
					onclick={toggleLike}
					disabled={likePending}
					aria-label={t('feed.card.like', 'Like')}
					aria-pressed={liked}
				>
					<span class="rounded-full p-2 transition-colors {liked ? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]' : 'group-hover/btn:bg-[var(--color-primary)]/10 group-hover/btn:text-[var(--color-primary)]'}">
						<Heart size={16} fill={liked ? 'currentColor' : 'none'} />
					</span>
					<span class="transition-colors {liked ? 'text-[var(--color-primary)]' : 'group-hover/btn:text-[var(--color-primary)]'}">{reactionCount > 0 ? reactionCount : ''}</span>
				</button>
				<button
					type="button"
					class="group/btn flex items-center transition-colors {bookmarked ? 'text-[var(--color-primary)]' : ''}"
					onclick={toggleBookmark}
					disabled={bookmarkPending}
					aria-label={t('feed.card.bookmark', 'Bookmark')}
					aria-pressed={bookmarked}
				>
					<span class="rounded-full p-2 transition-colors {bookmarked ? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]' : 'group-hover/btn:bg-[var(--color-primary)]/10 group-hover/btn:text-[var(--color-primary)]'}">
						<Bookmark size={16} fill={bookmarked ? 'currentColor' : 'none'} />
					</span>
				</button>
				<ShareMenu contentId={item.id} url={shareUrl} title={item.title} size={16} />
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
