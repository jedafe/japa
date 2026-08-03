<script lang="ts">
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { Layers, Search, Users, MessageSquare, Plus, ShieldX, ChevronLeft, ChevronRight } from 'lucide-svelte';
	import { EmptyState, SpaceJoinButton } from '$lib/components/ui';
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
	let selectedType = $state(untrack(() => data.type));
	const searchPlaceholder = $derived(t('spaces.search_placeholder', 'Search spaces…'));

	function buildHref(pageNum: number): string {
		const params = new URLSearchParams();
		if (data.q) params.set('q', data.q);
		if (data.type) params.set('type', data.type);
		if (pageNum > 0) params.set('page', String(pageNum));
		const qs = params.toString();
		return qs ? `/spaces?${qs}` : '/spaces';
	}

	function onFilterChange() {
		const params = new URLSearchParams();
		if (query.trim()) params.set('q', query.trim());
		if (selectedType) params.set('type', selectedType);
		const qs = params.toString();
		window.location.href = qs ? `/spaces?${qs}` : '/spaces';
	}

	// Look up the space type name by ID from the data-driven type list.
	const spaceTypeMap = $derived(
		data.spaceTypes.reduce((m, t) => { m[t.id] = t.name; return m; }, {} as Record<string, string>)
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

<div class="mx-auto w-full max-w-full">
	<!-- Header + search + filters -->
	<div class="border-b border-[var(--color-border)] px-4 py-4">
		<div class="flex items-center justify-between">
			<div>
				<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
					<Layers size={20} /> {t('spaces.heading', 'Spaces')}
				</h1>
				<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
					{t('spaces.subtitle', 'Join spaces to connect with people who share your interests.')}
				</p>
			</div>
			{#if page.data.user}
				<button
					type="button"
					onclick={() => (showCreateModal = true)}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-white hover:bg-[var(--color-primary)]/90"
				>
					<Plus size={16} /> {t('spaces.create', 'Create')}
				</button>
			{/if}
		</div>

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
			{#if selectedType}
				<input type="hidden" name="type" value={selectedType} />
			{/if}
		</form>

		<!-- Space type filter chips (data-driven from GET /space-types) -->
		{#if data.spaceTypes.length > 0}
			<div class="mt-3 flex flex-wrap gap-2">
				<button
					type="button"
					onclick={() => { selectedType = ''; onFilterChange(); }}
					class="rounded-full border px-3 py-1 text-xs transition-colors {selectedType === ''
						? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
						: 'border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)]/40'}"
				>
					All types
				</button>
				{#each data.spaceTypes as st (st.id)}
					<button
						type="button"
						onclick={() => { selectedType = st.slug; onFilterChange(); }}
						class="rounded-full border px-3 py-1 text-xs transition-colors {selectedType === st.slug
							? 'border-[var(--color-primary)] bg-[var(--color-primary)]/10 text-[var(--color-primary)]'
							: 'border-[var(--color-border)] text-[var(--color-muted)] hover:border-[var(--color-primary)]/40'}"
					>
						{st.name}
					</button>
				{/each}
			</div>
		{/if}
	</div>

	<div class="p-4">
		{#if data.forbidden}
			<div class="flex flex-col items-center gap-3 py-16 text-center">
				<ShieldX size={40} class="text-[var(--color-muted)]" />
				<p class="text-sm font-medium text-[var(--color-text)]">
					{t('spaces.forbidden', 'You do not have permission to view this directory.')}
				</p>
			</div>
		{:else if data.loadError}
			<p class="py-12 text-center text-[var(--color-text-subtle)]">
				{t('spaces.load_error', 'Failed to load spaces. Please try again later.')}
			</p>
		{:else if data.spaces.length === 0}
			<EmptyState icon={Layers} title={t('spaces.empty', 'No spaces yet.')} />
		{:else}
			<p class="mb-3 text-xs text-[var(--color-muted)]">
				{data.total} {t('spaces.results', 'spaces')}
			</p>
			<div class="grid gap-3 sm:grid-cols-2">
				{#each data.spaces as space (space.id)}
					<div
						class="group flex items-start gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:border-[var(--color-primary)]/40 hover:bg-[var(--color-surface-subtle)]"
					>
						<a href="/spaces/{space.slug}" class="flex min-w-0 flex-1 items-start gap-3">
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
									<span class="mt-1 line-clamp-2 block text-xs text-[var(--color-text-subtle)]">
										{space.description}
									</span>
								{/if}
								<span class="mt-2 flex items-center gap-3 text-xs text-[var(--color-muted)]">
									<span class="flex items-center gap-1">
										<Users size={12} /> {space.member_count}
									</span>
									<span class="flex items-center gap-1">
										<MessageSquare size={12} /> {space.post_count}
									</span>
									<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-[var(--color-text-subtle)]">
										{spaceTypeMap[space.space_type_id] ?? "Space"}
									</span>
								</span>
							</span>
						</a>
						{#if page.data.user}
							<SpaceJoinButton entityId={space.id} entitySlug={space.slug} compact />
						{/if}
					</div>
				{/each}
			</div>

			<!-- Pagination -->
			{#if data.totalPages > 1}
				<div class="mt-6 flex items-center justify-center gap-4">
					<a
						href={buildHref(data.page - 1)}
						class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] {data.page === 0 ? 'pointer-events-none opacity-40' : ''}"
					>
						<ChevronLeft size={16} /> {t('common.prev', 'Prev')}
					</a>
					<span class="text-xs text-[var(--color-muted)]">
						{data.page + 1} / {data.totalPages}
					</span>
					<a
						href={buildHref(data.page + 1)}
						class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] {data.page >= data.totalPages - 1 ? 'pointer-events-none opacity-40' : ''}"
					>
						{t('common.next', 'Next')} <ChevronRight size={16} />
					</a>
				</div>
			{/if}
		{/if}
	</div>
</div>

<SpaceCreationModal bind:open={showCreateModal} />
