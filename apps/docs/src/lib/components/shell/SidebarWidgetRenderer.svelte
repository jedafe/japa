<script lang="ts">
	// Ref: Section 59 (Sidebar Management System), ADR-066.
	// Renders a single sidebar widget based on its widget_type.
	// Only widget types scoped to the Docs app in the backend registry
	// (WidgetType.Apps) render real UI here — the registry rejects creating
	// main/learn-only types on docs sidebars, and any legacy rows render as
	// no-ops below (never a permanent "Loading…" state).

	import { t } from '$lib/i18n';
	import { Sun, Moon, Languages, Mail, Share2, BarChart3, MousePointerClick, AtSign, Globe, Link, Rss, Users, TrendingUp, Activity, Star, Trophy, Calendar, MessageSquare, BookOpen, Bookmark, Bell, Hash } from 'lucide-svelte';
	import HelpfulWidget from '$lib/components/docs/HelpfulWidget.svelte';
	import { theme } from '$lib/stores/theme.svelte';
	import { i18n } from '$lib/i18n';
	import { apiPost } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import SearchLauncher from './SearchLauncher.svelte';
	import UserDropdown from './UserDropdown.svelte';
	import AdSlot from './AdSlot.svelte';
	import { sanitizeSidebarHtml, safeHref, resolveMenuHref } from '$lib/utils/safeHtml';
	import { env } from '$env/dynamic/public';
	import type { SessionUser } from '$lib/types';

	interface SidebarWidget {
		id: string;
		widget_type: string;
		label: string;
		props: Record<string, unknown>;
		sort_order: number;
	}

	interface Props {
		widget: SidebarWidget;
		user?: SessionUser | null;
	}

	let { widget, user = null }: Props = $props();

	const mainOrigin = (env.PUBLIC_MAIN_ORIGIN ?? 'http://localhost:3000').replace(/\/$/, '');

	const widgetProps = $derived(widget.props ?? {});
	const title = $derived(typeof widgetProps.title === 'string' ? widgetProps.title : '');
	const adSlot = $derived(typeof widgetProps.slot === 'string' ? widgetProps.slot : 'sidebar_top');
	const adFormat = $derived(
		typeof widgetProps.format === 'string' && ['square', 'banner', 'inline'].includes(widgetProps.format)
			? (widgetProps.format as 'square' | 'banner' | 'inline')
			: 'square'
	);

	// Enabled languages + default language from site settings (§18, ADR-013).
	// If the setting is unavailable, fall back to the active locale instead of
	// a hardcoded list (Rule 16).
	const enabledLanguages = $derived(
		((page.data?.siteSettings?.['i18n.enabled_languages'] as string | undefined) ?? (page.data?.locale as string) ?? 'en')
			.split(',').map((l) => l.trim()).filter(Boolean)
	);
	const defaultLanguage = $derived(
		(page.data?.siteSettings?.['i18n.default_language'] as string | undefined) ?? 'en'
	);
	// The language_toggle `languages` prop restricts the offered locales to a
	// subset of the site-enabled list — empty means all enabled languages.
	const configuredLanguages = $derived(
		(typeof widgetProps.languages === 'string' ? widgetProps.languages : '')
			.split(',').map((l) => l.trim()).filter(Boolean)
	);
	const langList = $derived(
		configuredLanguages.length > 0
			? enabledLanguages.filter((l) => configuredLanguages.includes(l))
			: enabledLanguages
	);
	const showLanguageToggle = $derived(langList.length > 1);

	const currentPath = $derived(page.url.pathname.replace(/^\//, ''));

	const displayNames = (() => {
		try { return new Intl.DisplayNames(undefined, { type: 'language' }); }
		catch { return null; }
	})();
	function localeLabel(code: string): string {
		return displayNames?.of(code) ?? code;
	}
	function localeHref(lang: string): string {
		let path = currentPath || '';
		// Strip an existing locale prefix — page.url.pathname can retain it on
		// prefixed URLs, which would produce /fr/fr/... double-prefixed links.
		const firstSeg = path.split('/')[0];
		if (enabledLanguages.includes(firstSeg)) path = path.slice(firstSeg.length).replace(/^\//, '');
		return lang === defaultLanguage ? `/${path}` : `/${lang}${path ? `/${path}` : ''}`;
	}

	// Switch locale: persist the choice in the jc_locale cookie AND navigate
	// to the locale-prefixed URL. The cookie write is required — the hooks
	// fall back to the cookie when the URL has no prefix, so without it,
	// switching to the default language keeps rendering the previous locale.
	// Uses SvelteKit's goto() for client-side navigation (ADR-013) rather
	// than a full window.location reload.
	function switchLanguage(lang: string) {
		document.cookie = `jc_locale=${lang}; path=/; max-age=${60 * 60 * 24 * 365}; samesite=lax`;
		// invalidateAll forces a load re-run even when the target URL equals
		// the current one — switching back to the default language strips the
		// prefix, producing a same-URL navigation that would otherwise leave
		// the previous locale rendered.
		goto(localeHref(lang), { invalidateAll: true });
	}

	// lucide icon lookup for admin-configured icon names (metric_card.icon,
	// social_links.platform). Unknown names fall back to a neutral icon so a
	// typo in the admin panel never crashes the renderer.
	const iconByName: Record<string, typeof BarChart3> = {
		'bar-chart-3': BarChart3, bar_chart_3: BarChart3, barchart3: BarChart3,
		users: Users, 'trending-up': TrendingUp, trending_up: TrendingUp,
		activity: Activity, globe: Globe, star: Star, trophy: Trophy,
		calendar: Calendar, 'message-square': MessageSquare, message_square: MessageSquare,
		'book-open': BookOpen, book_open: BookOpen, bookmark: Bookmark, bell: Bell, mail: Mail,
		hash: Hash,
	};
	// lucide 1.x removed brand icons — map platforms to generic icons:
	// social handles → AtSign, web/blog → Globe, email → Mail, feeds → Rss.
	const socialIconByPlatform: Record<string, typeof Share2> = {
		website: Globe, web: Globe, blog: Globe, url: Globe,
		email: Mail, mail: Mail, newsletter: Mail,
		rss: Rss, feed: Rss,
		twitter: AtSign, x: AtSign, instagram: AtSign, facebook: AtSign,
		youtube: AtSign, github: AtSign, linkedin: AtSign, tiktok: AtSign,
		mastodon: AtSign, threads: AtSign, bluesky: AtSign,
		link: Link,
	};

	// Newsletter subscribe form state (Rule 16: calls POST /newsletter/subscribe).
	// The endpoint is gated behind the feature.newsletter flag — when the
	// feature is off the widget hides entirely instead of 404ing on submit.
	const newsletterEnabled = $derived(
		(page.data?.features as Record<string, boolean> | undefined)?.['feature.newsletter'] !== false
	);
	let newsletterEmail = $state('');
	let newsletterSubmitting = $state(false);
	let newsletterMessage = $state<{ kind: 'success' | 'error' | 'info'; text: string } | null>(null);

	async function handleNewsletterSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (newsletterSubmitting) return;
		const email = newsletterEmail.trim();
		if (!email || !email.includes('@') || !email.includes('.')) {
			newsletterMessage = { kind: 'error', text: t('sidebar.newsletter_invalid', 'Please enter a valid email address') };
			return;
		}
		newsletterSubmitting = true;
		newsletterMessage = null;
		const res = await apiPost('/newsletter/subscribe', { email });
		newsletterSubmitting = false;
		if (!res.ok) {
			newsletterMessage = { kind: 'error', text: t('sidebar.newsletter_error', 'Subscription failed. Please try again.') };
			return;
		}
		newsletterMessage = { kind: 'success', text: t('sidebar.newsletter_success', 'You are subscribed!') };
		newsletterEmail = '';
	}
</script>

{#if widget.widget_type === 'ad_slot'}
	<AdSlot slot={adSlot} format={adFormat} />
{:else if widget.widget_type === 'rich_text'}
	{#if typeof widgetProps.content === 'string' && widgetProps.content.trim()}
		<!-- Sanitized server-side via internal/content.SanitizeHTML (BE-4) and
		     client-side via sanitizeSidebarHtml as defense in depth. Ref: §5.1. -->
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 prose prose-sm max-w-none">
			{@html sanitizeSidebarHtml(widgetProps.content)}
		</section>
	{/if}
{:else if widget.widget_type === 'custom_html'}
	{#if typeof widgetProps.html === 'string' && widgetProps.html.trim()}
		<!-- Custom HTML — rendered in a sandboxed iframe (no allow-scripts) so
		     injected script cannot run. Also sanitized server-side (BE-4) and
		     client-side via sanitizeSidebarHtml as defense in depth. -->
		<iframe
			class="sidebar-widget-custom-html w-full rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]"
			sandbox="allow-same-origin"
			srcdoc={sanitizeSidebarHtml(widgetProps.html)}
			title={widget.label || 'Custom HTML'}
		></iframe>
	{/if}
{:else if widget.widget_type === 'search_launcher'}
	<SearchLauncher placeholder={typeof widgetProps.placeholder === 'string' ? widgetProps.placeholder : ''} />
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
		<!-- Language selector — a dropdown field rendered through the shared
		     Autocomplete component (app-wide dropdown contract). -->
		<div class="flex items-center gap-1.5 text-[var(--color-muted)]">
			<Languages size={16} class="shrink-0" />
			<AutocompleteSelect
				value={i18n.locale}
				options={langList.map((lang) => ({ value: lang, label: localeLabel(lang) }))}
				onValueChange={(lang) => { if (lang && lang !== i18n.locale) switchLanguage(lang); }}
				ariaLabel={t('locale.switch_language', 'Switch language')}
				class="lang-select"
				inputClass="lang-input"
			/>
		</div>
	{/if}
{:else if widget.widget_type === 'helpful_widget'}
	<!-- DOC-011: helpful widget delegates to the canonical HelpfulWidget so
	     votes are persisted. Guests are excluded — /api/docs/feedback requires
	     a session, so a guest-visible vote button could only ever error. -->
	{#if user && page.data?.page?.id && page.data?.audience && page.data?.page?.slug}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<HelpfulWidget
				pageId={page.data.page.id}
				audience={page.data.audience}
				slug={page.data.page.slug}
				question={typeof widgetProps.question === 'string' ? widgetProps.question : ''}
				lastReviewedAt={page.data.page.last_reviewed_at}
				lastReviewedBy={page.data.page.last_reviewed_by}
			/>
		</section>
	{/if}
{:else if widget.widget_type === 'newsletter_signup'}
	{#if newsletterEnabled}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="mb-3 flex items-center gap-2">
			<Mail size={18} class="text-[var(--color-primary)]" />
			<h2 class="text-sm font-medium text-[var(--color-text)]">{title || t('sidebar.newsletter', 'Subscribe')}</h2>
		</div>
		{#if typeof widgetProps.description === 'string' && widgetProps.description}
			<p class="mb-3 text-xs text-[var(--color-muted)]">{widgetProps.description}</p>
		{/if}
		<form class="space-y-2" onsubmit={handleNewsletterSubmit}>
			<input
				type="email"
				bind:value={newsletterEmail}
				placeholder={typeof widgetProps.placeholder === 'string' ? widgetProps.placeholder : t('sidebar.email_placeholder')}
				disabled={newsletterSubmitting}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none disabled:opacity-60"
			/>
			<button type="submit" disabled={newsletterSubmitting} class="w-full rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-60">
				{typeof widgetProps.button_text === 'string' ? widgetProps.button_text : t('sidebar.subscribe', 'Subscribe')}
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
	{/if}
{:else if widget.widget_type === 'social_links'}
	<!-- Only render when at least one configured link has a URL — an empty
	     "Follow Us" card is dead space on every page otherwise. -->
	{@const validLinks = (Array.isArray(widgetProps.links) ? widgetProps.links : []).filter(
		(l) => typeof l === 'object' && l !== null && typeof (l as { url?: unknown }).url === 'string' && ((l as { url: string }).url).trim() !== ''
	)}
	{#if validLinks.length > 0}
		<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			{#if title}<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{title}</h2>{/if}
			<div class="flex flex-wrap gap-2">
				{#each validLinks as link, i (String(link.platform ?? '') + '-' + i)}
					{@const SocialIcon = socialIconByPlatform[String(link.platform ?? '').toLowerCase().trim()] ?? Share2}
					<a
						href={safeHref(link.url)}
						target="_blank"
						rel="noopener noreferrer"
						class="grid h-9 w-9 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)] hover:bg-[var(--color-primary)] hover:text-[var(--color-primary-contrast)]"
						aria-label={link.platform}
					>
						<SocialIcon size={16} />
					</a>
				{/each}
			</div>
		</section>
	{/if}
{:else if widget.widget_type === 'cta_button'}
	{@const ctaVariant = String(widgetProps.variant ?? 'primary').toLowerCase()}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<a
			href={resolveMenuHref(typeof widgetProps.url === 'string' ? widgetProps.url : '', mainOrigin) || `${mainOrigin}/signup`}
			class="flex w-full items-center justify-center gap-2 rounded-[var(--radius)] px-4 py-2.5 text-sm font-semibold transition-colors {ctaVariant === 'secondary'
				? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)] hover:bg-[var(--color-border)]'
				: ctaVariant === 'outline'
					? 'border border-[var(--color-primary)] text-[var(--color-primary)] hover:bg-[var(--color-primary)] hover:text-[var(--color-primary-contrast)]'
					: 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]'}"
		>
			<MousePointerClick size={16} />
			{typeof widgetProps.label === 'string' ? widgetProps.label : t('sidebar.get_started')}
		</a>
	</section>
{:else if widget.widget_type === 'metric_card'}
	{@const MetricIcon = iconByName[String(widgetProps.icon ?? '').toLowerCase().replace(/[\s]+/g, '-')] ?? BarChart3}
	<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="flex items-center gap-2 text-[var(--color-text-subtle)]">
			<MetricIcon size={16} />
			<span class="text-xs font-medium">{typeof widgetProps.label === 'string' ? widgetProps.label : t('sidebar.metric')}</span>
		</div>
		<p class="mt-2 text-2xl font-bold text-[var(--color-text)]">{typeof widgetProps.value === 'string' ? widgetProps.value : '—'}</p>
		{#if typeof widgetProps.delta === 'string' && widgetProps.delta}
			<p class="mt-1 text-xs text-[var(--color-text-subtle)]">{widgetProps.delta}</p>
		{/if}
	</section>
{:else if widget.widget_type === 'search'}
	<SearchLauncher placeholder={typeof widgetProps.placeholder === 'string' ? widgetProps.placeholder : ''} />
{:else if widget.widget_type === 'user_dropdown'}
	{#if user}
		<UserDropdown {user} />
	{/if}
{:else if ['recent_content', 'space_list', 'trending_tags', 'suggested_spaces', 'suggested_content', 'leaderboard', 'featured_content', 'related_discussions', 'upcoming_events', 'tag_cloud', 'user_card', 'space_categories', 'bookmark_folders', 'notification_centre', 'continue_learning', 'learning_enrollments', 'progress_ring', 'track_card'].includes(widget.widget_type)}
	<!-- Main-app / Learn-only widget type — the backend registry rejects
	     creating these on docs sidebars; legacy rows render as a no-op
	     instead of a permanent "Loading…" stub. -->
{:else}
	<!-- Unknown widget type — render nothing rather than crashing. -->
{/if}

<style>
	/* Compact language selector — sits in the topbar icon row. */
	:global(.lang-select) {
		width: 8.5rem;
	}
	:global(.lang-select .autocomplete-input-wrapper) {
		height: 2rem;
		padding: 0 0.5rem;
		font-size: 0.8125rem;
	}
	:global(.lang-select .autocomplete-chip-label) {
		max-width: 5rem;
	}
</style>
