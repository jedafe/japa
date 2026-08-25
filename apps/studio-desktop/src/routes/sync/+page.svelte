<script lang="ts">
	import { onMount } from 'svelte';
	import { RefreshCw, AlertCircle, Check, Loader2 } from 'lucide-svelte';
	import { queueStore } from '$lib/stores/queue.svelte';

	let loading = $state(false);

	onMount(() => {
		queueStore.load();
	});

	function refresh() {
		loading = true;
		setTimeout(() => {
			loading = false;
		}, 300);
	}

	function formatDate(iso: string): string {
		if (!iso) return '—';
		const d = new Date(iso);
		return isNaN(d.getTime()) ? '—' : d.toLocaleString();
	}

	const history = $derived(
		queueStore.files
			.filter((f) => f.lastPublishedAt || f.lastError)
			.slice()
			.sort((a, b) => {
				const ta = a.lastPublishedAt ? new Date(a.lastPublishedAt).getTime() : 0;
				const tb = b.lastPublishedAt ? new Date(b.lastPublishedAt).getTime() : 0;
				return tb - ta;
			})
	);
</script>

<div class="flex h-full flex-col p-6">
	<header class="mb-6 flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold">Sync Log</h1>
			<p class="text-sm text-muted-foreground">History of publish, update, and failed sync attempts.</p>
		</div>
		<button
			type="button"
			class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent disabled:opacity-60"
			disabled={loading}
			onclick={refresh}
		>
			{#if loading}
				<Loader2 class="h-4 w-4 animate-spin" />
			{:else}
				<RefreshCw class="h-4 w-4" />
			{/if}
			<span>Refresh</span>
		</button>
	</header>

	{#if history.length === 0}
		<div class="rounded-xl border border-border bg-card p-8 text-center text-muted-foreground">
			<p>Sync log will appear here after the first publish or update.</p>
		</div>
	{:else}
		<div class="overflow-hidden rounded-xl border border-border bg-card">
			<table class="w-full text-sm">
				<thead class="border-b border-border bg-accent">
					<tr>
						<th class="px-4 py-2 text-left font-medium">Title</th>
						<th class="px-4 py-2 text-left font-medium">State</th>
						<th class="px-4 py-2 text-left font-medium">Last Synced</th>
						<th class="px-4 py-2 text-left font-medium">Message</th>
					</tr>
				</thead>
				<tbody>
					{#each history as row (row.relativePath)}
						<tr class="border-b border-border last:border-0 hover:bg-accent/50">
							<td class="px-4 py-2">
								<a
									href="/editor/{encodeURIComponent(row.relativePath)}"
									class="font-medium text-primary hover:underline"
								>
									{row.title}
								</a>
								<p class="text-xs text-muted-foreground">{row.relativePath}</p>
							</td>
							<td class="px-4 py-2 capitalize">
								<span
									class="inline-flex items-center gap-1 rounded border px-2 py-0.5 text-xs {row.syncState === 'published'
										? 'border-success/20 bg-success/10 text-success'
										: row.syncState === 'in_review'
											? 'border-purple-500/20 bg-purple-500/10 text-purple-600'
											: 'border-danger/20 bg-danger/5 text-danger'}"
								>
									{#if row.syncState === 'published'}
										<Check class="h-3 w-3" />
									{:else}
										<AlertCircle class="h-3 w-3" />
									{/if}
									{row.syncState.replace('_', ' ')}
								</span>
							</td>
							<td class="px-4 py-2 text-muted-foreground">
								{formatDate(row.lastPublishedAt)}
							</td>
							<td class="px-4 py-2">
								{#if row.lastError}
									<p class="text-danger">{row.lastError}</p>
								{:else}
									<span class="text-muted-foreground">—</span>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
