<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';

	import { Select } from 'bits-ui';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { actionColorClass } from '$lib/constants/audit';
	import { escapeHtml } from '$lib/utils';
	import type { AuditLogEntry, PaginatedResponse, DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';
	import { Trash2 } from 'lucide-svelte';


	let entries = $state<AuditLogEntry[]>([]);
	let page = $state(0);
	let pageSize = $state(10);
	let totalRows = $state(0);
	let loading = $state(true);
	let apiAvailable = $state(true);
	let message = $state('');
	let deleteModalOpen = $state(false);
	let pendingDeleteId = $state<string | null>(null);
	let clearAllModalOpen = $state(false);

	let sortKey = $state('timestamp');
	let sortDirection = $state<'asc' | 'desc'>('desc');

	let filterActor = $state('');
	let filterAction = $state('all');
	let filterTarget = $state('all');
	let filterFrom = $state('');
	let filterTo = $state('');

	let actionOptions = $state<string[]>([]);
	let targetOptions = $state<string[]>([]);

	const actionTypes = $derived([
		{ value: 'all', label: t('audit.action_all', 'All Actions') },
		...actionOptions.map((a) => ({ value: a, label: a }))
	]);

	const targetTypes = $derived([
		{ value: 'all', label: t('audit.target_all', 'All Target Types') },
		...targetOptions.map((tt) => ({ value: tt, label: tt }))
	]);

	interface AuditLogFilterOptions {
		action_types: string[];
		target_types: string[];
	}

	async function loadFilters() {
		try {
			const result = await api<AuditLogFilterOptions>('/control/audit-logs/filters');
			if (result.ok) {
				actionOptions = result.data.action_types ?? [];
				targetOptions = result.data.target_types ?? [];
			}
		} catch {
			// Filter dropdowns fall back to empty (user can still free-type actor/date).
		}
	}

	function formatTimestamp(val: unknown): string {
		if (!val) return '—';
		try {
			return new Date(String(val)).toLocaleString();
		} catch {
			return String(val);
		}
	}

	function formatActor(val: unknown): string {
		const actor = val as AuditLogEntry['actor'];
		if (!actor) return '—';
		return actor.username || actor.id || '—';
	}

	function formatMetadata(row: Record<string, unknown>): string {
		const meta = row.metadata as Record<string, unknown> | undefined;
		if (!meta || Object.keys(meta).length === 0) return '';
		// Surface the most useful metadata fields in priority order.
		const parts: string[] = [];
		if (meta.email) parts.push(`email: ${meta.email}`);
		if (meta.target_id) parts.push(`target: ${String(meta.target_id).slice(0, 8)}...`);
		if (meta.admin_id) parts.push(`admin: ${String(meta.admin_id).slice(0, 8)}...`);
		// Include any remaining keys not already shown.
		for (const [k, v] of Object.entries(meta)) {
			if (['email', 'target_id', 'admin_id', 'ip'].includes(k)) continue;
			const vs = typeof v === 'string' ? v : JSON.stringify(v);
			parts.push(`${k}: ${vs.slice(0, 30)}`);
		}
		return parts.join(' · ');
	}

	const columns: DataTableColumn[] = [
		{
			key: 'timestamp',
			label: t('audit.timestamp', 'Timestamp'),
			sortable: true,
			render: (val: unknown) =>
				`<span class="font-mono text-xs">${escapeHtml(formatTimestamp(val))}</span>`
		},
		{
			key: 'actor',
			label: t('audit.actor', 'Actor'),
			render: (val: unknown, row: Record<string, unknown>) => {
				const actor = val as AuditLogEntry['actor'];
				if (actor) {
					return `<span class="font-medium">${escapeHtml(actor.username)}</span>
						<span class="text-muted-foreground ml-1 text-xs">${escapeHtml(actor.id?.slice(0, 8))}...</span>`;
				}
				// No actor (e.g. failed login) — fall back to the attempted email
				// recorded in metadata so the column is not uselessly empty.
				const meta = row.metadata as Record<string, unknown> | undefined;
				const attemptedEmail = meta?.email;
				if (attemptedEmail) {
					return `<span class="font-medium">${escapeHtml(String(attemptedEmail))}</span>
						<span class="text-muted-foreground ml-1 text-xs italic">(attempted)</span>`;
				}
				return `<span class="text-muted-foreground">—</span>`;
			}
		},
		{
			key: 'action_type',
			label: t('audit.action', 'Action'),
			sortable: true,
			render: (val: unknown) => {
				const action = String(val ?? '');
				const color = actionColorClass(action);
				return `<span class="${color} rounded-full px-2 py-0.5 text-xs font-medium">${escapeHtml(action)}</span>`;
			}
		},
		{
			key: 'entity_type',
			label: t('audit.target_type', 'Target Type'),
			render: (val: unknown) =>
				`<span class="text-muted-foreground text-xs">${val ? escapeHtml(val) : '—'}</span>`
		},
		{
			key: 'entity_id',
			label: t('audit.target_id', 'Target ID'),
			render: (val: unknown) => {
				const s = val ? String(val) : '';
				const truncated = s.length > 12 ? s.slice(0, 12) + '...' : s;
				return `<span class="font-mono text-xs">${escapeHtml(truncated || '—')}</span>`;
			}
		},
		{
			key: 'ip_address',
			label: t('audit.ip_address', 'IP Address'),
			render: (val: unknown) => {
				const s = val ? String(val) : '';
				return `<span class="font-mono text-xs">${escapeHtml(s || '—')}</span>`;
			}
		},
		{
			key: 'metadata',
			label: t('audit.details', 'Details'),
			render: (_val: unknown, row: Record<string, unknown>) => {
				const summary = formatMetadata(row);
				return `<span class="text-muted-foreground text-xs">${escapeHtml(summary || '—')}</span>`;
			}
		}
	];

	const rowActions: DataTableRowAction[] = [
		{
			label: t('audit.clear_entry', 'Clear'),
			icon: Trash2,
			variant: 'destructive',
			onClick: (row) => {
				pendingDeleteId = (row as unknown as AuditLogEntry).id;
				deleteModalOpen = true;
			}
		}
	];

	const bulkActions: DataTableBulkAction[] = [
		{
			label: t('audit.clear_all', 'Clear All'),
			icon: Trash2,
			variant: 'destructive',
			onClick: () => {
				clearAllModalOpen = true;
			}
		}
	];

	async function deleteEntry(id: string) {
		try {
			const result = await api(`/control/audit-logs/${id}`, { method: 'DELETE' });
			if (result.ok) {
				message = '';
				load();
			} else {
				message = result.error || t('audit.clear_error', 'Failed to clear audit entry.');
			}
		} catch {
			message = t('audit.clear_error', 'Failed to clear audit entry.');
		}
	}

	async function clearAll() {
		try {
			// Backend POST /control/audit-logs/clear accepts {confirm?: bool, before?: RFC3339, action_type?: string}.
			// Without a scope it purges logs older than the retention window; {confirm:true} clears everything.
			const result = await api('/control/audit-logs/clear', {
				method: 'POST',
				body: { confirm: true }
			});
			if (result.ok) {
				message = '';
				clearAllModalOpen = false;
				load();
			} else {
				message = result.error || t('audit.clear_error', 'Failed to clear audit logs.');
			}
		} catch {
			message = t('audit.clear_error', 'Failed to clear audit logs.');
		}
	}

	function buildQuery() {
		const params = new URLSearchParams({
			page: String(page),
			per_page: String(pageSize)
		});
		if (filterActor) params.set('actor', filterActor);
		if (filterAction !== 'all') params.set('action_type', filterAction);
		if (filterTarget !== 'all') params.set('entity_type', filterTarget);
		if (filterFrom) params.set('from', filterFrom);
		if (filterTo) params.set('to', filterTo);
		if (sortKey) {
			params.set('sort_by', sortKey);
			params.set('sort_dir', sortDirection);
		}
		return params.toString();
	}

	async function load() {
		loading = true;
		try {
			const result = await api<PaginatedResponse<AuditLogEntry> | AuditLogEntry[]>(
				`/control/audit-logs?${buildQuery()}`
			);
			if (result.ok) {
				const raw = result.data;
				if (Array.isArray(raw)) {
					entries = raw;
					totalRows = raw.length;
				} else {
					entries = raw.data ?? [];
					totalRows = raw.total ?? entries.length;
				}
				apiAvailable = true;
			} else {
				apiAvailable = false;
			}
		} catch {
			apiAvailable = false;
		} finally {
			loading = false;
		}
	}

	function applyFilters() {
		page = 0;
		load();
	}

	function handleFilterKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') applyFilters();
	}

	let prevPage = $state(0);
	let prevPageSize = $state(10);
	let prevSortKey = $state('timestamp');
	let prevSortDirection = $state<'asc' | 'desc'>('desc');

	let initialized = false;
	$effect(() => {
		if (!initialized) {
			initialized = true;
			loadFilters();
			load();
			return;
		}
		if (
			page !== prevPage ||
			pageSize !== prevPageSize ||
			sortKey !== prevSortKey ||
			sortDirection !== prevSortDirection
		) {
			prevPage = page;
			prevPageSize = pageSize;
			prevSortKey = sortKey;
			prevSortDirection = sortDirection;
			if (apiAvailable) load();
		}
	});
</script>

<PageHeader title={t('audit.title', 'Audit Log')} description={t('audit.description', 'View system activity and user actions')} />

{#if !apiAvailable}
	<div class="border-tertiary bg-tertiary/10 text-tertiary mb-4 flex items-center gap-2 rounded-lg border px-4 py-3 text-sm">
		{t('audit.unavailable', 'Audit log is currently unavailable.')}
	</div>
{/if}

{#if message}
	<div class="border-destructive bg-destructive/10 text-destructive mb-4 rounded-lg border px-4 py-3 text-sm">
		{message}
	</div>
{/if}

<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
	<input
		type="text"
		placeholder={t('audit.actor_placeholder', 'Filter by actor...')}
		class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
		bind:value={filterActor}
		onkeydown={handleFilterKeydown}
		aria-label={t('audit.actor_placeholder', 'Filter by actor...')}
	/>
	<!-- H3: Select and date filters auto-apply on change (filter-on-change
	     per DataTable spec). Text inputs keep Enter-to-apply to avoid
	     fetching on every keystroke. -->
	<Select.Root type="single" bind:value={filterAction} items={actionTypes} onValueChange={() => applyFilters()}>
		<Select.Trigger
			class="border-border-input bg-background data-placeholder:text-muted-foreground/50 rounded-9px flex h-10 w-full items-center border px-3 text-sm transition-colors hover:border-border-input-hover"
			aria-label={t('audit.action_placeholder', 'Filter by action')}
		>
			<Select.Value placeholder={t('audit.action_all', 'All Actions')} />
		</Select.Trigger>
		<Select.Portal>
			<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-xl border px-1 py-1" sideOffset={4}>
				<Select.Viewport class="p-1">
					{#each actionTypes as item}
						<Select.Item
							class="data-highlighted:bg-accent rounded-9px flex h-9 cursor-pointer items-center px-3 text-sm outline-none"
							value={item.value}
							label={item.label}
						>
							{#snippet children({ selected })}
								{item.label}
								{#if selected}<span class="ml-auto">&#10003;</span>{/if}
							{/snippet}
						</Select.Item>
					{/each}
				</Select.Viewport>
			</Select.Content>
		</Select.Portal>
	</Select.Root>
	<Select.Root type="single" bind:value={filterTarget} items={targetTypes} onValueChange={() => applyFilters()}>
		<Select.Trigger
			class="border-border-input bg-background data-placeholder:text-muted-foreground/50 rounded-9px flex h-10 w-full items-center border px-3 text-sm transition-colors hover:border-border-input-hover"
			aria-label={t('audit.target_placeholder', 'Filter by target type')}
		>
			<Select.Value placeholder={t('audit.target_all', 'All Target Types')} />
		</Select.Trigger>
		<Select.Portal>
			<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-xl border px-1 py-1" sideOffset={4}>
				<Select.Viewport class="p-1">
					{#each targetTypes as item}
						<Select.Item
							class="data-highlighted:bg-accent rounded-9px flex h-9 cursor-pointer items-center px-3 text-sm outline-none"
							value={item.value}
							label={item.label}
						>
							{#snippet children({ selected })}
								{item.label}
								{#if selected}<span class="ml-auto">&#10003;</span>{/if}
							{/snippet}
						</Select.Item>
					{/each}
				</Select.Viewport>
			</Select.Content>
		</Select.Portal>
	</Select.Root>
	<input
		type="date"
		class="border-border-input bg-background text-foreground rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
		bind:value={filterFrom}
		onchange={applyFilters}
		aria-label="Filter from date"
	/>
	<input
		type="date"
		class="border-border-input bg-background text-foreground rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
		bind:value={filterTo}
		onchange={applyFilters}
		aria-label="Filter to date"
	/>
</div>
<div class="mb-4">
	<button
		class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-3 py-1.5 text-sm font-medium transition-colors"
		onclick={applyFilters}
	>
		{t('audit.apply_filters', 'Apply Filters')}
	</button>
</div>

<DataTable
	{columns}
	rows={entries}
	{totalRows}
	{loading}
	bind:page
	bind:pageSize
	bind:sortKey
	bind:sortDirection
	rowActions={rowActions}
	bulkActions={bulkActions}
	selectable={true}
	emptyMessage={t('audit.empty', 'No audit entries found')}
/>

<ConfirmModal
	bind:open={deleteModalOpen}
	title={t('audit.clear_title', 'Clear Audit Entry')}
	consequence={t('audit.clear_confirm', 'Permanently clear this audit log entry? This cannot be undone.')}
	confirmText={t('audit.clear_confirm_text', 'CLEAR')}
	onConfirm={() => {
		if (pendingDeleteId) deleteEntry(pendingDeleteId);
		pendingDeleteId = null;
	}}
	onCancel={() => {
		pendingDeleteId = null;
	}}
/>

<ConfirmModal
	bind:open={clearAllModalOpen}
	title={t('audit.clear_all_title', 'Clear All Audit Logs')}
	consequence={t('audit.clear_all_confirm', 'Permanently clear ALL audit log entries? This cannot be undone.')}
	confirmText={t('audit.clear_all_confirm_text', 'CLEAR ALL')}
	onConfirm={() => {
		clearAll();
	}}
	onCancel={() => {
		clearAllModalOpen = false;
	}}
/>
