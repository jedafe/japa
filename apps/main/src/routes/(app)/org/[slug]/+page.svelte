<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { FileText, Users, Info, ShieldQuestion, Image as ImageIcon } from 'lucide-svelte';
	import SetSidebar from '$lib/components/shell/SetSidebar.svelte';
	import OrgHeader from '$lib/components/profile/OrgHeader.svelte';
	import ProfileTabs from '$lib/components/profile/ProfileTabs.svelte';
	import OrgProfileSidebar from '$lib/components/profile/OrgProfileSidebar.svelte';
	import { EmptyState, FollowButton } from '$lib/components/ui';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const org = $derived(data.org);
	const isOwner = $derived(data.isOwner);
	const initiallyFollowing = $derived(data.initiallyFollowing ?? false);
	const activeTab = $derived(page.url.searchParams.get('tab') ?? 'about');

	const tabs = $derived([
		{ key: 'about', labelKey: 'profile.tab.about', href: `?tab=about` },
		{ key: 'posts', labelKey: 'profile.tab.posts', href: `?tab=posts` },
		{ key: 'media', labelKey: 'profile.tab.media', href: `?tab=media` },
		{ key: 'members', labelKey: 'profile.tab.members', href: `?tab=members` }
	]);

	const title = $derived(`${org.name} — ${t('app.name')}`);
	const description = $derived(org.description ?? org.tagline ?? org.name);
	// JSON-LD: Organization schema (H3 — audit finding).
	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'Organization',
			name: org.name,
			url: page.url.href,
			description: org.description ?? org.tagline ?? '',
			...(org.verified ? { sameAs: page.url.href } : {})
		})
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={org.name} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="profile" />
	<meta property="og:url" content={page.url.href} />
	{@html `<script type="application/ld+json">${jsonLd}</script>`}
</svelte:head>

<SetSidebar>
	<OrgProfileSidebar {org} {isOwner} members={data.members} />
</SetSidebar>

<OrgHeader {org} {isOwner}>
	{#snippet children()}
		{#if page.data.user}
			<FollowButton entityType="organization" entityId={org.id} entitySlug={org.slug} {initiallyFollowing} />
			{#if !org.verified}
				<a
					href="/claim-ownership?org={org.slug}"
					class="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-[var(--color-border)] px-4 py-1.5 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					title={t('org.claim_ownership', 'Claim ownership of this organization')}
				>
					<ShieldQuestion size={15} />
					{t('org.claim', 'Claim')}
				</a>
			{/if}
		{/if}
	{/snippet}
</OrgHeader>

<ProfileTabs {tabs} active={activeTab} />

<div class="mx-auto w-full max-w-2xl">
	<div class="p-3">
		{#if activeTab === 'about'}
			<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				{#if org.description}
					<p class="text-sm leading-relaxed text-[var(--color-text)]">{org.description}</p>
				{:else}
					<EmptyState icon={Info} title={t('profile.no_about', 'No description yet')} />
				{/if}
			</section>
		{:else if activeTab === 'posts'}
			{#if data.posts.length === 0}
				<EmptyState icon={FileText} title={t('profile.posts_empty', 'No posts yet')} />
			{:else}
				<div class="divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					{#each data.posts as post (post.id)}
						<a href="/content/{post.slug}" class="block p-4 transition-colors hover:bg-[var(--color-surface-subtle)]">
							<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
								<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 font-medium text-[var(--color-text-subtle)]">{post.content_type}</span>
								<span>· {post.read_time_minutes} {t('feed.min_read')}</span>
							</div>
							<h3 class="mt-1.5 text-sm font-medium text-[var(--color-text)]">{post.title}</h3>
							<div class="mt-2 flex items-center gap-4 text-xs text-[var(--color-muted)]">
								<span>{post.view_count} {t('profile.views', 'views')}</span>
								<span>{post.comment_count} {t('profile.comments', 'comments')}</span>
							</div>
						</a>
					{/each}
				</div>
			{/if}
		{:else if activeTab === 'media'}
			{#if data.media.length === 0}
				<EmptyState icon={ImageIcon} title={t('profile.media_empty', 'No media yet')} />
			{:else}
				<div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
					{#each data.media as item (item.id)}
						{#if item.mime_type.startsWith('image/')}
							<a
								href={item.url}
								target="_blank"
								rel="noopener"
								class="group block overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]"
								title={item.alt_text ?? item.filename}
							>
								<img src={item.url} alt={item.alt_text ?? item.filename} loading="lazy" class="aspect-square w-full object-cover transition-transform group-hover:scale-[1.03]" />
							</a>
						{:else}
							<a
								href={item.url}
								target="_blank"
								rel="noopener"
								class="flex aspect-square flex-col items-center justify-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-center transition-colors hover:bg-[var(--color-surface-subtle)]"
								title={item.filename}
							>
								<ImageIcon size={22} class="text-[var(--color-muted)]" />
								<span class="line-clamp-2 text-xs text-[var(--color-text-subtle)]">{item.filename}</span>
							</a>
						{/if}
					{/each}
				</div>
			{/if}
		{:else if activeTab === 'members'}
			{#if data.members.length === 0}
				<EmptyState icon={Users} title={t('profile.members_empty', 'No members yet')} />
			{:else}
				<ul class="space-y-2">
					{#each data.members as member (member.user.id)}
						<li>
							<a
								href="/u/{member.user.username}"
								class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								{#if member.user.avatar_url}
									<img src={member.user.avatar_url} alt="" class="h-10 w-10 rounded-full object-cover" />
								{:else}
									<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">
										{member.user.display_name?.charAt(0)?.toUpperCase() ?? '?'}
									</span>
								{/if}
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium text-[var(--color-text)]">{member.user.display_name}</span>
									<span class="block text-xs text-[var(--color-muted)]">@{member.user.username}</span>
								</span>
								<span class="shrink-0 rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-0.5 text-xs capitalize text-[var(--color-text-subtle)]">{member.role}</span>
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		{/if}
	</div>
</div>
