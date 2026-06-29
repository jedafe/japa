<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Search, Loader2 } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { getToken } from '$lib/api';
	import autoAnimate from '@formkit/auto-animate';
	import { t } from '$lib/stores/i18n.svelte';

	let query = $state(page.url.searchParams.get('q') || '');
	let results = $state<Array<{ id: string; title: string; type: string; excerpt: string }>>([]);
	let loading = $state(false);
	let searched = $state(false);

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
			goto(`/search?q=${encodeURIComponent(query.trim())}`, { replaceState: true });
			doSearch(query.trim());
		}
	}

	async function doSearch(q: string) {
		loading = true;
		searched = true;
		try {
			const headers: Record<string, string> = { 'Content-Type': 'application/json' };
			const token = getToken();
			if (token) headers['Authorization'] = `Bearer ${token}`;
			const res = await fetch(`/api/v1/search?q=${encodeURIComponent(q)}&limit=20`, { headers });
			const body = await res.json();
			results = body.data || [];
		} catch {
			results = [];
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>{$t('search.results')} | JapaConnect</title>
	<meta name="description" content="{$t('search.find')}" />
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
	<header class="mb-8">
		<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">{$t('search.results')}</h1>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			{$t('search.find')}
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
				placeholder={$t('search.placeholder_short')}
				bind:value={query}
				class="w-full rounded-xl border py-3 pl-10 pr-4 text-sm outline-none transition-colors focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
			/>
		</div>
	</form>

	{#if loading}
		<div class="flex items-center justify-center py-12">
			<Loader2 class="h-6 w-6 animate-spin" style="color: var(--color-primary)" />
		</div>
	{:else if searched && results.length === 0}
		<EmptyState
			icon={Search}
			title={$t('search.no_results')}
			message={$t('search.no_results_hint')}
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
				{$t('search.enter_term')}
			</p>
		</div>
	{/if}
</main>
