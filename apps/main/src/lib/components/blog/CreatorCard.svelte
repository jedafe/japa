<script lang="ts">
	import { t } from '$lib/i18n';
	import { Pencil } from 'lucide-svelte';
	import FollowButton from '$lib/components/ui/FollowButton.svelte';

	interface Props {
		authorName?: string;
		/** Author username for the profile link. When absent, the avatar/name is not linked. */
		authorUsername?: string | null;
		authorAvatarUrl?: string | null;
		spaceName?: string | null;
		/** Slug of the primary space so the "Writing in" text can link to it. */
		spaceSlug?: string | null;
		/** Author's user ID — required for the follow button to call the API.
		 * When absent, the follow button is hidden (e.g. anonymous author). */
		authorId?: string | null;
		/** Whether the viewer is already following the author (hydrated SSR). */
		initiallyFollowing?: boolean;
		/** Whether the viewer is authenticated — guests see no follow button. */
		authed?: boolean;
		/** The viewer's user ID — when it matches authorId, the card shows an
		 * "Edit profile" link instead of a follow button (self-author). */
		currentUserId?: string | null;
	}
	let {
		authorName,
		authorUsername = null,
		authorAvatarUrl,
		spaceName,
		spaceSlug = null,
		authorId = null,
		initiallyFollowing = false,
		authed = false,
		currentUserId = null
	}: Props = $props();

	const isOwnContent = $derived(!!currentUserId && !!authorId && currentUserId === authorId);
</script>

<!-- ADR-058 Exception #1: CreatorCard REPLACES the search/avatar bar in the right sidebar. -->
<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
	{#snippet authorIdentity()}
		{#if authorAvatarUrl}
			<img src={authorAvatarUrl} alt={authorName ?? ''} class="h-20 w-20 rounded-full object-cover" />
		{:else}
			<span class="grid h-20 w-20 place-items-center rounded-full bg-[var(--color-primary)]/10 text-2xl font-bold text-[var(--color-primary)]">
				{authorName?.charAt(0) ?? '?'}
			</span>
		{/if}
		<h3 class="mt-3 font-semibold text-[var(--color-text)] group-hover:text-[var(--color-primary)]">{authorName ?? t('blog.anonymous', 'Anonymous')}</h3>
	{/snippet}

	<div class="flex flex-col items-center text-center">
		{#if authorUsername}
			<a href="/u/{authorUsername}" class="group flex flex-col items-center transition-colors hover:text-[var(--color-primary)]">
				{@render authorIdentity()}
			</a>
		{:else}
			<div class="flex flex-col items-center">
				{@render authorIdentity()}
			</div>
		{/if}
		{#if spaceName}
			{#if spaceSlug}
				<a href="/spaces/{spaceSlug}" class="mt-1 block text-sm text-[var(--color-primary)] hover:underline">
					{t('blog.writing_in', 'Writing in')} {spaceName}
				</a>
			{:else}
				<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{t('blog.writing_in', 'Writing in')} {spaceName}</p>
			{/if}
		{/if}
	</div>

	{#if isOwnContent}
		<a
			href="/settings/profile"
			class="mt-4 flex w-full items-center justify-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			<Pencil size={16} />
			{t('profile.edit_profile', 'Edit profile')}
		</a>
	{:else if authed && authorId}
		<div class="mt-4 flex justify-center">
			<FollowButton entityType="user" entityId={authorId} {initiallyFollowing} />
		</div>
	{/if}
</div>
