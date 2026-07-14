<script lang="ts">
	import { enhance } from '$app/forms';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { RefreshCw, Save, Check } from 'lucide-svelte';

	let { data, form } = $props();

	let saving = $state(false);
	let savedCategory = $state('');

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

	$effect(() => {
		// Initialize toggle states from server data
		for (const cat of data.categories) {
			for (const s of cat.settings) {
				if (s.type === 'toggle') {
					const id = `${cat.key}.${s.key}`;
					if (!(id in toggleStates)) {
						toggleStates[id] = Boolean(s.value);
					}
				}
			}
		}
	});

	$effect(() => {
		if (form?.success && form?.savedCategory) {
			savedCategory = form.savedCategory;
			const t = setTimeout(() => (savedCategory = ''), 3000);
			return () => clearTimeout(t);
		}
	});

	function toggleSetting(catKey: string, settingKey: string) {
		const id = `${catKey}.${settingKey}`;
		toggleStates[id] = !toggleStates[id];
	}
</script>

<svelte:head>
	<title>Settings — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title="Settings"
	description="Configure platform settings and preferences"
/>

<div class="space-y-6">
	{#each data.categories as category (category.key)}
		<Card>
			<CardHeader>
				<div class="flex items-center gap-3">
					<span class="text-2xl">{CATEGORY_ICONS[category.key] ?? '⚙️'}</span>
					<div>
						<h3 class="text-lg font-semibold text-gray-900">{category.label}</h3>
						<p class="text-sm text-gray-500">{CATEGORY_DESCRIPTIONS[category.key] ?? ''}</p>
					</div>
				</div>
			</CardHeader>
			<CardContent>
				<form
					method="POST"
					use:enhance={() => {
						saving = true;
						return async ({ result, update }) => {
							saving = false;
							await update();
						};
					}}
				>
					<input type="hidden" name="_category" value={category.key} />
					<div class="space-y-6">
						{#each category.settings as setting (setting.key)}
							{@const toggleId = `${category.key}.${setting.key}`}
							<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 sm:items-center">
								<div>
									<label for={toggleId} class="text-sm font-medium text-gray-900">
										{setting.label}
									</label>
								</div>

								<div>
									{#if setting.type === 'text'}
										<input
											type={setting.masked ? 'password' : 'text'}
											id={toggleId}
											name={setting.key}
											defaultValue={String(setting.value ?? '')}
											placeholder={setting.masked ? '••••••••' : ''}
											class="h-10 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
										/>
									{:else if setting.type === 'number'}
										<input
											type="number"
											id={toggleId}
											name={setting.key}
											defaultValue={setting.value != null ? String(setting.value) : ''}
											class="h-10 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
										/>
									{:else if setting.type === 'toggle'}
										<div class="flex items-center gap-3">
											<input type="hidden" name={setting.key} value={toggleStates[toggleId] ? 'true' : 'false'} />
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
											<span class="text-sm text-gray-600">
												{toggleStates[toggleId] ? 'Enabled' : 'Disabled'}
											</span>
										</div>
									{:else if setting.type === 'select'}
										<input type="hidden" name={setting.key} value={String(setting.value ?? '')} />
										<select
											id={toggleId}
											class="h-10 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
											onchange={(e) => {
												const form = (e.currentTarget as HTMLElement).closest('form');
												const hidden = form?.querySelector(`input[name="${setting.key}"]`) as HTMLInputElement;
												if (hidden) hidden.value = (e.target as HTMLSelectElement).value;
											}}
										>
											{#each setting.options ?? [] as opt}
												<option value={opt} selected={setting.value === opt}>{opt}</option>
											{/each}
										</select>
									{/if}
								</div>
							</div>
						{/each}
					</div>

					<div class="mt-6 flex items-center justify-end gap-4">
						{#if savedCategory === category.key}
							<div class="flex items-center gap-2 text-green-600">
								<Check class="h-4 w-4" />
								<span class="text-sm font-medium">Saved!</span>
							</div>
						{/if}

						<Button type="submit" disabled={saving} variant="outline" size="sm">
							{#if saving}
								<RefreshCw class="h-4 w-4 animate-spin" />
								Saving...
							{:else}
								<Save class="h-4 w-4" />
								Save {category.label}
							{/if}
						</Button>
					</div>
				</form>
			</CardContent>
		</Card>
	{/each}
</div>
