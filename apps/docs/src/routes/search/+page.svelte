<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { t } from '$lib/stores/i18n.svelte';
	import { Search, FileText, ArrowRight } from 'lucide-svelte';

	const { data } = $props<{
		data: {
			query: string;
			results: Array<{ id: string; title: string; slug: string; audience: string; excerpt: string }>;
		};
	}>();

	// svelte-ignore state_referenced_locally
	let query = $state(data.query);
	let debounceTimer: ReturnType<typeof setTimeout>;

	function onInput(e: Event) {
		const val = (e.target as HTMLInputElement).value;
		query = val;
		clearTimeout(debounceTimer);
		debounceTimer = setTimeout(() => {
			const params = new URLSearchParams(page.url.searchParams);
			if (val.length >= 2) {
				params.set('q', val);
			} else {
				params.delete('q');
			}
			goto(`?${params.toString()}`, { replaceState: true, keepFocus: true });
		}, 300);
	}
</script>

<svelte:head>
	<title>{t('search.title') ?? 'Search'} — JapaConnect Docs</title>
</svelte:head>

<div class="mx-auto max-w-3xl">
	<h1 class="text-3xl font-bold tracking-tight">{t('search.title') ?? 'Search Docs'}</h1>

	<!-- Search input -->
	<div class="relative mt-6">
		<Search class="pointer-events-none absolute left-3 top-1/2 h-5 w-5 -translate-y-1/2" style="color: var(--color-muted)" />
		<input
			type="text"
			value={query}
			oninput={onInput}
			placeholder={t('search.placeholder') ?? 'Search documentation...'}
			class="w-full rounded-lg border py-3 pl-10 pr-4 text-base outline-none focus:ring-2 focus:ring-blue-500"
			style="border-color: var(--color-border); background: var(--color-surface)"
		/>
	</div>

	<!-- Results -->
	<div class="mt-6 space-y-3">
		{#if query.length >= 2}
			{#if data.results.length === 0}
				<p class="py-8 text-center text-sm" style="color: var(--color-muted)">
					{t('search.no_results') ?? 'No results found'}
				</p>
			{:else}
				<p class="text-xs" style="color: var(--color-muted)">
					{data.results.length} {t('search.result_count') ?? 'results'}
				</p>
				{#each data.results as result}
					<a
						href="/{result.audience}/{result.slug}"
						class="flex items-start justify-between rounded-lg border p-4 transition-all hover:shadow-sm"
						style="border-color: var(--color-border)"
					>
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<FileText class="h-4 w-4 shrink-0" style="color: var(--color-muted)" />
								<span class="font-medium">{result.title}</span>
							</div>
							<p class="mt-1 text-sm" style="color: var(--color-muted)">{result.excerpt}</p>
							<span class="mt-2 inline-block rounded px-2 py-0.5 text-xs" style="background: var(--color-surface); color: var(--color-muted)">
								{result.audience}
							</span>
						</div>
						<ArrowRight class="mt-1 h-4 w-4 shrink-0" style="color: var(--color-muted)" />
					</a>
				{/each}
			{/if}
		{:else}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">
				{t('search.hint') ?? 'Type at least 2 characters to search'}
			</p>
		{/if}
	</div>
</div>
