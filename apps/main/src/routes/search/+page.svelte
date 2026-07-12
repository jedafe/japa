<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Search, Loader2, X } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api } from '$lib/api';
	import { trackEvent, AnalyticsEvents } from '$lib/analytics';
	import autoAnimate from '@formkit/auto-animate';
	import { t } from '$lib/stores/i18n.svelte';

	let query = $state(page.url.searchParams.get('q') || '');
	let results = $state<Array<{ id: string; title: string; type: string; excerpt: string }>>([]);
	let suggestions = $state<Array<{ id: string; title: string; type: string }>>([]);
	let loading = $state(false);
	let searched = $state(false);
	let showSuggestions = $state(false);
	let debounceTimer: ReturnType<typeof setTimeout> | null = null;

	$effect(() => {
		const q = page.url.searchParams.get('q');
		if (q && q !== query) {
			query = q;
			doSearch(q);
		}
	});

	function handleSubmit(e: Event) {
		e.preventDefault();
		if (query.trim()) {
			showSuggestions = false;
			goto(`/search?q=${encodeURIComponent(query.trim())}`, { replaceState: true });
			doSearch(query.trim());
		}
	}

	function handleInput() {
		if (debounceTimer) clearTimeout(debounceTimer);
		if (query.trim().length >= 2) {
			debounceTimer = setTimeout(fetchSuggestions, 300);
		} else {
			suggestions = [];
			showSuggestions = false;
		}
	}

	async function fetchSuggestions() {
		try {
			const res = await fetch(`/api/v1/search/autocomplete?q=${encodeURIComponent(query.trim())}&limit=5`, {
				credentials: 'include'
			});
			if (res.ok) {
				const { data } = await res.json();
				suggestions = data ?? [];
				showSuggestions = suggestions.length > 0;
			}
		} catch {
			suggestions = [];
		}
	}

	function selectSuggestion(suggestion: { id: string; title: string; type: string }) {
		showSuggestions = false;
		query = suggestion.title;
		goto(`/search?q=${encodeURIComponent(suggestion.title)}`, { replaceState: true });
		doSearch(suggestion.title);
	}

	function clearQuery() {
		query = '';
		suggestions = [];
		showSuggestions = false;
	}

	async function doSearch(q: string) {
		loading = true;
		searched = true;
		try {
			results = await api.search(q, { limit: 20 });
			trackEvent(AnalyticsEvents.SEARCH, { query: q, result_count: results.length });
		} catch {
			results = [];
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>{t('search.results')} | JapaConnect</title>
	<meta name="description" content="{t('search.find')}" />
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
	<header class="mb-8">
		<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">{t('search.results')}</h1>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			{t('search.find')}
		</p>
	</header>

	<form onsubmit={handleSubmit} class="mb-8">
		<div class="relative">
			<Search
				class="absolute left-3 top-1/2 h-5 w-5 -translate-y-1/2"
				style="color: var(--color-muted)"
			/>
			<input
				type="search"
				placeholder={t('search.placeholder_short')}
				bind:value={query}
				oninput={handleInput}
				onfocus={() => { if (suggestions.length > 0) showSuggestions = true; }}
				onblur={() => { setTimeout(() => { showSuggestions = false; }, 200); }}
				autocomplete="off"
				class="w-full rounded-xl border py-3 pl-10 pr-10 text-sm outline-none transition-colors focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
			/>
			{#if query}
				<button
					type="button"
					onclick={clearQuery}
					class="absolute right-3 top-1/2 -translate-y-1/2 rounded-full p-0.5 transition-colors hover:bg-black/10 dark:hover:bg-white/10"
				>
					<X class="h-4 w-4" style="color: var(--color-muted)" />
				</button>
			{/if}

			<!-- Autocomplete suggestions -->
			{#if showSuggestions && suggestions.length > 0}
				<div
					class="absolute left-0 right-0 top-full z-50 mt-1 overflow-hidden rounded-xl border shadow-lg"
					style="border-color: var(--color-border); background-color: var(--color-surface)"
				>
					{#each suggestions as suggestion (suggestion.id)}
						<button
							type="button"
							onmousedown={() => selectSuggestion(suggestion)}
							class="flex w-full items-center gap-3 px-4 py-3 text-left text-sm transition-colors hover:bg-black/5 dark:hover:bg-white/5"
						>
							<Search class="h-4 w-4 shrink-0" style="color: var(--color-muted)" />
							<div class="flex-1 min-w-0">
								<p class="font-medium truncate" style="color: var(--color-foreground)">{suggestion.title}</p>
								<p class="text-xs" style="color: var(--color-muted)">{suggestion.type}</p>
							</div>
						</button>
					{/each}
				</div>
			{/if}
		</div>
	</form>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<Loader2 class="h-6 w-6 animate-spin" style="color: var(--color-primary)" />
		</div>
	{:else if searched && results.length === 0}
		<EmptyState
			icon={Search}
			title={t('search.no_results')}
			message={t('search.no_results_hint')}
		/>
	{:else if results.length > 0}
		<div class="divide-y rounded-xl border" style="border-color: var(--color-border); divide-color: var(--color-border)" use:autoAnimate>
			{#each results as result (result.id)}
				<a
					href={result.type === 'user' ? `/profile/${result.id}` : `/content/${result.id}`}
					class="block p-4 transition-colors hover:bg-black/5 dark:hover:bg-white/5"
				>
					<div class="flex items-start gap-3">
						<div class="flex-1 min-w-0">
							<p class="text-sm font-medium" style="color: var(--color-foreground)">
								{result.title}
							</p>
							<p class="mt-1 text-xs" style="color: var(--color-muted)">
								{result.type}
							</p>
							{#if result.excerpt}
								<p class="mt-1 text-sm line-clamp-2" style="color: var(--color-muted)">
									{result.excerpt}
								</p>
							{/if}
						</div>
					</div>
				</a>
			{/each}
		</div>
	{:else}
		<div class="py-12 text-center">
			<Search class="mx-auto h-12 w-12" style="color: var(--color-border)" />
			<p class="mt-4 text-sm" style="color: var(--color-muted)">
				{t('search.enter_term')}
			</p>
		</div>
	{/if}
</main>
