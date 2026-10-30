<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md (child-type
	// directories) — card grid + scoped search on a directory-mode category
	// page (e.g. /wiki/nigeria/states). Children link to their own wiki at
	// /wiki/{slug}; search calls GET /wiki/destinations?within={slug} so the
	// full set stays reachable beyond the embedded cap.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import type { WikiDestination, WikiDirectory, WikiDirectoryChild } from '$lib/types';
	import { entityPageHref } from '$lib/utils/wikiEntityLinks';
	import { MapPin } from 'lucide-svelte';

	let {
		directory,
		spaceSlug
	}: {
		directory: WikiDirectory;
		spaceSlug: string;
	} = $props();

	// Distinct child types in the listing — a multi-level directory (e.g.
	// states + universities) gets a type filter; single-type directories
	// render flat. directory.levels covers the full (uncapped) listing —
	// children alone would hide a type whose entries all sit past the cap.
	const childTypes = $derived.by(() => {
		if (directory.levels?.length) return directory.levels;
		const seen = new Set<string>();
		for (const c of directory.children) seen.add(c.type_slug || c.level);
		return [...seen];
	});

	let selectedType = $state<string | null>(null); // null = all types

	function typeLabel(key: string): string {
		return t(`wiki.type_${key}`, key.replace(/_/g, ' ').replace(/\b\w/g, (ch) => ch.toUpperCase()));
	}

	// Grouped view for 'all': one section per type, children already
	// depth/name-ordered by the backend.
	const groups = $derived.by(() => {
		const out = new Map<string, WikiDirectoryChild[]>();
		const list = selectedType
			? directory.children.filter((c) => (c.type_slug || c.level) === selectedType)
			: directory.children;
		for (const c of list) {
			const key = c.type_slug || c.level;
			const arr = out.get(key) ?? [];
			arr.push(c);
			out.set(key, arr);
		}
		return out;
	});

	// Entity-bound children (e.g. a university's course offerings) — the
	// space hierarchy can't see them, so they ride as their own groups
	// keyed by child type slug.
	const entityGroups = $derived(
		Object.entries(directory.entity_children ?? {}).filter(
			([typeSlug, items]) =>
				items.length > 0 && (selectedType === null || selectedType === typeSlug)
		)
	);

	async function searchChildren(q: string): Promise<AutocompleteItem[]> {
		const query = q.trim();
		if (!query) return [];
		try {
			const params = new URLSearchParams({
				within: spaceSlug,
				has_wiki: 'false',
				q: query
			});
			if (selectedType) params.set('level', selectedType);
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

	function onPick(item: AutocompleteItem) {
		if (item.id) goto(`/wiki/${encodeURIComponent(item.id)}`);
	}

	function pageCountLabel(count: number): string {
		return t(
			count === 1 ? 'wiki.destination_page_count_one' : 'wiki.destination_page_count',
			count === 1 ? '{count} guide' : '{count} guides'
		).replace('{count}', String(count));
	}
</script>

<!-- §80 Reader UX — fixed anchor for deep links. -->
<section id="wiki-directory" class="mt-6 scroll-mt-24 space-y-4">
	<!-- The scoped search earns its slot once the listing outgrows one
	     screenful — with ≤12 children every item is already visible. -->
	{#if directory.total > 12}
		<div class="w-full max-w-sm">
			<Autocomplete
				id="wiki-directory-search"
				placeholder={t('wiki.directory_search_hint', 'Search this list…')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={searchChildren}
				selectedId={null}
				selectedLabel={null}
				onSelect={onPick}
			/>
		</div>
	{/if}

	{#if childTypes.length > 1}
		<div class="flex flex-wrap gap-2">
			<button
				type="button"
				class="rounded-full border px-3 py-1 text-xs transition-colors {selectedType === null
					? 'border-[var(--color-primary)] bg-[var(--color-primary)] text-white'
					: 'border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)] hover:border-[var(--color-primary)]'}"
				onclick={() => (selectedType = null)}
			>
				{t('common.all', 'All')}
			</button>
			{#each childTypes as key (key)}
				<button
					type="button"
					class="rounded-full border px-3 py-1 text-xs capitalize transition-colors {selectedType === key
						? 'border-[var(--color-primary)] bg-[var(--color-primary)] text-white'
						: 'border-[var(--color-border)] bg-[var(--color-surface)] text-[var(--color-text)] hover:border-[var(--color-primary)]'}"
					onclick={() => (selectedType = key)}
				>
					{typeLabel(key)}
				</button>
			{/each}
		</div>
	{/if}

	{#each [...groups] as [typeKey, children] (typeKey)}
			{#if groups.size > 1}
				<h2 class="text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
					{typeLabel(typeKey)}
				</h2>
			{/if}
			<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each children as child (child.slug)}
					<!-- §80 rich cards — an image_url child renders as an
					     image-first card; others keep the text card. -->
					<a
						href="/wiki/{encodeURIComponent(child.slug)}"
						class="flex flex-col overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] transition-colors hover:border-[var(--color-primary)] hover:bg-[var(--color-surface-subtle)]"
						data-sveltekit-preload-data="hover"
					>
						{#if child.image_url}
							<img
								src={child.image_url}
								alt={child.name}
								loading="lazy"
								class="aspect-[16/9] w-full object-cover"
							/>
						{/if}
						<span class="flex flex-col gap-1 px-4 py-3">
							<span class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
								{child.name}
								{#if child.geo_code}
									<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] uppercase text-[var(--color-muted)]">{child.geo_code}</span>
								{/if}
								{#if child.kind}
									<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] capitalize text-[var(--color-muted)]">{child.kind.replace(/-/g, ' ')}</span>
								{/if}
							</span>
							{#if child.blurb}
								<span class="line-clamp-2 text-xs text-[var(--color-muted)]">{child.blurb}</span>
							{/if}
							<span class="flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
								<MapPin size={11} />
								{pageCountLabel(child.page_count)}
							</span>
						</span>
					</a>
				{/each}
			</div>
		{/each}
	{#each entityGroups as [typeSlug, items] (typeSlug)}
		{#if groups.size > 0 || entityGroups.length > 1}
			<h2 class="text-sm font-semibold uppercase tracking-wide text-[var(--color-muted)]">
				{typeLabel(typeSlug)}
			</h2>
		{/if}
		<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{#each items as child (child.id)}
				<!-- Entity cards carry a primary + secondary link, so the
				     card is a div (nested <a> is invalid). -->
				<div class="flex flex-col gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 transition-colors hover:border-[var(--color-primary)]">
					<a
						href={entityPageHref(typeSlug, child.slug)}
						class="text-sm font-medium text-[var(--color-text)] hover:text-[var(--color-primary)]"
						data-sveltekit-preload-data="hover"
					>
						{child.name}
					</a>
					{#if child.program_slug}
						<a
							href={entityPageHref('program', child.program_slug)}
							class="text-xs text-[var(--color-muted)] hover:text-[var(--color-primary)]"
						>
							{t('wiki.view_canonical_program', 'Canonical program')}
						</a>
					{/if}
				</div>
			{/each}
		</div>
	{/each}
	{#if directory.total > directory.children.length}
		<p class="text-xs text-[var(--color-muted)]">
			{t('wiki.directory_truncated', 'Showing {shown} of {total} — search to find the rest.', {
				shown: directory.children.length,
				total: directory.total
			})}
		</p>
	{/if}
</section>
