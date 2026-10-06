<script lang="ts">
	import { beforeNavigate, goto } from '$app/navigation';
	import { page } from '$app/state';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import Switch from '$lib/components/ui/Switch.svelte';
	import Tooltip from '$lib/components/ui/Tooltip.svelte';
	import { api, getToken } from '$lib/api';
	import type { SystemSetting } from '$lib/api/types';
	import { t, translations } from '$lib/stores/i18n.svelte';
	import { convertImageToModernFormat } from '$lib/utils/imageConvert';
	import AchievementReactionsEditor from '$lib/components/settings/AchievementReactionsEditor.svelte';
	import ComparisonDimensionHintsEditor from '$lib/components/settings/ComparisonDimensionHintsEditor.svelte';
	import CountryMultiSelect from '$lib/components/settings/CountryMultiSelect.svelte';
	import KeyValueEditor from '$lib/components/settings/KeyValueEditor.svelte';
	import CsvRepeater from '$lib/components/settings/CsvRepeater.svelte';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { categoryIcons, categoryNames, sensitiveKeys as categorySensitiveKeys, textareaKeys as categoryTextareaKeys, uploadKeys as categoryUploadKeys, colorKeys as categoryColorKeys } from '$lib/components/settings/categoryConfig';
	import {
		Settings,
		RefreshCw,
		RotateCcw,
		Check,
		Loader2,
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

	// Studio options fetched from the admin API so studio.default_persona_id
	// renders as a persona select and studio.auto_publish_types renders as a
	// content-type multi-select (Rule 16 — backend is source of truth).
	let studioPersonaOptions = $state<{ value: string; label: string }[]>([]);
	let studioContentTypeOptions = $state<{ value: string; label: string }[]>([]);

	// RAG embedding model options fetched from the AI gateway so
	// rag.embedding_model renders as a live-debounced autocomplete listing
	// only models from providers registered for the "embedding" category
	// (§69, ADR-069 — Rule 16: backend is source of truth).
	let ragEmbeddingModelOptions = $state<{ value: string; label: string }[]>([]);

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
	// C3: university.subdomain / university.custom_domain change warning (ADR-063 §2)
	let confirmSurfaceDomainOpen = $state(false);
	let pendingSurfaceDomainValue = $state('');

	// Feature on/off toggles are controlled by the Control Panel extensions
	// page (single source of truth). The setting_key of every extension in the
	// backend knownExtensions catalog is fetched from /control/extensions and
	// hidden from the settings page so the enable/disable switch is not
	// duplicated. This keeps the backend catalog as the single source of truth —
	// new extensions are automatically hidden without a frontend update.
	interface ExtensionEntry {
		key: string;
		setting_key?: string;
	}
	let extensionSettingKeys = $state<Set<string>>(new Set());

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
		if (key === 'comparison.auto_dimension_hints') {
			try {
				const parsed = JSON.parse(str);
				const defaults = Array.isArray(parsed.default) ? parsed.default.length : 0;
				const groups = Array.isArray(parsed.groups) ? parsed.groups.length : 0;
				return `${defaults} default(s), ${groups} group(s)`;
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
			// C3: university.subdomain / university.custom_domain change warning
			// (ADR-063 §2). Changing either value affects how the Learn app
			// detects its host — show a confirmation dialog before saving.
			// Ref: japaconnect-engineering/conventions/system-settings-catalog.md
			const surfaceDomainSetting = changed.find(
				(s) => s.key === 'university.subdomain' || s.key === 'university.custom_domain'
			);
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

	// Apply a boolean setting toggle directly. Extension on/off toggles
	// are filtered out of the settings list (see load()) and are controlled
	// by the /extensions page.
	function handleSettingToggle(key: string, checked: boolean) {
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
		// Fetch system settings and the extensions catalog in parallel. The
		// extensions catalog supplies every extension's setting_key, which is
		// the enable/disable toggle that must be hidden from the settings page
		// (it is owned by the /extensions page).
		const [result, extResult] = await Promise.all([
			api<SystemSetting[]>('/control/system-settings'),
			api<ExtensionEntry[]>('/control/extensions')
		]);
		if (extResult.ok && extResult.data) {
			extensionSettingKeys = new Set(
				extResult.data
					.map((e) => e.setting_key ?? '')
					.filter((k) => k !== '')
			);
		}
		if (result.ok) {
			// Filter to control-panel settings only (Panel === 'control') and
			// exclude extension enable/disable toggles (owned by /extensions).
			// Using a positive filter ensures uncatalogued settings (panel === '')
			// never leak into the control panel — only explicitly control-panel
			// settings are shown.
			const normalized = normalizeSettings(
				result.data.filter(
					(s) => s.panel === 'control' && !extensionSettingKeys.has(s.key)
				)
			);
			originalSettings = JSON.parse(JSON.stringify(normalized));
			currentSettings = JSON.parse(JSON.stringify(normalized));
			// Fetch entity names for onboarding promoted_* fields (UUID → name).
			await loadEntityOptions();
			// Fetch studio persona and content-type options for the studio
			// settings category (§75, ADR-076).
			await loadStudioOptions();
			// Fetch embedding models from the AI gateway for the RAG embedding
			// model autocomplete (§69, ADR-069).
			await loadEmbeddingModels();
		}
		isLoading = false;
	}

	// loadEntityOptions fetches spaces, users, and tags from the admin API so
	// the onboarding.promoted_* settings can display entity names instead of
	// raw UUIDs. The values stored in the setting are JSON arrays of UUIDs.
	//
	// The backend list endpoints use `page_size` (max 100) — not `per_page`.
	// After loading the first page, any UUIDs already stored in the promoted_*
	// settings that were not in the list are fetched individually via
	// GET /admin/{resource}/{id} so every chip shows a human-readable name
	// instead of a truncated UUID prefix.
	async function loadEntityOptions() {
		const [spacesRes, usersRes, tagsRes] = await Promise.all([
			api<{ items: { id: string; name: string }[] }>('/api/v1/admin/spaces?page_size=100'),
			api<{ items: { id: string; username: string }[] }>('/api/v1/admin/users?page_size=100'),
			api<{ id: string; name: string }[]>('/api/v1/admin/tags?page_size=100')
		]);

		const spaceOpts = spacesRes.ok && spacesRes.data?.items
			? spacesRes.data.items.map((s) => ({ value: s.id, label: s.name }))
			: [];
		const userOpts = usersRes.ok && usersRes.data?.items
			? usersRes.data.items.map((u) => ({ value: u.id, label: u.username }))
			: [];
		const tagOpts = tagsRes.ok && Array.isArray(tagsRes.data)
			? tagsRes.data.map((t) => ({ value: t.id, label: t.name }))
			: [];

		// Resolve any promoted UUIDs that were not in the first 100 list
		// results by fetching them individually. This ensures every chip
		// shows a name rather than a UUID prefix.
		const promotedSpaceUuids = getCurrentUuids('onboarding.promoted_spaces');
		const promotedUserUuids = getCurrentUuids('onboarding.promoted_users');

		const unresolvedSpaceUuids = promotedSpaceUuids.filter(
			(uuid) => !spaceOpts.some((o) => o.value === uuid)
		);
		const unresolvedUserUuids = promotedUserUuids.filter(
			(uuid) => !userOpts.some((o) => o.value === uuid)
		);

		const [extraSpaces, extraUsers] = await Promise.all([
			Promise.all(
				unresolvedSpaceUuids.map((uuid) =>
					api<{ space: { id: string; name: string } }>(`/api/v1/admin/spaces/${uuid}`).then(
						(r) => r.ok && r.data?.space ? { value: r.data.space.id, label: r.data.space.name } : null
					).catch(() => null)
				)
			),
			Promise.all(
				unresolvedUserUuids.map((uuid) =>
					api<{ id: string; username: string }>(`/api/v1/admin/users/${uuid}`).then(
						(r) => r.ok && r.data ? { value: r.data.id, label: r.data.username } : null
					).catch(() => null)
				)
			)
		]);

		entityOptions = {
			'onboarding.promoted_spaces': [
				...spaceOpts,
				...extraSpaces.filter((o): o is { value: string; label: string } => o !== null)
			],
			'onboarding.promoted_users': [
				...userOpts,
				...extraUsers.filter((o): o is { value: string; label: string } => o !== null)
			],
			'onboarding.promoted_tags': tagOpts
		};
	}

	// loadStudioOptions fetches active personas and content types from the
	// admin Studio API so studio.default_persona_id can render as a persona
	// autocomplete and studio.auto_publish_types as a content-type autocomplete
	// multi-select (§75, ADR-076). Only content types with can_auto_publish=true
	// are offered as auto-publish options — other types are always reviewed.
	async function loadStudioOptions() {
		const [personasRes, contentTypesRes] = await Promise.all([
			api<{ id: string; label: string }[]>('/api/v1/admin/studio/personas'),
			api<{ types: { type: string; label: string; can_auto_publish: boolean }[] }>('/api/v1/admin/studio/content-types')
		]);
		studioPersonaOptions = personasRes.ok && Array.isArray(personasRes.data)
			? personasRes.data.map((p) => ({ value: p.id, label: p.label }))
			: [];
		studioContentTypeOptions = contentTypesRes.ok && contentTypesRes.data?.types
			? contentTypesRes.data.types
					.filter((ct) => ct.can_auto_publish)
					.map((ct) => ({ value: ct.type, label: ct.label }))
			: [];
	}

	// Studio Autocomplete fetchers — filter the pre-loaded options client-side
	// by the debounced query string. The Autocomplete component handles the
	// debounce timing; these functions just do the matching.

	function fetchStudioPersonas(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? studioPersonaOptions
			: studioPersonaOptions.filter((o) => o.label.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((o) => ({ id: o.value, label: o.label })));
	}

	function fetchStudioPersonasInitial(): Promise<AutocompleteItem[]> {
		return Promise.resolve(studioPersonaOptions.map((o) => ({ id: o.value, label: o.label })));
	}

	// getStudioPersonaLabel returns the display label for the currently
	// selected persona UUID (for the Autocomplete chip display).
	function getStudioPersonaLabel(uuid: string): string {
		if (!uuid) return '';
		return studioPersonaOptions.find((o) => o.value === uuid)?.label ?? uuid.slice(0, 8) + '…';
	}

	function fetchStudioContentTypes(q: string, excludeIds: string[] = []): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = studioContentTypeOptions
			.filter((o) => !excludeIds.includes(o.value))
			.filter((o) => query === '' || o.label.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((o) => ({ id: o.value, label: o.label })));
	}

	function fetchStudioContentTypesInitial(excludeIds: string[] = []): Promise<AutocompleteItem[]> {
		return Promise.resolve(
			studioContentTypeOptions
				.filter((o) => !excludeIds.includes(o.value))
				.map((o) => ({ id: o.value, label: o.label }))
		);
	}

	// getStudioContentTypeLabel returns the display label for a content type
	// value (for chip display in the multi-select).
	function getStudioContentTypeLabel(value: string): string {
		return studioContentTypeOptions.find((o) => o.value === value)?.label ?? value;
	}

	// loadEmbeddingModels fetches all available AI models from the AI gateway
	// so rag.embedding_model renders as an autocomplete listing models from
	// every integrated provider (§69, ADR-069 — Rule 16: backend is source of
	// truth, no hardcoded model list).
	async function loadEmbeddingModels() {
		const res = await api<{ id: string; label: string }[]>(
			'/api/v1/admin/ai/models'
		);
		ragEmbeddingModelOptions = res.ok && Array.isArray(res.data)
			? res.data
					.filter((m) => m.id !== '')
					.map((m) => ({ value: m.id, label: m.label }))
			: [];
	}

	// RAG embedding model Autocomplete fetchers — filter the pre-loaded
	// options client-side by the debounced query string. The Autocomplete
	// component handles the debounce timing.

	function fetchEmbeddingModels(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? ragEmbeddingModelOptions
			: ragEmbeddingModelOptions.filter(
					(o) => o.value.toLowerCase().includes(query) || o.label.toLowerCase().includes(query)
				);
		return Promise.resolve(filtered.map((o) => ({ id: o.value, label: o.label })));
	}

	function fetchEmbeddingModelsInitial(): Promise<AutocompleteItem[]> {
		return Promise.resolve(ragEmbeddingModelOptions.map((o) => ({ id: o.value, label: o.label })));
	}

	// getEmbeddingModelLabel returns the display label for the currently
	// selected embedding model (for the Autocomplete chip display). Falls
	// back to the raw model ID if the model is not in the loaded list (e.g.
	// the provider was disabled after the setting was last saved).
	function getEmbeddingModelLabel(modelId: string): string {
		if (!modelId) return '';
		return ragEmbeddingModelOptions.find((o) => o.value === modelId)?.label ?? modelId;
	}

	// --- Entity multi-select fetchers (onboarding promoted_* fields) ---
	// Live debounced search via the Autocomplete component. Each fetcher
	// calls the backend admin list endpoint with the query string and
	// excludes already-selected UUIDs so the user cannot add duplicates.

	// fetchEntitySpaces searches spaces via GET /admin/spaces?q=...
	async function fetchEntitySpaces(q: string, excludeIds: string[] = []): Promise<AutocompleteItem[]> {
		const res = await api<{ items: { id: string; name: string }[] }>(
			`/api/v1/admin/spaces?q=${encodeURIComponent(q)}&page_size=50`
		);
		if (!res.ok || !res.data?.items) return [];
		return res.data.items
			.filter((s) => !excludeIds.includes(s.id))
			.map((s) => ({ id: s.id, label: s.name }));
	}

	function fetchEntitySpacesInitial(excludeIds: string[] = []): Promise<AutocompleteItem[]> {
		return fetchEntitySpaces('', excludeIds);
	}

	// fetchEntityUsers searches users via GET /admin/users?search=...
	async function fetchEntityUsers(q: string, excludeIds: string[] = []): Promise<AutocompleteItem[]> {
		const res = await api<{ items: { id: string; username: string }[] }>(
			`/api/v1/admin/users?search=${encodeURIComponent(q)}&page_size=50`
		);
		if (!res.ok || !res.data?.items) return [];
		return res.data.items
			.filter((u) => !excludeIds.includes(u.id))
			.map((u) => ({ id: u.id, label: u.username }));
	}

	function fetchEntityUsersInitial(excludeIds: string[] = []): Promise<AutocompleteItem[]> {
		return fetchEntityUsers('', excludeIds);
	}

	// fetchEntityTags searches tags via GET /admin/tags?q=...
	async function fetchEntityTags(q: string, excludeIds: string[] = []): Promise<AutocompleteItem[]> {
		const res = await api<{ id: string; name: string }[]>(
			`/api/v1/admin/tags?q=${encodeURIComponent(q)}&page_size=50`
		);
		if (!res.ok || !Array.isArray(res.data)) return [];
		return res.data
			.filter((t) => !excludeIds.includes(t.id))
			.map((t) => ({ id: t.id, label: t.name }));
	}

	function fetchEntityTagsInitial(excludeIds: string[] = []): Promise<AutocompleteItem[]> {
		return fetchEntityTags('', excludeIds);
	}

	// getEntityLabel returns the display label for a selected entity UUID
	// (for chip display). Falls back to the pre-loaded entityOptions map
	// (populated in loadEntityOptions), then to a truncated UUID prefix.
	function getEntityLabel(settingKey: string, uuid: string): string {
		if (!uuid) return '';
		return entityOptions[settingKey]?.find((o) => o.value === uuid)?.label ?? uuid.slice(0, 8) + '…';
	}

	// getEntityFetcher returns the appropriate live-search fetcher for the
	// given onboarding promoted_* setting key.
	function getEntityFetcher(settingKey: string): (q: string, excludeIds: string[]) => Promise<AutocompleteItem[]> {
		if (settingKey === 'onboarding.promoted_spaces') return fetchEntitySpaces;
		if (settingKey === 'onboarding.promoted_users') return fetchEntityUsers;
		return fetchEntityTags;
	}

	function getEntityInitialFetcher(settingKey: string): (excludeIds: string[]) => Promise<AutocompleteItem[]> {
		if (settingKey === 'onboarding.promoted_spaces') return fetchEntitySpacesInitial;
		if (settingKey === 'onboarding.promoted_users') return fetchEntityUsersInitial;
		return fetchEntityTagsInitial;
	}

	beforeNavigate(({ cancel, to, from }) => {
		// Same-page navigations (e.g. the ?category= deep-link update in
		// selectCategory) never lose state — the dirty guard only applies
		// when actually leaving the settings page.
		if (to?.url.pathname === from?.url.pathname) return;
		if (isDirty) {
			const confirmed = window.confirm(
				t('settings.unsaved_navigate', 'You have unsaved changes. Are you sure you want to leave?')
			);
			if (!confirmed) cancel();
		}
	});

	// Deep-link support: read ?category= from the URL on load and whenever it
	// changes, so other pages (e.g. /extensions) can link directly to a
	// specific settings category.
	$effect(() => {
		const cat = page.url.searchParams.get('category');
		if (cat && cat !== activeCategory) {
			activeCategory = cat;
		}
	});

	// selectCategory sets the active category and updates the URL so the
	// selection is shareable and survives navigation.
	function selectCategory(cat: string) {
		activeCategory = cat;
		const url = new URL(page.url);
		if (cat === 'all') {
			url.searchParams.delete('category');
		} else {
			url.searchParams.set('category', cat);
		}
		goto(url.pathname + url.search, { replaceState: true, noScroll: true });
	}

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
		// Common acronyms that should be fully uppercased, not just title-cased.
		const acronyms = new Set(['rag', 'api', 'ai', 'cv', 'seo', 'url', 'ip', 'dns', 'ssl', 'tls', 'sms', 'fcm', 'apns', 'pr']);
		return (key ?? '')
			.replace(/_/g, ' ')
			.replace(/([a-z])([A-Z])/g, '$1 $2')
			.replace(/\./g, ' · ')
			.replace(/\b\w+/g, (word) => {
				if (acronyms.has(word.toLowerCase())) return word.toUpperCase();
				return word.charAt(0).toUpperCase() + word.slice(1);
			});
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
		// achievement_reactions stores a JSONB array of {emoji, label} pairs (§89).
		// Render as a dedicated reaction editor instead of raw JSON text.
		if (setting.key === 'achievement_reactions') return 'achievement-reactions';
		// Comma-separated list of allowed iframe hosts is much clearer as a
		// repeater of individual domain inputs than a single CSV text field.
		if (setting.key === 'content.sanitization.allowed_iframe_hosts') return 'csv-repeater';
		// comparison.auto_dimension_hints stores a JSON object of default
		// dimensions and keyword-based hint groups. Render a structured editor
		// instead of raw JSON text (§30, system-settings-catalog).
		if (setting.key === 'comparison.auto_dimension_hints') return 'comparison-dimension-hints-editor';
		// Geo personalization market lists — CSV of ISO 3166-1 alpha-2 codes.
		// Render as a country multi-select with chips sourced from the spaces
		// API (Rule 16 — no hardcoded country list). ADR-064 / Phase 38.03.
		if (setting.key === 'geo_personalization.origin_markets' || setting.key === 'geo_personalization.destination_markets') return 'country-multi-select';
		// Geo personalization JSON map settings — structured key-value editors
		// instead of raw JSON textareas. ADR-064a recommendation engine config.
		// tool_labels / tool_urls: slug → string (label or URL).
		// stage_tool_matrix: archetype:stage → CSV of tool slugs.
		if (setting.key === 'geo_personalization.tool_labels' || setting.key === 'geo_personalization.tool_urls') return 'key-value-editor';
		// AI provider cost overrides — JSON map of "provider" or "provider.model" to USD per 1k tokens.
		if (setting.key === 'ai.provider_cost_overrides') return 'key-value-editor';
		if (setting.key === 'geo_personalization.stage_tool_matrix') return 'stage-tool-matrix-editor';
		// Onboarding promoted_* fields store JSON arrays of UUIDs — render as
		// an entity multi-select that shows names instead of raw UUIDs.
		if (setting.key in entityOptions) return 'entity-multi-select';
		// Studio fields with API-backed option lists (§75, ADR-076):
		// studio.default_persona_id → persona autocomplete (live debounced
		// search), studio.auto_publish_types → content-type autocomplete
		// multi-select (csv). Options fetched in loadStudioOptions.
		if (setting.key === 'studio.default_persona_id') return 'studio-persona-autocomplete';
		if (setting.key === 'studio.auto_publish_types') return 'studio-content-type-autocomplete';
		// RAG embedding model — live-debounced autocomplete listing models
		// from all integrated AI providers (§69, ADR-069 — Rule 16: no
		// hardcoded model list).
		if (setting.key === 'rag.embedding_model') return 'rag-embedding-model-autocomplete';
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
					onclick={() => selectCategory('all')}
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
						onclick={() => selectCategory(category)}
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
			<AutocompleteSelect
				value={activeCategory}
				onValueChange={(v) => selectCategory(v)}
				placeholder={t('settings.all', 'All')}
				class="w-full"
				options={[
					{ value: 'all', label: `${t('settings.all', 'All')} (${currentSettings.length})` },
					...categoryItems.map(({ category, count }) => ({ value: category, label: `${formatNamespace(category)} (${count})` }))
				]}
			/>
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
									<label for="setting-{setting.key}" class="text-foreground text-sm font-medium">
										{formatSettingKey(setting.key)}
									</label>
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
										<Switch
											id="setting-{setting.key}"
											checked={isTrue}
											onCheckedChange={(checked) =>
												handleSettingToggle(setting.key, checked)}
										/>
									</div>

								{:else if componentType === 'studio-persona-autocomplete'}
									{@const personaId = getDisplayValue(setting) || null}
									<div class="w-56">
										<Autocomplete
											id="setting-{setting.key}"
											placeholder={t('settings.studio_search_persona', 'Search persona...')}
											emptyMessage={t('settings.studio_no_personas', 'No personas found')}
											fetcher={fetchStudioPersonas}
											initialFetcher={fetchStudioPersonasInitial}
											selectedId={personaId}
											selectedLabel={personaId ? getStudioPersonaLabel(personaId) : null}
											onSelect={(item) => updateSetting(setting.key, item.id)}
											onClear={() => updateSetting(setting.key, '')}
										/>
									</div>

								{:else if componentType === 'rag-embedding-model-autocomplete'}
									{@const modelId = getDisplayValue(setting) || null}
									<div class="w-56">
										<Autocomplete
											id="setting-{setting.key}"
											placeholder={t('settings.rag_search_model', 'Search embedding model...')}
											emptyMessage={t('settings.rag_no_models', 'No embedding models found — type a model name manually')}
											fetcher={fetchEmbeddingModels}
											initialFetcher={fetchEmbeddingModelsInitial}
											selectedId={modelId}
											selectedLabel={modelId ? getEmbeddingModelLabel(modelId) : null}
											freeText={true}
											onSelect={(item) => updateSetting(setting.key, item.id)}
											onFreeTextBlur={(q) => updateSetting(setting.key, q)}
											onClear={() => updateSetting(setting.key, '')}
										/>
									</div>

								{:else if componentType === 'studio-content-type-autocomplete'}
									{@const selectedTypes = getMultiSelectValues(getDisplayValue(setting))}
									<div class="w-56">
										{#if selectedTypes.length > 0}
											<div class="mb-1 flex flex-wrap gap-1">
												{#each selectedTypes as val}
													<span class="bg-accent text-accent-foreground inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px]">
														{getStudioContentTypeLabel(val)}
														<button class="hover:text-destructive" onclick={() => updateSetting(setting.key, selectedTypes.filter((v) => v !== val).join(','))}>×</button>
													</span>
												{/each}
											</div>
										{/if}
										<Autocomplete
											id="setting-{setting.key}"
											placeholder={t('settings.studio_add_type', 'Add content type...')}
											emptyMessage={t('settings.studio_no_types', 'No content types found')}
											fetcher={(q) => fetchStudioContentTypes(q, selectedTypes)}
											initialFetcher={() => fetchStudioContentTypesInitial(selectedTypes)}
											selectedId={null}
											onSelect={(item) => updateSetting(setting.key, [...selectedTypes, item.id].join(','))}
										/>
									</div>

								{:else if componentType === 'select'}
									<AutocompleteSelect
										id="setting-{setting.key}"
										value={getSelectValue(getDisplayValue(setting))}
										onValueChange={(v) => updateSetting(setting.key, v)}
										options={getSelectOptionsForSetting(setting)}
										placeholder={t('settings.select_placeholder')}
										class={selectTriggerClasses}
									/>

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
										<AutocompleteSelect
											id="setting-{setting.key}"
											value=""
											onValueChange={(v) => {
												if (v && !selectedValues.includes(v)) {
													updateSetting(setting.key, [...selectedValues, v].join(','));
												}
											}}
											options={getMultiSelectOptionsForSetting(setting).filter((o) => !selectedValues.includes(o.value))}
											placeholder={t('settings.add_placeholder', 'Add...')}
											class="{selectTriggerClasses} w-full"
										/>
									</div>

							{:else if componentType === 'csv-repeater'}
								<div class="w-full max-w-md">
									<CsvRepeater
										value={getDisplayValue(setting)}
										disabled={isSaving}
										onchange={(next) => updateSetting(setting.key, next)}
									/>
								</div>

							{:else if componentType === 'comparison-dimension-hints-editor'}
								<div class="w-full max-w-2xl">
									<ComparisonDimensionHintsEditor
										value={setting.value ?? '{}'}
										disabled={isSaving}
										onchange={(next) => updateSetting(setting.key, next)}
									/>
								</div>

							{:else if componentType === 'achievement-reactions'}
								<div class="w-full max-w-md">
									<AchievementReactionsEditor
										value={setting.value ?? '[]'}
										disabled={isSaving}
										onchange={(next) => updateSetting(setting.key, next)}
									/>
								</div>

							{:else if componentType === 'country-multi-select'}
								<CountryMultiSelect
									value={setting.value ?? ''}
									disabled={isSaving}
									onchange={(next) => updateSetting(setting.key, next)}
								/>

							{:else if componentType === 'key-value-editor'}
								<KeyValueEditor
									value={setting.value ?? '{}'}
									valueMode="string"
									keyPlaceholder={setting.key === 'ai.provider_cost_overrides' ? 'provider.model' : 'slug'}
									valuePlaceholder={setting.key === 'ai.provider_cost_overrides' ? '0.0003' : setting.key === 'geo_personalization.tool_urls' ? '/path' : 'Display label'}
									disabled={isSaving}
									onchange={(next) => updateSetting(setting.key, next)}
								/>

							{:else if componentType === 'stage-tool-matrix-editor'}
								<KeyValueEditor
									value={setting.value ?? '{}'}
									valueMode="csv"
									keyPlaceholder="archetype:stage"
									valuePlaceholder="tool-slug-1, tool-slug-2, ..."
									disabled={isSaving}
									onchange={(next) => updateSetting(setting.key, next)}
								/>

							{:else if componentType === 'entity-multi-select'}
									{@const selectedUuids = getCurrentUuids(setting.key)}
								<div class="w-64">
									{#if selectedUuids.length > 0}
										<div class="mb-1 flex flex-wrap gap-1">
											{#each selectedUuids as uuid}
												<span class="bg-accent text-accent-foreground inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px]">
													{getEntityLabel(setting.key, uuid)}
													<button class="hover:text-destructive" onclick={() => removeUuidFromSetting(setting.key, uuid)}>×</button>
												</span>
											{/each}
										</div>
									{/if}
									<Autocomplete
										id="setting-{setting.key}"
										placeholder={t('settings.add_placeholder', 'Add...')}
										emptyMessage={t('settings.no_results', 'No results found')}
										fetcher={(q) => getEntityFetcher(setting.key)(q, selectedUuids)}
										initialFetcher={() => getEntityInitialFetcher(setting.key)(selectedUuids)}
										selectedId={null}
										onSelect={(item) => addUuidToSetting(setting.key, item.id)}
									/>
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
										min={typeof setting.min === 'number' ? setting.min : undefined}
										max={typeof setting.max === 'number' ? setting.max : undefined}
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
												accept="image/*,.heic,.heif,.svg"
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
								<Tooltip sideOffset={4} class="w-[220px] rounded-xl p-3">
									<button
										class="text-muted-foreground hover:text-foreground hover:bg-muted rounded-lg p-1 transition-colors"
										onclick={() => {
											confirmResetSetting = setting;
											resetModalOpen = true;
										}}
									>
										<RotateCcw class="size-3.5" />
									</button>
									{#snippet content()}
										<p class="font-medium">{t('settings.reset_to_default')}</p>
										<p class="text-muted-foreground mt-1">
											{t('settings.current_label')} <span class="font-medium">{isSensitiveKey(setting.key, setting) ? getMaskedValue(setting.value) : setting.value}</span>
										</p>
										<p class="text-muted-foreground">
											{t('settings.default_label')} <span class="font-medium">{isSensitiveKey(setting.key, setting) ? getMaskedValue(setting.default_value) : setting.default_value}</span>
										</p>
									{/snippet}
								</Tooltip>
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
								onclick={() => selectCategory(category)}
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
											<label for="setting-{setting.key}" class="text-foreground text-sm font-medium">
												{formatSettingKey(setting.key)}
											</label>
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
												<Switch
													id="setting-{setting.key}"
													checked={isTrue}
													onCheckedChange={(checked) => handleSettingToggle(setting.key, checked)}
												/>
											</div>
										{:else if componentType === 'studio-persona-autocomplete'}
											{@const personaId = getDisplayValue(setting) || null}
											<div class="w-56">
												<Autocomplete
													id="setting-{setting.key}"
													placeholder={t('settings.studio_search_persona', 'Search persona...')}
													emptyMessage={t('settings.studio_no_personas', 'No personas found')}
													fetcher={fetchStudioPersonas}
													initialFetcher={fetchStudioPersonasInitial}
													selectedId={personaId}
													selectedLabel={personaId ? getStudioPersonaLabel(personaId) : null}
													onSelect={(item) => updateSetting(setting.key, item.id)}
													onClear={() => updateSetting(setting.key, '')}
												/>
											</div>

										{:else if componentType === 'rag-embedding-model-autocomplete'}
											{@const modelId = getDisplayValue(setting) || null}
											<div class="w-56">
												<Autocomplete
													id="setting-{setting.key}"
													placeholder={t('settings.rag_search_model', 'Search embedding model...')}
													emptyMessage={t('settings.rag_no_models', 'No embedding models found — type a model name manually')}
													fetcher={fetchEmbeddingModels}
													initialFetcher={fetchEmbeddingModelsInitial}
													selectedId={modelId}
													selectedLabel={modelId ? getEmbeddingModelLabel(modelId) : null}
													freeText={true}
													onSelect={(item) => updateSetting(setting.key, item.id)}
													onFreeTextBlur={(q) => updateSetting(setting.key, q)}
													onClear={() => updateSetting(setting.key, '')}
												/>
											</div>

										{:else if componentType === 'studio-content-type-autocomplete'}
											{@const selectedTypes = getMultiSelectValues(getDisplayValue(setting))}
											<div class="w-56">
												{#if selectedTypes.length > 0}
													<div class="mb-1 flex flex-wrap gap-1">
														{#each selectedTypes as val}
															<span class="bg-accent text-accent-foreground inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px]">
																{getStudioContentTypeLabel(val)}
																<button class="hover:text-destructive" onclick={() => updateSetting(setting.key, selectedTypes.filter((v) => v !== val).join(','))}>×</button>
															</span>
														{/each}
													</div>
												{/if}
												<Autocomplete
													id="setting-{setting.key}"
													placeholder={t('settings.studio_add_type', 'Add content type...')}
													emptyMessage={t('settings.studio_no_types', 'No content types found')}
													fetcher={(q) => fetchStudioContentTypes(q, selectedTypes)}
													initialFetcher={() => fetchStudioContentTypesInitial(selectedTypes)}
													selectedId={null}
													onSelect={(item) => updateSetting(setting.key, [...selectedTypes, item.id].join(','))}
												/>
											</div>

										{:else if componentType === 'select'}
											<AutocompleteSelect
												id="setting-{setting.key}"
												value={getSelectValue(getDisplayValue(setting))}
												onValueChange={(v) => updateSetting(setting.key, v)}
												options={getSelectOptionsForSetting(setting)}
												placeholder={t('settings.select_placeholder')}
												class={selectTriggerClasses}
											/>

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
											<AutocompleteSelect
												id="setting-{setting.key}"
												value=""
												onValueChange={(v) => {
													if (v && !selectedValues.includes(v)) {
														updateSetting(setting.key, [...selectedValues, v].join(','));
													}
												}}
												options={getMultiSelectOptionsForSetting(setting).filter((o) => !selectedValues.includes(o.value))}
												placeholder={t('settings.add_placeholder', 'Add...')}
												class="{selectTriggerClasses} w-full"
											/>
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

								{:else if componentType === 'csv-repeater'}
									<div class="w-full max-w-md">
										<CsvRepeater
											value={getDisplayValue(setting)}
											disabled={isSaving}
											onchange={(next) => updateSetting(setting.key, next)}
										/>
									</div>

								{:else if componentType === 'comparison-dimension-hints-editor'}
									<div class="w-full">
										<ComparisonDimensionHintsEditor
											value={setting.value ?? '{}'}
											disabled={isSaving}
											onchange={(next) => updateSetting(setting.key, next)}
										/>
									</div>

								{:else if componentType === 'achievement-reactions'}
									<div class="w-full">
										<AchievementReactionsEditor
											value={setting.value ?? '[]'}
											disabled={isSaving}
											onchange={(next) => updateSetting(setting.key, next)}
										/>
									</div>

								{:else if componentType === 'entity-multi-select'}
									{@const selectedUuids = getCurrentUuids(setting.key)}
									<div class="w-64">
										{#if selectedUuids.length > 0}
											<div class="mb-1 flex flex-wrap gap-1">
												{#each selectedUuids as uuid}
													<span class="bg-accent text-accent-foreground inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px]">
														{getEntityLabel(setting.key, uuid)}
														<button class="hover:text-destructive" onclick={() => removeUuidFromSetting(setting.key, uuid)}>×</button>
													</span>
												{/each}
											</div>
										{/if}
										<Autocomplete
											id="setting-{setting.key}"
											placeholder={t('settings.add_placeholder', 'Add...')}
											emptyMessage={t('settings.no_results', 'No results found')}
											fetcher={(q) => getEntityFetcher(setting.key)(q, selectedUuids)}
											initialFetcher={() => getEntityInitialFetcher(setting.key)(selectedUuids)}
											selectedId={null}
											onSelect={(item) => addUuidToSetting(setting.key, item.id)}
										/>
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
											min={typeof setting.min === 'number' ? setting.min : undefined}
											max={typeof setting.max === 'number' ? setting.max : undefined}
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
												<input type="file" class="hidden" accept="image/*,.heic,.heif,.svg" onchange={(e) => { const input = e.currentTarget; const file = input.files?.[0]; if (file) handleFileUpload(setting, file, input); }} />
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
										<Tooltip sideOffset={4} class="w-[220px] rounded-xl p-3">
											<button class="text-muted-foreground hover:text-foreground hover:bg-muted rounded-lg p-1 transition-colors" onclick={() => { confirmResetSetting = setting; resetModalOpen = true; }}>
												<RotateCcw class="size-3.5" />
											</button>
											{#snippet content()}
												<p class="font-medium">{t('settings.reset_to_default')}</p>
												<p class="text-muted-foreground mt-1">{t('settings.current_label')} <span class="font-medium">{isSensitiveKey(setting.key, setting) ? getMaskedValue(setting.value) : setting.value}</span></p>
												<p class="text-muted-foreground">{t('settings.default_label')} <span class="font-medium">{isSensitiveKey(setting.key, setting) ? getMaskedValue(setting.default_value) : setting.default_value}</span></p>
											{/snippet}
										</Tooltip>
									</div>
								</div>
							{/each}
							{#if catSettings.length > 5}
								<button
									class="text-accent-foreground hover:bg-muted/50 w-full px-5 py-2.5 text-left text-xs font-medium transition-colors"
									onclick={() => selectCategory(category)}
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

<!-- C3: university.subdomain / university.custom_domain change warning (ADR-063 §2) -->
<ConfirmModal
	bind:open={confirmSurfaceDomainOpen}
	title={t('settings.surface_domain_warning_title', 'Surface Domain Change')}
	consequence={t(
		'settings.surface_domain_warning',
		'Changing university.subdomain or university.custom_domain affects how the Learn app detects its host. This may break Learn app routing until DNS and the Learn app config are updated. New value: '
	) + pendingSurfaceDomainValue}
	confirmText={t('common.confirm', 'Confirm')}
	onConfirm={confirmSurfaceDomainSave}
	onCancel={() => { confirmSurfaceDomainOpen = false; }}
/>
