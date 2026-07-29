<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from '$lib/i18n';
	import { BadgeCheck, Globe, Users, Calendar } from 'lucide-svelte';
	import type { PublicOrganization } from '$lib/types';

	interface Props {
		org: PublicOrganization;
		isOwner: boolean;
		children?: Snippet;
	}
	let { org, isOwner, children }: Props = $props();

	const joinDate = $derived(
		new Date(org.created_at).toLocaleDateString(undefined, {
			year: 'numeric',
			month: 'long'
		})
	);
	const initials = $derived(org.name?.charAt(0)?.toUpperCase() ?? '?');
</script>

<header class="border-b border-[var(--color-border)] bg-[var(--color-surface)]">
	<div class="h-32 bg-gradient-to-br from-[var(--color-primary)]/15 to-[var(--color-surface-subtle)] sm:h-40"></div>

	<div class="px-4 pb-4">
		<div class="-mt-10 flex flex-col gap-3 sm:-mt-14 sm:flex-row sm:items-end sm:gap-4">
			<span
				class="grid h-20 w-20 place-items-center rounded-[var(--radius-card)] border-4 border-[var(--color-surface)] bg-[var(--color-primary)]/10 text-2xl font-bold text-[var(--color-primary)] sm:h-28 sm:w-28"
			>
				{initials}
			</span>

			<div class="flex-1 pb-1">
				<div class="flex items-center gap-2">
					<h1 class="text-xl font-semibold text-[var(--color-text)]">{org.name}</h1>
					{#if org.verified}
						<BadgeCheck size={18} class="text-[var(--color-primary)]" />
					{/if}
				</div>
				<p class="text-sm text-[var(--color-muted)]">@{org.slug}</p>
				{#if org.tagline}
					<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{org.tagline}</p>
				{/if}
			</div>

			{#if isOwner}
				<a
					href="/settings/organizations"
					class="shrink-0 rounded-full border border-[var(--color-border)] px-4 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					{t('profile.manage_org', 'Manage')}
				</a>
			{:else if children}
				{@render children()}
			{/if}
		</div>

		{#if org.description}
			<p class="mt-3 text-sm text-[var(--color-text)]">{org.description}</p>
		{/if}

		<div class="mt-3 flex flex-wrap items-center gap-4 text-sm text-[var(--color-muted)]">
			<span class="flex items-center gap-1"><Users size={14} /> {t('profile.members', '{n} members').replace('{n}', String(org.member_count))}</span>
			<span>{t('profile.followers', '{n} followers').replace('{n}', String(org.follower_count))}</span>
			{#if org.website_url}
				<a href={org.website_url} target="_blank" rel="noopener noreferrer" class="flex items-center gap-1 text-[var(--color-primary)] hover:underline">
					<Globe size={14} /> {t('profile.website', 'Website')}
				</a>
			{/if}
			<span class="flex items-center gap-1"><Calendar size={14} /> {t('profile.joined', 'Joined')} {joinDate}</span>
		</div>
	</div>
</header>
