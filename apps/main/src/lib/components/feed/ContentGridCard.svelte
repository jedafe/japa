<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Heart, Bookmark, Play } from 'lucide-svelte';
	import { t, i18n } from '$lib/i18n';
	import { contentHref } from '$lib/utils/contentLink';
	import { formatDuration } from '$lib/utils/duration';
	import { api } from '$lib/api/client';
	import type { FeedItem } from '$lib/types';
	import EntityHoverChip from '$lib/components/ui/EntityHoverChip.svelte';
	import ShareMenu from './ShareMenu.svelte';

	interface Props {
		item: FeedItem;
	}
	let { item }: Props = $props();

	const authed = $derived(!!page.data.isAuthed);
	const canonicalHref = $derived(contentHref(item));
	const shareUrl = $derived(`${page.url.origin}${canonicalHref}`);
	const isVideo = $derived(item.content_type === 'video' || !!item.video_source_id);
	const duration = $derived(formatDuration(item.duration_secs));

	let liked = $state(false);
	let likePending = $state(false);
	let reactionCount = $state(0);
	let bookmarked = $state(false);
	let bookmarkPending = $state(false);
	let bookmarkCount = $state(0);
	let engagementInitialized = $state(false);

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
			}
		})();
	});

	function humanizeKey(key?: string | null): string {
		if (!key) return '';
		return key.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}

	const overlayTags = $derived.by((): string[] => {
		const tags: string[] = [];
		if (item.tags?.length) {
			tags.push(...item.tags.slice(0, 2));
		} else if (item.content_type === 'guide') {
			if (item.guide_category) tags.push(humanizeKey(item.guide_category));
			if (item.journey_stage) tags.push(humanizeKey(item.journey_stage));
		} else if (item.content_type === 'story') {
			if (item.source_country && item.target_country) {
				tags.push(`${item.source_country} → ${item.target_country}`);
			} else if (item.target_country) {
				tags.push(item.target_country);
			}
			if (item.visa_route) tags.push(humanizeKey(item.visa_route));
		}
		return tags.slice(0, 2);
	});

	const formattedDate = $derived(
		item.published_at || item.created_at
			? new Intl.DateTimeFormat(i18n.locale, {
					year: 'numeric',
					month: 'short',
					day: 'numeric'
			  }).format(new Date(item.published_at || item.created_at))
			: ''
	);

	function requireAuth(e: Event): boolean {
		if (!authed) {
			e.preventDefault();
			e.stopPropagation();
			goto(`/login?redirect=${encodeURIComponent(page.url.pathname + page.url.search)}`);
			return false;
		}
		return true;
	}

	async function toggleLike(e: Event) {
		e.stopPropagation();
		if (!requireAuth(e) || likePending) return;
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
		if (!requireAuth(e) || bookmarkPending) return;
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

	function navigate(e: MouseEvent) {
		const target = e.target as HTMLElement | null;
		if (target && (target.closest('a') || target.closest('button'))) return;
		goto(canonicalHref);
	}

	function onKeyDown(e: KeyboardEvent) {
		if (e.key === 'Enter') goto(canonicalHref);
	}
</script>

<div
	class="group relative flex cursor-pointer flex-col overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-all hover:-translate-y-0.5 hover:shadow-lg"
	role="link"
	tabindex="0"
	onclick={navigate}
	onkeydown={onKeyDown}
>
	<!-- Cover image -->
	<div class="relative aspect-[4/3] w-full overflow-hidden">
		{#if item.cover_image_url}
			<img
				src={item.cover_image_url}
				alt={item.title}
				loading="lazy"
				class="h-full w-full object-cover transition-transform duration-500 group-hover:scale-105"
			/>
		{:else}
			<div
				class="h-full w-full bg-gradient-to-br from-[var(--color-primary)]/20 via-[var(--color-surface-subtle)] to-[var(--color-surface-subtle)]"
			></div>
		{/if}

		{#if isVideo}
			<span class="pointer-events-none absolute inset-0 grid place-items-center bg-black/25">
				<span class="grid h-11 w-11 place-items-center rounded-full bg-black/60 text-white">
					<Play size={20} class="ml-0.5" />
				</span>
			</span>
			{#if duration}
				<span
					class="absolute bottom-1.5 right-1.5 rounded bg-black/75 px-1.5 py-0.5 text-[10.5px] font-semibold text-white [font-variant-numeric:tabular-nums]"
				>
					{duration}
				</span>
			{/if}
		{/if}

		<!-- Overlay tags -->
		{#if overlayTags.length}
			<div class="absolute bottom-3 left-3 flex flex-wrap gap-1.5">
				{#each overlayTags as tag (tag)}
					<span
						class="rounded-full bg-black/60 px-2.5 py-1 text-xs font-medium text-white backdrop-blur-sm"
					>
						{tag}
					</span>
				{/each}
			</div>
		{/if}

		<!-- Bookmark button -->
		<button
			type="button"
			class="absolute right-3 top-3 grid h-9 w-9 place-items-center rounded-full bg-black/40 text-white backdrop-blur-sm transition-colors hover:bg-black/60"
			class:text-[var(--color-primary)]={bookmarked}
			aria-label={bookmarked ? t('feed.card.bookmarked', 'Bookmarked') : t('feed.card.bookmark', 'Bookmark')}
			aria-pressed={bookmarked}
			disabled={bookmarkPending}
			onclick={toggleBookmark}
		>
			<Bookmark size={18} fill={bookmarked ? 'currentColor' : 'none'} />
		</button>
	</div>

	<!-- Body -->
	<div class="flex flex-1 flex-col gap-3 p-5">
		<h2 class="line-clamp-2 text-[17px] font-semibold leading-snug tracking-tight text-[var(--color-text)]">
			{item.title}
		</h2>

		{#if item.excerpt}
			<p class="line-clamp-2 text-sm leading-relaxed text-[var(--color-text-subtle)]">
				{item.excerpt}
			</p>
		{/if}

		<!-- Footer -->
		<div class="mt-auto flex items-center justify-between border-t border-[var(--color-border)] pt-4">
			<EntityHoverChip
				type="user"
				id={item.author_id}
				slug={item.author_username ?? ''}
				name={item.author_name ?? t('feed.anonymous', 'Unknown')}
				avatar={item.author_avatar_url}
				variant="row"
				secondary={formattedDate}
			/>

			<div class="flex items-center gap-1">
				<button
					type="button"
					class="group/btn flex items-center gap-1.5 transition-colors"
					class:text-[var(--color-primary)]={liked}
					class:text-[var(--color-muted)]={!liked}
					onclick={toggleLike}
					disabled={likePending}
					aria-label={t('feed.card.like', 'Like')}
					aria-pressed={liked}
				>
					<span
						class="rounded-full p-2 transition-colors {liked
							? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
							: 'group-hover/btn:bg-[var(--color-primary)]/10 group-hover/btn:text-[var(--color-primary)]'}"
					>
						<Heart size={18} fill={liked ? 'currentColor' : 'none'} />
					</span>
					{#if reactionCount > 0}
						<span
							class="transition-colors {liked
								? 'text-[var(--color-primary)]'
								: 'group-hover/btn:text-[var(--color-primary)]'}"
						>
							{reactionCount}
						</span>
					{/if}
				</button>

				<ShareMenu contentId={item.id} url={shareUrl} title={item.title} size={18} />
			</div>
		</div>
	</div>
</div>
