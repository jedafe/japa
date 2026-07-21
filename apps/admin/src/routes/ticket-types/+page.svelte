<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { FilterBar } from '$lib/components/ui/filter-bar';
	import { Button } from '$lib/components/ui/button';
	import { Select } from '$lib/components/ui/select';
	import { ticketTypesApi } from '$lib/api';
	import { hasPermission } from '$lib/stores/permissions.svelte';

	type TicketType = {
		id: string;
		name: string;
		slug: string;
		description: string;
		is_active: boolean;
		default_priority: string;
	};

	let ticketTypes = $state<TicketType[]>([]);
	let filteredTicketTypes = $state<TicketType[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let searchQuery = $state('');
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	let showModal = $state(false);
	let editingId = $state<string | null>(null);
	let saving = $state(false);
	let showDeleteConfirm = $state(false);
	let deleteTargetId = $state<string | null>(null);

	let form = $state({
		name: '',
		slug: '',
		description: '',
		default_priority: 'medium',
		is_active: true
	});

	const canManageTickets = $derived(hasPermission('tickets.review'));

	const filterDefs = [
		{
			key: 'is_active',
			label: t('app.ticket_types.filter_status', 'Status'),
			options: [
				{ value: 'true', label: t('app.ticket_types.filter_active', 'Active') },
				{ value: 'false', label: t('app.ticket_types.filter_inactive', 'Inactive') }
			]
		}
	];

	let filterValues = $state<Record<string, string>>({});

	function handleSearch(values: Record<string, string>) {
		searchQuery = values.search ?? '';
		filterValues = values;
		applySearch(searchQuery);
	}

	function handleClear() {
		searchQuery = '';
		filterValues = {};
		applySearch('');
	}

	const priorities = ['low', 'medium', 'high', 'urgent'];

	const columns = [
		{ key: 'name', label: t('ticket-types.name'), sortable: true },
		{ key: 'slug', label: t('ticket-types.slug'), sortable: true, class: 'font-mono text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${v}</span>` },
		{ key: 'description', label: t('ticket-types.description'), sortable: false, render: (v: string) => `<span style="color: var(--color-muted)">${v || '—'}</span>` },
		{ key: 'is_active', label: t('ticket-types.active'), sortable: true, render: (v: boolean) => v
			? `<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-success/15 text-success dark:text-success">${t('app.ticket_types.status_active', 'Active')}</span>`
			: `<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-muted/15 text-muted dark:text-muted">${t('app.ticket_types.status_inactive', 'Inactive')}</span>`
		},
		{ key: 'default_priority', label: t('ticket-types.priority'), sortable: true, render: (v: string) => {
			const cls = v === 'urgent' ? 'text-danger dark:text-danger' : v === 'high' ? 'text-warning dark:text-warning' : '';
			return `<span class="text-xs font-medium ${cls}">${v}</span>`;
		}},
		{ key: 'actions', label: '', sortable: false, render: (_v: unknown, row: TicketType) => {
			if (!canManageTickets) return '';
			return `<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" style="color: var(--color-info)" data-action="edit" data-id="${row.id}">${t('app.ticket_types.edit', 'Edit')}</button><button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" style="color: var(--color-danger)" data-action="delete" data-id="${row.id}">${t('app.ticket_types.delete', 'Delete')}</button>`;
		}}
	];

	function slugify(text: string): string {
		return text
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

	function applySearch(q: string) {
		searchQuery = q.toLowerCase();
		filteredTicketTypes = searchQuery
			? ticketTypes.filter(tt =>
				tt.name.toLowerCase().includes(searchQuery) ||
				tt.slug.toLowerCase().includes(searchQuery) ||
				(tt.description ?? '').toLowerCase().includes(searchQuery)
			)
			: ticketTypes;
	}

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams();
		if (filterValues.is_active) params.set('is_active', filterValues.is_active);
		const qs = params.toString();
		const result = await ticketTypesApi.list(qs || undefined);
		if (result.ok) {
			const data = result.data as { items: TicketType[]; total_pages: number };
			ticketTypes = data.items ?? [];
			applySearch(searchQuery);
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	function openCreate() {
		editingId = null;
		form = { name: '', slug: '', description: '', default_priority: 'medium', is_active: true };
		showModal = true;
	}

	function openEdit(item: TicketType) {
		editingId = item.id;
		form = {
			name: item.name,
			slug: item.slug,
			description: item.description ?? '',
			default_priority: item.default_priority,
			is_active: item.is_active
		};
		showModal = true;
	}

	async function submit() {
		if (!form.name.trim()) return;
		saving = true;
		errorMessage = '';

		const slug = form.slug.trim() || slugify(form.name);
		const body = {
			name: form.name.trim(),
			slug,
			description: form.description.trim(),
			default_priority: form.default_priority,
			is_active: form.is_active
		};

		const result = editingId
			? await ticketTypesApi.update(editingId, body)
			: await ticketTypesApi.create(body);

		if (result.ok) {
			showModal = false;
			load();
		} else {
			errorMessage = result.error;
		}
		saving = false;
	}

	function confirmDelete(id: string) {
		deleteTargetId = id;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTargetId) return;
		errorMessage = '';
		const result = await ticketTypesApi.delete(deleteTargetId);
		if (result.ok) {
			showDeleteConfirm = false;
			deleteTargetId = null;
			load();
		} else {
			errorMessage = result.error;
		}
	}

	async function bulkDelete() {
		if (selectedIds.size === 0) return;
		bulkDeleting = true;
		errorMessage = '';
		try {
			for (const id of selectedIds) {
				await ticketTypesApi.delete(id);
			}
			selectedIds = new Set();
			load();
		} catch {
			errorMessage = t('app.ticket_types.failed_to_delete_selected', 'Failed to delete selected items');
		}
		bulkDeleting = false;
	}

	function handleTableClick(e: MouseEvent) {
		const target = e.target as HTMLElement;
		const btn = target.closest('[data-action]') as HTMLElement | null;
		if (!btn) return;
		e.stopPropagation();
		const action = btn.dataset.action;
		const id = btn.dataset.id;
		if (!id) return;
		if (action === 'edit') {
			const item = ticketTypes.find(t => t.id === id);
			if (item) openEdit(item);
		} else if (action === 'delete') {
			confirmDelete(id);
		}
	}

	const tableData = $derived(filteredTicketTypes.map(item => ({ ...item })));

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('app.ticket_types.page_title', 'Ticket Types')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('ticket-types.ticket_types')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('ticket-types.manage_ticket_type_categories_and_their_')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
		onclick={openCreate}
	>
		{t('app.ticket_types.add_ticket_type', 'Add Ticket Type')}
	</button>
</div>

<FilterBar filters={filterDefs} onsearch={handleSearch} onclear={handleClear} />

<div role="presentation" onclick={handleTableClick} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') handleTableClick(e as unknown as MouseEvent); }}>
	<DataTable
		{columns}
		data={tableData}
		{loading}
		pageSize={25}
		emptyMessage={t('ticket-types.no_ticket_types_configured')}
		searchable={true}
		searchPlaceholder={t('ticket-types.search_ticket_types', 'Search ticket types...')}
		onsearch={applySearch}
		selectable
		bind:selectedIds
	>
		{#snippet bulkActions()}
			<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting || selectedIds.size === 0}>
				{t('app.ticket_types.delete_selected', 'Delete Selected')} ({selectedIds.size})
			</Button>
		{/snippet}
	</DataTable>
</div>

<!-- Create/Edit Modal -->
<Modal bind:open={showModal} title={editingId ? t('app.ticket_types.edit_ticket_type', 'Edit Ticket Type') : t('app.ticket_types.add_ticket_type', 'Add Ticket Type')} onConfirm={submit} confirmLabel={saving ? t('app.ticket_types.saving', 'Saving...') : (editingId ? t('app.ticket_types.update', 'Update') : t('app.ticket_types.create', 'Create'))}>
	<div class="space-y-3">
		<div>
			<label for="tt-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('ticket-types.name')}</label>
			<input id="tt-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="Bug Report" bind:value={form.name} oninput={() => { if (!editingId) form.slug = slugify(form.name); }} />
		</div>
		<div>
			<label for="tt-slug" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('ticket-types.slug')}</label>
			<input id="tt-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="bug-report" bind:value={form.slug} />
		</div>
		<div>
			<label for="tt-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('ticket-types.description')}</label>
			<textarea id="tt-desc" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" rows="2" bind:value={form.description}></textarea>
		</div>
		<div>
			<label for="tt-priority" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('ticket-types.default_priority')}</label>
			<Select
				options={priorities.map(p => ({ value: p, label: p }))}
				bind:value={form.default_priority}
			/>
		</div>
		<div class="flex items-center gap-2">
			<input id="tt-active" type="checkbox" bind:checked={form.is_active} />
			<label for="tt-active" class="text-sm">{t('ticket-types.active')}</label>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t('app.ticket_types.confirm_delete', 'Confirm Delete')} onConfirm={executeDelete} confirmLabel={t('app.ticket_types.delete', 'Delete')}>
	<p class="text-sm" style="color: var(--color-muted)">{t('ticket-types.are_you_sure_you_want_to_delete_this_tic')}</p>
</Modal>
