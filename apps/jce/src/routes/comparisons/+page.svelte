<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { Search, Eye, MessageCircle, Share2, Plus, ChevronLeft, ChevronRight } from 'lucide-svelte';
	import { i18n, t } from '$lib/i18n';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let items = $derived(data.items ?? []);
	let meta = $derived(data.meta ?? { total: 0, page: 1, limit: 12 });
	let types = $derived(data.types ?? []);
	let filters = $derived(data.filters);
	let loadError = $derived(data.loadError ?? '');
	let loadErrorParams = $derived(data.loadErrorParams ?? {});
	let user = $derived(data.user);

	let query = $state('');
	let queryInitialized = $state(false);

	$effect.pre(() => {
		if (!queryInitialized) {
			query = filters.q ?? '';
			queryInitialized = true;
		}
	});

	// Ref: sections/30-generic-comparison-system.md — sort/page-size options come from the API list meta.
	let sortValues = $derived(data.meta?.sort_options ?? []);
	let sortOptions = $derived(
		sortValues.map((value) => ({ value, label: t(`jce.comparisons.sort.${value}`) }))
	);

	let pageSizeOptions = $derived(data.meta?.page_size_options ?? []);

	function buildURL(updates: Record<string, string | number | null>) {
		const url = new URL($page.url);
		for (const [key, value] of Object.entries(updates)) {
			if (value === '' || value === null || value === undefined) {
				url.searchParams.delete(key);
			} else {
				url.searchParams.set(key, String(value));
			}
		}
		return url;
	}

	function apply() {
		const url = buildURL({ q: query, page: 1 });
		goto(url, { replaceState: true, keepFocus: true });
	}

	function setSort(sort: string) {
		goto(buildURL({ sort, page: 1 }), { replaceState: true });
	}

	function setPageSize(size: number) {
		goto(buildURL({ page_size: size, page: 1 }), { replaceState: true });
	}

	function setPage(pageNum: number) {
		goto(buildURL({ page: pageNum }), { replaceState: true });
	}

	function setFilterType(slug: string) {
		goto(buildURL({ filter_type: slug || null, page: 1 }), { replaceState: true });
	}

	function setTab(mine: boolean) {
		const updates: Record<string, string | number | null> = {
			mine: mine ? 'true' : null,
			filter_depth: null,
			filter_status: null,
			filter_visibility: null,
			page: 1
		};
		goto(buildURL(updates), { replaceState: true });
	}

	function excerpt(text?: string | null, max = 140) {
		if (!text) return '';
		// Strip simple HTML tags and truncate.
		const plain = text.replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();
		if (plain.length <= max) return plain;
		return plain.slice(0, max).replace(/\s+\S*$/, '') + '…';
	}

	function formatDate(iso?: string) {
		if (!iso) return '';
		return new Date(iso).toLocaleDateString(i18n.locale, { year: 'numeric', month: 'short', day: 'numeric' });
	}

	const totalPages = $derived(Math.max(1, Math.ceil((meta.total || 0) / (meta.limit || 1))));
	const currentPage = $derived(meta.page || 1);
	const isMine = $derived(filters.mine);

	let searchTimeout: ReturnType<typeof setTimeout> | null = null;
	function onSearchInput() {
		if (searchTimeout) clearTimeout(searchTimeout);
		searchTimeout = setTimeout(() => apply(), 300);
	}
</script>

<div class="max-w-6xl mx-auto p-4 sm:p-6 space-y-6">
	<div class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
		<h1 class="text-2xl sm:text-3xl font-bold" style="color: var(--color-text)">{t('jce.comparisons.heading')}</h1>
		<a
			href="/compare"
			class="inline-flex items-center gap-2 self-start px-4 py-2 rounded-[var(--radius-button)] text-sm font-medium transition-colors"
			style="background-color: var(--color-surface); border: 1px solid var(--color-border); color: var(--color-text)"
		>
			<Plus size={16} /> {t('jce.comparisons.create')}
		</a>
	</div>

	{#if loadError}
		<div class="rounded-[var(--radius-button)] p-4 text-sm" role="alert" style="background-color: rgba(239,68,68,0.1); color: #ef4444; border: 1px solid rgba(239,68,68,0.2)">
			{t(loadError, undefined, loadErrorParams)}
		</div>
	{/if}

	{#if user}
		<div class="flex gap-2 border-b" style="border-color: var(--color-border)">
			<button
				type="button"
				class="px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-[1px]"
				class:opacity-60={isMine}
				style="border-color: {isMine ? 'transparent' : 'var(--color-text)'}; color: var(--color-text)"
				onclick={() => setTab(false)}
			>
				{t('jce.comparisons.tab_public')}
			</button>
			<button
				type="button"
				class="px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-[1px]"
				class:opacity-60={!isMine}
				style="border-color: {!isMine ? 'transparent' : 'var(--color-text)'}; color: var(--color-text)"
				onclick={() => setTab(true)}
			>
				{t('jce.comparisons.tab_mine')}
			</button>
		</div>
	{/if}

	<div class="grid gap-3 sm:grid-cols-[1fr_auto_auto] items-end">
		<div class="relative">
			<Search size={16} class="absolute left-3 top-1/2 -translate-y-1/2" style="color: var(--color-muted)" />
			<input
				type="text"
				bind:value={query}
				oninput={onSearchInput}
				placeholder={t('jce.comparisons.search_placeholder')}
				class="w-full rounded-[var(--radius-button)] border bg-[var(--color-surface)] pl-9 pr-4 py-2 text-sm outline-none focus:border-[var(--color-text)]"
				style="border-color: var(--color-border); color: var(--color-text)"
			/>
		</div>

		<select
			value={filters.sort || sortValues[0] || 'created_at'}
			onchange={(e) => setSort(e.currentTarget.value)}
			class="rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm outline-none focus:border-[var(--color-text)]"
			style="border-color: var(--color-border); color: var(--color-text)"
		>
			{#each sortOptions as opt}
				<option value={opt.value}>{opt.label}</option>
			{/each}
		</select>

		<select
			value={filters.filter_type || ''}
			onchange={(e) => setFilterType(e.currentTarget.value)}
			class="rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-3 py-2 text-sm outline-none focus:border-[var(--color-text)]"
			style="border-color: var(--color-border); color: var(--color-text)"
		>
			<option value="">{t('jce.comparisons.all_types')}</option>
			{#each types as type}
				<option value={type.slug}>{type.name}</option>
			{/each}
		</select>
	</div>

	{#if items.length === 0}
		<div class="rounded-[var(--radius-card)] border p-8 text-center" style="background-color: var(--color-surface); border-color: var(--color-border)">
			<p style="color: var(--color-muted)">
				{isMine ? t('jce.comparisons.empty_mine') : t('jce.comparisons.empty_public')}
			</p>
			{#if isMine}
				<a href="/create?type=comparison" class="mt-4 inline-block text-sm underline" style="color: var(--color-text)">{t('jce.comparisons.create_detailed')}</a>
			{:else}
				<a href="/compare" class="mt-4 inline-block text-sm underline" style="color: var(--color-text)">{t('jce.comparisons.create_one')}</a>
			{/if}
		</div>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each items as item (item.id)}
				<article
					class="group rounded-[var(--radius-card)] border p-4 transition-colors hover:opacity-95"
					style="background-color: var(--color-surface); border-color: var(--color-border)"
				>
					<a href="/comparisons/{item.slug}" class="block space-y-3">
						<div class="flex items-start justify-between gap-2">
							<h2 class="font-semibold leading-snug" style="color: var(--color-text)">{item.title}</h2>
						</div>
						{#if item.excerpt}
							<p class="text-sm line-clamp-2" style="color: var(--color-muted)">{item.excerpt}</p>
						{:else}
							<p class="text-sm line-clamp-2" style="color: var(--color-muted)">{excerpt(item.body)}</p>
						{/if}

						<div class="flex flex-wrap items-center gap-2 text-xs" style="color: var(--color-muted)">
							{#if item.type_name}
								<span class="rounded px-2 py-0.5" style="background-color: var(--color-bg)">{item.type_name}</span>
							{/if}
							<span class="rounded px-2 py-0.5" style="background-color: var(--color-bg)">{item.depth}</span>
							{#if isMine}
								<span class="rounded px-2 py-0.5" style="background-color: var(--color-bg)">{item.status}</span>
								<span class="rounded px-2 py-0.5" style="background-color: var(--color-bg)">{item.visibility}</span>
							{/if}
						</div>

						<div class="flex items-center gap-4 text-xs" style="color: var(--color-muted)">
							<span>{formatDate(item.created_at)}</span>
							<span class="inline-flex items-center gap-1"><Eye size={12} /> {item.view_count ?? 0}</span>
							<span class="inline-flex items-center gap-1"><MessageCircle size={12} /> {item.comment_count ?? 0}</span>
							<span class="inline-flex items-center gap-1"><Share2 size={12} /> {item.share_count ?? 0}</span>
						</div>
					</a>
				</article>
			{/each}
		</div>

		<div class="flex flex-col sm:flex-row items-center justify-between gap-4 pt-4 border-t" style="border-color: var(--color-border)">
			<div class="flex items-center gap-2 text-sm" style="color: var(--color-muted)">
				<span>{t('jce.common.page_of', 'Page {current} of {total}').replace('{current}', String(currentPage)).replace('{total}', String(totalPages))}</span>
				<span>·</span>
				<select
					value={filters.page_size || meta.limit || pageSizeOptions[0] || 12}
					onchange={(e) => setPageSize(parseInt(e.currentTarget.value, 10))}
					class="rounded-[var(--radius-button)] border bg-[var(--color-surface)] px-2 py-1 text-sm outline-none"
					style="border-color: var(--color-border); color: var(--color-text)"
				>
					{#each pageSizeOptions as size}
						<option value={size}>{size} {t('jce.common.per_page')}</option>
					{/each}
				</select>
			</div>

			<div class="flex items-center gap-2">
				<button
					type="button"
					disabled={currentPage <= 1}
					onclick={() => setPage(currentPage - 1)}
					class="inline-flex items-center gap-1 px-3 py-1.5 rounded-[var(--radius-button)] border text-sm disabled:opacity-40"
					style="border-color: var(--color-border); color: var(--color-text); background-color: var(--color-surface)"
				>
					<ChevronLeft size={14} /> {t('jce.common.previous')}
				</button>
				<button
					type="button"
					disabled={currentPage >= totalPages}
					onclick={() => setPage(currentPage + 1)}
					class="inline-flex items-center gap-1 px-3 py-1.5 rounded-[var(--radius-button)] border text-sm disabled:opacity-40"
					style="border-color: var(--color-border); color: var(--color-text); background-color: var(--color-surface)"
				>
					{t('jce.common.next')} <ChevronRight size={14} />
				</button>
			</div>
		</div>
	{/if}
</div>
