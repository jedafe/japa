<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { Hash, Search, Users } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const title = $derived(t('tags.title', 'Tags — JapaConnect'));
	const description = $derived(
		t('tags.description', 'Browse all tags on JapaConnect and find content that matters to you.')
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: 'Tags',
			description,
			url: page.url.href
		})
	);

	// Client-side filter refines the SSR list without a round-trip; the server
	// `q` param seeds the input so deep links and back/forward keep their state.
	let query = $state(untrack(() => data.q));
	const searchPlaceholder = $derived(t('tags.search_placeholder', 'Search tags…'));
	let filtered = $derived(
		query.trim()
			? data.tags.filter((tag) =>
					tag.name.toLowerCase().includes(query.trim().toLowerCase())
				)
			: data.tags
	);
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
	{@html `<script type="application/ld+json">${jsonLd}</script>`}
</svelte:head>

<div class="mx-auto w-full max-w-4xl">
	<!-- Header + search -->
	<div class="border-b border-[var(--color-border)] px-4 py-4">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Hash size={20} /> {t('tags.heading', 'Tags')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{t('tags.subtitle', 'Follow tags to shape your feed.')}
		</p>

		<form class="mt-3" action="/tags" method="get" role="search">
			<div
				class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 focus-within:border-[var(--color-primary)]"
			>
				<Search size={16} class="text-[var(--color-muted)]" />
				<input
					type="search"
					name="q"
					placeholder={searchPlaceholder}
					aria-label={t('tags.heading', 'Tags')}
					bind:value={query}
					class="w-full bg-transparent text-sm text-[var(--color-text)] outline-none placeholder:text-[var(--color-muted)]"
				/>
			</div>
		</form>
	</div>

	<div class="p-4">
		{#if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('tags.load_error', 'Failed to load tags. Please try again later.')}
			</p>
		{:else if data.tags.length === 0}
			<EmptyState icon={Hash} title={t('tags.empty', 'No tags yet.')} />
		{:else if filtered.length === 0}
			<EmptyState
				icon={Search}
				title={t('tags.no_match', 'No tags match your search.')}
			/>
		{:else}
			<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each filtered as tag (tag.id)}
					<a
						href="/tags/{tag.slug}"
						class="group flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
					>
						{#if tag.logo_url}
							<img
								src={tag.logo_url}
								alt={tag.name}
								loading="lazy"
								class="h-10 w-10 shrink-0 rounded-[var(--radius)] border border-[var(--color-border)] object-cover"
							/>
						{:else}
							<span
								class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
							>
								<Hash size={18} />
							</span>
						{/if}
						<span class="min-w-0 flex-1">
							<span
								class="block truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
							>
								#{tag.name}
							</span>
							<span
								class="mt-1 flex items-center gap-1 text-xs text-[var(--color-muted)]"
							>
								<Users size={12} />
								{tag.follower_count}
								{t('tags.followers', 'followers')}
							</span>
						</span>
					</a>
				{/each}
			</div>
		{/if}
	</div>
</div>
