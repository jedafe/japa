<script lang="ts">
	import { t } from '$lib/i18n';
	import { UserPlus } from 'lucide-svelte';

	interface Props {
		authorName?: string;
		authorAvatarUrl?: string | null;
		spaceName?: string | null;
	}
	let { authorName, authorAvatarUrl, spaceName }: Props = $props();

	let following = $state(false);

	function toggleFollow() {
		following = !following;
	}
</script>

<!-- ADR-058 Exception #1: CreatorCard REPLACES the search/avatar bar in the right sidebar. -->
<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
	<div class="flex flex-col items-center text-center">
		{#if authorAvatarUrl}
			<img src={authorAvatarUrl} alt={authorName ?? ''} class="h-20 w-20 rounded-full object-cover" />
		{:else}
			<span class="grid h-20 w-20 place-items-center rounded-full bg-[var(--color-primary)]/10 text-2xl font-bold text-[var(--color-primary)]">
				{authorName?.charAt(0) ?? '?'}
			</span>
		{/if}
		<h3 class="mt-3 font-semibold text-[var(--color-text)]">{authorName ?? t('blog.anonymous', 'Anonymous')}</h3>
		{#if spaceName}
			<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{t('blog.writing_in', 'Writing in')} {spaceName}</p>
		{/if}
	</div>

	<button
		type="button"
		onclick={toggleFollow}
		class="mt-4 flex w-full items-center justify-center gap-2 rounded-[var(--radius)] px-4 py-2 text-sm font-semibold transition-colors {following
			? 'border border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'
			: 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'}"
	>
		{#if !following}<UserPlus size={16} />{/if}
		{following ? t('blog.following', 'Following') : t('blog.follow', 'Follow')}
	</button>

	<!-- More from author placeholder — populated when author content API is available -->
	<div class="mt-5 border-t border-[var(--color-border)] pt-4">
		<h4 class="mb-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">{t('blog.more_from', 'More from author')}</h4>
		<p class="text-sm text-[var(--color-text-subtle)]">{t('blog.no_more_posts', 'No more posts yet.')}</p>
	</div>
</div>
