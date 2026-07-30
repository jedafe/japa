<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from '$lib/i18n';
	import { BadgeCheck, Calendar } from 'lucide-svelte';
	import type { PublicUser } from '$lib/types';

	interface Props {
		user: PublicUser;
		isOwnProfile?: boolean;
		children?: Snippet;
	}
	let { user, isOwnProfile = false, children }: Props = $props();

	const joinDate = $derived(
		new Date(user.created_at).toLocaleDateString(undefined, {
			year: 'numeric',
			month: 'long'
		})
	);
	const initials = $derived(user.display_name?.charAt(0)?.toUpperCase() ?? '?');
</script>

<header class="border-b border-[var(--color-border)] bg-[var(--color-surface)]">
	<!-- Cover image — falls back to a gradient when cover_url is absent -->
	{#if user.cover_url}
		<div class="h-32 overflow-hidden sm:h-40">
			<img src={user.cover_url} alt="" class="h-full w-full object-cover" />
		</div>
	{:else}
		<div class="h-32 bg-gradient-to-br from-[var(--color-primary)]/15 to-[var(--color-surface-subtle)] sm:h-40"></div>
	{/if}

	<div class="px-4 pb-4">
		<div class="-mt-12 flex flex-col gap-3 sm:-mt-16 sm:flex-row sm:items-end sm:gap-4">
			{#if user.avatar_url}
				<img
					src={user.avatar_url}
					alt={user.display_name}
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
					<h1 class="text-xl font-semibold text-[var(--color-text)]">{user.display_name}</h1>
					{#if user.profile_type === 'business' || user.profile_type === 'organisation'}
						<BadgeCheck size={18} class="text-[var(--color-primary)]" />
					{/if}
				</div>
				<p class="text-sm text-[var(--color-muted)]">@{user.username}</p>
			</div>

			{#if isOwnProfile}
				<a
					href="/settings/profile"
					class="shrink-0 rounded-full border border-[var(--color-border)] px-4 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					{t('profile.edit_profile', 'Edit profile')}
				</a>
			{:else if children}
				{@render children()}
			{/if}
		</div>

		{#if user.bio}
			<p class="mt-3 text-sm text-[var(--color-text)]">{user.bio}</p>
		{/if}

		<div class="mt-3 flex flex-wrap items-center gap-4 text-sm text-[var(--color-muted)]">
			<span>{t('profile.followers', '{n} followers').replace('{n}', String(user.follower_count))}</span>
			<span>{t('profile.following', '{n} following').replace('{n}', String(user.following_count))}</span>
			<span class="flex items-center gap-1"><Calendar size={14} /> {t('profile.joined', 'Joined')} {joinDate}</span>
		</div>
	</div>
</header>
