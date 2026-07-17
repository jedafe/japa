<script lang="ts">
	import { beforeNavigate } from '$app/navigation';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import SearchInput from '$lib/components/ui/SearchInput.svelte';
	import { Tabs, Switch, Tooltip, Label, Select } from 'bits-ui';
	import { api } from '$lib/api';
	import type { SystemSetting } from '$lib/api/types';
	import { t } from '$lib/stores/i18n';
	import {
		Settings,
		RefreshCw,
		RotateCcw,
		Check,
		Loader2,
		ChevronDown,
		Save,
		Undo2,
		AlertTriangle,
		Sliders,
		Globe,
		Shield,
		Mail,
		Monitor,
		Palette,
		CreditCard,
		Server,
		Cpu,
		Bell,
		FolderOpen
	} from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data = {} as PageData } = $props();

	let originalSettings = $state<SystemSetting[]>(data.settings ?? []);
	let currentSettings = $state<SystemSetting[]>(structuredClone(originalSettings));
	let isDirty = $derived(
		JSON.stringify(currentSettings) !== JSON.stringify(originalSettings)
	);
	let dirtyCount = $derived(
		currentSettings.filter(
			(s, i) => s.value !== originalSettings[i]?.value
		).length
	);

	let isSaving = $state(false);
	let filter = $state('');
	let activeTab = $state('all');
	let message = $state('');
	let messageType = $state<'success' | 'error' | ''>('');
	let confirmResetSetting = $state<SystemSetting | null>(null);
	let resetModalOpen = $state(false);
	let confirmResetAllOpen = $state(false);

	const categoryIcons: Record<string, typeof Settings> = {
		general: Monitor,
		advanced: Cpu,
		feed: FolderOpen,
		appearance: Palette,
		media: Server,
		i18n: Globe,
		payment: CreditCard,
		notification: Bell,
		security: Shield,
		email: Mail,
		cors: Globe,
		content: Sliders
	};

	const selectOptions: Record<string, { value: string; label: string }[]> = $derived(
		(() => {
			const opts: Record<string, { value: string; label: string }[]> = {
				'advanced.log_level': [
					{ value: 'debug', label: t('settings.log_level_debug', 'Debug') },
					{ value: 'info', label: t('settings.log_level_info', 'Info') },
					{ value: 'warn', label: t('settings.log_level_warn', 'Warn') },
					{ value: 'error', label: t('settings.log_level_error', 'Error') }
				],
				'appearance.feed_card_style': [
					{ value: 'reddit', label: t('settings.card_style_reddit', 'Reddit') },
					{ value: 'compact', label: t('settings.card_style_compact', 'Compact') },
					{ value: 'grid', label: t('settings.card_style_grid', 'Grid') },
					{ value: 'magazine', label: t('settings.card_style_magazine', 'Magazine') }
				],
				'feed.card_style': [
					{ value: 'reddit', label: t('settings.card_style_reddit', 'Reddit') },
					{ value: 'compact', label: t('settings.card_style_compact', 'Compact') },
					{ value: 'grid', label: t('settings.card_style_grid', 'Grid') },
					{ value: 'magazine', label: t('settings.card_style_magazine', 'Magazine') }
				],
				'i18n.default_language': [
					{ value: 'en', label: t('settings.lang_en', 'English') },
					{ value: 'fr', label: t('settings.lang_fr', 'Francais') }
				],
				'media.storage.provider': [
					{ value: 'seaweedfs', label: t('settings.storage_seaweedfs', 'SeaweedFS') },
					{ value: 's3', label: t('settings.storage_s3', 'Amazon S3') },
					{ value: 'gcs', label: t('settings.storage_gcs', 'Google Cloud Storage') },
					{ value: 'azure', label: t('settings.storage_azure', 'Azure Blob') },
					{ value: 'local', label: t('settings.storage_local', 'Local Disk') }
				],
				'payment.currency': [
					{ value: 'NGN', label: t('settings.currency_ngn', 'Nigerian Naira (NGN)') },
					{ value: 'USD', label: t('settings.currency_usd', 'US Dollar (USD)') },
					{ value: 'GBP', label: t('settings.currency_gbp', 'British Pound (GBP)') },
					{ value: 'EUR', label: t('settings.currency_eur', 'Euro (EUR)') }
				]
			};
			return opts;
		})()
	);

	const multiSelectOptions: Record<string, { value: string; label: string }[]> = $derived(
		(() => {
			const opts: Record<string, { value: string; label: string }[]> = {
				'i18n.enabled_languages': [
					{ value: 'en', label: t('settings.lang_en', 'English') },
					{ value: 'fr', label: t('settings.lang_fr', 'Francais') },
					{ value: 'yo', label: t('settings.lang_yo', 'Yoruba') },
					{ value: 'ha', label: t('settings.lang_ha', 'Hausa') },
					{ value: 'ig', label: t('settings.lang_ig', 'Igbo') },
					{ value: 'pcm', label: t('settings.lang_pcm', 'Nigerian Pidgin') }
				],
				'media.allowed_video_types': [
					{ value: 'mp4', label: t('settings.video_mp4', 'MP4') },
					{ value: 'webm', label: t('settings.video_webm', 'WebM') },
					{ value: 'mov', label: t('settings.video_mov', 'MOV') },
					{ value: 'avi', label: t('settings.video_avi', 'AVI') },
					{ value: 'mkv', label: t('settings.video_mkv', 'MKV') }
				]
			};
			return opts;
		})()
	);

	const textareaKeys = new Set([
		'general.site_description',
		'cors.allowed_origins',
		'content.link_preview.blacklist'
	]);

	function isSelectKey(key: string): boolean {
		return key in selectOptions;
	}

	function isMultiSelectKey(key: string): boolean {
		return key in multiSelectOptions;
	}

	function isTextareaKey(key: string): boolean {
		return textareaKeys.has(key);
	}

	function getDisplayValue(setting: SystemSetting): string {
		const val = setting.value ?? '';
		if (val === '[]' || val === '{}') return '';
		if (val.startsWith('"') && val.endsWith('"')) return val.slice(1, -1);
		return val;
	}

	function getSelectValue(currentValue: string): string {
		return currentValue.startsWith('"') && currentValue.endsWith('"')
			? currentValue.slice(1, -1)
			: currentValue;
	}

	function getMultiSelectValues(currentValue: string): string[] {
		const val = currentValue.startsWith('"') && currentValue.endsWith('"')
			? currentValue.slice(1, -1)
			: currentValue;
		if (!val || val === '*') return [];
		return val
			.split(',')
			.map((s) => s.trim())
			.filter(Boolean);
	}

	function updateSetting(key: string, value: string) {
		currentSettings = currentSettings.map((s) =>
			s.key === key ? { ...s, value } : s
		);
	}

	function isSettingDirty(key: string): boolean {
		const current = currentSettings.find((s) => s.key === key);
		const original = originalSettings.find((s) => s.key === key);
		return current?.value !== original?.value;
	}

	function getOriginalValue(key: string): string {
		return originalSettings.find((s) => s.key === key)?.value ?? '';
	}

	async function saveAll() {
		isSaving = true;
		message = '';
		const changed = currentSettings.filter(
			(s, i) => s.value !== originalSettings[i]?.value
		);
		let failed = 0;
		for (const setting of changed) {
			const result = await api(`/control/system-settings/${setting.key}`, {
				method: 'PATCH',
				body: { value: setting.value }
			});
			if (!result.ok) failed++;
		}
		if (failed === 0) {
			originalSettings = structuredClone(currentSettings);
			message = t('settings.save_all_success', `${changed.length} setting(s) saved successfully.`);
			messageType = 'success';
		} else {
			message = t('settings.save_all_error', `Failed to save ${failed} of ${changed.length} setting(s).`);
			messageType = 'error';
		}
		isSaving = false;
	}

	function resetAll() {
		currentSettings = structuredClone(originalSettings);
		confirmResetAllOpen = false;
		message = t('settings.reset_all_success', 'All changes reverted.');
		messageType = 'success';
	}

	async function resetSetting(key: string) {
		message = '';
		const result = await api<{ key: string; value: string; default_value: string }>(
			`/control/system-settings/${key}/reset`,
			{ method: 'POST' }
		);
		if (result.ok) {
			const newValue = result.data.value;
			currentSettings = currentSettings.map((s) =>
				s.key === key ? { ...s, value: newValue } : s
			);
			originalSettings = originalSettings.map((s) =>
				s.key === key ? { ...s, value: newValue } : s
			);
			message = t('settings.reset_success', `Setting "${key}" reset to default value.`);
			messageType = 'success';
		} else {
			message = result.error || t('settings.reset_error', 'Failed to reset setting.');
			messageType = 'error';
		}
	}

	async function load() {
		isSaving = true;
		message = '';
		const result = await api<SystemSetting[]>('/control/system-settings');
		if (result.ok) {
			originalSettings = result.data;
			currentSettings = structuredClone(result.data);
		}
		isSaving = false;
	}

	beforeNavigate(({ cancel }) => {
		if (isDirty) {
			const confirmed = window.confirm(
				t('settings.unsaved_navigate', 'You have unsaved changes. Are you sure you want to leave?')
			);
			if (!confirmed) cancel();
		}
	});

	let categories = $derived(
		[
			...new Set(
				currentSettings.map((s) => {
					const ns = s.namespace ?? 'unknown';
					return ns.split('.')[0] ?? ns;
				})
			)
		].sort()
	);

	let categoryItems = $derived(
		categories.map((cat) => ({
			category: cat,
			count: currentSettings.filter((s) => {
				const ns = s.namespace ?? 'unknown';
				return (ns.split('.')[0] ?? ns) === cat;
			}).length
		}))
	);

	let filtered = $derived(
		currentSettings.filter((s) => {
			const matchesFilter = filter
				? (s.key ?? '').toLowerCase().includes(filter.toLowerCase()) ||
					(s.namespace ?? '').toLowerCase().includes(filter.toLowerCase()) ||
					(s.description ?? '').toLowerCase().includes(filter.toLowerCase())
				: true;
			const matchesCategory =
				activeTab === 'all'
					? true
					: (s.namespace ?? 'unknown').split('.')[0] === activeTab;
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
			.replace(/\./g, ' · ')
			.replace(/\b\w/g, (c) => c.toUpperCase());
	}

	function formatNamespace(ns: string): string {
		return (ns ?? '')
			.replace(/_/g, ' ')
			.replace(/\b\w/g, (c) => c.toUpperCase());
	}

	function isBoolValue(value: string): boolean {
		const unquoted = value.replace(/^"(.*)"$/, '$1').toLowerCase();
		return unquoted === 'true' || unquoted === 'false';
	}

	function getSettingComponentType(setting: SystemSetting): string {
		if (setting.type === 'bool' || isBoolValue(setting.value)) return 'switch';
		if (isSelectKey(setting.key)) return 'select';
		if (isMultiSelectKey(setting.key)) return 'multi-select';
		if (isTextareaKey(setting.key)) return 'textarea';
		if (setting.type === 'int' || setting.type === 'float') return 'number';
		return 'text';
	}

	function typeBadgeClass(setting: SystemSetting): string {
		const type = getSettingComponentType(setting);
		switch (type) {
			case 'switch':
				return 'bg-emerald-500/10 text-emerald-600 border-emerald-500/20';
			case 'select':
			case 'multi-select':
				return 'bg-amber-500/10 text-amber-600 border-amber-500/20';
			case 'textarea':
				return 'bg-purple-500/10 text-purple-600 border-purple-500/20';
			case 'number':
				return 'bg-blue-500/10 text-blue-600 border-blue-500/20';
			default:
				return 'bg-muted text-muted-foreground border-border';
		}
	}

	function typeLabel(setting: SystemSetting): string {
		const type = getSettingComponentType(setting);
		switch (type) {
			case 'switch':
				return 'bool';
			case 'select':
				return 'enum';
			case 'multi-select':
				return 'array';
			case 'textarea':
				return 'text';
			case 'number':
				return setting.type === 'float' ? 'float' : 'int';
			default:
				return 'string';
		}
	}

	function getCategoryIcon(cat: string): typeof Settings {
		return categoryIcons[cat] ?? Settings;
	}

	const inputClasses =
		'border-border-input bg-background text-foreground rounded-lg border px-3 py-2 text-sm transition-all hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus disabled:opacity-50';
	const selectTriggerClasses =
		'border-border-input bg-background text-foreground flex items-center justify-between rounded-lg border px-3 py-2 text-sm transition-all hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus w-56';
	const selectContentClasses =
		'border-border-input bg-background shadow-popover z-50 rounded-xl border p-1';
	const selectItemClasses =
		'data-highlighted:bg-accent rounded-lg flex h-9 cursor-pointer items-center px-3 text-sm outline-none';
</script>

<PageHeader
	title={t('settings.title', 'Settings')}
	description={t('settings.description', 'Platform configuration by category')}
/>

{#if message}
	<div
		class="mb-4 flex items-center gap-2 rounded-xl border px-4 py-3 text-sm {messageType ===
		'success'
			? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-600'
			: 'border-destructive/30 bg-destructive/10 text-destructive'}"
	>
		{#if messageType === 'success'}
			<Check class="size-4" />
		{/if}
		{message}
	</div>
{/if}

{#if isDirty}
	<div
		class="border-amber-500/30 bg-amber-500/5 mb-4 flex items-center gap-3 rounded-xl border px-4 py-3"
	>
		<AlertTriangle class="text-amber-500 size-4 flex-shrink-0" />
		<span class="text-foreground text-sm font-medium">
			{t('settings.unsaved_changes', `You have ${dirtyCount} unsaved change(s).`)}
		</span>
		<div class="flex-1"></div>
		<button
			class="border-border-input bg-background hover:bg-muted text-foreground flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors"
			onclick={() => {
				confirmResetAllOpen = true;
			}}
			disabled={isSaving}
		>
			<Undo2 class="size-3" />
			{t('settings.reset_all', 'Reset All')}
		</button>
		<button
			class="bg-accent text-accent-foreground hover:bg-accent/90 flex items-center gap-1.5 rounded-lg px-4 py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
			onclick={saveAll}
			disabled={isSaving || !isDirty}
		>
			{#if isSaving}
				<Loader2 class="size-3 animate-spin" />
			{:else}
				<Save class="size-3" />
			{/if}
			{t('settings.save_all', 'Save All')}
			{#if dirtyCount > 0}
				<span class="bg-accent-foreground/20 rounded-full px-1.5 py-0.5 text-[10px] font-bold">
					{dirtyCount}
				</span>
			{/if}
		</button>
	</div>
{/if}

<div class="mb-4 flex items-center gap-3">
	<SearchInput
		bind:value={filter}
		placeholder={t('settings.filter_placeholder', 'Filter settings...')}
		class="w-72"
	/>
	<button
		class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-1.5 rounded-9px border px-3 py-2 text-sm font-medium transition-colors"
		onclick={load}
		disabled={isSaving}
	>
		<RefreshCw class="size-4 {isSaving ? 'animate-spin' : ''}" />
		{t('common.refresh', 'Refresh')}
	</button>
	<div class="flex-1"></div>
	<span class="text-muted-foreground text-xs">
		{filtered.length} {t('settings.of', 'of')} {currentSettings.length} {t('settings.settings', 'settings')}
	</span>
</div>

{#if isSaving && currentSettings.length === 0}
	<div class="space-y-4">
		{#each Array(4) as _}
			<div class="border-border-card bg-background-alt h-48 animate-pulse rounded-xl border"></div>
		{/each}
	</div>
{:else}
	<Tabs.Root bind:value={activeTab}>
		<Tabs.List class="bg-muted mb-5 inline-flex flex-wrap gap-1 rounded-9px p-1">
			<Tabs.Trigger
				value="all"
				class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all"
			>
				{t('settings.all', 'All')} ({currentSettings.length})
			</Tabs.Trigger>
			{#each categoryItems as { category, count }}
				<Tabs.Trigger
					value={category}
					class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all"
				>
					{formatNamespace(category)} ({count})
				</Tabs.Trigger>
			{/each}
		</Tabs.List>

		<Tabs.Content value={activeTab} class="mt-0">
			{#if filtered.length === 0}
				<div class="border-border-card bg-background-alt rounded-9px border p-8 text-center">
					<Settings class="text-muted-foreground mx-auto mb-3 size-8" />
					<p class="text-muted-foreground text-sm">
						{t('settings.no_results', 'No settings match your filters.')}
					</p>
				</div>
			{:else}
				<div class="space-y-5">
					{#each grouped as [namespace, items]}
						{@const catKey = namespace.split('.')[0] ?? namespace}
						{@const CatIcon = getCategoryIcon(catKey)}
						{@const hasDirtyItems = items.some((s) => isSettingDirty(s.key))}
						<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
							<div class="border-border-card flex items-center justify-between border-b px-5 py-3.5">
								<div class="flex items-center gap-2.5">
									<div class="bg-accent flex size-8 items-center justify-center rounded-lg">
										<CatIcon class="text-accent-foreground size-4" />
									</div>
									<div>
										<h3 class="text-foreground text-sm font-semibold">
											{formatNamespace(namespace)}
										</h3>
										<p class="text-muted-foreground text-xs">
											{items.length} {items.length === 1 ? 'setting' : 'settings'}
										</p>
									</div>
								</div>
								<div class="flex items-center gap-2">
									{#if hasDirtyItems}
										<span
											class="bg-amber-500/10 text-amber-600 border-amber-500/20 rounded-full border px-2 py-0.5 text-[10px] font-medium"
										>
											{items.filter((s) => isSettingDirty(s.key)).length} modified
										</span>
									{/if}
								</div>
							</div>

							<div class="divide-border-card divide-y">
								{#each items as setting (setting.key)}
									{@const componentType = getSettingComponentType(setting)}
									{@const dirty = isSettingDirty(setting.key)}
									<div
										class="group px-5 py-4 transition-colors {dirty
											? 'bg-amber-500/5'
											: 'hover:bg-muted/30'}"
									>
										<div class="flex items-start gap-4">
											<div class="min-w-0 flex-1">
												<div class="flex items-center gap-2">
													<Label.Root
														for="setting-{setting.key}"
														class="text-foreground text-sm font-medium"
													>
														{formatSettingKey(setting.key)}
													</Label.Root>
													<span
														class="{typeBadgeClass(setting)} rounded-full border px-2 py-0.5 text-[10px] font-medium"
													>
														{typeLabel(setting)}
													</span>
													{#if componentType === 'select' || componentType === 'multi-select'}
														<span
															class="bg-accent text-accent-foreground rounded-full border px-2 py-0.5 text-[10px] font-medium"
														>
															dropdown
														</span>
													{:else if componentType === 'textarea'}
														<span
															class="bg-amber-500/10 text-amber-600 border-amber-500/20 rounded-full border px-2 py-0.5 text-[10px] font-medium"
														>
															multiline
														</span>
													{/if}
													{#if dirty}
														<span
															class="rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-[10px] font-medium text-amber-600"
														>
															unsaved
														</span>
													{/if}
												</div>
												{#if setting.description}
													<p class="text-muted-foreground mt-0.5 text-xs">
														{setting.description}
													</p>
												{/if}
												{#if dirty}
													<p class="mt-1 text-[10px] text-amber-600/70">
														Original: {getOriginalValue(setting.key)}
													</p>
												{/if}
											</div>
											<div class="flex items-center gap-3">
												{#if componentType === 'switch'}
													{@const isTrue =
														isBoolValue(setting.value) &&
														setting.value.replace(/^"(.*)"$/, '$1').toLowerCase() ===
															'true'}
													<div class="flex items-center gap-2">
														<Switch.Root
															id="setting-{setting.key}"
															checked={isTrue}
															onCheckedChange={(checked) =>
																updateSetting(setting.key, checked ? 'true' : 'false')}
															class="bg-muted data-[state=checked]:bg-emerald-500 peer inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full px-[3px] transition-colors disabled:cursor-not-allowed disabled:opacity-50"
														>
															<Switch.Thumb
																class="bg-background pointer-events-none block size-5 shrink-0 rounded-full shadow-lg transition-transform data-[state=checked]:translate-x-5 data-[state=unchecked]:translate-x-0"
															/>
														</Switch.Root>
														<span class="text-muted-foreground w-10 text-center text-xs">
															{isTrue ? 'On' : 'Off'}
														</span>
													</div>

												{:else if componentType === 'select'}
													<Select.Root
														type="single"
														value={getSelectValue(getDisplayValue(setting))}
														onValueChange={(v) => {
															if (v !== undefined) updateSetting(setting.key, v);
														}}
													>
														<Select.Trigger
															class={selectTriggerClasses}
															id="setting-{setting.key}"
														>
															<Select.Value placeholder="Select..." />
															<ChevronDown class="text-muted-foreground size-4" />
														</Select.Trigger>
														<Select.Portal>
															<Select.Content class={selectContentClasses}>
																<Select.Viewport class="p-1">
																	{#each selectOptions[setting.key] ?? [] as option}
																		<Select.Item
																			class={selectItemClasses}
																			value={option.value}
																			label={option.label}
																		>
																			{#snippet children({ selected })}
																				{option.label}
																				{#if selected}<span class="ml-auto">✓</span>{/if}
																			{/snippet}
																		</Select.Item>
																	{/each}
																</Select.Viewport>
															</Select.Content>
														</Select.Portal>
													</Select.Root>

												{:else if componentType === 'multi-select'}
													{@const selectedValues = getMultiSelectValues(
														getDisplayValue(setting)
													)}
													<div class="w-64">
														<div class="mb-1.5 flex flex-wrap gap-1">
															{#each selectedValues as val}
																{@const optionLabel =
																	(multiSelectOptions[setting.key] ?? []).find(
																		(o) => o.value === val
																	)?.label ?? val}
																<span
																	class="bg-accent text-accent-foreground inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs"
																>
																	{optionLabel}
																	<button
																		class="hover:text-destructive ml-0.5"
																		onclick={() =>
																			updateSetting(
																				setting.key,
																				selectedValues
																					.filter((v) => v !== val)
																					.join(',')
																			)}
																	>
																		×
																	</button>
																</span>
															{/each}
														</div>
														<Select.Root
															type="single"
															value=""
															onValueChange={(v) => {
																if (v && !selectedValues.includes(v)) {
																	updateSetting(
																		setting.key,
																		[...selectedValues, v].join(',')
																	);
																}
															}}
														>
															<Select.Trigger
																class="{selectTriggerClasses} w-full"
																id="setting-{setting.key}"
															>
																<Select.Value placeholder="Add language..." />
																<ChevronDown class="text-muted-foreground size-4" />
															</Select.Trigger>
															<Select.Portal>
																<Select.Content class={selectContentClasses}>
																	<Select.Viewport class="p-1">
																		{#each (multiSelectOptions[setting.key] ?? []).filter((o) => !selectedValues.includes(o.value)) as option}
																			<Select.Item
																				class={selectItemClasses}
																				value={option.value}
																				label={option.label}
																			>
																				{option.label}
																			</Select.Item>
																		{/each}
																	</Select.Viewport>
																</Select.Content>
															</Select.Portal>
														</Select.Root>
													</div>

												{:else if componentType === 'textarea'}
													<textarea
														id="setting-{setting.key}"
														rows={3}
														value={getDisplayValue(setting)}
														onchange={(e) => {
															const target = e.currentTarget as HTMLTextAreaElement;
															const val = target.value.trim();
															if ((val === '' || val === '[]') && setting.value?.startsWith('[')) {
																updateSetting(setting.key, '[]');
															} else {
																updateSetting(setting.key, val);
															}
														}}
														class="{inputClasses} w-72 resize-y font-mono text-xs"
													></textarea>

												{:else if componentType === 'number'}
													<input
														id="setting-{setting.key}"
														type="number"
														value={getDisplayValue(setting)}
														onchange={(e) => {
															const target = e.currentTarget as HTMLInputElement;
															updateSetting(setting.key, target.value);
														}}
														step={setting.type === 'float' ? '0.01' : '1'}
														class="{inputClasses} w-32"
													/>

												{:else}
													<input
														id="setting-{setting.key}"
														type="text"
														value={getDisplayValue(setting)}
														onchange={(e) => {
															const target = e.currentTarget as HTMLInputElement;
															updateSetting(setting.key, target.value);
														}}
														class="{inputClasses} w-56"
													/>
												{/if}
												<Tooltip.Root>
													<Tooltip.Trigger>
														<button
															class="text-muted-foreground hover:text-foreground hover:bg-muted rounded-lg p-1.5 transition-colors"
															onclick={() => {
																confirmResetSetting = setting;
																resetModalOpen = true;
															}}
														>
															<RotateCcw class="size-4" />
														</button>
													</Tooltip.Trigger>
													<Tooltip.Portal>
														<Tooltip.Content
															side="top"
															sideOffset={4}
															class="border-dark-10 bg-background shadow-popover z-50 w-[240px] rounded-xl border p-3 text-xs"
														>
															<p class="font-medium">Reset to default</p>
															<p class="text-muted-foreground mt-1">
																Current: <span class="font-medium">{setting.value}</span>
															</p>
															<p class="text-muted-foreground">
																Default: <span class="font-medium">{setting.default_value}</span>
															</p>
														</Tooltip.Content>
													</Tooltip.Portal>
												</Tooltip.Root>
											</div>
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
		? t(
				'settings.reset_consequence',
				`Reset '${confirmResetSetting.key}' to its default value. Current value: ${confirmResetSetting.value}`
			)
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

<ConfirmModal
	bind:open={confirmResetAllOpen}
	title={t('settings.reset_all_title', 'Reset All Changes')}
	consequence={t(
		'settings.reset_all_consequence',
		'Revert all unsaved changes back to their saved values.'
	)}
	confirmText={t('settings.reset_confirm', 'RESET')}
	onConfirm={resetAll}
/>
