<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { onMount } from 'svelte';
	import { RefreshCw, Shield, Trash2, CheckCircle, Clock, XCircle, Star } from 'lucide-svelte';
	import { Select } from '$lib/components/ui/select';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Card, CardContent } from '$lib/components/ui/card';
	import Pagination from '$lib/components/ui/Pagination.svelte';
	import Modal from '$lib/components/Modal.svelte';

	type Review = {
		id: string;
		entity_type: string;
		entity_id: string;
		entity_name: string;
		rating: number;
		status: string;
		created_at: string;
	};

	let reviews = $state<Review[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let searchQuery = $state('');
	let statusFilter = $state('');
	let entityTypeFilter = $state('');
	let currentPage = $state(0);
	let totalItems = $state(0);
	let totalPages = $state(1);
	let pageSize = $state(25);

	let showViewModal = $state(false);
	let viewTarget = $state<Review | null>(null);

	let showModerateModal = $state(false);
	let moderateTarget = $state<Review | null>(null);
	let moderateStatus = $state('published');
	let moderateSaving = $state(false);

	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<Review | null>(null);
	let deleteSaving = $state(false);

	let showHardDeleteConfirm = $state(false);
	let hardDeleteTarget = $state<Review | null>(null);
	let hardDeleteSaving = $state(false);

	const canModerate = $derived(hasPermission('reviews.moderate'));
	const canDelete = $derived(hasPermission('reviews.delete_own'));

	const statusTabs = [
		{ value: '', label: t('reviews.all', 'All'), icon: Star },
		{ value: 'published', label: t('reviews.status_published', 'Published'), icon: CheckCircle },
		{ value: 'flagged', label: t('reviews.status_flagged', 'Flagged'), icon: Clock },
		{ value: 'removed', label: t('reviews.status_removed', 'Removed'), icon: XCircle }
	];

	const columns = [
		{ key: 'entity_type', label: t('reviews.entity_type'), sortable: true },
		{ key: 'entity_name', label: t('reviews.entity_name', 'Entity'), sortable: true },
		{ key: 'rating', label: t('reviews.rating'), sortable: true,
			render: (v: number) => `${v}/5`
		},
		{ key: 'status', label: t('reviews.status'), sortable: true,
			render: (v: string) => {
				const cls = v === 'published' ? 'bg-success/15 text-success' : v === 'flagged' ? 'bg-warning/15 text-warning' : 'bg-danger/15 text-danger';
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${v}</span>`;
			}
		},
		{ key: 'created_at', label: t('reviews.created'), sortable: true,
			render: (v: string) => new Date(v).toLocaleDateString()
		},
		{ key: 'actions', label: t('common.actions', 'Actions'), class: 'w-36', componentSnippet: ReviewActionsCell }
	];

	function openView(review: Review) {
		viewTarget = review;
		showViewModal = true;
	}

	function openModerate(review: Review) {
		moderateTarget = review;
		moderateStatus = review.status ?? 'published';
		showModerateModal = true;
	}

	function confirmDelete(review: Review) {
		deleteTarget = review;
		showDeleteConfirm = true;
	}

	function confirmHardDelete(review: Review) {
		hardDeleteTarget = review;
		showHardDeleteConfirm = true;
	}

	async function saveModeration() {
		if (!moderateTarget) return;
		moderateSaving = true;
		try {
			const result = await api(`/admin/reviews/${moderateTarget.id}`, {
				method: 'PATCH',
				body: { status: moderateStatus }
			});
			if (result.ok) {
				showModerateModal = false;
				await load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			moderateSaving = false;
		}
	}

	async function deleteReview() {
		if (!deleteTarget) return;
		deleteSaving = true;
		try {
			const result = await api(`/admin/reviews/${deleteTarget.id}`, {
				method: 'PATCH',
				body: { status: 'removed' }
			});
			if (result.ok) {
				showDeleteConfirm = false;
				await load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			deleteSaving = false;
		}
	}

	async function hardDeleteReview() {
		if (!hardDeleteTarget) return;
		hardDeleteSaving = true;
		try {
			const result = await api(`/admin/reviews/${hardDeleteTarget.id}`, { method: 'DELETE' });
			if (result.ok) {
				showHardDeleteConfirm = false;
				await load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			hardDeleteSaving = false;
		}
	}

	function handleStatusTab(tab: string) {
		statusFilter = tab;
		currentPage = 0;
		load();
	}

	function handleEntityTypeFilter(val: string) {
		entityTypeFilter = val;
		currentPage = 0;
		load();
	}

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(currentPage * pageSize)
			});
			if (statusFilter) params.set('status', statusFilter);
			if (entityTypeFilter) params.set('entity_type', entityTypeFilter);

			const result = await api<{ reviews: Review[]; total: number }>(`/admin/reviews?${params.toString()}`);
			if (result.ok) {
				reviews = result.data?.reviews ?? [];
				totalItems = result.data?.total ?? 0;
				totalPages = Math.max(1, Math.ceil(totalItems / pageSize));
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('reviews.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('reviews.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('reviews.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<!-- Status filter tabs -->
<div class="mb-4 flex gap-1 rounded-lg p-1" style="background: var(--color-surface)">
	{#each statusTabs as tab}
		{@const Icon = tab.icon}
		<button
			class="inline-flex items-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-all"
			style={statusFilter === tab.value
				? "background: var(--color-background); color: var(--color-foreground); box-shadow: 0 1px 2px rgba(0,0,0,0.05)"
				: "color: var(--color-muted)"}
			onclick={() => handleStatusTab(tab.value)}
		>
			<Icon class="mr-2 h-4 w-4" />
			{tab.label}
		</button>
	{/each}

	<div class="ml-auto flex items-center gap-2">
		<div class="w-44">
			<Select
				options={[
					{ value: '', label: t('reviews.all_types', 'All types') },
					{ value: 'organization', label: t('reviews.type_organization', 'Organization') },
					{ value: 'university', label: t('reviews.type_university', 'University') },
					{ value: 'service', label: t('reviews.type_service', 'Service') },
					{ value: 'mentor', label: t('reviews.type_mentor', 'Mentor') }
				]}
				value={entityTypeFilter}
				onchange={handleEntityTypeFilter}
			/>
		</div>
		<button class="btn btn-secondary flex items-center gap-1.5 text-xs" onclick={load}>
			<RefreshCw class="h-3 w-3" />
			{t('common.refresh')}
		</button>
	</div>
</div>

<Card>
	<CardContent>
		{#if loading}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
		{:else}
			<DataTable
				{columns}
				data={reviews}
				emptyMessage={t('reviews.empty')}
			/>
			<Pagination bind:page={currentPage} {totalPages} {totalItems} bind:pageSize />
		{/if}
	</CardContent>
</Card>

<Modal bind:open={showViewModal} title={t('reviews.details', 'Review Details')}>
	{#if viewTarget}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reviews.entity_type', 'Entity Type')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.entity_type}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reviews.entity_name', 'Entity')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.entity_name}</p>
			</div>
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reviews.rating', 'Rating')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.rating}/5</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reviews.status', 'Status')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.status}</p>
				</div>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reviews.created', 'Created')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(viewTarget.created_at).toLocaleString()}</p>
			</div>
		</div>
	{/if}
</Modal>

<Modal bind:open={showModerateModal} title={t('reviews.moderate', 'Moderate Review')} onConfirm={saveModeration} confirmLabel={moderateSaving ? t('common.saving', 'Saving…') : t('common.save', 'Save')}>
	{#if moderateTarget}
		<div class="space-y-4">
			<p class="text-sm" style="color: var(--color-muted)">{t('reviews.moderate_help', 'Set the moderation status for this review.')}</p>
			<div>
				<label for="moderateStatus" class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reviews.status', 'Status')}</label>
				<Select
					options={[
						{ value: "published", label: t('reviews.status_published', 'Published') },
						{ value: "flagged", label: t('reviews.status_flagged', 'Flagged') },
						{ value: "removed", label: t('reviews.status_removed', 'Removed') }
					]}
					bind:value={moderateStatus}
				/>
			</div>
		</div>
	{/if}
</Modal>

<Modal bind:open={showDeleteConfirm} title={t('reviews.delete_title', 'Delete Review')} onConfirm={deleteReview} confirmLabel={deleteSaving ? t('common.deleting', 'Deleting…') : t('common.delete', 'Delete')}>
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">{t('reviews.delete_confirm', 'Are you sure you want to delete this review? This action cannot be undone.')}</p>
	{/if}
</Modal>

<Modal bind:open={showHardDeleteConfirm} title={t('reviews.hard_delete_title', 'Permanently Delete Review')} onConfirm={hardDeleteReview} confirmLabel={hardDeleteSaving ? t('common.deleting', 'Deleting…') : t('reviews.hard_delete', 'Hard Delete')}>
	{#if hardDeleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">{t('reviews.hard_delete_confirm', 'This will permanently remove the review from the database. This action cannot be undone.')}</p>
	{/if}
</Modal>

{#snippet ReviewActionsCell({ value, row }: { value: any; row: Review })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(row)}>
			{t('common.view', 'View')}
		</Button>
		{#if canModerate}
			<Button variant="ghost" size="sm" onclick={() => openModerate(row)} style="color: var(--color-primary)">
				<Shield class="inline h-3 w-3" />
				{t('reviews.moderate', 'Moderate')}
			</Button>
		{/if}
		{#if canModerate && row.status === 'removed'}
			<Button variant="ghost" size="sm" onclick={() => confirmHardDelete(row)} style="color: var(--color-danger)">
				<Trash2 class="inline h-3 w-3" />
				{t('reviews.hard_delete', 'Hard Delete')}
			</Button>
		{:else if canDelete}
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)">
				<Trash2 class="inline h-3 w-3" />
				{t('common.delete', 'Delete')}
			</Button>
		{/if}
	</div>
{/snippet}
