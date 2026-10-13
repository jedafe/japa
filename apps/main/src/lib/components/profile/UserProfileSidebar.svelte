<script lang="ts">
	// Route-specific right sidebar for /u/[username] — mockup card set.
	// Owner: Profile Stats, Achievements, Saved Items, Top Connections,
	// Quick Actions. Visitor: Community Badges, Recent Posts, Connections,
	// Liked Items, Report/Block. Certificates/Spaces cards are shared and
	// render only when they have data.
	import { t, i18n } from '$lib/i18n';
	import {
		Award,
		Ban,
		BarChart3,
		Bookmark as BookmarkIcon,
		Flag,
		GraduationCap,
		Heart,
		HelpCircle,
		Image as ImageIcon,
		Plane,
		School
	} from 'lucide-svelte';
	import { bookmarkLabel, bookmarkUrl } from '$lib/components/profile/bookmarks';
	import RelativeTime from '$lib/components/ui/RelativeTime.svelte';
	import type {
		Badge,
		Bookmark,
		EarnedBadge,
		ProfileCertificate,
		ProfileLike,
		ProfilePost,
		ProfileSpace,
		PublicUser
	} from '$lib/types';

	interface SidebarConnection {
		id: string;
		username: string;
		display_name: string;
		avatar_url?: string | null;
	}

	interface Props {
		user: PublicUser;
		isOwnProfile: boolean;
		isAuthenticated: boolean;
		badges: EarnedBadge[];
		lockedBadges?: Badge[];
		posts: ProfilePost[];
		likes: ProfileLike[];
		bookmarks: Bookmark[];
		connections: SidebarConnection[];
		spaces: ProfileSpace[];
		certificates: ProfileCertificate[];
		stats: { posts: number; connections: number; saved: number };
		savedLabel: string;
		/** Japa Journey summary — renders as the top card when the profiled
		 *  user has a public stage set. */
		journey?: { title: string; subtitle: string; percent: number; href: string } | null;
		onMessage?: (recipient: { id: string; name: string }) => void;
		onReport?: () => void;
		onToggleBlock?: () => void;
		isBlocked?: boolean;
		blockPending?: boolean;
	}
	let {
		user,
		isOwnProfile,
		isAuthenticated,
		badges,
		lockedBadges = [],
		posts,
		likes,
		bookmarks,
		connections,
		spaces,
		certificates,
		stats,
		savedLabel,
		journey = null,
		onMessage,
		onReport,
		onToggleBlock,
		isBlocked = false,
		blockPending = false
	}: Props = $props();

	// Cap sidebar lists — the matching profile tab carries the full list.
	const TOP = 3;
	const MAX_SIDEBAR_ITEMS = 6;
	const visibleSpaces = $derived(spaces.slice(0, MAX_SIDEBAR_ITEMS));
	const visibleBadges = $derived(badges.slice(0, 4));
	const visibleLocked = $derived(lockedBadges.slice(0, Math.max(0, 4 - visibleBadges.length)));
	const visiblePosts = $derived(posts.slice(0, TOP));
	const visibleConnections = $derived(connections.slice(0, TOP));
	const visibleSaved = $derived(bookmarks.slice(0, TOP));
	const visibleLikes = $derived(likes.slice(0, TOP));

	const badgeIconColors = [
		'bg-blue-500/10 text-blue-500',
		'bg-amber-500/10 text-amber-500',
		'bg-violet-500/10 text-violet-500',
		'bg-emerald-500/10 text-emerald-500'
	];
	function badgeColor(i: number): string {
		return badgeIconColors[i % badgeIconColors.length];
	}

	function formatDate(iso: string | undefined): string {
		if (!iso) return '';
		return new Date(iso).toLocaleDateString(i18n.locale);
	}

</script>

{#if journey}
	<!-- Japa Journey — top card of the widget area. -->
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<a href={journey.href} class="block">
			<span class="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-muted)]">
				<Plane size={12} class="text-[var(--color-primary)]" />
				{t('profile.journey.label', 'Japa Journey')}
			</span>
			<span class="mt-1.5 block truncate text-sm font-semibold text-[var(--color-text)]">{journey.title}</span>
			<span class="mt-2 flex items-center gap-2">
				<span class="h-1.5 flex-1 overflow-hidden rounded-full bg-[var(--color-surface-subtle)]">
					<span class="block h-full rounded-full bg-[var(--color-primary)]" style="width: {journey.percent}%"></span>
				</span>
				<span class="text-xs font-semibold text-[var(--color-primary)]">{journey.percent}%</span>
			</span>
			<span class="mt-1.5 block text-xs leading-snug text-[var(--color-muted)]">{journey.subtitle}</span>
			<span class="mt-2 block text-xs font-medium text-[var(--color-primary)]">
				{t('profile.journey.view', 'View Journey')} →
			</span>
		</a>
	</section>
{/if}

{#if isOwnProfile}
	<!-- Profile Stats — owner overview numbers -->
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-semibold text-[var(--color-text)]">
			{t('profile.sidebar.stats', 'Profile Stats')}
		</h2>
		<div class="grid grid-cols-3 text-center">
			<div>
				<p class="text-lg font-bold text-[var(--color-primary)]">{stats.posts.toLocaleString()}</p>
				<p class="text-xs text-[var(--color-muted)]">{t('profile.stats.posts', 'Posts')}</p>
			</div>
			<div>
				<p class="text-lg font-bold text-[var(--color-primary)]">{stats.connections.toLocaleString()}</p>
				<p class="text-xs text-[var(--color-muted)]">{t('profile.stats.connections', 'Connections')}</p>
			</div>
			<div>
				<p class="text-lg font-bold text-[var(--color-primary)]">{stats.saved.toLocaleString()}</p>
				<p class="text-xs text-[var(--color-muted)]">{savedLabel}</p>
			</div>
		</div>
	</section>
{/if}

{#if badges.length > 0 || (isOwnProfile && lockedBadges.length > 0)}
	<!-- Community Badges (visitor) / Achievements (owner) -->
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-semibold text-[var(--color-text)]">
			{isOwnProfile ? t('profile.sidebar.achievements', 'Achievements') : t('profile.sidebar.badges', 'Community Badges')}
		</h2>
		<ul class="space-y-2.5">
			{#each visibleBadges as badge, i (badge.id)}
				<li class="flex items-center gap-2.5" title={badge.description}>
					<span class="grid h-9 w-9 shrink-0 place-items-center rounded-full {badgeColor(i)}">
						<Award size={16} />
					</span>
					<span class="min-w-0 flex-1">
						<span class="block truncate text-sm font-medium text-[var(--color-text)]">{badge.name}</span>
						{#if badge.description}
							<span class="block truncate text-xs text-[var(--color-muted)]">{badge.description}</span>
						{/if}
					</span>
				</li>
			{/each}
			{#each visibleLocked as badge, i (badge.id)}
				<li class="flex items-center gap-2.5 opacity-60" title={badge.description}>
					<span class="grid h-9 w-9 shrink-0 place-items-center rounded-full {badgeColor(i + visibleBadges.length)}">
						<Award size={16} />
					</span>
					<span class="min-w-0 flex-1">
						<span class="block truncate text-sm font-medium text-[var(--color-muted)]">{badge.name}</span>
						<span class="block truncate text-xs text-[var(--color-muted)]">
							{t('profile.how_to_earn', 'How to earn')}: {badge.description}
						</span>
					</span>
				</li>
			{/each}
		</ul>
	</section>
{/if}

{#if !isOwnProfile && visiblePosts.length > 0}
	<!-- Recent Posts — visitor -->
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center justify-between">
			<h2 class="text-sm font-semibold text-[var(--color-text)]">{t('profile.sidebar.recent_posts', 'Recent Posts')}</h2>
			<a href="?tab=posts" class="text-xs font-medium text-[var(--color-primary)] hover:underline">{t('profile.view_all', 'View all')}</a>
		</div>
		<ul class="space-y-3">
			{#each visiblePosts as post (post.id)}
				<li>
					<a href="/content/{post.slug}" class="flex items-center gap-2.5">
						{#if post.cover_image_url}
							<img src={post.cover_image_url} alt="" class="h-11 w-11 shrink-0 rounded-[var(--radius)] object-cover" loading="lazy" />
						{:else}
							<span class="grid h-11 w-11 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-muted)]">
								<ImageIcon size={16} />
							</span>
						{/if}
						<span class="min-w-0 flex-1">
							<span class="block text-sm font-medium leading-snug text-[var(--color-text)] line-clamp-2">{post.title}</span>
							<span class="mt-0.5 flex items-center gap-2 text-xs text-[var(--color-muted)]">
								<RelativeTime iso={post.published_at ?? post.created_at} />
								{#if (post.reaction_count ?? 0) > 0}
									<span class="inline-flex items-center gap-0.5"><Heart size={11} /> {post.reaction_count}</span>
								{/if}
							</span>
						</span>
					</a>
				</li>
			{/each}
		</ul>
	</section>
{/if}

{#if visibleConnections.length > 0}
	<!-- Connections / Top Connections -->
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center justify-between">
			<h2 class="text-sm font-semibold text-[var(--color-text)]">
				{isOwnProfile ? t('profile.sidebar.top_connections', 'Top Connections') : t('profile.sidebar.connections', 'Connections')}
				<span class="font-normal text-[var(--color-muted)]">({stats.connections.toLocaleString()})</span>
			</h2>
			<a href="?tab=connections" class="text-xs font-medium text-[var(--color-primary)] hover:underline">{t('profile.view_all', 'View all')}</a>
		</div>
		<ul class="space-y-2.5">
			{#each visibleConnections as c (c.id)}
				<li class="flex items-center gap-2.5">
					<a href="/u/{c.username}" class="flex min-w-0 flex-1 items-center gap-2.5">
						{#if c.avatar_url}
							<img src={c.avatar_url} alt={c.display_name} class="h-9 w-9 shrink-0 rounded-full object-cover" loading="lazy" />
						{:else}
							<span class="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">
								{(c.display_name?.charAt(0) ?? c.username?.charAt(0) ?? '?').toUpperCase()}
							</span>
						{/if}
						<span class="min-w-0 flex-1">
							<span class="block truncate text-sm font-medium text-[var(--color-text)]">{c.display_name ?? c.username}</span>
							<span class="block truncate text-xs text-[var(--color-muted)]">@{c.username}</span>
						</span>
					</a>
					{#if isAuthenticated && onMessage}
						<button
							type="button"
							onclick={() => onMessage({ id: c.id, name: c.display_name ?? c.username })}
							class="shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] px-2.5 py-1 text-xs font-medium text-[var(--color-primary)] transition-colors hover:bg-[var(--color-primary)]/10"
						>
							{t('profile.message', 'Message')}
						</button>
					{/if}
				</li>
			{/each}
		</ul>
		{#if connections.length > visibleConnections.length}
			<a href="?tab=connections" class="mt-2 block text-xs font-medium text-[var(--color-primary)] hover:underline">
				{t('profile.view_all_connections', 'View all connections')} →
			</a>
		{/if}
	</section>
{/if}

{#if isOwnProfile && visibleSaved.length > 0}
	<!-- Saved Items — owner bookmarks -->
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center justify-between">
			<h2 class="text-sm font-semibold text-[var(--color-text)]">
				{t('profile.sidebar.saved', 'Saved Items')}
				<span class="font-normal text-[var(--color-muted)]">({stats.saved.toLocaleString()})</span>
			</h2>
			<a href="?tab=saved" class="text-xs font-medium text-[var(--color-primary)] hover:underline">{t('profile.view_all', 'View all')}</a>
		</div>
		<ul class="space-y-2.5">
			{#each visibleSaved as b (b.id)}
				<li>
					<a href={bookmarkUrl(b)} class="flex items-center gap-2.5">
						<span class="grid h-9 w-9 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
							<BookmarkIcon size={15} />
						</span>
						<span class="min-w-0 flex-1">
							<span class="block text-sm font-medium leading-snug text-[var(--color-text)] line-clamp-2">{bookmarkLabel(b)}</span>
							<span class="mt-0.5 block text-xs text-[var(--color-muted)]">
								{t('profile.saved_on', 'Saved')} <RelativeTime iso={b.created_at} />
							</span>
						</span>
					</a>
				</li>
			{/each}
		</ul>
	</section>
{:else if !isOwnProfile && visibleLikes.length > 0}
	<!-- Liked Items — visitor stand-in for the mockup's Saved Items card
	     (bookmarks are private; likes are the public equivalent). -->
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center justify-between">
			<h2 class="text-sm font-semibold text-[var(--color-text)]">
				{t('profile.sidebar.liked', 'Liked Items')}
				<span class="font-normal text-[var(--color-muted)]">({likes.length.toLocaleString()})</span>
			</h2>
			<a href="?tab=saved" class="text-xs font-medium text-[var(--color-primary)] hover:underline">{t('profile.view_all', 'View all')}</a>
		</div>
		<ul class="space-y-2.5">
			{#each visibleLikes as item (item.id)}
				<li>
					<a href="/content/{item.slug}" class="block">
						<span class="block text-sm font-medium leading-snug text-[var(--color-text)] line-clamp-2">{item.title}</span>
						<span class="mt-0.5 block text-xs text-[var(--color-muted)]">
							{t('profile.liked_on', 'Liked')} <RelativeTime iso={item.reacted_at} />
						</span>
					</a>
				</li>
			{/each}
		</ul>
	</section>
{/if}

{#if isOwnProfile}
	<!-- Quick Actions — owner shortcuts -->
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('profile.sidebar.quick_actions', 'Quick Actions')}</h2>
		<ul class="divide-y divide-[var(--color-border)]">
			<li>
				<a href="/forum" class="flex items-center gap-3 py-2.5">
					<span class="grid h-9 w-9 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]"><HelpCircle size={16} /></span>
					<span class="min-w-0 flex-1">
						<span class="block text-sm font-medium text-[var(--color-text)]">{t('profile.qa.ask', 'Ask a Question')}</span>
						<span class="block text-xs text-[var(--color-muted)]">{t('profile.qa.ask_desc', 'Get help from the community')}</span>
					</span>
					<span class="text-[var(--color-muted)]">›</span>
				</a>
			</li>
			<li>
				<a href="/scholarships" class="flex items-center gap-3 py-2.5">
					<span class="grid h-9 w-9 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]"><GraduationCap size={16} /></span>
					<span class="min-w-0 flex-1">
						<span class="block text-sm font-medium text-[var(--color-text)]">{t('profile.qa.scholarships', 'Find Scholarships')}</span>
						<span class="block text-xs text-[var(--color-muted)]">{t('profile.qa.scholarships_desc', 'Explore scholarship opportunities')}</span>
					</span>
					<span class="text-[var(--color-muted)]">›</span>
				</a>
			</li>
			<li>
				<a href="/universities" class="flex items-center gap-3 py-2.5">
					<span class="grid h-9 w-9 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]"><School size={16} /></span>
					<span class="min-w-0 flex-1">
						<span class="block text-sm font-medium text-[var(--color-text)]">{t('profile.qa.programs', 'Browse Study Programs')}</span>
						<span class="block text-xs text-[var(--color-muted)]">{t('profile.qa.programs_desc', 'Find programs abroad')}</span>
					</span>
					<span class="text-[var(--color-muted)]">›</span>
				</a>
			</li>
			<li>
				<a href="/dashboard/analytics" class="flex items-center gap-3 py-2.5">
					<span class="grid h-9 w-9 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]"><BarChart3 size={16} /></span>
					<span class="min-w-0 flex-1">
						<span class="block text-sm font-medium text-[var(--color-text)]">{t('profile.qa.analytics', 'View Analytics')}</span>
						<span class="block text-xs text-[var(--color-muted)]">{t('profile.qa.analytics_desc', 'Your profile performance')}</span>
					</span>
					<span class="text-[var(--color-muted)]">›</span>
				</a>
			</li>
		</ul>
	</section>
{/if}

{#if certificates.length > 0}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-semibold text-[var(--color-text)]">{t('profile.certificates', 'Certificates')}</h2>
		<ul class="space-y-2">
			{#each certificates as cert (cert.id)}
				<li>
					<a
						href={`/certificates/${cert.id}`}
						class="flex items-center gap-2 rounded-[var(--radius)] p-1.5 hover:bg-[var(--color-surface-subtle)]"
					>
						<span class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
							<Award size={16} />
						</span>
						<span class="min-w-0 flex-1">
							<span class="block truncate text-sm text-[var(--color-text)]">{cert.title ?? t('profile.unknown_certificate', 'Certificate')}</span>
							{#if cert.certificate_number}
								<span class="block text-xs text-[var(--color-muted)]">{cert.certificate_number}</span>
							{/if}
							{#if cert.issued_at}
								<span class="block text-xs text-[var(--color-muted)]">{formatDate(cert.issued_at)}</span>
							{/if}
						</span>
					</a>
				</li>
			{/each}
		</ul>
	</section>
{/if}

{#if spaces.length > 0}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-semibold text-[var(--color-text)]">{t('profile.spaces', 'Spaces')}</h2>
		<ul class="space-y-2">
			{#each visibleSpaces as space (space.id)}
				<li>
					<a
						href={`/spaces/${space.slug}`}
						class="flex items-center gap-2 rounded-[var(--radius)] p-1.5 hover:bg-[var(--color-surface-subtle)]"
					>
						<span class="grid h-8 w-8 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">{space.name.charAt(0).toUpperCase()}</span>
						<span class="min-w-0 flex-1">
							<span class="block truncate text-sm text-[var(--color-text)]">{space.name}</span>
							{#if space.role}
								<span class="block text-xs capitalize text-[var(--color-muted)]">{space.role}</span>
							{/if}
						</span>
					</a>
				</li>
			{/each}
		</ul>
		{#if spaces.length > visibleSpaces.length}
			<a
				href="?tab=spaces"
				class="mt-2 block text-xs font-medium text-[var(--color-primary)] hover:underline"
			>
				{t('profile.view_all_spaces', 'View all {count} spaces').replace('{count}', String(spaces.length))}
			</a>
		{/if}
	</section>
{/if}

{#if !isOwnProfile && isAuthenticated && (onReport || onToggleBlock)}
	<!-- Report / Block — visitor moderation actions -->
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">{t('profile.sidebar.report_block', 'Report / Block')}</h2>
		<div class="flex flex-col">
			{#if onReport}
				<button
					type="button"
					onclick={onReport}
					class="flex items-center gap-2 py-1.5 text-sm text-[var(--color-danger)] hover:underline"
				>
					<Flag size={14} /> {t('profile.sidebar.report', 'Report this profile')}
				</button>
			{/if}
			{#if onToggleBlock}
				<button
					type="button"
					onclick={onToggleBlock}
					disabled={blockPending}
					class="flex items-center gap-2 py-1.5 text-sm text-[var(--color-danger)] hover:underline disabled:opacity-50"
				>
					<Ban size={14} />
					{isBlocked ? t('profile.sidebar.unblock', 'Unblock this member') : t('profile.sidebar.block', 'Block this member')}
				</button>
			{/if}
		</div>
	</section>
{/if}
