<script lang="ts">
	import { onMount } from 'svelte';
	import { request } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw, ExternalLink } from 'lucide-svelte';

	type AffiliateLink = {
		id: string;
		code: string;
		url: string;
		clicks: number;
		conversions: number;
	};

	let links = $state<AffiliateLink[]>([]);
	let earnings = $state<{ total: number; pending: number } | null>(null);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const [linksData, earningsData] = await Promise.all([
				request<AffiliateLink[]>('/me/affiliate-links'),
				request<{ total: number; pending: number }>('/me/affiliate-earnings')
			]);
			links = linksData ?? [];
			earnings = earningsData;
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('affiliate.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('affiliate.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	{#if earnings}
		<div class="mb-6 grid gap-4 sm:grid-cols-2">
			<div class="rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
				<p class="text-sm" style="color: var(--color-muted)">{t('affiliate.total_earnings')}</p>
				<p class="mt-1 text-3xl font-bold">${earnings.total}</p>
			</div>
			<div class="rounded-lg border p-6" style="border-color: var(--color-border); background: var(--color-surface)">
				<p class="text-sm" style="color: var(--color-muted)">{t('affiliate.pending')}</p>
				<p class="mt-1 text-3xl font-bold">${earnings.pending}</p>
			</div>
		</div>
	{/if}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('affiliate.code')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('affiliate.url')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('affiliate.clicks')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('affiliate.conversions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each links as link (link.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-mono text-xs">{link.code}</td>
						<td class="px-4 py-3">
							<a href={link.url} target="_blank" rel="noopener" class="flex items-center gap-1 text-xs underline">
								{link.url.slice(0, 30)}...
								<ExternalLink class="h-3 w-3" />
							</a>
						</td>
						<td class="px-4 py-3 text-xs">{link.clicks}</td>
						<td class="px-4 py-3 text-xs">{link.conversions}</td>
					</tr>
				{:else}
					<tr>
						<td colspan="4" class="px-4 py-8 text-center" style="color: var(--color-muted)">{t('affiliate.empty')}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
