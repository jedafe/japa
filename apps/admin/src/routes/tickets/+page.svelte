<script lang="ts">
	import { onMount } from 'svelte';
	import { RefreshCw, Download, Ticket, Clock, CheckCircle, AlertCircle } from 'lucide-svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { FilterBar } from '$lib/components/ui/filter-bar';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { api } from '$lib/api';

	type TicketItem = {
		id: string;
		type: string;
		subject: string;
		status: string;
		priority: string;
		assignee: string;
		created_at: string;
	};

	let tickets = $state<TicketItem[]>([]);
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);
	let sortKey = $state('created_at');
	let sortAsc = $state(false);
	let filterType = $state('');
	let filterStatus = $state('');
	let filterPriority = $state('');

	const tabs = [
		{ value: 'all', label: 'All Tickets', icon: Ticket },
		{ value: 'open', label: 'Open', icon: AlertCircle },
		{ value: 'in_progress', label: 'In Progress', icon: Clock },
		{ value: 'resolved', label: 'Resolved', icon: CheckCircle }
	];

	const filterDefs = [
		{
			key: 'type',
			label: 'Type',
			options: [
				{ value: 'bug', label: 'Bug' },
				{ value: 'feature', label: 'Feature' },
				{ value: 'question', label: 'Question' },
				{ value: 'account', label: 'Account' }
			]
		},
		{
			key: 'priority',
			label: 'Priority',
			options: [
				{ value: 'low', label: 'Low' },
				{ value: 'medium', label: 'Medium' },
				{ value: 'high', label: 'High' },
				{ value: 'urgent', label: 'Urgent' }
			]
		}
	];

	const columns = [
		{
			key: 'id',
			label: 'Ticket',
			sortable: true,
			render: (_: any, row: TicketItem) => `
				<div>
					<p class="font-medium text-gray-900">#${row.id} — ${row.subject || 'No subject'}</p>
					<p class="text-sm text-gray-500">${row.type}</p>
				</div>`
		},
		{
			key: 'type',
			label: 'Type',
			sortable: true,
			render: (value: string) => {
				const cls = 'bg-gray-100 text-gray-800';
				return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${cls}">${value}</span>`;
			}
		},
		{
			key: 'status',
			label: 'Status',
			sortable: true,
			render: (value: string) => {
				const cls =
					value === 'open'
						? 'bg-warning-100 text-warning-800'
						: value === 'resolved'
							? 'bg-success-100 text-success-800'
							: value === 'in_progress'
								? 'bg-info-100 text-info-800'
								: 'bg-gray-100 text-gray-800';
				return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${cls}">${value?.replace('_', ' ') || 'open'}</span>`;
			}
		},
		{
			key: 'priority',
			label: 'Priority',
			sortable: true,
			render: (value: string) => {
				const cls =
					value === 'urgent'
						? 'text-danger-700 font-semibold'
						: value === 'high'
							? 'text-warning-700 font-medium'
							: 'text-gray-600';
				return `<span class="text-sm ${cls}">${value}</span>`;
			}
		},
		{
			key: 'assignee',
			label: 'Assignee',
			render: (value: string) =>
				`<span class="text-sm text-gray-600">${value || '—'}</span>`
		},
		{
			key: 'created_at',
			label: 'Created',
			sortable: true,
			render: (value: string) =>
				`<span class="text-sm text-gray-500">${new Date(value).toLocaleDateString()}</span>`
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			render: (_: any, row: TicketItem) => `
				<button onclick="window.location.href='/tickets/${row.id}'" class="inline-flex items-center justify-center rounded-md px-3 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100 transition-colors">View</button>`
		}
	];

	async function load() {
		loading = true;
		const params = new URLSearchParams({
			page: String(page),
			sort: sortKey,
			order: sortAsc ? 'asc' : 'desc'
		});
		if (filterType) params.set('type', filterType);
		if (filterStatus) params.set('status', filterStatus);
		if (filterPriority) params.set('priority', filterPriority);
		try {
			const result = await api<{ tickets: TicketItem[]; page: number; total_pages: number }>(
				`/admin/tickets?${params}`
			);
			if (result.ok) {
				tickets = result.data.tickets;
				totalPages = result.data.total_pages;
			}
		} catch {
			// API not available
		} finally {
			loading = false;
		}
	}

	function handleFilter(values: Record<string, string>) {
		filterType = values.type ?? '';
		filterPriority = values.priority ?? '';
		page = 0;
		load();
	}

	function handleSearch(searchFilters: Record<string, string>) {
		handleFilter(searchFilters);
	}

	function handleClear() {
		filterType = '';
		filterStatus = '';
		filterPriority = '';
		page = 0;
		load();
	}

	function handleTabChange(tab: string) {
		if (tab === 'all') {
			filterStatus = '';
		} else {
			filterStatus = tab;
		}
		page = 0;
		load();
	}

	onMount(load);
</script>

<svelte:head>
	<title>Tickets — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title="Support Tickets"
	description="Manage user support requests"
	actions={[
		{
			label: 'Export',
			icon: Download,
			variant: 'outline' as const,
			onclick: () => {}
		}
	]}
/>

<div class="space-y-6">
	<Tabs tabs={[]} bind:value={filterStatus}>
		{#snippet children()}
			<!-- Custom tab bar -->
		{/snippet}
	</Tabs>

	<div class="flex gap-1 rounded-lg bg-gray-100 p-1">
		{#each tabs as tab}
			{@const Icon = tab.icon}
			<button
				class="inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-all disabled:pointer-events-none disabled:opacity-50 {(filterStatus === '' && tab.value === 'all') || filterStatus === tab.value
					? 'bg-white text-gray-950 shadow-sm'
					: 'text-gray-500 hover:text-gray-900'}"
				onclick={() => handleTabChange(tab.value)}
			>
				<Icon class="mr-2 h-4 w-4" />
				{tab.label}
			</button>
		{/each}
	</div>

	<FilterBar filters={filterDefs} onsearch={handleSearch} onclear={handleClear} />

	{#if loading}
		<Card>
			<CardContent class="py-12">
				<div class="flex items-center justify-center text-gray-500">
					<RefreshCw class="h-5 w-5 mr-2 animate-spin" />
					Loading...
				</div>
			</CardContent>
		</Card>
	{:else}
		<Card>
			<CardContent class="p-0">
				<DataTable
					{columns}
					data={tickets}
					pageSize={20}
					selectable={true}
					emptyMessage="No tickets found"
				/>
			</CardContent>
		</Card>
	{/if}
</div>
