<script lang="ts">
	import { onMount } from 'svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import { api } from '$lib/api';
	import type { SystemSetting } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { Settings, Save, RefreshCw, Search } from 'lucide-svelte';

	let settings = $state<SystemSetting[]>([]);
	let loading = $state(true);
	let saving = $state<string | null>(null);
	let filter = $state('');
	let message = $state('');
	let messageType = $state<'success' | 'error' | ''>('');

	onMount(load);

	async function load() {
		loading = true;
		const result = await api<SystemSetting[]>('/control/settings');
		if (result.ok) {
			settings = result.data;
		}
		loading = false;
	}

	async function saveSetting(key: string, value: string) {
		saving = key;
		message = '';
		const result = await api(`/control/settings/${key}`, {
			method: 'PUT',
			body: { value }
		});
		if (result.ok) {
			settings = settings.map((s) => (s.key === key ? { ...s, value } : s));
			message = `Setting "${key}" saved.`;
			messageType = 'success';
		} else {
			message = result.error || 'Failed to save setting.';
			messageType = 'error';
		}
		saving = null;
	}

	let filtered = $derived(
		filter
			? settings.filter(
					(s) =>
						s.key.toLowerCase().includes(filter.toLowerCase()) ||
						s.namespace.toLowerCase().includes(filter.toLowerCase()) ||
						s.description.toLowerCase().includes(filter.toLowerCase())
				)
			: settings
	);

	let grouped = $derived(
		Object.entries(
			filtered.reduce(
				(acc, s) => {
					(acc[s.namespace] ??= []).push(s);
					return acc;
				},
				{} as Record<string, SystemSetting[]>
			)
		).sort(([a], [b]) => a.localeCompare(b))
	);
</script>

<PageHeader title={t('settings.title')} description={t('settings.description')} />

<div class="mb-4 flex items-center gap-3">
	<div class="relative">
		<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--color-muted)" />
		<input
			bind:value={filter}
			placeholder="Filter settings..."
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
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-{messageType}); background-color: color-mix(in srgb, var(--color-{messageType}) 8%, transparent); color: var(--color-{messageType})">
		{message}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else}
	<div class="space-y-6">
		{#each grouped as [namespace, items]}
			<div class="rounded-lg border" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<div class="border-b px-4 py-3" style="border-color: var(--color-border)">
					<h3 class="flex items-center gap-2 text-sm font-semibold">
						<Settings class="h-4 w-4" style="color: var(--color-primary)" />
						{namespace}
					</h3>
				</div>
				<div class="divide-y" style="border-color: var(--color-border)">
					{#each items as setting}
						<div class="flex items-center gap-4 px-4 py-3">
							<div class="min-w-0 flex-1">
								<p class="text-sm font-medium">{setting.key}</p>
								<p class="text-xs" style="color: var(--color-muted)">{setting.description}</p>
							</div>
							<div class="flex items-center gap-2">
								{#if setting.type === 'bool'}
									<input
										type="checkbox"
										checked={setting.value === 'true'}
										onchange={(e) => saveSetting(setting.key, e.currentTarget.checked ? 'true' : 'false')}
										class="h-4 w-4 rounded"
									/>
								{:else}
									<input
										type="text"
										value={setting.value}
										onchange={(e) => saveSetting(setting.key, e.currentTarget.value)}
										class="w-32 rounded border px-2 py-1 text-xs font-mono"
										style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
									/>
								{/if}
								<span class="rounded px-1 py-0.5 text-[10px]" style="background: color-mix(in srgb, var(--color-muted) 10%, transparent); color: var(--color-muted)">
									{setting.type}
								</span>
							</div>
						</div>
					{/each}
				</div>
			</div>
		{/each}
	</div>
{/if}
