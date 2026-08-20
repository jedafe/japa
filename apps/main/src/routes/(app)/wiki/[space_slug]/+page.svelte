<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md
	// Geo-aware pattern: destination selector is the first UI input.
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { BookOpen, Lock, Star, ArrowLeft, Globe } from 'lucide-svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import type { WikiCategory, WikiPageIndex, GeoCountry } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// BUG-018: fetch the canonical category list from the API (Rule 16)
	// instead of hardcoding it. Falls back to the page-derived categories
	// if the API call fails.
	type ApiCategory = { slug: string; label_key: string; label: string };
	let apiCategories = $state<ApiCategory[]>([]);

	$effect(() => {
		(async () => {
			try {
				const res = await api.get<{ data?: ApiCategory[] }>(`/wiki/categories`);
				apiCategories = res.data?.data ?? [];
			} catch {
				/* keep empty — fallback below */
			}
		})();
	});

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

	// Resolve the space name from the first page's space_id-adjacent data, or
	// fall back to the slug. The index endpoint may include a space name; we
	// degrade gracefully to the slug when absent.
	let spaceName = $state<string>(untrack(() => data.spaceSlug));

	// Geo-aware destination selector: load the canonical country list from
	// /geo/countries (Rule 16 — no hardcoded lists, ADR-005). The selector is
	// the first input per the geo-aware tool pattern.
	let destOptions = $state<GeoCountry[]>([]);
	let destLoading = $state(false);

	$effect(() => {
		// Best-effort: fetch a lightweight space name. The index payload does not
		// carry the space name, so we resolve it client-side from the spaces API.
		(async () => {
			try {
				const res = await api.get<{ name?: string; space?: { name?: string } }>(
					`/spaces/${encodeURIComponent(data.spaceSlug)}`
				);
				const name = res.data?.name ?? res.data?.space?.name;
				if (name) spaceName = name;
			} catch {
				/* keep slug fallback */
			}
		})();

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
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="flex items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<a
			href="/spaces/{data.spaceSlug}"
			class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={14} /> {t('wiki.back_to_index', 'Back')}
		</a>
		<h1 class="text-lg font-semibold text-[var(--color-text)]">
			{spaceName} — {t('wiki.heading', 'Wiki')}
		</h1>
	</div>

	<!-- Geo-aware destination selector (first input per geo-aware tool pattern) -->
	<div class="border-b border-[var(--color-border)] px-4 py-3">
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
	</div>

	<div class="p-3">
		{#if data.loadError}
			<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-4 py-3 text-sm text-[var(--color-text-subtle)]">
				{t('wiki.load_error', 'Wiki could not be loaded.')}
			</div>
		{:else if data.index.length === 0}
			<EmptyState
				icon={BookOpen}
				title={t('wiki.empty_title', 'No wiki pages yet')}
				body={t('wiki.empty_body', "This space doesn't have any wiki pages yet.")}
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
								class="flex flex-col gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 transition-colors hover:border-[var(--color-primary)]"
							>
								<span class="text-sm font-medium text-[var(--color-text)]">{page.title}</span>
								<span class="text-xs text-[var(--color-muted)]">{categoryLabel(page.category as WikiCategory)}</span>
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
									class="flex items-center justify-between gap-3 px-4 py-2.5 text-sm transition-colors hover:bg-[var(--color-surface-subtle)]"
								>
									<span class="flex items-center gap-2 text-[var(--color-text)]">
										{page.title}
										{#if page.is_locked}
											<Lock size={13} class="text-[var(--color-muted)]" />
										{/if}
									</span>
									<span class="text-xs text-[var(--color-muted)]">{categoryLabel(page.category as WikiCategory)}</span>
								</a>
							</li>
						{/each}
					</ul>
				</section>
			{/each}
		{/if}
	</div>
</div>
