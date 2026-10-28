<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Bookmark, Play } from 'lucide-svelte';
	import { t, i18n } from '$lib/i18n';
	import { contentHref } from '$lib/utils/contentLink';
	import { formatDuration } from '$lib/utils/duration';
	import { api } from '$lib/api/client';
	import { getViewerState, updateViewerState } from '$lib/api/viewerState';
	import type { FeedItem } from '$lib/types';
	import EntityHoverChip from '$lib/components/ui/EntityHoverChip.svelte';
	import ReactionButton from '$lib/components/ui/ReactionButton.svelte';
	import ShareButton from '$lib/components/ui/ShareButton.svelte';
	import TipButton from '$lib/components/tips/TipButton.svelte';

	interface Props {
		item: FeedItem;
	}
	let { item }: Props = $props();

	const authed = $derived(!!page.data.isAuthed);
	const canonicalHref = $derived(contentHref(item));
	const shareUrl = $derived(`${page.url.origin}${canonicalHref}`);
	const isVideo = $derived(item.content_type === 'video' || !!item.video_source_id);
	const duration = $derived(formatDuration(item.duration_secs));

	let bookmarked = $state(false);
	let bookmarkPending = $state(false);
	let bookmarkCount = $state(0);
	let bookmarkInitialized = $state(false);

	$effect(() => {
		bookmarkCount = item.bookmark_count ?? 0;
		bookmarked = false;
		bookmarkInitialized = false;
	});

	$effect(() => {
		if (bookmarkInitialized || !authed) return;
		bookmarkInitialized = true;
		// Batched: one GET /me/content-states serves every card on the page.
		getViewerState(item.id).then((state) => {
			bookmarked = state.bookmarked;
		});
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
		// item.tags can repeat a name (same API dup as DailyFeedCard) — dedupe
		// so the keyed each block can't crash on a duplicate key.
		return [...new Set(tags)].slice(0, 2);
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

	async function toggleBookmark(e: Event) {
		e.stopPropagation();
		if (!requireAuth(e) || bookmarkPending) return;
		bookmarkPending = true;
		const next = !bookmarked;
		bookmarked = next;
		bookmarkCount += next ? 1 : -1;
		updateViewerState(item.id, { bookmarked: next });
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
			updateViewerState(item.id, { bookmarked: !next });
			bookmarkPending = false;
			return;
		}
		// No finally: Svelte 5 $state set in finally does not re-render.
		bookmarkPending = false;
	}

	function navigate(e: MouseEvent) {
		const target = e.target as HTMLElement | null;
		if (target && (target.closest('a') || target.closest('button'))) return;
		goto(canonicalHref);
	}

	function onKeyDown(e: KeyboardEvent) {
		// Let Enter on inner links/buttons run their native behavior.
		const target = e.target as HTMLElement | null;
		if (target && (target.closest('a') || target.closest('button'))) return;
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
					<Play size={20} class="ms-0.5" />
				</span>
			</span>
			{#if duration}
				<span
					class="absolute bottom-1.5 end-1.5 rounded bg-black/75 px-1.5 py-0.5 text-[10.5px] font-semibold text-white [font-variant-numeric:tabular-nums]"
				>
					{duration}
				</span>
			{/if}
		{/if}

		<!-- Overlay tags -->
		{#if overlayTags.length}
			<div class="absolute bottom-3 start-3 flex flex-wrap gap-1.5">
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
			class="absolute end-3 top-3 grid h-9 w-9 place-items-center rounded-full bg-black/40 text-white backdrop-blur-sm transition-colors hover:bg-black/60"
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
		<!-- The title is a real anchor so the card is crawlable and supports
		     open-in-new-tab/copy-link — the card div's role="link" + goto()
		     gave none of that (SEO/a11y). navigate() ignores clicks that
		     land on an <a>, so no double navigation. -->
		<h2 class="line-clamp-2 text-[17px] font-semibold leading-snug tracking-tight text-[var(--color-text)]">
			<a href={canonicalHref} class="transition-colors hover:text-[var(--color-primary)]">{item.title}</a>
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
				<ReactionButton
					contentId={item.id}
					count={item.reaction_count ?? 0}
					ariaLabel={t('feed.card.like', 'Like')}
					class="group/btn flex items-center gap-1.5 text-[var(--color-muted)] transition-colors hover:text-[var(--color-primary)]"
					side="top"
					align="center"
				/>

				<ShareButton
					contentId={item.id}
					url={shareUrl}
					title={item.title}
					size={18}
					class="group/btn flex shrink-0 items-center justify-center rounded-full p-2 text-[var(--color-muted)] transition-colors hover:bg-[var(--color-primary)]/10 hover:text-[var(--color-primary)]"
				/>

				{#if item.author_username}
					<TipButton
						recipientType="user"
						recipientKey={item.author_username}
						recipientId={item.author_id}
						recipientName={item.author_name ?? item.author_username}
						contentId={item.id}
						compact
					/>
				{/if}
			</div>
		</div>
	</div>
</div>
