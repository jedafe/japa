<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from '$lib/i18n';
	import { i18n } from '$lib/i18n';
	import { BadgeCheck, Calendar, MessageSquare, MoreVertical, Pencil } from 'lucide-svelte';
	import { Menu, MenuItem } from '$lib/components/ui';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import type { PublicUser } from '$lib/types';
	import FollowListModal, { type FollowListItem } from '$lib/components/profile/FollowListModal.svelte';

	interface Props {
		user: PublicUser;
		isOwnProfile?: boolean;
		followers?: FollowListItem[];
		following?: FollowListItem[];
		children?: Snippet;
		/** Items rendered inside the cover overflow menu (other profiles). */
		menuItems?: Snippet;
	}
	let { user, isOwnProfile = false, followers = [], following = [], children, menuItems }: Props = $props();

	let followersModalOpen = $state(false);
	let followingModalOpen = $state(false);

	const joinDate = $derived(
		new Date(user.created_at).toLocaleDateString(i18n.locale, {
			year: 'numeric',
			month: 'long'
		})
	);
	const initials = $derived((user.display_name?.charAt(0) ?? user.username?.charAt(0) ?? '?').toUpperCase());
</script>

<header class="border-b border-[var(--color-border)] bg-[var(--color-surface)]">
	<!-- Cover image — falls back to a gradient when cover_url is absent -->
	<div class="relative">
		{#if user.cover_url}
			<div class="h-32 overflow-hidden sm:h-40">
				<img src={user.cover_url} alt="" class="h-full w-full object-cover" />
			</div>
		{:else}
			<div class="h-32 bg-gradient-to-br from-[var(--color-primary)]/15 to-[var(--color-surface-subtle)] sm:h-40"></div>
		{/if}

		<!-- Overflow actions pinned to the cover's top-right corner; the header
		     row keeps only primary actions (e.g. Follow) as buttons. -->
		{#if isOwnProfile || (menuItems && page.data.isAuthed)}
			<div class="absolute right-3 top-3 z-10">
				<Menu>
					{#snippet trigger(props)}
						<button
							type="button"
							{...props}
							class="grid h-9 w-9 place-items-center rounded-full bg-black/40 text-white backdrop-blur-sm transition-colors hover:bg-black/60"
							aria-label={t('common.actions', 'Actions')}
						>
							<MoreVertical size={16} />
						</button>
					{/snippet}
					{#if isOwnProfile}
						<MenuItem icon={Pencil} onSelect={() => goto('/settings/profile')}>
							{t('profile.edit_profile', 'Edit profile')}
						</MenuItem>
					{:else if menuItems}
						{@render menuItems()}
					{/if}
				</Menu>
			</div>
		{/if}
	</div>

	<div class="px-4 pb-4">
		<!-- relative + z-10: the cover wrapper above is position:relative, so
		     without its own stacking position this row (and the avatar it
		     overlaps upward via -mt-12/-mt-16) would paint under the cover. -->
		<div class="relative z-10 -mt-12 flex flex-col gap-3 sm:-mt-16 sm:flex-row sm:items-end sm:gap-4">
			{#if user.avatar_url}
				<img
					src={user.avatar_url}
					alt={user.display_name ?? user.username}
					class="h-24 w-24 rounded-full border-4 border-[var(--color-surface)] object-cover sm:h-32 sm:w-32"
				/>
			{:else}
				<span
					class="grid h-24 w-24 place-items-center rounded-full border-4 border-[var(--color-surface)] bg-[var(--color-primary)]/10 text-3xl font-bold text-[var(--color-primary)] sm:h-32 sm:w-32"
				>
					{initials}
				</span>
			{/if}

			<div class="flex-1 pb-1">
				<div class="flex items-center gap-2">
					<h1 class="text-xl font-semibold text-[var(--color-text)]">{user.display_name ?? user.username}</h1>
					{#if user.profile_type === 'business'}
						<BadgeCheck size={18} class="text-[var(--color-primary)]" />
					{/if}
				</div>
				<p class="text-sm text-[var(--color-muted)]">@{user.username}</p>
			</div>

			{#if children}
				{@render children()}
			{/if}
		</div>

		{#if user.bio}
			<p class="mt-3 text-sm text-[var(--color-text)]">{user.bio}</p>
		{/if}

		<div class="mt-3 flex flex-wrap items-center gap-4 text-sm text-[var(--color-muted)]">
			<span class="flex items-center gap-1"><Calendar size={14} /> {t('profile.joined', 'Joined')} {joinDate}</span>
			<a
				href="/forum?author_id={user.id}"
				class="inline-flex items-center gap-1 transition-colors hover:text-[var(--color-text)]"
			>
				<MessageSquare size={14} /> {t('profile.forum_posts', 'Forum posts')}
			</a>
			<button
				type="button"
				class="ml-auto transition-colors hover:text-[var(--color-text)]"
				onclick={() => (followersModalOpen = true)}
			>
				{(user.follower_count === 1 ? t('profile.followers_one', '{n} follower') : t('profile.followers', '{n} followers')).replace('{n}', String(user.follower_count))}
			</button>
			<button
				type="button"
				class="transition-colors hover:text-[var(--color-text)]"
				onclick={() => (followingModalOpen = true)}
			>
				{(user.following_count === 1 ? t('profile.following_one', '{n} following') : t('profile.following', '{n} following')).replace('{n}', String(user.following_count))}
			</button>
		</div>
	</div>
</header>

<FollowListModal bind:open={followersModalOpen} userId={user.id} type="followers" count={user.follower_count} initialItems={followers} />
<FollowListModal bind:open={followingModalOpen} userId={user.id} type="following" count={user.following_count} initialItems={following} />
