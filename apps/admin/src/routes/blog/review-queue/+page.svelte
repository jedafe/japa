<script lang="ts">
	import { blogAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { CheckCircle, XCircle, RefreshCw, ExternalLink } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

	let { data } = $props();

	type ReviewItem = {
		id: string;
		title: string;
		slug: string;
		author_name: string;
		author_id: string;
		status: string;
		submitted_at: string;
		excerpt?: string;
	};

	// svelte-ignore state_referenced_locally
	let items = $state<ReviewItem[]>(data.initial ?? []);
	let loading = $state(false);
	// svelte-ignore state_referenced_locally
	let errorMessage = $state(data.error ?? '');

	// Reject modal
	let showRejectModal = $state(false);
	let rejectItem = $state<ReviewItem | null>(null);
	let rejectReason = $state('');
	let acting = $state(false);

	async function refresh() {
		loading = true;
		errorMessage = '';
		try {
			const result = await blogAdminApi.getReviewQueue();
			if (result.ok) {
				items = (result.data as ReviewItem[]) ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function approve(id: string) {
		acting = true;
		try {
			const result = await blogAdminApi.approvePost(id);
			if (result.ok) {
				items = items.filter((i) => i.id !== id);
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
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
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			acting = false;
		}
	}
</script>

<svelte:head>
	<title>Blog Review Queue — Admin</title>
</svelte:head>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold">Blog Review Queue</h1>
		<button class="flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium hover:bg-gray-100 dark:hover:bg-gray-700" onclick={refresh}>
			<RefreshCw class="h-4 w-4" />
			Refresh
		</button>
	</div>

	{#if errorMessage}
		<div class="rounded-md bg-red-50 p-4 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-400">{errorMessage}</div>
	{/if}

	{#if loading}
		<div class="py-12 text-center text-sm text-gray-500">Loading...</div>
	{:else if items.length === 0}
		<div class="rounded-md border p-8 text-center text-sm text-gray-500" style="border-color: var(--color-border)">
			No posts pending review.
		</div>
	{:else}
		<div class="space-y-3">
			{#each items as item (item.id)}
				<div class="rounded-md border p-4" style="border-color: var(--color-border)">
					<div class="flex items-start justify-between gap-4">
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<h3 class="font-semibold">{item.title}</h3>
								<a href="/blog/{item.slug}" target="_blank" class="text-gray-400 hover:text-gray-600">
									<ExternalLink class="h-3.5 w-3.5" />
								</a>
							</div>
							<p class="mt-1 text-xs text-gray-500">
								By {item.author_name} — submitted {new Date(item.submitted_at).toLocaleDateString()}
							</p>
							{#if item.excerpt}
								<p class="mt-2 text-sm text-gray-600 line-clamp-2 dark:text-gray-400">{item.excerpt}</p>
							{/if}
						</div>
						<div class="flex items-center gap-2">
							<button
								class="flex items-center gap-1 rounded-md bg-green-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-green-700 disabled:opacity-50"
								onclick={() => approve(item.id)}
								disabled={acting}
							>
								<CheckCircle class="h-4 w-4" />
								Approve
							</button>
							<button
								class="flex items-center gap-1 rounded-md bg-red-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
								onclick={() => openReject(item)}
								disabled={acting}
							>
								<XCircle class="h-4 w-4" />
								Reject
							</button>
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<Modal bind:open={showRejectModal} title="Reject Post" onConfirm={confirmReject} confirmLabel="Reject">
	<div class="space-y-4">
		<p class="text-sm text-gray-600 dark:text-gray-400">
			Are you sure you want to reject "{rejectItem?.title}"?
		</p>
		<div>
			<label for="reject-reason" class="mb-1 block text-sm font-medium">Reason (optional)</label>
			<textarea
				id="reject-reason"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				rows="3"
				bind:value={rejectReason}
				placeholder="Explain why this post was rejected..."
			></textarea>
		</div>
	</div>
</Modal>
