<script lang="ts">
	import { beforeNavigate } from '$app/navigation';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { Switch, Tooltip, Label, Select } from 'bits-ui';
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

	let originalSettings = $state<SystemSetting[]>([]);
	let currentSettings = $state<SystemSetting[]>([]);
	let isDirty = $derived(
		JSON.stringify(currentSettings) !== JSON.stringify(originalSettings)
	);
	let dirtyCount = $derived(
		currentSettings.filter(
			(s, i) => s.value !== originalSettings[i]?.value
		).length
	);

	let isSaving = $state(false);
	let activeCategory = $state('all');
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
		content: Sliders,
		auth: Shield,
		spaces: Globe,
		moderation: Shield,
		wallet: CreditCard,
		api: Server,
		ads: Sliders,
		ai: Cpu
	};

	const categoryNames: Record<string, string> = {
		auth: 'Authentication',
		feed: 'Feed & Content',
		content: 'Feed & Content',
		spaces: 'Spaces',
		moderation: 'Moderation',
		security: 'Security',
		wallet: 'Wallet',
		api: 'API Rate Limits',
		cors: 'CORS',
		ads: 'Ads',
		ai: 'AI',
		general: 'General',
		advanced: 'Advanced',
		appearance: 'Appearance',
		media: 'Media',
		i18n: 'Languages',
		payment: 'Payment',
		notification: 'Notifications',
		email: 'Email'
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

	// validateSettingValue checks a single changed setting's value against its
	// declared type before it is sent to the backend. Returns a localized error
	// message (with the setting key interpolated) or null when valid.
	function validateSettingValue(setting: SystemSetting): string | null {
		const raw = setting.value ?? '';
		const key = setting.key;
		switch (setting.type) {
			case 'int':
			case 'float': {
				const trimmed = raw.trim();
				if (trimmed === '' || Number.isNaN(Number(trimmed))) {
					return t('settings.validation_number', '"{key}" must be a valid number.').replace('{key}', key);
				}
				const num = Number(trimmed);
				if (setting.type === 'int' && !Number.isInteger(num)) {
					return t('settings.validation_integer', '"{key}" must be a whole number.').replace('{key}', key);
				}
				const min = (setting as unknown as Record<string, unknown>).min;
				const max = (setting as unknown as Record<string, unknown>).max;
				if (typeof min === 'number' && num < min) {
					return t('settings.validation_number', '"{key}" must be ≥ {min}.').replace('{key}', key).replace('{min}', String(min));
				}
				if (typeof max === 'number' && num > max) {
					return t('settings.validation_number', '"{key}" must be ≤ {max}.').replace('{key}', key).replace('{max}', String(max));
				}
				return null;
			}
			case 'bool': {
				const unquoted = raw.replace(/^"(.*)"$/, '$1').toLowerCase();
				if (unquoted !== 'true' && unquoted !== 'false') {
					return t('settings.validation_bool', '"{key}" must be true or false.').replace('{key}', key);
				}
				return null;
			}
			default: {
				const trimmed = raw.trim();
				if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
					try {
						JSON.parse(trimmed);
					} catch {
						return t('settings.validation_json', '"{key}" is not valid JSON.').replace('{key}', key);
					}
				}
				return null;
			}
		}
	}

	async function saveAll() {
		isSaving = true;
		message = '';
		const changed = currentSettings.filter(
			(s, i) => s.value !== originalSettings[i]?.value
		);
		// Validate every changed value before sending a single request.
		for (const setting of changed) {
			const error = validateSettingValue(setting);
			if (error) {
				message = error;
				messageType = 'error';
				isSaving = false;
				return;
			}
		}
		let failed = 0;
		const results = await Promise.all(
			changed.map((setting) =>
				api(`/control/system-settings/${setting.key}`, {
					method: 'PATCH',
					body: { value: setting.value }
				})
			)
		);
		for (const result of results) {
			if (!result.ok) failed++;
		}
		if (failed === 0) {
			originalSettings = JSON.parse(JSON.stringify(currentSettings));
			message = t('settings.save_all_success', `${changed.length} setting(s) saved successfully.`);
			messageType = 'success';
		} else {
			message = t('settings.save_all_error', `Failed to save ${failed} of ${changed.length} setting(s).`);
			messageType = 'error';
		}
		isSaving = false;
	}

	function resetAll() {
		currentSettings = JSON.parse(JSON.stringify(originalSettings));
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
			originalSettings = JSON.parse(JSON.stringify(result.data));
			currentSettings = JSON.parse(JSON.stringify(result.data));
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

	load();

	let categories = $derived(
		[
			...new Set(
				currentSettings.map((s) => getSettingCategory(s))
			)
		].sort()
	);

	let categoryItems = $derived(
		categories.map((cat) => ({
			category: cat,
			count: currentSettings.filter((s) => getSettingCategory(s) === cat).length
		}))
	);

	let filtered = $derived(
		currentSettings.filter((s) => {
			const matchesCategory =
				activeCategory === 'all'
					? true
					: getSettingCategory(s) === activeCategory;
			return matchesCategory;
		})
	);

	let grouped = $derived(
		Object.entries(
			filtered.reduce(
				(acc, s) => {
					const ns = getSettingCategory(s);
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
		if (categoryNames[ns]) return categoryNames[ns];
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

	function getCategoryIcon(cat: string): typeof Settings {
		return categoryIcons[cat] ?? Settings;
	}

	function getSettingCategory(setting: SystemSetting): string {
		return (setting.key ?? '').split('.')[0] || 'general';
	}

	const inputClasses =
		'border-border-input bg-background text-foreground rounded-lg border px-3 py-1.5 text-sm transition-all hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus disabled:opacity-50';
	const selectTriggerClasses =
		'border-border-input bg-background text-foreground flex items-center justify-between rounded-lg border px-3 py-1.5 text-sm transition-all hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus w-44';
	const selectContentClasses =
		'border-border-input bg-background shadow-popover z-50 rounded-xl border p-1';
	const selectItemClasses =
		'data-highlighted:bg-accent rounded-lg flex h-9 cursor-pointer items-center px-3 text-sm outline-none';

	const sensitiveKeys = new Set([
		'email.smtp_password',
		'email.smtp_username',
		'payment.secret_key',
		'payment.publishable_key',
		'media.storage.access_key',
		'media.storage.secret_key',
		'security.jwt_secret',
		'security.encryption_key',
		'oauth.client_secret',
		'oauth.google_client_secret',
		'oauth.github_client_secret'
	]);

	function isSensitiveKey(key: string): boolean {
		return sensitiveKeys.has(key) || key.includes('secret') || key.includes('password') || key.includes('token') || key.includes('api_key');
	}

	let revealedKeys = $state<Set<string>>(new Set());

	function toggleReveal(key: string) {
		const next = new Set(revealedKeys);
		if (next.has(key)) {
			next.delete(key);
		} else {
			next.add(key);
		}
		revealedKeys = next;
	}

	function getMaskedValue(value: string): string {
		if (!value) return '';
		return '•'.repeat(Math.min(value.length, 16));
	}
</script>

<div class="flex h-full gap-6">
	<!-- Sidebar Navigation -->
	<nav class="hidden w-56 shrink-0 md:block">
		<div class="sticky top-4">
			<h2 class="text-foreground mb-4 text-sm font-semibold">
				{t('settings.title', 'Settings')}
			</h2>
			<div class="space-y-0.5">
				<button
					class="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors {activeCategory === 'all'
						? 'bg-accent text-foreground font-medium'
						: 'text-muted-foreground hover:bg-muted/50 hover:text-foreground'}"
					onclick={() => (activeCategory = 'all')}
				>
					<Settings class="size-4 shrink-0" />
					<span class="flex-1 text-left">{t('settings.all', 'All')}</span>
					<span class="text-muted-foreground text-xs">{currentSettings.length}</span>
				</button>
				{#each categoryItems as { category, count }}
					{@const Icon = getCategoryIcon(category)}
					<button
						class="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors {activeCategory === category
							? 'bg-accent text-foreground font-medium'
							: 'text-muted-foreground hover:bg-muted/50 hover:text-foreground'}"
						onclick={() => (activeCategory = category)}
					>
						<Icon class="size-4 shrink-0" />
						<span class="flex-1 text-left">{formatNamespace(category)}</span>
						<span class="text-muted-foreground text-xs">{count}</span>
					</button>
				{/each}
			</div>
		</div>
	</nav>

	<!-- Main Content -->
	<div class="min-w-0 flex-1">
		<!-- Mobile category selector -->
		<div class="mb-4 md:hidden">
			<Select.Root
				type="single"
				value={activeCategory}
				onValueChange={(v) => {
					if (v !== undefined) activeCategory = v;
				}}
			>
				<Select.Trigger
					class="border-border-input bg-background text-foreground w-full rounded-lg border px-3 py-2 text-sm"
				>
					<Select.Value placeholder={t('settings.all', 'All')} />
				</Select.Trigger>
				<Select.Portal>
					<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-xl border">
						<Select.Viewport class="p-1">
							<Select.Item
								class="data-highlighted:bg-accent rounded-lg px-3 py-1.5 text-sm outline-none"
								value="all"
								label={t('settings.all', 'All')}
							>
								{t('settings.all', 'All')} ({currentSettings.length})
							</Select.Item>
							{#each categoryItems as { category, count }}
								<Select.Item
									class="data-highlighted:bg-accent rounded-lg px-3 py-1.5 text-sm outline-none"
									value={category}
									label={formatNamespace(category)}
								>
									{formatNamespace(category)} ({count})
								</Select.Item>
							{/each}
						</Select.Viewport>
					</Select.Content>
				</Select.Portal>
			</Select.Root>
		</div>

		<!-- Page Header -->
		<div class="mb-4 flex items-center justify-between">
			<div>
				<h1 class="text-foreground text-lg font-semibold">
					{activeCategory === 'all'
						? t('settings.all_settings', 'All Settings')
						: formatNamespace(activeCategory)}
				</h1>
				<p class="text-muted-foreground text-xs">
					{filtered.length} {t('settings.of', 'of')} {currentSettings.length} {t('settings.settings', 'settings')}
				</p>
			</div>
			<button
				class="border-border-input bg-background-alt hover:bg-muted text-foreground flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors"
				onclick={load}
				disabled={isSaving}
			>
				<RefreshCw class="size-3 {isSaving ? 'animate-spin' : ''}" />
				{t('common.refresh', 'Refresh')}
			</button>
		</div>

		<!-- Message banner -->
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

		<!-- Unsaved changes banner -->
		{#if isDirty}
			<div
				class="border-amber-500/30 bg-amber-500/5 mb-4 flex items-center gap-3 rounded-xl border px-4 py-3"
			>
				<AlertTriangle class="text-amber-500 size-4 shrink-0" />
				<span class="text-foreground text-sm font-medium">
					{t('settings.unsaved_changes', `You have ${dirtyCount} unsaved change(s).`)}
				</span>
				<div class="flex-1"></div>
				<button
					class="border-border-input bg-background hover:bg-muted text-foreground flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors"
					onclick={() => { confirmResetAllOpen = true; }}
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

		<!-- Settings content -->
		{#if isSaving && currentSettings.length === 0}
			<div class="space-y-4">
				{#each Array(3) as _}
					<div class="border-border-card bg-background-alt h-32 animate-pulse rounded-xl border"></div>
				{/each}
			</div>
		{:else if filtered.length === 0}
			<div class="border-border-card bg-background-alt rounded-xl border p-12 text-center">
				<Settings class="text-muted-foreground mx-auto mb-3 size-8" />
				<p class="text-muted-foreground text-sm">
					{t('settings.no_results', 'No settings match your filters.')}
				</p>
			</div>
		{:else if activeCategory !== 'all'}
			<!-- Single category: flat list of settings -->
			<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
				<div class="divide-border-card divide-y">
					{#each filtered as setting (setting.key)}
						{@const componentType = getSettingComponentType(setting)}
						{@const dirty = isSettingDirty(setting.key)}
						<div class="flex items-center gap-4 px-5 py-3.5 transition-colors {dirty ? 'bg-amber-500/5' : 'hover:bg-muted/30'}">
							<!-- Label + description -->
							<div class="min-w-0 flex-1">
								<div class="flex items-center gap-2">
									<Label.Root for="setting-{setting.key}" class="text-foreground text-sm font-medium">
										{formatSettingKey(setting.key)}
									</Label.Root>
									{#if dirty}
										<span class="rounded-full border border-amber-500/30 bg-amber-500/10 px-1.5 py-0.5 text-[9px] font-medium text-amber-600">
											unsaved
										</span>
									{/if}
								</div>
								{#if setting.description}
									<p class="text-muted-foreground mt-0.5 text-xs">{setting.description}</p>
								{/if}
								{#if dirty}
									<p class="mt-0.5 text-[10px] text-amber-600/70">
										{t('settings.original_value', 'Original:')} {isSensitiveKey(setting.key) ? getMaskedValue(getOriginalValue(setting.key)) : getOriginalValue(setting.key)}
									</p>
								{/if}
							</div>

							<!-- Control -->
							<div class="flex items-center gap-2">
								{#if componentType === 'switch'}
									{@const isTrue =
										isBoolValue(setting.value) &&
										setting.value.replace(/^"(.*)"$/, '$1').toLowerCase() === 'true'}
									<div class="flex items-center gap-2">
										<span class="text-muted-foreground w-8 text-right text-xs">
											{isTrue ? 'On' : 'Off'}
										</span>
										<Switch.Root
											id="setting-{setting.key}"
											checked={isTrue}
											onCheckedChange={(checked) =>
												updateSetting(setting.key, checked ? 'true' : 'false')}
											class="bg-muted data-[state=checked]:bg-emerald-500 peer inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full px-[2px] transition-colors disabled:cursor-not-allowed disabled:opacity-50"
										>
											<Switch.Thumb
												class="bg-background pointer-events-none block size-4 shrink-0 rounded-full shadow-lg transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0"
											/>
										</Switch.Root>
									</div>

								{:else if componentType === 'select'}
									<Select.Root
										type="single"
										value={getSelectValue(getDisplayValue(setting))}
										onValueChange={(v) => {
											if (v !== undefined) updateSetting(setting.key, v);
										}}
									>
										<Select.Trigger class={selectTriggerClasses} id="setting-{setting.key}">
											<Select.Value placeholder="Select..." />
											<ChevronDown class="text-muted-foreground size-3.5" />
										</Select.Trigger>
										<Select.Portal>
											<Select.Content class={selectContentClasses}>
												<Select.Viewport class="p-1">
													{#each selectOptions[setting.key] ?? [] as option}
														<Select.Item class={selectItemClasses} value={option.value} label={option.label}>
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
									{@const selectedValues = getMultiSelectValues(getDisplayValue(setting))}
									<div class="w-56">
										<div class="mb-1 flex flex-wrap gap-1">
											{#each selectedValues as val}
												{@const optionLabel = (multiSelectOptions[setting.key] ?? []).find((o) => o.value === val)?.label ?? val}
												<span class="bg-accent text-accent-foreground inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px]">
													{optionLabel}
													<button class="hover:text-destructive" onclick={() => updateSetting(setting.key, selectedValues.filter((v) => v !== val).join(','))}>×</button>
												</span>
											{/each}
										</div>
										<Select.Root type="single" value="" onValueChange={(v) => {
											if (v && !selectedValues.includes(v)) {
												updateSetting(setting.key, [...selectedValues, v].join(','));
											}
										}}>
											<Select.Trigger class="{selectTriggerClasses} w-full" id="setting-{setting.key}">
												<Select.Value placeholder="Add..." />
												<ChevronDown class="text-muted-foreground size-3.5" />
											</Select.Trigger>
											<Select.Portal>
												<Select.Content class={selectContentClasses}>
													<Select.Viewport class="p-1">
														{#each (multiSelectOptions[setting.key] ?? []).filter((o) => !selectedValues.includes(o.value)) as option}
															<Select.Item class={selectItemClasses} value={option.value} label={option.label}>
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
										rows={2}
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
										class="{inputClasses} w-64 resize-y font-mono text-xs"
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
										class="{inputClasses} w-28"
									/>

								{:else if isSensitiveKey(setting.key)}
									<div class="flex items-center gap-1">
										<input
											id="setting-{setting.key}"
											type={revealedKeys.has(setting.key) ? 'text' : 'password'}
											value={getDisplayValue(setting)}
											onchange={(e) => {
												const target = e.currentTarget as HTMLInputElement;
												updateSetting(setting.key, target.value);
											}}
											class="{inputClasses} w-44 font-mono text-xs"
										/>
										<button
											type="button"
											class="text-muted-foreground hover:text-foreground rounded p-1 transition-colors"
											onclick={() => toggleReveal(setting.key)}
											aria-label={revealedKeys.has(setting.key) ? 'Hide value' : 'Reveal value'}
										>
											{revealedKeys.has(setting.key) ? '🙈' : '👁'}
										</button>
									</div>

								{:else}
									<input
										id="setting-{setting.key}"
										type="text"
										value={getDisplayValue(setting)}
										onchange={(e) => {
											const target = e.currentTarget as HTMLInputElement;
											updateSetting(setting.key, target.value);
										}}
										class="{inputClasses} w-44"
									/>
								{/if}

								<!-- Reset button -->
								<Tooltip.Root>
									<Tooltip.Trigger>
										<button
											class="text-muted-foreground hover:text-foreground hover:bg-muted rounded-lg p-1 transition-colors"
											onclick={() => {
												confirmResetSetting = setting;
												resetModalOpen = true;
											}}
										>
											<RotateCcw class="size-3.5" />
										</button>
									</Tooltip.Trigger>
									<Tooltip.Portal>
										<Tooltip.Content
											side="top"
											sideOffset={4}
											class="border-dark-10 bg-background shadow-popover z-50 w-[220px] rounded-xl border p-3 text-xs"
										>
											<p class="font-medium">Reset to default</p>
											<p class="text-muted-foreground mt-1">
												Current: <span class="font-medium">{isSensitiveKey(setting.key) ? getMaskedValue(setting.value) : setting.value}</span>
											</p>
											<p class="text-muted-foreground">
												Default: <span class="font-medium">{isSensitiveKey(setting.key) ? getMaskedValue(setting.default_value) : setting.default_value}</span>
											</p>
										</Tooltip.Content>
									</Tooltip.Portal>
								</Tooltip.Root>
							</div>
						</div>
					{/each}
				</div>
			</div>
		{:else}
			<!-- All categories: grouped by category -->
			<div class="space-y-6">
				{#each categoryItems as { category, count }}
					{@const CatIcon = getCategoryIcon(category)}
					{@const catSettings = currentSettings.filter((s) => getSettingCategory(s) === category)}
					{@const hasDirtyItems = catSettings.some((s) => isSettingDirty(s.key))}
					<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
						<!-- Category header -->
						<div class="border-border-card flex items-center justify-between border-b px-5 py-3">
							<div class="flex items-center gap-2.5">
								<div class="bg-accent flex size-7 items-center justify-center rounded-lg">
									<CatIcon class="text-accent-foreground size-3.5" />
								</div>
								<div>
									<h3 class="text-foreground text-sm font-semibold">
										{formatNamespace(category)}
									</h3>
									<p class="text-muted-foreground text-[11px]">
										{count} {count === 1 ? 'setting' : 'settings'}
									</p>
								</div>
							</div>
							{#if hasDirtyItems}
								<span class="bg-amber-500/10 text-amber-600 rounded-full px-2 py-0.5 text-[10px] font-medium">
									{catSettings.filter((s) => isSettingDirty(s.key)).length} modified
								</span>
							{/if}
							<button
								class="text-muted-foreground hover:text-foreground text-xs font-medium transition-colors"
								onclick={() => (activeCategory = category)}
							>
								View all →
							</button>
						</div>

						<!-- Settings rows -->
						<div class="divide-border-card divide-y">
							{#each catSettings.slice(0, 5) as setting (setting.key)}
								{@const componentType = getSettingComponentType(setting)}
								{@const dirty = isSettingDirty(setting.key)}
								<div class="flex items-center gap-4 px-5 py-3 transition-colors {dirty ? 'bg-amber-500/5' : 'hover:bg-muted/30'}">
									<div class="min-w-0 flex-1">
										<div class="flex items-center gap-2">
											<Label.Root for="setting-{setting.key}" class="text-foreground text-sm font-medium">
												{formatSettingKey(setting.key)}
											</Label.Root>
											{#if dirty}
												<span class="rounded-full border border-amber-500/30 bg-amber-500/10 px-1.5 py-0.5 text-[9px] font-medium text-amber-600">
													unsaved
												</span>
											{/if}
										</div>
										{#if setting.description}
											<p class="text-muted-foreground mt-0.5 text-xs">{setting.description}</p>
										{/if}
									</div>
									<div class="flex items-center gap-2">
										{#if componentType === 'switch'}
											{@const isTrue = isBoolValue(setting.value) && setting.value.replace(/^"(.*)"$/, '$1').toLowerCase() === 'true'}
											<div class="flex items-center gap-2">
												<span class="text-muted-foreground w-8 text-right text-xs">{isTrue ? 'On' : 'Off'}</span>
												<Switch.Root
													id="setting-{setting.key}"
													checked={isTrue}
													onCheckedChange={(checked) => updateSetting(setting.key, checked ? 'true' : 'false')}
													class="bg-muted data-[state=checked]:bg-emerald-500 peer inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full px-[2px] transition-colors disabled:cursor-not-allowed disabled:opacity-50"
												>
													<Switch.Thumb class="bg-background pointer-events-none block size-4 shrink-0 rounded-full shadow-lg transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0" />
												</Switch.Root>
											</div>
										{:else if componentType === 'select'}
											<Select.Root type="single" value={getSelectValue(getDisplayValue(setting))} onValueChange={(v) => { if (v !== undefined) updateSetting(setting.key, v); }}>
												<Select.Trigger class={selectTriggerClasses} id="setting-{setting.key}">
													<Select.Value placeholder="Select..." />
													<ChevronDown class="text-muted-foreground size-3.5" />
												</Select.Trigger>
												<Select.Portal>
													<Select.Content class={selectContentClasses}>
														<Select.Viewport class="p-1">
															{#each selectOptions[setting.key] ?? [] as option}
																<Select.Item class={selectItemClasses} value={option.value} label={option.label}>
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
										{:else if isSensitiveKey(setting.key)}
											<div class="flex items-center gap-1">
												<input id="setting-{setting.key}" type={revealedKeys.has(setting.key) ? 'text' : 'password'} value={getDisplayValue(setting)} onchange={(e) => updateSetting(setting.key, (e.currentTarget as HTMLInputElement).value)} class="{inputClasses} w-44 font-mono text-xs" />
												<button type="button" class="text-muted-foreground hover:text-foreground rounded p-1 transition-colors" onclick={() => toggleReveal(setting.key)} aria-label={revealedKeys.has(setting.key) ? 'Hide' : 'Reveal'}>
													{revealedKeys.has(setting.key) ? '🙈' : '👁'}
												</button>
											</div>
										{:else}
											<input id="setting-{setting.key}" type="text" value={getDisplayValue(setting)} onchange={(e) => updateSetting(setting.key, (e.currentTarget as HTMLInputElement).value)} class="{inputClasses} w-44" />
										{/if}
										<Tooltip.Root>
											<Tooltip.Trigger>
												<button class="text-muted-foreground hover:text-foreground hover:bg-muted rounded-lg p-1 transition-colors" onclick={() => { confirmResetSetting = setting; resetModalOpen = true; }}>
													<RotateCcw class="size-3.5" />
												</button>
											</Tooltip.Trigger>
											<Tooltip.Portal>
												<Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 w-[220px] rounded-xl border p-3 text-xs">
													<p class="font-medium">Reset to default</p>
													<p class="text-muted-foreground mt-1">Current: <span class="font-medium">{isSensitiveKey(setting.key) ? getMaskedValue(setting.value) : setting.value}</span></p>
													<p class="text-muted-foreground">Default: <span class="font-medium">{isSensitiveKey(setting.key) ? getMaskedValue(setting.default_value) : setting.default_value}</span></p>
												</Tooltip.Content>
											</Tooltip.Portal>
										</Tooltip.Root>
									</div>
								</div>
							{/each}
							{#if catSettings.length > 5}
								<button
									class="text-accent-foreground hover:bg-muted/50 w-full px-5 py-2.5 text-left text-xs font-medium transition-colors"
									onclick={() => (activeCategory = category)}
								>
									View all {catSettings.length} settings →
								</button>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>

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
