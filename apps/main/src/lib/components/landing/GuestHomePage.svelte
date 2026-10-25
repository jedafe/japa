<script lang="ts">
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { page } from '$app/state';
	import ThemeLogo from '$lib/components/branding/ThemeLogo.svelte';
	import heroCommunity from '$lib/assets/branding/hero-community.webp';
	import type {
		GuestHomeData,
		GuestHomeOpportunity,
		GuestHomeStory,
		GuestHomeTool,
		GuestHomeStat,
		GuestHomeCategory
	} from '$lib/types';
	import {
		Briefcase,
		GraduationCap,
		Plane,
		TrendingUp,
		Users,
		Heart,
		Globe,
		MapPin,
		ChevronLeft,
		ChevronRight,
		ArrowRight,
		Mail,
		Phone,
		Clock,
		FileCheck,
		Calculator,
		Bot,
		Sparkles,
		Star,
		ClipboardCheck
	} from 'lucide-svelte';

	interface Props {
		home: GuestHomeData;
	}

	let { home }: Props = $props();

	// ---------------------------------------------------------------------------
	// Newsletter state
	// ---------------------------------------------------------------------------
	let email = $state('');
	let newsletterStatus = $state<'idle' | 'loading' | 'success' | 'error'>('idle');
	let newsletterMessage = $state('');

	let oppScroller: HTMLDivElement | undefined = $state(undefined);
	let storyScroller: HTMLDivElement | undefined = $state(undefined);
	let storyIndex = $state(0);

	const iconMap: Record<string, any> = {
		Briefcase,
		GraduationCap,
		Plane,
		TrendingUp,
		Users,
		Heart,
		Globe,
		MapPin,
		ChevronLeft,
		ChevronRight,
		ArrowRight,
		Mail,
		Phone,
		Clock,
		FileCheck,
		Calculator,
		Bot,
		Sparkles,
		Star,
		ClipboardCheck
	};

	function getIcon(name: string | undefined): any {
		if (!name) return null;
		return iconMap[name] ?? null;
	}

	function timeAgo(dateStr: string | undefined): string {
		if (!dateStr) return t('landing.home.time_recent', 'Recently');
		const date = new Date(dateStr);
		const now = new Date();
		const seconds = Math.floor((now.getTime() - date.getTime()) / 1000);
		const intervals = [
			{ label: t('landing.home.time_year', 'year'), seconds: 31536000 },
			{ label: t('landing.home.time_month', 'month'), seconds: 2592000 },
			{ label: t('landing.home.time_week', 'week'), seconds: 604800 },
			{ label: t('landing.home.time_day', 'day'), seconds: 86400 },
			{ label: t('landing.home.time_hour', 'hour'), seconds: 3600 },
			{ label: t('landing.home.time_minute', 'minute'), seconds: 60 }
		];
		for (const i of intervals) {
			const count = Math.floor(seconds / i.seconds);
			if (count >= 1) return `${count} ${i.label}${count > 1 ? 's' : ''} ${t('landing.home.time_ago', 'ago')}`;
		}
		return t('landing.home.time_just', 'Just now');
	}

	async function subscribe(e: Event) {
		e.preventDefault();
		if (!email) return;
		newsletterStatus = 'loading';
		const res = await api.post<{ message?: string }>('/newsletter/subscribe', { email });
		if (res.error) {
			newsletterStatus = 'error';
			newsletterMessage = res.error.message ?? t('landing.home.newsletter_error', 'Something went wrong. Please try again.');
		} else {
			newsletterStatus = 'success';
			newsletterMessage = res.data?.message ?? t('landing.home.newsletter_success', "You're subscribed! Check your inbox.");
			email = '';
		}
	}

	function scrollBy(scroller: HTMLDivElement | undefined, direction: number) {
		if (!scroller) return;
		const card = scroller.firstElementChild as HTMLElement | null;
		const width = card?.offsetWidth ?? 320;
		scroller.scrollBy({ left: direction * (width + 16), behavior: 'smooth' });
	}

	function categoryColorClass(color: string): string {
		switch (color) {
			case 'green':
				return 'text-[#16a34a] bg-[#dcfce7]';
			case 'blue':
				return 'text-[#2563eb] bg-[#dbeafe]';
			case 'purple':
				return 'text-[#9333ea] bg-[#f3e8ff]';
			case 'orange':
				return 'text-[#ea580c] bg-[#ffedd5]';
			case 'teal':
				return 'text-[#0d9488] bg-[#ccfbf1]';
			default:
				return 'text-[var(--color-primary)] bg-[var(--color-surface-subtle)]';
		}
	}

	function opportunityTagClasses(type: string): string {
		switch (type) {
			case 'job':
				return 'bg-[#dcfce7] text-[#16a34a]';
			case 'scholarship':
				return 'bg-[#dbeafe] text-[#2563eb]';
			case 'visa':
				return 'bg-[#ffedd5] text-[#ea580c]';
			case 'business':
				return 'bg-[#ccfbf1] text-[#0d9488]';
			case 'internship':
				return 'bg-[#f3e8ff] text-[#9333ea]';
			default:
				return 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]';
		}
	}

	function opportunityIconName(name: string | undefined): string {
		if (!name) return 'Briefcase';
		const map: Record<string, string> = {
			FileCheck: 'FileCheck',
			ClipboardCheck: 'ClipboardCheck',
			Calculator: 'Calculator',
			GraduationCap: 'GraduationCap',
			TrendingUp: 'TrendingUp',
			Bot: 'Bot',
			Briefcase: 'Briefcase',
			Plane: 'Plane'
		};
		return map[name] ?? 'Briefcase';
	}

	function toolHref(tool: GuestHomeTool): string {
		return tool.href ?? (tool.slug ? `/tools/${tool.slug}` : '/');
	}

	function opportunityHref(opp: GuestHomeOpportunity): string {
		if (opp.href) return opp.href;
		if (opp.type === 'job') return `/jobs`;
		if (opp.type === 'scholarship') return `/scholarships`;
		return `/explore`;
	}

	function logoInitials(name: string): string {
		return name
			.split(' ')
			.map((w) => w[0])
			.slice(0, 2)
			.join('')
			.toUpperCase();
	}

	function logoBg(type: string): string {
		switch (type) {
			case 'job':
				return 'bg-[#f0f9ff] text-[#0ea5e9]';
			case 'scholarship':
				return 'bg-[#eff6ff] text-[#2563eb]';
			case 'visa':
				return 'bg-[#fff7ed] text-[#ea580c]';
			case 'business':
				return 'bg-[#f0fdf4] text-[#16a34a]';
			case 'internship':
				return 'bg-[#faf5ff] text-[#9333ea]';
			default:
				return 'bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]';
		}
	}

	const communityStat = $derived<GuestHomeStat | undefined>(
		home.stats.find((s) => s.label.toLowerCase().includes('community'))
	);

	const destinations = $derived(home.topDestinations.slice(0, 4));

	const title = $derived(t('landing.home.title', '{site} — Work. Study. Thrive. Anywhere in the World.'));
	const description = $derived(
		t(
			'landing.home.description',
			'{site} is your all-in-one platform for migration guidance, job opportunities, scholarships, business insights, and everything you need to succeed globally.'
		)
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary_large_image" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
</svelte:head>

<!-- Hero -->
<section class="guest-hero" aria-label={t('landing.home.hero_label', 'Hero')}>
	<div class="guest-hero-grid">
		<div class="guest-hero-content">
			<span class="hero-eyebrow">{t('landing.home.eyebrow', 'Your Bridge to Global Opportunities')}</span>
			<h1 class="hero-headline">
				{t('landing.home.headline_prefix', 'Work. Study. Thrive.')}
				<span class="hero-headline-accent">{t('landing.home.headline_accent', 'Anywhere in the World.')}</span>
			</h1>
			<p class="hero-subhead">
				{t(
					'landing.home.subhead',
					'{site} is your all-in-one platform for migration guidance, job opportunities, scholarships, business insights, and everything you need to succeed globally.'
				)}
			</p>
			<div class="hero-actions">
				<a href="/signup" class="hero-btn hero-btn-primary">
					{t('landing.home.cta_start', 'Get Started Free')}
					<ArrowRight class="rtl:-scale-x-100" size={18} />
				</a>
				<a href="/explore" class="hero-btn hero-btn-secondary">
					{t('landing.home.cta_explore', 'Explore Opportunities')}
				</a>
			</div>

			<div class="hero-stats">
				{#each home.stats as stat (stat.label)}
					<div class="hero-stat">
						<div class="hero-stat-icon">
							{#if getIcon(stat.icon)}
								{@const Icon = getIcon(stat.icon)}
								<Icon size={20} />
							{/if}
						</div>
						<div class="hero-stat-body">
							<span class="hero-stat-value">{stat.value}</span>
							<span class="hero-stat-label">{stat.label}</span>
						</div>
					</div>
				{/each}
			</div>
		</div>

		<div class="hero-visual">
			<div class="hero-image-wrap">
				<img
					src={heroCommunity}
					alt={t('landing.home.hero_image_alt', 'A confident traveler at the airport')}
					class="hero-image"
					loading="eager"
					fetchpriority="high"
					width="800"
					height="534"
					decoding="async"
				/>

				<div class="floating-card floating-card-destinations">
					<h2 class="floating-card-title">{t('landing.home.top_destinations', 'Top Destinations')}</h2>
					<ul class="destination-list">
						{#each destinations as dest (dest.id)}
							<li class="destination-item">
								<div class="destination-flag">
									{#if dest.icon_url}
										<img src={dest.icon_url} alt="" class="destination-flag-img" />
									{:else}
										<Globe size={16} />
									{/if}
								</div>
								<span class="destination-name">{dest.name}</span>
							</li>
						{/each}
					</ul>
					<a href="/explore" class="destination-all">
						{t('landing.home.see_all_countries', 'See all countries')}
						<ArrowRight class="rtl:-scale-x-100" size={14} />
					</a>
				</div>

				<div class="floating-card floating-card-community">
					<div class="community-avatars">
						{#each [1, 2, 3] as i (i)}
							<div class="community-avatar" style="z-index: {4 - i}; margin-inline-start: {i === 1 ? 0 : -10}px">
								<Users size={16} />
							</div>
						{/each}
					</div>
					<div class="community-body">
						<h2 class="community-title">
							{t('landing.home.join_title', 'Join {count} members worldwide', { count: communityStat?.value ?? '500K+' })}
						</h2>
						<p class="community-subtitle">
							{t('landing.home.join_subtitle', 'Your future starts with the right information.')}
						</p>
					</div>
					<a href="/explore" class="community-cta" aria-label={t('landing.home.join_cta_label', 'Join the community')}>
						<ArrowRight class="rtl:-scale-x-100" size={18} />
					</a>
				</div>
			</div>
		</div>
	</div>
</section>

<!-- Explore categories -->
<section class="guest-section" aria-labelledby="explore-heading">
	<div class="guest-section-inner">
		<div class="section-header">
			<h2 id="explore-heading" class="section-title">{t('landing.home.explore_title', 'Explore What Matters to You')}</h2>
			<p class="section-subtitle">{t('landing.home.explore_subtitle', 'Everything you need to achieve your global dreams.')}</p>
		</div>

		<div class="category-grid">
			{#each home.categories as cat (cat.id)}
				<a href={cat.href} class="category-card group">
					<div class="category-icon {categoryColorClass(cat.color)}">
						{#if getIcon(cat.icon)}
							{@const Icon = getIcon(cat.icon)}
							<Icon size={28} />
						{/if}
					</div>
					<h3 class="category-title">{cat.title}</h3>
					<p class="category-description">{cat.description}</p>
					<span class="category-link">
						{t('landing.home.explore_link', 'Explore {title}', { title: cat.title })}
						<ArrowRight class="rtl:-scale-x-100" size={16} />
					</span>
				</a>
			{/each}
		</div>
	</div>
</section>

<!-- Featured opportunities -->
<section class="guest-section" aria-labelledby="opportunities-heading">
	<div class="guest-section-inner">
		<div class="section-header-row">
			<h2 id="opportunities-heading" class="section-title">{t('landing.home.opportunities_title', 'Featured Opportunities')}</h2>
			<a href="/explore" class="section-link">
				{t('landing.home.opportunities_view_all', 'View all opportunities')}
				<ArrowRight class="rtl:-scale-x-100" size={16} />
			</a>
		</div>

		<div class="carousel-wrap">
			<button
				type="button"
				class="carousel-arrow carousel-arrow-left"
				onclick={() => scrollBy(oppScroller, -1)}
				aria-label={t('landing.home.carousel_prev', 'Previous')}
			>
				<ChevronLeft class="rtl:-scale-x-100" size={20} />
			</button>

			<div bind:this={oppScroller} class="opportunity-scroll">
				{#each home.opportunities as opp (opp.id)}
					<a href={opportunityHref(opp)} class="opportunity-card">
						<div class="opportunity-card-top">
							<span class="opportunity-type {opportunityTagClasses(opp.type)}">{opp.tag}</span>
							{#if opp.badge}
								<span class="opportunity-badge">{opp.badge}</span>
							{/if}
						</div>
						<div class="opportunity-logo">
							{#if opp.logo_url}
								<img src={opp.logo_url} alt="" class="opportunity-logo-img" />
							{:else}
								<div class="opportunity-logo-fallback {logoBg(opp.type)}">{logoInitials(opp.organization)}</div>
							{/if}
						</div>
						<h3 class="opportunity-title">{opp.title}</h3>
						<p class="opportunity-org">{opp.organization}</p>
						<div class="opportunity-meta">
							<span class="opportunity-location">
								<MapPin size={14} />
								{opp.location}
							</span>
							{#if opp.value}
								<span class="opportunity-value">{opp.value}</span>
							{/if}
						</div>
						<div class="opportunity-footer">
							<span class="opportunity-type-pill {opportunityTagClasses(opp.type)}">{opp.tag}</span>
							<span class="opportunity-time">{timeAgo(opp.published_at)}</span>
						</div>
					</a>
				{/each}
			</div>

			<button
				type="button"
				class="carousel-arrow carousel-arrow-right"
				onclick={() => scrollBy(oppScroller, 1)}
				aria-label={t('landing.home.carousel_next', 'Next')}
			>
				<ChevronRight class="rtl:-scale-x-100" size={20} />
			</button>
		</div>
	</div>
</section>

<!-- Success stories -->
<section class="guest-section stories-section" aria-labelledby="stories-heading">
	<div class="guest-section-inner stories-inner">
		<div class="stories-header">
			<span class="stories-eyebrow">{t('landing.home.stories_eyebrow', 'Real People. Real Stories.')}</span>
			<h2 id="stories-heading" class="section-title section-title-light">{t('landing.home.stories_title', 'Success Stories That Inspire')}</h2>
			<p class="stories-subtitle">
				{t(
					'landing.home.stories_subtitle',
					'From landing dream jobs to getting scholarships and starting new lives—our community is achieving greatness worldwide.'
				)}
			</p>
			<a href="/stories" class="stories-cta">
				{t('landing.home.stories_read_more', 'Read More Stories')}
				<ArrowRight class="rtl:-scale-x-100" size={18} />
			</a>
		</div>

		<div class="carousel-wrap stories-carousel-wrap">
			<button
				type="button"
				class="carousel-arrow carousel-arrow-left stories-arrow"
				onclick={() => scrollBy(storyScroller, -1)}
				aria-label={t('landing.home.carousel_prev', 'Previous')}
			>
				<ChevronLeft class="rtl:-scale-x-100" size={20} />
			</button>

			<div bind:this={storyScroller} class="story-scroll" role="list">
				{#each home.stories as story (story.id)}
					<div class="story-card" role="listitem" style={story.author_avatar_url ? `background-image: linear-gradient(rgba(15,23,42,0.55), rgba(15,23,42,0.85)), url(${story.author_avatar_url})` : undefined}>
						<div class="story-quote">“{story.quote}”</div>
						<div class="story-author">
							<div class="story-avatar">
								{#if story.author_avatar_url}
									<img src={story.author_avatar_url} alt="" class="story-avatar-img" />
								{:else}
									{@render UserInitials(story.author_name)}
								{/if}
							</div>
							<div class="story-author-body">
								<span class="story-author-name">{story.author_name}</span>
								{#if story.author_role}
									<span class="story-author-role">{story.author_role}</span>
								{/if}
							</div>
						</div>
					</div>
				{/each}
			</div>

			<button
				type="button"
				class="carousel-arrow carousel-arrow-right stories-arrow"
				onclick={() => scrollBy(storyScroller, 1)}
				aria-label={t('landing.home.carousel_next', 'Next')}
			>
				<ChevronRight class="rtl:-scale-x-100" size={20} />
			</button>
		</div>

		<div class="stories-dots">
			{#each home.stories as _, i (i)}
				<button
					type="button"
					class="story-dot {storyIndex === i ? 'story-dot-active' : ''}"
					onclick={() => {
						storyIndex = i;
						if (storyScroller) {
							const card = storyScroller.children[i] as HTMLElement | undefined;
							if (card) card.scrollIntoView({ behavior: 'smooth', inline: 'start', block: 'nearest' });
						}
					}}
					aria-label={t('landing.home.story_dot', 'Go to story {index}', { index: i + 1 })}
				></button>
			{/each}
		</div>
	</div>
</section>

<!-- Tools -->
<section class="guest-section" aria-labelledby="tools-heading">
	<div class="guest-section-inner">
		<div class="section-header">
			<h2 id="tools-heading" class="section-title">{t('landing.home.tools_title', 'Powerful Tools & Resources')}</h2>
			<p class="section-subtitle">{t('landing.home.tools_subtitle', 'Free tools to help you plan, apply, and succeed.')}</p>
		</div>

		<div class="tools-grid">
			{#each home.tools as tool (tool.id)}
				<a href={toolHref(tool)} class="tool-card group">
					<div class="tool-icon" style="background-color: {tool.accent ? `color-mix(in srgb, ${tool.accent} 15%, transparent)` : 'var(--color-surface-subtle)'}; color: {tool.accent ?? 'var(--color-primary)'}">
						{#if getIcon(opportunityIconName(tool.icon))}
							{@const Icon = getIcon(opportunityIconName(tool.icon))}
							<Icon size={24} />
						{/if}
					</div>
					<h3 class="tool-name">{tool.name}</h3>
					<p class="tool-description">{tool.description}</p>
				</a>
			{/each}
		</div>
	</div>
</section>

<!-- Newsletter -->
<section class="guest-section newsletter-section" aria-labelledby="newsletter-heading">
	<div class="newsletter-inner">
		<div class="newsletter-content">
			<div class="newsletter-icon">
				<Plane size={28} />
			</div>
			<div>
				<h2 id="newsletter-heading" class="newsletter-title">{t('landing.home.newsletter_title', 'Stay Ahead, Always')}</h2>
				<p class="newsletter-subtitle">{t('landing.home.newsletter_subtitle', 'Get the latest opportunities, tips, and updates straight to your inbox.')}</p>
			</div>
		</div>

		<form class="newsletter-form" onsubmit={subscribe}>
			<input
				type="email"
				bind:value={email}
				placeholder={t('landing.home.newsletter_placeholder', 'Enter your email address')}
				class="newsletter-input"
				required
			/>
			<button type="submit" class="newsletter-btn" disabled={newsletterStatus === 'loading'}>
				{t('landing.home.newsletter_cta', 'Subscribe Now')}
				<ArrowRight class="rtl:-scale-x-100" size={18} />
			</button>
		</form>

		{#if newsletterStatus === 'success' || newsletterStatus === 'error'}
			<p class="newsletter-status {newsletterStatus === 'success' ? 'newsletter-status-success' : 'newsletter-status-error'}" role="status">
				{newsletterMessage}
			</p>
		{/if}
	</div>
</section>

<!-- Footer -->
<footer class="guest-landing-footer" aria-labelledby="footer-heading">
	<h2 id="footer-heading" class="sr-only">{t('landing.home.footer_label', 'Footer')}</h2>
	<div class="footer-inner">
		<div class="footer-brand">
			<ThemeLogo
				alt={t('app.name')}
				class="h-8 w-auto"
			/>
			<p class="footer-tagline">
				{t(
					'landing.home.footer_tagline',
					'Your trusted partner in navigating global opportunities and building a better future.'
				)}
			</p>
			<div class="footer-socials">
				<a href="/" aria-label={t('landing.home.social_facebook', 'Facebook')}><Globe size={18} /></a>
				<a href="/" aria-label={t('landing.home.social_twitter', 'Twitter')}><Globe size={18} /></a>
				<a href="/" aria-label={t('landing.home.social_instagram', 'Instagram')}><Globe size={18} /></a>
				<a href="/" aria-label={t('landing.home.social_youtube', 'YouTube')}><Globe size={18} /></a>
				<a href="/" aria-label={t('landing.home.social_linkedin', 'LinkedIn')}><Globe size={18} /></a>
			</div>
		</div>

		<div class="footer-columns">
			<div class="footer-col">
				<h3 class="footer-col-title">{t('landing.home.footer_explore', 'Explore')}</h3>
				<ul class="footer-col-list">
					<li><a href="/jobs">{t('landing.home.footer_jobs', 'Jobs Abroad')}</a></li>
					<li><a href="/universities">{t('landing.home.footer_study', 'Study Abroad')}</a></li>
					<li><a href="/tools/visa-checklist">{t('landing.home.footer_visa', 'Visa & Relocation')}</a></li>
					<li><a href="/explore">{t('landing.home.footer_business', 'Business')}</a></li>
					<li><a href="/explore">{t('landing.home.footer_countries', 'Countries')}</a></li>
				</ul>
			</div>

			<div class="footer-col">
				<h3 class="footer-col-title">{t('landing.home.footer_resources', 'Resources')}</h3>
				<ul class="footer-col-list">
					<li><a href="/blog">{t('landing.home.footer_blog', 'Blog')}</a></li>
					<li><a href="/explore">{t('landing.home.footer_guides', 'Guides')}</a></li>
					<li><a href="/tools">{t('landing.home.footer_tools', 'Tools')}</a></li>
					<li><a href="/compare">{t('landing.home.footer_compare', 'Compare Countries')}</a></li>
					<li><a href="/stories">{t('landing.home.footer_stories', 'Success Stories')}</a></li>
				</ul>
			</div>

			<div class="footer-col">
				<h3 class="footer-col-title">{t('landing.home.footer_company', 'Company')}</h3>
				<ul class="footer-col-list">
					<li><a href="/about">{t('landing.home.footer_about', 'About Us')}</a></li>
					<li><a href="/how-it-works">{t('landing.home.footer_how', 'How It Works')}</a></li>
					<li><a href="/pricing">{t('landing.home.footer_pricing', 'Pricing')}</a></li>
					<li><a href="/faqs">{t('landing.home.footer_faqs', 'FAQs')}</a></li>
					<li><a href="/contact">{t('landing.home.footer_contact', 'Contact Us')}</a></li>
				</ul>
			</div>

			<div class="footer-col">
				<h3 class="footer-col-title">{t('landing.home.footer_help', 'Need Help?')}</h3>
				<ul class="footer-col-list footer-col-contact">
					<li>
						<Mail size={16} />
						<a href="mailto:support@japa.to">support@japa.to</a>
					</li>
					<li>
						<Phone size={16} />
						<a href="tel:+12345678900">+1 (234) 567-8900</a>
					</li>
					<li>
						<Clock size={16} />
						<span>{t('landing.home.footer_hours', 'Mon - Fri: 9AM - 6PM (EST)')}</span>
					</li>
				</ul>
			</div>
		</div>
	</div>

	<div class="footer-bottom">
		<p class="footer-copyright">
			&copy; {new Date().getFullYear()} {t('app.name')}. {t('landing.home.footer_rights', 'All rights reserved.')}
		</p>
		<div class="footer-legal">
			<a href="/privacy">{t('landing.home.footer_privacy', 'Privacy Policy')}</a>
			<a href="/terms">{t('landing.home.footer_terms', 'Terms of Service')}</a>
			<a href="/cookies">{t('landing.home.footer_cookies', 'Cookies Policy')}</a>
		</div>
	</div>
</footer>

{#snippet UserInitials(name: string)}
	<div class="initials-avatar" aria-hidden="true">
		{logoInitials(name)}
	</div>
{/snippet}

<style>
	.guest-hero {
		background: linear-gradient(135deg, #f0fdf4 0%, #f8fafc 50%, #ffffff 100%);
		padding: 2rem 1rem 3rem;
	}
	@media (min-width: 768px) {
		.guest-hero {
			padding: 3rem 1rem 4rem;
		}
	}
	:global([data-theme='dark'] .guest-hero) {
		background: linear-gradient(135deg, #052e16 0%, #0f172a 50%, #000000 100%);
	}

	.guest-hero-grid {
		max-width: 1280px;
		margin: 0 auto;
		display: grid;
		grid-template-columns: 1fr;
		gap: 2.5rem;
		align-items: center;
	}
	@media (min-width: 768px) {
		.guest-hero-grid {
			grid-template-columns: 1fr 1fr;
			gap: 3rem;
		}
	}
	@media (min-width: 1024px) {
		.guest-hero-grid {
			gap: 4rem;
		}
	}

	.guest-hero-content {
		display: flex;
		flex-direction: column;
		gap: 1.25rem;
	}

	.hero-eyebrow {
		display: inline-flex;
		align-self: flex-start;
		align-items: center;
		gap: 0.5rem;
		padding: 0.375rem 0.875rem;
		border-radius: 9999px;
		background: #dcfce7;
		color: #16a34a;
		font-size: 0.875rem;
		font-weight: 500;
	}
	:global([data-theme='dark'] .hero-eyebrow) {
		background: #14532d;
		color: #86efac;
	}

	.hero-headline {
		font-size: clamp(2.25rem, 5vw, 3.5rem);
		font-weight: 700;
		line-height: 1.1;
		color: var(--color-text);
		letter-spacing: -0.02em;
	}
	.hero-headline-accent {
		color: #16a34a;
		display: block;
	}
	:global([data-theme='dark'] .hero-headline-accent) {
		color: #4ade80;
	}

	.hero-subhead {
		font-size: 1.125rem;
		color: var(--color-text-subtle);
		line-height: 1.7;
		max-width: 540px;
	}

	.hero-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.75rem;
		margin-top: 0.5rem;
	}
	.hero-actions a,
	.hero-actions :global(button) {
		flex: 1 1 auto;
		justify-content: center;
	}
	@media (min-width: 480px) {
		.hero-actions a,
		.hero-actions :global(button) {
			flex: 0 0 auto;
		}
	}

	.hero-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.875rem 1.5rem;
		border-radius: var(--radius);
		font-weight: 600;
		font-size: 0.9375rem;
		transition: background 0.15s, color 0.15s, box-shadow 0.15s;
		white-space: nowrap;
	}
	.hero-btn-primary {
		background: #16a34a;
		color: white;
	}
	.hero-btn-primary:hover {
		background: #15803d;
	}
	.hero-btn-secondary {
		background: white;
		color: var(--color-text);
		border: 1px solid var(--color-border);
	}
	.hero-btn-secondary:hover {
		background: var(--color-surface-subtle);
	}
	:global([data-theme='dark'] .hero-btn-secondary) {
		background: var(--color-surface);
	}

	.hero-stats {
		display: flex;
		flex-wrap: wrap;
		gap: 1rem;
		margin-top: 1.25rem;
	}
	@media (min-width: 768px) {
		.hero-stats {
			gap: 1.5rem;
			margin-top: 1.5rem;
		}
	}

	.hero-stat {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.hero-stat-icon {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 2.5rem;
		height: 2.5rem;
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-primary);
		box-shadow: 0 2px 8px rgb(0 0 0 / 0.04);
	}

	.hero-stat-body {
		display: flex;
		flex-direction: column;
	}

	.hero-stat-value {
		font-size: 1.25rem;
		font-weight: 700;
		color: var(--color-text);
		line-height: 1;
	}

	.hero-stat-label {
		font-size: 0.8125rem;
		color: var(--color-muted);
	}

	.hero-visual {
		position: relative;
		display: flex;
		justify-content: center;
	}

	.hero-image-wrap {
		position: relative;
		width: 100%;
		max-width: 560px;
	}

	.hero-image {
		width: 100%;
		aspect-ratio: 4/5;
		object-fit: cover;
		border-radius: 1.5rem;
		box-shadow: 0 24px 60px rgb(0 0 0 / 0.12);
	}

	.floating-card {
		position: absolute;
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		box-shadow: 0 16px 40px rgb(0 0 0 / 0.08);
		padding: 1rem;
	}

	.floating-card-destinations {
		top: 1.5rem;
		inset-inline-start: -1.5rem;
		width: 220px;
	}

	.floating-card-community {
		bottom: 2.5rem;
		inset-inline-end: -1.5rem;
		width: 260px;
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	@media (max-width: 1024px) and (min-width: 641px) {
		.floating-card-destinations {
			inset-inline-start: 1rem;
			width: 200px;
		}
		.floating-card-community {
			inset-inline-end: 1rem;
			width: 220px;
		}
		.floating-card-community .community-avatars {
			display: none;
		}
	}

	@media (max-width: 640px) {
		.floating-card {
			padding: 0.75rem;
			border-radius: calc(var(--radius-card) - 4px);
		}

		.floating-card-destinations {
			inset-inline-start: 1rem;
			width: 170px;
		}

		.floating-card-community {
			inset-inline-end: 1rem;
			width: 200px;
			gap: 0.5rem;
		}

		.floating-card-community .community-avatars {
			display: none;
		}

		.floating-card-title {
			font-size: 0.875rem;
			margin-bottom: 0.5rem;
		}

		.destination-all {
			font-size: 0.75rem;
		}
	}

	.floating-card-title {
		font-size: 0.9375rem;
		font-weight: 600;
		color: var(--color-text);
		margin: 0 0 0.75rem;
	}

	.destination-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.destination-item {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.875rem;
		color: var(--color-text-subtle);
	}

	.destination-flag {
		width: 1.5rem;
		height: 1.5rem;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 9999px;
		background: var(--color-surface-subtle);
		overflow: hidden;
	}

	.destination-flag-img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.destination-all {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		margin-top: 0.75rem;
		font-size: 0.8125rem;
		font-weight: 500;
		color: var(--color-primary);
	}

	.community-avatars {
		display: flex;
		align-items: center;
	}

	.community-avatar {
		width: 2rem;
		height: 2rem;
		border-radius: 9999px;
		background: linear-gradient(135deg, var(--color-primary), var(--color-primary-hover));
		color: white;
		display: flex;
		align-items: center;
		justify-content: center;
		border: 2px solid var(--color-surface);
	}

	.community-body {
		flex: 1;
		min-width: 0;
	}

	.community-title {
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--color-text);
		margin: 0;
	}

	.community-subtitle {
		font-size: 0.75rem;
		color: var(--color-muted);
		margin: 0.125rem 0 0;
	}

	.community-cta {
		width: 2rem;
		height: 2rem;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 9999px;
		background: #16a34a;
		color: white;
		flex-shrink: 0;
	}

	.guest-section {
		padding: 3rem 1rem;
	}
	.guest-section-inner {
		max-width: 1200px;
		margin: 0 auto;
	}
	@media (min-width: 768px) {
		.guest-section {
			padding: 4rem 1rem;
		}
	}

	.section-header {
		text-align: center;
		margin-bottom: 2rem;
	}
	@media (min-width: 768px) {
		.section-header {
			margin-bottom: 2.5rem;
		}
	}

	.section-title {
		font-size: clamp(1.5rem, 3vw, 2rem);
		font-weight: 700;
		color: var(--color-text);
		margin: 0 0 0.5rem;
	}

	.section-title-light {
		color: white;
	}

	.section-subtitle {
		color: var(--color-muted);
		font-size: 1rem;
		margin: 0;
	}

	.section-header-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 2rem;
		gap: 1rem;
		flex-wrap: wrap;
	}

	.section-link {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-primary);
		white-space: nowrap;
	}

	.category-grid {
		display: grid;
		grid-template-columns: 1fr;
		gap: 1.25rem;
	}
	@media (min-width: 480px) {
		.category-grid {
			grid-template-columns: repeat(2, 1fr);
		}
	}
	@media (min-width: 768px) {
		.category-grid {
			grid-template-columns: repeat(3, 1fr);
		}
	}
	@media (min-width: 1024px) {
		.category-grid {
			grid-template-columns: repeat(5, 1fr);
		}
	}

	.category-card {
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		padding: 1.25rem;
		transition: box-shadow 0.15s, transform 0.15s;
	}
	.category-card:hover {
		box-shadow: 0 12px 32px rgb(0 0 0 / 0.06);
		transform: translateY(-2px);
	}
	@media (min-width: 768px) {
		.category-card {
			padding: 1.5rem;
		}
	}

	.category-icon {
		width: 3.5rem;
		height: 3.5rem;
		border-radius: 9999px;
		display: flex;
		align-items: center;
		justify-content: center;
		margin-bottom: 1rem;
	}

	.category-title {
		font-size: 1.125rem;
		font-weight: 600;
		color: var(--color-text);
		margin: 0 0 0.5rem;
	}

	.category-description {
		font-size: 0.9375rem;
		color: var(--color-text-subtle);
		line-height: 1.6;
		margin: 0 0 1rem;
	}

	.category-link {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-primary);
	}

	.carousel-wrap {
		position: relative;
	}

	.carousel-arrow {
		position: absolute;
		top: 50%;
		transform: translateY(-50%);
		z-index: 10;
		width: 2.5rem;
		height: 2.5rem;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 9999px;
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		color: var(--color-text);
		box-shadow: 0 4px 12px rgb(0 0 0 / 0.06);
		cursor: pointer;
	}
	.carousel-arrow:hover {
		background: var(--color-surface-subtle);
	}
	.carousel-arrow-left {
		inset-inline-start: -1rem;
	}
	.carousel-arrow-right {
		inset-inline-end: -1rem;
	}
	@media (max-width: 768px) {
		.carousel-arrow {
			display: none;
		}
	}

	.opportunity-scroll {
		display: flex;
		gap: 1rem;
		overflow-x: auto;
		scroll-snap-type: x mandatory;
		scroll-padding-inline: 0;
		padding: 0.5rem 0;
		margin: -0.5rem 0;
	}
	.opportunity-scroll::-webkit-scrollbar {
		display: none;
	}
	@media (min-width: 768px) {
		.opportunity-scroll {
			gap: 1.25rem;
		}
	}

	.opportunity-card {
		flex: 0 0 clamp(250px, 70vw, 280px);
		scroll-snap-align: start;
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		padding: 1rem;
		transition: box-shadow 0.15s;
	}
	.opportunity-card:hover {
		box-shadow: 0 12px 32px rgb(0 0 0 / 0.06);
	}
	@media (min-width: 768px) {
		.opportunity-card {
			padding: 1.25rem;
		}
	}

	.opportunity-card-top {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 1rem;
	}

	.opportunity-type {
		font-size: 0.75rem;
		font-weight: 600;
		padding: 0.25rem 0.5rem;
		border-radius: 9999px;
	}

	.opportunity-badge {
		font-size: 0.75rem;
		font-weight: 600;
		padding: 0.25rem 0.5rem;
		border-radius: 9999px;
		background: #dcfce7;
		color: #16a34a;
	}

	.opportunity-logo {
		width: 3.5rem;
		height: 3.5rem;
		margin-bottom: 1rem;
	}

	.opportunity-logo-img {
		width: 100%;
		height: 100%;
		object-fit: contain;
	}

	.opportunity-logo-fallback {
		width: 100%;
		height: 100%;
		border-radius: var(--radius);
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 1rem;
		font-weight: 700;
	}

	.opportunity-title {
		font-size: 1rem;
		font-weight: 600;
		color: var(--color-text);
		margin: 0 0 0.25rem;
	}

	.opportunity-org {
		font-size: 0.875rem;
		color: var(--color-muted);
		margin: 0 0 0.75rem;
	}

	.opportunity-meta {
		display: flex;
		align-items: center;
		justify-content: space-between;
		font-size: 0.8125rem;
		color: var(--color-text-subtle);
		margin-bottom: 1rem;
	}

	.opportunity-location {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
	}

	.opportunity-value {
		font-weight: 600;
		color: #16a34a;
	}

	.opportunity-footer {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding-top: 0.75rem;
		border-top: 1px solid var(--color-border);
	}

	.opportunity-type-pill {
		font-size: 0.75rem;
		font-weight: 600;
		padding: 0.25rem 0.5rem;
		border-radius: 9999px;
	}

	.opportunity-time {
		font-size: 0.75rem;
		color: var(--color-muted);
	}

	.stories-section {
		background: #0f172a;
		color: white;
		padding: 3rem 1rem;
	}
	@media (min-width: 768px) {
		.stories-section {
			padding: 4rem 1rem;
		}
	}

	.stories-inner {
		max-width: 1200px;
		margin: 0 auto;
	}

	.stories-header {
		text-align: center;
		max-width: 680px;
		margin: 0 auto 2.5rem;
	}

	.stories-eyebrow {
		display: inline-block;
		font-size: 0.875rem;
		font-weight: 500;
		color: #86efac;
		margin-bottom: 0.75rem;
	}

	.stories-subtitle {
		font-size: 1rem;
		color: #94a3b8;
		line-height: 1.7;
		margin: 0.5rem 0 1.25rem;
	}

	.stories-cta {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 1.25rem;
		border-radius: var(--radius);
		background: #16a34a;
		color: white;
		font-weight: 600;
		font-size: 0.9375rem;
	}

	.stories-carousel-wrap {
		margin-top: 1.5rem;
	}

	.stories-arrow {
		background: #1e293b;
		border-color: #334155;
		color: white;
	}
	.stories-arrow:hover {
		background: #334155;
	}

	.story-scroll {
		display: flex;
		gap: 1rem;
		overflow-x: auto;
		scroll-snap-type: x mandatory;
		padding: 0.5rem 0;
		margin: -0.5rem 0;
	}
	.story-scroll::-webkit-scrollbar {
		display: none;
	}
	@media (min-width: 768px) {
		.story-scroll {
			gap: 1.25rem;
		}
	}

	.story-card {
		flex: 0 0 clamp(280px, 85vw, 340px);
		scroll-snap-align: start;
		min-height: 240px;
		border-radius: var(--radius-card);
		padding: 1.25rem;
		display: flex;
		flex-direction: column;
		justify-content: space-between;
		background-size: cover;
		background-position: center;
		background-color: #1e293b;
		border: 1px solid #334155;
	}
	@media (min-width: 768px) {
		.story-card {
			min-height: 260px;
			padding: 1.5rem;
		}
	}

	.story-quote {
		font-size: 1rem;
		font-weight: 500;
		line-height: 1.6;
		color: white;
	}

	.story-author {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-top: 1.5rem;
	}

	.story-avatar {
		width: 2.5rem;
		height: 2.5rem;
		border-radius: 9999px;
		overflow: hidden;
		background: #334155;
	}

	.story-avatar-img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.story-author-body {
		display: flex;
		flex-direction: column;
	}

	.story-author-name {
		font-size: 0.9375rem;
		font-weight: 600;
		color: white;
	}

	.story-author-role {
		font-size: 0.75rem;
		color: #94a3b8;
	}

	.stories-dots {
		display: flex;
		justify-content: center;
		gap: 0.5rem;
		margin-top: 1.5rem;
	}

	.story-dot {
		width: 0.5rem;
		height: 0.5rem;
		border-radius: 9999px;
		background: #475569;
		border: none;
		cursor: pointer;
	}
	.story-dot-active {
		background: #16a34a;
		width: 1.25rem;
	}

	.tools-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
		gap: 1rem;
	}
	@media (min-width: 768px) {
		.tools-grid {
			gap: 1.25rem;
		}
	}

	.tool-card {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		padding: 1.25rem;
		transition: box-shadow 0.15s, transform 0.15s;
	}
	.tool-card:hover {
		box-shadow: 0 12px 32px rgb(0 0 0 / 0.06);
		transform: translateY(-2px);
	}
	@media (min-width: 768px) {
		.tool-card {
			padding: 1.5rem;
		}
	}

	.tool-icon {
		width: 3rem;
		height: 3rem;
		border-radius: var(--radius);
		display: flex;
		align-items: center;
		justify-content: center;
		margin-bottom: 1rem;
	}

	.tool-name {
		font-size: 1rem;
		font-weight: 600;
		color: var(--color-text);
		margin: 0 0 0.375rem;
	}

	.tool-description {
		font-size: 0.875rem;
		color: var(--color-text-subtle);
		line-height: 1.5;
		margin: 0;
	}

	.newsletter-section {
		padding: 0 1rem 4rem;
	}

	.newsletter-inner {
		max-width: 1200px;
		margin: 0 auto;
		background: linear-gradient(135deg, #f0fdf4 0%, #ffffff 100%);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		padding: 1.5rem;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1.5rem;
		flex-wrap: wrap;
	}
	@media (min-width: 768px) {
		.newsletter-inner {
			padding: 2.5rem;
			gap: 2rem;
		}
	}
	:global([data-theme='dark'] .newsletter-inner) {
		background: linear-gradient(135deg, #052e16 0%, #0f172a 100%);
	}

	.newsletter-content {
		display: flex;
		align-items: center;
		gap: 1rem;
		max-width: 420px;
	}

	.newsletter-icon {
		width: 3.5rem;
		height: 3.5rem;
		border-radius: 9999px;
		background: #16a34a;
		color: white;
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
	}

	.newsletter-title {
		font-size: 1.25rem;
		font-weight: 700;
		color: var(--color-text);
		margin: 0 0 0.25rem;
	}

	.newsletter-subtitle {
		font-size: 0.9375rem;
		color: var(--color-text-subtle);
		margin: 0;
	}

	.newsletter-form {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		flex: 1;
		min-width: 0;
	}
	@media (min-width: 480px) {
		.newsletter-form {
			flex-wrap: nowrap;
		}
		.newsletter-input {
			flex: 1 1 auto;
		}
	}

	.newsletter-input {
		flex: 1 1 100%;
		min-width: 0;
		padding: 0.875rem 1rem;
		border-radius: var(--radius);
		border: 1px solid var(--color-border);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.9375rem;
	}
	.newsletter-input::placeholder {
		color: var(--color-muted);
	}

	.newsletter-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.5rem;
		padding: 0.875rem 1.5rem;
		border-radius: var(--radius);
		background: #16a34a;
		color: white;
		font-weight: 600;
		font-size: 0.9375rem;
		white-space: nowrap;
		cursor: pointer;
		flex: 1 1 auto;
	}
	@media (min-width: 480px) {
		.newsletter-btn {
			flex: 0 0 auto;
		}
	}
	.newsletter-btn:disabled {
		opacity: 0.7;
		cursor: not-allowed;
	}

	.newsletter-status {
		width: 100%;
		font-size: 0.875rem;
		margin-top: 0.75rem;
	}
	.newsletter-status-success {
		color: #16a34a;
	}
	.newsletter-status-error {
		color: #dc2626;
	}

	.guest-landing-footer {
		background: #0f172a;
		color: white;
		padding: 3rem 1rem 1.5rem;
	}
	@media (min-width: 768px) {
		.guest-landing-footer {
			padding: 4rem 1rem 1.5rem;
		}
	}

	.footer-inner {
		max-width: 1200px;
		margin: 0 auto;
		display: grid;
		grid-template-columns: 1fr;
		gap: 3rem;
	}
	@media (min-width: 768px) {
		.footer-inner {
			grid-template-columns: 280px 1fr;
		}
	}

	.footer-brand {
		display: flex;
		flex-direction: column;
		gap: 1rem;
		align-items: center;
		text-align: center;
	}
	@media (min-width: 768px) {
		.footer-brand {
			align-items: flex-start;
			text-align: start;
		}
	}

	.footer-tagline {
		font-size: 0.9375rem;
		color: #94a3b8;
		line-height: 1.6;
		max-width: 280px;
	}

	.footer-socials {
		display: flex;
		gap: 0.75rem;
	}
	.footer-socials a {
		width: 2rem;
		height: 2rem;
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 9999px;
		background: #1e293b;
		color: #94a3b8;
		transition: background 0.15s, color 0.15s;
	}
	.footer-socials a:hover {
		background: #334155;
		color: white;
	}

	.footer-columns {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 2rem;
	}
	@media (min-width: 768px) {
		.footer-columns {
			grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
		}
	}

	.footer-col-title {
		font-size: 1rem;
		font-weight: 600;
		color: white;
		margin: 0 0 1rem;
	}

	.footer-col-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.625rem;
	}

	.footer-col-list a,
	.footer-col-list span {
		font-size: 0.875rem;
		color: #94a3b8;
		transition: color 0.15s;
	}
	.footer-col-list a:hover {
		color: white;
	}

	.footer-col-contact li {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		color: #94a3b8;
	}

	.footer-bottom {
		max-width: 1200px;
		margin: 3rem auto 0;
		padding-top: 1.5rem;
		border-top: 1px solid #1e293b;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: center;
		text-align: center;
		gap: 1rem;
	}
	@media (min-width: 768px) {
		.footer-bottom {
			justify-content: space-between;
			text-align: start;
		}
	}

	.footer-copyright {
		font-size: 0.875rem;
		color: #94a3b8;
		margin: 0;
	}

	.footer-legal {
		display: flex;
		gap: 1.5rem;
	}
	.footer-legal a {
		font-size: 0.875rem;
		color: #94a3b8;
	}
	.footer-legal a:hover {
		color: white;
	}

	.initials-avatar {
		width: 100%;
		height: 100%;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 0.875rem;
		font-weight: 600;
		color: white;
		background: linear-gradient(135deg, var(--color-primary), var(--color-primary-hover));
	}

	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border-width: 0;
	}
</style>