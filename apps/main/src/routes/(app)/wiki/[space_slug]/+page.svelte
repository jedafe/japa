<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md
	// Geo-aware pattern: destination selector is the first UI input.
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import EntityFieldsPanel from '$lib/components/wiki/EntityFieldsPanel.svelte';
	import { BookOpen, Lock, Star, ArrowLeft, Globe, GraduationCap, Plus, Table2 } from 'lucide-svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import type { WikiCategory, WikiPageIndex, GeoCountry, WikiDestination } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// The canonical category list is scoped to the space type server-side:
	// university spaces get the university set (§80 amendment). Falls back to
	// the page-derived categories if the list is empty.
	type ApiCategory = { slug: string; label_key: string; label: string };
	// svelte-ignore state_referenced_locally
	const apiCategories: ApiCategory[] = data.apiCategories ?? [];
	// svelte-ignore state_referenced_locally
	const isUniversityWiki = data.wikiScope === 'university';

	// Derive the ordered category list from the API response. If the API
	// hasn't loaded yet, derive from the loaded pages (data-driven fallback).
	const CATEGORIES = $derived.by<WikiCategory[]>(() => {
		if (apiCategories.length > 0) {
			return apiCategories.map((c) => c.slug as WikiCategory);
		}
		// Fallback: derive unique categories from loaded pages, plus 'other'.
		const seen = new Set<WikiCategory>();
		for (const page of data.index) {
			seen.add(page.category as WikiCategory);
		}
		seen.add('other');
		return Array.from(seen);
	});

	function categoryLabel(cat: WikiCategory): string {
		// Use the API-provided label_key if available, else fall back to
		// the convention-based key.
		const apiCat = apiCategories.find((c) => c.slug === cat);
		const key = apiCat?.label_key ?? `wiki.category_${cat}`;
		return t(key, cat.replace(/_/g, ' '));
	}

	// Resolve the space name from the space detail (authoritative — present
	// even for spaces with no pages), then the first page's space_name, then
	// the slug.
	const spaceName = $derived(
		data.spaceDetail?.space?.name ?? data.index[0]?.space_name ?? data.spaceSlug
	);

	// Geo-aware destination selector: load the canonical country list from
	// /geo/countries (Rule 16 — no hardcoded lists, ADR-005). The selector is
	// the first input per the geo-aware tool pattern.
	let destOptions = $state<GeoCountry[]>([]);
	let destLoading = $state(false);

	$effect(() => {
		// University wikis use a university search instead — skip the geo load.
		if (isUniversityWiki) return;
		// Load country-level spaces for the destination selector.
		(async () => {
			destLoading = true;
			try {
				const res = await api.get<GeoCountry[]>('/geo/countries');
				destOptions = res.data ?? [];
			} catch {
				/* keep empty options */
			} finally {
				destLoading = false;
			}
		})();
	});

	// University selector: server-side search — the set is ~10k, too large
	// for a client-side list. Requires a query.
	async function searchUniversities(q: string): Promise<AutocompleteItem[]> {
		const query = q.trim();
		if (!query) return [];
		try {
			const params = new URLSearchParams({ level: 'university', has_wiki: 'false', q: query });
			const res = await api.get<WikiDestination[]>(`/wiki/destinations?${params.toString()}`);
			return (res.data ?? []).map((d) => ({
				id: d.slug,
				label: d.name,
				sublabel: d.geo_code ?? ''
			}));
		} catch {
			return [];
		}
	}

	function fetchDestinations(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase().trim();
		const filtered = query === ''
			? destOptions
			: destOptions.filter(
					(c) =>
						c.name.toLowerCase().includes(query) ||
						c.geo_code.toLowerCase().includes(query) ||
						c.slug.toLowerCase().includes(query)
				);
		return Promise.resolve(filtered.map((c) => ({ id: c.slug, label: c.name, sublabel: c.geo_code })));
	}
	function fetchDestinationsInitial(): Promise<AutocompleteItem[]> {
		return fetchDestinations('');
	}
	const selectedDestLabel = $derived(
		destOptions.find((c) => c.slug === data.spaceSlug)?.name ?? data.spaceSlug
	);

	// Group pages by category, preserving the CATEGORIES order.
	const grouped = $derived.by(() => {
		const map = new Map<WikiCategory, WikiPageIndex[]>();
		for (const cat of CATEGORIES) map.set(cat, []);
		for (const page of data.index) {
			const bucket = map.get(page.category as WikiCategory) ?? map.get('other')!;
			bucket.push(page);
		}
		return CATEGORIES.map((cat) => ({ cat, pages: map.get(cat) ?? [] })).filter(
			(g) => g.pages.length > 0
		);
	});

	const featured = $derived(data.index.filter((p) => p.is_featured));

	const title = $derived(`${spaceName} ${t('wiki.heading')} — JapaConnect`);
	const description = $derived(t('wiki.index_description'));
	const ogImage = $derived(
		data.spaceDetail?.header_url || data.spaceDetail?.logo_url || ''
	);
	const wikiEnabled = $derived(data.features?.['wiki.enabled'] !== false);
	const canCreate = $derived(data.authed && data.userPermissions?.includes('wiki.edit'));

	// §80 community field ops — the space itself is a community-editable
	// entity; its field groups render as a data panel on the index, and pages
	// bound to a child entity class (data_entity_type) carry a data badge.
	const structuredEditsOn = $derived(
		wikiEnabled && data.features?.['wiki.structured_edits'] === true
	);
	const spaceId = $derived(data.spaceDetail?.space?.id ?? data.index[0]?.space_id ?? '');
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	{#if ogImage}
		<meta property="og:image" content={ogImage} />
		<meta name="twitter:image" content={ogImage} />
		<meta name="twitter:card" content="summary_large_image" />
	{/if}
	<link rel="canonical" href={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="flex items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<a
			href="/wiki"
			class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={14} /> {t('wiki.back_to_index', 'Back')}
		</a>
		<h1 class="text-lg font-semibold text-[var(--color-text)]">
			{spaceName} — {t('wiki.heading', 'Wiki')}
		</h1>
	</div>

	<!-- Geo destination selector, or university search for university wikis -->
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		{#if isUniversityWiki}
			<label class="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
				<GraduationCap size={13} /> {t('wiki.universities_label', 'Universities')}
			</label>
			<div class="w-full max-w-sm">
				<Autocomplete
					placeholder={t('wiki.university_search_hint', 'Search a university…')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={searchUniversities}
					selectedId={data.spaceSlug || null}
					selectedLabel={spaceName}
					onSelect={(item) => {
						if (item.id && item.id !== data.spaceSlug) goto(`/wiki/${encodeURIComponent(item.id)}`);
					}}
				/>
			</div>
		{:else}
			<label class="mb-1 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
				<Globe size={13} /> {t('wiki.destination_label', 'Destination')}
			</label>
			{#if destLoading}
				<div class="w-full max-w-sm rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-muted)]">
					{t('wiki.destination_loading', 'Loading destinations…')}
				</div>
			{:else}
				<div class="w-full max-w-sm">
					<Autocomplete
						placeholder={t('wiki.destination_label', 'Destination')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchDestinations}
						initialFetcher={fetchDestinationsInitial}
						selectedId={data.spaceSlug || null}
						selectedLabel={selectedDestLabel}
						onSelect={(item) => {
							if (item.id && item.id !== data.spaceSlug) goto(`/wiki/${encodeURIComponent(item.id)}`);
						}}
					/>
				</div>
			{/if}
		{/if}
	</div>

	<div class="p-3">
		{#if !wikiEnabled && data.loadError}
			<EmptyState
				icon={BookOpen}
				title={t('wiki.coming_soon', 'Wiki is not enabled for this destination')}
				body={t('wiki.coming_soon_body', 'Wiki is currently disabled. Please check back later.')}
				action={{ label: t('wiki.back_to_index', 'Back'), href: '/wiki' }}
			/>
		{:else if data.loadError}
			<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-4 py-3 text-sm text-[var(--color-text-subtle)]">
				{t('wiki.load_error', 'Wiki could not be loaded.')}
			</div>
		{:else if data.index.length === 0}
			<EmptyState
				icon={BookOpen}
				title={t('wiki.empty_title', 'No wiki pages yet')}
				body={t('wiki.empty_body', "This space doesn't have any wiki pages yet.")}
				action={canCreate ? { label: t('wiki.create_first_page', 'Create the first page'), href: `/wiki/${encodeURIComponent(data.spaceSlug)}/create` } : undefined}
			/>
		{:else}
			{#if featured.length > 0}
				<section class="mb-6">
					<h2 class="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
						<Star size={13} /> {t('wiki.featured', 'Featured')}
					</h2>
					<div class="grid gap-2 sm:grid-cols-2">
						{#each featured as page (page.id)}
							<a
								href="/wiki/{data.spaceSlug}/{page.slug}"
								aria-label={page.title}
								class="flex flex-col gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 transition-colors hover:border-[var(--color-primary)]"
							>
								<span class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
									{page.title}
									{#if structuredEditsOn && page.data_entity_type}
										<span
											class="inline-flex items-center gap-1 rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] font-medium uppercase text-[var(--color-muted)]"
											title={t('wiki.data_badge_hint', 'Includes community-editable data')}
										>
											<Table2 size={10} /> {t('wiki.data_badge', 'Data')}
										</span>
									{/if}
								</span>
								<span class="text-xs text-[var(--color-muted)]" aria-hidden="true">{categoryLabel(page.category as WikiCategory)}</span>
							</a>
						{/each}
					</div>
				</section>
			{/if}

			{#each grouped as group (group.cat)}
				<section class="mb-6">
					<h2 class="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
						{categoryLabel(group.cat)}
					</h2>
					<ul class="divide-y divide-[var(--color-border)] rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]">
						{#each group.pages as page (page.id)}
							<li>
								<a
									href="/wiki/{data.spaceSlug}/{page.slug}"
									aria-label={page.title}
									class="flex items-center justify-between gap-3 px-4 py-2.5 text-sm transition-colors hover:bg-[var(--color-surface-subtle)]"
								>
									<span class="flex items-center gap-2 text-[var(--color-text)]">
										{page.title}
										{#if page.is_locked}
											<Lock size={13} class="text-[var(--color-muted)]" />
										{/if}
										{#if structuredEditsOn && page.data_entity_type}
											<span
												class="inline-flex items-center gap-1 rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] font-medium uppercase text-[var(--color-muted)]"
												title={t('wiki.data_badge_hint', 'Includes community-editable data')}
											>
												<Table2 size={10} /> {t('wiki.data_badge', 'Data')}
											</span>
										{/if}
									</span>
									<span class="text-xs text-[var(--color-muted)]" aria-hidden="true">{categoryLabel(page.category as WikiCategory)}</span>
								</a>
							</li>
						{/each}
					</ul>
				</section>
			{/each}
		{/if}

		{#if structuredEditsOn && spaceId && !data.loadError}
			<EntityFieldsPanel
				entityType="space"
				entityId={spaceId}
				title={t('wiki.space_data_title', '{name} data', { name: spaceName })}
			/>
		{/if}
	</div>
</div>
