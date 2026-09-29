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
	// render flat.
	const childTypes = $derived.by(() => {
		const seen = new Map<string, string>();
		for (const c of directory.children) {
			const key = c.type_slug || c.level;
			if (!seen.has(key)) seen.set(key, key);
		}
		return [...seen.keys()];
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

<section class="mt-6 space-y-4">
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
			<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each children as child (child.slug)}
					<a
						href="/wiki/{encodeURIComponent(child.slug)}"
						class="flex flex-col gap-1 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3 transition-colors hover:border-[var(--color-primary)] hover:bg-[var(--color-surface-subtle)]"
						data-sveltekit-preload-data="hover"
					>
						<span class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
							{child.name}
							{#if child.geo_code}
								<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] uppercase text-[var(--color-muted)]">{child.geo_code}</span>
							{/if}
						</span>
						<span class="flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
							<MapPin size={11} />
							{pageCountLabel(child.page_count)}
						</span>
					</a>
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
