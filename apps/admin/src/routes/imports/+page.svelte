<script lang="ts">
	// /imports — Content Import oversight (Section 65, ADR-083).
	// Full-featured admin page matching the data-exports pattern: stat cards,
	// status tabs, DataTable with filters + row actions, view SlideOver with
	// per-item error log, retry action for failed/partial imports.
	// Gated by `import.manage`.
	//
	// Ref: japaconnect-engineering/sections/65-content-import.md
	import { t } from '$lib/stores/i18n.svelte';
	import { api, apiRaw } from '$lib/api';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Tabs } from '$lib/components/ui/tabs';
	import { DataTable } from '$lib/components/ui/data-table';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { Select } from '$lib/components/ui/select';
	import UserAutocomplete from '$lib/components/ui/UserAutocomplete.svelte';
	import {
		Download,
		Plus,
		RefreshCw,
		Clock,
		CheckCircle,
		XCircle,
		AlertTriangle,
		Eye,
		RotateCw,
		FileUp
	} from 'lucide-svelte';

	type ImportErrorEntry = {
		item_title?: string;
		url?: string;
		error?: string;
	};

	type ContentImport = {
		id: string;
		tenant_id: string;
		user_id: string;
		source_type: string;
		source_url?: string | null;
		status: 'pending' | 'processing' | 'completed' | 'failed' | 'partial';
		total_items: number;
		imported_count: number;
		failed_count: number;
		error_log: ImportErrorEntry[] | string;
		created_at: string;
		completed_at?: string | null;
		username?: string;
	};

	let imports = $state<ContentImport[]>([]);
	let loading = $state(true);
	let activeTab = $state('all');
	let showViewSlideOver = $state(false);
	let viewTarget = $state<ContentImport | null>(null);
	let retryConfirmOpen = $state(false);
	let retryingId = $state<string | null>(null);
	let saving = $state(false);
	let errorMessage = $state('');
	let successMessage = $state('');
	let selectedIds = $state<Set<string>>(new Set());

	const canManage = $derived(hasPermission('import.manage'));

	// New Import form state
	let createSlideOverOpen = $state(false);
	let selectedUserId = $state<string>('');
	let selectedUsername = $state('');
	let sourceType = $state('medium');
	let sourceUrl = $state('');

	const sourceTypeOptions = [
		{ value: 'medium', label: t('imports.source_medium', 'Medium') },
		{ value: 'substack', label: t('imports.source_substack', 'Substack') },
		{ value: 'rss', label: t('imports.source_rss', 'RSS Feed') },
		{ value: 'zip', label: t('imports.source_zip', 'ZIP Export') }
	];

	const requiresUrl = $derived(sourceType !== 'zip');

	let tabs = $derived([
		{
			value: 'all',
			label: t('imports.tab_all', 'All Imports'),
			icon: Download,
			count: imports.length
		},
		{
			value: 'active',
			label: t('imports.tab_in_progress', 'In Progress'),
			icon: Clock,
			count: imports.filter((i) => i.status === 'pending' || i.status === 'processing').length
		},
		{
			value: 'completed',
			label: t('imports.tab_completed', 'Completed'),
			icon: CheckCircle,
			count: imports.filter((i) => i.status === 'completed').length
		},
		{
			value: 'partial',
			label: t('imports.tab_partial', 'Partial'),
			icon: AlertTriangle,
			count: imports.filter((i) => i.status === 'partial').length
		},
		{
			value: 'failed',
			label: t('imports.tab_failed', 'Failed'),
			icon: XCircle,
			count: imports.filter((i) => i.status === 'failed').length
		}
	]);

	let filteredImports = $derived.by(() => {
		let result = [...imports];
		if (activeTab === 'active')
			result = result.filter((i) => i.status === 'pending' || i.status === 'processing');
		if (activeTab === 'completed') result = result.filter((i) => i.status === 'completed');
		if (activeTab === 'partial') result = result.filter((i) => i.status === 'partial');
		if (activeTab === 'failed') result = result.filter((i) => i.status === 'failed');
		return result;
	});

	const importFilters: FilterDef[] = $derived([
		{
			key: 'source_type',
			label: t('imports.filter_source', 'Source Type'),
			options: [
				{ value: 'medium', label: t('imports.source_medium', 'Medium') },
				{ value: 'substack', label: t('imports.source_substack', 'Substack') },
				{ value: 'rss', label: t('imports.source_rss', 'RSS') },
				{ value: 'zip', label: t('imports.source_zip', 'ZIP Export') }
			]
		}
	]);

	function statusVariant(status: string): string {
		switch (status) {
			case 'completed':
				return 'success';
			case 'failed':
				return 'destructive';
			case 'processing':
				return 'warning';
			case 'partial':
				return 'warning';
			default:
				return 'secondary';
		}
	}

	function statusBadgeClass(status: string): string {
		const variant = statusVariant(status);
		switch (variant) {
			case 'success':
				return 'bg-success/15 text-success border-success';
			case 'destructive':
				return 'bg-danger/15 text-danger border-danger';
			case 'warning':
				return 'bg-warning/15 text-warning border-warning';
			default:
				return 'bg-surface text-foreground border';
		}
	}

	function parseErrorLog(raw: ImportErrorEntry[] | string): ImportErrorEntry[] {
		if (Array.isArray(raw)) return raw;
		if (typeof raw === 'string' && raw.length > 0) {
			try {
				const parsed = JSON.parse(raw);
				return Array.isArray(parsed) ? parsed : [];
			} catch {
				return [];
			}
		}
		return [];
	}

	const importColumns = [
		{
			key: 'username',
			label: t('imports.col_user', 'User'),
			sortable: true,
			render: (v: string, row: ContentImport) => `
				<div>
					<p class="font-medium text-foreground">${v || 'unknown'}</p>
					<p class="text-xs text-muted">${row.source_type}</p>
				</div>
			`
		},
		{
			key: 'source_url',
			label: t('imports.col_source_url', 'Source URL'),
			render: (v: string | null | undefined) =>
				v
					? `<a href="${v}" target="_blank" rel="noopener" class="text-xs text-info truncate hover:underline">${v}</a>`
					: `<span class="text-xs text-muted">—</span>`
		},
		{
			key: 'status',
			label: t('imports.col_status', 'Status'),
			sortable: true,
			render: (value: string) => {
				const cls = statusBadgeClass(value);
				return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold capitalize ${cls}">${value}</span>`;
			}
		},
		{
			key: 'imported_count',
			label: t('imports.col_progress', 'Progress'),
			sortable: true,
			render: (v: number, row: ContentImport) => {
				const total = row.total_items || 0;
				const failed = row.failed_count || 0;
				const failedTag =
					failed > 0
						? ` <span class="text-xs text-danger">· ${failed} ${t('imports.failed', 'failed')}</span>`
						: '';
				return `<span class="text-sm text-foreground-alt">${v}/${total} ${t('imports.imported', 'imported')}${failedTag}</span>`;
			}
		},
		{
			key: 'created_at',
			label: t('imports.col_created', 'Created'),
			sortable: true,
			render: (value: string) =>
				`<span class="text-sm text-foreground-alt">${new Date(value).toLocaleString()}</span>`
		},
		{
			key: 'completed_at',
			label: t('imports.col_completed', 'Completed'),
			sortable: true,
			render: (value: string | null | undefined) =>
				value
					? `<span class="text-sm text-foreground-alt">${new Date(value).toLocaleString()}</span>`
					: `<span class="text-sm text-muted">—</span>`
		}
	];

	const importRowActions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row: ContentImport) => openView(row) },
		{
			label: t('imports.retry', 'Retry'),
			icon: RotateCw,
			show: (row: ContentImport) => row.status === 'failed' || row.status === 'partial',
			onclick: (row: ContentImport) => openRetryConfirm(row.id)
		}
	];

	async function loadImports() {
		loading = true;
		errorMessage = '';
		const result = await apiRaw<{ data: ContentImport[]; pagination: { total: number } }>(
			'/admin/imports?page=1&limit=100'
		);
		if (result.ok) {
			imports = Array.isArray(result.data.data) ? result.data.data : [];
		} else {
			errorMessage = result.error ?? t('imports.msg_load_failed', 'Failed to load imports');
		}
		loading = false;
	}

	function resetImportForm() {
		selectedUserId = '';
		selectedUsername = '';
		sourceType = 'medium';
		sourceUrl = '';
	}

	async function submitImport() {
		if (!selectedUserId) {
			errorMessage = t('imports.msg_select_user', 'Please select a user');
			return;
		}
		if (requiresUrl && !sourceUrl.trim()) {
			errorMessage = t('imports.msg_source_url_required', 'Source URL is required for this source type');
			return;
		}
		saving = true;
		errorMessage = '';
		const result = await api('/admin/imports', {
			method: 'POST',
			body: {
				user_id: selectedUserId,
				source_type: sourceType,
				source_url: requiresUrl ? sourceUrl.trim() : undefined
			}
		});
		if (result.ok) {
			createSlideOverOpen = false;
			resetImportForm();
			successMessage = t('imports.msg_import_queued', 'Import job queued successfully');
			loadImports();
		} else {
			errorMessage = result.error ?? t('imports.msg_import_failed', 'Failed to start import');
		}
		saving = false;
	}

	function openView(imp: ContentImport) {
		viewTarget = imp;
		showViewSlideOver = true;
	}

	function openRetryConfirm(id: string) {
		retryingId = id;
		retryConfirmOpen = true;
	}

	async function confirmRetry() {
		if (!retryingId) return;
		saving = true;
		errorMessage = '';
		const result = await api(`/admin/imports/${retryingId}/retry`, { method: 'POST' });
		if (result.ok) {
			successMessage = t('imports.msg_retry_queued', 'Import retry queued successfully');
			loadImports();
		} else {
			errorMessage = result.error ?? t('imports.msg_retry_failed', 'Failed to retry import');
		}
		retryConfirmOpen = false;
		retryingId = null;
		saving = false;
	}

	async function bulkRetry() {
		if (!selectedIds.size) return;
		saving = true;
		const ids = [...selectedIds];
		let ok = 0;
		let fail = 0;
		for (const id of ids) {
			const r = await api(`/admin/imports/${id}/retry`, { method: 'POST' });
			if (r.ok) ok++;
			else fail++;
		}
		selectedIds = new Set();
		successMessage = t('imports.msg_bulk_retry', `${ok} import(s) requeued, ${fail} failed`);
		loadImports();
		saving = false;
	}

	$effect(() => {
		loadImports();
	});
</script>

<svelte:head>
	<title>{t('imports.page_title', 'Content Imports — JapaConnect Admin')}</title>
</svelte:head>

<PageHeader
	title={t('imports.title', 'Content Imports')}
	description={t('imports.description', 'Oversight of all user content imports (Medium, Substack, RSS). Retry failed imports.')}
	actions={[
		{
			label: t('imports.action_new', 'New Import'),
			icon: Plus,
			onclick: () => (createSlideOverOpen = true)
		},
		{
			label: t('imports.action_refresh', 'Refresh'),
			icon: RefreshCw,
			variant: 'ghost',
			onclick: loadImports
		}
	]}
/>

<div class="space-y-6">
	{#if errorMessage}
		<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{errorMessage}
		</div>
	{/if}
	{#if successMessage}
		<div class="rounded-lg border border-success bg-success-50 px-4 py-3 text-sm text-success-700">
			{successMessage}
		</div>
	{/if}

	<!-- Summary Cards -->
	<div class="grid grid-cols-1 gap-4 sm:grid-cols-4">
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-muted">{t('imports.total_imports', 'Total Imports')}</p>
						<p class="text-2xl font-bold text-foreground">{imports.length}</p>
					</div>
					<div class="rounded-lg bg-primary-100 p-3">
						<FileUp class="h-6 w-6 text-primary-600" />
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-muted">{t('imports.in_progress', 'In Progress')}</p>
						<p class="text-2xl font-bold text-warning-600">
							{imports.filter((i) => i.status === 'pending' || i.status === 'processing').length}
						</p>
					</div>
					<div class="rounded-lg bg-warning/15 p-3">
						<Clock class="h-6 w-6 text-warning-600" />
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-muted">{t('imports.completed', 'Completed')}</p>
						<p class="text-2xl font-bold text-success-600">
							{imports.filter((i) => i.status === 'completed').length}
						</p>
					</div>
					<div class="rounded-lg bg-success/15 p-3">
						<CheckCircle class="h-6 w-6 text-success-600" />
					</div>
				</div>
			</CardContent>
		</Card>
		<Card>
			<CardContent class="p-6">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-medium text-muted">{t('imports.failed', 'Failed / Partial')}</p>
						<p class="text-2xl font-bold text-danger-600">
							{imports.filter((i) => i.status === 'failed' || i.status === 'partial').length}
						</p>
					</div>
					<div class="rounded-lg bg-danger/15 p-3">
						<XCircle class="h-6 w-6 text-danger-600" />
					</div>
				</div>
			</CardContent>
		</Card>
	</div>

	<Tabs {tabs} bind:value={activeTab} />

	<Card>
		<CardContent class="p-0">
			{#if loading}
				<p class="py-8 text-center text-sm text-muted">{t('imports.loading', 'Loading imports…')}</p>
			{:else}
				<DataTable
					columns={importColumns}
					data={filteredImports}
					filters={importFilters}
					actions={importRowActions}
					pageSize={20}
					emptyMessage={t('imports.empty', 'No content imports found')}
					selectable
					bind:selectedIds
				>
					{#snippet bulkActions()}
						{#if canManage}
							<Button variant="default" size="sm" onclick={bulkRetry} disabled={saving}>
								<RotateCw class="mr-1 h-3.5 w-3.5" />
								{t('imports.action_retry_selected', 'Retry Selected')} ({selectedIds.size})
							</Button>
						{/if}
					{/snippet}
				</DataTable>
			{/if}
		</CardContent>
	</Card>
</div>

<!-- View SlideOver -->
<SlideOver bind:open={showViewSlideOver} title={t('imports.modal_details_title', 'Import Details')}>
	{#if viewTarget}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('imports.detail_user', 'User')}
					</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
						{viewTarget.username || viewTarget.user_id}
					</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('imports.detail_source', 'Source')}
					</p>
					<p class="mt-1 text-sm font-medium capitalize" style="color: var(--color-foreground)">
						{viewTarget.source_type}
					</p>
				</div>
				{#if viewTarget.source_url}
					<div class="col-span-2">
						<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
							{t('imports.detail_source_url', 'Source URL')}
						</p>
						<p class="mt-1 break-all text-sm font-mono" style="color: var(--color-foreground)">
							{viewTarget.source_url}
						</p>
					</div>
				{/if}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('imports.detail_status', 'Status')}
					</p>
					<p class="mt-1">
						<span
							class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold capitalize {statusBadgeClass(viewTarget.status)}"
						>
							{viewTarget.status}
						</span>
					</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('imports.detail_progress', 'Progress')}
					</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">
						{viewTarget.imported_count}/{viewTarget.total_items} {t('imports.imported', 'imported')}
						· {viewTarget.failed_count} {t('imports.failed', 'failed')}
					</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('imports.detail_created', 'Created')}
					</p>
					<p class="mt-1 text-sm" style="color: var(--color-foreground)">
						{new Date(viewTarget.created_at).toLocaleString()}
					</p>
				</div>
				{#if viewTarget.completed_at}
					<div>
						<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
							{t('imports.detail_completed', 'Completed')}
						</p>
						<p class="mt-1 text-sm" style="color: var(--color-foreground)">
							{new Date(viewTarget.completed_at).toLocaleString()}
						</p>
					</div>
				{/if}
			</div>

			{#if parseErrorLog(viewTarget.error_log).length > 0}
				{@const errors = parseErrorLog(viewTarget.error_log)}
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('imports.detail_error_log', 'Error Log')} ({errors.length})
					</p>
					<ul class="mt-2 space-y-2">
						{#each errors as err, i (i)}
							<li class="rounded-md border p-2 text-xs" style="border-color: var(--color-border)">
								{#if err.item_title}
									<p class="font-medium" style="color: var(--color-foreground)">{err.item_title}</p>
								{/if}
								{#if err.url}
									<p class="mt-0.5 break-all font-mono" style="color: var(--color-muted)">{err.url}</p>
								{/if}
								<p class="mt-0.5" style="color: var(--color-danger)">{err.error}</p>
							</li>
						{/each}
					</ul>
				</div>
			{/if}
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end gap-3 pt-2">
			{#if viewTarget && (viewTarget.status === 'failed' || viewTarget.status === 'partial') && canManage}
				<Button
					onclick={() => {
						showViewSlideOver = false;
						openRetryConfirm(viewTarget!.id);
					}}
					disabled={saving}
				>
					<RotateCw class="mr-1 h-3.5 w-3.5" />
					{t('imports.retry', 'Retry')}
				</Button>
			{/if}
			<Button variant="outline" onclick={() => (showViewSlideOver = false)}>
				{t('imports.btn_close', 'Close')}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Create Import SlideOver -->
<SlideOver bind:open={createSlideOverOpen} title={t('imports.modal_new_title', 'New Content Import')}>
	<div class="space-y-4">
		{#if errorMessage}
			<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">
				{errorMessage}
			</div>
		{/if}
		<div>
			<label for="import-user" class="mb-1 block text-sm font-medium text-foreground-alt">
				{t('imports.target_user', 'Target User')}
			</label>
			<UserAutocomplete
				bind:value={selectedUsername}
				placeholder={t('imports.user_placeholder', 'Search by username…')}
				onSelectUser={(u) => { selectedUserId = u.id; }}
			/>
			{#if selectedUserId}
				<p class="mt-1 text-xs text-muted">ID: {selectedUserId}</p>
			{/if}
		</div>
		<div>
			<label for="import-source-type" class="mb-1 block text-sm font-medium text-foreground-alt">
				{t('imports.source_type', 'Source Type')}
			</label>
			<Select bind:value={sourceType} options={sourceTypeOptions} />
		</div>
		{#if requiresUrl}
			<div>
				<label for="import-source-url" class="mb-1 block text-sm font-medium text-foreground-alt">
					{t('imports.source_url', 'Source URL')}
				</label>
				<input
					id="import-source-url"
					type="url"
					bind:value={sourceUrl}
					placeholder="https://medium.com/@username"
					class="w-full rounded-lg border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					style="border-color: var(--color-border)"
				/>
				<p class="mt-1 text-xs text-muted">
					{t('imports.source_url_hint', 'The RSS feed URL or profile URL of the blog to import.')}
				</p>
			</div>
		{/if}
		<div class="rounded-lg border p-3 text-xs" style="border-color: var(--color-border); color: var(--color-muted)">
			{t('imports.form_help', 'Imported posts become draft blog_post content attributed to the selected user, with canonical_url set to the original source. The user can review and publish them.')}
		</div>
	</div>
	{#snippet footer()}
		<div class="flex justify-end gap-3 pt-2">
			<Button variant="outline" onclick={() => { createSlideOverOpen = false; resetImportForm(); }}>
				{t('imports.cancel', 'Cancel')}
			</Button>
			<Button onclick={submitImport} disabled={saving || !selectedUserId || (requiresUrl && !sourceUrl.trim())}>
				{saving ? t('imports.btn_queuing', 'Queuing…') : t('imports.btn_start', 'Start Import')}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Retry Confirmation Modal -->
<Modal
	bind:open={retryConfirmOpen}
	title={t('imports.modal_retry_title', 'Confirm Retry')}
	description={t('imports.modal_retry_description', 'This will re-process the import source')}
>
	<div class="space-y-4">
		<p class="text-sm text-foreground-alt">
			{t('imports.modal_retry_body', 'Are you sure you want to retry this import? The source will be re-parsed and new draft posts created for any items that previously failed.')}
		</p>
		<div class="flex justify-end gap-3">
			<Button variant="outline" onclick={() => (retryConfirmOpen = false)}>
				{t('imports.cancel', 'Cancel')}
			</Button>
			<Button onclick={confirmRetry} disabled={saving}>
				{saving ? t('imports.btn_retrying', 'Retrying…') : t('imports.btn_retry_import', 'Retry Import')}
			</Button>
		</div>
	</div>
</Modal>
