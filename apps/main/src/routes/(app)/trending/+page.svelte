<script lang="ts">
	import { page } from '$app/state';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import { goto } from '$app/navigation';
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import {
		Flame,
		Hash,
		Layers,
		Users,
		Eye,
		ArrowUp,
		Building2
	} from 'lucide-svelte';
	import { EmptyState, ListPageBar, UrlTabs, AutocompleteSelect, type UrlTab } from '$lib/components/ui';
	import { trendingWindowOptions, trendingTabs } from '$lib/enumOptions';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';
	import { contentHref } from '$lib/utils/contentLink';
	import type { PageData } from './$types';
	import type { TrendingWindow } from '$lib/types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('trending.title', 'Trending — {site}'));
	const description = $derived(
		t(
			'trending.description',
			'The hottest content, tags, spaces, and people on {site} right now.'
		)
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: 'Trending',
			description,
			url: page.url.href
		})
	);

	const tabs = $derived(trendingTabs(t));

	const urlTabs = $derived<UrlTab[]>(
		tabs.map((tab) => ({ value: tab.id, label: tab.label }))
	);

	// Instant client-side tab switching — all tab data is loaded server-side
	// regardless of ?tab=, so we switch without navigation (UrlTabs syncs the
	// URL via replaceState, preserving other params like ?window=).
	let activeTab = $state(untrack(() => data.tab));

	const windows = $derived(trendingWindowOptions(t));

	// Round the trending score for display (Rule: round every displayed number).
	const score = (n: number) => Math.round(n);

	// Build the canonical URL for a trending content item via the shared
	// contentHref mapping — every content type lands directly on its own
	// route family (blog posts /blog/{slug}, threads /forum/{slug}, guides
	// /guide/{slug}, …) instead of bouncing through /content's 301.
	function contentUrl(item: { content_type: string; slug: string }): string {
		return contentHref(item);
	}

	function windowHref(w: TrendingWindow): string {
		const params = new URLSearchParams(page.url.searchParams);
		params.set('window', w);
		return `?${params.toString()}`;
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={title} />
	<meta name="twitter:description" content={description} />
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<div class="mx-auto w-full max-w-full" style="--page-gutter: max(1rem, calc((100% - 48rem)/2))">
	<!-- Header: title + description (kept) -->
	<div class="border-b border-[var(--color-border)] px-4 py-4" style="margin-inline: var(--page-gutter)">
		<div class="min-w-0">
			<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
				<Flame size={20} /> {t('trending.heading', 'Trending')}
			</h1>
			<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
				{t('trending.subtitle', 'What the community is engaging with.')}
			</p>
		</div>
	</div>

	<!-- Tab + window bar — ListPageBar pattern, same as /guides -->
	<ListPageBar>
		<UrlTabs tabs={urlTabs} bind:value={activeTab} class="!border-b-0 !bg-transparent" />
		{#snippet actions()}
			<!-- Window selector (24h / 7d / 30d) — shared Autocomplete control -->
			<AutocompleteSelect
				value={data.window}
				options={windows}
				ariaLabel={t('trending.window_label', 'Time window')}
				class="shrink-0 w-32"
				onValueChange={(v) => {
					if (v && v !== data.window) goto(windowHref(v as TrendingWindow));
				}}
			/>
		{/snippet}
	</ListPageBar>

	<div class="mx-auto w-full max-w-3xl p-4">
		{#if activeTab === 'content'}
			<!-- Content leaderboard -->
			{#if data.content.length === 0}
				<EmptyState
					icon={Flame}
					title={t('trending.empty_content', 'No trending content in this window yet.')}
				/>
			{:else}
				<ol class="space-y-2">
					{#each data.content as item, i (item.id)}
						<li>
							<a
								href={contentUrl(item)}
								class="group flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
							>
								<span
									class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-semibold text-[var(--color-text-subtle)]"
								>
									{i + 1}
								</span>
								<span class="min-w-0 flex-1">
									<span
										class="block truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
									>
										{item.title}
									</span>
									<span
										class="mt-0.5 flex items-center gap-3 text-xs text-[var(--color-muted)]"
									>
										<span class="capitalize">{item.content_type.replace('_', ' ')}</span>
										<span class="flex items-center gap-1">
											<Eye size={12} /> {item.view_count}
										</span>
									</span>
								</span>
								<span
									class="flex shrink-0 items-center gap-0.5 text-xs font-medium text-[var(--color-primary)]"
								>
									<ArrowUp size={12} /> {score(item.score)}
								</span>
							</a>
						</li>
					{/each}
				</ol>
			{/if}
		{:else if activeTab === 'tags'}
			<!-- Tags leaderboard -->
			{#if data.tags.length === 0}
				<EmptyState
					icon={Hash}
					title={t('trending.empty_tags', 'No trending tags in this window yet.')}
				/>
			{:else}
				<ol class="space-y-2">
					{#each data.tags as item, i (item.id)}
						<li>
							<a
								href="/tags/{item.slug}"
								class="group flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
							>
								<span
									class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-semibold text-[var(--color-text-subtle)]"
								>
									{i + 1}
								</span>
								<span class="min-w-0 flex-1">
									<span
										class="block truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
									>
										#{item.name}
									</span>
									<span class="text-xs text-[var(--color-muted)]">
										{item.follower_count}
										{t('trending.followers', 'followers')}
									</span>
								</span>
								<span
									class="flex shrink-0 items-center gap-0.5 text-xs font-medium text-[var(--color-primary)]"
								>
									<ArrowUp size={12} /> {score(item.score)}
								</span>
							</a>
						</li>
					{/each}
				</ol>
			{/if}
		{:else if activeTab === 'spaces'}
			<!-- Spaces leaderboard -->
			{#if data.spaces.length === 0}
				<EmptyState
					icon={Layers}
					title={t('trending.empty_spaces', 'No trending spaces in this window yet.')}
				/>
			{:else}
				<ol class="space-y-2">
					{#each data.spaces as item, i (item.id)}
						<li>
							<a
								href="/spaces/{item.slug}"
								class="group flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
							>
								<span
									class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-semibold text-[var(--color-text-subtle)]"
								>
									{i + 1}
								</span>
								<span class="min-w-0 flex-1">
									<span
										class="block truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
									>
										{item.name}
									</span>
									<span
										class="mt-0.5 flex items-center gap-3 text-xs text-[var(--color-muted)]"
									>
										<span class="flex items-center gap-1">
											<Users size={12} /> {item.member_count}
										</span>
										<span>{item.post_count} {t('trending.posts', 'posts')}</span>
									</span>
								</span>
								<span
									class="flex shrink-0 items-center gap-0.5 text-xs font-medium text-[var(--color-primary)]"
								>
									<ArrowUp size={12} /> {score(item.score)}
								</span>
							</a>
						</li>
					{/each}
				</ol>
			{/if}
		{:else if activeTab === 'users'}
			<!-- People leaderboard -->
			{#if data.users.length === 0}
				<EmptyState
					icon={Users}
					title={t('trending.empty_users', 'No trending people in this window yet.')}
				/>
			{:else}
				<ol class="space-y-2">
					{#each data.users as item, i (item.id)}
						<li>
							<UserHoverCard
								id={item.id}
								slug={item.username}
								name={item.display_name ?? item.username}
								avatar={item.avatar_url}
								class="group inline-flex w-full min-w-0 max-w-full items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-start transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
							>
								{#snippet trigger()}
									<span
										class="grid h-8 w-8 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-semibold text-[var(--color-text-subtle)]"
									>
										{i + 1}
									</span>
									<span class="grid h-9 w-9 shrink-0 place-items-center overflow-hidden rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-muted)]">
										{#if item.avatar_url}
											<img
												src={item.avatar_url}
												alt={item.username}
												class="h-full w-full object-cover"
												loading="lazy"
											/>
										{:else}
											<Building2 size={16} />
										{/if}
									</span>
									<span class="min-w-0 flex-1">
										<span
											class="block truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
										>
											{item.display_name ?? item.username}
										</span>
										<span class="block truncate text-xs text-[var(--color-muted)]">
											@{item.username}
										</span>
									</span>
									<span
										class="flex shrink-0 items-center gap-0.5 text-xs font-medium text-[var(--color-primary)]"
									>
										<ArrowUp size={12} /> {score(item.score)}
									</span>
								{/snippet}
							</UserHoverCard>
						</li>
					{/each}
				</ol>
			{/if}
		{/if}
	</div>
</div>
