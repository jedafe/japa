<script lang="ts">
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { FilterBar } from '$lib/components/ui/filter-bar';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import Pagination from '$lib/components/ui/Pagination.svelte';
	import { TableSkeleton } from '$lib/components/ui/skeleton';
	import { RefreshCw, Trash2, Plus, Eye, Pencil, Rocket, Archive } from 'lucide-svelte';
	import { pagesApi, type AdminPage } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { goto } from '$app/navigation';
	import { hasPermission } from '$lib/stores/permissions.svelte';

	let pages = $state<AdminPage[]>([]);
	let loading = $state(true);
	let error = $state('');
	let searchQuery = $state('');
	let filterValues = $state<Record<string, string>>({});
	let selectedIds = $state<Set<string>>(new Set());

	// Pagination — ListPages returns a flat array, so we paginate client-side.
	let currentPage = $state(0);
	let pageSize = $state(25);

	// Modals
	let showViewModal = $state(false);
	let viewTarget = $state<AdminPage | null>(null);
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<AdminPage | null>(null);
	let showBulkDeleteConfirm = $state(false);
	let acting = $state(false);

	// Create modal
	let showCreateModal = $state(false);
	let newTitle = $state('');
	let newSlug = $state('');
	let creating = $state(false);
	let createError = $state('');

	const canManage = $derived(hasPermission('pages.manage'));
	const canPublish = $derived(hasPermission('pages.publish'));

	const statusFilterDefs = $state([
		{
			key: 'status',
			label: t('common.status', 'Status'),
			options: [
				{ value: '', label: t('common.all', 'All') },
				{ value: 'draft', label: t('common.draft', 'Draft') },
				{ value: 'published', label: t('common.published', 'Published') },
				{ value: 'archived', label: t('common.archived', 'Archived') },
			],
		},
	]);

	function slugify(value: string): string {
		return value
			.toLowerCase()
			.trim()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-+|-+$/g, '');
	}

	const columns = $derived([
		{ key: 'title', label: t('landing.title', 'Title'), sortable: true },
		{ key: 'slug', label: t('landing.slug', 'Slug'), sortable: true },
		{
			key: 'status',
			label: t('common.status', 'Status'),
			sortable: true,
			render: (v: string) => {
				const colors =
					v === 'published'
						? 'bg-success/15 text-success dark:text-success'
						: v === 'archived'
							? 'bg-muted/30 text-muted dark:text-muted'
							: 'bg-warning/15 text-warning dark:text-warning';
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors}">${v}</span>`;
			},
		},
		{
			key: 'updated_at',
			label: t('landing.last_updated', 'Last Updated'),
			sortable: true,
			render: (v: string) => (v ? new Date(v).toLocaleDateString() : '—'),
		},
		{
			key: 'actions',
			label: t('common.actions', 'Actions'),
			class: 'w-56',
			componentSnippet: ActionsCellSnippet,
		},
	]);

	async function loadPages() {
		loading = true;
		error = '';
		try {
			const params = new URLSearchParams({ page_type: 'landing' });
			if (searchQuery) params.set('q', searchQuery);
			if (filterValues.status) params.set('status', filterValues.status);
			const result = await pagesApi.list(params.toString());
			if (result.ok) {
				pages = result.data ?? [];
			} else {
				error = result.error ?? t('common.error', 'Failed to load landing pages');
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('common.error', 'Failed to load landing pages');
		} finally {
			loading = false;
		}
	}

	function handleSearch(values: Record<string, string>) {
		searchQuery = values.search ?? '';
		filterValues = { status: values.status ?? '' };
		currentPage = 0;
	}

	function handleClear() {
		searchQuery = '';
		filterValues = {};
		currentPage = 0;
	}

	function openView(page: AdminPage) {
		viewTarget = page;
		showViewModal = true;
	}

	function confirmDelete(page: AdminPage) {
		deleteTarget = page;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		acting = true;
		try {
			const result = await pagesApi.delete(deleteTarget.id);
			if (result.ok) {
				showDeleteConfirm = false;
				deleteTarget = null;
				loadPages();
			} else {
				error = result.error ?? t('common.error', 'Failed to delete page');
			}
		} catch {
			error = t('common.error', 'Failed to delete page');
		} finally {
			acting = false;
		}
	}

	async function togglePublish(page: AdminPage) {
		acting = true;
		try {
			const result =
				page.status === 'published'
					? await pagesApi.archive(page.id)
					: await pagesApi.publish(page.id);
			if (result.ok) {
				loadPages();
			} else {
				error = result.error ?? t('common.error', 'Failed to update page status');
			}
		} catch {
			error = t('common.error', 'Failed to update page status');
		} finally {
			acting = false;
		}
	}

	async function bulkDelete() {
		const ids = Array.from(selectedIds);
		if (ids.length === 0) return;
		acting = true;
		try {
			await Promise.all(ids.map((id) => pagesApi.delete(id)));
			selectedIds = new Set();
			loadPages();
		} catch {
			error = t('common.error', 'Failed to delete pages');
		} finally {
			acting = false;
		}
	}

	async function createLanding() {
		if (!newSlug.trim()) return;
		creating = true;
		createError = '';
		try {
			const slug = newSlug.trim() || slugify(newTitle);
			const result = await pagesApi.create({
				title: newTitle || slug,
				slug,
				page_type: 'landing',
			});
			if (result.ok) {
				showCreateModal = false;
				newSlug = '';
				newTitle = '';
				goto('/landing/' + result.data.slug);
			} else {
				createError = result.error ?? t('common.error', 'Failed to create landing page');
			}
		} catch {
			createError = t('common.error', 'Failed to create landing page');
		} finally {
			creating = false;
		}
	}

	// Re-fetch whenever the filter/pagination inputs change.
	$effect(() => {
		void searchQuery;
		void filterValues;
		loadPages();
	});

	// Reset bulk selection when the page changes.
	$effect(() => {
		void currentPage;
		selectedIds = new Set();
	});
</script>

<svelte:head>
	<title>{t('landing.title', 'Landing Pages')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('landing.title', 'Landing Pages')}
	description={t('landing.description', 'Manage standalone landing pages')}
	actions={[
		...(canManage
			? [
					{
						label: t('landing.create', 'Create Landing Page'),
						icon: Plus,
						variant: 'default' as const,
						onclick: () => {
							newTitle = '';
							newSlug = '';
							showCreateModal = true;
						},
					},
				]
			: []),
		{ label: t('common.refresh', 'Refresh'), icon: RefreshCw, variant: 'ghost' as const, onclick: loadPages },
	]}
/>

{#if error}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{error}
	</div>
{/if}

{#if selectedIds.size > 0}
	<div class="mb-4 flex items-center gap-3 rounded-lg p-3" style="background: var(--color-primary); color: white">
		<span class="text-sm font-medium"
			>{selectedIds.size} {t('landing.selected_count', 'selected')}</span
		>
		{#if canManage}
			<Button variant="ghost" size="sm" onclick={() => (showBulkDeleteConfirm = true)} disabled={acting}>
				<Trash2 class="mr-1 h-3.5 w-3.5" /> {t('landing.bulk_delete', 'Delete Selected')}
			</Button>
		{/if}
	</div>
{/if}

<Card>
	<CardHeader>
		<FilterBar filters={statusFilterDefs} onsearch={handleSearch} onclear={handleClear} />
	</CardHeader>
	<CardContent>
		{#if loading}
			<TableSkeleton rows={8} columns={columns.length} />
		{:else}
			<div aria-live="polite">
				<DataTable {columns} data={pages} selectable={canManage} bind:selectedIds />
			</div>
			<Pagination bind:page={currentPage} totalPages={Math.max(1, Math.ceil(pages.length / pageSize))} totalItems={pages.length} bind:pageSize />
		{/if}
	</CardContent>
</Card>

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t('landing.details', 'Landing Page Details')} size="default">
	{#if viewTarget}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('landing.title', 'Title')}
					</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.title}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('landing.slug', 'Slug')}
					</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.slug}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('common.status', 'Status')}
					</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
						{t('landing.last_updated', 'Last Updated')}
					</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
						{viewTarget.updated_at ? new Date(viewTarget.updated_at).toLocaleString() : '—'}
					</p>
				</div>
			</div>
		</div>
	{/if}
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t('landing.delete_title', 'Delete Landing Page')} size="sm">
	<p class="text-sm" style="color: var(--color-foreground-alt)">
		{t('landing.delete_confirm', 'Are you sure you want to delete this landing page? This action cannot be undone.')}
	</p>
	{#if deleteTarget}
		<p class="mt-2 text-sm font-medium" style="color: var(--color-foreground)">{deleteTarget.title}</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)} disabled={acting}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={executeDelete} disabled={acting}>
			{acting ? t('common.deleting', 'Deleting...') : t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>

<!-- Bulk Delete Confirmation -->
<Modal bind:open={showBulkDeleteConfirm} title={t('landing.bulk_delete_title', 'Delete Selected Pages')} size="sm">
	<p class="text-sm" style="color: var(--color-foreground-alt)">
		{t('landing.bulk_delete_confirm', 'Delete all selected landing pages? This action cannot be undone.')}
	</p>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showBulkDeleteConfirm = false)} disabled={acting}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={bulkDelete} disabled={acting}>
			{acting ? t('common.deleting', 'Deleting...') : t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>

<!-- Create Modal -->
<Modal bind:open={showCreateModal} title={t('landing.create', 'Create Landing Page')} size="default">
	<div class="space-y-3">
		<div>
			<label for="landing-title" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground-alt)">
				{t('landing.title', 'Title')}
			</label>
			<input
				id="landing-title"
				type="text"
				bind:value={newTitle}
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border-input); background-color: var(--color-background); color: var(--color-foreground)"
			/>
		</div>
		<div>
			<label for="landing-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground-alt)">
				{t('landing.slug', 'Slug')}
			</label>
			<input
				id="landing-slug"
				type="text"
				bind:value={newSlug}
				placeholder="homepage"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border-input); background-color: var(--color-background); color: var(--color-foreground)"
			/>
		</div>
		{#if createError}
			<p class="text-sm" style="color: var(--color-danger)">{createError}</p>
		{/if}
	</div>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showCreateModal = false)} disabled={creating}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button onclick={createLanding} disabled={creating || !newSlug.trim()}>
			{creating ? t('common.creating', 'Creating...') : t('common.create', 'Create')}
		</Button>
	</div>
</Modal>

{#snippet ActionsCellSnippet({ row }: { value: any; row: AdminPage })}
	<div class="flex items-center gap-3">
		<button
			type="button"
			onclick={() => openView(row)}
			class="text-sm font-medium hover:underline"
			style="color: var(--color-muted)"
			aria-label={t('common.view', 'View')}
		>
			<Eye class="h-4 w-4" />
		</button>
		<button
			type="button"
			onclick={() => goto('/landing/' + row.slug)}
			class="text-sm font-medium hover:underline"
			style="color: var(--color-primary)"
			aria-label={t('common.edit', 'Edit')}
		>
			<Pencil class="h-4 w-4" />
		</button>
		{#if canPublish}
			<button
				type="button"
				onclick={() => togglePublish(row)}
				disabled={acting}
				class="text-sm font-medium hover:underline disabled:opacity-50"
				style="color: var(--color-foreground-alt)"
				aria-label={row.status === 'published' ? t('landing.archive', 'Archive') : t('landing.publish', 'Publish')}
			>
				{#if row.status === 'published'}
					<Archive class="h-4 w-4" />
				{:else}
					<Rocket class="h-4 w-4" />
				{/if}
			</button>
		{/if}
		{#if canManage}
			<button
				type="button"
				onclick={() => confirmDelete(row)}
				disabled={acting}
				class="text-sm font-medium hover:underline disabled:opacity-50"
				style="color: var(--color-danger)"
				aria-label={t('common.delete', 'Delete')}
			>
				<Trash2 class="h-4 w-4" />
			</button>
		{/if}
	</div>
{/snippet}
