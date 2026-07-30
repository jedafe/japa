<script lang="ts">
	import { beforeNavigate } from '$app/navigation';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { Switch, Tooltip, Label, Select } from 'bits-ui';
	import { api, getToken } from '$lib/api';
	import type { SystemSetting } from '$lib/api/types';
	import { t, translations } from '$lib/stores/i18n.svelte';
	import { convertImageToModernFormat } from '$lib/utils/imageConvert';
	import { categoryIcons, categoryNames, sensitiveKeys as categorySensitiveKeys, textareaKeys as categoryTextareaKeys, uploadKeys as categoryUploadKeys, colorKeys as categoryColorKeys } from '$lib/components/settings/categoryConfig';
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
		Upload
	} from 'lucide-svelte';

	let originalSettings = $state<SystemSetting[]>([]);
	let currentSettings = $state<SystemSetting[]>([]);
	// Entity name lookups for onboarding promoted_* fields (which store UUIDs).
	// Fetched once on load so the UI can show human-readable names instead of UUIDs.
	let entityOptions = $state<Record<string, { value: string; label: string }[]>>({
		'onboarding.promoted_spaces': [],
		'onboarding.promoted_users': [],
		'onboarding.promoted_tags': []
	});
	let entitySearch = $state('');
	let entityDropdownRefs = $state<Record<string, HTMLDivElement>>({});

	// Close entity dropdowns when clicking outside
	function handleEntityDropdownClick(e: MouseEvent) {
		const target = e.target as HTMLElement;
		if (!target.closest('[data-entity-dropdown]') && !target.closest('button')) {
			document.querySelectorAll('[data-entity-dropdown]').forEach((d) => d.classList.add('hidden'));
		}
	}

	let isDirty = $derived(
		JSON.stringify(currentSettings) !== JSON.stringify(originalSettings)
	);
	let dirtyCount = $derived(
		currentSettings.filter(
			(s) => s.value !== getOriginalValue(s.key)
		).length
	);

	let isSaving = $state(false);
	let isLoading = $state(true);
	let activeCategory = $state('all');
	let message = $state('');
	let messageType = $state<'success' | 'error' | ''>('');
	let confirmResetSetting = $state<SystemSetting | null>(null);
	let resetModalOpen = $state(false);
	let confirmResetAllOpen = $state(false);
	// C3: university.surface_domain change warning (ADR-063)
	let confirmSurfaceDomainOpen = $state(false);
	let pendingSurfaceDomainValue = $state('');
	// C4: geo_personalization.enabled disable warning (ADR-064)
	let confirmGeoDisableOpen = $state(false);

	const selectOptions: Record<string, { value: string; label: string }[]> = $derived(
		(() => {
			translations();
			const opts: Record<string, { value: string; label: string }[]> = {
				'media.storage.provider': [
					{ value: 'seaweedfs', label: t('settings.storage_seaweedfs', 'SeaweedFS') },
					{ value: 's3', label: t('settings.storage_s3', 'Amazon S3') },
					{ value: 'r2', label: t('settings.storage_r2', 'Cloudflare R2') },
					{ value: 'b2', label: t('settings.storage_b2', 'Backblaze B2') }
				]
			};
			return opts;
		})()
	);

	const multiSelectOptions: Record<string, { value: string; label: string }[]> = $derived(
		(() => {
			translations();
			// No control-panel settings use hardcoded multi-select options.
			// Settings with options (e.g. pricing.currency) get them from the
			// backend catalog via setting.options.
			return {};
		})()
	);

	const textareaKeys = new Set(categoryTextareaKeys);
	const uploadKeys = new Set(categoryUploadKeys);
	const colorKeys = new Set(categoryColorKeys);

	function isSelectKey(key: string): boolean {
		return key in selectOptions;
	}

	function isMultiSelectKey(key: string): boolean {
		return key in multiSelectOptions;
	}

	// getOptionsForSetting returns the API-provided options (setting.options)
	// when present, falling back to the hardcoded selectOptions/multiSelectOptions
	// maps. This satisfies Rule 16: when the backend catalog supplies options,
	// they are the source of truth.
	function getSelectOptionsForSetting(setting: SystemSetting): { value: string; label: string }[] {
		if (setting.options && setting.options.length > 0) return setting.options;
		return selectOptions[setting.key] ?? [];
	}

	function getMultiSelectOptionsForSetting(setting: SystemSetting): { value: string; label: string }[] {
		if (setting.options && setting.options.length > 0) return setting.options;
		return multiSelectOptions[setting.key] ?? [];
	}

	// parseJsonArray safely parses a JSON array string (e.g. '["uuid1", "uuid2"]')
	// returning an empty array on any parse error.
	// Handles double-encoded values where the backend stored a JSON string
	// containing a JSON array: "[\"uuid\"]" → after normalizeValue → [\"uuid\"]
	// which has literal backslash-quotes and is not valid JSON as-is.
	function parseJsonArray(v: string): string[] {
		if (!v) return [];
		try {
			const parsed = JSON.parse(v);
			if (Array.isArray(parsed)) return parsed.map(String);
			// Value might be a JSON string containing a JSON array
			if (typeof parsed === 'string') {
				const inner = JSON.parse(parsed);
				return Array.isArray(inner) ? inner.map(String) : [];
			}
			return [];
		} catch {
			// Handle the case where normalizeValue stripped the outer quotes
			// from a double-encoded value, leaving: [\"uuid\"]
			try {
				const unescaped = v.replace(/\\"/g, '"');
				const parsed = JSON.parse(unescaped);
				return Array.isArray(parsed) ? parsed.map(String) : [];
			} catch {
				return [];
			}
		}
	}

	// getEntityOptionsForSetting returns the entity-name options for the
	// onboarding promoted_* fields (spaces, users, tags).
	function getEntityOptionsForSetting(setting: SystemSetting): { value: string; label: string }[] {
		return entityOptions[setting.key] ?? [];
	}

	// getCurrentUuids reads the current value of a setting and parses it as a
	// JSON array. Unlike {@const selectedUuids} which is captured once per render,
	// this function always reads the latest value from currentSettings, making it
	// safe to use inside event handlers where the stale {at const} would cause bugs.
	function getCurrentUuids(key: string): string[] {
		const setting = currentSettings.find((s) => s.key === key);
		if (!setting) return [];
		return parseJsonArray(setting.value);
	}

	// addUuidToSetting appends a UUID to a setting's JSON array value,
	// then updates currentSettings so reactivity flows.
	function addUuidToSetting(key: string, uuid: string) {
		const current = getCurrentUuids(key);
		if (!current.includes(uuid)) {
			updateSetting(key, JSON.stringify([...current, uuid]));
		}
	}

	// removeUuidFromSetting removes a UUID from a setting's JSON array value.
	function removeUuidFromSetting(key: string, uuid: string) {
		const current = getCurrentUuids(key);
		updateSetting(key, JSON.stringify(current.filter((v) => v !== uuid)));
	}

	function isTextareaKey(key: string): boolean {
		return textareaKeys.has(key);
	}

	function isUploadKey(key: string): boolean {
		return uploadKeys.has(key);
	}

	function isColorKey(key: string): boolean {
		return colorKeys.has(key);
	}

	function getDisplayValue(setting: SystemSetting): string {
		const val = setting.value ?? '';
		if (val === '[]' || val === '{}') return '';
		if (val.startsWith('"') && val.endsWith('"')) {
			const inner = val.slice(1, -1);
			if (inner.startsWith('[') || inner.startsWith('{')) {
				// Check if it's an array and return item count
				try {
					const parsed = JSON.parse(val);
					if (Array.isArray(parsed)) {
						return parsed.length > 0 ? `${parsed.length} items` : '';
					}
				} catch { /* fall through */ }
				return val;
			}
			return inner;
		}
		// Check if the raw value is a JSON array (not wrapped in quotes)
		if (val.startsWith('[')) {
			try {
				const parsed = JSON.parse(val);
				if (Array.isArray(parsed)) {
					return parsed.length > 0 ? `${parsed.length} items` : '';
				}
			} catch { /* fall through */ }
		}
		return val;
	}

	// normalizeValue strips the JSON string quoting that the batch PATCH
	// endpoint used to add (e.g. "\"3\"" → "3"). This ensures originalSettings
	// and currentSettings hold clean values so isDirty / dirtyCount comparisons
	// are accurate and a value toggled back to its original is not permanently
	// marked dirty.
	// However, values that are JSON strings containing a JSON array/object
	// (e.g. '"[\\"uuid\\"]"') must NOT have their outer quotes stripped —
	// that would leave [\"uuid\"] which is not valid JSON. parseJsonArray
	// handles the full unescaping.
	function normalizeValue(v: string): string {
		if (typeof v !== 'string') return v;
		if (v.length >= 2 && v.startsWith('"') && v.endsWith('"')) {
			const inner = v.slice(1, -1);
			// Don't strip quotes if the inner value is a JSON array/object
			// string — those need the outer quotes to remain valid JSON strings
			// that parseJsonArray can handle via JSON.parse.
			if (inner.startsWith('[') || inner.startsWith('{')) {
				return v; // keep the outer quotes
			}
			return inner;
		}
		return v;
	}

	function normalizeSettings(settings: SystemSetting[]): SystemSetting[] {
		return settings.map((s) => ({
			...s,
			value: normalizeValue(s.value ?? ''),
			default_value: normalizeValue(s.default_value ?? '')
		}));
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

	function formatOriginalValue(key: string, setting: SystemSetting): string {
		const raw = getOriginalValue(key);
		if (!raw) return '';
		const str = String(raw);
		if (str.startsWith('[') && str.endsWith(']')) {
			try {
				const parsed = JSON.parse(str);
				if (Array.isArray(parsed)) {
					return parsed.length + ' items';
				}
			} catch { /* fall through */ }
		}
		return str;
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
		try {
			const changed = currentSettings.filter(
				(s) => s.value !== getOriginalValue(s.key)
			);
			for (const setting of changed) {
				const error = validateSettingValue(setting);
				if (error) {
					message = error;
					messageType = 'error';
					return;
				}
			}
			// C3: university.surface_domain change warning (ADR-063).
			// Changing this value affects Learn app host detection — show
			// a confirmation dialog before proceeding with the save.
			const surfaceDomainSetting = changed.find((s) => s.key === 'university.surface_domain');
			if (surfaceDomainSetting && !confirmSurfaceDomainOpen) {
				pendingSurfaceDomainValue = surfaceDomainSetting.value;
				confirmSurfaceDomainOpen = true;
				isSaving = false;
				return;
			}
			// Use the batch endpoint PATCH /control/system-settings which accepts
			// a map of {key: value} and applies them in a single request.
			const body = Object.fromEntries(changed.map((s) => [s.key, s.value]));
			const result = await api('/control/system-settings', {
				method: 'PATCH',
				body
			});
			if (result.ok) {
				originalSettings = JSON.parse(JSON.stringify(currentSettings));
				message = t('settings.save_all_success', '{count} setting(s) saved successfully.').replace('{count}', String(changed.length));
				messageType = 'success';
			} else {
				message = result.error || t('settings.save_all_error', 'Failed to save {count} setting(s).').replace('{count}', String(changed.length));
				messageType = 'error';
			}
		} catch {
			message = t('settings.save_all_error', 'Failed to save settings.');
			messageType = 'error';
		} finally {
			isSaving = false;
		}
	}

	// C3: Called when the user confirms the surface_domain change warning.
	async function confirmSurfaceDomainSave() {
		confirmSurfaceDomainOpen = false;
		await saveAll();
	}

	// C4: Called when the user confirms the geo_personalization disable warning.
	function confirmGeoDisable() {
		confirmGeoDisableOpen = false;
		updateSetting('geo_personalization.enabled', 'false');
	}

	// C4: Intercept geo_personalization.enabled toggle — on disable, show
	// a confirmation dialog before applying the change (ADR-064).
	function handleSettingToggle(key: string, checked: boolean) {
		if (key === 'geo_personalization.enabled' && !checked) {
			confirmGeoDisableOpen = true;
			return;
		}
		updateSetting(key, checked ? 'true' : 'false');
	}

	function resetAll() {
		currentSettings = JSON.parse(JSON.stringify(originalSettings));
		confirmResetAllOpen = false;
		message = t('settings.reset_all_success', 'All changes reverted.');
		messageType = 'success';
	}

	async function resetSetting(key: string) {
		message = '';
		try {
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
		} catch {
			message = t('settings.reset_error', 'Failed to reset setting.');
			messageType = 'error';
		}
	}

	async function load() {
		isLoading = true;
		message = '';
		const result = await api<SystemSetting[]>('/control/system-settings');
		if (result.ok) {
			// Filter to control-panel settings only (Panel === 'control').
			// Using a positive filter ensures uncatalogued settings (panel === '')
			// never leak into the control panel — only explicitly control-panel
			// settings are shown.
			const normalized = normalizeSettings(
				result.data.filter((s) => s.panel === 'control')
			);
			originalSettings = JSON.parse(JSON.stringify(normalized));
			currentSettings = JSON.parse(JSON.stringify(normalized));
			// Fetch entity names for onboarding promoted_* fields (UUID → name).
			await loadEntityOptions();
		}
		isLoading = false;
	}

	// loadEntityOptions fetches spaces, users, and tags from the admin API so
	// the onboarding.promoted_* settings can display entity names instead of
	// raw UUIDs. The values stored in the setting are JSON arrays of UUIDs.
	async function loadEntityOptions() {
		const [spacesRes, usersRes, tagsRes] = await Promise.all([
			api<{ items: { id: string; name: string }[] }>('/api/v1/admin/spaces?per_page=200'),
			api<{ items: { id: string; username: string }[] }>('/api/v1/admin/users?per_page=200'),
			api<{ id: string; name: string }[]>('/api/v1/admin/tags')
		]);
		entityOptions = {
			'onboarding.promoted_spaces': spacesRes.ok && spacesRes.data?.items
				? spacesRes.data.items.map((s) => ({ value: s.id, label: s.name }))
				: [],
			'onboarding.promoted_users': usersRes.ok && usersRes.data?.items
				? usersRes.data.items.map((u) => ({ value: u.id, label: u.username }))
				: [],
			'onboarding.promoted_tags': tagsRes.ok && Array.isArray(tagsRes.data)
				? tagsRes.data.map((t) => ({ value: t.id, label: t.name }))
				: []
		};
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
		// Onboarding promoted_* fields store JSON arrays of UUIDs — render as
		// an entity multi-select that shows names instead of raw UUIDs.
		if (setting.key in entityOptions) return 'entity-multi-select';
		// When the API provides options, render as select (single) or multi-select
		// (csv type) per Rule 16 — the backend catalog is the source of truth.
		const hasApiOptions = !!(setting.options && setting.options.length > 0);
		if (hasApiOptions && setting.type === 'csv') return 'multi-select';
		if (hasApiOptions || isSelectKey(setting.key)) return 'select';
		if (isMultiSelectKey(setting.key)) return 'multi-select';
		if (isTextareaKey(setting.key)) return 'textarea';
		if (isUploadKey(setting.key)) return 'upload';
		if (isColorKey(setting.key)) return 'color';
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

	const sensitiveKeys = new Set(categorySensitiveKeys);

	// L7: Prefer the backend's `sensitive` field (Rule 16 — backend is source
	// of truth). Fall back to the heuristic only if the backend field is
	// missing (e.g. for settings not yet in the catalog).
	function isSensitiveKey(key: string, setting?: SystemSetting): boolean {
		if (setting?.sensitive !== undefined) {
			return setting.sensitive;
		}
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

	const MAX_UPLOAD_SIZE_MB = 50;

	// readCookieValue returns the value of a document cookie by name, or null.
	// Used for the double-submit CSRF token on multipart uploads that bypass
	// the JSON-based api() helper.
	function readCookieValue(name: string): string | null {
		if (typeof document === 'undefined') return null;
		const match = document.cookie
			.split(';')
			.map((c) => c.trim())
			.find((c) => c.startsWith(`${name}=`));
		return match ? decodeURIComponent(match.slice(name.length + 1)) : null;
	}

	async function handleFileUpload(setting: SystemSetting, file: File, inputEl?: HTMLInputElement) {
		uploadingKey = setting.key;
		uploadError = '';

		// Client-side file size check — fail fast instead of waiting for the backend.
		const maxBytes = MAX_UPLOAD_SIZE_MB * 1024 * 1024;
		if (file.size > maxBytes) {
			uploadError = `File is ${(file.size / 1024 / 1024).toFixed(1)}MB — max is ${MAX_UPLOAD_SIZE_MB}MB`;
			uploadingKey = '';
			return;
		}

		try {
			// Use the admin direct multipart upload endpoint which stores files
			// locally in development (no S3/SeaweedFS required) and returns a
			// ready-to-use serve URL. The two-step /media/upload flow requires
			// a configured S3 client for presigned URLs.
			const convertedFile = await convertImageToModernFormat(file);
			const formData = new FormData();
			formData.append('file', convertedFile);
			const token = getToken();
			const csrf = readCookieValue('csrf_token');
			const res = await fetch('/api/v1/admin/media/upload', {
				method: 'POST',
				headers: {
					...(token ? { authorization: `Bearer ${token}` } : {}),
					...(csrf ? { 'X-CSRF-Token': csrf } : {})
				},
				body: formData
			});
			if (!res.ok) {
				const errBody = await res.json().catch(() => ({}));
				uploadError = errBody?.error?.message || `Upload failed: ${res.statusText}`;
				return;
			}
			const body = await res.json();
			const url = body?.data?.url;
			if (!url) {
				uploadError = 'Upload succeeded but no URL returned';
				return;
			}
			updateSetting(setting.key, url);
		} catch (e) {
			uploadError = e instanceof Error ? e.message : 'Upload failed';
		} finally {
			uploadingKey = '';
			// Reset file input so the same file can be re-uploaded.
			if (inputEl) inputEl.value = '';
		}
	}

	let uploadingKey = $state('');
	let uploadError = $state('');

	$effect(() => {
		document.addEventListener('click', handleEntityDropdownClick);
		return () => document.removeEventListener('click', handleEntityDropdownClick);
	});
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
				disabled={isLoading || isSaving}
			>
				<RefreshCw class="size-3 {isLoading || isSaving ? 'animate-spin' : ''}" />
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
					{t('settings.unsaved_changes', 'You have {count} unsaved change(s).').replace('{count}', String(dirtyCount))}
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
		{#if isLoading && currentSettings.length === 0}
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
											{t('settings.badge_dirty')}
										</span>
									{/if}
								</div>
								{#if setting.description}
									<p class="text-muted-foreground mt-0.5 text-xs">{setting.description}</p>
								{/if}
								{#if dirty}
									<p class="mt-0.5 text-[10px] text-amber-600/70">
										{t('settings.original_value', 'Original:')} {isSensitiveKey(setting.key, setting) ? getMaskedValue(getOriginalValue(setting.key)) : formatOriginalValue(setting.key, setting)}
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
											{isTrue ? t('settings.on') : t('settings.off')}
										</span>
										<Switch.Root
											id="setting-{setting.key}"
											checked={isTrue}
											onCheckedChange={(checked) =>
												handleSettingToggle(setting.key, checked)}
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
											<Select.Value placeholder={t('settings.select_placeholder')} />
											<ChevronDown class="text-muted-foreground size-3.5" />
										</Select.Trigger>
										<Select.Portal>
											<Select.Content class={selectContentClasses}>
												<Select.Viewport class="p-1">
													{#each getSelectOptionsForSetting(setting) as option}
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
												{@const optionLabel = getMultiSelectOptionsForSetting(setting).find((o) => o.value === val)?.label ?? val}
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
												<Select.Value placeholder={t('settings.add_placeholder', 'Add...')} />
												<ChevronDown class="text-muted-foreground size-3.5" />
											</Select.Trigger>
											<Select.Portal>
												<Select.Content class={selectContentClasses}>
													<Select.Viewport class="p-1">
														{#each getMultiSelectOptionsForSetting(setting).filter((o) => !selectedValues.includes(o.value)) as option}
															<Select.Item class={selectItemClasses} value={option.value} label={option.label}>
																{option.label}
															</Select.Item>
														{/each}
													</Select.Viewport>
												</Select.Content>
											</Select.Portal>
										</Select.Root>
									</div>

							{:else if componentType === 'entity-multi-select'}
								<div class="w-64">
									<div class="mb-1 flex flex-wrap gap-1">
										{#each getCurrentUuids(setting.key) as uuid}
											<span class="bg-accent text-accent-foreground inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px]">
												{entityOptions[setting.key]?.find((o) => o.value === uuid)?.label ?? uuid.slice(0, 8) + '…'}
												<button class="hover:text-destructive" onclick={() => removeUuidFromSetting(setting.key, uuid)}>×</button>
											</span>
										{/each}
									</div>
									{#if (entityOptions[setting.key] ?? []).filter((o) => !getCurrentUuids(setting.key).includes(o.value)).length > 0}
										<div class="relative" bind:this={entityDropdownRefs[setting.key]}>
											<button
												type="button"
												class="{selectTriggerClasses} w-full"
												onclick={(e) => {
													const container = e.currentTarget.parentElement;
													const dropdown = container?.querySelector('[data-entity-dropdown]') as HTMLElement | null;
													if (dropdown) {
														const isHidden = dropdown.classList.contains('hidden');
														(document.querySelectorAll('[data-entity-dropdown]') as NodeListOf<HTMLElement>).forEach(d => {
															d.classList.add('hidden');
															d.style.bottom = '';
															d.style.top = '';
															d.style.marginBottom = '';
															d.style.marginTop = '';
														});
if (isHidden) {
														dropdown.classList.remove('hidden');
														const triggerBtn = e.currentTarget as HTMLElement;
														requestAnimationFrame(() => {
															const btnRect = triggerBtn.getBoundingClientRect();
																const spaceBelow = window.innerHeight - btnRect.bottom;
																if (spaceBelow < 220) {
																	dropdown.style.bottom = '100%';
																	dropdown.style.top = 'auto';
																	dropdown.style.marginBottom = '4px';
																	dropdown.style.marginTop = '0';
																} else {
																	dropdown.style.bottom = 'auto';
																	dropdown.style.top = '100%';
																	dropdown.style.marginTop = '4px';
																	dropdown.style.marginBottom = '0';
																}
															});
														}
													}
												}}
											>
												<span class="text-muted-foreground">{t('settings.add_placeholder', 'Add...')}</span>
												<ChevronDown class="text-muted-foreground size-3.5 ml-auto" />
											</button>
											<div
												data-entity-dropdown
												class="border-border-input bg-background absolute z-[99999] mt-1 hidden max-h-48 w-full overflow-y-auto rounded-lg border p-1 shadow-lg"
											>
												<input
													type="text"
													placeholder="Search..."
													class="bg-background border-border-input mb-1 w-full rounded border px-2 py-1 text-xs outline-none"
												oninput={(e) => {
													const container = e.currentTarget.closest('[data-entity-dropdown]');
													const buttons = container?.querySelectorAll('button[data-entity-option]');
													const query = (e.target as HTMLInputElement).value.toLowerCase();
													buttons?.forEach((btn) => {
														const el = btn as HTMLElement;
														const label = el.dataset.optionLabel?.toLowerCase() ?? '';
														el.style.display = label.includes(query) ? '' : 'none';
													});
												}}
												onfocus={(e) => e.stopPropagation()}
											/>
												{#each (entityOptions[setting.key] ?? []).filter((o) => !getCurrentUuids(setting.key).includes(o.value)) as option}
													<button
														type="button"
														data-entity-option
														data-option-label={option.label}
														class="hover:bg-muted flex w-full items-center rounded px-2 py-1 text-left text-xs"
														onmousedown={(e) => e.preventDefault()}
														onclick={(e) => {
															addUuidToSetting(setting.key, option.value);
															const dropdown = e.currentTarget.closest('[data-entity-dropdown]') as HTMLElement | null;
															if (dropdown) {
																dropdown.classList.add('hidden');
																dropdown.style.bottom = '';
																dropdown.style.top = '';
																dropdown.style.marginBottom = '';
																dropdown.style.marginTop = '';
															}
															const input = dropdown?.querySelector('input') as HTMLInputElement;
															if (input) input.value = '';
														}}
													>
														{option.label}
													</button>
												{/each}
											</div>
										</div>
									{/if}
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

								{:else if componentType === 'upload'}
									<div class="flex items-center gap-2">
										<input
											id="setting-{setting.key}"
											type="text"
											value={getDisplayValue(setting)}
											onchange={(e) => updateSetting(setting.key, (e.currentTarget as HTMLInputElement).value)}
											placeholder="https://..."
											class="{inputClasses} w-44"
										/>
											<button
											type="button"
											class="border-border-input bg-background-alt hover:bg-muted text-muted-foreground flex items-center gap-1 rounded-lg border px-2 py-1.5 text-xs transition-colors"
											aria-label={t('settings.upload_image', 'Upload image')}
											onclick={(e) => { (e.currentTarget as HTMLButtonElement).querySelector<HTMLInputElement>('input[type=file]')?.click(); }}
											disabled={uploadingKey === setting.key}
										>
											<input
												type="file"
												class="hidden"
												accept="image/*,.svg"
												onchange={(e) => {
													const input = e.currentTarget;
													const file = input.files?.[0];
													if (file) handleFileUpload(setting, file, input);
												}}
											/>
											{#if uploadingKey === setting.key}
												<Loader2 class="size-3 animate-spin" />
											{:else}
												<Upload class="size-3" />
											{/if}
										</button>
										{#if getDisplayValue(setting)}
											<img
												src={getDisplayValue(setting)}
												alt=""
												class="size-6 rounded border border-border-input object-cover"
												onerror={(e) => { (e.currentTarget as HTMLImageElement).style.display = 'none'; }}
											/>
										{/if}
									</div>
									{#if uploadError && uploadingKey === setting.key}
										<p class="text-destructive text-[10px]">{uploadError}</p>
									{/if}

								{:else if componentType === 'color'}
									<div class="flex items-center gap-2">
										<input
											id="setting-{setting.key}"
											type="color"
											value={getDisplayValue(setting) || '#000000'}
											onchange={(e) => updateSetting(setting.key, (e.currentTarget as HTMLInputElement).value)}
											class="size-8 cursor-pointer rounded-lg border border-border-input bg-transparent"
										/>
										<input
											type="text"
											value={getDisplayValue(setting)}
											onchange={(e) => updateSetting(setting.key, (e.currentTarget as HTMLInputElement).value)}
											placeholder="#000000"
											class="{inputClasses} w-24 font-mono text-xs"
										/>
									</div>

								{:else if isSensitiveKey(setting.key, setting)}
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
											aria-label={revealedKeys.has(setting.key) ? t('settings.hide_value', 'Hide value') : t('settings.reveal_value', 'Reveal value')}
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
											<p class="font-medium">{t('settings.reset_to_default')}</p>
											<p class="text-muted-foreground mt-1">
												{t('settings.current_label')} <span class="font-medium">{isSensitiveKey(setting.key, setting) ? getMaskedValue(setting.value) : setting.value}</span>
											</p>
											<p class="text-muted-foreground">
												{t('settings.default_label')} <span class="font-medium">{isSensitiveKey(setting.key, setting) ? getMaskedValue(setting.default_value) : setting.default_value}</span>
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
										{count} {count === 1 ? t('settings.setting_singular', 'setting') : t('settings.settings', 'settings')}
									</p>
								</div>
							</div>
							{#if hasDirtyItems}
								<span class="bg-amber-500/10 text-amber-600 rounded-full px-2 py-0.5 text-[10px] font-medium">
									{catSettings.filter((s) => isSettingDirty(s.key)).length} {t('settings.modified')}
								</span>
							{/if}
							<button
								class="text-muted-foreground hover:text-foreground text-xs font-medium transition-colors"
								onclick={() => (activeCategory = category)}
							>
								{t('settings.view_all', 'View all')} →
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
													{t('settings.badge_dirty')}
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
												<span class="text-muted-foreground w-8 text-right text-xs">{isTrue ? t('settings.on') : t('settings.off')}</span>
												<Switch.Root
													id="setting-{setting.key}"
													checked={isTrue}
													onCheckedChange={(checked) => handleSettingToggle(setting.key, checked)}
													class="bg-muted data-[state=checked]:bg-emerald-500 peer inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full px-[2px] transition-colors disabled:cursor-not-allowed disabled:opacity-50"
												>
													<Switch.Thumb class="bg-background pointer-events-none block size-4 shrink-0 rounded-full shadow-lg transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0" />
												</Switch.Root>
											</div>
										{:else if componentType === 'select'}
											<Select.Root type="single" value={getSelectValue(getDisplayValue(setting))} onValueChange={(v) => { if (v !== undefined) updateSetting(setting.key, v); }}>
												<Select.Trigger class={selectTriggerClasses} id="setting-{setting.key}">
													<Select.Value placeholder={t('settings.select_placeholder')} />
													<ChevronDown class="text-muted-foreground size-3.5" />
												</Select.Trigger>
												<Select.Portal>
													<Select.Content class={selectContentClasses}>
														<Select.Viewport class="p-1">
															{#each getSelectOptionsForSetting(setting) as option}
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
													{@const optionLabel = getMultiSelectOptionsForSetting(setting).find((o) => o.value === val)?.label ?? val}
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
													<Select.Value placeholder={t('settings.add_placeholder', 'Add...')} />
													<ChevronDown class="text-muted-foreground size-3.5" />
												</Select.Trigger>
												<Select.Portal>
													<Select.Content class={selectContentClasses}>
														<Select.Viewport class="p-1">
															{#each getMultiSelectOptionsForSetting(setting).filter((o) => !selectedValues.includes(o.value)) as option}
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

								{:else if componentType === 'entity-multi-select'}
									<div class="w-64">
										<div class="mb-1 flex flex-wrap gap-1">
											{#each getCurrentUuids(setting.key) as uuid}
												<span class="bg-accent text-accent-foreground inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px]">
													{entityOptions[setting.key]?.find((o) => o.value === uuid)?.label ?? uuid.slice(0, 8) + '…'}
													<button class="hover:text-destructive" onclick={() => removeUuidFromSetting(setting.key, uuid)}>×</button>
												</span>
											{/each}
										</div>
										{#if (entityOptions[setting.key] ?? []).filter((o) => !getCurrentUuids(setting.key).includes(o.value)).length > 0}
											<div class="relative" bind:this={entityDropdownRefs[setting.key]}>
											<button
												type="button"
												class="{selectTriggerClasses} w-full"
												onclick={(e) => {
													const container = e.currentTarget.parentElement;
													const dropdown = container?.querySelector('[data-entity-dropdown]') as HTMLElement | null;
													if (dropdown) {
														const isHidden = dropdown.classList.contains('hidden');
														(document.querySelectorAll('[data-entity-dropdown]') as NodeListOf<HTMLElement>).forEach(d => {
															d.classList.add('hidden');
															d.style.bottom = '';
															d.style.top = '';
															d.style.marginBottom = '';
															d.style.marginTop = '';
														});
if (isHidden) {
													dropdown.classList.remove('hidden');
													const triggerBtn = e.currentTarget as HTMLElement;
													requestAnimationFrame(() => {
														const btnRect = triggerBtn.getBoundingClientRect();
															const spaceBelow = window.innerHeight - btnRect.bottom;
															if (spaceBelow < 220) {
																dropdown.style.bottom = '100%';
																dropdown.style.top = 'auto';
																dropdown.style.marginBottom = '4px';
																dropdown.style.marginTop = '0';
															} else {
																dropdown.style.bottom = 'auto';
																dropdown.style.top = '100%';
																dropdown.style.marginTop = '4px';
																dropdown.style.marginBottom = '0';
															}
														});
													}
												}
											}}
										>
											<span class="text-muted-foreground">{t('settings.add_placeholder', 'Add...')}</span>
											<ChevronDown class="text-muted-foreground size-3.5 ml-auto" />
										</button>
										<div
											data-entity-dropdown
											class="border-border-input bg-background absolute z-[99999] mt-1 hidden max-h-48 w-full overflow-y-auto rounded-lg border p-1 shadow-lg"
										>
												<input
													type="text"
													placeholder="Search..."
													class="bg-background border-border-input mb-1 w-full rounded border px-2 py-1 text-xs outline-none"
													oninput={(e) => {
															const container = e.currentTarget.closest('[data-entity-dropdown]');
															const buttons = container?.querySelectorAll('button[data-entity-option]');
															const query = (e.target as HTMLInputElement).value.toLowerCase();
															buttons?.forEach((btn) => {
																const el = btn as HTMLElement;
																const label = el.dataset.optionLabel?.toLowerCase() ?? '';
																el.style.display = label.includes(query) ? '' : 'none';
															});
														}}
														onfocus={(e) => e.stopPropagation()}
													/>
												{#each (entityOptions[setting.key] ?? []).filter((o) => !getCurrentUuids(setting.key).includes(o.value)) as option}
													<button
														type="button"
														data-entity-option
														data-option-label={option.label}
														class="hover:bg-muted flex w-full items-center rounded px-2 py-1 text-left text-xs"
														onmousedown={(e) => e.preventDefault()}
														onclick={(e) => {
															addUuidToSetting(setting.key, option.value);
															const dropdown = e.currentTarget.closest('[data-entity-dropdown]') as HTMLElement | null;
															if (dropdown) {
																dropdown.classList.add('hidden');
																dropdown.style.bottom = '';
																dropdown.style.top = '';
																dropdown.style.marginBottom = '';
																dropdown.style.marginTop = '';
															}
															const input = dropdown?.querySelector('input') as HTMLInputElement;
															if (input) input.value = '';
														}}
													>
														{option.label}
													</button>
												{/each}
											</div>
										</div>
									{/if}
								</div>

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

										{:else if componentType === 'upload'}
										<div class="flex items-center gap-2">
											<input id="setting-{setting.key}" type="text" value={getDisplayValue(setting)} onchange={(e) => updateSetting(setting.key, (e.currentTarget as HTMLInputElement).value)} placeholder="https://..." class="{inputClasses} w-44" />
											<button
												type="button"
												class="border-border-input bg-background-alt hover:bg-muted text-muted-foreground flex items-center gap-1 rounded-lg border px-2 py-1.5 text-xs transition-colors"
												aria-label={t('settings.upload_image', 'Upload image')}
												onclick={(e) => { (e.currentTarget as HTMLButtonElement).querySelector<HTMLInputElement>('input[type=file]')?.click(); }}
												disabled={uploadingKey === setting.key}
											>
												<input type="file" class="hidden" accept="image/*,.svg" onchange={(e) => { const input = e.currentTarget; const file = input.files?.[0]; if (file) handleFileUpload(setting, file, input); }} />
												{#if uploadingKey === setting.key}
													<Loader2 class="size-3 animate-spin" />
												{:else}
													<Upload class="size-3" />
												{/if}
											</button>
											{#if getDisplayValue(setting)}
												<img src={getDisplayValue(setting)} alt="" class="size-6 rounded border border-border-input object-cover" onerror={(e) => { (e.currentTarget as HTMLImageElement).style.display = 'none'; }} />
											{/if}
										</div>
										{#if uploadError && uploadingKey === setting.key}
											<p class="text-destructive text-[10px]">{uploadError}</p>
										{/if}

									{:else if componentType === 'color'}
										<div class="flex items-center gap-2">
											<input id="setting-{setting.key}" type="color" value={getDisplayValue(setting) || '#000000'} onchange={(e) => updateSetting(setting.key, (e.currentTarget as HTMLInputElement).value)} class="size-8 cursor-pointer rounded-lg border border-border-input bg-transparent" />
											<input type="text" value={getDisplayValue(setting)} onchange={(e) => updateSetting(setting.key, (e.currentTarget as HTMLInputElement).value)} placeholder="#000000" class="{inputClasses} w-24 font-mono text-xs" />
										</div>

									{:else if isSensitiveKey(setting.key, setting)}
											<div class="flex items-center gap-1">
												<input id="setting-{setting.key}" type={revealedKeys.has(setting.key) ? 'text' : 'password'} value={getDisplayValue(setting)} onchange={(e) => updateSetting(setting.key, (e.currentTarget as HTMLInputElement).value)} class="{inputClasses} w-44 font-mono text-xs" />
												<button type="button" class="text-muted-foreground hover:text-foreground rounded p-1 transition-colors" onclick={() => toggleReveal(setting.key)} aria-label={revealedKeys.has(setting.key) ? t('settings.hide_value', 'Hide') : t('settings.reveal_value', 'Reveal')}>
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
													<p class="font-medium">{t('settings.reset_to_default')}</p>
													<p class="text-muted-foreground mt-1">{t('settings.current_label')} <span class="font-medium">{isSensitiveKey(setting.key, setting) ? getMaskedValue(setting.value) : setting.value}</span></p>
													<p class="text-muted-foreground">{t('settings.default_label')} <span class="font-medium">{isSensitiveKey(setting.key, setting) ? getMaskedValue(setting.default_value) : setting.default_value}</span></p>
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
									{t('settings.view_all_count', 'View all {count} settings').replace('{count}', String(catSettings.length))} →
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

<!-- C3: university.surface_domain change warning (ADR-063) -->
<ConfirmModal
	bind:open={confirmSurfaceDomainOpen}
	title={t('settings.surface_domain_warning_title', 'Surface Domain Change')}
	consequence={t(
		'settings.surface_domain_warning',
		'Changing university.surface_domain affects how the Learn app detects its host. This may break Learn app routing until DNS and the Learn app config are updated. New value: '
	) + pendingSurfaceDomainValue}
	confirmText={t('common.confirm', 'Confirm')}
	onConfirm={confirmSurfaceDomainSave}
	onCancel={() => { confirmSurfaceDomainOpen = false; }}
/>

<!-- C4: geo_personalization.enabled disable warning (ADR-064) -->
<ConfirmModal
	bind:open={confirmGeoDisableOpen}
	title={t('settings.geo_disable_warning_title', 'Disable Geo-Personalization')}
	consequence={t(
		'settings.geo_disable_warning',
		'This will disable geo-personalized content for all users. Content feeds will no longer be tailored by geographic location.'
	)}
	confirmText={t('common.confirm', 'Confirm')}
	onConfirm={confirmGeoDisable}
	onCancel={() => { confirmGeoDisableOpen = false; }}
/>
