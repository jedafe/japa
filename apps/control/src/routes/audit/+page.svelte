<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import { Select } from 'bits-ui';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n';
	import { escapeHtml } from '$lib/utils';
	import type { AuditLogEntry, PaginatedResponse, DataTableColumn } from '$lib/api/types';

	let entries = $state<AuditLogEntry[]>([]);
	let page = $state(0);
	let pageSize = $state(10);
	let totalRows = $state(0);
	let loading = $state(true);
	let apiAvailable = $state(true);

	let sortKey = $state('timestamp');
	let sortDirection = $state<'asc' | 'desc'>('desc');

	let filterActor = $state('');
	let filterAction = $state('all');
	let filterTarget = $state('');
	let filterFrom = $state('');
	let filterTo = $state('');

	const actionTypes = [
		{ value: 'all', label: t('audit.action_all', 'All Actions') },
		{ value: 'create', label: t('audit.action_create', 'Create') },
		{ value: 'update', label: t('audit.action_update', 'Update') },
		{ value: 'delete', label: t('audit.action_delete', 'Delete') },
		{ value: 'login', label: t('audit.action_login', 'Login') },
		{ value: 'logout', label: t('audit.action_logout', 'Logout') }
	];

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
			render: (val: unknown) => {
				const actor = val as AuditLogEntry['actor'];
				if (!actor) return `<span class="text-muted-foreground">—</span>`;
				return `<span class="font-medium">${escapeHtml(actor.username)}</span>
					<span class="text-muted-foreground ml-1 text-xs">${escapeHtml(actor.id?.slice(0, 8))}...</span>`;
			}
		},
		{
			key: 'action_type',
			label: t('audit.action', 'Action'),
			sortable: true,
			render: (val: unknown) => {
				const action = escapeHtml(val);
				const color =
					action === 'delete'
						? 'bg-destructive/10 text-destructive'
						: action === 'create'
							? 'bg-green-500/10 text-green-600'
							: 'bg-accent text-accent-foreground';
				return `<span class="${color} rounded-full px-2 py-0.5 text-xs font-medium">${action}</span>`;
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
		}
	];

	function buildQuery() {
		const params = new URLSearchParams({
			page: String(page),
			per_page: String(pageSize)
		});
		if (filterActor) params.set('actor', filterActor);
		if (filterAction !== 'all') params.set('action_type', filterAction);
		if (filterTarget) params.set('entity_type', filterTarget);
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

	load();

	function handleFilterKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') applyFilters();
	}

	let prevPage = $state(0);
	let prevPageSize = $state(10);
	let prevSortKey = $state('timestamp');
	let prevSortDirection = $state<'asc' | 'desc'>('desc');

	$effect(() => {
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

<div class="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
	<input
		type="text"
		placeholder={t('audit.actor_placeholder', 'Filter by actor...')}
		class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
		bind:value={filterActor}
		onkeydown={handleFilterKeydown}
		aria-label={t('audit.actor_placeholder', 'Filter by actor...')}
	/>
	<Select.Root type="single" bind:value={filterAction} items={actionTypes}>
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
	<input
		type="text"
		placeholder={t('audit.target_placeholder', 'Filter by target type...')}
		class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
		bind:value={filterTarget}
		onkeydown={handleFilterKeydown}
		aria-label={t('audit.target_placeholder', 'Filter by target type...')}
	/>
	<input
		type="date"
		class="border-border-input bg-background text-foreground rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
		bind:value={filterFrom}
		aria-label="Filter from date"
	/>
	<input
		type="date"
		class="border-border-input bg-background text-foreground rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
		bind:value={filterTo}
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
	selectable={false}
	emptyMessage={t('audit.empty', 'No audit entries found')}
/>
