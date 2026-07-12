<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { RefreshCw } from 'lucide-svelte';

	type FeatureFlag = {
		name: string;
		enabled: boolean;
		description?: string;
		updated_at?: string;
	};

	let flags = $state<FeatureFlag[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let saving = $state<string | null>(null);

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<FeatureFlag[]>('/admin/feature-flags');
			if (result.ok) {
				flags = result.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function toggleFlag(flag: FeatureFlag) {
		saving = flag.name;
		try {
			const result = await api<unknown>(`/admin/feature-flags/${flag.name}`, {
				method: 'PATCH',
				body: { enabled: !flag.enabled }
			});
			if (result.ok) {
				flag.enabled = !flag.enabled;
				flags = [...flags];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			saving = null;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>Feature Flags — Admin</title>
</svelte:head>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h1 class="text-2xl font-bold">Feature Flags</h1>
		<button class="flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium hover:bg-gray-100 dark:hover:bg-gray-700" onclick={load}>
			<RefreshCw class="h-4 w-4" />
			Refresh
		</button>
	</div>

	{#if errorMessage}
		<div class="rounded-md bg-red-50 p-4 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-400">{errorMessage}</div>
	{/if}

	{#if loading}
		<div class="py-12 text-center text-sm text-gray-500">Loading...</div>
	{:else if flags.length === 0}
		<div class="rounded-md border p-8 text-center text-sm text-gray-500" style="border-color: var(--color-border)">
			No feature flags found.
		</div>
	{:else}
		<div class="rounded-md border overflow-hidden" style="border-color: var(--color-border)">
			<table class="w-full text-sm">
				<thead>
					<tr class="border-b" style="border-color: var(--color-border)">
						<th class="px-4 py-3 text-left font-medium">Flag</th>
						<th class="px-4 py-3 text-left font-medium">Description</th>
						<th class="px-4 py-3 text-center font-medium">Status</th>
						<th class="px-4 py-3 text-right font-medium">Action</th>
					</tr>
				</thead>
				<tbody>
					{#each flags as flag (flag.name)}
						<tr class="border-b last:border-b-0" style="border-color: var(--color-border)">
							<td class="px-4 py-3 font-mono text-xs">{flag.name}</td>
							<td class="px-4 py-3 text-gray-600 dark:text-gray-400">{flag.description ?? '—'}</td>
							<td class="px-4 py-3 text-center">
								<span
									class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium {flag.enabled
										? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
										: 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400'}"
								>
									{flag.enabled ? 'Enabled' : 'Disabled'}
								</span>
							</td>
							<td class="px-4 py-3 text-right">
								<button
									class="rounded-md px-3 py-1 text-xs font-medium hover:bg-gray-100 dark:hover:bg-gray-700 disabled:opacity-50"
									onclick={() => toggleFlag(flag)}
									disabled={saving === flag.name}
								>
									{saving === flag.name ? 'Saving...' : flag.enabled ? 'Disable' : 'Enable'}
								</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
