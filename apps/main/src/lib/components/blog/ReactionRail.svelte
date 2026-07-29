<script lang="ts">
	import { untrack } from 'svelte';
	import { Bookmark, MessageCircle, Share2, Heart } from 'lucide-svelte';
	import { t } from '$lib/i18n';

	interface Props {
		voteScore?: number;
		commentCount?: number;
		bookmarked?: boolean;
	}
	let { voteScore = 0, commentCount = 0, bookmarked = false }: Props = $props();

	let isBookmarked = $state(untrack(() => bookmarked));
	let isLiked = $state(false);
	let likeCount = $state(untrack(() => voteScore));

	function toggleLike() {
		isLiked = !isLiked;
		likeCount += isLiked ? 1 : -1;
	}

	function toggleBookmark() {
		isBookmarked = !isBookmarked;
	}

	async function share() {
		try {
			await navigator.share({ url: window.location.href });
		} catch {
			await navigator.clipboard.writeText(window.location.href);
		}
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
	>
		<span class="grid h-10 w-10 place-items-center rounded-full transition-colors {isBookmarked ? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]' : 'text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]'}">
			<Bookmark size={20} fill={isBookmarked ? 'currentColor' : 'none'} />
		</span>
	</button>

	<button
		type="button"
		onclick={share}
		class="flex flex-col items-center gap-1 text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
		aria-label={t('blog.share', 'Share')}
	>
		<span class="grid h-10 w-10 place-items-center rounded-full hover:bg-[var(--color-surface-subtle)]">
			<Share2 size={20} />
		</span>
	</button>
</div>
