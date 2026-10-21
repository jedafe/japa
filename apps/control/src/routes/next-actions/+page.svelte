<!-- §122 Next-Action Suggestions — rule management -->
<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import AlertBanner from '$lib/components/ui/AlertBanner.svelte';
	import { t, translations } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import { Plus, RefreshCw } from 'lucide-svelte';
	import { listRules, getStats, deleteRule, type NextActionRule, type NextActionStat } from '$lib/api/nextactions';
	import type { DataTableColumn, DataTableRowAction } from '$lib/api/types';
	import { goto } from '$app/navigation';

	let rules = $state<NextActionRule[]>([]);
	let stats = $state<NextActionStat[]>([]);
	let loading = $state(true);
	let message = $state('');
	let messageType = $state<'success' | 'error' | 'info'>('info');
	let search = $state('');

	const columns = $derived.by<DataTableColumn[]>(() => {
		translations();
		return [
			{ key: 'slug', label: 'Slug', sortable: true,
			  render: (v: unknown) => `<span class="font-mono text-xs">${escapeHtml(v)}</span>` },
			{ key: 'name', label: 'Name', sortable: true,
			  render: (v: unknown) => escapeHtml(v) },
			{ key: 'enabled', label: 'Enabled', sortable: true,
			  render: (v: unknown) => v ? '✓' : '—' },
			{ key: 'priority', label: 'Priority', sortable: true },
			{ key: 'channels', label: 'Channels',
			  render: (v: unknown) => `<span class="text-muted-foreground text-xs">${escapeHtml(Array.isArray(v) ? v.join(', ') : '')}</span>` },
			{ key: 'id', label: '',
			  render: (v: unknown) => `<a href="/next-actions/${escapeHtml(v)}" class="text-primary text-xs underline">Edit</a>` }
		];
	});

	async function remove(row: NextActionRule) {
		if (!confirm(`Delete rule "${row.slug}"?`)) return;
		const r = await deleteRule(row.id);
		if (r.ok) await load();
		else { message = r.error || 'Delete failed'; messageType = 'error'; }
	}

	const rowActions: DataTableRowAction[] = [
		{
			label: 'Edit',
			onClick: (row) => goto(`/next-actions/${row.id}`)
		},
		{
			label: 'Delete',
			onClick: (row) => void remove(row as unknown as NextActionRule),
			variant: 'destructive'
		}
	];

	async function load() {
		loading = true;
		try {
			const [r, s] = await Promise.all([listRules(), getStats()]);
			if (r.ok) rules = r.data ?? [];
			else { message = r.error || 'Failed to load rules'; messageType = 'error'; }
			if (s.ok) stats = s.data ?? [];
		} catch {
			message = 'Failed to load rules';
			messageType = 'error';
		} finally {
			loading = false;
		}
	}

	$effect(() => { load(); });

	let filtered = $derived.by(() => {
		const q = search.trim().toLowerCase();
		if (!q) return rules;
		return rules.filter((r) => r.slug.toLowerCase().includes(q) || r.name.toLowerCase().includes(q));
	});
</script>

<PageHeader title="Next-Action Suggestions" description="Manage contextual next-step suggestion rules (§122)" />

<div class="mb-3 flex items-center gap-2">
	<button onclick={() => goto('/next-actions/new')} class="bg-primary text-primary-foreground inline-flex items-center gap-1 rounded-[var(--radius)] px-3 py-1.5 text-sm">
		<Plus size={14} /> New rule
	</button>
</div>

{#if message}
	<AlertBanner variant={messageType} {message} />
{/if}

<div class="mb-3 flex items-center gap-2">
	<input bind:value={search} placeholder="Filter rules…" class="border-border bg-background w-64 rounded-[var(--radius)] border px-3 py-1.5 text-sm" />
	<button onclick={load} class="text-muted-foreground hover:text-foreground p-1" aria-label="Refresh"><RefreshCw size={14} /></button>
</div>

<DataTable {columns} rows={filtered} {loading} {rowActions} />

{#if stats.length > 0}
	<h2 class="text-foreground mt-8 mb-2 text-sm font-medium">Funnel (lifetime)</h2>
	<p class="text-muted-foreground mb-2 text-xs">Served = suggestions returned by the API (a suggestion may be served without rendering); Completed = real completions via completion_signal or an explicit complete call, not CTA clicks.</p>
	<div class="border-border overflow-x-auto rounded-[var(--radius-card)] border">
		<table class="w-full text-sm">
			<thead><tr class="text-muted-foreground border-border border-b text-left">
				<th class="px-3 py-2">Rule</th><th class="px-3 py-2">Served</th><th class="px-3 py-2">Clicked</th><th class="px-3 py-2">Dismissed</th><th class="px-3 py-2">Completed</th>
			</tr></thead>
			<tbody>
				{#each stats as s (s.slug)}
					<tr class="border-border border-b last:border-0">
						<td class="px-3 py-2 font-mono text-xs">{s.slug}</td>
						<td class="px-3 py-2">{s.shown}</td>
						<td class="px-3 py-2">{s.clicked}</td>
						<td class="px-3 py-2">{s.dismissed}</td>
						<td class="px-3 py-2">{s.completed}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
