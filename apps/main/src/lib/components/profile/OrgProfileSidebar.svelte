<script lang="ts">
	import { t } from '$lib/i18n';
	import { UserPlus, UserCheck } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import type { PublicOrganization, OrgMember } from '$lib/types';

	interface Props {
		org: PublicOrganization;
		isOwner: boolean;
		members: OrgMember[];
	}
	let { org, isOwner, members }: Props = $props();

	let following = $state(false);
	let pending = $state(false);

	async function toggle() {
		if (pending || isOwner) return;
		const wasFollowing = following;
		following = !wasFollowing;
		pending = true;
		const res = wasFollowing
			? await api.delete(`/organizations/${org.id}/follow`)
			: await api.post(`/organizations/${org.id}/follow`);
		if (res.error) following = wasFollowing;
		pending = false;
	}
</script>

{#if !isOwner}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<button
			type="button"
			onclick={toggle}
			disabled={pending}
			aria-busy={pending}
			class="flex w-full items-center justify-center gap-1.5 rounded-full px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50 {following
				? 'border border-[var(--color-border)] text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]'
				: 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'}"
		>
			{#if following}
				<UserCheck size={16} />
				{t('profile.following', 'Following')}
			{:else}
				<UserPlus size={16} />
				{t('profile.follow', 'Follow')}
			{/if}
		</button>
	</section>
{/if}

{#if members.length > 0}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('profile.members', 'Members')}</h2>
		<ul class="space-y-2">
			{#each members.slice(0, 8) as member (member.user.id)}
				<li>
					<a
						href={`/u/${member.user.username}`}
						class="flex items-center gap-2 rounded-[var(--radius)] p-1.5 hover:bg-[var(--color-surface-subtle)]"
					>
						{#if member.user.avatar_url}
							<img src={member.user.avatar_url} alt="" class="h-8 w-8 rounded-full object-cover" />
						{:else}
							<span class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">
								{member.user.display_name?.charAt(0)?.toUpperCase() ?? '?'}
							</span>
						{/if}
						<span class="min-w-0 flex-1">
							<span class="block truncate text-sm text-[var(--color-text)]">{member.user.display_name}</span>
							<span class="block text-xs text-[var(--color-muted)] capitalize">{member.role}</span>
						</span>
					</a>
				</li>
			{/each}
		</ul>
		{#if members.length > 8}
			<p class="mt-2 text-xs text-[var(--color-muted)]">+{members.length - 8} {t('profile.more_members', 'more')}</p>
		{/if}
	</section>
{/if}
