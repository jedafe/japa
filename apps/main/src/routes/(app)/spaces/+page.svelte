<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { Layers, Search, Users, MessageSquare, Plus } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import SpaceCreationModal from '$lib/components/spaces/SpaceCreationModal.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let showCreateModal = $state(false);

	const title = $derived(t('spaces.title', 'Spaces — JapaConnect'));
	const description = $derived(
		t('spaces.description', 'Discover and join spaces — countries, states, cities, and topical communities.')
	);

	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'CollectionPage',
			name: 'Spaces',
			description,
			url: page.url.href
		})
	);

	let query = $state(untrack(() => data.q));
	const searchPlaceholder = $derived(t('spaces.search_placeholder', 'Search spaces…'));
	let filtered = $derived(
		query.trim()
			? data.spaces.filter((s) =>
					s.name.toLowerCase().includes(query.trim().toLowerCase())
				)
			: data.spaces
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
		<div class="flex items-center justify-between">
			<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
				<Layers size={20} /> {t('spaces.heading', 'Spaces')}
			</h1>
			{#if data.user}
				<button
					type="button"
					onclick={() => (showCreateModal = true)}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-white transition hover:opacity-90"
				>
					<Plus size={16} /> {t('spaces.create_button')}
				</button>
			{/if}
		</div>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{t('spaces.subtitle', 'Join spaces to follow their feed and post.')}
		</p>

		<form class="mt-3" action="/spaces" method="get" role="search">
			<div
				class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 focus-within:border-[var(--color-primary)]"
			>
				<Search size={16} class="text-[var(--color-muted)]" />
				<input
					type="search"
					name="q"
					placeholder={searchPlaceholder}
					aria-label={t('spaces.heading', 'Spaces')}
					bind:value={query}
					class="w-full bg-transparent text-sm text-[var(--color-text)] outline-none placeholder:text-[var(--color-muted)]"
				/>
			</div>
		</form>
	</div>

	<div class="p-4">
		{#if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('spaces.load_error', 'Failed to load spaces. Please try again later.')}
			</p>
		{:else if data.spaces.length === 0}
			<EmptyState icon={Layers} title={t('spaces.empty', 'No spaces yet.')} />
		{:else if filtered.length === 0}
			<EmptyState
				icon={Search}
				title={t('spaces.no_match', 'No spaces match your search.')}
			/>
		{:else}
			<div class="grid gap-3 sm:grid-cols-2">
				{#each filtered as space (space.id)}
					<a
						href="/spaces/{space.slug}"
						class="group flex items-start gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
					>
						<span
							class="grid h-11 w-11 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
						>
							<Layers size={20} />
						</span>
						<span class="min-w-0 flex-1">
							<span
								class="block truncate text-sm font-medium text-[var(--color-text)] group-hover:text-[var(--color-primary)]"
							>
								{space.name}
							</span>
							{#if space.description}
								<span
									class="mt-1 line-clamp-2 block text-xs text-[var(--color-text-subtle)]"
								>
									{space.description}
								</span>
							{/if}
							<span
								class="mt-2 flex items-center gap-3 text-xs text-[var(--color-muted)]"
							>
								<span class="flex items-center gap-1">
									<Users size={12} /> {space.member_count}
								</span>
								<span class="flex items-center gap-1">
									<MessageSquare size={12} /> {space.post_count}
								</span>
							</span>
						</span>
					</a>
				{/each}
			</div>
		{/if}
	</div>
</div>

<SpaceCreationModal bind:open={showCreateModal} />
