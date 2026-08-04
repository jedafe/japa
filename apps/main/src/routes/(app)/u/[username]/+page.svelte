<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { FileText, MessageSquare, Image, Heart, Users, Info, BarChart3, Award } from 'lucide-svelte';
	import SetSidebar from '$lib/components/shell/SetSidebar.svelte';
	import ProfileHeader from '$lib/components/profile/ProfileHeader.svelte';
	import ProfileTabs from '$lib/components/profile/ProfileTabs.svelte';
	import FollowButton from '$lib/components/ui/FollowButton.svelte';
	import UserProfileSidebar from '$lib/components/profile/UserProfileSidebar.svelte';
	import { EmptyState } from '$lib/components/ui';
	import type { ProfileAnalytics } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const user = $derived(data.user);
	const isOwnProfile = $derived(data.isOwnProfile);
	const initiallyFollowing = $derived(data.initiallyFollowing);
	const activeTab = $derived(page.url.searchParams.get('tab') ?? 'posts');

	const tabs = $derived([
		{ key: 'posts', labelKey: 'profile.tab.posts', href: `?tab=posts` },
		{ key: 'about', labelKey: 'profile.tab.about', href: `?tab=about` },
		{ key: 'replies', labelKey: 'profile.tab.replies', href: `?tab=replies` },
		{ key: 'media', labelKey: 'profile.tab.media', href: `?tab=media` },
		{ key: 'likes', labelKey: 'profile.tab.likes', href: `?tab=likes` },
		{ key: 'spaces', labelKey: 'profile.tab.spaces', href: `?tab=spaces` },
		// Analytics is a self-dashboard — owner-only per JEOS §342 [auth, self].
		// Hidden from guests and from other authenticated users.
		...(isOwnProfile
			? [{ key: 'analytics', labelKey: 'profile.tab.analytics', href: `?tab=analytics` }]
			: [])
	]);

	const analytics = $derived(data.analytics as ProfileAnalytics | null);

	const title = $derived(`${user.display_name} (@${user.username}) — ${t('app.name')}`);
	const description = $derived(user.bio ?? `${user.display_name} on ${t('app.name')}`);
	// JSON-LD: Person schema (Section 14 / Section 26). The backend resolves
	// the admin-managed Person template server-side and returns it in the
	// `json_ld` sidecar. Fall back to a basic schema when absent.
	const jsonLd = $derived(
		data.jsonLd
			? JSON.stringify(data.jsonLd)
			: JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'Person',
					name: user.display_name,
					alternateName: user.username ? `@${user.username}` : undefined,
					url: page.url.href,
					description: user.bio ?? '',
					...(user.avatar_url ? { image: user.avatar_url } : {})
				})
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="profile" />
	<meta property="og:url" content={page.url.href} />
	{@html `<script type="application/ld+json">${jsonLd}</script>`}
</svelte:head>

<SetSidebar>
	<UserProfileSidebar
		certificates={data.certificates}
		spaces={data.spaces}
	/>
</SetSidebar>

<ProfileHeader {user} {isOwnProfile}>
	{#if !isOwnProfile}
		<FollowButton entityType="user" entityId={user.id} {initiallyFollowing} />
	{/if}
</ProfileHeader>

{#if data.badges && data.badges.length > 0}
	<div class="mx-auto w-full max-w-full px-3 pb-1">
		<div class="flex flex-wrap gap-2">
			{#each data.badges as badge (badge.id)}
				<span
					class="inline-flex items-center gap-1.5 rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1 text-xs text-[var(--color-text)]"
					title={badge.description}
				>
					<Award size={12} class="text-[var(--color-primary)]" />
					{badge.name}
				</span>
			{/each}
		</div>
	</div>
{/if}

<ProfileTabs {tabs} active={activeTab} />

<div class="mx-auto w-full max-w-full">
	<div class="p-3">
		{#if activeTab === 'posts'}
			{#if data.posts.length === 0}
				<EmptyState icon={FileText} title={t('profile.posts_empty', 'No posts yet')} />
			{:else}
				<div class="divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					{#each data.posts as post (post.id)}
						<a href="/content/{post.slug}" class="block p-4 transition-colors hover:bg-[var(--color-surface-subtle)]">
							<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
								<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 font-medium text-[var(--color-text-subtle)]">{post.content_type}</span>
								{#if post.organization}
									<span class="text-[var(--color-primary)]">{post.organization.name}</span>
								{/if}
								<span>· {post.read_time_minutes} {t('feed.min_read')}</span>
							</div>
							<h3 class="mt-1.5 text-sm font-medium text-[var(--color-text)]">{post.title}</h3>
							{#if post.excerpt}
								<p class="mt-1 line-clamp-2 text-xs text-[var(--color-muted)]">{post.excerpt}</p>
							{/if}
							<div class="mt-2 flex items-center gap-4 text-xs text-[var(--color-muted)]">
								<span>{post.view_count} {t('profile.views', 'views')}</span>
								<span>{post.comment_count} {t('profile.comments', 'comments')}</span>
								<span>{post.share_count} {t('profile.shares', 'shares')}</span>
							</div>
						</a>
					{/each}
				</div>
			{/if}
		{:else if activeTab === 'about'}
			<!-- Phase 11.06 — About tab -->
			<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">
					{t('profile.about', 'About')}
				</h2>
				{#if user.bio}
					<p class="text-sm leading-relaxed text-[var(--color-text)]">{user.bio}</p>
				{:else}
					<EmptyState icon={Info} title={t('profile.no_about', 'No bio yet')} />
				{/if}
				<dl class="mt-4 space-y-2 border-t border-[var(--color-border)] pt-4">
					<div class="flex justify-between gap-4">
						<dt class="text-sm text-[var(--color-muted)]">{t('profile.joined', 'Joined')}</dt>
						<dd class="text-sm text-[var(--color-text)]">
							{new Date(user.created_at).toLocaleDateString(undefined, { year: 'numeric', month: 'long' })}
						</dd>
					</div>
					<div class="flex justify-between gap-4">
						<dt class="text-sm text-[var(--color-muted)]">{t('profile.followers_label', 'Followers')}</dt>
						<dd class="text-sm text-[var(--color-text)]">{user.follower_count}</dd>
					</div>
					<div class="flex justify-between gap-4">
						<dt class="text-sm text-[var(--color-muted)]">{t('profile.following_label', 'Following')}</dt>
						<dd class="text-sm text-[var(--color-text)]">{user.following_count}</dd>
					</div>
				</dl>
			</section>
		{:else if activeTab === 'replies'}
			{#if data.replies.length === 0}
				<EmptyState icon={MessageSquare} title={t('profile.replies_empty', 'No replies yet')} />
			{:else}
				<ul class="space-y-2">
					{#each data.replies as reply (reply.id)}
						<li class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<p class="text-sm text-[var(--color-text)]">{reply.body}</p>
							<a href="/content/{reply.content_slug}" class="mt-2 block text-xs text-[var(--color-primary)] hover:underline">
								{t('profile.in_reply_to', 'In reply to')}: {reply.content_title}
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		{:else if activeTab === 'media'}
			{#if data.media.length === 0}
				<EmptyState icon={Image} title={t('profile.media_empty', 'No media yet')} />
			{:else}
				<div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
					{#each data.media as item (item.id)}
						<div class="aspect-square overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)]">
							<img src={item.url} alt={item.alt_text ?? ''} class="h-full w-full object-cover" loading="lazy" />
						</div>
					{/each}
				</div>
			{/if}
		{:else if activeTab === 'likes'}
			{#if data.likes.length === 0}
				<EmptyState icon={Heart} title={t('profile.likes_empty', 'No liked posts yet')} />
			{:else}
				<ul class="space-y-2">
					{#each data.likes as item (item.id)}
						<li>
							<a href="/content/{item.slug}" class="block rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:bg-[var(--color-surface-subtle)]">
								<span class="text-xs text-[var(--color-muted)]">{item.content_type}</span>
								<h3 class="mt-1 text-sm font-medium text-[var(--color-text)]">{item.title}</h3>
								{#if item.excerpt}
									<p class="mt-1 line-clamp-2 text-xs text-[var(--color-muted)]">{item.excerpt}</p>
								{/if}
								<div class="mt-1.5 flex items-center gap-2 text-xs text-[var(--color-muted)]">
									{#if item.author_name}<span>{item.author_name}</span>{/if}
									<span>· {item.view_count} views</span>
								</div>
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		{:else if activeTab === 'spaces'}
			{#if data.spaces.length === 0}
				<EmptyState icon={Users} title={t('profile.spaces_empty', 'No spaces yet')} />
			{:else}
				<ul class="space-y-2">
					{#each data.spaces as space (space.id)}
						<li>
							<a
								href={`/spaces/${space.slug}`}
								class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								<span class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">{space.name.charAt(0).toUpperCase()}</span>
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium text-[var(--color-text)]">{space.name}</span>
									{#if space.role}
										<span class="block text-xs capitalize text-[var(--color-muted)]">{space.role}</span>
									{/if}
								</span>
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		{:else if activeTab === 'analytics'}
			<!-- Phase 11.07 — Analytics tab -->
			{#if analytics}
				<div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
							<BarChart3 size={14} /> {t('profile.analytics.views', 'Total views')}
						</div>
						<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.total_views ?? 0}</p>
					</div>
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
							<Heart size={14} /> {t('profile.analytics.likes', 'Total likes')}
						</div>
						<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.total_likes ?? 0}</p>
					</div>
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
							<FileText size={14} /> {t('profile.analytics.posts', 'Total posts')}
						</div>
						<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.total_posts ?? 0}</p>
					</div>
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
							<Users size={14} /> {t('profile.analytics.followers', 'Followers')}
						</div>
						<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.total_followers ?? 0}</p>
					</div>
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
							<Users size={14} /> {t('profile.analytics.following', 'Following')}
						</div>
						<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.total_following ?? 0}</p>
					</div>
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
							<FileText size={14} /> {t('profile.analytics.shares', 'Total shares')}
						</div>
						<p class="mt-1 text-lg font-semibold text-[var(--color-text)]">{analytics.total_shares ?? 0}</p>
					</div>
				</div>
			{:else}
				<EmptyState icon={BarChart3} title={t('profile.analytics.unavailable', 'Analytics unavailable')} />
			{/if}
		{/if}
	</div>
</div>
