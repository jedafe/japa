<script lang="ts">
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Select } from '$lib/components/ui/select';
	import { RefreshCw, Save, Check } from 'lucide-svelte';
	import { settingsAdminApi } from '$lib/api';

	let saving = $state(false);
	let savedCategory = $state('');
	let loading = $state(true);
	let error = $state('');

	interface SettingDef {
		key: string;
		label: string;
		type: string;
		value?: unknown;
		options?: string[];
		masked?: boolean;
	}

	interface SettingCategory {
		key: string;
		label: string;
		settings: SettingDef[];
	}

	let categories = $state<SettingCategory[]>([]);

	const CATEGORY_ICONS: Record<string, string> = {
		general: '⚙️',
		features: '🧩',
		email: '📧',
		security: '🔒',
		appearance: '🎨',
		users: '👤',
		content: '📝',
		payment: '💳',
		media: '🖼️',
		seo: '🔍',
		integrations: '🔗',
		ai: '🤖',
		advanced: '⚡'
	};

	const CATEGORY_DESCRIPTIONS: Record<string, string> = {
		general: 'Basic platform settings',
		features: 'Enable or disable platform features',
		email: 'Email delivery configuration',
		security: 'Authentication and security settings',
		appearance: 'Branding, logo, favicon, and visual theme',
		users: 'User profile and registration settings',
		content: 'Content creation and moderation settings',
		payment: 'Payment gateway configuration',
		media: 'Image processing and upload settings',
		seo: 'Search engine optimization and metadata',
		integrations: 'Third-party service connections',
		ai: 'AI-powered features configuration',
		advanced: 'System-level and debug settings'
	};

	// Track toggle states per category+key
	let toggleStates = $state<Record<string, boolean>>({});

	async function loadSettings() {
		loading = true;
		error = '';
		try {
			const result = await settingsAdminApi.list();
			if (result.ok) {
				categories = (result.data as SettingCategory[]) ?? [];
				// Initialize toggle states
				for (const cat of categories) {
					for (const s of cat.settings) {
						if (s.type === 'toggle') {
							const id = `${cat.key}.${s.key}`;
							if (!(id in toggleStates)) {
								toggleStates[id] = Boolean(s.value);
							}
						}
					}
				}
			} else {
				error = result.error;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load settings';
		} finally {
			loading = false;
		}
	}

	async function saveCategory(categoryKey: string) {
		saving = true;
		error = '';
		try {
			// Collect values from form
			const cat = categories.find(c => c.key === categoryKey);
			if (!cat) return;
			
			const entries: Record<string, unknown> = {};
			for (const s of cat.settings) {
				const id = `${cat.key}.${s.key}`;
				if (s.type === 'toggle') {
					entries[s.key] = toggleStates[id] ?? false;
				} else {
					entries[s.key] = s.value;
				}
			}
			
			const result = await settingsAdminApi.updateCategory(categoryKey, entries);
			if (result.ok) {
				savedCategory = categoryKey;
				setTimeout(() => (savedCategory = ''), 3000);
			} else {
				error = result.error;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save settings';
		} finally {
			saving = false;
		}
	}

	function toggleSetting(catKey: string, settingKey: string) {
		const id = `${catKey}.${settingKey}`;
		toggleStates[id] = !toggleStates[id];
	}

	function updateSettingValue(catKey: string, settingKey: string, value: unknown) {
		const cat = categories.find(c => c.key === catKey);
		if (!cat) return;
		const setting = cat.settings.find(s => s.key === settingKey);
		if (setting) {
			setting.value = value;
		}
	}

	$effect(() => {
		loadSettings();
	});
</script>

<svelte:head>
	<title>Settings — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title="Settings"
	description="Configure platform settings and preferences"
/>

{#if loading}
	<div class="flex items-center justify-center py-12">
		<RefreshCw class="h-5 w-5 animate-spin" style="color: var(--color-muted)" />
		<span class="ml-2 text-sm" style="color: var(--color-muted)">Loading settings...</span>
	</div>
{:else if error}
	<div class="rounded-lg p-4 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{error}
	</div>
{:else}
	<div class="space-y-6">
		{#each categories as category (category.key)}
			<Card>
				<CardHeader>
					<div class="flex items-center gap-3">
						<span class="text-2xl">{CATEGORY_ICONS[category.key] ?? '⚙️'}</span>
						<div>
							<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{category.label}</h3>
							<p class="text-sm" style="color: var(--color-muted)">{CATEGORY_DESCRIPTIONS[category.key] ?? ''}</p>
						</div>
					</div>
				</CardHeader>
				<CardContent>
					<div class="space-y-6">
						{#each category.settings as setting (setting.key)}
							{@const toggleId = `${category.key}.${setting.key}`}
							<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 sm:items-center">
								<div>
									<label for={toggleId} class="text-sm font-medium" style="color: var(--color-foreground)">
										{setting.label}
									</label>
								</div>

								<div>
									{#if setting.type === 'text'}
										<input
											type={setting.masked ? 'password' : 'text'}
											id={toggleId}
											value={String(setting.value ?? '')}
											placeholder={setting.masked ? '••••••••' : ''}
											class="h-10 w-full rounded-lg border px-3 py-2 text-sm"
											style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
											oninput={(e) => updateSettingValue(category.key, setting.key, (e.target as HTMLInputElement).value)}
										/>
									{:else if setting.type === 'number'}
										<input
											type="number"
											id={toggleId}
											value={setting.value != null ? String(setting.value) : ''}
											class="h-10 w-full rounded-lg border px-3 py-2 text-sm"
											style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
											oninput={(e) => updateSettingValue(category.key, setting.key, Number((e.target as HTMLInputElement).value))}
										/>
									{:else if setting.type === 'toggle'}
										<div class="flex items-center gap-3">
											<button
												type="button"
												role="switch"
												aria-checked={toggleStates[toggleId]}
												aria-label={setting.label}
												onclick={() => toggleSetting(category.key, setting.key)}
												class="relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 {toggleStates[toggleId] ? 'bg-primary-600' : 'bg-gray-200'}"
											>
												<span
													class="inline-block h-4 w-4 transform rounded-full bg-white shadow-sm transition-transform {toggleStates[toggleId] ? 'translate-x-6' : 'translate-x-1'}"
												></span>
											</button>
											<span class="text-sm" style="color: var(--color-muted)">
												{toggleStates[toggleId] ? 'Enabled' : 'Disabled'}
											</span>
										</div>
									{:else if setting.type === 'select'}
										<Select
											options={(setting.options ?? []).map((opt: string) => ({ value: opt, label: opt }))}
											value={String(setting.value ?? '')}
											onchange={(val: string) => updateSettingValue(category.key, setting.key, val)}
										/>
									{/if}
								</div>
							</div>
						{/each}
					</div>

					<div class="mt-6 flex items-center justify-end gap-4">
						{#if savedCategory === category.key}
							<div class="flex items-center gap-2" style="color: var(--color-success)">
								<Check class="h-4 w-4" />
								<span class="text-sm font-medium">Saved!</span>
							</div>
						{/if}

						<Button disabled={saving} variant="outline" size="sm" onclick={() => saveCategory(category.key)}>
							{#if saving}
								<RefreshCw class="h-4 w-4 animate-spin" />
								Saving...
							{:else}
								<Save class="h-4 w-4" />
								Save {category.label}
							{/if}
						</Button>
					</div>
				</CardContent>
			</Card>
		{/each}
	</div>
{/if}
