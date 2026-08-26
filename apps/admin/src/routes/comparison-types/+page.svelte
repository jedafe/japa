<script lang="ts">
	// /comparison-types — migrated to the EntityPage scaffold.
	//
	// The comparison-type form needs a dependent field-group selector (loaded
	// dynamically), a locked field-group on edit, an immutable `is_system`
	// checkbox on edit, and auto-slugify-from-name on create — none of which
	// EntityForm supports — so the form is rendered via the `formSnippet` escape
	// hatch. The dimensions + access-control sub-flow (legacy expand row) is kept
	// as a page-local `<SlideOver>` triggered by a custom row action, mirroring
	// the `/tags` moderators sub-flow pattern.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { Button } from '$lib/components/ui/button';
	import { TableSkeleton } from '$lib/components/ui/skeleton';
	import Autocomplete, { type AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import {
		comparisonAdminApi,
		jceAdminApi,
		userManagementApi,
		type ComparisonType,
		type ComparisonTypeDetail,
		type ComparisonTypeAccessRow,
		type CustomFieldGroupOption,
		type ScoreFactorAdmin,
		type UserGroup
	} from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission, hasAnyPermission } from '$lib/stores/permissions.svelte';
	import { parseJson, jsonToString } from '$lib/utils/json';
	import { Tabs } from '$lib/components/ui/tabs';
	import { GitCompare, Shield, Trash2, Eye, Pencil, Sliders } from 'lucide-svelte';

	const canCreateType = $derived(hasPermission('comparison.type.create'));
	const canEditType = $derived(hasPermission('comparison.type.edit'));
	const canDeleteType = $derived(hasPermission('comparison.type.delete'));
	const canManageAccess = $derived(hasPermission('comparison.type.access.manage'));
	const canViewFactors = $derived(hasAnyPermission(['jce.score.view', 'jce.factor.manage']));
	const canManageFactors = $derived(hasPermission('jce.factor.manage'));

	// ── Dependent form options (field groups + user groups + comparison types) ──────────────────
	let fieldGroups = $state<CustomFieldGroupOption[]>([]);
	let userGroups = $state<UserGroup[]>([]);
	let allComparisonTypes = $state<ComparisonType[]>([]);

	async function loadOptions() {
		const [groupsResult, userGroupsResult, typesResult] = await Promise.all([
			comparisonAdminApi.listFieldGroups(),
			userManagementApi.listGroups(),
			comparisonAdminApi.listTypes()
		]);
		if (groupsResult.ok) fieldGroups = groupsResult.data ?? [];
		if (userGroupsResult.ok) userGroups = userGroupsResult.data?.groups ?? [];
		if (typesResult.ok) allComparisonTypes = typesResult.data ?? [];
	}

	$effect(() => {
		void loadOptions();
	});

	function slugify(text: string): string {
		return text
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

	function parseConfig(value: unknown): Record<string, unknown> | undefined {
		if (value == null || value === '') return undefined;
		const parsed =
			typeof value === 'object'
				? (value as Record<string, unknown>)
				: parseJson(value, t('comparison_types.config', 'Config (JSON)'), undefined as Record<string, unknown> | undefined);
		if (parsed && typeof parsed === 'object' && !Array.isArray(parsed) && Object.keys(parsed).length === 0) {
			return undefined;
		}
		return parsed;
	}

	function configObj(value: Record<string, any>): Record<string, any> {
		if (!value.config || typeof value.config !== 'object' || Array.isArray(value.config)) {
			value.config = {};
		}
		return value.config as Record<string, any>;
	}

	function applySort<T extends Record<string, any>>(data: T[], sort: string): T[] {
		if (!sort) return data;
		const [key, dir] = sort.split(':') as [string, 'asc' | 'desc'];
		if (!key) return data;
		const sign = dir === 'desc' ? -1 : 1;
		return [...data].sort((a, b) => {
			const av = a[key];
			const bv = b[key];
			if (av == null && bv == null) return 0;
			if (av == null) return 1 * sign;
			if (bv == null) return -1 * sign;
			if (typeof av === 'string' && typeof bv === 'string') return av.localeCompare(bv) * sign;
			if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * sign;
			if (typeof av === 'boolean' && typeof bv === 'boolean') return (Number(av) - Number(bv)) * sign;
			return String(av).localeCompare(String(bv)) * sign;
		});
	}

	function nestedTypesAllowed(value: Record<string, any>): string[] {
		const cfg = configObj(value);
		const list = cfg.allowed_nested_types;
		return Array.isArray(list) ? list : [];
	}

	function isNestedTypeAllowed(value: Record<string, any>, slug: string): boolean {
		return nestedTypesAllowed(value).includes(slug);
	}

	function toggleNestedType(value: Record<string, any>, slug: string) {
		const cfg = configObj(value);
		const list = nestedTypesAllowed(value);
		if (list.includes(slug)) {
			cfg.allowed_nested_types = list.filter((s: string) => s !== slug);
		} else {
			cfg.allowed_nested_types = [...list, slug];
		}
	}

	// ── Score-engine config helpers (type-level repeaters) ───────────────────
	type AllowedGoal = { key: string; label: string; description: string; default_weight: number };
	type EligibilityRule = { operator: string; field: string; value: any; on_fail: string };
	type RecommendationTemplate = {
		text: string;
		action_type: string;
		action_url?: string;
		priority: number;
		condition?: string;
		threshold?: number;
		statuses?: string[] | string;
		conditions?: Record<string, any> | string;
	};
	type DefaultWeight = { factor_key: string; weight: number };

	const ENGINE_OPERATORS = ['eq', 'ne', 'gt', 'gte', 'lt', 'lte', 'in', 'nin', 'contains'];
	const RECOMMENDATION_CONDITIONS = ['score_lt', 'score_gt', 'score_lte', 'score_gte', 'status_in'];
	let activeFormTab = $state('general');

	function ensureEngineConfig(value: Record<string, any>) {
		const cfg = configObj(value);
		// Convert backend default_weights map to a UI list, but do not seed empty
		// engine arrays unless the config already contains them.
		if (cfg.default_weights && typeof cfg.default_weights === 'object' && !Array.isArray(cfg.default_weights)) {
			cfg.default_weights = Object.entries(cfg.default_weights).map(([factor_key, w]) => ({
				factor_key,
				weight: Number(w) || 0
			}));
		}
		return cfg;
	}

	function addAllowedGoal(value: Record<string, any>) {
		const cfg = ensureEngineConfig(value);
		cfg.allowed_goals = [...(cfg.allowed_goals ?? []), { key: '', label: '', description: '', default_weight: 0 } as AllowedGoal];
	}

	function removeAllowedGoal(value: Record<string, any>, index: number) {
		const cfg = ensureEngineConfig(value);
		cfg.allowed_goals = (cfg.allowed_goals ?? []).filter((_: any, i: number) => i !== index);
	}

	function addEligibilityRule(value: Record<string, any>) {
		const cfg = ensureEngineConfig(value);
		cfg.eligibility_rules = [...(cfg.eligibility_rules ?? []), { operator: 'eq', field: '', value: '', on_fail: 'ineligible' } as EligibilityRule];
	}

	function removeEligibilityRule(value: Record<string, any>, index: number) {
		const cfg = ensureEngineConfig(value);
		cfg.eligibility_rules = (cfg.eligibility_rules ?? []).filter((_: any, i: number) => i !== index);
	}

	function addRecommendationTemplate(value: Record<string, any>) {
		const cfg = ensureEngineConfig(value);
		cfg.recommendation_templates = [
			...(cfg.recommendation_templates ?? []),
			{ text: '', action_type: '', priority: 0, condition: 'score_lt', threshold: 0, statuses: [] } as RecommendationTemplate
		];
	}

	function removeRecommendationTemplate(value: Record<string, any>, index: number) {
		const cfg = ensureEngineConfig(value);
		cfg.recommendation_templates = (cfg.recommendation_templates ?? []).filter((_: any, i: number) => i !== index);
	}

	function addDefaultWeight(value: Record<string, any>) {
		const cfg = ensureEngineConfig(value);
		cfg.default_weights = [...(cfg.default_weights ?? []), { factor_key: '', weight: 0 } as DefaultWeight];
	}

	function removeDefaultWeight(value: Record<string, any>, index: number) {
		const cfg = ensureEngineConfig(value);
		cfg.default_weights = (cfg.default_weights ?? []).filter((_: any, i: number) => i !== index);
	}

	function buildTypeConfig(value: Record<string, any>): Record<string, unknown> | undefined {
		const raw = value.config;
		const cfg: Record<string, any> =
			typeof raw === 'string'
				? parseJson(raw, t('comparison_types.config', 'Config (JSON)'), {} as Record<string, any>)
				: (raw ?? {});
		if (!cfg || typeof cfg !== 'object' || Array.isArray(cfg)) return undefined;

		const result: Record<string, any> = { ...cfg };

		// Preserve a top-level ideal as a number, parsing strings when necessary.
		if ('ideal' in result) {
			if (typeof result.ideal === 'string') {
				const parsed = Number(result.ideal);
				result.ideal = Number.isNaN(parsed) ? 0 : parsed;
			} else if (typeof result.ideal !== 'number') {
				delete result.ideal;
			}
		}

		// Only persist engine arrays when they are present and non-empty.
		if (result.allowed_goals !== undefined) {
			if (!Array.isArray(result.allowed_goals) || result.allowed_goals.length === 0) {
				delete result.allowed_goals;
			}
		}

		if (result.eligibility_rules !== undefined) {
			if (!Array.isArray(result.eligibility_rules) || result.eligibility_rules.length === 0) {
				delete result.eligibility_rules;
			}
		}

		if (result.recommendation_templates !== undefined) {
			if (!Array.isArray(result.recommendation_templates) || result.recommendation_templates.length === 0) {
				delete result.recommendation_templates;
			} else {
				result.recommendation_templates = result.recommendation_templates.map((t: any) => {
					const template: Record<string, any> = { ...t };
					// Flatten legacy nested conditions objects to top-level fields
					// so the scoring engine can read them directly.
					if (template.conditions != null && typeof template.conditions === 'object') {
						template.condition = template.condition ?? template.conditions.condition ?? '';
						template.threshold = template.threshold ?? template.conditions.threshold ?? 0;
						template.statuses = template.statuses ?? template.conditions.statuses ?? [];
						delete template.conditions;
					} else if (typeof template.conditions === 'string') {
						try {
							const parsed = JSON.parse(template.conditions);
							if (parsed && typeof parsed === 'object') {
								template.condition = template.condition ?? parsed.condition ?? '';
								template.threshold = template.threshold ?? parsed.threshold ?? 0;
								template.statuses = template.statuses ?? parsed.statuses ?? [];
							}
						} catch {
							// leave malformed string as-is
						}
						delete template.conditions;
					}
					if (typeof template.threshold === 'string') {
						template.threshold = Number(template.threshold) || 0;
					}
					if (typeof template.statuses === 'string') {
						template.statuses = template.statuses
							.split(',')
							.map((s: string) => s.trim())
							.filter((s: string) => s.length > 0);
					} else if (!Array.isArray(template.statuses)) {
						template.statuses = [];
					}
					return template;
				});
			}
		}

		if (result.default_weights !== undefined) {
			const weightsList = Array.isArray(result.default_weights) ? result.default_weights : [];
			const weights: Record<string, number> = {};
			for (const w of weightsList) {
				if (w.factor_key) weights[w.factor_key] = Number(w.weight) || 0;
			}
			if (Object.keys(weights).length > 0) {
				result.default_weights = weights;
			} else {
				delete result.default_weights;
			}
		}

		// Preserve smart_modes as a map of mode key -> { factor_multipliers, filter_ineligible }.
		if (result.smart_modes !== undefined) {
			if (typeof result.smart_modes === 'string') {
				try {
					result.smart_modes = JSON.parse(result.smart_modes);
				} catch {
					delete result.smart_modes;
				}
			}
			if (!result.smart_modes || typeof result.smart_modes !== 'object' || Array.isArray(result.smart_modes) || Object.keys(result.smart_modes).length === 0) {
				delete result.smart_modes;
			}
		}

		if (Object.keys(result).length === 0) return undefined;
		return result as Record<string, unknown>;
	}

	// ── Bulk delete ───────── EntityPage auto-reloads + clears selection after a bulk action handler completes.
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => comparisonAdminApi.deleteType(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete comparison types');
		} finally {
			bulkDeleting = false;
		}
	}

	// ── Dimensions + access sub-flow (page-local SlideOver) ──────────────────
	let showDetailModal = $state(false);
	let detailType = $state<ComparisonType | null>(null);
	let detail = $state<ComparisonTypeDetail | null>(null);
	let detailLoading = $state(false);
	let detailError = $state('');
	let accessRows = $state<ComparisonTypeAccessRow[]>([]);
	let accessForm = $state({ user_group_id: '', can_create: false, can_fill: true, ai_can_fill: false });
	let accessSaving = $state(false);

	async function openDetail(row: ComparisonType) {
		detailType = row;
		showDetailModal = true;
		detailLoading = true;
		detail = null;
		accessRows = [];
		detailError = '';
		accessForm = { user_group_id: '', can_create: false, can_fill: true, ai_can_fill: false };
		const [detailResult, accessResult] = await Promise.all([
			comparisonAdminApi.getType(row.slug),
			comparisonAdminApi.getTypeAccess(row.id)
		]);
		if (detailResult.ok) detail = detailResult.data;
		if (accessResult.ok) accessRows = accessResult.data ?? [];
		detailLoading = false;
	}

	async function saveAccess() {
		if (!detailType) return;
		accessSaving = true;
		detailError = '';
		const result = await comparisonAdminApi.setTypeAccess(detailType.id, {
			user_group_id: accessForm.user_group_id.trim(),
			can_create: accessForm.can_create,
			can_fill: accessForm.can_fill,
			ai_can_fill: accessForm.ai_can_fill
		});
		if (result.ok) {
			const fresh = await comparisonAdminApi.getTypeAccess(detailType.id);
			if (fresh.ok) accessRows = fresh.data ?? [];
			accessForm = { user_group_id: '', can_create: false, can_fill: true, ai_can_fill: false };
			entityController?.reload();
		} else {
			detailError = result.error;
		}
		accessSaving = false;
	}

	// ── Field group autocomplete helpers (filter pre-loaded options client-side) ──
	// ── Score factors sub-flow (page-local SlideOver) ───────────────────────────────────────────
	let scoreFactorOpen = $state(false);
	let selectedType = $state<ComparisonType | null>(null);
	let scoreFactors = $state<ScoreFactorAdmin[]>([]);
	let scoreFactorLoading = $state(false);
	let scoreFactorError = $state('');
	let selectedFactor = $state<ScoreFactorAdmin | null>(null);
	let factorForm = $state<Record<string, any>>({});
	let factorSaving = $state(false);

	const DATA_SOURCE_OPTIONS = [
		{ value: 'dimension', label: t('score_factors.data_source_dimension', 'Dimension') },
		{ value: 'resolver_metadata', label: t('score_factors.data_source_resolver_metadata', 'Resolver metadata') },
		{ value: 'profile', label: t('score_factors.data_source_profile', 'Profile') },
		{ value: 'eligibility', label: t('score_factors.data_source_eligibility', 'Eligibility') },
		{ value: 'aggregate', label: t('score_factors.data_source_aggregate', 'Aggregate') }
	];

	const SF_KIND_OPTIONS = [
		{ value: 'numeric', label: t('score_factors.kind_numeric', 'Numeric') },
		{ value: 'boolean', label: t('score_factors.kind_boolean', 'Boolean') },
		{ value: 'text', label: t('score_factors.kind_text', 'Text') },
		{ value: 'set', label: t('score_factors.kind_set', 'Set') }
	];

	const SF_DIRECTION_OPTIONS = [
		{ value: 'higher_is_better', label: t('score_factors.direction_higher', 'Higher is better') },
		{ value: 'lower_is_better', label: t('score_factors.direction_lower', 'Lower is better') },
		{ value: 'closer_to_ideal', label: t('score_factors.direction_closer_to_ideal', 'Closer to ideal') }
	];

	const SF_MATCH_STRATEGY_OPTIONS = [
		{ value: 'exact', label: t('score_factors.match_exact', 'Exact') },
		{ value: 'contains', label: t('score_factors.match_contains', 'Contains') },
		{ value: 'budget_lte', label: t('score_factors.match_budget_lte', 'Budget less than or equal') },
		{ value: 'budget_gte', label: t('score_factors.match_budget_gte', 'Budget greater than or equal') },
		{ value: 'match_set', label: t('score_factors.match_set', 'Match set') },
		{ value: 'intersection', label: t('score_factors.match_intersection', 'Intersection') },
		{ value: 'proximity', label: t('score_factors.match_proximity', 'Proximity') }
	];

	const SF_CONFIG_FIELDS_BY_SOURCE: Record<string, string[]> = {
		dimension: ['kind', 'direction', 'min', 'max', 'missing_score'],
		resolver_metadata: ['value_key', 'kind', 'direction', 'min', 'max', 'missing_score'],
		profile: ['profile_key', 'kind', 'direction', 'min', 'max', 'match_strategy', 'ideal_texts', 'missing_score'],
		eligibility: ['item_key', 'kind', 'match_strategy', 'ideal_texts', 'missing_score', 'is_output'],
		aggregate: ['kind', 'direction', 'value_key', 'min', 'max', 'missing_score', 'is_output']
	};

	function sfConfigVisible(field: string, source: string): boolean {
		return SF_CONFIG_FIELDS_BY_SOURCE[source]?.includes(field) ?? false;
	}

	function sfIdealVisible(source: string, cfg: Record<string, any>): boolean {
		return (
			(SF_CONFIG_FIELDS_BY_SOURCE[source]?.includes('direction') && cfg?.direction === 'closer_to_ideal') ||
			(SF_CONFIG_FIELDS_BY_SOURCE[source]?.includes('match_strategy') && cfg?.match_strategy === 'proximity')
		);
	}

	function emptyFactorForm(): Record<string, any> {
		return {
			factor_key: '',
			label: '',
			description: '',
			data_source: 'dimension',
			dimension_slug: '',
			default_weight: 0.5,
			config: { ideal: 0 }
		};
	}

	async function openScoreFactors(row: ComparisonType) {
		selectedType = row;
		scoreFactorOpen = true;
		scoreFactorLoading = true;
		scoreFactorError = '';
		scoreFactors = [];
		selectedFactor = null;
		factorForm = emptyFactorForm();
		try {
			const result = await jceAdminApi.listScoreFactors(row.id);
			if (result.ok) {
				scoreFactors = result.data ?? [];
			} else {
				scoreFactorError = result.error;
				scoreFactors = [];
			}
		} catch (e) {
			scoreFactorError = e instanceof Error ? e.message : t('common.error', 'Failed to load score factors');
			scoreFactors = [];
		}
		scoreFactorLoading = false;
	}

	function parseFactorConfig(raw: unknown): Record<string, any> {
		let parsed: Record<string, any>;
		if (raw == null || raw === '') {
			parsed = {};
		} else if (typeof raw === 'string') {
			try {
				parsed = JSON.parse(raw) as Record<string, any>;
				if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) parsed = {};
			} catch {
				parsed = {};
			}
		} else if (typeof raw === 'object' && !Array.isArray(raw)) {
			parsed = { ...(raw as Record<string, any>) };
		} else {
			parsed = {};
		}
		parsed.ideal = parsed.ideal ?? 0;
		return parsed;
	}

	function selectFactor(factor: ScoreFactorAdmin | null) {
		selectedFactor = factor;
		if (factor) {
			factorForm = {
				factor_key: factor.factor_key,
				label: factor.label,
				description: factor.description ?? '',
				data_source: factor.data_source,
				dimension_slug: factor.dimension_slug ?? '',
				default_weight: factor.default_weight,
				config: parseFactorConfig(factor.config)
			};
		} else {
			factorForm = emptyFactorForm();
		}
	}

	function resetToGlobalDefaults() {
		if (!canManageFactors) return;
		const global = scoreFactors.find((f) => f.is_system && f.factor_key === factorForm.factor_key);
		if (global) {
			factorForm = {
				...factorForm,
				label: global.label,
				description: global.description ?? '',
				data_source: global.data_source,
				dimension_slug: global.dimension_slug ?? '',
				default_weight: global.default_weight,
				config: parseFactorConfig(global.config)
			};
		} else {
			factorForm = {
				...factorForm,
				data_source: 'dimension',
				dimension_slug: '',
				default_weight: 0.5,
				config: {}
			};
		}
	}

	async function saveFactor() {
		if (!selectedType || !canManageFactors) return;
		factorSaving = true;
		scoreFactorError = '';

		const factorKey = String(factorForm.factor_key ?? '').trim();
		const label = String(factorForm.label ?? '').trim();

		if (!factorKey || !label) {
			scoreFactorError = t('score_factors.factor_key_label_required', 'Factor key and label are required');
			factorSaving = false;
			return;
		}

		if (selectedFactor?.is_system) {
			scoreFactorError = t('score_factors.system_protected', 'System score factors cannot be modified');
			factorSaving = false;
			return;
		}

		const rawConfig = factorForm.config ?? {};
		const config: Record<string, any> = { ...rawConfig };

		// Normalise numeric/structured fields from the form.
		if (sfConfigVisible('min', String(factorForm.data_source))) {
			config.min = Number(config.min ?? 0);
		} else {
			delete config.min;
		}
		if (sfConfigVisible('max', String(factorForm.data_source))) {
			config.max = Number(config.max ?? 0);
		} else {
			delete config.max;
		}
		if (sfConfigVisible('missing_score', String(factorForm.data_source))) {
			config.missing_score = Number(config.missing_score ?? 0);
		} else {
			delete config.missing_score;
		}
		if (sfConfigVisible('ideal_texts', String(factorForm.data_source))) {
			config.ideal_texts = Array.isArray(config.ideal_texts) ? config.ideal_texts : [];
		} else {
			delete config.ideal_texts;
		}
		if (sfConfigVisible('is_output', String(factorForm.data_source))) {
			config.is_output = Boolean(config.is_output);
		} else {
			delete config.is_output;
		}

		// Preserve ideal as a number when this factor uses a direction or match strategy
		// that depends on an ideal target value.
		if (sfIdealVisible(String(factorForm.data_source), config)) {
			const rawIdeal = config.ideal;
			const parsed = typeof rawIdeal === 'string' ? Number(rawIdeal) : Number(rawIdeal ?? 0);
			config.ideal = Number.isNaN(parsed) ? 0 : parsed;
		} else {
			delete config.ideal;
		}

		const body: Record<string, any> = {
			factor_key: factorKey,
			label,
			description: String(factorForm.description ?? '').trim(),
			data_source: String(factorForm.data_source ?? 'dimension'),
			dimension_slug: factorForm.data_source === 'dimension' ? (String(factorForm.dimension_slug ?? '').trim() || null) : null,
			default_weight: Number(factorForm.default_weight ?? 0),
			config,
			is_system: false
		};

		const wasEditing = !!selectedFactor;
		const editingId = selectedFactor?.id;
		const result = selectedFactor
			? await jceAdminApi.updateScoreFactor(selectedFactor.id, body)
			: await jceAdminApi.createScoreFactor(selectedType.id, { ...body, comparison_type_id: selectedType.id });

		if (result.ok) {
			await openScoreFactors(selectedType);
			if (wasEditing && editingId) {
				const fresh = scoreFactors.find((f) => f.id === editingId);
				selectFactor(fresh ?? null);
			} else if (result.data) {
				selectFactor(result.data);
			} else {
				selectFactor(null);
			}
		} else {
			scoreFactorError = result.error;
		}
		factorSaving = false;
	}

	async function deleteFactor() {
		if (!selectedType || !selectedFactor || selectedFactor.is_system || !canManageFactors) return;
		const confirmed = confirm(
			t('score_factors.delete_confirm', 'Delete score factor "{factor}"? This action cannot be undone.').replace(
				'{factor}',
				selectedFactor.factor_key
			)
		);
		if (!confirmed) return;
		factorSaving = true;
		scoreFactorError = '';
		const result = await jceAdminApi.deleteScoreFactor(selectedFactor.id);
		if (result.ok) {
			await openScoreFactors(selectedType);
			selectFactor(null);
		} else {
			scoreFactorError = result.error;
		}
		factorSaving = false;
	}

	function fetchFieldGroupOptions(q: string): Promise<{ id: string; label: string }[]> {
		const opts = fieldGroups.map((fg) => ({ id: fg.id, label: fg.name }));
		const query = q.toLowerCase();
		const filtered = query === ''
			? opts
			: opts.filter((o) => o.label.toLowerCase().includes(query));
		return Promise.resolve(filtered);
	}

	function fetchFieldGroupOptionsInitial(): Promise<{ id: string; label: string }[]> {
		return Promise.resolve(fieldGroups.map((fg) => ({ id: fg.id, label: fg.name })));
	}

	function getFieldGroupSelectedLabel(id: string): string {
		return fieldGroups.find((fg) => fg.id === id)?.name ?? id;
	}

	// `fields` is used by EntityPage to seed the form value on edit; the actual
	// inputs are rendered by the `formSnippet` below (the rich comparison-type form).
	const fields: FieldDef[] = [
		{ key: 'name', label: t('comparison_types.name', 'Name'), type: 'text', required: true },
		{ key: 'slug', label: t('comparison_types.slug', 'Slug'), type: 'text' },
		{ key: 'description', label: t('comparison_types.description_label', 'Description'), type: 'textarea' },
		{ key: 'icon', label: t('comparison_types.icon', 'Icon'), type: 'text' },
		{ key: 'item_label', label: t('comparison_types.item_label', 'Item label'), type: 'text' },
		{ key: 'field_group_id', label: t('comparison_types.field_group', 'Field group'), type: 'autocomplete', placeholder: t('comparison_types.field_group_select', 'Select a field group…'), emptyMessage: t('common.no_results', 'No results found'), fetcher: fetchFieldGroupOptions, initialFetcher: fetchFieldGroupOptionsInitial, getSelectedLabel: getFieldGroupSelectedLabel },
		{ key: 'config', label: t('comparison_types.config', 'Config (JSON)'), type: 'textarea' },
		{ key: 'is_system', label: t('comparison_types.system', 'System'), type: 'checkbox' }
	];

	const config: EntityPageConfig = {
		title: t('comparison_types.title', 'Comparison Types'),
		description: t('comparison_types.description', 'Manage comparison type categories, dimensions, and access control'),
		createLabel: t('comparison_types.add', 'Add Comparison Type'),
		createPermission: 'comparison.type.create',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('comparison_types.name', 'Name'), sortable: true },
			{
				key: 'slug',
				label: t('comparison_types.slug', 'Slug'),
				render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{
				key: 'description',
				label: t('comparison_types.description_label', 'Description'),
				render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v || '—'}</span>`
			},
			{
				key: 'is_system',
				label: t('comparison_types.visibility', 'Visibility'),
				sortable: true,
				componentSnippet: VisibilityCell
			},
			{
				key: 'item_count',
				label: t('comparison_types.items', 'Items'),
				render: (v: unknown, row: ComparisonType) => `<span class="text-xs" style="color: var(--color-muted)">${row.item_count ?? 0}</span>`
			}
		],
		filters: [
			{
				key: 'is_system',
				label: t('comparison_types.visibility', 'Visibility'),
				options: [
					{ value: 'true', label: t('comparison_types.system', 'System') },
					{ value: 'false', label: t('comparison_types.custom', 'Custom') }
				]
			}
		],
		actions: [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) },
			{
				label: t('common.edit', 'Edit'),
				icon: Pencil,
				onclick: (_row, ctx) => ctx?.openEdit(_row),
				show: () => canEditType
			},
			{
				label: t('comparison_types.dimensions', 'Dimensions & Access'),
				icon: GitCompare,
				onclick: (row) => openDetail(row)
			},
			{
				label: t('comparison_types.score_factors', 'Score factors'),
				icon: Sliders,
				onclick: (row) => openScoreFactors(row),
				show: () => canViewFactors
			},
			{
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (_row, ctx) => ctx?.openDelete(_row),
				show: () => canDeleteType
			}
		],
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ page, pageSize, q, sort, filter }) => {
			const result = await comparisonAdminApi.listTypes('page_size=1000');
			if (result.ok) {
				let data = (result.data as ComparisonType[]) ?? [];
				const lq = (q ?? '').toLowerCase();
				if (lq) {
					data = data.filter((ct) =>
						[ct.name, ct.slug, ct.description, ct.item_label].some((v) =>
							String(v ?? '').toLowerCase().includes(lq)
						)
					);
				}
				if (filter?.is_system) {
					const val = filter.is_system === 'true';
					data = data.filter((ct) => ct.is_system === val);
				}
				data = applySort(data, sort);
				const size = pageSize || 25;
				const offset = (page || 0) * size;
				const total = data.length;
				const totalPages = Math.max(1, Math.ceil(total / size));
				return {
					data: data.slice(offset, offset + size),
					total,
					totalPages
				};
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load comparison types'));
		},
		create: async (body) => {
			const slug = String(body.slug ?? '').trim() || slugify(String(body.name ?? ''));
			const itemLabel = String(body.item_label ?? '').trim() || 'Item';
			const res = await comparisonAdminApi.createType({
				name: String(body.name ?? '').trim(),
				slug,
				description: String(body.description ?? '').trim(),
				icon: String(body.icon ?? '').trim(),
				field_group_id: String(body.field_group_id ?? ''),
				item_label: itemLabel,
				config: buildTypeConfig(body)
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			// `field_group_id` and `is_system` are immutable — update omits them (mirrors the legacy submit()).
			const slug = String(body.slug ?? '').trim() || slugify(String(body.name ?? ''));
			const itemLabel = String(body.item_label ?? '').trim() || 'Item';
			const res = await comparisonAdminApi.updateType(id, {
				name: String(body.name ?? '').trim(),
				slug,
				description: String(body.description ?? '').trim(),
				icon: String(body.icon ?? '').trim(),
				item_label: itemLabel,
				config: buildTypeConfig(body)
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await comparisonAdminApi.deleteType(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: ComparisonTypeView,
		formSnippet: ComparisonTypeForm
	};
</script>

<svelte:head>
	<title>{t('comparison_types.title', 'Comparison Types')} — {t('common.admin_title_suffix', 'JapaConnect Admin')}</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- Dimensions + Access sub-flow (page-local SlideOver) -->
<SlideOver bind:open={showDetailModal} title={t('comparison_types.dimensions', 'Dimensions & Access')} size="lg">
	{#if detailType}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('comparison_types.name', 'Name')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{detailType.name}</p>
			</div>

			{#if detailError}
				<div
					class="rounded-lg p-2 text-sm"
					style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
				>
					{detailError}
				</div>
			{/if}

			{#if detailLoading}
				<TableSkeleton rows={3} columns={2} />
			{:else}
				<!-- Dimensions (read-only) -->
				<div>
					<h3 class="mb-2 flex items-center gap-2 text-sm font-semibold">
						<GitCompare class="h-4 w-4" style="color: var(--color-primary)" />
						{t('comparison_types.dimensions', 'Dimensions')}
					</h3>
					{#if detail && detail.dimensions.length > 0}
						<div class="space-y-1">
							{#each detail.dimensions as dim (dim.id)}
								<div class="rounded-md border px-3 py-2 text-xs" style="border-color: var(--color-border)">
									<span class="font-medium">{dim.name}</span>
									{#if dim.description}
										<span style="color: var(--color-muted)"> — {dim.description}</span>
									{/if}
								</div>
							{/each}
						</div>
					{:else}
						<p class="text-xs" style="color: var(--color-muted)">
							{t('comparison_types.no_dimensions_defined', 'No dimensions defined')}
						</p>
					{/if}
				</div>

				<p class="text-xs" style="color: var(--color-muted)">
					{t('comparison_types.items', 'Items')}: <span class="font-medium">{detail?.item_count ?? detailType.item_count ?? 0}</span>
				</p>

				<!-- Access control -->
				<div>
					<h3 class="mb-2 flex items-center gap-2 text-sm font-semibold">
						<Shield class="h-4 w-4" style="color: var(--color-primary)" />
						{t('comparison_types.access', 'Access Control')}
					</h3>
					<div class="space-y-3">
						{#if accessRows.length > 0}
							<div class="space-y-1">
								{#each accessRows as row (row.user_group_id ?? 'ai')}
									<div class="rounded-md border px-3 py-2 text-xs" style="border-color: var(--color-border)">
										<span class="font-medium">{row.group_name ?? t('comparison_types.ai_only', 'AI agent only')}</span>
										<span style="color: var(--color-muted)">
											— {row.can_create ? t('comparison_types.can_create', 'Can create') : ''}{row.can_fill ? ` · ${t('comparison_types.can_fill', 'Can fill')}` : ''}{row.ai_can_fill ? ` · ${t('comparison_types.ai_can_fill', 'AI can fill')}` : ''}
										</span>
									</div>
								{/each}
							</div>
						{:else}
							<p class="text-xs" style="color: var(--color-muted)">
								{t('comparison_types.access_empty', 'No access entries configured')}
							</p>
						{/if}

						{#if canManageAccess}
							<div class="space-y-2 rounded-md border p-3" style="border-color: var(--color-border)">
								<div>
									<label for="ct-user-group" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">
										{t('comparison_types.user_group', 'User group ID')}
									</label>
									<select
										id="ct-user-group"
										class="w-full rounded-md border px-3 py-2 text-sm"
										style="border-color: var(--color-border); background-color: var(--color-background)"
										bind:value={accessForm.user_group_id}
									>
										<option value="">{t('comparison_types.user_group_placeholder', 'Leave empty for AI-agent-only access')}</option>
										{#each userGroups as ug (ug.id)}
											<option value={ug.id}>{ug.name}</option>
										{/each}
									</select>
								</div>
								<div class="flex flex-wrap items-center gap-4">
									<label class="flex items-center gap-2 text-xs">
										<input type="checkbox" bind:checked={accessForm.can_create} />
										{t('comparison_types.can_create', 'Can create')}
									</label>
									<label class="flex items-center gap-2 text-xs">
										<input type="checkbox" bind:checked={accessForm.can_fill} />
										{t('comparison_types.can_fill', 'Can fill')}
									</label>
									<label class="flex items-center gap-2 text-xs">
										<input type="checkbox" bind:checked={accessForm.ai_can_fill} />
										{t('comparison_types.ai_can_fill', 'AI can fill')}
									</label>
								</div>
								<Button onclick={saveAccess} disabled={accessSaving}>
									{accessSaving ? t('common.saving', 'Saving…') : t('comparison_types.add_access', 'Add Access Entry')}
								</Button>
							</div>
						{/if}
					</div>
				</div>
			{/if}
		</div>
	{/if}

	{#snippet footer()}
		<Button variant="outline" onclick={() => (showDetailModal = false)}>
			{t('common.close', 'Close')}
		</Button>
	{/snippet}
</SlideOver>

<!-- Score factors sub-flow (page-local SlideOver) -->
<SlideOver bind:open={scoreFactorOpen} title={t('comparison_types.score_factors_title', 'Score Factors')} size="lg">
	{#if selectedType}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('comparison_types.name', 'Name')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{selectedType.name}</p>
			</div>

			{#if scoreFactorError}
				<div
					class="rounded-lg p-2 text-sm"
					style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
				>
					{scoreFactorError}
				</div>
			{/if}

			{#if scoreFactorLoading}
				<TableSkeleton rows={3} columns={3} />
			{:else if scoreFactors.length === 0}
				<div class="py-8 text-center text-sm" style="color: var(--color-muted)">
					{t('score_factors.empty', 'No score factors configured')}
				</div>
			{:else}
				<div class="space-y-1">
					{#each scoreFactors as factor (factor.id)}
						<button
							type="button"
							class="w-full rounded-md border px-3 py-2 text-left text-xs transition-colors hover:bg-surface"
							style="border-color: var(--color-border); background: {selectedFactor?.id === factor.id ? 'var(--color-surface)' : 'transparent'}"
							onclick={() => selectFactor(factor)}
							disabled={!canViewFactors}
						>
							<div class="flex items-center justify-between">
								<span class="font-medium" style="color: var(--color-foreground)">{factor.factor_key}</span>
								<span class="text-xs" style="color: var(--color-muted)">{factor.data_source} · {factor.default_weight}</span>
							</div>
							<div class="mt-0.5 text-xs" style="color: var(--color-muted)">
								{factor.label}
								{#if factor.is_system}
									<span class="ml-1 rounded-full bg-success/15 px-1.5 py-0.5 text-success">{t('comparison_types.system', 'System')}</span>
								{/if}
							</div>
						</button>
					{/each}
				</div>
			{/if}

			{#if canViewFactors}
				<div class="space-y-3 rounded-lg border p-3" style="border-color: var(--color-border)">
					<div class="flex items-center justify-between">
						<h3 class="text-sm font-semibold" style="color: var(--color-foreground)">
							{selectedFactor ? t('score_factors.edit', 'Edit score factor') : t('score_factors.add', 'Add score factor')}
						</h3>
						{#if canManageFactors}
							<Button variant="outline" size="sm" onclick={() => selectFactor(null)}>
								{t('common.add', 'Add')}
							</Button>
						{/if}
					</div>

					<div>
						<label for="sf-factor-key" class="block text-xs font-medium" style="color: var(--color-muted)">
							{t('score_factors.factor_key', 'Factor key')}
						</label>
						<input
							id="sf-factor-key"
							type="text"
							class="mt-1 w-full rounded-lg border p-2 text-sm"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
							bind:value={factorForm.factor_key}
							disabled={!!selectedFactor?.is_system || !canManageFactors}
						/>
					</div>
					<div>
						<label for="sf-label" class="block text-xs font-medium" style="color: var(--color-muted)">
							{t('score_factors.label', 'Label')}
						</label>
						<input
							id="sf-label"
							type="text"
							class="mt-1 w-full rounded-lg border p-2 text-sm"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
							bind:value={factorForm.label}
							disabled={!!selectedFactor?.is_system || !canManageFactors}
						/>
					</div>
					<div>
						<label for="sf-description" class="block text-xs font-medium" style="color: var(--color-muted)">
							{t('score_factors.description', 'Description')}
						</label>
						<textarea
							id="sf-description"
							rows="2"
							class="mt-1 w-full rounded-lg border p-2 text-sm"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
							bind:value={factorForm.description}
							disabled={!!selectedFactor?.is_system || !canManageFactors}
						></textarea>
					</div>
					<div>
						<label for="sf-data-source" class="block text-xs font-medium" style="color: var(--color-muted)">
							{t('score_factors.data_source', 'Data source')}
						</label>
						<select
							id="sf-data-source"
							class="mt-1 w-full rounded-lg border p-2 text-sm"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
							bind:value={factorForm.data_source}
							disabled={!!selectedFactor?.is_system || !canManageFactors}
						>
							{#each DATA_SOURCE_OPTIONS as option (option.value)}
								<option value={option.value}>{option.label}</option>
							{/each}
						</select>
					</div>
					{#if factorForm.data_source === 'dimension'}
						<div>
							<label for="sf-dimension-slug" class="block text-xs font-medium" style="color: var(--color-muted)">
								{t('score_factors.dimension_slug', 'Dimension slug')}
							</label>
							<input
								id="sf-dimension-slug"
								type="text"
								class="mt-1 w-full rounded-lg border p-2 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={factorForm.dimension_slug}
								disabled={!!selectedFactor?.is_system || !canManageFactors}
							/>
						</div>
					{/if}
					<div>
						<label for="sf-default-weight" class="block text-xs font-medium" style="color: var(--color-muted)">
							{t('score_factors.default_weight', 'Default weight')}
						</label>
						<input
							id="sf-default-weight"
							type="number"
							min="0"
							max="1"
							step="0.01"
							class="mt-1 w-full rounded-lg border p-2 text-sm"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
							bind:value={factorForm.default_weight}
							disabled={!!selectedFactor?.is_system || !canManageFactors}
						/>
					</div>
					{#if sfConfigVisible('kind', String(factorForm.data_source ?? 'dimension'))}
						<div>
							<label for="sf-kind" class="block text-xs font-medium" style="color: var(--color-muted)">
								{t('score_factors.kind', 'Kind')}
							</label>
							<select
								id="sf-kind"
								class="mt-1 w-full rounded-lg border p-2 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={factorForm.config.kind}
								disabled={!!selectedFactor?.is_system || !canManageFactors}
							>
								<option value="">{t('common.select', 'Select…')}</option>
								{#each SF_KIND_OPTIONS as option (option.value)}
									<option value={option.value}>{option.label}</option>
								{/each}
							</select>
						</div>
					{/if}
					{#if sfConfigVisible('direction', String(factorForm.data_source ?? 'dimension'))}
						<div>
							<label for="sf-direction" class="block text-xs font-medium" style="color: var(--color-muted)">
								{t('score_factors.direction', 'Direction')}
							</label>
							<select
								id="sf-direction"
								class="mt-1 w-full rounded-lg border p-2 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={factorForm.config.direction}
								disabled={!!selectedFactor?.is_system || !canManageFactors}
							>
								<option value="">{t('common.select', 'Select…')}</option>
								{#each SF_DIRECTION_OPTIONS as option (option.value)}
									<option value={option.value}>{option.label}</option>
								{/each}
							</select>
						</div>
					{/if}
					{#if sfConfigVisible('value_key', String(factorForm.data_source ?? 'dimension'))}
						<div>
							<label for="sf-value-key" class="block text-xs font-medium" style="color: var(--color-muted)">
								{t('score_factors.value_key', 'Value key')}
							</label>
							<input
								id="sf-value-key"
								type="text"
								class="mt-1 w-full rounded-lg border p-2 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={factorForm.config.value_key}
								disabled={!!selectedFactor?.is_system || !canManageFactors}
							/>
						</div>
					{/if}
					{#if sfConfigVisible('profile_key', String(factorForm.data_source ?? 'dimension'))}
						<div>
							<label for="sf-profile-key" class="block text-xs font-medium" style="color: var(--color-muted)">
								{t('score_factors.profile_key', 'Profile key')}
							</label>
							<input
								id="sf-profile-key"
								type="text"
								class="mt-1 w-full rounded-lg border p-2 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={factorForm.config.profile_key}
								disabled={!!selectedFactor?.is_system || !canManageFactors}
							/>
						</div>
					{/if}
					{#if sfConfigVisible('item_key', String(factorForm.data_source ?? 'dimension'))}
						<div>
							<label for="sf-item-key" class="block text-xs font-medium" style="color: var(--color-muted)">
								{t('score_factors.item_key', 'Item key')}
							</label>
							<input
								id="sf-item-key"
								type="text"
								class="mt-1 w-full rounded-lg border p-2 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={factorForm.config.item_key}
								disabled={!!selectedFactor?.is_system || !canManageFactors}
							/>
						</div>
					{/if}
					{#if sfConfigVisible('match_strategy', String(factorForm.data_source ?? 'dimension'))}
						<div>
							<label for="sf-match-strategy" class="block text-xs font-medium" style="color: var(--color-muted)">
								{t('score_factors.match_strategy', 'Match strategy')}
							</label>
							<select
								id="sf-match-strategy"
								class="mt-1 w-full rounded-lg border p-2 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={factorForm.config.match_strategy}
								disabled={!!selectedFactor?.is_system || !canManageFactors}
							>
								<option value="">{t('common.select', 'Select…')}</option>
								{#each SF_MATCH_STRATEGY_OPTIONS as option (option.value)}
									<option value={option.value}>{option.label}</option>
								{/each}
							</select>
						</div>
					{/if}
					{#if sfConfigVisible('ideal_texts', String(factorForm.data_source ?? 'dimension'))}
						<div>
							<label for="sf-ideal-texts" class="block text-xs font-medium" style="color: var(--color-muted)">
								{t('score_factors.ideal_texts', 'Ideal texts')}
							</label>
							<input
								id="sf-ideal-texts"
								type="text"
								class="mt-1 w-full rounded-lg border p-2 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								value={Array.isArray(factorForm.config.ideal_texts) ? factorForm.config.ideal_texts.join(', ') : ''}
								onchange={(e) => {
									factorForm.config.ideal_texts = e.currentTarget.value
										.split(',')
										.map((s) => s.trim())
										.filter(Boolean);
								}}
								disabled={!!selectedFactor?.is_system || !canManageFactors}
							/>
							<p class="mt-1 text-xs" style="color: var(--color-muted)">
								{t('score_factors.ideal_texts_hint', 'Comma-separated values')}
							</p>
						</div>
					{/if}
					{#if sfConfigVisible('min', String(factorForm.data_source ?? 'dimension'))}
						<div>
							<label for="sf-min" class="block text-xs font-medium" style="color: var(--color-muted)">
								{t('score_factors.min', 'Min')}
							</label>
							<input
								id="sf-min"
								type="number"
								class="mt-1 w-full rounded-lg border p-2 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={factorForm.config.min}
								disabled={!!selectedFactor?.is_system || !canManageFactors}
							/>
						</div>
					{/if}
					{#if sfConfigVisible('max', String(factorForm.data_source ?? 'dimension'))}
						<div>
							<label for="sf-max" class="block text-xs font-medium" style="color: var(--color-muted)">
								{t('score_factors.max', 'Max')}
							</label>
							<input
								id="sf-max"
								type="number"
								class="mt-1 w-full rounded-lg border p-2 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={factorForm.config.max}
								disabled={!!selectedFactor?.is_system || !canManageFactors}
							/>
						</div>
					{/if}
					{#if sfIdealVisible(String(factorForm.data_source ?? 'dimension'), factorForm.config)}
						<div>
							<label for="sf-ideal" class="block text-xs font-medium" style="color: var(--color-muted)">
								{t('score_factors.ideal', 'Ideal')}
							</label>
							<input
								id="sf-ideal"
								type="number"
								class="mt-1 w-full rounded-lg border p-2 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={factorForm.config.ideal}
								disabled={!!selectedFactor?.is_system || !canManageFactors}
							/>
						</div>
					{/if}
					{#if sfConfigVisible('missing_score', String(factorForm.data_source ?? 'dimension'))}
						<div>
							<label for="sf-missing-score" class="block text-xs font-medium" style="color: var(--color-muted)">
								{t('score_factors.missing_score', 'Missing score')}
							</label>
							<input
								id="sf-missing-score"
								type="number"
								min="0"
								max="100"
								step="1"
								class="mt-1 w-full rounded-lg border p-2 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={factorForm.config.missing_score}
								disabled={!!selectedFactor?.is_system || !canManageFactors}
							/>
						</div>
					{/if}
					{#if sfConfigVisible('is_output', String(factorForm.data_source ?? 'dimension'))}
						<div class="flex items-center gap-2">
							<input
								id="sf-is-output"
								type="checkbox"
								bind:checked={factorForm.config.is_output}
								disabled={!!selectedFactor?.is_system || !canManageFactors}
							/>
							<label for="sf-is-output" class="text-xs" style="color: var(--color-foreground)">
								{t('score_factors.is_output', 'Is output')}
							</label>
						</div>
					{/if}

					{#if canManageFactors}
						<div class="flex flex-wrap gap-2">
							<Button onclick={saveFactor} disabled={factorSaving || !!selectedFactor?.is_system}>
								{factorSaving ? t('common.saving', 'Saving…') : t('common.save', 'Save')}
							</Button>
							{#if selectedFactor}
								<Button variant="outline" onclick={() => selectFactor(null)}>
									{t('common.add', 'Add')}
								</Button>
								<Button
									variant="destructive"
									onclick={deleteFactor}
									disabled={factorSaving || selectedFactor.is_system}
								>
									{t('common.delete', 'Delete')}
								</Button>
							{/if}
							<Button
								variant="outline"
								onclick={resetToGlobalDefaults}
								disabled={!selectedFactor || selectedFactor.is_system}
							>
								{t('score_factors.reset_to_global', 'Reset to global defaults')}
							</Button>
						</div>
					{/if}
				</div>
			{/if}
		</div>
	{/if}

	{#snippet footer()}
		<Button variant="outline" onclick={() => (scoreFactorOpen = false)}>
			{t('common.close', 'Close')}
		</Button>
	{/snippet}
</SlideOver>

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

{#snippet VisibilityCell({ value }: { value: boolean; row: ComparisonType })}
	<span
		class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {value
			? 'bg-success/15 text-success'
			: 'bg-muted/15 text-muted'}"
	>
		{value ? t('comparison_types.system', 'System') : t('comparison_types.custom', 'Custom')}
	</span>
{/snippet}

<!-- View slide-over body (mirrors the legacy view) -->
{#snippet ComparisonTypeView({ row }: { row: ComparisonType })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('comparison_types.name', 'Name')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('comparison_types.slug', 'Slug')}
			</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('comparison_types.description_label', 'Description')}
			</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('comparison_types.icon', 'Icon')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.icon || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('comparison_types.item_label', 'Item label')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.item_label || '—'}</p>
			</div>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('comparison_types.visibility', 'Visibility')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.is_system ? t('comparison_types.system', 'System') : t('comparison_types.custom', 'Custom')}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('comparison_types.items', 'Items')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.item_count ?? 0}</p>
		</div>
		{#if row.config && Object.keys(row.config).length > 0}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('comparison_types.config', 'Config')}
				</p>
				<pre class="mt-1 max-h-48 overflow-auto rounded-md border p-2 text-xs font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)">{JSON.stringify(row.config, null, 2)}</pre>
			</div>
		{/if}
	</div>
{/snippet}

<!-- Create/Edit form body (rich form: dependent field-group selector + locked fields on edit + score engine repeaters) -->
{#snippet ComparisonTypeForm({ value, editing }: { value: Record<string, any>; editing: any | null })}
	{@const cfg = ensureEngineConfig(value)}
	<div class="space-y-4">
		<Tabs
			tabs={[
				{ value: 'general', label: t('comparison_types.tab_general', 'General') },
				{ value: 'score_engine', label: t('comparison_types.tab_score_engine', 'Score engine') }
			]}
			bind:value={activeFormTab}
		>
			{#if activeFormTab === 'general'}
				<div class="space-y-4">
					<div>
						<label for="ct-name" class="block text-sm font-medium" style="color: var(--color-foreground)">
							{t('comparison_types.name', 'Name')}
						</label>
						<input
							id="ct-name"
							type="text"
							class="mt-1 w-full rounded-lg border p-2 text-sm"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
							bind:value={value.name}
							oninput={() => {
								if (!editing) value.slug = slugify(String(value.name ?? ''));
							}}
						/>
					</div>
					<div>
						<label for="ct-slug" class="block text-sm font-medium" style="color: var(--color-foreground)">
							{t('comparison_types.slug', 'Slug')}
						</label>
						<input
							id="ct-slug"
							type="text"
							class="mt-1 w-full rounded-lg border p-2 text-sm font-mono"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
							bind:value={value.slug}
						/>
					</div>
					<div>
						<label for="ct-desc" class="block text-sm font-medium" style="color: var(--color-foreground)">
							{t('comparison_types.description_label', 'Description')}
						</label>
						<textarea
							id="ct-desc"
							rows="2"
							class="mt-1 w-full rounded-lg border p-2 text-sm"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
							bind:value={value.description}
						></textarea>
					</div>
					<div class="grid grid-cols-2 gap-4">
						<div>
							<label for="ct-icon" class="block text-sm font-medium" style="color: var(--color-foreground)">
								{t('comparison_types.icon', 'Icon')}
							</label>
							<input
								id="ct-icon"
								type="text"
								class="mt-1 w-full rounded-lg border p-2 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={value.icon}
							/>
						</div>
						<div>
							<label for="ct-item-label" class="block text-sm font-medium" style="color: var(--color-foreground)">
								{t('comparison_types.item_label', 'Item label')}
							</label>
							<input
								id="ct-item-label"
								type="text"
								class="mt-1 w-full rounded-lg border p-2 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								bind:value={value.item_label}
							/>
						</div>
					</div>
					<div>
						<label for="ct-field-group" class="block text-sm font-medium" style="color: var(--color-foreground)">
							{t('comparison_types.field_group', 'Field group')}
						</label>
						{#if editing}
							<input
								id="ct-field-group"
								type="text"
								disabled
								value={value.field_group_id}
								class="mt-1 w-full rounded-lg border p-2 text-sm font-mono opacity-60"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
							/>
							<p class="mt-1 text-xs" style="color: var(--color-muted)">
								{t('comparison_types.field_group_locked', 'Field group cannot be changed after creation')}
							</p>
						{:else}
							<Autocomplete
								id="ct-field-group"
								placeholder={t('comparison_types.field_group_select', 'Select a field group…')}
								emptyMessage={t('common.no_results', 'No results found')}
								fetcher={fetchFieldGroupOptions}
								initialFetcher={fetchFieldGroupOptionsInitial}
								selectedId={value.field_group_id ?? null}
								selectedLabel={value.field_group_id ? getFieldGroupSelectedLabel(value.field_group_id) : null}
								onSelect={(item: AutocompleteItem) => { value.field_group_id = item.id; }}
								onClear={() => { value.field_group_id = ''; }}
							/>
						{/if}
					</div>
					<div>
						<label for="ct-config" class="block text-sm font-medium" style="color: var(--color-foreground)">
							{t('comparison_types.config', 'Config (JSON)')}
						</label>
						<textarea
							id="ct-config"
							rows="4"
							class="mt-1 w-full rounded-lg border p-2 text-sm font-mono"
							style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
							value={typeof value.config === 'string' ? value.config : JSON.stringify(value.config ?? {}, null, 2)}
							onchange={(e) => {
								try {
									value.config = JSON.parse(e.currentTarget.value) as Record<string, unknown>;
								} catch {
									// Keep the previous valid config if the user input is not valid JSON.
								}
							}}
						></textarea>
						<p class="mt-1 text-xs" style="color: var(--color-muted)">
							{t('comparison_types.config_hint', 'Optional JSON configuration for this comparison type')}
						</p>
					</div>
					<div class="rounded-lg border p-3 space-y-3" style="border-color: var(--color-border)">
						<div class="flex items-center gap-2">
							<input
								id="ct-allow-nested"
								type="checkbox"
								checked={Boolean(configObj(value).allow_nested_comparisons)}
								onchange={(e) => { configObj(value).allow_nested_comparisons = e.currentTarget.checked; }}
							/>
							<label for="ct-allow-nested" class="text-sm" style="color: var(--color-foreground)">
								{t('comparison_types.allow_nested_comparisons', 'Allow nested comparisons')}
							</label>
						</div>
						{#if Boolean(configObj(value).allow_nested_comparisons)}
							<div>
								<p class="text-xs font-medium" style="color: var(--color-muted)">
									{t('comparison_types.allowed_nested_types', 'Allowed nested comparison types')}
								</p>
								<div class="mt-2 flex flex-wrap gap-2">
									{#each allComparisonTypes.filter((t) => !editing || t.slug !== editing.slug) as type (type.id)}
										<label class="inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs" style="border-color: var(--color-border); color: var(--color-foreground)">
											<input
												type="checkbox"
												checked={isNestedTypeAllowed(value, type.slug)}
												onchange={() => toggleNestedType(value, type.slug)}
											/>
											{type.name}
										</label>
									{/each}
								</div>
							</div>
						{/if}
					</div>
					<div class="flex items-center gap-2">
						<input id="ct-system" type="checkbox" bind:checked={value.is_system} disabled={!!editing} />
						<label for="ct-system" class="text-sm" style="color: var(--color-foreground)">
							{t('comparison_types.system', 'System')}
						</label>
					</div>
				</div>
			{/if}

			{#if activeFormTab === 'score_engine'}
				<div class="space-y-6">
					<!-- Allowed goals -->
					<div class="space-y-2">
						<div class="flex items-center justify-between">
							<h3 class="text-sm font-semibold" style="color: var(--color-foreground)">
								{t('score_engine.allowed_goals', 'Allowed goals')}
							</h3>
							<Button variant="outline" size="sm" onclick={() => addAllowedGoal(value)}>
								{t('common.add', 'Add')}
							</Button>
						</div>
						{#if (cfg.allowed_goals ?? []).length === 0}
							<p class="text-xs" style="color: var(--color-muted)">
								{t('score_engine.no_allowed_goals', 'No allowed goals configured')}
							</p>
						{:else}
							<div class="space-y-2">
								{#each (cfg.allowed_goals ?? []) as goal, i (i)}
									<div class="grid grid-cols-12 gap-2 rounded-md border p-2" style="border-color: var(--color-border)">
										<div class="col-span-3">
											<input
												type="text"
												placeholder={t('score_engine.goal_key', 'Key')}
												class="w-full rounded border p-1.5 text-xs"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												bind:value={goal.key}
											/>
										</div>
										<div class="col-span-3">
											<input
												type="text"
												placeholder={t('score_engine.goal_label', 'Label')}
												class="w-full rounded border p-1.5 text-xs"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												bind:value={goal.label}
											/>
										</div>
										<div class="col-span-3">
											<input
												type="text"
												placeholder={t('score_engine.goal_description', 'Description')}
												class="w-full rounded border p-1.5 text-xs"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												bind:value={goal.description}
											/>
										</div>
										<div class="col-span-2">
											<input
												type="number"
												min="0"
												max="1"
												step="0.01"
												placeholder={t('score_engine.default_weight', 'Weight')}
												class="w-full rounded border p-1.5 text-xs"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												bind:value={goal.default_weight}
											/>
										</div>
										<div class="col-span-1 flex items-center justify-end">
											<Button variant="outline" size="sm" onclick={() => removeAllowedGoal(value, i)}>
												<Trash2 class="h-3.5 w-3.5" />
											</Button>
										</div>
									</div>
								{/each}
							</div>
						{/if}
					</div>

					<!-- Eligibility rules -->
					<div class="space-y-2">
						<div class="flex items-center justify-between">
							<h3 class="text-sm font-semibold" style="color: var(--color-foreground)">
								{t('score_engine.eligibility_rules', 'Eligibility rules')}
							</h3>
							<Button variant="outline" size="sm" onclick={() => addEligibilityRule(value)}>
								{t('common.add', 'Add')}
							</Button>
						</div>
						{#if (cfg.eligibility_rules ?? []).length === 0}
							<p class="text-xs" style="color: var(--color-muted)">
								{t('score_engine.no_eligibility_rules', 'No eligibility rules configured')}
							</p>
						{:else}
							<div class="space-y-2">
								{#each (cfg.eligibility_rules ?? []) as rule, i (i)}
									<div class="grid grid-cols-12 gap-2 rounded-md border p-2" style="border-color: var(--color-border)">
										<div class="col-span-2">
											<select
												class="w-full rounded border p-1.5 text-xs"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												bind:value={rule.operator}
											>
												{#each ENGINE_OPERATORS as op (op)}
													<option value={op}>{op}</option>
												{/each}
											</select>
										</div>
										<div class="col-span-3">
											<input
												type="text"
												placeholder={t('score_engine.rule_field', 'Field')}
												class="w-full rounded border p-1.5 text-xs"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												bind:value={rule.field}
											/>
										</div>
										<div class="col-span-3">
											<input
												type="text"
												placeholder={t('score_engine.rule_value', 'Value')}
												class="w-full rounded border p-1.5 text-xs"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												bind:value={rule.value}
											/>
										</div>
										<div class="col-span-2">
											<input
												type="text"
												placeholder={t('score_engine.rule_on_fail', 'On fail')}
												class="w-full rounded border p-1.5 text-xs"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												bind:value={rule.on_fail}
											/>
										</div>
										<div class="col-span-2 flex items-center justify-end">
											<Button variant="outline" size="sm" onclick={() => removeEligibilityRule(value, i)}>
												<Trash2 class="h-3.5 w-3.5" />
											</Button>
										</div>
									</div>
								{/each}
							</div>
						{/if}
					</div>

					<!-- Recommendation templates -->
					<div class="space-y-2">
						<div class="flex items-center justify-between">
							<h3 class="text-sm font-semibold" style="color: var(--color-foreground)">
								{t('score_engine.recommendation_templates', 'Recommendation templates')}
							</h3>
							<Button variant="outline" size="sm" onclick={() => addRecommendationTemplate(value)}>
								{t('common.add', 'Add')}
							</Button>
						</div>
						{#if (cfg.recommendation_templates ?? []).length === 0}
							<p class="text-xs" style="color: var(--color-muted)">
								{t('score_engine.no_recommendation_templates', 'No recommendation templates configured')}
							</p>
						{:else}
							<div class="space-y-2">
								{#each (cfg.recommendation_templates ?? []) as template, i (i)}
									<div class="grid grid-cols-12 gap-2 rounded-md border p-2" style="border-color: var(--color-border)">
										<div class="col-span-4">
											<textarea
												rows="2"
												placeholder={t('score_engine.template_text', 'Text')}
												class="w-full rounded border p-1.5 text-xs"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												bind:value={template.text}
											></textarea>
										</div>
										<div class="col-span-2">
											<input
												type="text"
												placeholder={t('score_engine.action_type', 'Action type')}
												class="w-full rounded border p-1.5 text-xs"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												bind:value={template.action_type}
											/>
										</div>
										<div class="col-span-2">
											<input
												type="text"
												placeholder={t('score_engine.action_url', 'Action URL')}
												class="w-full rounded border p-1.5 text-xs"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												bind:value={template.action_url}
											/>
										</div>
										<div class="col-span-2">
											<input
												type="number"
												placeholder={t('score_engine.priority', 'Priority')}
												class="w-full rounded border p-1.5 text-xs"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												bind:value={template.priority}
											/>
										</div>
										<div class="col-span-2 flex items-center justify-end">
											<Button variant="outline" size="sm" onclick={() => removeRecommendationTemplate(value, i)}>
												<Trash2 class="h-3.5 w-3.5" />
											</Button>
										</div>
										<div class="col-span-3">
											<select
												class="w-full rounded border p-1.5 text-xs"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												bind:value={template.condition}
											>
												<option value="">{t('score_engine.condition', 'Condition')}</option>
												{#each RECOMMENDATION_CONDITIONS as c (c)}
													<option value={c}>{c}</option>
												{/each}
											</select>
										</div>
										<div class="col-span-2">
											<input
												type="number"
												placeholder={t('score_engine.threshold', 'Threshold')}
												class="w-full rounded border p-1.5 text-xs"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												bind:value={template.threshold}
											/>
										</div>
										<div class="col-span-7">
											<input
												type="text"
												placeholder={t('score_engine.statuses', 'Statuses (comma-separated, status_in only)')}
												class="w-full rounded border p-1.5 text-xs"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												value={Array.isArray(template.statuses) ? template.statuses.join(', ') : template.statuses ?? ''}
												onchange={(e) => {
													template.statuses = e.currentTarget.value;
												}}
											/>
										</div>
									</div>
								{/each}
							</div>
						{/if}
					</div>

					<!-- Smart modes -->
				<div class="space-y-2">
					<label for="ct-smart-modes" class="block text-sm font-medium" style="color: var(--color-foreground)">
						{t('score_engine.smart_modes', 'Smart modes (JSON)')}
					</label>
					<textarea
						id="ct-smart-modes"
						rows="4"
						placeholder={t('score_engine.smart_modes_hint', '{ "eligibility": { "filter_ineligible": true, "factor_multipliers": {} } }')}
						class="w-full rounded border p-1.5 text-xs font-mono"
						style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
						value={jsonToString(ensureEngineConfig(value).smart_modes, '{}')}
						onchange={(e) => { ensureEngineConfig(value).smart_modes = e.currentTarget.value; }}
					></textarea>
					<p class="text-xs" style="color: var(--color-muted)">
						{t('score_engine.smart_modes_help', 'Each key is a smart mode; value is { filter_ineligible: boolean, factor_multipliers: { factor_key: multiplier } }.')}
					</p>
				</div>

				<!-- Default weights -->
					<div class="space-y-2">
						<div class="flex items-center justify-between">
							<h3 class="text-sm font-semibold" style="color: var(--color-foreground)">
								{t('score_engine.default_weights', 'Default weights')}
							</h3>
							<Button variant="outline" size="sm" onclick={() => addDefaultWeight(value)}>
								{t('common.add', 'Add')}
							</Button>
						</div>
						{#if (cfg.default_weights ?? []).length === 0}
							<p class="text-xs" style="color: var(--color-muted)">
								{t('score_engine.no_default_weights', 'No default weights configured')}
							</p>
						{:else}
							<div class="space-y-2">
								{#each (cfg.default_weights ?? []) as weight, i (i)}
									<div class="grid grid-cols-12 gap-2 rounded-md border p-2" style="border-color: var(--color-border)">
										<div class="col-span-9">
											<input
												type="text"
												placeholder={t('score_engine.factor_key', 'Factor key')}
												class="w-full rounded border p-1.5 text-xs"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												bind:value={weight.factor_key}
											/>
										</div>
										<div class="col-span-2">
											<input
												type="number"
												min="0"
												max="1"
												step="0.01"
												placeholder={t('score_engine.weight', 'Weight')}
												class="w-full rounded border p-1.5 text-xs"
												style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
												bind:value={weight.weight}
											/>
										</div>
										<div class="col-span-1 flex items-center justify-end">
											<Button variant="outline" size="sm" onclick={() => removeDefaultWeight(value, i)}>
												<Trash2 class="h-3.5 w-3.5" />
											</Button>
										</div>
									</div>
								{/each}
							</div>
						{/if}
					</div>
				</div>
			{/if}
		</Tabs>
	</div>
{/snippet}
