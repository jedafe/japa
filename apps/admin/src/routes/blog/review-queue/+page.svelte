<script lang="ts">
	import { onMount } from 'svelte';
	import { blogAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { TableSkeleton } from '$lib/components/ui/skeleton';
	import { RefreshCw, CheckCircle, XCircle, ExternalLink } from 'lucide-svelte';

	const canManage = $derived(hasPermission('blog.review.manage'));

	type ReviewItem = {
		id: string;
		title: string;
		slug: string;
		author_name: string;
		author_id: string;
		status: string;
		submitted_at: string;
		excerpt?: string;
		url?: string;
	};

	let items = $state<ReviewItem[]>([]);
	let loading = $state(true);
	let error = $state('');

	// Reject modal
	let showRejectModal = $state(false);
	let rejectItem = $state<ReviewItem | null>(null);
	let rejectReason = $state('');
	let acting = $state(false);

	const columns = $derived([
		{
			key: 'title',
			label: t('app.blog.col_title', 'Title'),
			sortable: true
		},
		{
			key: 'author_name',
			label: t('app.blog.col_author', 'Author'),
			sortable: true,
			render: (value: string) => value || t('app.blog.unknown', 'Unknown')
		},
		{
			key: 'submitted_at',
			label: t('app.blog.review_submitted_at', 'Submitted'),
			sortable: true,
			render: (value: string) => value ? new Date(value).toLocaleDateString() : '—'
		},
		{
			key: 'excerpt',
			label: t('app.blog.detail_excerpt', 'Excerpt'),
			render: (value: string) => {
				if (!value) return '—';
				return value.length > 80 ? value.substring(0, 80) + '…' : value;
			}
		},
		{
			key: 'actions',
			label: t('app.blog.col_actions', 'Actions'),
			class: 'w-32',
			componentSnippet: ReviewActionsCell
		}
	]);

	async function refresh() {
		loading = true;
		error = '';
		try {
			const result = await blogAdminApi.getReviewQueue();
			loading = false;
			if (result.ok) {
				items = (result.data as ReviewItem[]) ?? [];
			} else {
				error = result.error;
			}
		} catch {
			loading = false;
			error = t('common.error', 'An error occurred');
		}
	}

	onMount(() => {
		refresh();
	});

	async function approve(id: string) {
		acting = true;
		try {
			const result = await blogAdminApi.approvePost(id);
			if (result.ok) {
				items = items.filter((i) => i.id !== id);
			} else {
				error = result.error;
			}
		} catch {
			error = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	function openReject(item: ReviewItem) {
		rejectItem = item;
		rejectReason = '';
		showRejectModal = true;
	}

	async function confirmReject() {
		if (!rejectItem) return;
		acting = true;
		try {
			const result = await blogAdminApi.rejectPost(rejectItem.id, { reason: rejectReason });
			if (result.ok) {
				items = items.filter((i) => i.id !== rejectItem!.id);
				showRejectModal = false;
			} else {
				error = result.error;
			}
		} catch {
			error = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	function openExternal(item: ReviewItem) {
		if (item.url) {
			window.open(item.url, '_blank');
		}
	}
</script>

<svelte:head>
	<title>{t('app.blog.review_queue_page_title', 'Blog Review Queue — JapaConnect Admin')}</title>
</svelte:head>

<PageHeader
	title={t('app.blog.review_queue_heading', 'Blog Review Queue')}
	description={t('app.blog.review_queue_desc', 'Review and approve pending blog submissions')}
	actions={[
		{ label: t('app.blog.refresh', 'Refresh'), icon: RefreshCw, variant: 'ghost', onclick: refresh }
	]}
/>

<div class="space-y-6">
	{#if error}
		<div class="rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
			{error}
		</div>
	{/if}

	<Card>
		<CardContent class="p-0">
			{#if loading}
				<TableSkeleton rows={5} columns={columns.length} />
			{:else}
				<div aria-live="polite">
					<DataTable
						{columns}
						data={items}
						pageSize={25}
						emptyMessage={t('app.blog.review_queue_empty', 'No posts pending review.')}
					/>
				</div>
			{/if}
		</CardContent>
	</Card>
</div>

<Modal bind:open={showRejectModal} title={t('app.blog.modal_reject_post', 'Reject Post')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.blog.reject_confirm', 'Are you sure you want to reject "{title}"? The author will be notified.').replace('{title}', rejectItem?.title ?? '')}
		</p>
		<div>
			<label for="reject-reason" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.blog.reason_optional', 'Reason (optional)')}</label>
			<textarea
				id="reject-reason"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="3"
				bind:value={rejectReason}
				placeholder={t('app.blog.reject_placeholder', 'Explain why this post was rejected...')}
			></textarea>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showRejectModal = false)}>{t('app.blog.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmReject} disabled={acting}>
			{acting ? t('app.blog.rejecting', 'Rejecting...') : t('app.blog.reject', 'Reject')}
		</Button>
	</div>
</Modal>

{#snippet ReviewActionsCell({ row }: { value: any; row: ReviewItem })}
	<div class="flex items-center gap-1">
		{#if canManage}
			<Button
				variant="ghost"
				size="sm"
				onclick={() => approve(row.id)}
				disabled={acting}
				style="color: var(--color-success)"
			>
				<CheckCircle class="h-3.5 w-3.5" />
			</Button>
			<Button
				variant="ghost"
				size="sm"
				onclick={() => openReject(row)}
				disabled={acting}
				style="color: var(--color-danger)"
			>
				<XCircle class="h-3.5 w-3.5" />
			</Button>
		{/if}
		{#if row.url}
			<Button variant="ghost" size="sm" onclick={() => openExternal(row)}>
				<ExternalLink class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
