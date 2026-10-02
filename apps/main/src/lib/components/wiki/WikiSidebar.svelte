<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md
	// Route-level right-sidebar bundle for wiki pages (registered via
	// SetSidebar — renders above the global widget rail, ADR-058):
	//   Quick Facts → Need Help? → Related Articles → Popular Cities.
	import { ArrowRight, FileText, MapPin } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import WikiQuickFacts from './WikiQuickFacts.svelte';
	import type { WikiDestination, WikiPageIndex } from '$lib/types';

	let {
		spaceSlug,
		spaceName,
		spaceId = '',
		spaceLevel = '',
		pages = [],
		currentPageSlug = '',
		quickFactsGroup = '',
		toc = []
	}: {
		spaceSlug: string;
		spaceName: string;
		spaceId?: string;
		spaceLevel?: string;
		pages?: WikiPageIndex[];
		/** Exclude this page from the related list (detail view). */
		currentPageSlug?: string;
		/** §80 Wiki Topic Pages — the page's bound group slug; scopes Quick
		 *  Facts to its `quick_facts` section when declared. */
		quickFactsGroup?: string;
		/** §80 Wiki Topic Pages — ON THIS PAGE anchors: the page's
		 *  fields.sections[] plus rollup/directory panels. Rendered when
		 *  the page has ≥2 sections. */
		toc?: Array<{ id: string; label: string }>;
	} = $props();

	// Related articles: featured pages first, then the rest in index order.
	const eligible = $derived(pages.filter((p) => p.slug !== currentPageSlug));
	const related = $derived(
		[...eligible]
			.sort((a, b) => Number(b.is_featured) - Number(a.is_featured))
			.slice(0, 5)
	);
	const hasMore = $derived(eligible.length > related.length);

	// Popular places — child destinations that already have wiki pages (the
	// endpoint defaults has_wiki=true), rendered for geo spaces only. The
	// ?within= filter resolves the whole subtree in ONE call — cities of a
	// state, or cities of every state under a country — so there's no
	// per-region fan-out. When no city wikis exist we fall back to the
	// space's direct children (states/regions with pages).
	const isGeoSpace = $derived(spaceLevel === 'country' || spaceLevel === 'state');
	let cities = $state<WikiDestination[]>([]);

	// "Popular" = most wiki content; ties break alphabetically for stability.
	function sortPlaces(list: WikiDestination[]): WikiDestination[] {
		return [...list].sort(
			(a, b) => b.page_count - a.page_count || a.name.localeCompare(b.name)
		);
	}

	$effect(() => {
		if (!isGeoSpace) return;
		let cancelled = false;
		const slug = spaceSlug;

		const fetchDestinations = async (
			params: Record<string, string>
		): Promise<WikiDestination[]> => {
			const res = await api.get<WikiDestination[]>(
				`/wiki/destinations?${new URLSearchParams(params).toString()}`
			);
			return res.data ?? [];
		};

		(async () => {
			try {
				let places = await fetchDestinations({ within: slug, level: 'city' });
				if (places.length === 0) places = await fetchDestinations({ parent: slug });
				if (!cancelled) cities = sortPlaces(places);
			} catch {
				if (!cancelled) cities = [];
			}
		})();

		return () => {
			cancelled = true;
		};
	});

	const placesLabel = $derived(
		cities[0]?.level === 'city'
			? t('wiki.popular_cities', 'Popular Cities')
			: t('wiki.popular_places', 'Popular Places')
	);
	const seeAllLabel = $derived(
		cities[0]?.level === 'city'
			? t('wiki.see_all_cities', 'See all cities')
			: t('wiki.see_all', 'See all')
	);
</script>

{#if toc.length > 0}
	<!-- §80 Wiki Topic Pages — ON THIS PAGE anchor list: the page's
	     field-table sections plus rollup/directory panels. -->
	<nav
		class="rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
		aria-label={t('wiki.on_this_page', 'On this page')}
	>
		<h2 class="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-muted)]">
			{t('wiki.on_this_page', 'On this page')}
		</h2>
		<ul class="space-y-1.5">
			{#each toc as item (item.id)}
				<li>
					<a
						href="#{item.id}"
						class="block truncate text-sm text-[var(--color-text)] transition-colors hover:text-[var(--color-primary)]"
					>
						{item.label}
					</a>
				</li>
			{/each}
		</ul>
	</nav>
{/if}

{#if spaceId}
	<WikiQuickFacts entityType="space" entityId={spaceId} groupSlug={quickFactsGroup} />
{/if}

<!-- Need Help? — routes readers to the destination's community space. -->
<section class="rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
	<h2 class="mb-1.5 text-sm font-semibold text-[var(--color-text)]">
		{t('wiki.need_help', 'Need Help?')}
	</h2>
	<p class="mb-3 text-xs leading-relaxed text-[var(--color-muted)]">
		{t('wiki.need_help_body', 'Ask questions, get advice, and connect with people already in {name}.', { name: spaceName })}
	</p>
	<a
		href="/spaces/{encodeURIComponent(spaceSlug)}"
		class="inline-flex w-full items-center justify-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-success)] px-3 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
	>
		{t('wiki.join_community', 'Join Community')}
		<ArrowRight size={14} aria-hidden="true" />
	</a>
</section>

{#if related.length > 0}
	<section class="rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">
			{t('wiki.related_articles', 'Related Articles')}
		</h2>
		<ul class="divide-y divide-[var(--color-border)]">
			{#each related as p (p.id)}
				<li>
					<a
						href="/wiki/{encodeURIComponent(spaceSlug)}/{encodeURIComponent(p.slug)}"
						class="flex items-center gap-2 py-2 text-sm text-[var(--color-text)] transition-colors hover:text-[var(--color-primary)]"
					>
						<FileText size={14} class="shrink-0 text-[var(--color-muted)]" aria-hidden="true" />
						<span class="min-w-0 truncate">{p.title}</span>
					</a>
				</li>
			{/each}
		</ul>
		{#if hasMore}
			<a
				href="/wiki/{encodeURIComponent(spaceSlug)}"
				class="mt-2 inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary)] hover:underline"
			>
				{t('wiki.view_all_articles', 'View all articles')}
				<ArrowRight size={14} aria-hidden="true" />
			</a>
		{/if}
	</section>
{/if}

{#if cities.length > 0}
	<section class="rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-2 text-sm font-semibold text-[var(--color-text)]">
			{placesLabel}
		</h2>
		<ul class="divide-y divide-[var(--color-border)]">
			{#each cities.slice(0, 5) as city (city.slug)}
				<li>
					<a
						href="/wiki/{encodeURIComponent(city.slug)}"
						class="flex items-center gap-2.5 py-2 transition-colors"
					>
						<span
							class="grid h-8 w-8 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)]"
						>
							<MapPin size={14} class="text-[var(--color-muted)]" aria-hidden="true" />
						</span>
						<span class="min-w-0">
							<span class="block truncate text-sm font-medium text-[var(--color-text)]">
								{city.name}
							</span>
							{#if city.geo_code}
								<span class="block truncate text-xs text-[var(--color-muted)]">{city.geo_code}</span>
							{/if}
						</span>
					</a>
				</li>
			{/each}
		</ul>
		{#if cities.length > 5}
			<a
				href="/wiki"
				class="mt-2 inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary)] hover:underline"
			>
				{seeAllLabel}
				<ArrowRight size={14} aria-hidden="true" />
			</a>
		{/if}
	</section>
{/if}
