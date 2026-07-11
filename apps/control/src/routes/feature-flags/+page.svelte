<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import { api } from '$lib/api';
	import type { FeatureFlag } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { Flag, RefreshCw, Search } from 'lucide-svelte';

	let flags = $state<FeatureFlag[]>([]);
	let loading = $state(true);
	let filter = $state('');
	let toggling = $state<string | null>(null);
	let message = $state('');

	onMount(load);

	async function load() {
		loading = true;
		const result = await api<FeatureFlag[]>('/control/feature-flags');
		if (result.ok) {
			flags = result.data;
		}
		loading = false;
	}

	async function toggleFlag(key: string, enabled: boolean) {
		toggling = key;
		const result = await api(`/control/feature-flags/${key}`, {
			method: 'PUT',
			body: { enabled }
		});
		if (result.ok) {
			flags = flags.map((f) => (f.key === key ? { ...f, enabled } : f));
			message = `Flag "${key}" ${enabled ? 'enabled' : 'disabled'}.`;
		} else {
			message = result.error || 'Failed to toggle flag.';
		}
		toggling = null;
	}

	let filtered = $derived(
		filter
			? flags.filter(
					(f) =>
						f.key.toLowerCase().includes(filter.toLowerCase()) ||
						f.description.toLowerCase().includes(filter.toLowerCase())
				)
			: flags
	);
</script>

<PageHeader title={t('feature_flags.title')} description={t('feature_flags.description')} />

<div class="mb-4 flex items-center gap-3">
	<div class="relative">
		<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
		<input
			bind:value={filter}
			placeholder="Search flags..."
			class="w-72 rounded-lg border py-2 pl-9 pr-3 text-sm outline-none"
			style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
		/>
	</div>
	<button class="btn btn-secondary text-xs" onclick={load}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if message}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-primary); background-color: color-mix(in srgb, var(--color-primary) 8%, transparent); color: var(--color-primary)">
		{message}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else if filtered.length === 0}
	<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<Flag class="mx-auto h-8 w-8" style="color: var(--color-muted)" />
		<p class="mt-3 text-sm" style="color: var(--color-muted)">{filter ? 'No flags match your search.' : 'No feature flags configured.'}</p>
	</div>
{:else}
	<div class="overflow-hidden rounded-lg border" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<table class="w-full text-left text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Flag</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Description</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Groups</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Status</th>
				</tr>
			</thead>
			<tbody>
				{#each filtered as flag (flag.key)}
					<tr style="border-bottom: 1px solid var(--color-border)">
						<td class="px-4 py-3 font-medium font-mono text-xs">{flag.key}</td>
						<td class="max-w-md truncate px-4 py-3 text-xs" style="color: var(--color-muted)">{flag.description}</td>
						<td class="px-4 py-3">
							<div class="flex flex-wrap gap-1">
								{#each flag.allowed_groups as group}
									<span class="rounded px-1 py-0.5 text-[10px]" style="background: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)">
										{group}
									</span>
								{/each}
							</div>
						</td>
						<td class="px-4 py-3">
							<button
								class="relative inline-flex h-5 w-9 items-center rounded-full transition-colors disabled:opacity-50"
								style="background-color: {flag.enabled ? 'var(--color-success)' : 'var(--color-border)'}"
								disabled={toggling === flag.key}
								onclick={() => toggleFlag(flag.key, !flag.enabled)}
							>
								<span
									class="inline-block h-3.5 w-3.5 rounded-full bg-white shadow-sm transition-transform"
									style="transform: translateX({flag.enabled ? '18px' : '2px'})"
								></span>
							</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
