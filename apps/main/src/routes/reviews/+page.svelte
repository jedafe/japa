<script lang="ts">
	import { onMount } from 'svelte';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, Star } from 'lucide-svelte';

	type Review = {
		id: string;
		rating: number;
		comment: string;
		created_at: string;
	};

	let reviews = $state<Review[]>([]);
	let entityType = $state('space');
	let entityId = $state('');
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			reviews = await request<Review[]>(`/reviews/${entityType}/${entityId}`);
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		const params = new URLSearchParams(window.location.search);
		entityType = params.get('type') || 'space';
		entityId = params.get('id') || '';
		if (entityId) load();
	});
</script>

<svelte:head>
	<title>{t('reviews.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('reviews.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="space-y-3">
		{#each reviews as review (review.id)}
			<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
				<div class="flex items-center gap-1 mb-2">
					{#each Array(review.rating) as _}
						<Star class="h-3 w-3 fill-current" style="color: var(--color-warning)" />
					{/each}
				</div>
				<p class="text-sm">{review.comment}</p>
				<p class="text-xs" style="color: var(--color-muted)">{new Date(review.created_at).toLocaleDateString()}</p>
			</div>
		{:else}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('reviews.empty')}</p>
		{/each}
	</div>
{/if}
