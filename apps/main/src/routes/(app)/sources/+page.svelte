<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Rss, Globe, Users, FileText } from 'lucide-svelte';
	import { DirectoryCard, UrlTabs, type UrlTab, type CardMetaItem } from '$lib/components/ui';
	import type { PageData } from './$types';
	import type { Source, SourceCollection } from '$lib/types/sources';

	let { data }: { data: PageData } = $props();

	const tabs = $derived<UrlTab[]>([
		{ value: 'popular', label: t('sources.sort.popular', 'Popular') },
		{ value: 'recent', label: t('sources.sort.recent', 'Recently added') },
		{ value: 'trending', label: t('sources.sort.trending', 'Trending') }
	]);

	const title = $derived(t('sources.title', 'Sources — JapaConnect'));
	const description = $derived(t('sources.description', 'Browse trusted sources for scholarships, jobs, remittance, universities, and more.'));

	function searchUrl(q: string) {
		const url = new URL(page.url);
		if (q) {
			url.searchParams.set('q', q);
		} else {
			url.searchParams.delete('q');
		}
		url.searchParams.delete('page');
		return url;
	}

	function changeSort(sort: string) {
		const url = new URL(page.url);
		url.searchParams.set('sort', sort);
		url.searchParams.delete('page');
		goto(url, { keepFocus: true });
	}

	function sourceIcon(source: Source) {
		if (source.source_type === 'youtube') return undefined;
		return Rss;
	}

	function sourceMeta(source: Source): CardMetaItem[] {
		return [
			{ icon: Users, text: `${source.follower_count} ${t('sources.followers', 'followers')}` },
			{ icon: FileText, text: `${source.post_count} ${t('sources.posts', 'posts')}` }
		];
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
</svelte:head>

<div class="flex flex-col gap-4 p-4">
	<div class="flex flex-col gap-2">
		<h1 class="text-xl font-bold">{t('sources.heading', 'Sources')}</h1>
		<p class="text-sm text-[var(--color-text-muted)]">{description}</p>
	</div>

	{#if data.collections && data.collections.length > 0}
		<div class="flex flex-col gap-6">
			{#each data.collections as collection (collection.slug)}
				<div class="flex flex-col gap-2">
					<div class="flex items-baseline justify-between">
						<h2 class="text-lg font-semibold">{collection.name}</h2>
					</div>
					{#if collection.description}
						<p class="text-sm text-[var(--color-text-muted)]">{collection.description}</p>
					{/if}
					<div class="flex gap-3 overflow-x-auto pb-2">
						{#each collection.sources as source (source.slug)}
							<div class="min-w-[260px]">
								<DirectoryCard
									href="/sources/{source.slug}"
									title={source.name}
									subtitle={source.domain}
									description={source.description}
									imageUrl={source.logo_url}
									fallback={source.name.charAt(0).toUpperCase()}
									icon={sourceIcon(source)}
									meta={sourceMeta(source)}
									hueSeed={source.slug}
								/>
							</div>
						{/each}
					</div>
				</div>
			{/each}
		</div>
	{/if}

	<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
		<form
			method="GET"
			action="/sources"
			class="flex gap-2"
			onsubmit={(e) => {
				e.preventDefault();
				const fd = new FormData(e.currentTarget);
				goto(searchUrl(String(fd.get('q') ?? '')));
			}}
		>
			<input
				name="q"
				type="search"
				value={data.q}
				placeholder={t('sources.search.placeholder', 'Search sources…')}
				class="rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm"
			/>
			<button
				type="submit"
				class="rounded-md bg-[var(--color-primary)] px-3 py-2 text-sm font-medium text-white"
			>
				{t('sources.search.label', 'Search')}
			</button>
		</form>

		<UrlTabs {tabs} value={data.sort} onValueChange={changeSort} />
	</div>

	{#if data.sources.length === 0}
		<p class="py-8 text-center text-sm text-[var(--color-text-muted)]">
			{t('sources.empty', 'No sources found.')}
		</p>
	{:else}
		<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{#each data.sources as source (source.slug)}
				<DirectoryCard
					href="/sources/{source.slug}"
					title={source.name}
					subtitle={source.domain}
					description={source.description}
					imageUrl={source.logo_url}
					fallback={source.name.charAt(0).toUpperCase()}
					icon={sourceIcon(source)}
					meta={sourceMeta(source)}
					hueSeed={source.slug}
				/>
			{/each}
		</div>
	{/if}

	{#if data.meta && data.meta.total}
		{@const total = data.meta.total}
		{@const pageSize = data.pageSize}
		{@const lastPage = Math.max(1, Math.ceil(total / pageSize))}
		<div class="flex items-center justify-between pt-4 text-sm">
			{#if data.page > 1}
				<a
					href="/sources?sort={data.sort}&page={data.page - 1}"
					class="rounded-md border border-[var(--color-border)] px-3 py-2"
				>
					{t('pagination.previous', 'Previous')}
				</a>
			{:else}
				<span></span>
			{/if}
			<span class="text-[var(--color-text-muted)]">
				{t('pagination.page', 'Page {page} of {total}')
					.replace('{page}', String(data.page))
					.replace('{total}', String(lastPage))}
			</span>
			{#if data.page < lastPage}
				<a
					href="/sources?sort={data.sort}&page={data.page + 1}"
					class="rounded-md border border-[var(--color-border)] px-3 py-2"
				>
					{t('pagination.next', 'Next')}
				</a>
			{:else}
				<span></span>
			{/if}
		</div>
	{/if}
</div>
