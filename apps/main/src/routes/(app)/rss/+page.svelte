<script lang="ts">
	import { t, siteName } from '$lib/i18n';
	import { page } from '$app/state';
	import { Rss, Copy, Check, ExternalLink } from 'lucide-svelte';
	import type { PageData } from './$types';

	/**
	 * RSS directory — every feed surface on the platform, grouped by entity.
	 * Static platform feeds are fixed; space/tag/user feeds are
	 * per-entity URL patterns with a live sample list from the API.
	 *
	 * Ref: japaconnect-engineering/sections/26-seo-performance.md
	 */
	let { data }: { data: PageData } = $props();

	type FeedLink = { name: string; desc: string; path: string };

	const platformFeeds: FeedLink[] = [
		{
			name: t('rss.feed.everything', 'Everything'),
			desc: t('rss.feed.everything_desc', 'All published content — stories, guides, blog posts, and forum threads'),
			path: '/feed.xml'
		},
		{
			name: t('rss.feed.blog', 'Blog'),
			desc: t('rss.feed.blog_desc', 'Official posts from the {site} team'),
			path: '/blog/feed.xml'
		},
		{
			name: t('rss.feed.forum', 'Forum'),
			desc: t('rss.feed.forum_desc', 'Latest discussions across public spaces'),
			path: '/forum/feed.xml'
		},
		{
			name: t('rss.feed.releases', 'Releases'),
			desc: t('rss.feed.releases_desc', 'Product releases and version history'),
			path: '/roadmap/releases/feed.xml'
		}
	];

	// Per-content-type feeds — one per ADR-058 route family. Registry mirrors
	// internal/rss/model TypeFeeds.
	const typeFeeds: FeedLink[] = [
		{ name: t('rss.feed.stories', 'Stories'), desc: '', path: '/story/feed.xml' },
		{ name: t('rss.feed.guides', 'Guides'), desc: '', path: '/guide/feed.xml' },
		{ name: t('rss.feed.comparisons', 'Comparisons'), desc: '', path: '/comparisons/feed.xml' },
		{ name: t('rss.feed.jobs', 'Jobs'), desc: '', path: '/jobs/feed.xml' },
		{ name: t('rss.feed.services', 'Services'), desc: '', path: '/services/feed.xml' },
		{ name: t('rss.feed.videos', 'Videos'), desc: '', path: '/video/feed.xml' },
		{ name: t('rss.feed.lists', 'Lists'), desc: '', path: '/list/feed.xml' }
	];

	let copiedPath = $state<string | null>(null);
	let copyTimer: ReturnType<typeof setTimeout> | null = null;

	function absUrl(path: string): string {
		return `${page.url.origin}${path}`;
	}

	async function copyFeed(path: string) {
		try {
			await navigator.clipboard.writeText(absUrl(path));
			if (copyTimer) clearTimeout(copyTimer);
			copiedPath = path;
			copyTimer = setTimeout(() => (copiedPath = null), 1500);
		} catch {
			// Clipboard API unavailable (insecure context) — the href still works.
		}
	}
</script>

<svelte:head>
	<title>{t('rss.page.title', 'RSS Feeds')} — {siteName()}</title>
	<meta
		name="description"
		content={t('rss.page.meta_desc', 'Subscribe to {site} feeds: everything, blog, forum, releases, spaces, tags, and your own posts.')}
	/>
	<link rel="alternate" type="application/rss+xml" title={t('feed.rss_title', '{site} Feed')} href="/feed.xml" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-8">
	<header class="mb-6 flex items-start gap-3">
		<div class="mt-0.5 flex size-10 shrink-0 items-center justify-center rounded-lg bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
			<Rss size={20} />
		</div>
		<div>
			<h1 class="text-2xl font-bold text-[var(--color-text)]">{t('rss.page.title', 'RSS Feeds')}</h1>
			<p class="mt-1 text-sm text-[var(--color-muted)]">
				{t('rss.page.intro', 'Follow new posts, threads, and releases in your favorite RSS reader. Copy a feed URL and paste it into your reader — updates arrive automatically.')}
			</p>
		</div>
	</header>

	<!-- Platform-wide feeds -->
	<section class="mb-8">
		<h2 class="mb-3 text-sm font-semibold tracking-wide text-[var(--color-muted)] uppercase">
			{t('rss.section.platform', 'Platform feeds')}
		</h2>
		<div class="flex flex-col gap-2">
			{#each platformFeeds as feed (feed.path)}
				{@render feedRow(feed)}
			{/each}
		</div>
	</section>

	<!-- Per-content-type feeds -->
	<section class="mb-8">
		<h2 class="mb-3 text-sm font-semibold tracking-wide text-[var(--color-muted)] uppercase">
			{t('rss.section.topics', 'Topic feeds')}
		</h2>
		<p class="mb-3 text-xs text-[var(--color-muted)]">
			{t('rss.section.topics_hint', 'One feed per content type — subscribe to just what you follow.')}
		</p>
		<div class="flex flex-col gap-2">
			{#each typeFeeds as feed (feed.path)}
				{@render feedRow(feed)}
			{/each}
		</div>
	</section>

	<!-- The viewer's own feed -->
	<section class="mb-8">
		<h2 class="mb-3 text-sm font-semibold tracking-wide text-[var(--color-muted)] uppercase">
			{t('rss.section.yours', 'Your feed')}
		</h2>
		{#if data.viewerUsername}
			<div class="flex flex-col gap-2">
				{@render feedRow({
					name: t('rss.feed.yours', 'Your posts'),
					desc: t('rss.feed.yours_desc', 'Everything you publish, for your followers'),
					path: `/u/${data.viewerUsername}/feed.xml`
				})}
			</div>
		{:else}
			<p class="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-sm text-[var(--color-muted)]">
				{t('rss.section.yours_guest', 'Every member gets a personal feed at /u/{username}/feed.xml — sign in to see yours.')}
			</p>
		{/if}
	</section>

	<!-- Space feeds -->
	<section class="mb-8">
		<h2 class="mb-3 text-sm font-semibold tracking-wide text-[var(--color-muted)] uppercase">
			{t('rss.section.spaces', 'Space feeds')}
		</h2>
		<p class="mb-3 text-xs text-[var(--color-muted)]">
			{t('rss.section.spaces_hint', 'Every public space has a feed at /spaces/{space}/feed.xml.')}
		</p>
		{#if data.spaces.length > 0}
			<div class="flex flex-col gap-2">
				{#each data.spaces as space (space.slug)}
					{@render feedRow({ name: space.name, desc: '', path: `/spaces/${space.slug}/feed.xml` })}
				{/each}
			</div>
			<a href="/spaces" class="mt-3 inline-block text-sm font-medium text-[var(--color-primary)] hover:underline">
				{t('rss.section.browse_spaces', 'Browse all spaces →')}
			</a>
		{:else if !data.loadError}
			<p class="text-sm text-[var(--color-muted)]">{t('rss.section.no_spaces', 'No public spaces yet.')}</p>
		{/if}
	</section>

	<!-- Tag feeds -->
	<section class="mb-8">
		<h2 class="mb-3 text-sm font-semibold tracking-wide text-[var(--color-muted)] uppercase">
			{t('rss.section.tags', 'Tag feeds')}
		</h2>
		<p class="mb-3 text-xs text-[var(--color-muted)]">
			{t('rss.section.tags_hint', 'Every tag has a feed at /tags/{tag}/feed.xml.')}
		</p>
		{#if data.tags.length > 0}
			<div class="flex flex-col gap-2">
				{#each data.tags as tag (tag.slug)}
					{@render feedRow({ name: `#${tag.name}`, desc: '', path: `/tags/${tag.slug}/feed.xml` })}
				{/each}
			</div>
			<a href="/tags" class="mt-3 inline-block text-sm font-medium text-[var(--color-primary)] hover:underline">
				{t('rss.section.browse_tags', 'Browse all tags →')}
			</a>
		{:else if !data.loadError}
			<p class="text-sm text-[var(--color-muted)]">{t('rss.section.no_tags', 'No tags yet.')}</p>
		{/if}
	</section>

	{#if data.loadError}
		<p class="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-xs text-[var(--color-muted)]">
			{t('rss.section.load_error', 'Some per-entity feeds could not be loaded — the platform feeds above still work.')}
		</p>
	{/if}
</div>

{#snippet feedRow(feed: FeedLink)}
	<div class="flex items-center gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
		<Rss size={16} class="shrink-0 text-[var(--color-primary)]" />
		<div class="min-w-0 flex-1">
			<div class="truncate text-sm font-medium text-[var(--color-text)]">{feed.name}</div>
			{#if feed.desc}
				<div class="truncate text-xs text-[var(--color-muted)]">{feed.desc}</div>
			{/if}
			<code class="mt-0.5 block truncate text-xs text-[var(--color-muted)]">{feed.path}</code>
		</div>
		<button
			type="button"
			class="flex shrink-0 items-center gap-1 rounded-md border border-[var(--color-border)] px-2.5 py-1.5 text-xs font-medium text-[var(--color-text)] transition hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]"
			title={t('rss.copy', 'Copy feed URL')}
			onclick={() => copyFeed(feed.path)}
		>
			{#if copiedPath === feed.path}
				<Check size={14} class="text-green-600" />
				{t('rss.copied', 'Copied')}
			{:else}
				<Copy size={14} />
				{t('rss.copy', 'Copy')}
			{/if}
		</button>
		<a
			href={feed.path}
			target="_blank"
			rel="noopener"
			class="flex shrink-0 items-center gap-1 rounded-md border border-[var(--color-border)] px-2.5 py-1.5 text-xs font-medium text-[var(--color-text)] transition hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]"
			title={t('rss.open', 'Open feed')}
		>
			<ExternalLink size={14} />
		</a>
	</div>
{/snippet}
