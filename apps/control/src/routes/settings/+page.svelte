<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import SearchInput from '$lib/components/ui/SearchInput.svelte';
	import { Tabs, Switch, Tooltip } from 'bits-ui';
	import { api } from '$lib/api';
	import type { SystemSetting } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { Settings, RefreshCw, RotateCcw } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data = {} as PageData } = $props();

	// svelte-ignore state_referenced_locally
	let settings = $state<SystemSetting[]>(data.settings ?? []);
	let loading = $state(false);
	let saving = $state<string | null>(null);
	let resetting = $state<string | null>(null);
	let filter = $state('');
	let activeTab = $state('all');
	let message = $state('');
	let messageType = $state<'success' | 'error' | ''>('');
	let confirmResetSetting = $state<SystemSetting | null>(null);
	let resetModalOpen = $state(false);

	async function load() {
		loading = true;
		message = '';
		const result = await api<SystemSetting[]>('/control/system-settings');
		if (result.ok) {
			settings = result.data;
		}
		loading = false;
	}

	async function saveSetting(key: string, value: string) {
		saving = key;
		message = '';
		const result = await api(`/control/system-settings/${key}`, {
			method: 'PATCH',
			body: { value }
		});
		if (result.ok) {
			settings = settings.map((s) => (s.key === key ? { ...s, value } : s));
			message = t('settings.save_success', `Setting "${key}" saved.`);
			messageType = 'success';
		} else {
			message = result.error || t('settings.save_error', 'Failed to save setting.');
			messageType = 'error';
		}
		saving = null;
	}

	async function resetSetting(key: string) {
		resetting = key;
		message = '';
		const result = await api<{ key: string; value: string; default_value: string }>(
			`/control/system-settings/${key}/reset`,
			{ method: 'POST' }
		);
		if (result.ok) {
			settings = settings.map((s) =>
				s.key === key ? { ...s, value: result.data.value } : s
			);
			message = t('settings.reset_success', `Setting "${key}" reset to default value.`);
			messageType = 'success';
		} else {
			message = result.error || t('settings.reset_error', 'Failed to reset setting.');
			messageType = 'error';
		}
		resetting = null;
	}

	// Group by top-level category (first segment before the first dot)
	let categories = $derived(
		[...new Set(settings.map((s) => {
			const ns = s.namespace ?? 'unknown';
			return ns.split('.')[0] ?? ns;
		}))].sort()
	);

	let categoryItems = $derived(
		categories.map((cat) => ({
			category: cat,
			count: settings.filter((s) => {
				const ns = s.namespace ?? 'unknown';
				return (ns.split('.')[0] ?? ns) === cat;
			}).length
		}))
	);

	let filtered = $derived(
		settings.filter((s) => {
			const matchesFilter = filter
				? (s.key ?? '').toLowerCase().includes(filter.toLowerCase()) ||
					(s.namespace ?? '').toLowerCase().includes(filter.toLowerCase()) ||
					(s.description ?? '').toLowerCase().includes(filter.toLowerCase())
				: true;
			const matchesCategory =
				activeTab === 'all' ? true : (s.namespace ?? 'unknown').split('.')[0] === activeTab;
			return matchesFilter && matchesCategory;
		})
	);

	let grouped = $derived(
		Object.entries(
			filtered.reduce(
				(acc, s) => {
					const ns = s.namespace ?? 'unknown';
					(acc[ns] ??= []).push(s);
					return acc;
				},
				{} as Record<string, SystemSetting[]>
			)
		).sort(([a], [b]) => a.localeCompare(b))
	);

	function formatSettingKey(key: string): string {
		return (key ?? '')
			.replace(/_/g, ' ')
			.replace(/([a-z])([A-Z])/g, '$1 $2')
			.replace(/\./g, ' - ')
			.replace(/\b\w/g, (c) => c.toUpperCase());
	}

	function getDisplayValue(setting: SystemSetting): string {
		const val = setting.value ?? '';
		// Remove surrounding quotes if present for display
		if (val.startsWith('"') && val.endsWith('"')) {
			return val.slice(1, -1);
		}
		return val;
	}

	function getSaveValue(setting: SystemSetting, inputValue: string): string {
		// For numeric types, store as number string without quotes
		if (setting.type === 'int' || setting.type === 'float') {
			return inputValue;
		}
		// For other types, keep as-is
		return inputValue;
	}

	function handleInput(e: Event, setting: SystemSetting) {
		const target = e.currentTarget as HTMLInputElement;
		const rawValue = target.value;
		const saveValue = getSaveValue(setting, rawValue);
		saveSetting(setting.key, saveValue);
	}
</script>

<PageHeader
	title={t('settings.title', 'Settings')}
	description={t('settings.description', 'Platform configuration by category')}
/>

<div class="mb-4 flex flex-wrap items-center gap-3">
	<SearchInput
		bind:value={filter}
		placeholder={t('settings.filter_placeholder', 'Filter settings...')}
		class="w-80"
	/>
	<button
		class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px flex items-center gap-1.5 border px-3 py-2 text-xs font-medium transition-colors"
		onclick={load}
		disabled={loading}
	>
		<RefreshCw class="size-3 {loading ? 'animate-spin' : ''}" />
		{t('common.refresh', 'Refresh')}
	</button>
	<span class="text-muted-foreground text-xs">
		{filtered.length} {t('settings.of', 'of')} {settings.length} {t('settings.settings', 'settings')}
	</span>
</div>

{#if message}
	<div
		class="mb-4 rounded-lg border px-4 py-3 text-sm {messageType === 'success'
			? 'border-green-500/30 bg-green-500/10 text-green-600'
			: messageType === 'error'
				? 'border-destructive/30 bg-destructive/10 text-destructive'
				: ''}"
	>
		{message}
	</div>
{/if}

{#if loading}
	<div class="flex items-center justify-center py-12">
		<RefreshCw class="text-muted-foreground size-6 animate-spin" />
	</div>
{:else}
	<Tabs.Root bind:value={activeTab}>
		<Tabs.List class="bg-muted mb-4 inline-flex flex-wrap gap-1 rounded-9px p-1">
			<Tabs.Trigger
				value="all"
				class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all"
			>
				{t('settings.all', 'All')} ({settings.length})
			</Tabs.Trigger>
			{#each categoryItems as { category, count }}
				<Tabs.Trigger
					value={category}
					class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all"
				>
					{category} ({count})
				</Tabs.Trigger>
			{/each}
		</Tabs.List>

		<Tabs.Content value={activeTab} class="mt-0">
			{#if filtered.length === 0}
				<div class="border-border-card bg-background-alt rounded-9px border p-8 text-center">
					<Settings class="text-muted-foreground mx-auto mb-3 size-8" />
					<p class="text-muted-foreground text-sm">{t('settings.no_results', 'No settings match your filters.')}</p>
				</div>
			{:else}
				<div class="space-y-4">
					{#each grouped as [namespace, items]}
						<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
							<div class="border-border-card bg-muted/50 border-b px-4 py-3">
								<h3 class="text-foreground flex items-center gap-2 text-sm font-semibold">
									<Settings class="text-accent-foreground size-4" />
									{namespace}
									<span class="text-muted-foreground font-normal">({items.length})</span>
								</h3>
							</div>
							<div class="divide-border-card divide-y">
								{#each items as setting (setting.key)}
									<div class="hover:bg-muted/30 flex items-center gap-4 px-4 py-3 transition-colors">
										<div class="min-w-0 flex-1">
											<div class="flex items-center gap-2">
												<p class="text-foreground text-sm font-medium">{formatSettingKey(setting.key)}</p>
												<Tooltip.Root>
													<Tooltip.Trigger>
														<span class="text-muted-foreground hover:text-foreground cursor-help text-xs transition-colors">ℹ</span>
													</Tooltip.Trigger>
													<Tooltip.Portal>
														<Tooltip.Content
															side="top"
															sideOffset={4}
															class="border-dark-10 bg-background shadow-popover z-50 w-[280px] rounded-input border p-3 text-xs"
														>
															<p class="mb-1 font-medium">{setting.key}</p>
															<p class="text-muted-foreground">{setting.description || t('settings.no_description', 'No description available.')}</p>
															<p class="text-muted-foreground mt-2">
																{t('settings.default', 'Default')}: <code class="bg-muted rounded px-1">{setting.default_value}</code>
															</p>
														</Tooltip.Content>
													</Tooltip.Portal>
												</Tooltip.Root>
											</div>
											<p class="text-muted-foreground truncate text-xs">{setting.description}</p>
										</div>
										<div class="flex items-center gap-2">
											{#if setting.type === 'bool'}
												<Switch.Root
													checked={setting.value === 'true'}
													onCheckedChange={(checked) => saveSetting(setting.key, checked ? 'true' : 'false')}
													disabled={saving === setting.key}
													class="bg-muted data-[state=checked]:bg-foreground peer inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full px-[3px] transition-colors disabled:cursor-not-allowed disabled:opacity-50"
												>
													<Switch.Thumb
														class="bg-background pointer-events-none block size-4 shrink-0 rounded-full transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0"
													/>
												</Switch.Root>
											{:else if setting.type === 'int'}
												<input
													type="number"
													value={getDisplayValue(setting)}
													onchange={(e) => handleInput(e, setting)}
													disabled={saving === setting.key}
													step="1"
													class="border-border-input bg-background text-foreground w-24 rounded-5px border px-2 py-1 font-mono text-xs transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus disabled:opacity-50"
												/>
											{:else if setting.type === 'float'}
												<input
													type="number"
													value={getDisplayValue(setting)}
													onchange={(e) => handleInput(e, setting)}
													disabled={saving === setting.key}
													step="0.01"
													class="border-border-input bg-background text-foreground w-24 rounded-5px border px-2 py-1 font-mono text-xs transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus disabled:opacity-50"
												/>
											{:else}
												<input
													type="text"
													value={getDisplayValue(setting)}
													onchange={(e) => handleInput(e, setting)}
													disabled={saving === setting.key}
													class="border-border-input bg-background text-foreground w-36 rounded-5px border px-2 py-1 font-mono text-xs transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus disabled:opacity-50"
												/>
											{/if}
											<span class="bg-muted text-muted-foreground rounded-5px px-1.5 py-0.5 text-[10px]">
												{setting.type}
											</span>
											<Tooltip.Root>
												<Tooltip.Trigger>
													<button
														class="text-tertiary hover:bg-tertiary/10 rounded-5px p-1 transition-colors"
														disabled={resetting === setting.key}
														onclick={() => {
															confirmResetSetting = setting;
															resetModalOpen = true;
														}}
													>
														{#if resetting === setting.key}
															<RefreshCw class="size-3 animate-spin" />
														{:else}
															<RotateCcw class="size-3" />
														{/if}
													</button>
												</Tooltip.Trigger>
												<Tooltip.Portal>
													<Tooltip.Content
														side="top"
														sideOffset={4}
														class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs"
													>
														{t('settings.reset_to_default', 'Reset to default')}: <code class="bg-muted rounded px-1">{setting.default_value}</code>
													</Tooltip.Content>
												</Tooltip.Portal>
											</Tooltip.Root>
										</div>
									</div>
								{/each}
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</Tabs.Content>
	</Tabs.Root>
{/if}

<ConfirmModal
	bind:open={resetModalOpen}
	title={t('settings.reset_title', 'Reset Setting')}
	consequence={confirmResetSetting
		? t('settings.reset_consequence', `Reset '${confirmResetSetting.key}' to its default value. Current value: ${confirmResetSetting.value}`)
		: ''}
	confirmText={t('settings.reset_confirm', 'RESET')}
	onConfirm={() => {
		if (confirmResetSetting) {
			resetSetting(confirmResetSetting.key);
		}
		confirmResetSetting = null;
	}}
	onCancel={() => {
		confirmResetSetting = null;
	}}
/>
