<script lang="ts">
	import { api, reviewsAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Eye } from 'lucide-svelte';
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

	let showViewModal = $state(false);
	let viewTarget = $state<Review | null>(null);

	function openView(review: Review) {
		viewTarget = review;
		showViewModal = true;
	}

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<Review[]>('/reviews');
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
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('reviews.entity_type')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('reviews.entity_id')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('reviews.rating')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('reviews.status')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('reviews.created')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions', 'Actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each reviews as review (review.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 text-xs">{review.entity_type}</td>
						<td class="px-4 py-3 text-xs font-mono" style="color: var(--color-muted)">{review.entity_id.slice(0, 12)}</td>
						<td class="px-4 py-3 text-xs">{review.rating}/5</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {review.status === 'approved' ? 'bg-success/15 text-success dark:text-success' : review.status === 'pending' ? 'bg-warning/15 text-warning dark:text-warning' : 'bg-danger/15 text-danger dark:text-danger'}">
								{review.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(review.created_at).toLocaleDateString()}</td>
						<td class="px-4 py-3">
							<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-surface/10" style="color: var(--color-muted)" onclick={() => openView(review)}>
								{t('common.view', 'View')}
							</button>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('reviews.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
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
