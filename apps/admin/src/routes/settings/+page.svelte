<script lang="ts">
	import { beforeNavigate, goto, replaceState } from '$app/navigation';
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { Select } from '$lib/components/ui/select';
	import { RefreshCw, Save, Check, Upload, RotateCcw, Undo2, AlertTriangle, ArrowRight, Image as ImageIcon } from 'lucide-svelte';
	import { Switch } from "bits-ui";
	import { settingsAdminApi, api } from '$lib/api';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import AuthBrandSlidesEditor from '$lib/components/settings/AuthBrandSlidesEditor.svelte';
	import MediaPickerModal from '$lib/components/ui/MediaPickerModal.svelte';
	import { convertImageToModernFormat } from '$lib/utils/imageConvert';

	const canManage = $derived(hasPermission('admin.settings'));

	let saving = $state(false);
	let savedCategory = $state('');
	let loading = $state(true);
	let error = $state('');
	let messageType = $state<'success' | 'error' | ''>('');

	interface SettingDef {
		key: string;
		label: string;
		type: string;
		value?: unknown;
		options?: string[];
		accept?: string;
		placeholder?: string;
		description?: string;
		masked?: boolean;
	}

	interface SettingCategory {
		key: string;
		label: string;
		settings: SettingDef[];
	}

	let categories = $state<SettingCategory[]>([]);
	let originalCategories = $state<SettingCategory[]>([]);
	let activeTab = $state(page.url.searchParams.get('category') ?? '');

	const CATEGORY_ICONS: Record<string, string> = {
		general: '⚙️',
		auth: '🔐',
		feed: '📰',
		security: '🔒',
		appearance: '🎨',
		users: '👤',
		content: '📝',
		media: '🖼️',
		seo: '🔍',
		ai: '🤖',
		spaces: '🏠',
		moderation: '🛡️',
		wallet: '💰',
		api: '🔌',
		ads: '📢',
		notifications: '🔔',
		tickets: '🎫',
		audit: '📋',
		i18n: '🌐',
		gdpr: '📜',
		search: '🔎',
		affiliate: '🤝'
	};

	const CATEGORY_DESCRIPTIONS: Record<string, string> = {
		general: t('settings.cat_general_desc', 'Basic platform settings'),
		auth: t('settings.cat_auth_desc', 'Password, session, and 2FA settings'),
		feed: t('settings.cat_feed_desc', 'Feed rendering and fan-out settings'),
		security: t('settings.cat_security_desc', 'Authentication and security settings'),
		appearance: t('settings.cat_appearance_desc', 'Branding, logo, favicon, and visual theme'),
		users: t('settings.cat_users_desc', 'User profile and registration settings'),
		content: t('settings.cat_content_desc', 'Content creation and moderation settings'),
		media: t('settings.cat_media_desc', 'Image processing and upload settings'),
		seo: t('settings.cat_seo_desc', 'Search engine optimization and metadata'),
		ai: t('settings.cat_ai_desc', 'AI-powered features configuration'),
		spaces: t('settings.cat_spaces_desc', 'Space hierarchy and pinning settings'),
		moderation: t('settings.cat_moderation_desc', 'Content moderation and spam detection'),
		wallet: t('settings.cat_wallet_desc', 'Points and credit conversion settings'),
		api: t('settings.cat_api_desc', 'API rate limits and key management'),
		ads: t('settings.cat_ads_desc', 'Advertising and campaign settings'),
		notifications: t('settings.cat_notifications_desc', 'Notification delivery settings'),
		tickets: t('settings.cat_tickets_desc', 'Support ticket SLA and auto-close'),
		audit: t('settings.cat_audit_desc', 'Audit log retention settings'),
		i18n: t('settings.cat_i18n_desc', 'Language and localization settings'),
		gdpr: t('settings.cat_gdpr_desc', 'Data protection and deletion settings'),
		search: t('settings.cat_search_desc', 'Search and autocomplete settings'),
		affiliate: t('settings.cat_affiliate_desc', 'Affiliate program commission settings')
	};

	// Tab definitions derived from the loaded categories. The count badge shows
	// the total number of settings in each category so the user can see the
	// category's size at a glance.
	let tabs = $derived(
		categories.map((cat) => ({
			value: cat.key,
			label: cat.label,
			icon: undefined,
			count: cat.settings.length
		}))
	);

	// Track toggle states per category+key
	let toggleStates = $state<Record<string, boolean>>({});
	let originalToggleStates = $state<Record<string, boolean>>({});

	// Dirty state tracking
	let isDirty = $derived(
		JSON.stringify(categories) !== JSON.stringify(originalCategories) ||
		JSON.stringify(toggleStates) !== JSON.stringify(originalToggleStates)
	);

	let dirtyCount = $derived(() => {
		let count = 0;
		for (const cat of categories) {
			const origCat = originalCategories.find(c => c.key === cat.key);
			if (!origCat) continue;
			for (const s of cat.settings) {
				const origSetting = origCat.settings.find(os => os.key === s.key);
				if (!origSetting) continue;
				if (s.type === 'toggle') {
					const id = `${cat.key}.${s.key}`;
					if (toggleStates[id] !== originalToggleStates[id]) count++;
				} else if (JSON.stringify(s.value) !== JSON.stringify(origSetting.value)) {
					count++;
				}
			}
		}
		return count;
	});

	// Unsaved changes warning — uses a modal instead of window.confirm
	let showUnsavedDialog = $state(false);
	let pendingNavigationUrl = $state('');

	beforeNavigate(({ to, cancel }) => {
		if (isDirty) {
			cancel();
			pendingNavigationUrl = to?.url?.pathname ?? '/';
			showUnsavedDialog = true;
		}
	});

	function confirmLeaveWithUnsaved() {
		showUnsavedDialog = false;
		if (pendingNavigationUrl) {
			goto(pendingNavigationUrl);
		}
	}

	function isSettingDirty(catKey: string, settingKey: string, settingType: string): boolean {
		const cat = categories.find(c => c.key === catKey);
		const origCat = originalCategories.find(c => c.key === catKey);
		if (!cat || !origCat) return false;

		const setting = cat.settings.find(s => s.key === settingKey);
		const origSetting = origCat.settings.find(s => s.key === settingKey);
		if (!setting || !origSetting) return false;

		if (settingType === 'toggle') {
			const id = `${catKey}.${settingKey}`;
			return toggleStates[id] !== originalToggleStates[id];
		}
		return JSON.stringify(setting.value) !== JSON.stringify(origSetting.value);
	}

	function getOriginalValue(catKey: string, settingKey: string, settingType: string): unknown {
		const origCat = originalCategories.find(c => c.key === catKey);
		if (!origCat) return '';
		const origSetting = origCat.settings.find(s => s.key === settingKey);
		if (!origSetting) return '';

		if (settingType === 'toggle') {
			const id = `${catKey}.${settingKey}`;
			return originalToggleStates[id] ?? false;
		}
		return origSetting.value;
	}

	function getDisplayValue(value: unknown): string {
		if (value === null || value === undefined) return '';
		if (value === '[]' || value === '{}') return '';
		if (Array.isArray(value)) {
			if (value.length === 0) return '';
			return value.length + ' ' + t('settings.items_count', 'items');
		}
		const str = String(value);
		// Check if the string is a JSON-encoded array
		if (str.startsWith('[') && str.endsWith(']')) {
			try {
				const parsed = JSON.parse(str);
				if (Array.isArray(parsed)) {
					return parsed.length + ' ' + t('settings.items_count', 'items');
				}
			} catch {
				// Not valid JSON, fall through to display as-is
			}
		}
		if (str.startsWith('"') && str.endsWith('"')) return str.slice(1, -1);
		return str;
	}

	// Validation
	function validateSettingValue(setting: SettingDef): string | null {
		const raw = String(setting.value ?? '');
		if (setting.type === 'number') {
			const trimmed = raw.trim();
			if (trimmed === '' || Number.isNaN(Number(trimmed))) {
				return t('settings.validation_number', '"{key}" must be a valid number.').replace('{key}', setting.label);
			}
			return null;
		}
		return null;
	}

	// Sync the active tab to the URL as ?category={tab_name} so the view is
	// shareable/bookmarkable and a page reload restores the exact tab. Uses
	// replaceState (not goto) so tab switches don't trigger the beforeNavigate
	// unsaved-changes guard — the user is still on the settings page.
	function syncTabToUrl(tab: string) {
		if (typeof window === 'undefined') return;
		const params = new URLSearchParams(page.url.searchParams.toString());
		if (tab) params.set('category', tab);
		else params.delete('category');
		const qs = params.toString();
		const url = `${page.url.pathname}${qs ? `?${qs}` : ''}`;
		try {
			replaceState(url, {});
		} catch {
			// ignore URL sync failures in non-browser contexts
		}
	}

	function selectTab(tab: string) {
		activeTab = tab;
		syncTabToUrl(tab);
	}

	async function loadSettings() {
		loading = true;
		error = '';
		messageType = '';
		try {
			const result = await settingsAdminApi.list();
			if (result.ok) {
				categories = (result.data as SettingCategory[]) ?? [];
				originalCategories = JSON.parse(JSON.stringify(categories));
				// Initialize toggle states
				for (const cat of categories) {
					for (const s of cat.settings) {
						if (s.type === 'toggle') {
							const id = `${cat.key}.${s.key}`;
							toggleStates[id] = Boolean(s.value);
						}
					}
				}
				originalToggleStates = JSON.parse(JSON.stringify(toggleStates));
			// Select the active tab once categories are loaded. Prefer the
			// ?category= URL param if it matches a loaded category, otherwise
			// fall back to the first category and sync the URL.
			if (categories.length > 0) {
				const urlCat = page.url.searchParams.get('category') ?? '';
				const matched = categories.find((c) => c.key === urlCat);
				if (matched) {
					activeTab = urlCat;
				} else if (!activeTab || !categories.find((c) => c.key === activeTab)) {
					activeTab = categories[0].key;
					syncTabToUrl(activeTab);
				}
			}
			} else {
				error = result.error;
				messageType = 'error';
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('settings.load_failed', 'Failed to load settings');
			messageType = 'error';
		} finally {
			loading = false;
		}
	}

	async function saveCategory(categoryKey: string) {
		saving = true;
		error = '';
		messageType = '';
		try {
			const cat = categories.find(c => c.key === categoryKey);
			if (!cat) return;

			// Validate all settings in category
			for (const s of cat.settings) {
				const validationError = validateSettingValue(s);
				if (validationError) {
					error = validationError;
					messageType = 'error';
					saving = false;
					return;
				}
			}

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
				// Update original state after successful save
				originalCategories = JSON.parse(JSON.stringify(categories));
				originalToggleStates = JSON.parse(JSON.stringify(toggleStates));
				savedCategory = categoryKey;
				messageType = 'success';
				error = t('settings.save_success', `${cat.label} settings saved successfully.`);
				setTimeout(() => {
					savedCategory = '';
					error = '';
					messageType = '';
				}, 3000);
			} else {
				error = result.error;
				messageType = 'error';
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('settings.save_failed', 'Failed to save settings');
			messageType = 'error';
		} finally {
			saving = false;
		}
	}

	function toggleSetting(catKey: string, settingKey: string, value: boolean) {
		const id = `${catKey}.${settingKey}`;
		toggleStates[id] = value;
	}

	function updateSettingValue(catKey: string, settingKey: string, value: unknown) {
		const cat = categories.find(c => c.key === catKey);
		if (!cat) return;
		const setting = cat.settings.find(s => s.key === settingKey);
		if (setting) {
			setting.value = value;
		}
	}

	function resetSetting(catKey: string, settingKey: string) {
		const cat = categories.find(c => c.key === catKey);
		const origCat = originalCategories.find(c => c.key === catKey);
		if (!cat || !origCat) return;

		const setting = cat.settings.find(s => s.key === settingKey);
		const origSetting = origCat.settings.find(s => s.key === settingKey);
		if (!setting || !origSetting) return;

		if (setting.type === 'toggle') {
			const id = `${catKey}.${settingKey}`;
			toggleStates[id] = originalToggleStates[id] ?? false;
		} else {
			setting.value = origSetting.value;
		}
	}

	function revertAll() {
		categories = JSON.parse(JSON.stringify(originalCategories));
		toggleStates = JSON.parse(JSON.stringify(originalToggleStates));
		error = t('settings.revert_success', 'All changes reverted.');
		messageType = 'success';
		setTimeout(() => {
			error = '';
			messageType = '';
		}, 3000);
	}

	let uploadingKey = $state('');
	let uploadError = $state('');

	async function handleFileUpload(catKey: string, settingKey: string, file: File) {
		uploadingKey = `${catKey}.${settingKey}`;
		uploadError = '';
		try {
			const convertedFile = await convertImageToModernFormat(file);
			const initiateRes = await api<{
				media_id: string;
				storage_path: string;
				presigned_url: string;
				expires_at: string;
			}>('/api/v1/media/upload', {
				method: 'POST',
				body: { filename: convertedFile.name, mime_type: convertedFile.type, file_size: convertedFile.size }
			});
			if (!initiateRes.ok) {
				uploadError = initiateRes.error || t('settings.upload_initiate_failed', 'Failed to initiate upload');
				return;
			}
			if (!initiateRes.data) {
				uploadError = t('settings.upload_initiate_failed', 'Failed to initiate upload');
				return;
			}
			const { media_id, presigned_url } = initiateRes.data;
			const putRes = await fetch(presigned_url, {
				method: 'PUT',
				headers: { 'Content-Type': convertedFile.type },
				body: convertedFile
			});
			if (!putRes.ok) {
				uploadError = t('settings.upload_file_failed', 'File upload failed: ') + putRes.statusText;
				return;
			}
			const confirmRes = await api(`/api/v1/media/confirm/${media_id}`, { method: 'POST' });
			if (!confirmRes.ok) {
				uploadError = confirmRes.error || t('settings.upload_confirm_failed', 'Failed to confirm upload');
				return;
			}
			updateSettingValue(catKey, settingKey, `/api/v1/media/${media_id}`);
		} catch (e) {
			uploadError = e instanceof Error ? e.message : t('settings.upload_error', 'Upload failed');
		} finally {
			uploadingKey = '';
		}
	}

	// --- Media picker (shared MediaPickerModal for logo/favicon/og_image) ---
	let mediaPickerOpen = $state(false);
	let mediaPickerTarget = $state<{ catKey: string; settingKey: string } | null>(null);

	function openMediaPicker(catKey: string, settingKey: string) {
		mediaPickerTarget = { catKey, settingKey };
		mediaPickerOpen = true;
	}

	function handleMediaSelect(media: { id: string; url: string; filename: string }) {
		if (mediaPickerTarget) {
			updateSettingValue(mediaPickerTarget.catKey, mediaPickerTarget.settingKey, media.url);
		}
		mediaPickerTarget = null;
	}

	$effect(() => {
		loadSettings();
	});

	// Sync activeTab from the URL when the user navigates back/forward or
	// edits the URL directly. Uses untrack for activeTab so that setting
	// activeTab in selectTab doesn't re-trigger this effect (which could
	// revert the tab if page.url hasn't caught up to replaceState yet).
	$effect(() => {
		const urlCat = page.url.searchParams.get('category') ?? '';
		if (urlCat && categories.find((c) => c.key === urlCat)) {
			if (untrack(() => activeTab) !== urlCat) {
				activeTab = urlCat;
			}
		}
	});
</script>

<svelte:head>
	<title>{t('settings.title', 'Settings')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('settings.title', 'Settings')}
	description={t('settings.description', 'Configure platform settings and preferences')}
/>

{#if loading}
	<div class="flex items-center justify-center py-12">
		<RefreshCw class="h-5 w-5 animate-spin" style="color: var(--color-muted)" />
		<span class="ml-2 text-sm" style="color: var(--color-muted)">{t('settings.loading_settings')}</span>
	</div>
{:else}
	<!-- Message banner -->
	{#if error}
		<div
			class="mb-4 flex items-center gap-2 rounded-xl border px-4 py-3 text-sm"
			style={messageType === 'success'
				? "border-color: var(--color-success); background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)"
				: "border-color: var(--color-danger); background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"}
		>
			{#if messageType === 'success'}
				<Check class="h-4 w-4" />
			{/if}
			{error}
		</div>
	{/if}

	<!-- Unsaved changes banner -->
	{#if isDirty}
		<div
			class="mb-4 flex items-center gap-3 rounded-xl border px-4 py-3"
			style="border-color: color-mix(in srgb, var(--color-warning) 30%, transparent); background: color-mix(in srgb, var(--color-warning) 5%, transparent)"
		>
			<AlertTriangle class="h-4 w-4 shrink-0" style="color: var(--color-warning)" />
			<span class="text-sm font-medium" style="color: var(--color-foreground)">
				{t('settings.unsaved_changes', `You have ${dirtyCount()} unsaved change(s).`)}
			</span>
			<div class="flex-1"></div>
			{#if canManage}
				<Button
					variant="outline"
					size="sm"
					onclick={revertAll}
					disabled={saving}
				>
					<Undo2 class="h-4 w-4" />
					{t('settings.revert_all', 'Revert All')}
				</Button>
			{/if}
		</div>
	{/if}

	<!-- Category sidebar + content -->
	{#if categories.length > 0}
		<div class="flex gap-6">
			<!-- Sidebar Navigation -->
			<nav class="hidden w-56 shrink-0 md:block">
				<div class="sticky top-4">
					<div class="space-y-0.5">
						{#each tabs as tab (tab)}
							<button
								class="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors {activeTab === tab.value
									? 'font-medium'
									: 'hover:bg-muted/50'}"
								style={activeTab === tab.value
									? 'background: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-foreground)'
									: 'color: var(--color-muted)'}
								onclick={() => selectTab(tab.value)}
							>
								<span class="flex-1 text-left">{tab.label}</span>
								<span class="text-xs" style="color: var(--color-muted)">{tab.count}</span>
							</button>
						{/each}
					</div>
				</div>
			</nav>

			<!-- Mobile category selector -->
			<div class="mb-4 w-full md:hidden">
				<Select
					options={tabs.map((tab) => ({ value: tab.value, label: `${tab.label} (${tab.count})` }))}
					value={activeTab}
					onchange={(val: string) => selectTab(val)}
				/>
			</div>

			<!-- Main Content -->
			<div class="min-w-0 flex-1">
		<!-- Tab content: one Card per category, shown when its tab is active -->
		{#each categories as category (category.key)}
			{@const catDirtyCount = (() => {
				let count = 0;
				for (const s of category.settings) {
					if (isSettingDirty(category.key, s.key, s.type)) count++;
				}
				return count;
			})()}
			{#if activeTab === category.key}
				<!-- Category header -->
				<div class="mb-4 flex items-center gap-3">
					<span class="text-2xl">{CATEGORY_ICONS[category.key] ?? '⚙️'}</span>
					<div>
						<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{category.label}</h3>
						<p class="text-sm" style="color: var(--color-muted)">{CATEGORY_DESCRIPTIONS[category.key] ?? ''}</p>
					</div>
					{#if catDirtyCount > 0}
						<span
							class="ml-auto rounded-full px-2 py-0.5 text-xs font-medium"
							style="background: color-mix(in srgb, var(--color-warning) 10%, transparent); color: var(--color-warning)"
						>
							{catDirtyCount} {t('settings.modified', 'modified')}
						</span>
					{/if}
				</div>

				<!-- Settings list -->
				<div class="overflow-hidden rounded-xl border" style="border-color: var(--color-border)">
					<div class="divide-y" style="border-color: var(--color-border)">
						{#each category.settings as setting (setting.key)}
							{@const toggleId = `${category.key}.${setting.key}`}
							{@const dirty = isSettingDirty(category.key, setting.key, setting.type)}
							<div class="grid grid-cols-1 gap-4 px-5 py-3.5 sm:grid-cols-2 sm:items-center" style={dirty ? "background: color-mix(in srgb, var(--color-warning) 5%, transparent)" : ''}>
								<div>
									<div class="flex items-center gap-2">
										<label for={toggleId} class="text-sm font-medium" style="color: var(--color-foreground)">
											{setting.label}
										</label>
										{#if dirty}
											<span
												class="rounded-full px-1.5 py-0.5 text-[9px] font-medium"
												style="background: color-mix(in srgb, var(--color-warning) 10%, transparent); color: var(--color-warning)"
											>
												{t('settings.badge_dirty', 'modified')}
											</span>
										{/if}
									</div>
									{#if setting.description}
										<p class="text-xs mt-0.5" style="color: var(--color-muted)">{setting.description}</p>
									{/if}
									{#if dirty}
										<p class="text-[10px] mt-0.5" style="color: var(--color-muted)">
											{t('settings.original_value', 'Original:')} {setting.type === 'toggle' ? (getOriginalValue(category.key, setting.key, setting.type) ? t('settings.enabled', 'Enabled') : t('settings.disabled', 'Disabled')) : getDisplayValue(getOriginalValue(category.key, setting.key, setting.type))}
										</p>
									{/if}
								</div>

								<div class="flex items-center gap-2">
									{#if setting.type === 'text'}
										<input
											type={setting.masked ? 'password' : 'text'}
											id={toggleId}
											value={String(setting.value ?? '')}
											placeholder={setting.masked ? '••••••••' : (setting.placeholder ?? '')}
											class="h-10 flex-1 rounded-lg border px-3 py-2 text-sm"
											style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
											disabled={!canManage}
											oninput={(e) => updateSettingValue(category.key, setting.key, (e.target as HTMLInputElement).value)}
										/>
									{:else if setting.type === 'number'}
										<input
											type="number"
											id={toggleId}
											value={setting.value != null ? String(setting.value) : ''}
											class="h-10 w-28 rounded-lg border px-3 py-2 text-sm"
											style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
											disabled={!canManage}
											oninput={(e) => {
												const raw = (e.target as HTMLInputElement).value;
												updateSettingValue(category.key, setting.key, raw === '' ? '' : Number(raw));
											}}
										/>
									{:else if setting.type === 'toggle'}
										<div class="flex items-center gap-3">
											<span class="text-sm w-8 text-right" style="color: var(--color-muted)">
												{toggleStates[toggleId] ? t('settings.on', 'On') : t('settings.off', 'Off')}
											</span>
											<Switch.Root
												checked={toggleStates[toggleId]}
												onCheckedChange={(v: boolean) => toggleSetting(category.key, setting.key, v)}
												disabled={!canManage}
												class="relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 {toggleStates[toggleId] ? 'bg-primary' : 'bg-surface'}"
												style="--ring-color: var(--color-primary); --ring-offset-color: var(--color-surface)"
												aria-label={setting.label}
											>
												<Switch.Thumb
													class="inline-block h-4 w-4 transform rounded-full bg-surface shadow-sm transition-transform {toggleStates[toggleId] ? 'translate-x-6' : 'translate-x-1'}"
												/>
											</Switch.Root>
										</div>
									{:else if setting.type === 'select'}
										<Select
											options={(setting.options ?? []).map((opt: string) => ({ value: opt, label: opt }))}
											value={String(setting.value ?? '')}
											disabled={!canManage}
											onchange={(val: string) => updateSettingValue(category.key, setting.key, val)}
										/>
									{:else if setting.type === 'brand-slides'}
										<AuthBrandSlidesEditor
											value={setting.value}
											disabled={!canManage}
											onchange={(next) => updateSettingValue(category.key, setting.key, next)}
										/>
									{:else if setting.type === 'card-style'}
										<a
											href="/content-types"
											class="inline-flex items-center gap-1.5 rounded-lg border px-3 py-2 text-sm transition-colors hover:bg-muted/50"
											style="border-color: var(--color-border); color: var(--color-primary)"
										>
											{t('content_types.title', 'Content Types')}
											<ArrowRight class="h-3.5 w-3.5" />
										</a>
									{:else if setting.type === 'textarea'}
										<textarea
											id={toggleId}
											value={String(setting.value ?? '')}
											rows={3}
											placeholder={setting.placeholder}
											class="w-full rounded-lg border px-3 py-2 text-sm font-mono resize-y"
											style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
											disabled={!canManage}
											oninput={(e) => updateSettingValue(category.key, setting.key, (e.target as HTMLTextAreaElement).value)}
										></textarea>
									{:else if setting.type === 'multiselect'}
										{@const selectedValues = Array.isArray(setting.value) ? setting.value : (typeof setting.value === 'string' && setting.value ? setting.value.split(',').map((v: string) => v.trim()) : [])}
										<div class="flex flex-wrap gap-2">
											{#each (setting.options ?? []) as option (option)}
												<button
													type="button"
													class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium transition-colors"
													style={selectedValues.includes(option) ? "border-color: var(--color-primary); background: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)" : "border-color: var(--color-border); color: var(--color-muted)"}
													disabled={!canManage}
													onclick={() => {
														const newValues = selectedValues.includes(option)
															? selectedValues.filter((v: string) => v !== option)
															: [...selectedValues, option];
														updateSettingValue(category.key, setting.key, newValues.join(','));
													}}
												>
													{option}
												</button>
											{/each}
										</div>
									{:else if setting.type === 'boolean'}
										<div class="flex items-center gap-3">
											<input
												type="checkbox"
												id={toggleId}
												checked={Boolean(setting.value)}
												disabled={!canManage}
												class="h-5 w-5 rounded border"
												style="accent-color: var(--color-primary)"
												onchange={(e) => updateSettingValue(category.key, setting.key, (e.target as HTMLInputElement).checked)}
											/>
											<label for={toggleId} class="text-sm" style="color: var(--color-muted)">
												{Boolean(setting.value) ? t('settings.enabled', 'Enabled') : t('settings.disabled', 'Disabled')}
											</label>
										</div>
									{:else if setting.type === 'color'}
										<div class="flex items-center gap-3">
											<input
												type="color"
												id={toggleId}
												value={String(setting.value ?? '#000000')}
												disabled={!canManage}
												class="h-10 w-14 cursor-pointer rounded border p-1"
												style="border-color: var(--color-border); background: var(--color-surface)"
												oninput={(e) => updateSettingValue(category.key, setting.key, (e.target as HTMLInputElement).value)}
											/>
											<input
												type="text"
												value={String(setting.value ?? '#000000')}
												class="h-10 w-24 rounded-lg border px-3 py-2 text-sm font-mono"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												disabled={!canManage}
												oninput={(e) => updateSettingValue(category.key, setting.key, (e.target as HTMLInputElement).value)}
											/>
										</div>
									{:else if setting.type === 'media'}
										<div class="flex items-center gap-2">
											<input
												type="text"
												id={toggleId}
												value={String(setting.value ?? '')}
												placeholder="https://..."
												class="h-10 flex-1 rounded-lg border px-3 py-2 text-sm"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												disabled={!canManage}
												oninput={(e) => updateSettingValue(category.key, setting.key, (e.target as HTMLInputElement).value)}
											/>
											<button
												type="button"
												class="flex items-center gap-1 rounded-lg border px-2 py-1.5 text-xs transition-colors hover:bg-muted/50 disabled:opacity-50"
												style="border-color: var(--color-border); color: var(--color-muted)"
												disabled={!canManage}
												onclick={() => openMediaPicker(category.key, setting.key)}
												title={t('settings.pick_from_media_library', 'Pick from media library')}
											>
												<ImageIcon class="h-3.5 w-3.5" />
												{t('settings.browse', 'Browse')}
											</button>
											{#if setting.value && typeof setting.value === 'string' && (setting.value.startsWith('/api/v1/media/') || setting.value.startsWith('http'))}
												<img src={setting.value} alt={setting.label} class="h-10 w-10 rounded border object-cover" style="border-color: var(--color-border)" />
											{/if}
										</div>
									{:else if setting.type === 'file'}
										<div class="flex items-center gap-2">
											<input
												type="text"
												id={toggleId}
												value={String(setting.value ?? '')}
												placeholder="https://..."
												class="h-10 flex-1 rounded-lg border px-3 py-2 text-sm"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												disabled={!canManage}
												oninput={(e) => updateSettingValue(category.key, setting.key, (e.target as HTMLInputElement).value)}
											/>
											<label class="flex cursor-pointer items-center gap-1 rounded-lg border px-2 py-1.5 text-xs transition-colors" style="border-color: var(--color-border); color: var(--color-muted)">
												<input
													type="file"
													class="hidden"
													accept={setting.accept}
													disabled={!canManage}
													onchange={(e) => {
														const file = (e.target as HTMLInputElement).files?.[0];
														if (file) handleFileUpload(category.key, setting.key, file);
													}}
												/>
												{#if uploadingKey === toggleId}
													<RefreshCw class="h-3 w-3 animate-spin" />
												{:else}
													<Upload class="h-3 w-3" />
												{/if}
											</label>
											{#if setting.value && typeof setting.value === 'string' && (setting.value.startsWith('/api/v1/media/') || setting.value.startsWith('http'))}
												<img src={setting.value} alt={setting.label} class="h-10 w-10 rounded border object-cover" style="border-color: var(--color-border)" />
											{/if}
										</div>
										{#if uploadError && uploadingKey === toggleId}
											<p class="text-xs mt-1" style="color: var(--color-danger)">{uploadError}</p>
										{/if}
									{/if}

									<!-- Reset button -->
									{#if canManage && dirty}
										<button
											type="button"
											class="rounded-lg p-1.5 transition-colors"
											style="color: var(--color-muted)"
											title={t('settings.reset_to_default', 'Reset to original value')}
											onclick={() => resetSetting(category.key, setting.key)}
										>
											<RotateCcw class="h-3.5 w-3.5" />
										</button>
									{/if}
								</div>
							</div>
						{/each}
					</div>
				</div>

				<!-- Save / Revert bar -->
				<div class="mt-4 flex items-center justify-end gap-4">
					{#if savedCategory === category.key}
						<div class="flex items-center gap-2" style="color: var(--color-success)">
							<Check class="h-4 w-4" />
							<span class="text-sm font-medium">{t('settings.saved')}</span>
						</div>
					{/if}

					{#if canManage}
						<Button disabled={saving || !isDirty} variant="outline" size="sm" onclick={() => revertAll()}>
							<Undo2 class="h-4 w-4" />
							{t('settings.revert', 'Revert')}
						</Button>
						<Button disabled={saving} variant="outline" size="sm" onclick={() => saveCategory(category.key)}>
							{#if saving}
								<RefreshCw class="h-4 w-4 animate-spin" />
								{t('common.saving', 'Saving...')}
							{:else}
								<Save class="h-4 w-4" />
								{t('settings.save', 'Save')} {category.label}
							{/if}
						</Button>
					{/if}
				</div>
			{/if}
		{/each}
			</div><!-- /min-w-0 flex-1 -->
		</div><!-- /flex gap-6 -->
	{/if}
{/if}

<Modal bind:open={showUnsavedDialog} title={t('settings.unsaved_changes', 'Unsaved Changes')} size="sm">
	<p class="text-sm" style="color: var(--color-foreground)">
		{t('settings.unsaved_navigate', 'You have unsaved changes. Are you sure you want to leave?')}
	</p>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showUnsavedDialog = false)}>{t('common.stay_here', 'Stay Here')}</Button>
		<Button variant="destructive" onclick={confirmLeaveWithUnsaved}>{t('common.leave', 'Leave')}</Button>
	</div>
</Modal>

<!-- Shared media picker for logo/favicon/og_image settings -->
<MediaPickerModal bind:open={mediaPickerOpen} onselect={handleMediaSelect} accept="image/*,.heic,.heif" />
