<script lang="ts">
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { RefreshCw, Shield, Trash2 } from 'lucide-svelte';
	import { Select } from '$lib/components/ui/select';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import Modal from '$lib/components/Modal.svelte';

	type Review = {
		id: string;
		entity_type: string;
		entity_id: string;
		rating: number;
		status: string;
		created_at: string;
	};

	let reviews = $state<Review[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let searchQuery = $state('');

	let showViewModal = $state(false);
	let viewTarget = $state<Review | null>(null);

	let showModerateModal = $state(false);
	let moderateTarget = $state<Review | null>(null);
	let moderateStatus = $state<'approved' | 'rejected' | 'pending'>('approved');
	let moderateSaving = $state(false);

	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<Review | null>(null);
	let deleteSaving = $state(false);

	const canModerate = $derived(hasPermission('reviews.moderate'));
	const canDelete = $derived(hasPermission('reviews.delete_own'));

	const filteredReviews = $derived(
		searchQuery
			? reviews.filter(
					(r) =>
						r.entity_type.toLowerCase().includes(searchQuery.toLowerCase()) ||
						r.entity_id.toLowerCase().includes(searchQuery.toLowerCase()) ||
						r.status.toLowerCase().includes(searchQuery.toLowerCase())
				)
			: reviews
	);

	const columns = [
		{ key: 'entity_type', label: t('reviews.entity_type'), sortable: true },
		{ key: 'entity_id', label: t('reviews.entity_id'), sortable: true,
			render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v.slice(0, 12)}</span>`
		},
		{ key: 'rating', label: t('reviews.rating'), sortable: true,
			render: (v: number) => `${v}/5`
		},
		{ key: 'status', label: t('reviews.status'), sortable: true,
			render: (v: string) => {
				const cls = v === 'approved' ? 'bg-success/15 text-success' : v === 'pending' ? 'bg-warning/15 text-warning' : 'bg-danger/15 text-danger';
				return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${v}</span>`;
			}
		},
		{ key: 'created_at', label: t('reviews.created'), sortable: true,
			render: (v: string) => new Date(v).toLocaleDateString()
		},
		{ key: 'actions', label: t('common.actions', 'Actions'), class: 'w-36', component: ReviewActionsCell }
	];

	function openView(review: Review) {
		viewTarget = review;
		showViewModal = true;
	}

	function openModerate(review: Review) {
		moderateTarget = review;
		moderateStatus = (review.status as 'approved' | 'rejected' | 'pending') ?? 'approved';
		showModerateModal = true;
	}

	function confirmDelete(review: Review) {
		deleteTarget = review;
		showDeleteConfirm = true;
	}

	async function saveModeration() {
		if (!moderateTarget) return;
		moderateSaving = true;
		try {
			const result = await api(`/admin/reviews/${moderateTarget.id}`, {
				method: 'PATCH',
				body: JSON.stringify({ status: moderateStatus })
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
			const result = await api(`/reviews/${deleteTarget.id}`, { method: 'DELETE' });
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

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<Review[]>('/admin/reviews');
			if (result.ok) {
				reviews = result.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	$effect(() => {
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

<div class="mb-4 flex justify-end">
	<button class="btn btn-secondary flex items-center gap-1.5 text-xs" onclick={load}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<DataTable
		{columns}
		data={filteredReviews}
		pageSize={25}
		searchable
		searchPlaceholder={t('reviews.search', 'Search reviews...')}
		onsearch={(q) => { searchQuery = q; }}
		emptyMessage={t('reviews.empty')}
	/>
{/if}

<Modal bind:open={showViewModal} title={t('reviews.details', 'Review Details')}>
	{#if viewTarget}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reviews.entity_type', 'Entity Type')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.entity_type}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reviews.entity_id', 'Entity ID')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.entity_id}</p>
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
	<div class="mt-6 flex justify-end">
		<button class="rounded-md px-4 py-2 text-sm font-medium transition-colors" style="border: 1px solid var(--color-border)" onclick={() => (showViewModal = false)}>{t('common.close', 'Close')}</button>
	</div>
</Modal>

<Modal bind:open={showModerateModal} title={t('reviews.moderate', 'Moderate Review')}>
	{#if moderateTarget}
		<div class="space-y-4">
			<p class="text-sm" style="color: var(--color-muted)">{t('reviews.moderate_help', 'Set the moderation status for this review.')}</p>
			<div>
				<label for="moderateStatus" class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reviews.status', 'Status')}</label>
				<Select
					options={[
						{ value: "approved", label: t('reviews.status_approved', 'Approved') },
						{ value: "pending", label: t('reviews.status_pending', 'Pending') },
						{ value: "rejected", label: t('reviews.status_rejected', 'Rejected') }
					]}
					value={moderateStatus}
					onchange={(val) => (moderateStatus = val as typeof moderateStatus)}
				/>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<button class="rounded-md px-4 py-2 text-sm font-medium transition-colors" style="border: 1px solid var(--color-border)" onclick={() => (showModerateModal = false)}>{t('common.cancel', 'Cancel')}</button>
		<button class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50" style="background-color: var(--color-primary)" onclick={saveModeration} disabled={moderateSaving}>
			{moderateSaving ? t('common.saving', 'Saving…') : t('common.save', 'Save')}
		</button>
	</div>
</Modal>

<Modal bind:open={showDeleteConfirm} title={t('reviews.delete_title', 'Delete Review')}>
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">{t('reviews.delete_confirm', 'Are you sure you want to delete this review? This action cannot be undone.')}</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<button class="rounded-md px-4 py-2 text-sm font-medium transition-colors" style="border: 1px solid var(--color-border)" onclick={() => (showDeleteConfirm = false)}>{t('common.cancel', 'Cancel')}</button>
		<button class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50" style="background-color: var(--color-danger)" onclick={deleteReview} disabled={deleteSaving}>
			{deleteSaving ? t('common.deleting', 'Deleting…') : t('common.delete', 'Delete')}
		</button>
	</div>
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
		{#if canDelete}
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)">
				<Trash2 class="inline h-3 w-3" />
				{t('common.delete', 'Delete')}
			</Button>
		{/if}
	</div>
{/snippet}
