<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { ArrowRight, Clock, Compass, Eye, MessageCircle, PenLine, Search, Users } from 'lucide-svelte';
	import type { FeedItem } from '$lib/types';
	import type { ContentArticle } from '$lib/types/content';

	interface Props {
		exploreItems: FeedItem[];
		blogPosts: ContentArticle[];
		destinations: Array<{ id: string; name: string; slug: string; description?: string }>;
	}
	let { exploreItems, blogPosts, destinations }: Props = $props();

	const title = $derived(t('landing.title', 'JapaConnect — The Nigerian Diaspora Community'));
	const description = $derived(t('landing.description', 'Stories, discussions, comparisons, and guides for Nigerians navigating life abroad.'));
	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'WebSite',
			name: t('app.name'),
			url: page.url.origin,
			description
		})
	);

	let email = $state('');
	let subscribed = $state(false);
	let subscribing = $state(false);
	let subscribeError = $state('');

	// Ref: japaconnect-engineering/sections/62-newsletter-digest.md (POST /newsletter/subscribe)
	// Rule 16: calls the backend API — no local-only state mutation.
	async function subscribe(e: SubmitEvent) {
		e.preventDefault();
		if (subscribing) return;
		const trimmed = email.trim();
		if (!trimmed) return;
		subscribing = true;
		subscribeError = '';
		try {
			const res = await api.post<{ status?: string }>('/newsletter/subscribe', { email: trimmed, source: 'footer' });
			if (res.error) {
				subscribeError = res.error.message ?? t('landing.newsletter_error', 'Something went wrong. Please try again.');
			} else {
				subscribed = true;
				email = '';
			}
		} catch {
			subscribeError = t('landing.newsletter_error', 'Something went wrong. Please try again.');
		} finally {
			subscribing = false;
		}
	}
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
	{@html `<script type="application/ld+json">${jsonLd}</script>`}
</svelte:head>

<div class="min-h-screen bg-[var(--color-bg)]">
	<!-- Hero — Hashnode-style with gradient + serif title -->
	<section class="relative overflow-hidden border-b border-[var(--color-border)]">
		<div class="absolute inset-0 bg-gradient-to-br from-[var(--color-primary)]/8 via-[var(--color-primary)]/3 to-transparent"></div>
		<div class="relative mx-auto max-w-4xl px-6 py-24 text-center sm:py-32">
			<div class="mb-6 inline-flex items-center gap-2 rounded-full border border-[var(--color-primary)]/20 bg-[var(--color-primary)]/5 px-4 py-1.5 text-sm font-medium text-[var(--color-primary)]">
				<Users size={14} /> {t('landing.hero_badge', 'The Nigerian diaspora community')}
			</div>
			<h1 class="text-balance font-serif text-4xl font-bold tracking-tight text-[var(--color-text)] sm:text-6xl" style="font-family: var(--font-serif);">
				{t('landing.hero_title', 'Your journey abroad starts here')}
			</h1>
			<p class="mx-auto mt-6 max-w-2xl text-balance text-lg text-[var(--color-text-subtle)] sm:text-xl">
				{t('landing.hero_subtitle', 'Join thousands of Nigerians sharing stories, comparing destinations, and navigating life abroad together.')}
			</p>
			<div class="mt-10 flex flex-col items-center justify-center gap-3 sm:flex-row">
				<a href="/signup" class="flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-6 py-3 text-sm font-semibold text-[var(--color-primary-contrast)] shadow-sm transition-all hover:bg-[var(--color-primary-hover)] hover:shadow-md">
					{t('landing.hero_cta', 'Join the community')} <ArrowRight size={16} />
				</a>
				<a href="/explore" class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-6 py-3 text-sm font-semibold text-[var(--color-text)] transition-all hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]">
					<Compass size={16} /> {t('landing.hero_explore', 'Explore content')}
				</a>
			</div>
		</div>
	</section>

	<!-- Explore by destination -->
	{#if destinations.length > 0}
		<section class="mx-auto max-w-6xl px-6 py-16">
			<h2 class="mb-2 text-2xl font-bold text-[var(--color-text)]">{t('landing.destinations_title', 'Explore by destination')}</h2>
			<p class="mb-8 text-[var(--color-text-subtle)]">{t('landing.destinations_subtitle', 'Find content tailored to your destination country.')}</p>
			<div class="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
				{#each destinations as dest (dest.id)}
					<a href="/spaces/{dest.slug}" class="group rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5 transition-all hover:border-[var(--color-primary)] hover:shadow-sm">
						<h3 class="font-semibold text-[var(--color-text)] group-hover:text-[var(--color-primary)]">{dest.name}</h3>
						{#if dest.description}
							<p class="mt-1 line-clamp-2 text-sm text-[var(--color-text-subtle)]">{dest.description}</p>
						{/if}
					</a>
				{/each}
			</div>
		</section>
	{/if}

	<!-- Trending discussions -->
	{#if exploreItems.length > 0}
		<section class="border-y border-[var(--color-border)] bg-[var(--color-surface-subtle)]">
			<div class="mx-auto max-w-4xl px-6 py-16">
				<div class="mb-8 flex items-center gap-2">
					<MessageCircle size={22} class="text-[var(--color-primary)]" />
					<h2 class="text-2xl font-bold text-[var(--color-text)]">{t('landing.trending_title', 'Trending discussions')}</h2>
				</div>
				<div class="space-y-3">
					{#each exploreItems.slice(0, 5) as item, i (item.id)}
						<a href={item.content_type === 'blog_post' ? `/blog/${item.slug}` : `/content/${item.slug}`} class="group flex items-start gap-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5 transition-all hover:border-[var(--color-primary)]/40 hover:shadow-md">
							<span class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-bold text-[var(--color-muted)]">{i + 1}</span>
							<div class="min-w-0 flex-1">
								<div class="mb-1 flex items-center gap-2 text-xs text-[var(--color-text-subtle)]">
									{#if item.author_name}<span class="font-medium">{item.author_name}</span>{/if}
									{#if item.space_name}<span>·</span><span>{item.space_name}</span>{/if}
									<span class="rounded-full bg-[var(--color-primary)]/8 px-2 py-0.5 text-[var(--color-primary)]">{item.content_type.replace(/_/g, ' ')}</span>
								</div>
								<h3 class="font-semibold leading-snug text-[var(--color-text)] group-hover:text-[var(--color-primary)]">{item.title}</h3>
								{#if item.excerpt}
									<p class="mt-1 line-clamp-2 text-sm text-[var(--color-text-subtle)]">{item.excerpt}</p>
								{/if}
								<div class="mt-3 flex items-center gap-4 text-xs text-[var(--color-muted)]">
									<span>▲ {item.vote_score ?? 0}</span>
									<span>💬 {item.comment_count ?? 0}</span>
									<span>👁 {item.view_count ?? 0}</span>
								</div>
							</div>
						</a>
					{/each}
				</div>
				<a href="/explore" class="mt-6 inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary)] hover:underline">
					{t('landing.see_all', 'See all content')} <ArrowRight size={14} />
				</a>
			</div>
		</section>
	{/if}

	<!-- Latest blog posts — Ghost.org magazine style with serif titles -->
	{#if blogPosts.length > 0}
		<section class="mx-auto max-w-6xl px-6 py-16">
			<div class="mb-8 flex items-center gap-2">
				<PenLine size={22} class="text-[var(--color-primary)]" />
				<h2 class="text-2xl font-bold text-[var(--color-text)]">{t('landing.latest_blog', 'From the blog')}</h2>
			</div>
			<div class="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
				{#each blogPosts.slice(0, 6) as post (post.id)}
					<a href="/blog/{post.slug}" class="group flex flex-col overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-all hover:border-[var(--color-primary)]/40 hover:shadow-lg">
						<div class="aspect-[16/9] bg-gradient-to-br from-[var(--color-primary)]/12 via-[var(--color-surface-subtle)] to-[var(--color-surface-subtle)]"></div>
						<div class="flex flex-1 flex-col p-5">
							<h3 class="text-xl font-semibold leading-snug text-[var(--color-text)] group-hover:text-[var(--color-primary)]" style="font-family: var(--font-serif);">{post.title}</h3>
							<p class="mt-2 line-clamp-3 flex-1 text-sm text-[var(--color-text-subtle)]">{post.body.slice(0, 160)}</p>
							<div class="mt-4 flex items-center gap-3 text-xs text-[var(--color-muted)]">
								{#if post.read_time_minutes > 0}<span class="flex items-center gap-1"><Clock size={12} /> {post.read_time_minutes} {t('blog.min_read', 'min read')}</span>{/if}
								<span class="flex items-center gap-1"><Eye size={12} /> {post.view_count}</span>
							</div>
						</div>
					</a>
				{/each}
			</div>
			<a href="/blog" class="mt-6 inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary)] hover:underline">
				{t('landing.more_posts', 'More articles')} <ArrowRight size={14} />
			</a>
		</section>
	{/if}

	<!-- Newsletter form embed -->
	<section class="border-y border-[var(--color-border)] bg-[var(--color-surface-subtle)]">
		<div class="mx-auto max-w-2xl px-6 py-16 text-center">
			<h2 class="text-2xl font-bold text-[var(--color-text)]">{t('landing.newsletter_title', 'Stay in the loop')}</h2>
			<p class="mt-2 text-[var(--color-text-subtle)]">{t('landing.newsletter_subtitle', 'Get the best diaspora stories delivered to your inbox weekly.')}</p>
			{#if subscribed}
				<p class="mt-6 rounded-[var(--radius)] bg-[var(--color-primary)]/10 px-4 py-3 text-sm font-medium text-[var(--color-primary)]">
					{t('landing.newsletter_thanks', 'Thanks for subscribing! Check your inbox to confirm.')}
				</p>
			{:else}
				<form onsubmit={subscribe} class="mx-auto mt-6 flex max-w-md gap-2">
					<input
						type="email"
						bind:value={email}
						placeholder={t('landing.newsletter_placeholder', 'your@email.com')}
						required
						disabled={subscribing}
						class="flex-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2.5 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none disabled:opacity-50"
					/>
					<button type="submit" disabled={subscribing} class="rounded-[var(--radius)] bg-[var(--color-primary)] px-5 py-2.5 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50">
						{subscribing ? t('common.submitting', 'Submitting…') : t('landing.newsletter_subscribe', 'Subscribe')}
					</button>
				</form>
				{#if subscribeError}
					<p class="mt-3 text-sm text-[var(--color-danger)]">{subscribeError}</p>
				{/if}
			{/if}
		</div>
	</section>

	<!-- CTA banner -->
	<section class="mx-auto max-w-4xl px-6 py-20 text-center">
		<div class="rounded-[var(--radius-card)] bg-[var(--color-primary)] px-8 py-12 text-white">
			<Users size={32} class="mx-auto mb-4" />
			<h2 class="text-3xl font-bold">{t('landing.cta_title', 'Ready to join the community?')}</h2>
			<p class="mx-auto mt-3 max-w-xl text-white/80">{t('landing.cta_subtitle', 'Create a free account and start sharing your journey, asking questions, and connecting with fellow Nigerians abroad.')}</p>
			<a href="/signup" class="mt-8 inline-flex items-center gap-2 rounded-[var(--radius)] bg-white px-6 py-3 text-sm font-semibold text-[var(--color-primary)] transition-colors hover:bg-white/90">
				{t('landing.cta_button', 'Create your account')} <ArrowRight size={16} />
			</a>
		</div>
	</section>
</div>
