<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';

	type SettingsCategory = {
		key: string;
		label: string;
		settings: { key: string; label: string; type: 'text' | 'toggle' | 'select' | 'number'; value: string | boolean | number; options?: string[] }[];
	};

	let categories = $state<SettingsCategory[]>([]);
	let activeTab = $state('general');
	let loading = $state(true);
	let saving = $state(false);
	let saveMsg = $state('');
	let apiAvailable = $state(true);

	const tabs = [
		'general', 'appearance', 'features', 'users', 'content',
		'email', 'payment', 'media', 'seo', 'security',
		'integrations', 'ai', 'advanced'
	];

	async function load() {
		try {
			const result = await api<SettingsCategory[]>('/admin/settings');
			if (result.ok) {
				categories = result.data;
				if (categories.length > 0) activeTab = categories[0].key;
			} else {
				apiAvailable = false;
			}
		} catch {
			apiAvailable = false;
		} finally {
			loading = false;
		}
	}

	async function save() {
		saving = true;
		saveMsg = '';
		const cat = categories.find((c) => c.key === activeTab);
		if (!cat) return;
		const payload: Record<string, unknown> = {};
		for (const s of cat.settings) {
			payload[s.key] = s.value;
		}
		try {
			await api(`/admin/settings/${activeTab}`, { method: 'PUT', body: payload });
			saveMsg = 'Saved';
			setTimeout(() => (saveMsg = ''), 3000);
		} catch {
			saveMsg = 'Failed to save';
		} finally {
			saving = false;
		}
	}

	function updateSetting(key: string, value: string | boolean | number) {
		const cat = categories.find((c) => c.key === activeTab);
		if (!cat) return;
		const setting = cat.settings.find((s) => s.key === key);
		if (setting) setting.value = value;
		categories = categories; // trigger reactivity
	}

	onMount(load);
</script>

<svelte:head>
	<title>Settings — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">Settings</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">Platform configuration by category</p>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else}
	<!-- Tabs -->
	<div class="mb-6 flex flex-wrap gap-1 border-b" style="border-color: var(--color-border)">
		{#each tabs as tab}
			<button
				class="rounded-t-md px-3 py-2 text-sm font-medium transition-colors"
				style={activeTab === tab ? 'color: var(--color-primary); border-bottom: 2px solid var(--color-primary)' : 'color: var(--color-muted)'}
				onclick={() => (activeTab = tab)}
			>
				{tab.charAt(0).toUpperCase() + tab.slice(1)}
			</button>
		{/each}
	</div>

	<!-- Settings form -->
	{#if !apiAvailable}
		<div class="rounded-lg border p-6 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<p class="text-sm" style="color: var(--color-muted)">Settings API not implemented yet. This section will be available when the backend settings endpoint is ready.</p>
		</div>
	{:else if categories.length === 0}
		<div class="rounded-lg border p-6 text-center" style="border-color: var(--color-border)">
			<p style="color: var(--color-muted)">No settings available. Connect to the API to load configuration.</p>
		</div>
	{:else}
		{@const active = categories.find((c) => c.key === activeTab)}
		{#if active}
			<div class="rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
				<h2 class="mb-4 text-lg font-semibold">{active.label}</h2>
				<div class="space-y-4">
					{#each active.settings as setting}
						<div class="flex items-center justify-between gap-4">
							<label for="setting-{setting.key}" class="text-sm font-medium">{setting.label}</label>
							{#if setting.type === 'toggle'}
								<button
									id="setting-{setting.key}"
									class="relative inline-flex h-6 w-11 items-center rounded-full transition-colors"
									style="background-color: {setting.value ? 'var(--color-primary)' : 'var(--color-border)'}"
									onclick={() => updateSetting(setting.key, !setting.value)}
									role="switch"
									aria-checked={!!setting.value}
									aria-label={setting.label}
								>
									<span
										class="inline-block h-4 w-4 transform rounded-full bg-white transition-transform"
										style="transform: translateX({setting.value ? '22px' : '2px'})"
									></span>
								</button>
							{:else if setting.type === 'select'}
								<select
									id="setting-{setting.key}"
									class="rounded-md border px-3 py-1.5 text-sm"
									style="border-color: var(--color-border); background-color: var(--color-background)"
									value={String(setting.value)}
									onchange={(e) => updateSetting(setting.key, (e.target as HTMLSelectElement).value)}
								>
									{#each setting.options ?? [] as opt}
										<option value={opt}>{opt}</option>
									{/each}
								</select>
							{:else if setting.type === 'number'}
								<input
									id="setting-{setting.key}"
									type="number"
									class="w-32 rounded-md border px-3 py-1.5 text-sm"
									style="border-color: var(--color-border); background-color: var(--color-background)"
									value={setting.value}
									oninput={(e) => updateSetting(setting.key, Number((e.target as HTMLInputElement).value))}
								/>
							{:else}
							<input
								id="setting-{setting.key}"
								type="text"
								class="w-64 rounded-md border px-3 py-1.5 text-sm"
								style="border-color: var(--color-border); background-color: var(--color-background)"
								value={String(setting.value ?? '')}
								oninput={(e) => updateSetting(setting.key, (e.target as HTMLInputElement).value)}
								/>
							{/if}
						</div>
					{/each}
				</div>
				<div class="mt-6 flex items-center gap-3">
					<button
						class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
						style="background-color: var(--color-primary)"
						disabled={saving}
						onclick={save}
					>
						{saving ? 'Saving...' : 'Save Changes'}
					</button>
					{#if saveMsg}
						<span class="text-xs" style="color: var(--color-muted)">{saveMsg}</span>
					{/if}
				</div>
			</div>
		{/if}
	{/if}
{/if}
