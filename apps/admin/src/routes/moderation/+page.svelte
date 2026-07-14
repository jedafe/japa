<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { RefreshCw, Check, X, AlertTriangle, FileText, Flag, Scale, Bell, List } from 'lucide-svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import { FilterBar } from '$lib/components/ui/filter-bar';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { page as pageState } from '$app/state';

	type ModItem = {
		id: string;
		type: 'user_report' | 'ai_flag';
		content_type: 'post' | 'comment' | 'blog' | 'forum' | 'message';
		content_id: string;
		reporter_id: string;
		reporter_name?: string;
		reason: string;
		status: string;
		created_at: string;
		content_preview?: string;
	};

	let items = $state<ModItem[]>([]);
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);
	let filterType = $state('');
	let filterStatus = $state('');
	let filterContentType = $state('');
	let selectedIds = $state<Set<string>>(new Set());
	let errorMessage = $state('');

	// Polling state
	let lastCount = $state(0);
	let newItemsCount = $state(0);
	let isPolling = $state(false);
	let pollingInterval: ReturnType<typeof setInterval> | undefined;

	// Expandable detail
	let expandedId = $state<string | null>(null);

	// Action modal
	let showActionModal = $state(false);
	let actionItem = $state<ModItem | null>(null);
	let actionType = $state<'approve' | 'reject' | 'delete'>('approve');
	let actionReason = $state('');
	let acting = $state(false);

	// Tab navigation - derived from current URL
	let activeTab = $derived.by(() => {
		const path = pageState.url.pathname;
		if (path === '/moderation') return 'queue';
		if (path.startsWith('/moderation/content')) return 'content';
		if (path.startsWith('/moderation/reports')) return 'reports';
		if (path.startsWith('/moderation/appeals')) return 'appeals';
		if (path.startsWith('/moderation/alerts')) return 'alerts';
		return 'queue';
	});

	const tabs = [
		{ value: 'queue', label: t('moderation.queue'), icon: List },
		{ value: 'content', label: t('moderation.content'), icon: FileText },
		{ value: 'reports', label: t('moderation.reports'), icon: Flag },
		{ value: 'appeals', label: t('moderation.appeals'), icon: Scale },
		{ value: 'alerts', label: t('moderation.alerts'), icon: Bell }
	];

	const filterDefs = [
		{
			key: 'type',
			label: t('moderation.source'),
			options: [
				{ value: 'user_report', label: 'User Report' },
				{ value: 'ai_flag', label: 'AI Flag' }
			]
		},
		{
			key: 'content_type',
			label: t('moderation.content_type'),
			options: [
				{ value: 'post', label: 'Post' },
				{ value: 'comment', label: 'Comment' },
				{ value: 'blog', label: 'Blog' },
				{ value: 'forum', label: 'Forum' },
				{ value: 'message', label: 'Message' }
			]
		},
		{
			key: 'status',
			label: t('moderation.status'),
			options: [
				{ value: 'pending', label: 'Pending' },
				{ value: 'reviewed', label: 'Reviewed' },
				{ value: 'resolved', label: 'Resolved' },
				{ value: 'dismissed', label: 'Dismissed' }
			]
		}
	];

	const columns = [
		{
			key: 'type',
			label: t('moderation.source'),
			sortable: true,
			render: (_: any, row: ModItem) => {
				const label = row.type === 'ai_flag' ? 'AI Flag' : 'User Report';
				const icon =
					row.type === 'ai_flag'
						? '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z"></path>'
						: '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4.5c-.77-.833-2.694-.833-3.464 0L3.34 16.5c-.77.833.192 2.5 1.732 2.5z"></path>';
				const cls =
					row.type === 'ai_flag'
						? 'bg-purple-100 text-purple-800'
						: 'bg-blue-100 text-blue-800';
				return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${cls}"><svg class="h-3.5 w-3.5 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">${icon}</svg>${label}</span>`;
			}
		},
		{
			key: 'content_type',
			label: t('moderation.content_type'),
			sortable: true,
			render: (value: string) => {
				const colorMap: Record<string, string> = {
					post: 'bg-primary-100 text-primary-800',
					comment: 'bg-info-100 text-info-800',
					blog: 'bg-danger-100 text-danger-800',
					forum: 'bg-warning-100 text-warning-800',
					message: 'bg-gray-100 text-gray-800'
				};
				return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${colorMap[value] ?? 'bg-gray-100 text-gray-800'}">${value}</span>`;
			}
		},
		{
			key: 'content_preview',
			label: t('moderation.content'),
			render: (_: any, row: ModItem) => {
				const preview = row.content_preview || row.content_id.slice(0, 12);
				return `<span class="text-sm text-gray-600 truncate max-w-xs block">${preview}</span>`;
			}
		},
		{
			key: 'reason',
			label: t('moderation.reason'),
			sortable: true,
			render: (value: string) => `<span class="text-sm text-gray-700">${value}</span>`
		},
		{
			key: 'status',
			label: t('moderation.status'),
			sortable: true,
			render: (value: string) => {
				const cls =
					value === 'resolved'
						? 'bg-success-100 text-success-800'
						: value === 'pending'
							? 'bg-warning-100 text-warning-800'
							: value === 'reviewed'
								? 'bg-info-100 text-info-800'
								: 'bg-gray-100 text-gray-800';
				return `<span class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${cls}">${value}</span>`;
			}
		},
		{
			key: 'created_at',
			label: t('moderation.date'),
			sortable: true,
			render: (value: string) =>
				`<span class="text-sm text-gray-500">${new Date(value).toLocaleDateString()}</span>`
		},
		{
			key: 'actions',
			label: '',
			class: 'w-32',
			render: (_: any, row: ModItem) => {
				if (row.status === 'pending') {
					return `
						<div class="flex items-center gap-1">
							<button onclick="window.moderationView('${row.id}')" class="inline-flex items-center justify-center rounded-md p-1.5 text-gray-600 hover:bg-gray-100 hover:text-gray-900 transition-colors" title="View">
								<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"></path></svg>
							</button>
							<button onclick="window.moderationApprove('${row.id}')" class="inline-flex items-center justify-center rounded-md p-1.5 text-success-700 hover:bg-success-50 transition-colors" title="Approve">
								<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"></path></svg>
							</button>
							<button onclick="window.moderationReject('${row.id}')" class="inline-flex items-center justify-center rounded-md p-1.5 text-danger-700 hover:bg-danger-50 transition-colors" title="Reject">
								<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path></svg>
							</button>
						</div>`;
				}
				return `
					<div class="flex items-center gap-1">
						<button onclick="window.moderationView('${row.id}')" class="inline-flex items-center justify-center rounded-md p-1.5 text-gray-600 hover:bg-gray-100 hover:text-gray-900 transition-colors" title="View">
							<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"></path></svg>
						</button>
						<button onclick="window.moderationDelete('${row.id}')" class="inline-flex items-center justify-center rounded-md p-1.5 text-danger-700 hover:bg-danger-50 transition-colors" title="Delete">
							<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path></svg>
						</button>
					</div>`;
			}
		}
	];

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams({ page: String(page) });
		if (filterType) params.set('type', filterType);
		if (filterStatus) params.set('status', filterStatus);
		if (filterContentType) params.set('content_type', filterContentType);
		try {
			const result = await api<{ items: ModItem[]; page: number; total_pages: number }>(
				`/admin/moderation-queue?${params}`
			);
			if (result.ok) {
				items = result.data.items;
				totalPages = result.data.total_pages;
				lastCount = items.length;
				newItemsCount = 0;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function startPolling() {
		pollingInterval = setInterval(async () => {
			const params = new URLSearchParams({ page: '0' });
			if (filterType) params.set('type', filterType);
			if (filterStatus) params.set('status', filterStatus);
			if (filterContentType) params.set('content_type', filterContentType);
			try {
				const result = await api<{ items: ModItem[]; page: number; total_pages: number }>(
					`/admin/moderation-queue?${params}`
				);
				if (result.ok) {
					const newCount = result.data.items.length;
					if (lastCount > 0 && newCount > lastCount) {
						newItemsCount = newCount - lastCount;
					}
					lastCount = newCount;
				}
			} catch {
				// Silently fail polling
			}
		}, 30_000);
		isPolling = true;
	}

	function stopPolling() {
		if (pollingInterval !== undefined) {
			clearInterval(pollingInterval);
			pollingInterval = undefined;
		}
		isPolling = false;
	}

	function refreshList() {
		newItemsCount = 0;
		load();
	}

	function handleFilter(values: Record<string, string>) {
		filterType = values.type ?? '';
		filterContentType = values.content_type ?? '';
		filterStatus = values.status ?? '';
		page = 0;
		load();
	}

	function toggleExpand(id: string) {
		expandedId = expandedId === id ? null : id;
	}

	function openAction(item: ModItem, action: 'approve' | 'reject' | 'delete') {
		actionItem = item;
		actionType = action;
		actionReason = '';
		showActionModal = true;
	}

	async function executeAction() {
		if (!actionItem) return;
		acting = true;
		errorMessage = '';
		const endpoint = `/admin/moderation-queue/${actionItem.id}`;
		try {
			const body: Record<string, unknown> = { action: actionType };
			if (actionReason.trim()) body.reason = actionReason.trim();
			const result = await api(endpoint, { method: 'PATCH', body });
			if (result.ok) {
				showActionModal = false;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			acting = false;
		}
	}

	async function bulkAction(action: 'approve' | 'reject') {
		const ids = Array.from(selectedIds);
		errorMessage = '';
		try {
			await api('/admin/moderation-queue/bulk', { method: 'POST', body: { ids, action } });
			selectedIds = new Set();
			load();
		} catch {
			errorMessage = t('moderation.bulk_error');
		}
	}

	function handleSearch(searchFilters: Record<string, string>) {
		handleFilter(searchFilters);
	}

	function handleClear() {
		filterType = '';
		filterContentType = '';
		filterStatus = '';
		page = 0;
		load();
	}

	// Make functions available globally for DataTable inline onclick handlers
	$effect(() => {
		(window as any).moderationView = (id: string) => {
			toggleExpand(id);
		};
		(window as any).moderationApprove = (id: string) => {
			const item = items.find((i) => i.id === id);
			if (item) openAction(item, 'approve');
		};
		(window as any).moderationReject = (id: string) => {
			const item = items.find((i) => i.id === id);
			if (item) openAction(item, 'reject');
		};
		(window as any).moderationDelete = (id: string) => {
			const item = items.find((i) => i.id === id);
			if (item) openAction(item, 'delete');
		};
	});

	onMount(() => {
		load();
		setTimeout(() => {
			lastCount = items.length;
			startPolling();
		}, 1000);
	});

	onDestroy(() => {
		stopPolling();
	});
</script>

<svelte:head>
	<title>{t('moderation.title')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader title={t('moderation.title')} description={t('moderation.description')} />

<div class="space-y-6">
	{#if isPolling}
		<div class="flex items-center gap-2 text-sm text-gray-500">
			<span class="relative flex h-2 w-2">
				<span
					class="absolute inline-flex h-full w-full animate-ping rounded-full opacity-75 bg-success-500"
				></span>
				<span class="relative inline-flex h-2 w-2 rounded-full bg-success-500"></span>
			</span>
			Auto-refresh active
		</div>
	{/if}

	{#if newItemsCount > 0}
		<div
			class="flex items-center gap-3 rounded-lg border border-info-200 bg-info-50 px-4 py-3"
		>
			<AlertTriangle class="h-4 w-4 text-info-600" />
			<span class="text-sm font-medium text-info-800">
				{newItemsCount} new {newItemsCount === 1 ? 'item' : 'items'}
			</span>
			<Button variant="ghost" size="sm" onclick={refreshList}>
				<RefreshCw class="h-3 w-3 mr-1" />
				Show
			</Button>
		</div>
	{/if}

	{#if errorMessage}
		<div class="rounded-lg border border-danger-200 bg-danger-50 px-4 py-3 text-sm text-danger-800">
			{errorMessage}
		</div>
	{/if}

	<Tabs {tabs} bind:value={activeTab} />

	<FilterBar filters={filterDefs} onsearch={handleSearch} onclear={handleClear} />

	{#if selectedIds.size > 0}
		<div class="flex items-center gap-3 rounded-lg border border-gray-200 bg-gray-50 px-4 py-3">
			<span class="text-sm font-medium text-gray-700">
				{selectedIds.size} {selectedIds.size === 1 ? 'item' : 'items'} selected
			</span>
			<Button variant="ghost" size="sm" onclick={() => bulkAction('approve')}>
				<Check class="h-3.5 w-3.5 mr-1" />
				{t('moderation.resolve')}
			</Button>
			<Button variant="ghost" size="sm" onclick={() => bulkAction('reject')}>
				<X class="h-3.5 w-3.5 mr-1" />
				{t('moderation.dismiss')}
			</Button>
		</div>
	{/if}

	{#if loading}
		<Card>
			<CardContent class="py-12">
				<div class="flex items-center justify-center text-gray-500">
					<RefreshCw class="h-5 w-5 mr-2 animate-spin" />
					{t('common.loading')}
				</div>
			</CardContent>
		</Card>
	{:else}
		<Card>
			<CardContent class="p-0">
				<DataTable
					{columns}
					data={items}
					pageSize={20}
					selectable={true}
					emptyMessage={t('moderation.empty')}
				/>
			</CardContent>
		</Card>
	{/if}

	<div class="flex items-center justify-between">
		<p class="text-sm text-gray-500">Page {page + 1} of {totalPages}</p>
		<div class="flex gap-1">
			<Button
				variant="outline"
				size="sm"
				onclick={() => { page = page - 1; load(); }}
				disabled={page === 0}
			>
				&larr;
			</Button>
			<Button
				variant="outline"
				size="sm"
				onclick={() => { page = page + 1; load(); }}
				disabled={page >= totalPages - 1}
			>
				&rarr;
			</Button>
		</div>
	</div>
</div>

<Modal
	bind:open={showActionModal}
	title={actionType === 'approve'
		? t('moderation.approve_title')
		: actionType === 'reject'
			? t('moderation.reject_title')
			: t('moderation.delete_title')}
	size="sm"
>
	<p class="text-sm text-gray-500 mb-3">
		{actionType === 'approve'
			? 'Approve this content and mark the report as resolved.'
			: actionType === 'reject'
				? 'Reject this content and dismiss the report.'
				: 'Permanently delete this content. This cannot be undone.'}
	</p>
	{#if actionItem}
		<div class="rounded-lg bg-gray-50 p-4 mb-3">
			<p class="text-sm font-medium text-gray-700">{actionItem.content_type}</p>
			<p class="text-sm text-gray-500">{actionItem.reason}</p>
		</div>
	{/if}
	{#if actionType !== 'approve'}
		<div class="mt-2">
			<label for="action-reason" class="block text-sm font-medium text-gray-700">
				{actionType === 'reject' ? 'Rejection reason' : 'Deletion reason'} (optional)
			</label>
			<textarea
				id="action-reason"
				rows="3"
				class="mt-1 block w-full rounded-lg border border-gray-300 px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500"
				placeholder="Enter a reason..."
				bind:value={actionReason}
			></textarea>
		</div>
	{/if}

	<div class="mt-6 flex justify-end gap-3">
		<Button variant="outline" onclick={() => (showActionModal = false)}>Cancel</Button>
		<Button
			variant={actionType === 'approve' ? 'default' : 'destructive'}
			onclick={executeAction}
			disabled={acting}
		>
			{#if acting}
				<RefreshCw class="h-4 w-4 mr-2 animate-spin" />
				Processing...
			{:else if actionType === 'approve'}
				<Check class="h-4 w-4 mr-2" />
				Approve
			{:else if actionType === 'reject'}
				<X class="h-4 w-4 mr-2" />
				Reject
			{:else}
				Delete
			{/if}
		</Button>
	</div>
</Modal>
