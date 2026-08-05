<script lang="ts">
	// Ref: Section 59 (Sidebar Management System), ADR-066.
	// Renders a single sidebar widget based on its widget_type.
	// Handles content-slot widget types: ad_slot, rich_text, custom_html,
	// recent_content, space_list, trending_tags, suggested_spaces.
	// Topbar widget types (search, theme_toggle, language_toggle, user_dropdown)
	// are rendered by the app shell's hardcoded topbar and are skipped here.

	import { t } from '$lib/i18n';
	import { Sun, Moon, Languages, Bell, Mail, Share2, Calendar, Trophy, Hash, Star, Bookmark, BarChart3, MousePointerClick, UserCircle, FolderTree, MessageSquare, PlayCircle, GraduationCap, BookOpen, CircleDashed, ThumbsUp, List } from 'lucide-svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import { i18n } from '$lib/i18n';
	import { switchLocale } from '$lib/appOrigins';
	import SearchAutocomplete from './SearchAutocomplete.svelte';
	import ContinueLearningCard from './ContinueLearningCard.svelte';
	import ProgressRingWidget from './ProgressRingWidget.svelte';
	import LearningEnrollmentsWidget from './LearningEnrollmentsWidget.svelte';
	import TrackCardWidget from './TrackCardWidget.svelte';

	interface SidebarWidget {
		id: string;
		widget_type: string;
		label: string;
		props: Record<string, unknown>;
		sort_order: number;
	}

	interface Props {
		widget: SidebarWidget;
	}

	let { widget }: Props = $props();

	const widgetProps = $derived(widget.props ?? {});
	const title = $derived(typeof widgetProps.title === 'string' ? widgetProps.title : '');
	const adSlot = $derived(typeof widgetProps.slot === 'string' ? widgetProps.slot : 'sidebar_top');
	const adFormat = $derived(
		typeof widgetProps.format === 'string' && ['square', 'banner', 'inline'].includes(widgetProps.format)
			? (widgetProps.format as 'square' | 'banner' | 'inline')
			: 'square'
	);

	const adDimensions = {
		banner: 'h-[90px] w-full',
		square: 'h-[250px] w-full max-w-[300px]',
		inline: 'h-[60px] w-full'
	} as const;
</script>

{#if widget.widget_type === 'ad_slot'}
	<div
		class="grid place-items-center rounded-[var(--radius)] border border-dashed border-[var(--color-border)] bg-[var(--color-surface-subtle)] text-xs text-[var(--color-muted)] {adDimensions[adFormat]}"
		data-ad-slot={adSlot}
		aria-label={t('ads.label', 'Advertisement')}
	>
		<span>{t('ads.label', 'Advertisement')} · {adSlot}</span>
	</div>
{:else if widget.widget_type === 'rich_text'}
	{#if typeof widgetProps.content === 'string' && widgetProps.content.trim()}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 prose prose-sm max-w-none">
			{@html widgetProps.content}
		</section>
	{/if}
{:else if widget.widget_type === 'custom_html'}
	{#if typeof widgetProps.html === 'string' && widgetProps.html.trim()}
		<section class="sidebar-widget-custom-html">
			{@html widgetProps.html}
		</section>
	{/if}
{:else if widget.widget_type === 'recent_content'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || 'Recent Posts'}</h2>
		<p class="text-xs text-[var(--color-muted)]">Loading recent content…</p>
	</section>
{:else if widget.widget_type === 'space_list'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || 'Spaces'}</h2>
		<p class="text-xs text-[var(--color-muted)]">Loading spaces…</p>
	</section>
{:else if widget.widget_type === 'trending_tags'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.trending_tags', 'Trending Tags')}</h2>
		<p class="text-xs text-[var(--color-muted)]">Loading trending tags…</p>
	</section>
{:else if widget.widget_type === 'suggested_spaces'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.suggested_spaces', 'Suggested Spaces')}</h2>
		<p class="text-xs text-[var(--color-muted)]">Loading suggested spaces…</p>
	</section>
{:else if widget.widget_type === 'search'}
	<SearchAutocomplete />
{:else if widget.widget_type === 'search_launcher'}
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
	<button
		type="button"
		onclick={() => switchLocale(i18n.locale === 'fr' ? 'en' : 'fr')}
		class="grid h-9 w-9 shrink-0 place-items-center rounded-full text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
		aria-label={i18n.locale === 'fr' ? t('locale.switch_to_en') : t('locale.switch_to_fr')}
		title={i18n.locale === 'fr' ? 'English' : 'Français'}
	>
		<Languages size={18} />
	</button>
{:else if widget.widget_type === 'continue_learning'}
	<!-- continue_learning is rendered globally by RightSidebar (ContinueLearningCard).
	     The admin-managed widget is a no-op here to avoid a duplicate with user=null. -->
{:else if widget.widget_type === 'learning_enrollments'}
	<LearningEnrollmentsWidget {title} limit={typeof widgetProps.limit === 'number' ? widgetProps.limit : 3} />
{:else if widget.widget_type === 'progress_ring'}
	<ProgressRingWidget {title} />
{:else if widget.widget_type === 'track_card'}
	<TrackCardWidget {title} trackSlug={typeof widgetProps.track_slug === 'string' ? widgetProps.track_slug : ''} />
{:else if widget.widget_type === 'curriculum_rail'}
	<!-- Curriculum rail is rendered by the lesson player route snippet, not the widget renderer. -->
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
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.newsletter', 'Subscribe')}</h2>
		</div>
		<form class="space-y-2" onsubmit={(e) => e.preventDefault()}>
			<input
				type="email"
				placeholder={typeof widgetProps.placeholder === 'string' ? widgetProps.placeholder : 'your@email.com'}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
			<button type="submit" class="w-full rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]">
				{typeof widgetProps.button_text === 'string' ? widgetProps.button_text : t('sidebar.subscribe', 'Subscribe')}
			</button>
		</form>
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
{:else if widget.widget_type === 'leaderboard'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<Trophy size={18} class="text-[var(--color-primary)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || 'Top Contributors'}</h2>
		</div>
		<p class="text-xs text-[var(--color-muted)]">Loading leaderboard…</p>
	</section>
{:else if widget.widget_type === 'featured_content'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<Star size={18} class="text-[var(--color-primary)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || 'Featured'}</h2>
		</div>
		<p class="text-xs text-[var(--color-muted)]">Loading featured content…</p>
	</section>
{:else if widget.widget_type === 'related_discussions'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<MessageSquare size={18} class="text-[var(--color-text-subtle)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || 'Related Discussions'}</h2>
		</div>
		<p class="text-xs text-[var(--color-muted)]">Loading discussions…</p>
	</section>
{:else if widget.widget_type === 'upcoming_events'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<Calendar size={18} class="text-[var(--color-text-subtle)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || 'Upcoming Events'}</h2>
		</div>
		<p class="text-xs text-[var(--color-muted)]">Loading events…</p>
	</section>
{:else if widget.widget_type === 'tag_cloud'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title || 'Popular Tags'}</h2>
		<p class="text-xs text-[var(--color-muted)]">Loading tags…</p>
	</section>
{:else if widget.widget_type === 'user_card'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<UserCircle size={18} class="text-[var(--color-text-subtle)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || 'Featured User'}</h2>
		</div>
		<p class="text-xs text-[var(--color-muted)]">Loading user…</p>
	</section>
{:else if widget.widget_type === 'space_categories'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<FolderTree size={18} class="text-[var(--color-text-subtle)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || 'Browse Spaces'}</h2>
		</div>
		<p class="text-xs text-[var(--color-muted)]">Loading categories…</p>
	</section>
{:else if widget.widget_type === 'bookmark_folders'}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<Bookmark size={18} class="text-[var(--color-text-subtle)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || 'Bookmarks'}</h2>
		</div>
		<p class="text-xs text-[var(--color-muted)]">Loading bookmarks…</p>
	</section>
{:else if widget.widget_type === 'table_of_contents'}
	<!-- TOC widget — rendered by the Docs app; no-op in Learn app. -->
{:else if widget.widget_type === 'helpful_widget'}
	<!-- Helpful widget — rendered by the Docs app; no-op in Learn app. -->
{:else if widget.widget_type === 'notification_centre'}
	<!-- Notification centre — rendered by the Main app; no-op in Learn app. -->
{:else if widget.widget_type === 'user_dropdown'}
	<!-- User dropdown — rendered by the app shell's hardcoded topbar. -->
{:else}
	<!-- Unknown widget type — render nothing rather than crashing. -->
{/if}
