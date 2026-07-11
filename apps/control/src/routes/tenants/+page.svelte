<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import { api } from '$lib/api';
	import type { Tenant } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { Building2, Search, RefreshCw } from 'lucide-svelte';

	let tenants = $state<Tenant[]>([]);
	let loading = $state(true);
	let filter = $state('');

	onMount(load);

	async function load() {
		loading = true;
		const result = await api<Tenant[]>('/control/tenants');
		if (result.ok) {
			tenants = result.data;
		}
		loading = false;
	}

	function formatBytes(bytes: number): string {
		if (bytes >= 1_073_741_824) return (bytes / 1_073_741_824).toFixed(1) + ' GB';
		if (bytes >= 1_048_576) return (bytes / 1_048_576).toFixed(1) + ' MB';
		if (bytes >= 1024) return (bytes / 1024).toFixed(1) + ' KB';
		return bytes + ' B';
	}

	function statusColor(status: string): string {
		switch (status) {
			case 'active': return 'var(--color-success)';
			case 'suspended': return 'var(--color-warning)';
			default: return 'var(--color-muted)';
		}
	}

	let filtered = $derived(
		filter
			? tenants.filter(
					(t) =>
						t.name.toLowerCase().includes(filter.toLowerCase()) ||
						t.slug.toLowerCase().includes(filter.toLowerCase()) ||
						t.plan.toLowerCase().includes(filter.toLowerCase())
				)
			: tenants
	);
</script>

<PageHeader title={t('tenants.title')} description={t('tenants.description')} />

<div class="mb-4 flex items-center gap-3">
	<div class="relative">
		<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
		<input
			bind:value={filter}
			placeholder="Search tenants..."
			class="w-72 rounded-lg border py-2 pl-9 pr-3 text-sm outline-none"
			style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
		/>
	</div>
	<button class="btn btn-secondary text-xs" onclick={load}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else if filtered.length === 0}
	<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<Building2 class="mx-auto h-8 w-8" style="color: var(--color-muted)" />
		<p class="mt-3 text-sm" style="color: var(--color-muted)">No tenants found.</p>
	</div>
{:else}
	<div class="overflow-hidden rounded-lg border" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<table class="w-full text-left text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Name</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Plan</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Users</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Content</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Storage</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Status</th>
				</tr>
			</thead>
			<tbody>
				{#each filtered as tenant (tenant.id)}
					<tr style="border-bottom: 1px solid var(--color-border)">
						<td class="px-4 py-3">
							<div>
								<p class="font-medium">{tenant.name}</p>
								<p class="text-xs" style="color: var(--color-muted)">{tenant.slug}</p>
							</div>
						</td>
						<td class="px-4 py-3">
							<span class="rounded px-1.5 py-0.5 text-xs" style="background: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)">
								{tenant.plan}
							</span>
						</td>
						<td class="px-4 py-3 text-xs">{tenant.user_count}</td>
						<td class="px-4 py-3 text-xs">{tenant.content_count}</td>
						<td class="px-4 py-3 text-xs">{formatBytes(tenant.storage_used)}</td>
						<td class="px-4 py-3">
							<span class="rounded-full px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, {statusColor(tenant.status)} 12%, transparent); color: {statusColor(tenant.status)}">
								{tenant.status}
							</span>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
