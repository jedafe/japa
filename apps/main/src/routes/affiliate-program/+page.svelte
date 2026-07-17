<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type ProgramInfo = {
		name: string;
		description: string;
		commission_rates: Record<string, number>;
		requirements: string[];
	};

	let info = $state<ProgramInfo | null>(null);
	let loading = $state(true);
	let error = $state('');
	let enrolling = $state(false);

	onMount(load);

	async function load() {
		loading = true;
		error = '';
		try {
			info = await api.getProgramInfo();
		} catch {
			error = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function enroll() {
		enrolling = true;
		try {
			await api.enrollAffiliate();
			window.location.href = '/affiliate';
		} catch {
			error = t('common.error');
		} finally {
			enrolling = false;
		}
	}
</script>

<svelte:head>
	<title>Affiliate Program — JapaConnect</title>
</svelte:head>

<div class="mx-auto max-w-3xl py-12">
	{#if loading}
		<p class="text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
	{:else if error}
		<div class="rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
			{error}
		</div>
	{:else if info}
		<h1 class="mb-4 text-3xl font-bold tracking-tight">{info.name ?? 'Affiliate Program'}</h1>
		<p class="mb-8 text-lg" style="color: var(--color-muted)">{info.description ?? ''}</p>

		{#if info.commission_rates && Object.keys(info.commission_rates).length > 0}
			<section class="mb-8">
				<h2 class="mb-4 text-xl font-semibold">{t('affiliate-program.commission_rates')}</h2>
				<div class="grid gap-4 sm:grid-cols-3">
					{#each Object.entries(info.commission_rates) as [key, rate]}
						<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
							<p class="text-sm" style="color: var(--color-muted)">{key.replace(/_/g, ' ')}</p>
							<p class="mt-1 text-2xl font-bold">{rate}%</p>
						</div>
					{/each}
				</div>
			</section>
		{/if}

		{#if info.requirements && info.requirements.length > 0}
			<section class="mb-8">
				<h2 class="mb-4 text-xl font-semibold">{t('affiliate-program.requirements')}</h2>
				<ul class="list-disc space-y-2 pl-5 text-sm" style="color: var(--color-muted)">
					{#each info.requirements as req}
						<li>{req}</li>
					{/each}
				</ul>
			</section>
		{/if}

		<section class="rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h2 class="mb-2 text-xl font-semibold">{t('affiliate-program.ready_to_join')}</h2>
			<p class="mb-4 text-sm" style="color: var(--color-muted)">
				Earn commissions by sharing your unique affiliate links. Track your performance and payouts from your dashboard.
			</p>
			<button
				class="rounded-md px-6 py-2.5 text-sm font-medium text-white transition-colors disabled:opacity-50"
				style="background-color: var(--color-primary)"
				disabled={enrolling}
				onclick={enroll}
			>
				{enrolling ? t('common.loading') : 'Enroll Now'}
			</button>
		</section>
	{/if}
</div>
