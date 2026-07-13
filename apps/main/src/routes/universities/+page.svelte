<script lang="ts">
	import { onMount } from 'svelte';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, GraduationCap } from 'lucide-svelte';

	type University = {
		id: string;
		name: string;
		slug: string;
		country?: string;
	};

	let universities = $state<University[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			universities = await request<University[]>('/universities');
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('universities.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('universities.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
		{#each universities as uni (uni.id)}
			<a href="/universities/{uni.slug}" class="flex items-center gap-3 rounded-lg border p-4 transition-colors hover:bg-black/5" style="border-color: var(--color-border)">
				<GraduationCap class="h-5 w-5" style="color: var(--color-primary)" />
				<div>
					<p class="font-medium">{uni.name}</p>
					{#if uni.country}
						<p class="text-xs" style="color: var(--color-muted)">{uni.country}</p>
					{/if}
				</div>
			</a>
		{:else}
			<p class="col-span-full py-8 text-center text-sm" style="color: var(--color-muted)">{t('universities.empty')}</p>
		{/each}
	</div>
{/if}
