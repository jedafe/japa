<script lang="ts">
	import { onMount } from 'svelte';
	import { RefreshCw, AlertCircle, Check, Loader2, Trash2 } from 'lucide-svelte';
	import { syncLogStore, type SyncLogEntry } from '$lib/stores/sync-log.svelte';

	let loading = $state(false);

	onMount(() => {
		syncLogStore.load();
	});

	function refresh() {
		loading = true;
		setTimeout(() => {
			loading = false;
		}, 300);
	}

	function clearLog() {
		syncLogStore.clear();
	}

	function formatDate(iso: string): string {
		if (!iso) return '—';
		const d = new Date(iso);
		return isNaN(d.getTime()) ? '—' : d.toLocaleString();
	}

	function actionLabel(action: SyncLogEntry['action']): string {
		const labels: Record<string, string> = {
			create: 'Create',
			update: 'Update',
			reassign: 'Reassign',
			tags: 'Tags',
			archive: 'Archive',
			failed: 'Failed'
		};
		return labels[action] ?? action;
	}

	const history = $derived(
		syncLogStore.entries.slice().sort((a, b) => {
			const ta = a.createdAt ? new Date(a.createdAt).getTime() : 0;
			const tb = b.createdAt ? new Date(b.createdAt).getTime() : 0;
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
		<div class="flex gap-2">
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
			<button
				type="button"
				class="inline-flex items-center gap-2 rounded-lg border border-danger/20 bg-danger/5 px-4 py-2 text-sm text-danger hover:bg-danger/10"
				onclick={clearLog}
			>
				<Trash2 class="h-4 w-4" />
				<span>Clear</span>
			</button>
		</div>
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
						<th class="px-4 py-2 text-left font-medium">Time</th>
						<th class="px-4 py-2 text-left font-medium">File</th>
						<th class="px-4 py-2 text-left font-medium">Action</th>
						<th class="px-4 py-2 text-left font-medium">Content ID</th>
						<th class="px-4 py-2 text-left font-medium">Message</th>
					</tr>
				</thead>
				<tbody>
					{#each history as row (row.id)}
						<tr class="border-b border-border last:border-0 hover:bg-accent/50">
							<td class="px-4 py-2 text-muted-foreground">
								{formatDate(row.createdAt)}
							</td>
							<td class="px-4 py-2">
								<a
									href="/editor/{encodeURIComponent(row.relativePath)}"
									class="font-medium text-primary hover:underline"
								>
									{row.relativePath.split('/').pop()}
								</a>
								<p class="text-xs text-muted-foreground">{row.relativePath}</p>
							</td>
							<td class="px-4 py-2">
								<span
									class="inline-flex items-center gap-1 rounded border px-2 py-0.5 text-xs {row.action === 'create' || row.action === 'update' || row.action === 'reassign' || row.action === 'tags'
										? 'border-success/20 bg-success/10 text-success'
										: 'border-danger/20 bg-danger/5 text-danger'}"
								>
									{#if row.action === 'failed'}
										<AlertCircle class="h-3 w-3" />
									{:else}
										<Check class="h-3 w-3" />
									{/if}
									{actionLabel(row.action)}
								</span>
							</td>
							<td class="px-4 py-2 text-muted-foreground">
								{row.contentId ?? '—'}
							</td>
							<td class="px-4 py-2">
								{#if row.error}
									<p class="text-danger">{row.error}</p>
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
