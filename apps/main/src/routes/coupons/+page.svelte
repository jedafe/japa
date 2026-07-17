<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type Coupon } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	let coupons = $state<Coupon[]>([]);
	let loading = $state(true);
	let error = $state('');
	let search = $state('');
	let page = $state(1);
	let total = $state(0);
	const pageSize = 12;

	onMount(load);

	async function load() {
		loading = true;
		error = '';
		try {
			const result = await api.listPublicCoupons({
				search: search || undefined,
				page,
				page_size: pageSize
			});
			coupons = result.coupons ?? [];
			total = result.total ?? 0;
		} catch {
			error = t('common.error');
		} finally {
			loading = false;
		}
	}

	function handleSearch() {
		page = 1;
		load();
	}

	let totalPages = $derived(Math.ceil(total / pageSize));
</script>

<svelte:head>
	<title>Coupons — JapaConnect</title>
</svelte:head>

<div class="mx-auto max-w-4xl py-12">
	<h1 class="mb-2 text-3xl font-bold tracking-tight">{t('coupons.coupons')}</h1>
	<p class="mb-6 text-sm" style="color: var(--color-muted)">{t('coupons.browse_available_coupons_and_discounts')}</p>

	{#if error}
		<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
			{error}
		</div>
	{/if}

	<!-- Search -->
	<div class="mb-6 flex gap-2">
		<input
			type="text"
			placeholder="Search coupons..."
			class="flex-1 rounded-md border px-3 py-2 text-sm"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
			bind:value={search}
			onkeydown={(e) => { if (e.key === 'Enter') handleSearch(); }}
		/>
		<button
			class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={handleSearch}
		>
			Search
		</button>
	</div>

	{#if loading}
		<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
	{:else if coupons.length === 0}
		<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('coupons.no_coupons_available')}</p>
	{:else}
		<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each coupons as coupon (coupon.id)}
				<div class="rounded-lg border p-4 transition-colors hover:shadow-sm" style="border-color: var(--color-border); background-color: var(--color-surface)">
					<div class="mb-2 flex items-start justify-between">
						<h3 class="font-semibold">{coupon.title}</h3>
						{#if coupon.discount_label}
							<span class="rounded-full bg-emerald-500/15 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:text-emerald-400">
								{coupon.discount_label}
							</span>
						{/if}
					</div>
					{#if coupon.description}
						<p class="mb-3 text-sm" style="color: var(--color-muted)">{coupon.description}</p>
					{/if}
					<div class="flex items-center justify-between text-xs" style="color: var(--color-muted)">
						<span class="font-mono">{coupon.code}</span>
						{#if coupon.expires_at}
							<span>Expires {new Date(coupon.expires_at).toLocaleDateString()}</span>
						{/if}
					</div>
					{#if coupon.affiliate_url}
						<a
							href={coupon.affiliate_url}
							target="_blank"
							rel="noopener noreferrer"
							class="mt-3 block rounded-md py-2 text-center text-xs font-medium text-white transition-colors"
							style="background-color: var(--color-primary)"
						>
							Use Coupon
						</a>
					{/if}
				</div>
			{/each}
		</div>

		<!-- Pagination -->
		{#if totalPages > 1}
			<div class="mt-6 flex items-center justify-center gap-2">
				<button
					class="rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50"
					style="border: 1px solid var(--color-border)"
					disabled={page <= 1}
					onclick={() => { page--; load(); }}
				>
					Previous
				</button>
				<span class="text-sm" style="color: var(--color-muted)">Page {page} of {totalPages}</span>
				<button
					class="rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50"
					style="border: 1px solid var(--color-border)"
					disabled={page >= totalPages}
					onclick={() => { page++; load(); }}
				>
					Next
				</button>
			</div>
		{/if}
	{/if}
</div>
