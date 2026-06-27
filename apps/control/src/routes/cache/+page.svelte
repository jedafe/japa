<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import { api } from '$lib/api';

	const cacheTargets = [
		{ key: 'all', label: 'Flush All Caches', description: 'Clear every cached layer' },
		{ key: 'pages', label: 'Page Cache', description: 'Rendered page fragments' },
		{ key: 'feeds', label: 'Feed Cache', description: 'Content feed results' },
		{ key: 'i18n', label: 'i18n Cache', description: 'Language pack translations' },
		{ key: 'search', label: 'Search Cache', description: 'Meilisearch result cache' }
	];

	let flushing = $state<Record<string, boolean>>({});
	let messages = $state<Record<string, string>>({});

	async function flush(target: string) {
		flushing[target] = true;
		messages[target] = '';
		try {
			await api(`/control/cache/${target}`, { method: 'POST' });
			messages[target] = 'Flushed';
		} catch {
			messages[target] = 'Failed';
		} finally {
			flushing[target] = false;
		}
	}
</script>

<PageHeader title="Cache Management" description="Clear cached layers independently" />

<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
	{#each cacheTargets as target}
		<div class="rounded-lg border p-5" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h3 class="text-sm font-semibold">{target.label}</h3>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">{target.description}</p>
			<div class="mt-4 flex items-center gap-2">
				<button
					class="rounded-md border px-3 py-1.5 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-50"
					style="border-color: var(--color-border)"
					disabled={flushing[target.key]}
					onclick={() => flush(target.key)}
				>
					{flushing[target.key] ? 'Flushing...' : 'Flush'}
				</button>
				{#if messages[target.key]}
					<span class="text-xs" style="color: var(--color-muted)">{messages[target.key]}</span>
				{/if}
			</div>
		</div>
	{/each}
</div>
