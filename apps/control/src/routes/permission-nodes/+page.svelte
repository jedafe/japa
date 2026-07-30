<!-- RBAC CRUD: Permission Nodes viewer -->
<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import AlertBanner from '$lib/components/ui/AlertBanner.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import { Shield, Search, RefreshCw } from 'lucide-svelte';
	import type { DataTableColumn } from '$lib/api/types';

	let {} = $props();

	interface PermissionNode {
		key: string;
		description: string;
	}

	let nodes = $state<PermissionNode[]>([]);
	let loading = $state(true);
	let message = $state('');
	let messageType = $state<'success' | 'error' | 'info'>('info');
	let search = $state('');

	let page = $state(0);
	let pageSize = $state(10);
	let sortKey = $state('');
	let sortDirection = $state<'asc' | 'desc'>('asc');

	const columns: DataTableColumn[] = [
		{
			key: 'key',
			label: t('permission_nodes.key', 'Permission Key'),
			sortable: true,
			render: (val: unknown) =>
				`<span class="bg-muted text-muted-foreground rounded-5px px-1.5 py-0.5 font-mono text-xs">${escapeHtml(val) || '—'}</span>`
		},
		{
			key: 'description',
			label: t('permission_nodes.description', 'Description'),
			sortable: false,
			render: (val: unknown) =>
				`<span class="text-muted-foreground text-sm">${escapeHtml(val) || '—'}</span>`
		}
	];

	async function load() {
		loading = true;
		try {
			const result = await api<PermissionNode[]>('/control/permission-nodes');
			if (result.ok) {
				nodes = Array.isArray(result.data) ? result.data : [];
			} else {
				message = result.error || t('permission_nodes.load_error', 'Failed to load permission nodes.');
				messageType = 'error';
			}
		} catch {
			message = t('permission_nodes.load_error', 'Failed to load permission nodes.');
			messageType = 'error';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
	});

	let filteredNodes = $derived.by(() => {
		const q = search.trim().toLowerCase();
		if (!q) return nodes;
		return nodes.filter(
			(n) =>
				n.key.toLowerCase().includes(q) ||
				n.description.toLowerCase().includes(q)
		);
	});

	let sortedNodes = $derived.by(() => {
		const list = [...filteredNodes];
		if (sortKey) {
			list.sort((a, b) => {
				const row = a as unknown as Record<string, unknown>;
				const row2 = b as unknown as Record<string, unknown>;
				const av = row[sortKey];
				const bv = row2[sortKey];
				if (av == null && bv == null) return 0;
				if (av == null) return 1;
				if (bv == null) return -1;
				const cmp = String(av).localeCompare(String(bv));
				return sortDirection === 'asc' ? cmp : -cmp;
			});
		}
		return list;
	});

	let totalRows = $derived(sortedNodes.length);

	let paginatedNodes = $derived.by(() => {
		const start = page * pageSize;
		return sortedNodes.slice(start, start + pageSize);
	});
</script>

<PageHeader
	title={t('permission_nodes.title', 'Permission Nodes')}
	description={t('permission_nodes.description_label', 'Registered RBAC permission keys (read-only)')}
/>

<div class="mb-4 flex flex-wrap items-center gap-3">
	<div class="border-border-input bg-background focus-within:border-border-input-hover focus-within:shadow-date-field-focus rounded-9px flex h-10 items-center gap-2 border px-3 transition-colors">
		<Search class="text-muted-foreground size-4 shrink-0" />
		<input
			type="text"
			bind:value={search}
			placeholder={t('permission_nodes.search_placeholder', 'Search by key or description...')}
			class="text-foreground placeholder:text-muted-foreground/60 w-56 bg-transparent text-sm outline-none"
			aria-label={t('permission_nodes.search', 'Search permission nodes')}
		/>
	</div>
	<div class="flex-1"></div>
	<div class="text-muted-foreground flex items-center gap-1.5 text-xs">
		<Shield class="size-3.5" />
		{t('permission_nodes.read_only_hint', 'Permission nodes are seeded and cannot be edited.')}
	</div>
	<button
		class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px flex items-center gap-1.5 border px-3 py-2 text-xs font-medium transition-colors"
		onclick={() => { message = ''; load(); }}
		disabled={loading}
	>
		<RefreshCw class="size-3 {loading ? 'animate-spin' : ''}" />
		{t('common.refresh', 'Refresh')}
	</button>
</div>

{#if message}
	<div class="mb-4">
		<AlertBanner variant={messageType} {message} />
	</div>
{/if}

<DataTable
	{columns}
	rows={paginatedNodes}
	{totalRows}
	bind:page
	bind:pageSize
	bind:sortKey
	bind:sortDirection
	{loading}
	emptyMessage={t('permission_nodes.no_nodes', 'No permission nodes registered.')}
	selectable={false}
	getRowId={(row) => (row as PermissionNode).key}
/>
