<script lang="ts">
	import { goto } from '$app/navigation';
	import { t } from '$lib/stores/i18n.svelte';
	import { BookOpen, ChevronRight, FileText, Search } from 'lucide-svelte';

	let { audience, navTree = [], currentSlug = '', onNavigate }: { audience: string; navTree?: Array<{ title?: string; slug?: string; audience?: string; children?: Array<{ title?: string; slug?: string }> }>; currentSlug?: string; onNavigate?: () => void } = $props();

	let query = $state('');
	let results = $state<Array<{ title: string; slug: string; audience: string; excerpt: string }>>([]);
	let searching = $state(false);
	let showResults = $state(false);

	let debounceTimer: ReturnType<typeof setTimeout>;

	function onInput(e: Event) {
		const val = (e.target as HTMLInputElement).value;
		query = val;
		clearTimeout(debounceTimer);
		if (val.length < 2) {
			results = [];
			showResults = false;
			return;
		}
		debounceTimer = setTimeout(() => search(val), 300);
	}

	async function search(q: string) {
		searching = true;
		showResults = true;
		try {
			const res = await fetch(`/api/v1/docs/search?q=${encodeURIComponent(q)}&limit=10`);
			if (res.ok) {
				const { data } = await res.json();
				results = data ?? [];
			} else {
				results = [];
			}
		} catch {
			results = [];
		} finally {
			searching = false;
		}
	}

	function selectResult(result: { slug: string; audience: string }) {
		query = '';
		results = [];
		showResults = false;
		goto(`/${result.audience}/${result.slug}`);
		onNavigate?.();
	}
</script>

<aside class="flex h-full w-64 flex-col border-r p-4" style="border-color: var(--color-border); background-color: var(--color-background)">
	<!-- Brand -->
	<div class="mb-6">
		<a href="/" class="text-lg font-bold tracking-tight" style="color: var(--color-primary)">
			{t('docs.brand')}
		</a>
	</div>

	<!-- Search -->
	<div class="relative mb-6">
		<Search class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
		<input
			type="text"
			placeholder={t('sidebar.search')}
			class="w-full rounded-md border py-2 pl-9 pr-3 text-sm outline-none focus:ring-2"
			style="border-color: var(--color-border); background: var(--color-surface)"
			value={query}
			oninput={onInput}
			onfocus={() => { if (query.length >= 2) showResults = true; }}
			onblur={() => setTimeout(() => (showResults = false), 200)}
		/>
		{#if showResults}
			<div class="absolute left-0 right-0 top-full z-50 mt-1 rounded-md border bg-white shadow-lg dark:bg-gray-800" style="border-color: var(--color-border)">
				{#if searching}
					<p class="px-3 py-2 text-sm" style="color: var(--color-muted)">Searching...</p>
				{:else if results.length === 0}
					<p class="px-3 py-2 text-sm" style="color: var(--color-muted)">No results found</p>
				{:else}
					{#each results as result}
						<button
							type="button"
							class="w-full px-3 py-2 text-left hover:bg-gray-50 dark:hover:bg-gray-700"
							onmousedown={() => selectResult(result)}
						>
							<p class="text-sm font-medium" style="color: var(--color-foreground)">{result.title}</p>
							<p class="mt-0.5 text-xs" style="color: var(--color-muted)">{result.excerpt}</p>
						</button>
					{/each}
				{/if}
			</div>
		{/if}
	</div>

	<!-- Navigation -->
	<nav class="flex-1 space-y-1 overflow-y-auto">
		{#each navTree as item}
			{@const Icon = item.children?.length ? ChevronRight : FileText}
			<a
				href="/{item.slug}"
				class="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors"
				class:bg-blue-50={audience === item.slug}
				class:text-blue-700={audience === item.slug}
				style={audience !== item.slug ? 'color: var(--color-muted)' : ''}
				onclick={onNavigate}
			>
				<Icon class="h-4 w-4" />
				{item.title}
			</a>
			{#if item.children && audience === item.slug}
				<div class="ml-4 space-y-0.5">
					{#each item.children as child}
						<a
							href="/{item.slug}/{child.slug}"
							class="flex items-center gap-2 rounded-md px-3 py-1.5 text-sm transition-colors"
							class:bg-blue-50={currentSlug === child.slug}
							class:text-blue-700={currentSlug === child.slug}
							style={currentSlug !== child.slug ? 'color: var(--color-muted)' : ''}
							onclick={onNavigate}
						>
							{child.title}
						</a>
					{/each}
				</div>
			{/if}
		{/each}
	</nav>
</aside>
