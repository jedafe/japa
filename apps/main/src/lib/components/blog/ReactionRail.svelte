<script lang="ts">
	import { untrack } from 'svelte';
	import { Bookmark, MessageCircle, Share2, Check, Heart, Plus } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { compose } from '$lib/components/shell/compose.svelte';

	interface SharePlatform {
		key: string;
		label: string;
		icon: string;
		url_template: string;
		color: string;
	}

	interface ReactionType {
		key: string;
		emoji: string;
		label: string;
		enabled: boolean;
		position: number;
	}

	interface ReactionCount {
		reaction_key: string;
		count: number;
	}

	interface UserReaction {
		reaction_key: string;
	}

	interface Props {
		/** The content ID (UUID) for API calls. */
		contentId: string;
		/** Legacy fallback for aggregate reaction count; not used when tenant
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
	}
	let {
		contentId,
		voteScore = 0,
		commentCount = 0,
		bookmarked = false,
		authed = false,
		horizontal = false,
		bottom = false
	}: Props = $props();

	const features = $derived((page.data.features as Record<string, boolean> | undefined) ?? {});
	const reactionsEnabled = $derived(features['feature.reactions'] ?? true);
	const bookmarksEnabled = $derived(features['feature.bookmarks'] ?? true);

	const isHorizontal = $derived(horizontal || bottom);
	const reactionPopoverPosition = $derived(bottom ? 'left-0 bottom-[calc(100%+0.5rem)]' : (isHorizontal ? 'left-0 top-10' : 'left-12 top-0'));
	const sharePopoverPosition = $derived(bottom ? 'right-0 bottom-[calc(100%+0.5rem)]' : (isHorizontal ? 'left-0 top-10' : 'left-12 top-0'));

	let isBookmarked = $state(untrack(() => bookmarked));
	let activeReactions = $state<Set<string>>(new Set());
	let reactionCounts = $state<Record<string, number>>({});
	let reactionPending = $state<string | null>(null);
	let bookmarkPending = $state(false);
	let shared = $state(false);
	let showShareMenu = $state(false);
	let showReactions = $state(false);
	let reactionCloseTimer: ReturnType<typeof setTimeout> | null = null;
	let sharePlatforms = $state<SharePlatform[]>([]);
	let reactionTypes = $state<ReactionType[]>([]);

	// Fetch admin-configured reaction types (Section 14 — reactions are
	// admin-configurable per tenant, not hardcoded).
	$effect(() => {
		if (!reactionsEnabled || !contentId) return;
		untrack(() => {});
		(async () => {
			const res = await api.get<ReactionType[]>('/reaction-types');
			if (res.data) {
				reactionTypes = res.data
					.filter((r) => r.enabled)
					.sort((a, b) => a.position - b.position);
			}
		})();
	});

	// Fetch per-type reaction counts (public since Section 14 counts are not
	 // user-scoped) so guest visitors see the same counts as authenticated users.
	$effect(() => {
		if (!reactionsEnabled || !contentId) return;
		untrack(() => {});
		(async () => {
			const res = await api.get<ReactionCount[]>(`/content/${contentId}/reactions`);
			if (res.data) {
				const next: Record<string, number> = {};
				for (const c of res.data) {
					next[c.reaction_key] = c.count;
				}
				reactionCounts = next;
			}
		})();
	});

	// Fetch the current user's active reactions and bookmark status.
	$effect(() => {
		if (!authed || !contentId) return;
		untrack(() => {});
		(async () => {
			const reqs: Promise<void>[] = [];
			if (reactionsEnabled) {
				reqs.push(
					api
						.get<UserReaction[]>(`/content/${contentId}/reactions/me`)
						.then((res) => {
							if (res.data) {
								activeReactions = new Set(res.data.map((r) => r.reaction_key));
							}
						})
				);
			}
			if (bookmarksEnabled) {
				reqs.push(
					api
						.get<{ bookmarked: boolean }>(`/content/${contentId}/bookmark`)
						.then((res) => {
							if (res.data) {
								isBookmarked = res.data.bookmarked;
							}
						})
				);
			}
			await Promise.all(reqs);
		})();
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
		reactionCounts[reactionKey] = (reactionCounts[reactionKey] || 0) + (wasActive ? -1 : 1);

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
			reactionCounts[reactionKey] = (reactionCounts[reactionKey] || 0) + (wasActive ? 1 : -1);
		} finally {
			reactionPending = null;
		}
	}

	async function toggleBookmark() {
		if (!bookmarksEnabled || !requireAuth() || bookmarkPending) return;
		bookmarkPending = true;
		const wasBookmarked = isBookmarked;
		isBookmarked = !wasBookmarked;

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
		} finally {
			bookmarkPending = false;
		}
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
	const primaryKey = $derived(primaryReaction.key);
	const primaryActive = $derived(activeReactions.has(primaryKey));
	const totalReactions = $derived(
		reactionsEnabled ? reactionTypes.reduce((sum, r) => sum + (reactionCounts[r.key] ?? 0), 0) : voteScore
	);
</script>

<!-- ADR-058 Exception #1: borderless, transparent vertical strip in the article's LEFT margin.
     Must read as distinct from the bordered 72px nav — separated by a clear gap.
     `horizontal` re-uses the same controls in a compact row for the mobile action bar.
     `bottom` renders a fixed bottom bar with popovers opening upward, like the shell's mobile menu. -->
<div class="{bottom ? 'fixed bottom-0 left-0 right-0 z-50 flex flex-row items-center justify-evenly gap-4 border-t border-[var(--color-border)] bg-[var(--color-surface)]/95 px-4 py-2 pb-[calc(0.5rem+env(safe-area-inset-bottom))] backdrop-blur' : (isHorizontal ? 'flex flex-row items-center gap-4' : 'sticky top-24 flex flex-col items-center gap-1.5 py-4')}">
	{#if reactionsEnabled}
		<div
			class="relative {isHorizontal ? 'flex items-center' : 'flex flex-col items-center gap-1'}"
			role="group"
			aria-label={t('blog.reactions', 'Reactions')}
			onmouseenter={openReactions}
			onmouseleave={closeReactionsSoon}
			onfocusin={openReactions}
			onfocusout={closeReactionsSoon}
		>
			<button
				type="button"
				onclick={() => toggleReaction(primaryKey)}
				class="flex {isHorizontal ? 'flex-row items-center gap-1.5' : 'flex-col items-center gap-1'} transition-colors"
				aria-label={primaryReaction.label}
				aria-pressed={primaryActive}
				aria-expanded={showReactions}
				aria-haspopup="true"
				disabled={reactionPending !== null}
			>
				<span
					class="grid h-9 w-9 place-items-center rounded-full text-lg transition-colors {primaryActive ? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]' : 'text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-primary)]'}"
				>
					<Heart size={20} fill={primaryActive ? 'currentColor' : 'none'} />
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
					onmouseenter={openReactions}
					onmouseleave={closeReactionsSoon}
					onfocusin={openReactions}
					onfocusout={closeReactionsSoon}
				>
					<div class="mb-1 px-2 py-1 text-xs font-semibold uppercase text-[var(--color-muted)]">
						{t('blog.add_reaction', 'Add reaction')}
					</div>
					<div class="grid grid-cols-2 gap-1">
						{#each reactionTypes as r (r.key)}
							{@const active = activeReactions.has(r.key)}
							{@const count = countFor(r.key)}
							<button
								type="button"
								onclick={() => toggleReaction(r.key)}
								class="flex w-full items-center gap-2 rounded-[var(--radius)] px-2 py-2 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
								aria-label={r.label}
								aria-pressed={active}
								disabled={reactionPending !== null}
							>
								<span class="text-lg">{r.emoji}</span>
								<span class="flex-1 truncate text-left">{r.label}</span>
								<span class="text-xs text-[var(--color-text-subtle)] [font-variant-numeric:tabular-nums]">
									{count > 0 ? count : ''}
								</span>
							</button>
						{/each}
					</div>
				</div>
			{/if}
		</div>
	{/if}

	<a
		href="#comments"
		class="flex {isHorizontal ? 'flex-row items-center gap-1.5' : 'flex-col items-center gap-1'} text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
		aria-label={t('blog.comments', 'Comments')}
	>
		<span class="grid h-9 w-9 place-items-center rounded-full hover:bg-[var(--color-surface-subtle)]">
			<MessageCircle size={18} />
		</span>
		<span class="{isHorizontal ? 'text-sm' : 'text-[11px]'} font-medium [font-variant-numeric:tabular-nums]">{commentCount}</span>
	</a>

	{#if isHorizontal}
		{#if authed}
			<button
				type="button"
				onclick={() => compose.openMenu()}
				class="flex flex-col items-center gap-0.5 text-[var(--color-primary)] transition-colors"
				aria-label={t('nav.compose', 'Compose')}
			>
				<span
					class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] shadow-sm transition-colors hover:bg-[var(--color-primary-hover)]"
				>
					<Plus size={20} />
				</span>
			</button>
		{:else}
			<a
				href="/login?redirect=/create"
				class="flex flex-col items-center gap-0.5 text-[var(--color-primary)] transition-colors"
				aria-label={t('nav.compose', 'Compose')}
			>
				<span
					class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] shadow-sm transition-colors hover:bg-[var(--color-primary-hover)]"
				>
					<Plus size={20} />
				</span>
			</a>
		{/if}
	{/if}

	<!-- Subtle separator between content actions and utility actions -->
	<div class="{isHorizontal ? 'hidden' : 'my-1 h-px w-5'} bg-[var(--color-border)]"></div>

	{#if bookmarksEnabled}
		<button
			type="button"
			onclick={toggleBookmark}
			class="flex {isHorizontal ? 'items-center' : 'flex-col items-center gap-1'} transition-colors"
			aria-label={t('blog.bookmark', 'Bookmark')}
			aria-pressed={isBookmarked}
		>
			<span
				class="grid h-9 w-9 place-items-center rounded-full transition-colors {isBookmarked ? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]' : 'text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]'}"
			>
				<Bookmark size={18} fill={isBookmarked ? 'currentColor' : 'none'} />
			</span>
		</button>
	{/if}

	<!-- Share button with social media dropdown -->
	<div class="relative {isHorizontal ? 'flex items-center' : 'flex flex-col items-center gap-1'}" data-share-menu>
		<button
			type="button"
			onclick={toggleShareMenu}
			class="flex {isHorizontal ? 'items-center' : 'flex-col items-center gap-1'} text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
			aria-label={t('blog.share', 'Share')}
			aria-expanded={showShareMenu}
		>
			<span class="grid h-9 w-9 place-items-center rounded-full transition-colors hover:bg-[var(--color-surface-subtle)]">
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
							class="grid h-6 w-6 shrink-0 place-items-center rounded-full text-xs font-bold text-[var(--color-primary-contrast)]"
							style="background-color: {platform.color || 'var(--color-muted)'}"
						>
							{platform.label.charAt(0)}
						</span>
						<span class="flex-1 text-left">{platform.label}</span>
					</button>
				{/each}
				<div class="my-1 border-t border-[var(--color-border)]"></div>
				<button
					type="button"
					onclick={nativeShare}
					class="flex w-full items-center gap-3 rounded-[var(--radius)] px-2 py-2 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					<span class="grid h-6 w-6 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]">
						<Share2 size={14} />
					</span>
					<span class="flex-1 text-left">{t('blog.copy_link', 'Copy link / Native share')}</span>
				</button>
			</div>
		{/if}
	</div>
</div>
