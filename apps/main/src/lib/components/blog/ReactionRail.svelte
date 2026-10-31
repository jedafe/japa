<script lang="ts">
	import { untrack } from 'svelte';
	import { Bookmark, MessageCircle, Share2, Check, Heart, Flag, MoreVertical } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { getReactionTypes, type ReactionType } from '$lib/api/reactions';
	import { getViewerState, updateViewerState } from '$lib/api/viewerState';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { shareContrastColor } from '$lib/utils/shareColor';
	import ReportDialog from '$lib/components/profile/ReportDialog.svelte';
	import TipButton from '$lib/components/tips/TipButton.svelte';
	import { Menu, MenuItem } from '$lib/components/ui';
	import type { SessionUser } from '$lib/types';

	interface SharePlatform {
		key: string;
		label: string;
		icon: string;
		url_template: string;
		color: string;
	}

	interface ReactionCount {
		reaction_key: string;
		count: number;
	}

	interface Props {
		/** The content ID (UUID) for API calls. */
		contentId: string;
		/** Legacy fallback for aggregate reaction count; not used when platform
		 *  reaction types are loaded. Kept for backward compatibility. */
		voteScore?: number;
		commentCount?: number;
		/** Whether the current user has already bookmarked this content. */
		bookmarked?: boolean;
		/** Auth gating: only allow interactions when true. */
		authed?: boolean;
		/** Horizontal layout for the mobile action bar. Default: vertical rail. */
		horizontal?: boolean;
		/** Fixed bottom bar for mobile (positions popovers above and adds safe-area padding). */
		bottom?: boolean;
		/** Content author — powers the tip affordance that replaced the old
		 *  copy-link button on the horizontal rail (§125). */
		authorUsername?: string | null;
		authorId?: string | null;
		authorName?: string | null;
	}
	let {
		contentId,
		voteScore = 0,
		commentCount = 0,
		bookmarked = false,
		authed = false,
		horizontal = false,
		bottom = false,
		authorUsername,
		authorId,
		authorName
	}: Props = $props();

	const features = $derived((page.data.features as Record<string, boolean> | undefined) ?? {});
	const reactionsEnabled = $derived(features['feature.reactions'] ?? true);

	// "Who reacted" — clicking a reaction row's count expands the reactor
	// list (GET /content/{id}/reactors?key=). dev.to parity.
	type Reactor = { user_id: string; username: string; display_name: string; reaction_key: string };
	const REACTORS_PAGE_SIZE = 10;
	let expandedKey = $state<string | null>(null);
	let reactors = $state<Reactor[]>([]);
	let reactorsLoading = $state(false);

	async function toggleReactorList(key: string, e: Event) {
		e.stopPropagation();
		e.preventDefault();
		if (expandedKey === key) {
			expandedKey = null;
			return;
		}
		expandedKey = key;
		reactorsLoading = true;
		try {
			const res = await fetch(
				`/api/v1/content/${contentId}/reactors?key=${encodeURIComponent(key)}&page_size=${REACTORS_PAGE_SIZE}`
			);
			const json = await res.json();
			reactors = res.ok ? (json.data ?? []) : [];
		} catch {
			reactors = [];
		} finally {
			reactorsLoading = false;
		}
	}
	const bookmarksEnabled = $derived(features['feature.bookmarks'] ?? true);
	const tipsEnabled = $derived(features['tips.enabled'] ?? false);
	const viewerId = $derived((page.data.user as SessionUser | null | undefined)?.id);

	const isHorizontal = $derived(horizontal || bottom);
	const reactionPopoverPosition = $derived(bottom ? 'start-0 bottom-[calc(100%+0.5rem)] sm:bottom-auto sm:start-1/2 sm:top-10 sm:-translate-x-1/2' : (isHorizontal ? 'start-1/2 top-10 -translate-x-1/2' : 'start-12 top-0'));
	const sharePopoverPosition = $derived(bottom ? 'end-0 bottom-[calc(100%+0.5rem)] sm:bottom-auto sm:start-1/2 sm:end-auto sm:top-10 sm:-translate-x-1/2' : (isHorizontal ? 'start-1/2 top-10 -translate-x-1/2' : 'start-12 top-0'));

	let isBookmarked = $state(untrack(() => bookmarked));
	let activeReactions = $state<Set<string>>(new Set());
	let reactionCounts = $state<Record<string, number>>({});
	// Net optimistic deltas applied while the counts fetch is in flight. That
	// snapshot can predate a toggle's POST/DELETE — even one that already
	// completed — because the GET may have been dispatched first, so the
	// deltas must be re-applied on top of whatever the server returns.
	let inflightDeltas: Record<string, number> | null = {};
	let reactionPending = $state<string | null>(null);
	let bookmarkPending = $state(false);
	let shared = $state(false);
	let showShareMenu = $state(false);
	let reportOpen = $state(false);
	let showReactions = $state(false);
	let reactionCloseTimer: ReturnType<typeof setTimeout> | null = null;
	let sharePlatforms = $state<SharePlatform[]>([]);
	const shareIconColor = $derived(sharePlatforms[0]?.color || '');
	let reactionTypes = $state<ReactionType[]>([]);

	// Fetch admin-configured reaction types (Section 14 — reactions are
	// admin-configurable per platform, not hardcoded). Shared cached loader —
	// one HTTP request per page, not one per rail instance.
	$effect(() => {
		if (!reactionsEnabled || !contentId) return;
		getReactionTypes().then((types) => {
			reactionTypes = types;
		});
	});

	// Fetch per-type reaction counts (public since Section 14 counts are not
	// user-scoped) so guest visitors see the same counts as authenticated users.
	// One request per page — this rail is used on single-item pages only.
	$effect(() => {
		if (!reactionsEnabled || !contentId) return;
		(async () => {
			const res = await api.get<ReactionCount[]>(`/content/${contentId}/reactions`);
			const deltas = inflightDeltas;
			inflightDeltas = null;
			if (res.data) {
				const next: Record<string, number> = {};
				for (const c of res.data) {
					next[c.reaction_key] = c.count;
				}
				for (const [k, d] of Object.entries(deltas ?? {})) {
					next[k] = (next[k] ?? 0) + d;
				}
				reactionCounts = next;
			}
		})();
	});

	// Caller's active reactions + bookmark status — batched across all cards on
	// the page into one GET /me/content-states request (see $lib/api/viewerState).
	$effect(() => {
		if (!authed || !contentId) return;
		getViewerState(contentId).then((state) => {
			if (reactionsEnabled) activeReactions = new Set(state.reactionKeys);
			if (bookmarksEnabled) isBookmarked = state.bookmarked;
		});
	});

	// Fetch enabled share platforms (public, no auth needed).
	$effect(() => {
		untrack(() => {});
		(async () => {
			const res = await api.get<SharePlatform[]>('/share-platforms/enabled');
			if (res.data) {
				sharePlatforms = res.data;
			}
		})();
	});

	// Close share menu on outside click.
	$effect(() => {
		if (!showShareMenu) return;
		const handler = (e: MouseEvent) => {
			if (!(e.target as HTMLElement)?.closest('[data-share-menu]')) {
				showShareMenu = false;
			}
		};
		document.addEventListener('click', handler);
		return () => document.removeEventListener('click', handler);
	});

	// Close the reaction picker on outside click — bottom-bar mode only,
	// where the panel is click-toggled like the share menu.
	$effect(() => {
		if (!showReactions || !bottom) return;
		const handler = (e: MouseEvent) => {
			if (!(e.target as HTMLElement)?.closest('[data-reaction-menu]')) {
				showReactions = false;
			}
		};
		document.addEventListener('click', handler);
		return () => document.removeEventListener('click', handler);
	});

	function requireAuth(): boolean {
		if (!authed) {
			goto(`/login?redirect=${encodeURIComponent(page.url.pathname)}`);
			return false;
		}
		return true;
	}

	function countFor(key: string): number {
		return reactionCounts[key] ?? 0;
	}

	async function toggleReaction(reactionKey: string) {
		if (!requireAuth() || reactionPending) return;
		reactionPending = reactionKey;
		const wasActive = activeReactions.has(reactionKey);

		// Optimistic update
		const next = new Set(activeReactions);
		if (wasActive) {
			next.delete(reactionKey);
		} else {
			next.add(reactionKey);
		}
		activeReactions = next;
		const delta = wasActive ? -1 : 1;
		reactionCounts[reactionKey] = (reactionCounts[reactionKey] || 0) + delta;
		if (inflightDeltas) inflightDeltas[reactionKey] = (inflightDeltas[reactionKey] ?? 0) + delta;
		updateViewerState(contentId, { reactionKeys: [...next] });

		try {
			if (wasActive) {
				const res = await api.delete(`/content/${contentId}/react?key=${encodeURIComponent(reactionKey)}`);
				if (res.error) throw new Error(res.error.message);
			} else {
				const res = await api.post(`/content/${contentId}/react`, {
					reaction_key: reactionKey
				});
				if (res.error) throw new Error(res.error.message);
			}
		} catch {
			// Rollback on failure
			const rollback = new Set(activeReactions);
			if (wasActive) {
				rollback.add(reactionKey);
			} else {
				rollback.delete(reactionKey);
			}
			activeReactions = rollback;
			const rollbackDelta = wasActive ? 1 : -1;
			reactionCounts[reactionKey] = (reactionCounts[reactionKey] || 0) + rollbackDelta;
			if (inflightDeltas) inflightDeltas[reactionKey] = (inflightDeltas[reactionKey] ?? 0) + rollbackDelta;
			updateViewerState(contentId, { reactionKeys: [...rollback] });
			reactionPending = null;
			return;
		}
		// No finally: Svelte 5 $state set in finally does not re-render.
		reactionPending = null;
	}

	async function toggleBookmark() {
		if (!bookmarksEnabled || !requireAuth() || bookmarkPending) return;
		bookmarkPending = true;
		const wasBookmarked = isBookmarked;
		isBookmarked = !wasBookmarked;
		updateViewerState(contentId, { bookmarked: !wasBookmarked });

		try {
			if (wasBookmarked) {
				const res = await api.delete(`/content/${contentId}/bookmark`);
				if (res.error) throw new Error(res.error.message);
			} else {
				const res = await api.post(`/content/${contentId}/bookmark`, {});
				if (res.error) throw new Error(res.error.message);
			}
		} catch {
			isBookmarked = wasBookmarked;
			bookmarkPending = false;
			return;
		}
		// No finally: Svelte 5 $state set in finally does not re-render.
		bookmarkPending = false;
	}

	function buildShareUrl(template: string): string {
		const url = encodeURIComponent(window.location.href);
		const title = encodeURIComponent(document.title);
		return template.replace('{url}', url).replace('{title}', title);
	}

	async function shareToPlatform(platform: SharePlatform) {
		const shareUrl = buildShareUrl(platform.url_template);
		window.open(shareUrl, '_blank', 'noopener,noreferrer,width=600,height=500');
		showShareMenu = false;
		shared = true;
		setTimeout(() => (shared = false), 2000);
		api.post(`/content/${contentId}/share`, {
			share_type: 'external',
			platform: platform.key
		});
	}

	async function nativeShare() {
		const url = window.location.href;
		try {
			if (navigator.share) {
				await navigator.share({ url });
				shared = true;
			} else {
				await navigator.clipboard.writeText(url);
				shared = true;
			}
		} catch {
			try {
				await navigator.clipboard.writeText(url);
				shared = true;
			} catch {
				// Both failed — silently ignore
			}
		}
		showShareMenu = false;
		setTimeout(() => (shared = false), 2000);
	}

	function toggleShareMenu() {
		showShareMenu = !showShareMenu;
	}

	// Bottom (mobile) bar: the heart behaves like the share button — tap to
	// open the reaction picker, which stays open until a type is tapped or
	// the user clicks outside. Desktop rail keeps hover/focus behavior.
	function toggleReactionMenu() {
		if (reactionCloseTimer) {
			clearTimeout(reactionCloseTimer);
			reactionCloseTimer = null;
		}
		showReactions = !showReactions;
	}

	// E-010: coarse pointers (touch tablets, large touch screens) have no
	// hover — on the non-bottom layouts the first tap must OPEN the picker,
	// not toggle the primary reaction (same approach as ReactionButton).
	function isCoarsePointer(e: MouseEvent): boolean {
		return (
			(e as PointerEvent).pointerType === 'touch' ||
			(typeof window !== 'undefined' && window.matchMedia('(pointer: coarse)').matches)
		);
	}

	function onHeartClick(e: MouseEvent) {
		if (bottom) {
			toggleReactionMenu();
			return;
		}
		if (isCoarsePointer(e)) {
			openedViaTouch = true;
			openReactions();
			return;
		}
		toggleReaction(primaryKey);
	}

	let openedViaTouch = $state(false);

	function pickReaction(reactionKey: string) {
		toggleReaction(reactionKey);
		if (bottom || openedViaTouch) showReactions = false;
	}

	function openReactions() {
		if (reactionCloseTimer) {
			clearTimeout(reactionCloseTimer);
			reactionCloseTimer = null;
		}
		showReactions = true;
	}

	function closeReactionsSoon() {
		reactionCloseTimer = setTimeout(() => {
			showReactions = false;
		}, 200);
	}

	// The heart button is the primary/shortcut reaction; the full panel of
	// configured reaction types appears on hover or focus (dev.to pattern).
	const primaryReaction = $derived(
		reactionTypes.find((r) => r.key === 'like') ?? reactionTypes[0] ?? { key: 'like', emoji: '👍', label: t('blog.like', 'Like'), enabled: true, position: 0 }
	);
	// Render the configured emoji as the trigger glyph when the primary
	// reaction isn't heart-like — a 'celebrate' primary shouldn't read as ❤.
	const heartLikeKeys = ['like', 'love', 'heart'];
	const useHeartGlyph = $derived(heartLikeKeys.includes(primaryReaction.key));
	const primaryKey = $derived(primaryReaction.key);
	const primaryActive = $derived(activeReactions.has(primaryKey));
	const totalReactions = $derived(
		reactionsEnabled ? reactionTypes.reduce((sum, r) => sum + (reactionCounts[r.key] ?? 0), 0) : voteScore
	);
</script>

<!--
	ADR-058 Exception #1 (vertical rail): borderless, transparent vertical strip in the article's
	LEFT margin, distinct from the 72px nav. "horizontal" re-uses the same controls in a compact
	row for a bottom action bar under the article body. "bottom" renders a fixed bottom bar
	below sm (popovers opening upward, like the shell's mobile menu) and falls back to the
	horizontal strip on sm+.
-->
<div
	class="{bottom
		? 'fixed bottom-0 start-0 end-0 z-50 flex flex-row items-center justify-evenly gap-4 border-t border-[var(--color-border)] bg-[var(--color-surface)]/95 px-4 py-2 pb-[calc(0.5rem+env(safe-area-inset-bottom))] backdrop-blur sm:static sm:z-30 sm:w-full sm:items-stretch sm:justify-start sm:gap-2 sm:rounded-[var(--radius-card)] sm:border sm:bg-[var(--color-surface)] sm:px-3 sm:py-2 sm:pb-2 sm:backdrop-blur-none'
		: horizontal
			? 'relative z-30 flex w-full flex-row items-stretch gap-2 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2'
			: 'sticky top-24 z-40 flex flex-col items-center gap-1.5 py-4'}"
>
	{#if reactionsEnabled}
		<div
			class="relative {isHorizontal ? 'flex flex-1 items-center justify-center' : 'flex flex-col items-center gap-1'}"
			role="group"
			aria-label={t('blog.reactions', 'Reactions')}
			data-reaction-menu
			onmouseenter={bottom ? undefined : openReactions}
			onmouseleave={bottom ? undefined : closeReactionsSoon}
			onfocusin={bottom ? undefined : openReactions}
			onfocusout={bottom ? undefined : closeReactionsSoon}
		>
			<button
				type="button"
				onclick={onHeartClick}
				class="flex {isHorizontal ? 'w-full flex-row items-center justify-center gap-1.5' : 'flex-col items-center gap-1'} transition-colors"
				aria-label={primaryReaction.label}
				aria-pressed={primaryActive}
				aria-expanded={showReactions}
				aria-haspopup="true"
				disabled={reactionPending !== null}
			>
				<span
					class="grid h-9 w-9 place-items-center rounded-full text-lg transition-colors {primaryActive ? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]' : 'text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-primary)]'}"
				>
					{#if useHeartGlyph}
						<Heart size={20} fill={primaryActive ? 'currentColor' : 'none'} />
					{:else}
						<span class="text-lg leading-none">{primaryReaction.emoji}</span>
					{/if}
				</span>
				<span class="{isHorizontal ? 'text-sm' : 'text-[11px]'} font-medium text-[var(--color-text-subtle)] [font-variant-numeric:tabular-nums]">
					{totalReactions > 0 ? totalReactions : ''}
				</span>
			</button>

			{#if showReactions}
				<div
					class="absolute {reactionPopoverPosition} z-50 w-56 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 shadow-lg"
					role="group"
					aria-label={t('blog.reactions', 'Reactions')}
					onmouseenter={bottom ? undefined : openReactions}
					onmouseleave={bottom ? undefined : closeReactionsSoon}
					onfocusin={bottom ? undefined : openReactions}
					onfocusout={bottom ? undefined : closeReactionsSoon}
				>
					<div class="mb-1 px-2 py-1 text-xs font-semibold uppercase text-[var(--color-muted)]">
						{t('blog.add_reaction', 'Add reaction')}
					</div>
					<div class="grid grid-cols-1 gap-1">
						{#each reactionTypes as r (r.key)}
							{@const active = activeReactions.has(r.key)}
							{@const count = countFor(r.key)}
							<button
								type="button"
								onclick={() => pickReaction(r.key)}
								class="flex w-full items-center gap-2 rounded-[var(--radius)] px-2 py-2 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
								aria-label={r.label}
								aria-pressed={active}
								disabled={reactionPending !== null}
							>
								<span class="text-lg">{r.emoji}</span>
								<span class="flex-1 truncate text-start">{r.label}</span>
								{#if count > 0}
									<span
										role="button"
										tabindex="0"
										class="text-xs text-[var(--color-text-subtle)] [font-variant-numeric:tabular-nums] hover:text-[var(--color-primary)] hover:underline"
										onclick={(e) => toggleReactorList(r.key, e)}
										onkeydown={(e) => e.key === 'Enter' && toggleReactorList(r.key, e)}
										title={t('blog.who_reacted', 'Who reacted')}
									>
										{count}
									</span>
								{/if}
							</button>
							{#if expandedKey === r.key}
								<div class="max-h-32 overflow-y-auto rounded-md bg-[var(--color-surface-subtle)] px-2 py-1.5">
									{#if reactorsLoading}
										<p class="text-xs text-[var(--color-text-subtle)]">…</p>
									{:else if reactors.length === 0}
										<p class="text-xs text-[var(--color-text-subtle)]">—</p>
									{:else}
										{#each reactors as u (u.user_id)}
											<div class="truncate py-0.5 text-xs text-[var(--color-text)]">
												{u.display_name || u.username}
											</div>
										{/each}
									{/if}
								</div>
							{/if}
						{/each}
					</div>
				</div>
			{/if}
		</div>
	{/if}

	<a
		href="#comments"
		class="flex {isHorizontal ? 'flex-1 flex-row items-center justify-center gap-1.5' : 'flex-col items-center gap-1'} text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
		aria-label={t('blog.comments', 'Comments')}
	>
		<span class="grid h-9 w-9 place-items-center rounded-full text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-primary)]">
			<MessageCircle size={18} />
		</span>
		<span class="{isHorizontal ? 'text-sm' : 'text-[11px]'} font-medium text-[var(--color-text-subtle)] [font-variant-numeric:tabular-nums]">{commentCount}</span>
	</a>

	<!-- Subtle separator between content actions and utility actions -->
	<div class="{isHorizontal ? 'h-5 w-px' : 'my-1 h-px w-5'} bg-[var(--color-border)]"></div>

	{#if bookmarksEnabled}
		<button
			type="button"
			onclick={toggleBookmark}
			class="flex {isHorizontal ? 'flex-1 items-center justify-center' : 'flex-col items-center gap-1'} transition-colors"
			aria-label={t('blog.bookmark', 'Bookmark')}
			aria-pressed={isBookmarked}
		>
			<span
				class="grid h-9 w-9 place-items-center rounded-full transition-colors {isBookmarked ? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]' : 'text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-primary)]'}"
			>
				<Bookmark size={18} fill={isBookmarked ? 'currentColor' : 'none'} />
			</span>
		</button>
	{/if}

	<!-- Share button with social media dropdown -->
	<div class="relative {isHorizontal ? 'flex flex-1 items-center justify-center' : 'flex flex-col items-center gap-1'}" data-share-menu>
		<button
			type="button"
			onclick={toggleShareMenu}
			class="flex {isHorizontal ? 'w-full items-center justify-center' : 'flex-col items-center gap-1'} text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
			aria-label={t('blog.share', 'Share')}
			aria-expanded={showShareMenu}
		>
			<span
				class="grid h-9 w-9 place-items-center rounded-full text-[var(--share-color,var(--color-primary))] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-primary)]"
				style:--share-color={shareIconColor}
			>
				{#if shared}
					<Check size={18} class="text-[var(--color-success)]" />
				{:else}
					<Share2 size={18} />
				{/if}
			</span>
		</button>

		{#if showShareMenu}
			<div
				class="absolute {sharePopoverPosition} z-50 w-56 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 shadow-lg"
				data-share-menu
			>
				<div class="mb-1 px-2 py-1 text-xs font-semibold uppercase text-[var(--color-muted)]">
					{t('blog.share_to', 'Share to')}
				</div>
				{#each sharePlatforms as platform (platform.key)}
					<button
						type="button"
						onclick={() => shareToPlatform(platform)}
						class="flex w-full items-center gap-3 rounded-[var(--radius)] px-2 py-2 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<span
							class="grid h-6 w-6 shrink-0 place-items-center rounded-full text-xs font-bold"
							style:backgroundColor={platform.color || 'var(--color-muted)'}
							style:color={platform.color ? shareContrastColor(platform.color) : 'var(--color-text)'}
						>
							{platform.label.charAt(0)}
						</span>
						<span class="flex-1 text-start">{platform.label}</span>
					</button>
				{/each}
				<div class="my-1 border-t border-[var(--color-border)]"></div>
				<button
					type="button"
					onclick={nativeShare}
					class="flex w-full items-center gap-3 rounded-[var(--radius)] px-2 py-2 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					<span class="grid h-6 w-6 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-text)]">
						<Share2 size={14} />
					</span>
					<span class="flex-1 text-start">{t('blog.copy_link', 'Copy link / Native share')}</span>
				</button>
			</div>
		{/if}
	</div>

	<!-- Actions overflow (horizontal rail) — always the 5th slot, keeping the
	     rail at exactly five evenly-spaced buttons like the discussion rail.
	     Extras live inside: Tip (when the author is tippable) and Report. -->
	{#if isHorizontal}
		<Menu side="top" contentClass="min-w-40">
			{#snippet trigger(props)}
				<button
					type="button"
					{...props}
					class="flex flex-1 items-center justify-center text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
					aria-label={t('comments.options', 'More options')}
					title={t('comments.options', 'More options')}
				>
					<span class="grid h-9 w-9 place-items-center rounded-full text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-primary)]">
						<MoreVertical size={18} />
					</span>
				</button>
			{/snippet}
			{#if tipsEnabled && authorUsername && viewerId !== authorId}
				<TipButton
					recipientType="user"
					recipientKey={authorUsername}
					recipientId={authorId ?? ''}
					recipientName={authorName ?? authorUsername}
					{contentId}
					menuItem
				/>
			{/if}
			<MenuItem icon={Flag} onSelect={() => { if (requireAuth()) reportOpen = true; }}>{t('report.action', 'Report')}</MenuItem>
		</Menu>
	{:else}
		<!-- Vertical rail — Report stays a visible stacked button (no overflow
		     menu; the vertical strip has room). -->
		<button
			type="button"
			onclick={() => {
				if (!requireAuth()) return;
				reportOpen = true;
			}}
			class="flex flex-col items-center gap-1 text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
			aria-label={t('report.action', 'Report')}
			title={t('report.action', 'Report')}
		>
			<span class="grid h-9 w-9 place-items-center rounded-full text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-primary)]">
				<Flag size={18} />
			</span>
		</button>
	{/if}
</div>

<ReportDialog bind:open={reportOpen} entityType="content" entityId={contentId} />
