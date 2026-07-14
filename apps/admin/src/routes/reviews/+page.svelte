<script lang="ts">
	import { onMount } from 'svelte';
	import { api, reviewsAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw } from 'lucide-svelte';

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

	onMount(load);
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
				</tr>
			</thead>
			<tbody>
				{#each reviews as review (review.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 text-xs">{review.entity_type}</td>
						<td class="px-4 py-3 text-xs font-mono" style="color: var(--color-muted)">{review.entity_id.slice(0, 12)}</td>
						<td class="px-4 py-3 text-xs">{review.rating}/5</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {review.status === 'approved' ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : review.status === 'pending' ? 'bg-amber-500/15 text-amber-700 dark:text-amber-400' : 'bg-red-500/15 text-red-700 dark:text-red-400'}">
								{review.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(review.created_at).toLocaleDateString()}</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('reviews.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
