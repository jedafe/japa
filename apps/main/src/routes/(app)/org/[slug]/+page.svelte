<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { FileText, Users, Info, ShieldQuestion, Image as ImageIcon, GitCompare, ArrowRight } from 'lucide-svelte';
	import SetSidebar from '$lib/components/shell/SetSidebar.svelte';
	import OrgHeader from '$lib/components/profile/OrgHeader.svelte';
	import OrgProfileSidebar from '$lib/components/profile/OrgProfileSidebar.svelte';
	import { EmptyState, FollowButton, UrlTabs } from '$lib/components/ui';
	import type { UrlTab } from '$lib/components/ui';
	import type { PageData } from './$types';
import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const org = $derived(data.org);
	const canManage = $derived(data.canManage);
	const initiallyFollowing = $derived(data.initiallyFollowing ?? false);
	let activeTab = $state(untrack(() => page.url.searchParams.get('tab') ?? 'about'));

	// Section 39 — university-type orgs get a "Compare" tab that links to
	// the Comparison System (Section 30) with this org pre-selected as
	// item 1. Other org types omit the tab.
	const isUniversity = $derived(org.org_type === 'university');

	const tabs = $derived<UrlTab[]>([
		{ value: 'about', label: t('profile.tab.about', 'About') },
		{ value: 'posts', label: t('profile.tab.posts', 'Posts') },
		{ value: 'media', label: t('profile.tab.media', 'Media') },
		{ value: 'members', label: t('profile.tab.members', 'Members') },
		...(isUniversity
			? [{ value: 'compare', label: t('profile.tab.compare', 'Compare') }]
			: [])
	]);

	// Pre-fill link to the JCE picker with this university as item 1 (Section 39).
	const compareNewHref = $derived(
		`/compare?type=universities&source=university&preselect=${encodeURIComponent(org.slug)}`
	);

	const title = $derived(`${org.name} — ${t('app.name')}`);
	const description = $derived(org.description ?? org.tagline ?? org.name);
	// JSON-LD: Organization schema (Section 14 / Section 26). The backend
	// resolves the admin-managed Organization template server-side and returns
	// it in the `json_ld` sidecar. Fall back to a basic schema when absent.
	const jsonLd = $derived(
		data.jsonLd
			? JSON.stringify(data.jsonLd)
			: JSON.stringify({
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
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<SetSidebar>
	<OrgProfileSidebar members={data.members} />
</SetSidebar>

<OrgHeader {org} {canManage}>
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

<UrlTabs {tabs} bind:value={activeTab} />

<div class="mx-auto w-full max-w-full">
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
		{:else if activeTab === 'compare'}
			<!-- Section 39 — Compare tab for university-type orgs.
			Links to the Comparison System (Section 30) with this org
			pre-selected as item 1. Also lists existing comparisons
			of this type if any were fetched server-side. -->
			<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<div class="flex flex-col gap-3">
					<div class="flex items-start gap-3">
						<span class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
							<GitCompare size={20} />
						</span>
						<div class="flex-1">
							<h2 class="text-sm font-medium text-[var(--color-text)]">
								{t('org.compare_title', 'Compare this university')}
							</h2>
							<p class="mt-1 text-xs text-[var(--color-text-subtle)]">
								{t('org.compare_description', 'Start a side-by-side comparison with another university across dimensions like tuition, ranking, programs, and more.')}
							</p>
						</div>
					</div>
					<a
						href={compareNewHref}
						class="inline-flex w-fit items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
					>
						{t('org.compare_start', 'Start a comparison')}
						<ArrowRight size={15} />
					</a>
				</div>
			</section>

			{#if data.comparisons && data.comparisons.length > 0}
				<div class="mt-4">
					<h3 class="mb-2 text-xs font-medium uppercase text-[var(--color-muted)]">
						{t('org.compare_existing', 'Existing comparisons')}
					</h3>
					<div class="space-y-2">
						{#each data.comparisons as cmp (cmp.id)}
							<a
								href="/compare/{cmp.slug}"
								class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
							>
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium text-[var(--color-text)]">{cmp.title}</span>
									<span class="block text-xs text-[var(--color-muted)]">
										{cmp.view_count} {t('profile.views', 'views')}
										· {cmp.comment_count} {t('profile.comments', 'comments')}
									</span>
								</span>
								<ArrowRight size={15} class="shrink-0 text-[var(--color-muted)]" />
							</a>
						{/each}
					</div>
				</div>
			{/if}
		{/if}
	</div>
</div>
