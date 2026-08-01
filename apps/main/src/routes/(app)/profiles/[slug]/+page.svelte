<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { BadgeCheck, Globe, Users, Briefcase, FileText } from 'lucide-svelte';
	import { EmptyState, FollowButton } from '$lib/components/ui';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const profile = $derived(data.profile);
	const posts = $derived(data.posts ?? []);
	const initiallyFollowing = $derived(data.initiallyFollowing ?? false);
	const activeTab = $derived(page.url.searchParams.get('tab') ?? 'about');

	// field_values is a record of key → value; render as key-value pairs.
	const fieldEntries = $derived(
		profile.field_values
			? Object.entries(profile.field_values).filter(([, v]) => v !== null && v !== undefined)
			: []
	);

	const title = $derived(`${profile.name} — ${t('app.name', 'JapaConnect')}`);
	const description = $derived(profile.tagline ?? profile.description ?? profile.name);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={profile.name} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="profile" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-2xl">
	<!-- Header -->
	<div class="border-b border-[var(--color-border)] px-4 py-4">
		<div class="flex items-start gap-3">
			<span
				class="grid h-12 w-12 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
			>
				<Briefcase size={22} />
			</span>
			<div class="min-w-0 flex-1">
				<div class="flex items-center gap-1.5">
					<h1 class="truncate text-lg font-semibold text-[var(--color-text)]">{profile.name}</h1>
					{#if profile.verified}
						<BadgeCheck
							size={18}
							class="shrink-0 text-[var(--color-primary)]"
							aria-label={t('profiles.verified', 'Verified')}
						/>
					{/if}
				</div>
				{#if profile.tagline}
					<p class="mt-0.5 text-sm text-[var(--color-text-subtle)]">{profile.tagline}</p>
				{/if}
				<div class="mt-2 flex flex-wrap items-center gap-3 text-xs text-[var(--color-muted)]">
					<span class="flex items-center gap-1">
						<Users size={12} />
						{profile.follower_count}
						{t('profiles.followers', 'followers')}
					</span>
					{#if profile.verification_type}
						<span
							class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 capitalize text-[var(--color-text-subtle)]"
						>
							{profile.verification_type}
						</span>
					{/if}
				</div>
			</div>
			{#if page.data.user}
				<FollowButton entityType="profile" entityId={profile.id} {initiallyFollowing} />
			{/if}
		</div>
	</div>

	<!-- Tabs -->
	<div class="flex gap-1 border-b border-[var(--color-border)] px-4">
		<a
			href="?tab=about"
			class="border-b-2 px-3 py-2 text-sm font-medium transition-colors {activeTab === 'about' ? 'border-[var(--color-primary)] text-[var(--color-primary)]' : 'border-transparent text-[var(--color-text-subtle)] hover:text-[var(--color-text)]'}"
		>
			{t('profiles.tab_about', 'About')}
		</a>
		<a
			href="?tab=posts"
			class="border-b-2 px-3 py-2 text-sm font-medium transition-colors {activeTab === 'posts' ? 'border-[var(--color-primary)] text-[var(--color-primary)]' : 'border-transparent text-[var(--color-text-subtle)] hover:text-[var(--color-text)]'}"
		>
			{t('profiles.tab_posts', 'Posts')}
		</a>
	</div>

	<div class="p-3 space-y-3">
		{#if activeTab === 'posts'}
			<!-- Posts tab -->
			{#if posts.length === 0}
				<EmptyState icon={FileText} title={t('profiles.no_posts', 'No posts yet')} />
			{:else}
				<div class="divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					{#each posts as post (post.id)}
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
		{:else}
			<!-- About tab -->
			{#if profile.description}
			<section
				class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
			>
				<h2 class="mb-2 text-sm font-medium text-[var(--color-text)]">
					{t('profiles.about', 'About')}
				</h2>
				<p class="text-sm leading-relaxed text-[var(--color-text)]">{profile.description}</p>
			</section>
		{/if}

		<!-- Website -->
		{#if profile.website_url}
			<section
				class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
			>
				<h2 class="mb-2 text-sm font-medium text-[var(--color-text)]">
					{t('profiles.website', 'Website')}
				</h2>
				<a
					href={profile.website_url}
					target="_blank"
					rel="noopener noreferrer"
					class="inline-flex items-center gap-1.5 text-sm text-[var(--color-primary)] hover:underline"
				>
					<Globe size={15} />
					{profile.website_url}
				</a>
			</section>
		{/if}

		<!-- Field values -->
		{#if fieldEntries.length > 0}
			<section
				class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
			>
				<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">
					{t('profiles.fields', 'Details')}
				</h2>
				<dl class="divide-y divide-[var(--color-border)]">
					{#each fieldEntries as [key, value] (key)}
						<div class="flex justify-between gap-4 py-2">
							<dt class="text-sm capitalize text-[var(--color-muted)]">
								{key.replace(/_/g, ' ')}
							</dt>
							<dd class="text-right text-sm text-[var(--color-text)]">
								{String(value)}
							</dd>
						</div>
					{/each}
				</dl>
			</section>
		{:else if !profile.description && !profile.website_url}
			<EmptyState
				icon={Briefcase}
				title={t('profiles.empty', 'No additional details for this profile yet')}
			/>
		{/if}
		{/if}
	</div>
</div>
