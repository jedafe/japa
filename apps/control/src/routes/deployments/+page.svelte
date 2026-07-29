<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import { api } from '$lib/api';
	import type { Deployment, DataTableColumn } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import { Rocket, RefreshCw } from 'lucide-svelte';

	let deployments = $state<Deployment[]>([]);
	let loading = $state(true);
	let apiAvailable = $state(true);
	let lastUpdated = $state<Date | null>(null);

	let page = $state(0);
	let pageSize = $state(10);
	let sortKey = $state('');
	let sortDirection = $state<'asc' | 'desc'>('asc');

	async function load() {
		loading = true;
		try {
			const result = await api<Deployment[]>('/control/deployments');
			if (result.ok) {
				deployments = Array.isArray(result.data) ? result.data : [];
				apiAvailable = true;
				lastUpdated = new Date();
			} else {
				apiAvailable = false;
			}
		} catch {
			apiAvailable = false;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
		const interval = setInterval(() => {
			if (document.visibilityState === 'visible') load();
		}, 30_000);
		return () => clearInterval(interval);
	});

	function formatDate(iso: string): string {
		if (!iso) return '—';
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return iso;
		return d.toLocaleString();
	}

	const statusClasses: Record<string, string> = {
		active: 'bg-emerald-500/15 text-emerald-600',
		rolled_back: 'bg-amber-500/15 text-amber-500',
		failed: 'bg-destructive/15 text-destructive'
	};

	let sorted = $derived.by(() => {
		if (!sortKey) return deployments;
		const copy = [...deployments];
		copy.sort((a, b) => {
			const aVal = a[sortKey as keyof Deployment] ?? '';
			const bVal = b[sortKey as keyof Deployment] ?? '';
			const cmp = String(aVal).localeCompare(String(bVal));
			return sortDirection === 'asc' ? cmp : -cmp;
		});
		return copy;
	});

	let totalRows = $derived(deployments.length);

	let paginatedDeployments = $derived.by(() => {
		const start = page * pageSize;
		return sorted.slice(start, start + pageSize);
	});

	const columns: DataTableColumn[] = [
		{
			key: 'version',
			label: t('deployments.version', 'Version'),
			sortable: true,
			render: (v) => `<span class="font-mono text-xs font-medium">${escapeHtml(v)}</span>`
		},
		{
			key: 'status',
			label: t('common.status', 'Status'),
			sortable: true,
			render: (v) => {
				const s = escapeHtml(v);
				const cls = statusClasses[v as string] ?? 'bg-muted text-foreground';
				return `<span class="rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${s}</span>`;
			}
		},
		{
			key: 'deployed_at',
			label: t('deployments.deployed_at', 'Deployed At'),
			sortable: true,
			render: (v) => `<span class="text-muted-foreground font-mono text-xs">${escapeHtml(formatDate(v as string))}</span>`,
			class: 'text-muted-foreground'
		}
	];
</script>

<PageHeader title={t('deployments.title', 'Deployments')} description={t('deployments.description', 'Current deployment version and history')} />

<div class="mb-4 flex items-center justify-end gap-3">
	{#if lastUpdated}
		<span class="text-muted-foreground text-xs">
			{t('deployments.last_updated', 'Last updated')}: {lastUpdated.toLocaleTimeString()}
		</span>
	{/if}
	<button class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium transition-colors" onclick={load}>
		<RefreshCw class="size-3" />
		{t('common.refresh')}
	</button>
</div>

{#if !apiAvailable}
	<div class="border-tertiary bg-tertiary/10 text-tertiary mb-4 flex items-center gap-2 rounded-lg border px-4 py-3 text-sm">
		{t('deployments.unavailable', 'Deployment information is currently unavailable.')}
	</div>
{/if}

<!-- M6: Convert raw table to DataTable -->
<DataTable
	{columns}
	rows={paginatedDeployments as unknown as Record<string, unknown>[]}
	{totalRows}
	bind:page
	bind:pageSize
	bind:sortKey
	bind:sortDirection
	{loading}
	selectable={false}
	getRowId={(row) => `${(row as Record<string, unknown>).version}-${(row as Record<string, unknown>).deployed_at}`}
	emptyMessage={t('deployments.no_data', 'No deployment records found.')}
	emptyIcon={Rocket}
/>
