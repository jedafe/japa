<script lang="ts">
	// Ref: Section 59 (Sidebar Management System), ADR-066.
	// Renders a single sidebar widget based on its widget_type. Each widget type
	// maps to a specific UI component. Widget props configure behavior (limit,
	// title, slot, format, etc.). Data-fetching widgets (trending_tags,
	// suggested_spaces) receive their data from the parent RightSidebar via
	// props, since the data is already fetched SSR in +layout.server.ts.

	import { Sun, Moon, Languages, Bell, Mail, Share2, Calendar, Trophy, Hash, Star, Bookmark, BarChart3, MousePointerClick, UserCircle, FolderTree, MessageSquare, PlayCircle, GraduationCap, BookOpen, CircleDashed } from 'lucide-svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import { t, i18n } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import SearchAutocomplete from './SearchAutocomplete.svelte';
	import UserDropdown from './UserDropdown.svelte';
	import NotificationCentre from './NotificationCentre.svelte';
	import { AdSlot } from '$lib/components/ui';
	import type { SessionUser, TagSuggestion, SpaceSuggestion } from '$lib/types';

	interface SidebarWidget {
		id: string;
		widget_type: string;
		label: string;
		props: Record<string, unknown>;
		sort_order: number;
	}

	interface Props {
		widget: SidebarWidget;
		user: SessionUser | null;
		trendingTags?: TagSuggestion[];
		suggestedSpaces?: SpaceSuggestion[];
		enabledLanguages?: string;
		isDashboard?: boolean;
	}

	let {
		widget,
		user,
		trendingTags = [],
		suggestedSpaces = [],
		enabledLanguages = 'en,fr',
		isDashboard = false
	}: Props = $props();

	const widgetProps = $derived(widget.props ?? {});
	const limit = $derived(typeof widgetProps.limit === 'number' ? widgetProps.limit : 8);
	const title = $derived(typeof widgetProps.title === 'string' ? widgetProps.title : '');
	const adSlot = $derived(typeof widgetProps.slot === 'string' ? widgetProps.slot : 'sidebar_top');
	const adFormat = $derived(
		(typeof widgetProps.format === 'string' && ['square', 'banner', 'inline'].includes(widgetProps.format))
			? widgetProps.format as 'square' | 'banner' | 'inline'
			: 'square' as const
	);

	let showLanguageToggle = $derived(enabledLanguages.split(',').filter(Boolean).length > 1);

	function currentPath(): string {
		if (typeof window === 'undefined') return '';
		return window.location.pathname.slice(1);
	}

	// --- Client-side data fetching for data-driven widgets (Rule 16: API-only) ---
	// Each fetchable widget type has its own $state + $effect. Widgets without a
	// backend endpoint render an empty state instead of a permanent "Loading…".

	interface FeaturedPost { id: string; title: string; slug: string; }
	interface SpaceItem { id: string; name: string; slug: string; member_count: number; }
	interface RecentContentItem { id: string; title: string; slug: string; content_type: string; }
	interface BookmarkFolderItem { id: string; name: string; bookmark_count?: number; }
	interface EnrollmentItem { id: string; track_id: string; track_title?: string; progress_percent?: number; }
	interface LeaderboardUser { id: string; username: string; display_name?: string; avatar_url?: string; score: number; }
	interface EventItem { id: string; title: string; starts_at?: string; url?: string; }

	let featuredPosts = $state<FeaturedPost[]>([]);
	let spaces = $state<SpaceItem[]>([]);
	let recentContent = $state<RecentContentItem[]>([]);
	let bookmarkFolders = $state<BookmarkFolderItem[]>([]);
	let enrollments = $state<EnrollmentItem[]>([]);
	let leaderboard = $state<LeaderboardUser[]>([]);
	let events = $state<EventItem[]>([]);
	let dataLoaded = $state<Record<string, boolean>>({});

	// Newsletter subscribe form state (Rule 16: calls POST /newsletter/subscribe).
	let newsletterEmail = $state('');
	let newsletterSubmitting = $state(false);
	let newsletterMessage = $state<{ kind: 'success' | 'error' | 'info'; text: string } | null>(null);

	async function handleNewsletterSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (newsletterSubmitting) return;
		const email = newsletterEmail.trim();
		if (!email || !email.includes('@') || !email.includes('.')) {
			newsletterMessage = { kind: 'error', text: t('sidebar.newsletter_invalid') };
			return;
		}
		newsletterSubmitting = true;
		newsletterMessage = null;
		const res = await api.post<{ status?: string }>('/newsletter/subscribe', { email });
		newsletterSubmitting = false;
		if (res.error) {
			newsletterMessage = { kind: 'error', text: t('sidebar.newsletter_error') };
			return;
		}
		if (res.data?.status === 'already_subscribed') {
			newsletterMessage = { kind: 'info', text: t('sidebar.newsletter_already') };
		} else {
			newsletterMessage = { kind: 'success', text: t('sidebar.newsletter_success') };
			newsletterEmail = '';
		}
	}

	$effect(() => {
		const wt = widget.widget_type;
		if (dataLoaded[wt]) return;

		async function fetchFeatured() {
			const res = await api.get<FeaturedPost[]>('/blog/featured');
			featuredPosts = res.data ?? [];
			dataLoaded = { ...dataLoaded, featured_content: true };
		}
		async function fetchSpaces() {
			const res = await api.get<SpaceItem[]>(`/spaces?page_size=${limit}`);
			spaces = res.data ?? [];
			dataLoaded = { ...dataLoaded, space_list: true };
		}
		async function fetchRecent() {
			if (!user) { dataLoaded = { ...dataLoaded, recent_content: true }; return; }
			const res = await api.get<RecentContentItem[]>(`/content?sort=recent&page_size=${limit}`);
			recentContent = res.data ?? [];
			dataLoaded = { ...dataLoaded, recent_content: true };
		}
		async function fetchBookmarks() {
			if (!user) { dataLoaded = { ...dataLoaded, bookmark_folders: true }; return; }
			const res = await api.get<BookmarkFolderItem[]>('/me/bookmark-folders');
			bookmarkFolders = res.data ?? [];
			dataLoaded = { ...dataLoaded, bookmark_folders: true };
		}
		async function fetchEnrollments() {
			if (!user) { dataLoaded = { ...dataLoaded, learning_enrollments: true }; return; }
			const res = await api.get<EnrollmentItem[]>('/me/enrollments');
			enrollments = res.data ?? [];
			dataLoaded = { ...dataLoaded, learning_enrollments: true };
		}
		async function fetchLeaderboard() {
			const res = await api.get<LeaderboardUser[]>(`/trending/users?limit=${limit}`);
			leaderboard = res.data ?? [];
			dataLoaded = { ...dataLoaded, leaderboard: true };
		}
		async function fetchEvents() {
			// The events feature is not yet implemented in JEOS (no section or
			// route catalog entry for /events). Skip the network call to avoid
			// console 404 errors on every page load; render the empty state
			// directly. Restore the fetch once the events section is added.
			events = [];
			dataLoaded = { ...dataLoaded, upcoming_events: true };
		}

		if (wt === 'featured_content') fetchFeatured();
		else if (wt === 'space_list') fetchSpaces();
		else if (wt === 'recent_content') fetchRecent();
		else if (wt === 'bookmark_folders') fetchBookmarks();
		else if (wt === 'learning_enrollments' || wt === 'continue_learning') fetchEnrollments();
		else if (wt === 'leaderboard') fetchLeaderboard();
		else if (wt === 'upcoming_events') fetchEvents();
	});
</script>

{#if widget.widget_type === 'search'}
	<SearchAutocomplete />
{:else if widget.widget_type === 'theme_toggle'}
	<button
		type="button"
		onclick={() => theme.toggle()}
		class="grid h-9 w-9 shrink-0 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
		aria-label={theme.current === 'dark' ? t('theme.toggle_light') : t('theme.toggle_dark')}
	>
		{#if theme.current === 'dark'}<Sun size={18} />{:else}<Moon size={18} />{/if}
	</button>
{:else if widget.widget_type === 'language_toggle'}
	{#if showLanguageToggle}
		<a
			href={i18n.locale === 'fr' ? `/en/${currentPath()}` : `/fr/${currentPath()}`}
			class="grid h-9 w-9 shrink-0 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={i18n.locale === 'fr' ? t('locale.switch_to_en') : t('locale.switch_to_fr')}
			title={i18n.locale === 'fr' ? 'English' : 'Français'}
		>
			<Languages size={18} />
		</a>
	{/if}
{:else if widget.widget_type === 'user_dropdown'}
	<UserDropdown {user} />
{:else if widget.widget_type === 'ad_slot'}
	{#if !isDashboard}
		<AdSlot slot={adSlot} format={adFormat} />
	{/if}
{:else if widget.widget_type === 'trending_tags'}
	{#if trendingTags.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.trending_tags')}</h2>
			<ul class="flex flex-wrap gap-2">
				{#each trendingTags.slice(0, limit) as tag (tag.id)}
					<li>
						<a
							href={`/tags/${tag.slug}`}
							class="inline-block rounded-full bg-[var(--color-surface-subtle)] px-3 py-1 text-xs text-[var(--color-text-subtle)] hover:text-[var(--color-primary)]"
							>#{tag.name}</a
						>
					</li>
				{/each}
			</ul>
		</section>
	{/if}
{:else if widget.widget_type === 'suggested_spaces'}
	{#if suggestedSpaces.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.suggested_spaces')}</h2>
			<ul class="space-y-2">
				{#each suggestedSpaces.slice(0, limit) as space (space.id)}
					<li>
						<a href={`/spaces/${space.slug}`} class="flex items-center gap-2 rounded-[var(--radius)] p-1.5 hover:bg-[var(--color-surface-subtle)]">
							<span class="grid h-8 w-8 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">{space.name.charAt(0).toUpperCase()}</span>
							<span class="min-w-0 flex-1">
								<span class="block truncate text-sm text-[var(--color-text)]">{space.name}</span>
								{#if space.member_count != null}
									<span class="block text-xs text-[var(--color-muted)]">{t('meta.members', '{n} members').replace('{n}', String(space.member_count))}</span>
								{/if}
							</span>
						</a>
					</li>
				{/each}
			</ul>
		</section>
	{/if}
{:else if widget.widget_type === 'rich_text'}
	<!-- Rich text widget — renders TipTap JSON content. Falls back to plain text
	     from props.content for simple use cases. Full TipTap rendering would use
	     the existing RichTextBlock component. -->
	{#if typeof widgetProps.content === 'string' && widgetProps.content.trim()}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 prose prose-sm max-w-none">
			{@html widgetProps.content}
		</section>
	{/if}
{:else if widget.widget_type === 'custom_html'}
	<!-- Custom HTML widget — renders raw HTML in a sandboxed container. -->
	{#if typeof widgetProps.html === 'string' && widgetProps.html.trim()}
		<section class="sidebar-widget-custom-html">
			{@html widgetProps.html}
		</section>
	{/if}
{:else if widget.widget_type === 'recent_content'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.recent_posts', 'Recent Posts')}</h2>
		{#if !user}
			<p class="text-xs text-[var(--color-muted)]">{t('sidebar.sign_in_to_see', 'Sign in to see recent posts')}</p>
		{:else if recentContent.length === 0}
			<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_content', 'No recent posts')}</p>
		{:else}
			<ul class="space-y-2">
				{#each recentContent.slice(0, limit) as item (item.id)}
					<li>
						<a href={`/content/${item.slug}`} class="block truncate text-sm text-[var(--color-text)] hover:text-[var(--color-primary)]">{item.title}</a>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{:else if widget.widget_type === 'space_list'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.spaces', 'Spaces')}</h2>
		{#if spaces.length === 0}
			<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_spaces', 'No spaces available')}</p>
		{:else}
			<ul class="space-y-2">
				{#each spaces.slice(0, limit) as space (space.id)}
					<li>
						<a href={`/spaces/${space.slug}`} class="flex items-center gap-2 rounded-[var(--radius)] p-1.5 hover:bg-[var(--color-surface-subtle)]">
							<span class="grid h-8 w-8 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">{space.name.charAt(0).toUpperCase()}</span>
							<span class="min-w-0 flex-1">
								<span class="block truncate text-sm text-[var(--color-text)]">{space.name}</span>
								<span class="block text-xs text-[var(--color-muted)]">{t('meta.members', '{n} members').replace('{n}', String(space.member_count))}</span>
							</span>
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{:else if widget.widget_type === 'notification_centre'}
	{#if user}
		<NotificationCentre side="bottom" align="end" />
	{/if}
{:else if widget.widget_type === 'metric_card'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="flex items-center gap-2 text-[var(--color-text-subtle)]">
			<BarChart3 size={16} />
			<span class="text-xs font-medium">{typeof widgetProps.label === 'string' ? widgetProps.label : 'Metric'}</span>
		</div>
		<p class="mt-2 text-2xl font-bold text-[var(--color-text)]">{typeof widgetProps.value === 'string' ? widgetProps.value : '—'}</p>
		{#if typeof widgetProps.delta === 'string' && widgetProps.delta}
			<p class="mt-1 text-xs text-[var(--color-text-subtle)]">{widgetProps.delta}</p>
		{/if}
	</section>
{:else if widget.widget_type === 'newsletter_signup'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<Mail size={18} class="text-[var(--color-primary)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.newsletter')}</h2>
		</div>
		<form class="space-y-2" onsubmit={handleNewsletterSubmit}>
			<input
				type="email"
				bind:value={newsletterEmail}
				placeholder={typeof widgetProps.placeholder === 'string' ? widgetProps.placeholder : 'your@email.com'}
				disabled={newsletterSubmitting}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none disabled:opacity-60"
			/>
			<button
				type="submit"
				disabled={newsletterSubmitting}
				class="w-full rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-60"
			>
				{typeof widgetProps.button_text === 'string' ? widgetProps.button_text : t('sidebar.subscribe')}
			</button>
		</form>
		{#if newsletterMessage}
			<p
				class="mt-2 text-xs {newsletterMessage.kind === 'error'
					? 'text-[var(--color-danger)]'
					: newsletterMessage.kind === 'success'
						? 'text-[var(--color-success)]'
						: 'text-[var(--color-muted)]'}"
			>
				{newsletterMessage.text}
			</p>
		{/if}
	</section>
{:else if widget.widget_type === 'social_links'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		{#if title}<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title}</h2>{/if}
		<div class="flex flex-wrap gap-2">
			{#each (Array.isArray(widgetProps.links) ? widgetProps.links : []) as link (link.platform)}
				{#if typeof link.url === 'string' && link.url}
					<a
						href={link.url}
						target="_blank"
						rel="noopener noreferrer"
						class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)] hover:bg-[var(--color-primary)] hover:text-[var(--color-primary-contrast)]"
						aria-label={link.platform}
					>
						<Share2 size={16} />
					</a>
				{/if}
			{/each}
		</div>
	</section>
{:else if widget.widget_type === 'cta_button'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<a
			href={typeof widgetProps.url === 'string' ? widgetProps.url : '/signup'}
			class="flex w-full items-center justify-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2.5 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
		>
			<MousePointerClick size={16} />
			{typeof widgetProps.label === 'string' ? widgetProps.label : 'Get Started'}
		</a>
	</section>
{:else if widget.widget_type === 'tag_cloud'}
	{#if trendingTags.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || 'Popular Tags'}</h2>
			<div class="flex flex-wrap gap-1.5">
				{#each trendingTags.slice(0, limit) as tag, i (tag.id)}
					<a
						href={`/tags/${tag.slug}`}
						class="inline-block rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-1 text-[var(--color-text-subtle)] hover:text-[var(--color-primary)]"
						style="font-size: {0.7 + (i < 5 ? 0.3 : 0)}rem"
					>#{tag.name}</a>
				{/each}
			</div>
		</section>
	{/if}
{:else if widget.widget_type === 'leaderboard'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<Trophy size={18} class="text-[var(--color-primary)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.top_contributors')}</h2>
		</div>
		{#if leaderboard.length === 0}
			<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_leaderboard')}</p>
		{:else}
			<ul class="space-y-2">
				{#each leaderboard.slice(0, limit) as entry, i (entry.id)}
					<li>
						<a href={`/u/${entry.username}`} class="flex items-center gap-2 rounded-[var(--radius)] p-1.5 hover:bg-[var(--color-surface-subtle)]">
							<span class="grid h-6 w-6 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-xs font-semibold text-[var(--color-text-subtle)]">{i + 1}</span>
							{#if entry.avatar_url}
								<img src={entry.avatar_url} alt="" class="h-8 w-8 shrink-0 rounded-full object-cover" />
							{:else}
								<span class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-xs font-medium text-[var(--color-text-subtle)]">{(entry.display_name ?? entry.username).charAt(0).toUpperCase()}</span>
							{/if}
							<span class="min-w-0 flex-1">
								<span class="block truncate text-sm text-[var(--color-text)]">{entry.display_name ?? entry.username}</span>
								<span class="block text-xs text-[var(--color-muted)]">@{entry.username}</span>
							</span>
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{:else if widget.widget_type === 'featured_content'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<Star size={18} class="text-[var(--color-primary)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.featured', 'Featured')}</h2>
		</div>
		{#if featuredPosts.length === 0}
			<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_featured', 'No featured content')}</p>
		{:else}
			<ul class="space-y-2">
				{#each featuredPosts.slice(0, limit) as post (post.id)}
					<li>
						<a href={`/blog/${post.slug}`} class="block truncate text-sm text-[var(--color-text)] hover:text-[var(--color-primary)]">{post.title}</a>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{:else if widget.widget_type === 'related_discussions'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<MessageSquare size={18} class="text-[var(--color-text-subtle)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.related_discussions', 'Related Discussions')}</h2>
		</div>
		<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_discussions', 'No related discussions')}</p>
	</section>
{:else if widget.widget_type === 'upcoming_events'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<Calendar size={18} class="text-[var(--color-text-subtle)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.upcoming_events')}</h2>
		</div>
		{#if events.length === 0}
			<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_events')}</p>
		{:else}
			<ul class="space-y-2">
				{#each events.slice(0, limit) as event (event.id)}
					<li>
						<a href={event.url ?? '#'} class="block text-sm text-[var(--color-text)] hover:text-[var(--color-primary)]">
							<span class="block truncate">{event.title}</span>
							{#if event.starts_at}
								<span class="mt-0.5 block text-xs text-[var(--color-muted)]">{new Date(event.starts_at).toLocaleDateString()}</span>
							{/if}
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{:else if widget.widget_type === 'bookmark_folders'}
	{#if user}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<div class="mb-3 flex items-center gap-2">
				<Bookmark size={18} class="text-[var(--color-text-subtle)]" />
				<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.bookmarks', 'Bookmarks')}</h2>
			</div>
			{#if bookmarkFolders.length === 0}
				<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_bookmarks', 'No bookmark folders')}</p>
			{:else}
				<ul class="space-y-1.5">
					{#each bookmarkFolders.slice(0, limit) as folder (folder.id)}
						<li>
							<a href={`/bookmarks?folder=${folder.id}`} class="flex items-center justify-between text-sm text-[var(--color-text)] hover:text-[var(--color-primary)]">
								<span class="truncate">{folder.name}</span>
								{#if folder.bookmark_count != null}
									<span class="text-xs text-[var(--color-muted)]">{folder.bookmark_count}</span>
								{/if}
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	{/if}
{:else if widget.widget_type === 'user_card'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<UserCircle size={18} class="text-[var(--color-text-subtle)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.featured_user', 'Featured User')}</h2>
		</div>
		<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_user', 'No user to display')}</p>
	</section>
{:else if widget.widget_type === 'space_categories'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<FolderTree size={18} class="text-[var(--color-text-subtle)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.browse_spaces', 'Browse Spaces')}</h2>
		</div>
		<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_categories', 'No categories available')}</p>
	</section>
{:else if widget.widget_type === 'continue_learning'}
	{#if user}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<div class="mb-3 flex items-center gap-2">
				<PlayCircle size={18} class="text-[var(--color-primary)]" />
				<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.continue_learning', 'Continue Learning')}</h2>
			</div>
			{#if enrollments.length === 0}
				<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_enrollments', 'No active enrollments')}</p>
			{:else}
				<ul class="space-y-2">
					{#each enrollments.slice(0, limit) as enr (enr.id)}
						<li>
							<a href={`/tracks/${enr.track_id}`} class="block text-sm text-[var(--color-text)] hover:text-[var(--color-primary)]">
								<span class="block truncate">{enr.track_title ?? t('sidebar.track', 'Track')}</span>
								{#if enr.progress_percent != null}
									<span class="mt-0.5 block text-xs text-[var(--color-muted)]">{enr.progress_percent}%</span>
								{/if}
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	{/if}
{:else if widget.widget_type === 'learning_enrollments'}
	{#if user}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<div class="mb-3 flex items-center gap-2">
				<GraduationCap size={18} class="text-[var(--color-primary)]" />
				<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.my_learning', 'My Learning')}</h2>
			</div>
			{#if enrollments.length === 0}
				<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_enrollments', 'No active enrollments')}</p>
			{:else}
				<ul class="space-y-2">
					{#each enrollments.slice(0, limit) as enr (enr.id)}
						<li>
							<a href={`/tracks/${enr.track_id}`} class="block text-sm text-[var(--color-text)] hover:text-[var(--color-primary)]">
								<span class="block truncate">{enr.track_title ?? t('sidebar.track', 'Track')}</span>
								{#if enr.progress_percent != null}
									<span class="mt-0.5 block text-xs text-[var(--color-muted)]">{enr.progress_percent}%</span>
								{/if}
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		</section>
	{/if}
{:else if widget.widget_type === 'progress_ring'}
	{#if user}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-center">
			<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.my_progress', 'My Progress')}</h2>
			<div class="mx-auto grid h-20 w-20 place-items-center">
				<CircleDashed size={64} class="text-[var(--color-border)]" />
			</div>
			<p class="mt-2 text-xs text-[var(--color-muted)]">{t('sidebar.no_progress', 'No progress data')}</p>
		</section>
	{/if}
{:else if widget.widget_type === 'track_card'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<BookOpen size={18} class="text-[var(--color-primary)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.featured_track', 'Featured Track')}</h2>
		</div>
		<p class="text-xs text-[var(--color-muted)]">{t('sidebar.no_track', 'No track to display')}</p>
	</section>
{:else if widget.widget_type === 'table_of_contents'}
	<!-- TOC widget — rendered by the Docs app; no-op in Main app. -->
{:else if widget.widget_type === 'search_launcher'}
	<!-- Search launcher — Docs/Learn variant; Main app uses 'search' widget. -->
{:else if widget.widget_type === 'helpful_widget'}
	<!-- Helpful widget — rendered by the Docs app; no-op in Main app. -->
{:else if widget.widget_type === 'curriculum_rail'}
	<!-- Curriculum rail — rendered by the Learn app on lesson pages; no-op in Main. -->
{:else}
	<!-- Unknown widget type — render nothing rather than crashing. -->
{/if}
