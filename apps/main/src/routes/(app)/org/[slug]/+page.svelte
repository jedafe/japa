<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import { page } from '$app/state';
	import { FileText, Users, Info, ShieldQuestion, Image as ImageIcon, GitCompare, ArrowRight, Briefcase, Pencil, TriangleAlert } from 'lucide-svelte';
	import SetSidebar from '$lib/components/shell/SetSidebar.svelte';
	import OrgHeader from '$lib/components/profile/OrgHeader.svelte';
	import OrgProfileSidebar from '$lib/components/profile/OrgProfileSidebar.svelte';
	import { EmptyState, FollowButton, UrlTabs } from '$lib/components/ui';
	import type { UrlTab } from '$lib/components/ui';
	import type { PageData } from './$types';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';
	import { openLightbox } from '$lib/lightbox.svelte';
	import FeedCard from '$lib/components/feed/FeedCard.svelte';
	import type { ProfilePost, FeedItem } from '$lib/types';

	let { data }: { data: PageData } = $props();

	const mediaImages = $derived(data.media.filter((m) => m.mime_type.startsWith('image/')));

	const org = $derived(data.org);
	const canManage = $derived(data.canManage);
	const canManageJobs = $derived(data.canManageJobs ?? false);
	const isJobAdmin = $derived(data.isJobAdmin ?? false);
	const orgJobs = $derived(data.orgJobs ?? []);
	// Published job listings from the public org content feed — shown on the
	// Jobs tab for viewers who aren't org job editors (they see the
	// management list from GET /org/{id}/jobs instead).
	const publicJobs = $derived(
		(data.posts ?? []).filter((p) => p.content_type === 'job_listing')
	);
	const initiallyFollowing = $derived(data.initiallyFollowing ?? false);
	// Map ProfilePost → FeedItem so org posts render through the shared FeedCard.
	function toFeedItem(post: ProfilePost): FeedItem {
		return {
			id: post.id,
			title: post.title,
			slug: post.slug,
			content_type: post.content_type,
			author_id: post.author_id,
			author_name: post.author_name,
			author_username: post.author_username,
			author_avatar_url: post.author_avatar_url,
			organization: post.organization,
			space_id: post.space_id,
			space_name: post.space_name,
			space_slug: post.space_slug,
			space_avatar_url: post.space_avatar_url,
			created_at: post.published_at ?? post.created_at,
			view_count: post.view_count,
			comment_count: post.comment_count,
			reaction_count: post.reaction_count,
			bookmark_count: post.bookmark_count,
			reading_time_minutes: post.read_time_minutes,
			excerpt: post.excerpt,
			cover_image_url: post.cover_image_url,
			tags: post.tags,
			journey_stage: post.journey_stage,
			visa_route: post.visa_route,
			source_country: post.source_country,
			target_country: post.target_country,
			guide_category: post.guide_category,
			video_source_id: post.video_source_id,
			duration_secs: post.duration_secs
		};
	}

	let activeTab = $state(untrack(() => page.url.searchParams.get('tab') ?? 'about'));
	// Keep the bound tab in sync with ?tab= on back/forward navigation —
	// UrlTabs only writes the URL on click, it doesn't re-read it.
	$effect(() => {
		const tabParam = page.url.searchParams.get('tab');
		untrack(() => { activeTab = tabParam ?? 'about'; });
	});

	// Section 39 — university-type orgs get a "Compare" tab that links to
	// the Comparison System (Section 30) with this org pre-selected as
	// item 1. Other org types omit the tab.
	const isUniversity = $derived(org.org_type === 'university');

	const tabs = $derived<UrlTab[]>([
		{ value: 'about', label: t('profile.tab.about', 'About') },
		{ value: 'posts', label: t('profile.tab.posts', 'Posts') },
		{ value: 'media', label: t('profile.tab.media', 'Media') },
		{ value: 'members', label: t('profile.tab.members', 'Members') },
		// Section 63 — the Jobs tab lists the org's published listings for all
		// viewers; org job editors see the management list (applications/edit).
		...(canManageJobs || publicJobs.length > 0
			? [{ value: 'jobs', label: t('profile.tab.jobs', 'Jobs') }]
			: []),
		...(isUniversity
			? [{ value: 'compare', label: t('profile.tab.compare', 'Compare') }]
			: [])
	]);

	// Pre-fill link to the JCE picker with this university as item 1 (Section 39).
	const compareNewHref = $derived(
		`/compare?type=universities&source=university&preselect=${encodeURIComponent(org.slug)}`
	);

	const title = $derived(`${org.name} — ${t('app.name')}`);
	// Section 26 — self-referencing canonical on the clean slug URL (never
	// the query-string form) so filtered/paginated hits consolidate.
	const canonical = $derived(`${page.url.origin}/org/${org.slug}`);
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
					url: canonical,
					description: org.description ?? org.tagline ?? '',
					...(org.verified ? { sameAs: canonical } : {})
				})
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={org.name} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="profile" />
	<link rel="canonical" href={canonical} />
	<meta property="og:url" content={canonical} />
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<SetSidebar>
	<OrgProfileSidebar members={data.members} owner={data.org.owner ?? null} />
</SetSidebar>

<div class="mx-auto w-full max-w-3xl">
<OrgHeader {org} {canManage}>
	{#snippet children()}
		{#if page.data.user}
			<FollowButton entityType="organization" entityId={org.id} entitySlug={org.slug} {initiallyFollowing} />
			{#if page.data.features?.['scam_alert.enabled']}
				<a
					href="/scam-alerts/new"
					class="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-[var(--color-border)] px-4 py-1.5 text-sm font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					title={t('scam_alerts.report_button', 'Report a Scam')}
				>
					<TriangleAlert size={15} />
					{t('scam_alerts.report_button', 'Report a Scam')}
				</a>
			{/if}
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
</div>

<div class="mx-auto w-full max-w-3xl"><UrlTabs {tabs} bind:value={activeTab} /></div>

<div class="mx-auto w-full max-w-full" style="--page-gutter: 1rem">
	<div class="mx-auto w-full max-w-3xl p-4">
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
				<div class="flex flex-col gap-3">
					{#each data.posts as post (post.id)}
						<FeedCard
							item={toFeedItem(post)}
							style="daily"
							layout="stream"
						/>
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
							<button
								type="button"
								class="group block cursor-zoom-in overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]"
								title={item.alt_text ?? item.filename}
								onclick={() => openLightbox(mediaImages, mediaImages.indexOf(item))}
							>
								<img src={item.url} alt={item.alt_text ?? item.filename} loading="lazy" class="aspect-square w-full object-cover transition-transform group-hover:scale-[1.03]" />
							</button>
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
							<UserHoverCard
								id={member.user.id}
								slug={member.user.username}
								name={member.user.display_name ?? member.user.username}
								avatar={member.user.avatar_url}
								class="inline-flex w-full min-w-0 max-w-full items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-start transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								{#snippet trigger()}
									{#if member.user.avatar_url}
										<img src={member.user.avatar_url} alt="" class="h-10 w-10 rounded-full object-cover" />
									{:else}
										<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">
											{(member.user.display_name ?? member.user.username).charAt(0).toUpperCase()}
										</span>
									{/if}
									<span class="min-w-0 flex-1">
										<span class="block truncate text-sm font-medium text-[var(--color-text)]">{member.user.display_name ?? member.user.username}</span>
										<span class="block text-xs text-[var(--color-muted)]">@{member.user.username}</span>
									</span>
									<span class="shrink-0 rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-0.5 text-xs capitalize text-[var(--color-text-subtle)]">{member.role}</span>
								{/snippet}
							</UserHoverCard>
						</li>
					{/each}
				</ul>
			{/if}
		{:else if activeTab === 'jobs'}
			<!-- Section 63 — org job editors get the management list (applications
			view + edit). Everyone else sees the org's published listings. -->
			{#if canManageJobs}
				{#if orgJobs.length === 0}
					<EmptyState
						icon={Briefcase}
						title={t('jobs.org_empty', 'No job listings yet')}
						body={t('jobs.org_empty_body', 'Post a job to start receiving applications.')}
						action={{ label: t('jobs.create_button', 'Post a job'), href: '/jobs/create' }}
					/>
				{:else}
					<div class="space-y-2">
						{#each orgJobs as job (job.id)}
							<div class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
								<div class="min-w-0 flex-1">
									<a href="/jobs/{job.slug}" class="block truncate text-sm font-medium text-[var(--color-text)] hover:underline">
										{job.title}
									</a>
									<p class="mt-0.5 text-xs text-[var(--color-muted)]">
										{job.field_values?.company_name ?? ''}
										{#if job.field_values?.job_type}· {job.field_values.job_type}{/if}
									</p>
								</div>
								{#if isJobAdmin}
									<a
										href="/org/{org.slug}/jobs/{job.id}"
										class="inline-flex shrink-0 items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
									>
										{t('jobs.view_applications', 'Applications')}
									</a>
								{/if}
								<a
									href="/jobs/{job.slug}/edit"
									class="inline-flex shrink-0 items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
								>
									<Pencil size={12} /> {t('common.edit', 'Edit')}
								</a>
							</div>
						{/each}
					</div>
				{/if}
			{:else if publicJobs.length === 0}
				<EmptyState
					icon={Briefcase}
					title={t('jobs.org_empty', 'No job listings yet')}
				/>
			{:else}
				<div class="space-y-2">
					{#each publicJobs as job (job.id)}
						<a
							href="/jobs/{job.slug}"
							class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<div class="min-w-0 flex-1">
								<span class="block truncate text-sm font-medium text-[var(--color-text)]">{job.title}</span>
								{#if job.excerpt}
									<p class="mt-0.5 truncate text-xs text-[var(--color-muted)]">{job.excerpt}</p>
								{/if}
							</div>
							<ArrowRight size={14} class="shrink-0 text-[var(--color-muted)] rtl:-scale-x-100" />
						</a>
					{/each}
				</div>
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
						<ArrowRight class="rtl:-scale-x-100" size={15} />
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
								<ArrowRight size={15} class="shrink-0 text-[var(--color-muted)] rtl:-scale-x-100" />
							</a>
						{/each}
					</div>
				</div>
			{/if}
		{/if}
	</div>
</div>
