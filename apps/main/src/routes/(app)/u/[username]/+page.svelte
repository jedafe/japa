<script lang="ts">
	import { t, i18n } from '$lib/i18n';
	import { page } from '$app/state';
	import { FileText, MessageSquare, Image, Heart, Users, Info, BarChart3, Award, Pin } from 'lucide-svelte';
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

	// Phase 34.03 — earned + unearned badges. On the own profile we also have
	// the full catalog (data.allBadges); unearned badges surface a "how to earn"
	// hint via their description. Earned badges are always shown (public route).
	const earnedSlugs = $derived(new Set((data.badges ?? []).map((b) => b.slug)));
	const unearnedBadges = $derived(
		isOwnProfile
			? (data.allBadges ?? []).filter((b) => b.is_active && !earnedSlugs.has(b.slug))
			: []
	);

	const tabs = $derived([
		{ key: 'posts', labelKey: 'profile.tab.posts', href: `?tab=posts` },
		{ key: 'about', labelKey: 'profile.tab.about', href: `?tab=about` },
		{ key: 'replies', labelKey: 'profile.tab.replies', href: `?tab=replies` },
		{ key: 'media', labelKey: 'profile.tab.media', href: `?tab=media` },
		{ key: 'likes', labelKey: 'profile.tab.likes', href: `?tab=likes` },
		{ key: 'spaces', labelKey: 'profile.tab.spaces', href: `?tab=spaces` },
		// Phase 34.03 — Badges tab (Section 64, ADR-080). Shown when the user
		// has earned badges (public) or when viewing own profile (unearned
		// badges from the full catalog are also surfaced with "how to earn"
		// hints).
		...((data.badges && data.badges.length > 0) || unearnedBadges.length > 0
			? [{ key: 'badges', labelKey: 'profile.tab.badges', href: `?tab=badges` }]
			: []),
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
								{#if post.is_pinned}
									<span class="flex items-center gap-0.5 font-medium text-[var(--color-primary)]">
										<Pin size={10} /> {t('profile.pinned', 'Pinned')}
									</span>
								{/if}
								{#if post.organization}
									<span class="text-[var(--color-primary)]">{post.organization.name}</span>
								{/if}
								{#if post.published_at}
									<span>· {new Date(post.published_at).toLocaleDateString(i18n.locale, { year: 'numeric', month: 'short', day: 'numeric' })}</span>
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
		{:else if activeTab === 'badges'}
			<!-- Phase 34.03 — Badges tab (Section 64, ADR-080). Earned badges
			are public; unearned badges with "how to earn" hints are shown only
			on the own profile (data.allBadges is owner-only). -->
			{#if (data.badges && data.badges.length > 0) || unearnedBadges.length > 0}
				<div class="space-y-4">
					{#if data.badges && data.badges.length > 0}
						<div>
							<h3 class="mb-2 flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
								<Award size={12} /> {t('profile.earned', 'Earned')}
							</h3>
							<div class="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
								{#each data.badges as badge (badge.id)}
									<div
										class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-2"
										title={badge.description}
									>
										<span class="grid h-7 w-7 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-primary)]">
											<Award size={14} />
										</span>
										<span class="min-w-0 flex-1">
											<span class="block truncate text-xs font-medium text-[var(--color-text)]">{badge.name}</span>
											<span class="block text-[10px] text-[var(--color-muted)]">
												{t('profile.earned', 'Earned')} {new Date(badge.awarded_at).toLocaleDateString(undefined, { year: 'numeric', month: 'short' })}
											</span>
										</span>
									</div>
								{/each}
							</div>
						</div>
					{/if}
					{#if unearnedBadges.length > 0}
						<div>
							<h3 class="mb-2 flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
								<Award size={12} /> {t('profile.locked', 'Locked')}
							</h3>
							<div class="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
								{#each unearnedBadges as badge (badge.id)}
									<div
										class="flex items-center gap-2 rounded-[var(--radius)] border border-dashed border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2.5 py-2 opacity-70"
										title={t('profile.how_to_earn', 'How to earn') + ': ' + badge.description}
									>
										<span class="grid h-7 w-7 shrink-0 place-items-center rounded-full bg-[var(--color-surface)] text-[var(--color-muted)]">
											<Award size={14} />
										</span>
										<span class="min-w-0 flex-1">
											<span class="block truncate text-xs font-medium text-[var(--color-muted)]">{badge.name}</span>
											<span class="block text-[10px] text-[var(--color-muted)]">{t('profile.locked', 'Locked')}</span>
										</span>
									</div>
								{/each}
							</div>
						</div>
					{/if}
				</div>
			{:else}
				<EmptyState icon={Award} title={t('profile.badges', 'Badges')} />
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
