<script lang="ts">
	import { untrack } from 'svelte';
	import { Bookmark, MessageCircle, Share2, Heart, Check } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';

	interface SharePlatform {
		key: string;
		label: string;
		icon: string;
		url_template: string;
		color: string;
	}

	interface Props {
		/** The content ID (UUID) for API calls. */
		contentId: string;
		/** Initial aggregate reshare/like count from the server. */
		voteScore?: number;
		commentCount?: number;
		/** Whether the current user has already bookmarked this content. */
		bookmarked?: boolean;
		/** Auth gating: only allow interactions when true. */
		authed?: boolean;
	}
	let {
		contentId,
		voteScore = 0,
		commentCount = 0,
		bookmarked = false,
		authed = false
	}: Props = $props();

	let isBookmarked = $state(untrack(() => bookmarked));
	let isLiked = $state(false);
	let likeCount = $state(untrack(() => voteScore));
	let likePending = $state(false);
	let bookmarkPending = $state(false);
	let shared = $state(false);
	let showShareMenu = $state(false);
	let sharePlatforms = $state<SharePlatform[]>([]);

	// Fetch initial bookmark + like status on mount when authenticated.
	$effect(() => {
		if (!authed || !contentId) return;
		(async () => {
			const [bmRes, reactRes] = await Promise.all([
				api.get<{ bookmarked: boolean }>(`/content/${contentId}/bookmark`),
				api.get<{ reaction_key: string }[]>(`/content/${contentId}/reactions/me`)
			]);
			if (bmRes.data) {
				isBookmarked = bmRes.data.bookmarked;
			}
			if (reactRes.data) {
				isLiked = reactRes.data.some((r) => r.reaction_key === 'like');
			}
		})();
	});

	// Fetch enabled share platforms (public, no auth needed).
	$effect(() => {
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

	async function toggleLike() {
		if (!requireAuth() || likePending) return;
		likePending = true;
		const wasLiked = isLiked;
		// Optimistic update
		isLiked = !wasLiked;
		likeCount += isLiked ? 1 : -1;

		try {
			if (wasLiked) {
				const res = await api.delete(`/content/${contentId}/reactions/like`);
				if (res.error) throw new Error(res.error.message);
			} else {
				const res = await api.post(`/content/${contentId}/reactions`, {
					reaction_key: 'like'
				});
				if (res.error) throw new Error(res.error.message);
			}
		} catch {
			// Rollback on failure
			isLiked = wasLiked;
			likeCount += wasLiked ? 1 : -1;
		} finally {
			likePending = false;
		}
	}

	async function toggleBookmark() {
		if (!requireAuth() || bookmarkPending) return;
		bookmarkPending = true;
		const wasBookmarked = isBookmarked;
		// Optimistic update
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
			// Rollback on failure
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

	function shareToPlatform(platform: SharePlatform) {
		const shareUrl = buildShareUrl(platform.url_template);
		window.open(shareUrl, '_blank', 'noopener,noreferrer,width=600,height=500');
		showShareMenu = false;
		shared = true;
		setTimeout(() => (shared = false), 2000);
	}

	async function nativeShare() {
		const url = window.location.href;
		try {
			if (navigator.share) {
				await navigator.share({ url });
				shared = true;
				setTimeout(() => (shared = false), 2000);
			} else {
				await navigator.clipboard.writeText(url);
				shared = true;
				setTimeout(() => (shared = false), 2000);
			}
		} catch {
			try {
				await navigator.clipboard.writeText(url);
				shared = true;
				setTimeout(() => (shared = false), 2000);
			} catch {
				// Both failed — silently ignore
			}
		}
		showShareMenu = false;
	}

	function toggleShareMenu() {
		showShareMenu = !showShareMenu;
	}
</script>

<!-- ADR-058 Exception #1: borderless, transparent vertical strip in the article's LEFT margin.
     Must read as distinct from the bordered 72px nav — separated by a clear gap. -->
<div class="sticky top-24 flex flex-col items-center gap-4 py-4">
	<button
		type="button"
		onclick={toggleLike}
		class="flex flex-col items-center gap-1 transition-colors"
		aria-label={t('blog.react', 'React')}
		aria-pressed={isLiked}
	>
		<span class="grid h-10 w-10 place-items-center rounded-full transition-colors {isLiked ? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]' : 'text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-primary)]'}">
			<Heart size={20} fill={isLiked ? 'currentColor' : 'none'} />
		</span>
		<span class="text-xs font-medium text-[var(--color-text-subtle)] [font-variant-numeric:tabular-nums]">{likeCount}</span>
	</button>

	<a
		href="#comments"
		class="flex flex-col items-center gap-1 text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
		aria-label={t('blog.comments', 'Comments')}
	>
		<span class="grid h-10 w-10 place-items-center rounded-full hover:bg-[var(--color-surface-subtle)]">
			<MessageCircle size={20} />
		</span>
		<span class="text-xs font-medium [font-variant-numeric:tabular-nums]">{commentCount}</span>
	</a>

	<button
		type="button"
		onclick={toggleBookmark}
		class="flex flex-col items-center gap-1 transition-colors"
		aria-label={t('blog.bookmark', 'Bookmark')}
		aria-pressed={isBookmarked}
	>
		<span class="grid h-10 w-10 place-items-center rounded-full transition-colors {isBookmarked ? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]' : 'text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]'}">
			<Bookmark size={20} fill={isBookmarked ? 'currentColor' : 'none'} />
		</span>
	</button>

	<!-- Share button with social media dropdown -->
	<div class="relative flex flex-col items-center gap-1" data-share-menu>
		<button
			type="button"
			onclick={toggleShareMenu}
			class="flex flex-col items-center gap-1 text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
			aria-label={t('blog.share', 'Share')}
			aria-expanded={showShareMenu}
		>
			<span class="grid h-10 w-10 place-items-center rounded-full transition-colors hover:bg-[var(--color-surface-subtle)]">
				{#if shared}
					<Check size={20} class="text-[var(--color-success)]" />
				{:else}
					<Share2 size={20} />
				{/if}
			</span>
		</button>

		{#if showShareMenu}
			<div
				class="fixed left-16 top-0 z-50 w-56 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2 shadow-lg"
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
							class="grid h-6 w-6 shrink-0 place-items-center rounded-full text-xs font-bold text-white"
							style="background-color: {platform.color || '#6B7280'}"
						>
							{platform.label.charAt(0)}
						</span>
						<span class="flex-1 text-left">{platform.label}</span>
					</button>
				{/each}
				<!-- Native share / copy link fallback -->
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
