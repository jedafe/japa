<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /gamification — migrated to the EntityPage scaffold.
	// Two tabs: Badges (full CRUD + award) and Challenges (full CRUD).
	// Rich forms (ActivityKeyAutocomplete) use the formSnippet escape hatch.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Tabs } from '$lib/components/ui/tabs';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import ActivityKeyAutocomplete from '$lib/components/gamification/ActivityKeyAutocomplete.svelte';
	import UserAutocomplete from '$lib/components/ui/UserAutocomplete.svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';
	import { Trophy, Target, Pencil, Award, Trash2, Eye, CalendarDays, Layers, ScrollText } from 'lucide-svelte';
	import { api, apiRaw, adminCatalogApi, featureFlagsAdminApi, type CatalogOption } from '$lib/api';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import MediaPickerModal from '$lib/components/ui/MediaPickerModal.svelte';

	type Badge = {
		id: string;
		slug: string;
		name: string;
		description: string;
		icon_media_id?: string;
		trigger_type: string;
		trigger_value: number | null;
		trigger_activity_key: string | null;
		is_active: boolean;
		is_system: boolean;
		display_order: number;
		category?: string;
		rarity?: string;
		tier?: string;
		badge_group?: string;
		is_secret?: boolean;
		is_limited?: boolean;
		is_featured?: boolean;
		points_reward?: number;
		available_from?: string;
		available_until?: string;
		feature_flag?: string | null;
		created_at: string;
	};

	type Challenge = {
		id: string;
		slug: string;
		name: string;
		description: string;
		image_media_id?: string;
		activity_key: string;
		target_count: number;
		reward_points: number;
		reward_badge_id: string | null;
		starts_at: string;
		ends_at: string | null;
		is_active: boolean;
		feature_flag?: string | null;
		created_at: string;
	};

	// Permission nodes per JEOS §64 — gamification.manage does NOT exist.
	const canManageBadges = $derived(hasPermission('gamification.manage_badges'));
	const canManageChallenges = $derived(hasPermission('gamification.manage_challenges'));
	const canAwardBadge = $derived(hasPermission('gamification.award_badge'));

	let activeTab = $state('badges');
	const tabs = $derived([
		{ value: 'badges', label: t('gamification.badges', 'Badges'), icon: Trophy },
		{ value: 'challenges', label: t('gamification.challenges', 'Challenges'), icon: Target },
		{ value: 'seasons', label: t('gamification.seasons', 'Seasons'), icon: CalendarDays },
		{ value: 'tiers', label: t('gamification.tiers', 'Tiers'), icon: Layers },
		{ value: 'awards', label: t('gamification.awards', 'Award Events'), icon: ScrollText }
	]);

	// ── Controllers ────────────────────────────────────────────────────────
	let badgeController = $state<EntityPageController | undefined>(undefined);
	let challengeController = $state<EntityPageController | undefined>(undefined);

	// ── Page-local state for sub-flows ─────────────────────────────────────
	let pageSuccess = $state('');
	let acting = $state(false);
	let successTimer: ReturnType<typeof setTimeout> | undefined;

	// Award badge modal
	let awardModalOpen = $state(false);
	let awardBadgeId = $state('');
	let awardBadgeName = $state('');
	let awardUsername = $state('');
	let awardTargetUserId = $state('');
	let awardSelectedUsername = $state('');
	let awardError = $state('');

	// Delete confirmation error (shown inside the modal, not at page top).
	let deleteError = $state('');

	// Media pickers — badge icon (icon_media_id) and challenge image
	// (image_media_id). The API stores only the media id; this map keeps the
	// selected URL so the form can render a thumbnail.
	let badgeIconPickerOpen = $state(false);
	let challengeImagePickerOpen = $state(false);
	let mediaUrlById = $state<Record<string, string>>({});
	// Set by the form's "Choose" button so the picker writes the selected
	// media into the correct form field.
	let pendingMediaSelect = $state<((m: { id: string; url: string; filename: string }) => void) | undefined>(undefined);

	function flashSuccess(msg: string) {
		pageSuccess = msg;
		if (successTimer) clearTimeout(successTimer);
		successTimer = setTimeout(() => (pageSuccess = ''), 5000);
	}

	// Delete confirmation (shared for badges + challenges)
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<{ kind: 'badge' | 'challenge'; id: string; name: string } | null>(null);

	// The admin list endpoints paginate server-side (default limit 50) —
	// page through until the full set is collected so nothing is silently
	// dropped from the UI (PTS-005). `apiRaw` keeps the top-level
	// `pagination` block that `api()` strips from the envelope.
	async function fetchAll<T>(base: string): Promise<T[]> {
		const pageSize = 200;
		const out: T[] = [];
		for (let offset = 0; ; offset += pageSize) {
			const res = await apiRaw<{ data?: T[]; pagination?: { total?: number } }>(
				`${base}?limit=${pageSize}&offset=${offset}`
			);
			if (!res.ok) throw new Error(res.error ?? 'request failed');
			const page = res.data?.data ?? [];
			out.push(...page);
			const total = res.data?.pagination?.total;
			if (page.length < pageSize || (total !== undefined && out.length >= total)) break;
		}
		return out;
	}

	// ── Payload helpers ──────────────────────────────────────────────────

	// toRFC3339 converts a native date-input value ('YYYY-MM-DD') into the
	// RFC 3339 timestamp the backend expects; values already in RFC 3339
	// form (an untouched seeded field) pass through. `endOfDay` maps to
	// 23:59:59Z so an end date is inclusive.
	function toRFC3339(v: unknown, endOfDay = false): string | undefined {
		if (typeof v !== 'string' || !v) return undefined;
		if (/^\d{4}-\d{2}-\d{2}$/.test(v)) return `${v}T${endOfDay ? '23:59:59' : '00:00:00'}Z`;
		return v;
	}

	// toDateInput renders a stored RFC 3339 value as 'YYYY-MM-DD' for a
	// native <input type="date">.
	function toDateInput(v: unknown): string {
		return typeof v === 'string' && v.length >= 10 ? v.slice(0, 10) : '';
	}

	// intOr normalizes a form value to an integer — empty inputs are
	// dropped so the backend treats them as unset rather than failing to
	// decode a "" into *int32.
	function intOr(v: unknown): number | undefined {
		if (v === '' || v === null || v === undefined) return undefined;
		const n = Number(v);
		return Number.isFinite(n) ? Math.trunc(n) : undefined;
	}

	// applySort orders a client-side page set by the DataTable sort param
	// ('key:dir'). Unknown keys and equal values fall back to the default
	// comparator so ordering stays stable.
	function applySort<T>(
		rows: T[],
		sort: string,
		accessors: Record<string, (r: T) => string | number | null | undefined>,
		fallback: (a: T, b: T) => number
	) {
		const [key, dir] = (sort || '').split(':');
		const get = key ? accessors[key] : undefined;
		const mul = dir === 'desc' ? -1 : 1;
		rows.sort((a, b) => {
			if (get) {
				const av = get(a);
				const bv = get(b);
				if (av == null && bv == null) return fallback(a, b);
				if (av == null) return 1;
				if (bv == null) return -1;
				const cmp =
					typeof av === 'number' && typeof bv === 'number'
						? av - bv
						: String(av).localeCompare(String(bv));
				if (cmp !== 0) return cmp * mul;
			}
			return fallback(a, b);
		});
	}

	// ── Client-side validation (mirrors §64 service constraints) ─────────

	function validateBadgeTrigger(
		triggerType: string | undefined,
		triggerValue: number | undefined,
		activityKey: string | undefined
	): string | null {
		if (!triggerType) return t('gamification.err_trigger_required', 'Trigger type is required');
		if (triggerType === 'lifetime_points' || triggerType === 'activity_count' || triggerType === 'streak_days') {
			if (!triggerValue || triggerValue <= 0)
				return t('gamification.err_trigger_value', 'A positive trigger value is required for this trigger type');
		}
		if ((triggerType === 'activity_count' || triggerType === 'streak_days') && !activityKey?.trim())
			return t('gamification.err_activity_key', 'An activity key is required for this trigger type');
		return null;
	}

	function validateBadgeBody(body: Record<string, any>): string | null {
		if (!body.name?.trim()) return t('gamification.err_name_required', 'Name is required');
		if (!body.slug?.trim()) return t('gamification.err_slug_required', 'Slug is required');
		return validateBadgeTrigger(body.trigger_type, intOr(body.trigger_value), body.trigger_activity_key);
	}

	function validateChallengeBody(body: Record<string, any>): string | null {
		if (!body.name?.trim()) return t('gamification.err_name_required', 'Name is required');
		if (!body.slug?.trim()) return t('gamification.err_slug_required', 'Slug is required');
		if (!body.activity_key?.trim())
			return t('gamification.err_activity_key_required', 'Activity key is required');
		const target = intOr(body.target_count);
		if (!target || target <= 0)
			return t('gamification.err_target', 'Target count must be a positive number');
		if (!toRFC3339(body.starts_at))
			return t('gamification.err_starts', 'Start date is required');
		const startsAt = toRFC3339(body.starts_at);
		const endsAt = toRFC3339(body.ends_at, true);
		if (startsAt && endsAt && endsAt < startsAt)
			return t('gamification.err_ends_before_starts', 'End date must be after the start date');
		return null;
	}

	// Shared single-flight list fetchers — the activity-key aggregation
	// effect and both EntityPage `load` callbacks share the same cached
	// promise instead of issuing duplicate requests. Mutations call
	// invalidateLists() so the next reload refetches.
	let badgesCache: Promise<Badge[]> | null = null;
	let challengesCache: Promise<Challenge[]> | null = null;
	const badgesList = () =>
		(badgesCache ??= fetchAll<Badge>('/admin/gamification/badges').catch((e) => {
			badgesCache = null;
			throw e;
		}));
	const challengesList = () =>
		(challengesCache ??= fetchAll<Challenge>('/admin/gamification/challenges').catch((e) => {
			challengesCache = null;
			throw e;
		}));

	// Aggregate all known activity keys from already-loaded badges and
	// challenges so the autocomplete can suggest them without an extra API
	// call. The ActivityKeyAutocomplete also fetches wallet rules internally.
	let knownActivityKeys = $state<string[]>([]);

	// Reward-badge options for the challenge form (reward_badge_id was
	// previously unsettable from the UI — PTS-012).
	let badgeOptions = $state<Array<{ value: string; label: string }>>([]);

	async function refreshMeta() {
		try {
			const [badgesRes, challengesRes] = await Promise.all([badgesList(), challengesList()]);
			const set = new Set<string>();
			for (const b of badgesRes) {
				if (b.trigger_activity_key) set.add(b.trigger_activity_key);
			}
			for (const c of challengesRes) {
				if (c.activity_key) set.add(c.activity_key);
			}
			knownActivityKeys = Array.from(set);
			badgeOptions = badgesRes.map((b) => ({ value: b.id, label: b.name }));
		} catch {
			// non-fatal — autocomplete still has wallet-rule suggestions
		}
	}

	function invalidateLists() {
		badgesCache = null;
		challengesCache = null;
		void refreshMeta();
	}

	// Feature-flag options for the feature_flag selects — same source the
	// menu-item flag picker uses; falls back to the public details
	// projection when the admin list is unavailable.
	let flagOptions = $state<{ key: string; name?: string }[]>([]);
	let flagOptionsLoaded = false;
	async function loadFlagOptions() {
		if (flagOptionsLoaded) return;
		try {
			const result = await featureFlagsAdminApi.list();
			if (result.ok) {
				flagOptions = result.data ?? [];
				flagOptionsLoaded = true;
				return;
			}
		} catch {
			// fall through to the public projection
		}
		try {
			const result = await featureFlagsAdminApi.listDetails();
			if (result.ok) flagOptions = result.data ?? [];
		} catch {
			flagOptions = [];
		}
		flagOptionsLoaded = true;
	}

	// featureFlagSelectOptions always carries the current value so the
	// select never renders blank when a flag was unset in the registry or
	// the catalog fetch failed.
	function featureFlagSelectOptions(current?: string | null): CatalogOption[] {
		return [
			{ value: '', label: t('gamification.no_feature_flag', 'None — always visible') },
			...(current && !flagOptions.some((f) => f.key === current)
				? [{ value: current, label: `${current} (custom)` }]
				: []),
			...flagOptions.map((f) => ({ value: f.key, label: f.name ? `${f.name} (${f.key})` : f.key }))
		];
	}

	$effect(() => {
		void refreshMeta();
		void loadFlagOptions();
	});

	// primeMediaUrl resolves an existing media id to its file URL so the
	// icon/image thumbnail shows on edit (GET /media/{id}).
	async function primeMediaUrl(id?: string) {
		if (!id || mediaUrlById[id]) return;
		const res = await apiRaw<{ data?: { url?: string; file_url?: string; cdn_url?: string } }>(`/media/${id}`);
		if (!res.ok) return;
		const m = res.data?.data;
		const url = m?.url ?? m?.file_url ?? m?.cdn_url;
		if (url) mediaUrlById = { ...mediaUrlById, [id]: url };
	}

	// Trigger type options matching the backend CHECK constraint (§64 schema).
	let triggerTypeOptions = $state<CatalogOption[]>([
		{ value: 'lifetime_points', label: t('gamification.trigger_lifetime_points', 'Lifetime Points') },
		{ value: 'activity_count', label: t('gamification.trigger_activity_count', 'Activity Count') },
		{ value: 'streak_days', label: t('gamification.trigger_streak_days', 'Streak Days') },
		{ value: 'manual', label: t('gamification.trigger_manual', 'Manual') }
	]);

	$effect(() => {
		void (async () => {
			const res = await adminCatalogApi.get('gamification.trigger_type');
			if (res.ok && res.data?.length) triggerTypeOptions = res.data;
		})();
	});

	// ── Badge EntityPage ───────────────────────────────────────────────────

	// Category/rarity/tier options come from the §64 admin catalogs
	// (gamification.badge_category / .badge_rarity / .badge_tier). The
	// seed-value defaults keep the form usable if the catalog call fails.
	let badgeCategories = $state<CatalogOption[]>(
		['getting_started', 'readiness', 'journey', 'countries', 'education',
			'career', 'community', 'content', 'forum', 'mentorship', 'services',
			'financial', 'travel', 'trust', 'consistency', 'special', 'comparison',
			'academy', 'housing', 'documents', 'ai', 'growth'
		].map((v) => ({ value: v, label: v.replace(/_/g, ' ') }))
	);

	let rarityOptions = $state<CatalogOption[]>(
		['common', 'uncommon', 'rare', 'epic', 'legendary', 'mythic'].map((v) => ({
			value: v, label: v[0].toUpperCase() + v.slice(1)
		}))
	);

	let tierOptions = $state<CatalogOption[]>(
		['bronze', 'silver', 'gold', 'platinum', 'diamond'].map((v) => ({
			value: v, label: v[0].toUpperCase() + v.slice(1)
		}))
	);

	$effect(() => {
		void (async () => {
			const [catRes, rarityRes, tierRes] = await Promise.all([
				adminCatalogApi.get('gamification.badge_category'),
				adminCatalogApi.get('gamification.badge_rarity'),
				adminCatalogApi.get('gamification.badge_tier')
			]);
			if (catRes.ok && catRes.data?.length) badgeCategories = catRes.data;
			if (rarityRes.ok && rarityRes.data?.length) rarityOptions = rarityRes.data;
			if (tierRes.ok && tierRes.data?.length) tierOptions = tierRes.data;
		})();
	});

	const badgeFields = $derived<FieldDef[]>([
		{ key: 'name', label: t('gamification.name', 'Name'), type: 'text', required: true },
		{ key: 'slug', label: t('gamification.slug', 'Slug'), type: 'text', required: true, readonlyInEdit: true },
		{ key: 'description', label: t('gamification.description', 'Description'), type: 'textarea' },
		{ key: 'icon_media_id', label: t('gamification.icon', 'Icon'), type: 'text' },
		{ key: 'trigger_type', label: t('gamification.trigger', 'Trigger Type'), type: 'select', options: triggerTypeOptions, required: true },
		{ key: 'trigger_value', label: t('gamification.trigger_value', 'Trigger Value'), type: 'number' },
		{ key: 'trigger_activity_key', label: t('gamification.activity_key', 'Activity Key'), type: 'text', help: t('gamification.activity_key_help', 'Avoid reversible keys (reaction.*, *.voted) — vote/unvote cycles can inflate activity_count progress') },
		{ key: 'category', label: t('gamification.category', 'Category'), type: 'select', options: badgeCategories },
		{ key: 'rarity', label: t('gamification.rarity', 'Rarity'), type: 'select', options: rarityOptions },
		{ key: 'accent_color', label: t('gamification.accent_color', 'Accent Color'), type: 'color' },
		{ key: 'tier', label: t('gamification.tier', 'Tier'), type: 'select', options: tierOptions },
		{ key: 'badge_group', label: t('gamification.badge_group', 'Badge Group'), type: 'text' },
		{ key: 'points_reward', label: t('gamification.points_reward', 'Points Reward'), type: 'number' },
		{ key: 'is_secret', label: t('gamification.secret', 'Secret'), type: 'checkbox' },
		{ key: 'is_limited', label: t('gamification.limited', 'Limited'), type: 'checkbox' },
		{ key: 'is_featured', label: t('gamification.featured', 'Featured'), type: 'checkbox' },
		{ key: 'available_from', label: t('gamification.available_from', 'Available From'), type: 'datetime-local' },
		{ key: 'available_until', label: t('gamification.available_until', 'Available Until'), type: 'datetime-local' },
		{ key: 'feature_flag', label: t('gamification.feature_flag', 'Feature Flag'), type: 'select', options: featureFlagSelectOptions() },
		{ key: 'is_active', label: t('gamification.active', 'Active'), type: 'checkbox', defaultValue: true },
		{ key: 'display_order', label: t('gamification.display_order', 'Display Order'), type: 'number' }
	]);

	const badgeFilters = $derived<FilterDef[]>([
		{
			key: 'is_active',
			label: t('gamification.active', 'Active'),
			options: [
				{ value: 'true', label: t('common.active', 'Active') },
				{ value: 'false', label: t('common.inactive', 'Inactive') }
			]
		},
		{
			key: 'trigger_type',
			label: t('gamification.trigger', 'Trigger'),
			options: triggerTypeOptions
		}
	]);

	const badgeActions = $derived<ActionItem[]>([
		{
			label: t('common.view', 'View'),
			icon: Eye,
			onclick: (row: Badge, ctx) => ctx?.openView(row)
		},
		{
			label: t('gamification.award', 'Award'),
			icon: Award,
			onclick: (row: Badge) => openAwardBadge(row),
			show: () => canAwardBadge
		},
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row: Badge, ctx) => {
				void primeMediaUrl(row.icon_media_id);
				ctx?.openEdit(row);
			},
			// §64: only DELETE is restricted to non-system badges — seeded
			// (is_system) badges must stay editable so admins can activate
			// them as the phased catalogue rolls out (is_active toggle).
			show: () => canManageBadges
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row: Badge) => promptDelete('badge', row.id, row.name),
			show: (row: Badge) => canManageBadges && !row.is_system
		}
	]);

	const badgeConfig = $derived<EntityPageConfig>({
		title: t('gamification.badges', 'Badges'),
		description: t('gamification.badges_description', 'Manage achievement badges and triggers'),
		createLabel: t('gamification.create_badge', 'Create Badge'),
		createPermission: 'gamification.manage_badges',
		editPermission: 'gamification.manage_badges',
		deletePermission: 'gamification.manage_badges',
		pageSize: 25,
		columns: [
			{
				key: 'name',
				label: t('gamification.name', 'Name'),
				sortable: true,
				render: (value: string) => `<span class="font-medium text-foreground">${value}</span>`
			},
			{
				key: 'slug',
				label: t('gamification.slug', 'Slug'),
				sortable: true,
				render: (value: string) => `<span class="font-mono text-xs text-muted-foreground">${value}</span>`
			},
			{
				key: 'trigger_type',
				label: t('gamification.trigger', 'Trigger'),
				sortable: true,
				render: (value: string) => `<span class="text-foreground">${value}</span>`
			},
			{
				key: 'is_active',
				label: t('gamification.active', 'Active'),
				sortable: true,
				render: (value: boolean) => value
					? `<span class="inline-flex items-center rounded-md border border-success bg-success/15 px-2 py-0.5 text-xs font-medium text-success">${t('common.active', 'Active')}</span>`
					: `<span class="inline-flex items-center rounded-md border border-border bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">${t('common.inactive', 'Inactive')}</span>`
			},
			{
				key: 'is_system',
				label: t('gamification.system', 'System'),
				sortable: true,
				render: (value: boolean) => value ? `<span class="text-xs text-muted-foreground">${t('common.system', 'System')}</span>` : `<span class="text-xs text-muted-foreground">${t('common.custom', 'Custom')}</span>`
			}
		],
		filters: badgeFilters,
		actions: badgeActions,
		fields: badgeFields,
		load: async ({ page, pageSize, q, filter, sort }) => {
			let data = await badgesList().catch(() => {
				throw new Error(t('gamification.load_error', 'Failed to load gamification data'));
			});
			// Client-side search
			if (q) {
				const query = q.toLowerCase();
				data = data.filter((b) =>
					b.name.toLowerCase().includes(query) ||
					b.slug.toLowerCase().includes(query)
				);
			}
			// Client-side filters
			if (filter.is_active) {
				const active = filter.is_active === 'true';
				data = data.filter((b) => b.is_active === active);
			}
			if (filter.trigger_type) {
				data = data.filter((b) => b.trigger_type === filter.trigger_type);
			}
			// Client-side sort — honours the column the admin clicked.
			applySort(data, sort, {
				name: (b) => b.name,
				slug: (b) => b.slug,
				trigger_type: (b) => b.trigger_type,
				is_active: (b) => (b.is_active ? 1 : 0),
				is_system: (b) => (b.is_system ? 1 : 0)
			}, (a, b) => a.display_order - b.display_order || a.name.localeCompare(b.name));
			const total = data.length;
			const totalPages = Math.max(1, Math.ceil(total / pageSize));
			const start = page * pageSize;
			return { data: data.slice(start, start + pageSize), total, totalPages };
		},
		create: async (body) => {
			const err = validateBadgeBody(body);
			if (err) return { ok: false, error: err };
			const res = await api('/admin/gamification/badges', {
				method: 'POST',
				body: {
					name: body.name.trim(),
					slug: body.slug.trim(),
					description: body.description ?? '',
					icon_media_id: body.icon_media_id || undefined,
					accent_color: body.accent_color || undefined,
					trigger_type: body.trigger_type,
					trigger_value: intOr(body.trigger_value),
					trigger_activity_key: body.trigger_activity_key?.trim() || undefined,
					category: body.category || undefined,
					rarity: body.rarity || undefined,
					tier: body.tier || undefined,
					badge_group: body.badge_group?.trim() || undefined,
					points_reward: intOr(body.points_reward),
					is_secret: typeof body.is_secret === 'boolean' ? body.is_secret : undefined,
					is_limited: typeof body.is_limited === 'boolean' ? body.is_limited : undefined,
					is_featured: typeof body.is_featured === 'boolean' ? body.is_featured : undefined,
					available_from: toISO(body.available_from),
					available_until: toISO(body.available_until),
					feature_flag: body.feature_flag?.trim() || undefined,
					is_active: typeof body.is_active === 'boolean' ? body.is_active : undefined,
					display_order: intOr(body.display_order)
				}
			});
			if (res.ok) invalidateLists();
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body, row) => {
			if (!body.name?.trim()) return { ok: false, error: t('gamification.err_name_required', 'Name is required') };
			const err = validateBadgeTrigger(body.trigger_type, intOr(body.trigger_value), body.trigger_activity_key);
			if (err) return { ok: false, error: err };
			const payload: Record<string, unknown> = {
				name: body.name.trim(),
				description: body.description ?? '',
				trigger_type: body.trigger_type || undefined,
				trigger_value: intOr(body.trigger_value),
				trigger_activity_key: body.trigger_activity_key?.trim() || undefined,
				category: body.category || undefined,
				rarity: body.rarity || undefined,
				tier: body.tier || undefined,
				badge_group: body.badge_group?.trim() || undefined,
				points_reward: intOr(body.points_reward),
				is_secret: typeof body.is_secret === 'boolean' ? body.is_secret : undefined,
				is_limited: typeof body.is_limited === 'boolean' ? body.is_limited : undefined,
				is_featured: typeof body.is_featured === 'boolean' ? body.is_featured : undefined,
				available_from: toISO(body.available_from),
				available_until: toISO(body.available_until),
				feature_flag: body.feature_flag?.trim() || undefined,
				is_active: typeof body.is_active === 'boolean' ? body.is_active : undefined,
				display_order: intOr(body.display_order)
			};
			if (body.icon_media_id) payload.icon_media_id = body.icon_media_id;
			else if (row?.icon_media_id) payload.clear_icon_media_id = true;
			if (body.accent_color) payload.accent_color = body.accent_color;
			else if (row?.accent_color) payload.clear_accent_color = true;
			if (payload.trigger_value === undefined && row?.trigger_value != null) payload.clear_trigger_value = true;
			if (payload.trigger_activity_key === undefined && row?.trigger_activity_key) payload.clear_trigger_activity_key = true;
			if (payload.category === undefined && row?.category) payload.clear_category = true;
			if (payload.tier === undefined && row?.tier) payload.clear_tier = true;
			if (payload.badge_group === undefined && row?.badge_group) payload.clear_badge_group = true;
			if (payload.available_from === undefined && row?.available_from) payload.clear_available_from = true;
			if (payload.available_until === undefined && row?.available_until) payload.clear_available_until = true;
			if (payload.feature_flag === undefined && row?.feature_flag) payload.clear_feature_flag = true;
			const res = await api(`/admin/gamification/badges/${id}`, { method: 'PATCH', body: payload });
			if (res.ok) invalidateLists();
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/gamification/badges/${id}`, { method: 'DELETE' });
			if (res.ok) invalidateLists();
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: BadgeView,
		formSnippet: BadgeForm
	});

	// ── Challenge EntityPage ───────────────────────────────────────────────

	const challengeFields = $derived<FieldDef[]>([
		{ key: 'name', label: t('gamification.name', 'Name'), type: 'text', required: true },
		{ key: 'slug', label: t('gamification.slug', 'Slug'), type: 'text', required: true, readonlyInEdit: true },
		{ key: 'description', label: t('gamification.description', 'Description'), type: 'textarea' },
		{ key: 'image_media_id', label: t('gamification.image', 'Image'), type: 'text' },
		{ key: 'activity_key', label: t('gamification.activity', 'Activity Key'), type: 'text', required: true, help: t('gamification.activity_key_help', 'Avoid reversible keys (reaction.*, *.voted) — vote/unvote cycles can inflate progress') },
		{ key: 'target_count', label: t('gamification.target', 'Target Count'), type: 'number', required: true },
		{ key: 'reward_points', label: t('gamification.reward', 'Reward Points'), type: 'number' },
		{ key: 'reward_badge_id', label: t('gamification.reward_badge', 'Reward Badge'), type: 'select', options: badgeOptions },
		{ key: 'starts_at', label: t('gamification.starts', 'Start Date'), type: 'date', required: true },
		{ key: 'ends_at', label: t('gamification.ends', 'End Date'), type: 'date' },
		{ key: 'feature_flag', label: t('gamification.feature_flag', 'Feature Flag'), type: 'select', options: featureFlagSelectOptions() },
		{ key: 'is_active', label: t('gamification.active', 'Active'), type: 'checkbox', defaultValue: true }
	]);

	const challengeFilters = $derived<FilterDef[]>([
		{
			key: 'is_active',
			label: t('gamification.active', 'Active'),
			options: [
				{ value: 'true', label: t('common.active', 'Active') },
				{ value: 'false', label: t('common.inactive', 'Inactive') }
			]
		}
	]);

	const challengeActions = $derived<ActionItem[]>([
		{
			label: t('common.view', 'View'),
			icon: Eye,
			onclick: (row: Challenge, ctx) => ctx?.openView(row)
		},
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row: Challenge, ctx) => {
				void primeMediaUrl(row.image_media_id);
				ctx?.openEdit(row);
			},
			show: () => canManageChallenges
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row: Challenge) => promptDelete('challenge', row.id, row.name),
			show: () => canManageChallenges
		}
	]);

	const challengeConfig = $derived<EntityPageConfig>({
		title: t('gamification.challenges', 'Challenges'),
		description: t('gamification.challenges_description', 'Manage time-bound challenges and rewards'),
		createLabel: t('gamification.create_challenge', 'Create Challenge'),
		createPermission: 'gamification.manage_challenges',
		editPermission: 'gamification.manage_challenges',
		deletePermission: 'gamification.manage_challenges',
		pageSize: 25,
		columns: [
			{
				key: 'name',
				label: t('gamification.name', 'Name'),
				sortable: true,
				render: (value: string) => `<span class="font-medium text-foreground">${value}</span>`
			},
			{
				key: 'activity_key',
				label: t('gamification.activity', 'Activity'),
				sortable: true,
				render: (value: string) => `<span class="font-mono text-xs text-muted-foreground">${value}</span>`
			},
			{
				key: 'target_count',
				label: t('gamification.target', 'Target'),
				sortable: true,
				render: (value: number) => `<span class="font-mono text-foreground">${value}</span>`
			},
			{
				key: 'reward_points',
				label: t('gamification.reward', 'Reward Points'),
				sortable: true,
				render: (value: number) => `<span class="font-mono text-foreground">${value}</span>`
			},
			{
				key: 'is_active',
				label: t('gamification.active', 'Active'),
				sortable: true,
				render: (value: boolean) => value
					? `<span class="inline-flex items-center rounded-md border border-success bg-success/15 px-2 py-0.5 text-xs font-medium text-success">${t('common.active', 'Active')}</span>`
					: `<span class="inline-flex items-center rounded-md border border-border bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">${t('common.inactive', 'Inactive')}</span>`
			},
			{
				key: 'ends_at',
				label: t('gamification.ends', 'Ends'),
				sortable: true,
				render: (value: string | null) => `<span class="text-muted-foreground text-xs">${value ? new Date(value).toLocaleDateString(undefined, { timeZone: 'UTC' }) : '—'}</span>`
			}
		],
		filters: challengeFilters,
		actions: challengeActions,
		fields: challengeFields,
		load: async ({ page, pageSize, q, filter, sort }) => {
			let data = await challengesList().catch(() => {
				throw new Error(t('gamification.load_error', 'Failed to load gamification data'));
			});
			// Client-side search
			if (q) {
				const query = q.toLowerCase();
				data = data.filter((c) =>
					c.name.toLowerCase().includes(query) ||
					c.slug.toLowerCase().includes(query) ||
					c.activity_key.toLowerCase().includes(query)
				);
			}
			// Client-side filters
			if (filter.is_active) {
				const active = filter.is_active === 'true';
				data = data.filter((c) => c.is_active === active);
			}
			// Client-side sort — honours the column the admin clicked.
			applySort(data, sort, {
				name: (c) => c.name,
				activity_key: (c) => c.activity_key,
				target_count: (c) => c.target_count,
				reward_points: (c) => c.reward_points,
				is_active: (c) => (c.is_active ? 1 : 0),
				ends_at: (c) => c.ends_at
			}, (a, b) => a.name.localeCompare(b.name));
			const total = data.length;
			const totalPages = Math.max(1, Math.ceil(total / pageSize));
			const start = page * pageSize;
			return { data: data.slice(start, start + pageSize), total, totalPages };
		},
		create: async (body) => {
			const err = validateChallengeBody(body);
			if (err) return { ok: false, error: err };
			const res = await api('/admin/gamification/challenges', {
				method: 'POST',
				body: {
					name: body.name.trim(),
					slug: body.slug.trim(),
					description: body.description ?? '',
					image_media_id: body.image_media_id || undefined,
					activity_key: body.activity_key.trim(),
					target_count: intOr(body.target_count),
					reward_points: intOr(body.reward_points) ?? 0,
					reward_badge_id: body.reward_badge_id || undefined,
					starts_at: toRFC3339(body.starts_at),
					ends_at: toRFC3339(body.ends_at, true),
					feature_flag: body.feature_flag?.trim() || undefined,
					is_active: typeof body.is_active === 'boolean' ? body.is_active : undefined
				}
			});
			if (res.ok) invalidateLists();
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body, row) => {
			if (!body.name?.trim()) return { ok: false, error: t('gamification.err_name_required', 'Name is required') };
			const target = intOr(body.target_count);
			if (target !== undefined && target <= 0)
				return { ok: false, error: t('gamification.err_target', 'Target count must be a positive number') };
			const startsAt = toRFC3339(body.starts_at);
			const endsAt = toRFC3339(body.ends_at, true);
			if (startsAt && endsAt && endsAt < startsAt)
				return { ok: false, error: t('gamification.err_ends_before_starts', 'End date must be after the start date') };
			const payload: Record<string, unknown> = {
				name: body.name.trim(),
				description: body.description ?? '',
				activity_key: body.activity_key?.trim() || undefined,
				target_count: target,
				reward_points: intOr(body.reward_points),
				reward_badge_id: body.reward_badge_id || undefined,
				starts_at: startsAt,
				ends_at: endsAt,
				feature_flag: body.feature_flag?.trim() || undefined,
				is_active: typeof body.is_active === 'boolean' ? body.is_active : undefined
			};
			if (body.image_media_id) payload.image_media_id = body.image_media_id;
			else if (row?.image_media_id) payload.clear_image_media_id = true;
			if (!payload.reward_badge_id && row?.reward_badge_id) payload.clear_reward_badge_id = true;
			if (!payload.ends_at && row?.ends_at) payload.clear_ends_at = true;
			if (!payload.feature_flag && row?.feature_flag) payload.clear_feature_flag = true;
			const res = await api(`/admin/gamification/challenges/${id}`, { method: 'PATCH', body: payload });
			if (res.ok) invalidateLists();
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/gamification/challenges/${id}`, { method: 'DELETE' });
			if (res.ok) invalidateLists();
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: ChallengeView,
		formSnippet: ChallengeForm
	});

	// ── Sub-flow handlers ──────────────────────────────────────────────────

	function openAwardBadge(badge: Badge) {
		awardBadgeId = badge.id;
		awardBadgeName = badge.name;
		awardUsername = '';
		awardTargetUserId = '';
		awardSelectedUsername = '';
		awardError = '';
		awardModalOpen = true;
	}

	async function awardBadge() {
		awardError = '';
		// The text must still match the selected suggestion — typing a new
		// name after picking one must not award the stale id.
		if (!awardTargetUserId || awardUsername !== awardSelectedUsername) {
			awardError = t('gamification.select_user', 'Please select a user from the search results.');
			return;
		}
		acting = true;
		try {
			const result = await api(`/admin/gamification/badges/${awardBadgeId}/award`, { method: 'POST', body: { user_id: awardTargetUserId } });
			if (result.ok) {
				awardModalOpen = false;
				awardUsername = '';
				awardTargetUserId = '';
				awardSelectedUsername = '';
				flashSuccess(t('gamification.award_success', 'Badge awarded'));
			} else {
				awardError = result.error ?? t('gamification.award_error', 'Failed to award badge');
			}
		} catch (e) {
			awardError = e instanceof Error ? e.message : t('gamification.award_error', 'Failed to award badge');
		} finally {
			acting = false;
		}
	}

	function promptDelete(kind: 'badge' | 'challenge', id: string, name: string) {
		deleteTarget = { kind, id, name };
		deleteError = '';
		showDeleteConfirm = true;
	}

	async function confirmDelete() {
		if (!deleteTarget) return;
		acting = true;
		deleteError = '';
		const kind = deleteTarget.kind;
		try {
			const endpoint = kind === 'badge'
				? `/admin/gamification/badges/${deleteTarget.id}`
				: `/admin/gamification/challenges/${deleteTarget.id}`;
			const result = await api(endpoint, { method: 'DELETE' });
			if (result.ok) {
				showDeleteConfirm = false;
				deleteTarget = null;
				invalidateLists();
				flashSuccess(t('gamification.delete_success', 'Deleted'));
				// Reload the appropriate tab's EntityPage
				if (kind === 'badge') {
					badgeController?.reload();
				} else {
					challengeController?.reload();
				}
			} else {
				deleteError = result.error ?? t('common.error', 'Failed to delete');
			}
		} catch (e) {
			deleteError = e instanceof Error ? e.message : t('common.error', 'Failed to delete');
		} finally {
			acting = false;
		}
	}
	// ── Seasons / Tiers / Award Events (gamification v2) ────────────────────

	const toLocalDT = (v: string | null | undefined) => {
		if (!v) return '';
		const d = new Date(v);
		return isNaN(d.getTime()) ? '' : d.toISOString().slice(0, 16);
	};
	// toISO normalises a datetime-local ('YYYY-MM-DDTHH:MM'), date
	// ('YYYY-MM-DD'), or RFC 3339 value into the strict RFC 3339 timestamp
	// the backend requires (§64 parseTimestamp uses time.RFC3339). Invalid
	// input returns undefined instead of throwing RangeError.
	const toISO = (v: unknown) => {
		const s = String(v ?? '').trim();
		if (!s) return undefined;
		const d = new Date(s);
		return isNaN(d.getTime()) ? undefined : d.toISOString();
	};

	const seasonFields = $derived<FieldDef[]>([
		{ key: 'slug', label: t('gamification.slug', 'Slug'), type: 'text', required: true, readonlyInEdit: true },
		{ key: 'name', label: t('gamification.name', 'Name'), type: 'text', required: true },
		{ key: 'description', label: t('gamification.description', 'Description'), type: 'textarea' },
		{ key: 'starts_at', label: t('gamification.starts', 'Starts'), type: 'datetime-local', required: true },
		{ key: 'ends_at', label: t('gamification.ends', 'Ends'), type: 'datetime-local' },
		{ key: 'is_active', label: t('gamification.active', 'Active'), type: 'checkbox', defaultValue: true }
	]);

	const seasonConfig = $derived<EntityPageConfig>({
		title: t('gamification.seasons', 'Seasons'),
		description: t('gamification.seasons_description', 'Limited-edition badge collection windows'),
		createLabel: t('gamification.create_season', 'Create Season'),
		createPermission: 'gamification.manage_badges',
		editPermission: 'gamification.manage_badges',
		deletePermission: 'gamification.manage_badges',
		fields: seasonFields,
		columns: [
			{ key: 'name', label: t('gamification.name', 'Name'), sortable: true, render: (v: string) => `<span class="font-medium text-foreground">${v}</span>` },
			{ key: 'slug', label: t('gamification.slug', 'Slug'), sortable: true, render: (v: string) => `<span class="font-mono text-xs text-muted-foreground">${v}</span>` },
			{ key: 'starts_at', label: t('gamification.starts', 'Starts'), sortable: true, render: (v: string) => `<span class="text-xs text-muted-foreground">${new Date(v).toLocaleDateString(undefined, { timeZone: 'UTC' })}</span>` },
			{ key: 'ends_at', label: t('gamification.ends', 'Ends'), sortable: true, render: (v: string | null) => `<span class="text-xs text-muted-foreground">${v ? new Date(v).toLocaleDateString(undefined, { timeZone: 'UTC' }) : '—'}</span>` },
			{
				key: 'is_active', label: t('gamification.active', 'Active'), sortable: true,
				render: (v: boolean) => v
					? `<span class="inline-flex items-center rounded-md border border-success bg-success/15 px-2 py-0.5 text-xs font-medium text-success">${t('common.active', 'Active')}</span>`
					: `<span class="inline-flex items-center rounded-md border border-border bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">${t('common.inactive', 'Inactive')}</span>`
			}
		],
		load: async ({ page, pageSize, q, sort }) => {
			const res = await api<any[]>('/admin/gamification/seasons');
			if (!res.ok) throw new Error(res.error || t('gamification.load_error', 'Failed to load'));
			let rows = (res.data ?? []).map((r) => ({ ...r, starts_at: toLocalDT(r.starts_at), ends_at: toLocalDT(r.ends_at) }));
			if (q) {
				const query = q.toLowerCase();
				rows = rows.filter((r) => r.name?.toLowerCase().includes(query) || r.slug?.toLowerCase().includes(query));
			}
			applySort(rows, sort, {
				name: (r) => r.name,
				slug: (r) => r.slug,
				starts_at: (r) => r.starts_at,
				ends_at: (r) => r.ends_at,
				is_active: (r) => (r.is_active ? 1 : 0)
			}, (a, b) => (a.starts_at < b.starts_at ? 1 : -1));
			const total = rows.length;
			return {
				data: rows.slice(page * pageSize, page * pageSize + pageSize),
				total,
				totalPages: Math.max(1, Math.ceil(total / pageSize))
			};
		},
		create: async (body) => {
			const res = await api('/admin/gamification/seasons', {
				method: 'POST',
				body: { slug: body.slug.trim(), name: body.name.trim(), description: body.description || undefined, starts_at: toISO(body.starts_at), ends_at: toISO(body.ends_at), is_active: !!body.is_active }
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await api(`/admin/gamification/seasons/${id}`, {
				method: 'PATCH',
				body: { name: body.name?.trim() || undefined, description: body.description ?? '', starts_at: toISO(body.starts_at), ends_at: toISO(body.ends_at), is_active: typeof body.is_active === 'boolean' ? body.is_active : undefined }
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/gamification/seasons/${id}`, { method: 'DELETE' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		}
	});

	const tierFields = $derived<FieldDef[]>([
		{ key: 'badge_group', label: t('gamification.badge_group', 'Badge Group'), type: 'text', required: true },
		{ key: 'tier', label: t('gamification.tier', 'Tier'), type: 'select', required: true, options: tierOptions },
		{ key: 'name', label: t('gamification.name', 'Name'), type: 'text', required: true },
		{ key: 'threshold', label: t('gamification.threshold', 'Threshold'), type: 'number', required: true },
		{ key: 'badge_id', label: t('gamification.badge', 'Badge'), type: 'select', options: badgeOptions }
	]);

	const tierConfig = $derived<EntityPageConfig>({
		title: t('gamification.tiers', 'Tiers'),
		description: t('gamification.tiers_description', 'Progression metadata for tiered badge collections'),
		createLabel: t('gamification.create_tier', 'Create Tier'),
		createPermission: 'gamification.manage_badges',
		editPermission: 'gamification.manage_badges',
		deletePermission: 'gamification.manage_badges',
		fields: tierFields,
		columns: [
			{ key: 'badge_group', label: t('gamification.badge_group', 'Group'), sortable: true, render: (v: string) => `<span class="font-mono text-xs text-muted-foreground">${v}</span>` },
			{ key: 'tier', label: t('gamification.tier', 'Tier'), sortable: true, render: (v: string) => `<span class="capitalize text-foreground">${v}</span>` },
			{ key: 'name', label: t('gamification.name', 'Name'), sortable: true, render: (v: string) => `<span class="font-medium text-foreground">${v}</span>` },
			{ key: 'threshold', label: t('gamification.threshold', 'Threshold'), sortable: true, render: (v: number) => `<span class="font-mono text-foreground">${v}</span>` }
		],
		load: async ({ page, pageSize, q, sort }) => {
			const res = await api<any[]>('/admin/gamification/tiers');
			if (!res.ok) throw new Error(res.error || t('gamification.load_error', 'Failed to load'));
			let rows = res.data ?? [];
			if (q) {
				const query = q.toLowerCase();
				rows = rows.filter((r: any) => r.name?.toLowerCase().includes(query) || r.badge_group?.toLowerCase().includes(query));
			}
			applySort(rows, sort, {
				badge_group: (r: any) => r.badge_group,
				tier: (r: any) => r.tier,
				name: (r: any) => r.name,
				threshold: (r: any) => r.threshold
			}, (a: any, b: any) => String(a.badge_group).localeCompare(String(b.badge_group)) || a.threshold - b.threshold);
			const total = rows.length;
			return {
				data: rows.slice(page * pageSize, page * pageSize + pageSize),
				total,
				totalPages: Math.max(1, Math.ceil(total / pageSize))
			};
		},
		create: async (body) => {
			const res = await api('/admin/gamification/tiers', {
				method: 'POST',
				body: { badge_group: body.badge_group.trim(), tier: body.tier, name: body.name.trim(), threshold: intOr(body.threshold) ?? 0, badge_id: body.badge_id?.trim() || undefined }
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body, row) => {
			const payload: Record<string, unknown> = { badge_group: body.badge_group?.trim() || undefined, tier: body.tier || undefined, name: body.name?.trim() || undefined, threshold: intOr(body.threshold), badge_id: body.badge_id?.trim() || undefined };
			if (!payload.badge_id && row?.badge_id) payload.clear_badge_id = true;
			const res = await api(`/admin/gamification/tiers/${id}`, { method: 'PATCH', body: payload });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/gamification/tiers/${id}`, { method: 'DELETE' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		}
	});

	// Award events — read-only audit trail, server-paginated. q, sort and the
	// badge_id filter are forwarded to the backend (§64 award-events route).
	const awardEventFilters = $derived<FilterDef[]>([
		{
			key: 'badge_id',
			label: t('gamification.badge', 'Badge'),
			options: badgeOptions
		}
	]);

	const awardEventConfig = $derived<EntityPageConfig>({
		title: t('gamification.awards', 'Award Events'),
		description: t('gamification.awards_description', 'Audit trail of every badge award'),
		readonly: true,
		pageSize: 25,
		fields: [],
		filters: awardEventFilters,
		columns: [
			{ key: 'username', label: t('gamification.username', 'Username'), sortable: true, render: (v: string) => `<span class="font-medium text-foreground">${v || '—'}</span>` },
			{ key: 'badge_name', label: t('gamification.badge', 'Badge'), sortable: true, render: (v: string) => `<span class="text-foreground">${v}</span>` },
			{ key: 'source', label: t('gamification.source', 'Source'), sortable: true, render: (v: string) => `<span class="font-mono text-xs text-muted-foreground">${v}</span>` },
			{ key: 'awarded_by_username', label: t('gamification.awarded_by', 'Awarded By'), render: (v: string) => `<span class="text-xs text-muted-foreground">${v || '—'}</span>` },
			{ key: 'created_at', label: t('gamification.awarded_at', 'Awarded'), sortable: true, render: (v: string) => `<span class="text-xs text-muted-foreground">${new Date(v).toLocaleString()}</span>` }
		],
		load: async ({ page, pageSize, q, sort, filter }) => {
			const params = new URLSearchParams();
			params.set('page_size', String(pageSize));
			params.set('page_offset', String(page * pageSize));
			if (q) params.set('q', q);
			if (filter.badge_id) params.set('badge_id', filter.badge_id);
			if (sort) {
				const [sortBy, sortDir] = sort.split(':');
				params.set('sort_by', sortBy);
				params.set('sort_dir', sortDir === 'asc' ? 'asc' : 'desc');
			}
			const res = await apiRaw<{ data?: any[]; pagination?: { total?: number } }>(`/admin/gamification/award-events?${params}`);
			if (!res.ok) throw new Error(res.error || t('gamification.load_error', 'Failed to load'));
			const rows = res.data?.data ?? [];
			const total = res.data?.pagination?.total ?? rows.length;
			return { data: rows, total, totalPages: Math.max(1, Math.ceil(total / pageSize)) };
		}
	});
</script>

<svelte:head>
	<title>{t('gamification.title', 'Gamification')} — {siteName()} Admin</title>
</svelte:head>

{#if pageSuccess}
	<div
		class="mb-4 rounded-lg border p-3 text-sm"
		style="border-color: var(--color-success); background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)"
	>
		{pageSuccess}
	</div>
{/if}

<Tabs {tabs} bind:value={activeTab} tabParam="tab" tabPreserve={[]} />

{#if activeTab === 'badges'}
	<EntityPage config={badgeConfig} bind:controller={badgeController} />
{:else if activeTab === 'challenges'}
	<EntityPage config={challengeConfig} bind:controller={challengeController} />
{:else if activeTab === 'seasons'}
	<EntityPage config={seasonConfig} />
{:else if activeTab === 'tiers'}
	<EntityPage config={tierConfig} />
{:else}
	<EntityPage config={awardEventConfig} />
{/if}

<!-- Award Badge Modal -->
<Modal bind:open={awardModalOpen} title={t('gamification.award_title', 'Award Badge')} size="sm">
	<div>
		<p class="mb-3 text-sm" style="color: var(--color-foreground)">
			{t('gamification.award_badge_to', 'Award')} <strong>{awardBadgeName}</strong>
		</p>
		<label for="award-username" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.username', 'Username')}</label>
		<UserAutocomplete
			id="award-username"
			bind:value={awardUsername}
			placeholder={t('gamification.search_user', 'Search by username...')}
			onSelectUser={(u) => {
				awardTargetUserId = u.id;
				awardSelectedUsername = u.username;
			}}
		/>
		{#if awardTargetUserId && awardUsername === awardSelectedUsername}
			<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('gamification.selected_user', 'Selected')}: {awardUsername} ({awardTargetUserId.slice(0, 8)}...)</p>
		{/if}
		{#if awardError}
			<p class="mt-2 text-sm" style="color: var(--color-danger)">{awardError}</p>
		{/if}
	</div>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (awardModalOpen = false)} disabled={acting}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={awardBadge} disabled={acting || !awardTargetUserId}>
			{acting ? t('common.saving', 'Saving...') : t('gamification.award_badge', 'Award Badge')}
		</Button>
	</div>
</Modal>

<!-- Delete confirmation (per jc-admin-develop: destructive actions behind Modal) -->
<Modal bind:open={showDeleteConfirm} title={t('common.confirmDelete', 'Confirm Delete')} size="sm">
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('gamification.delete_confirm', 'Delete')}
			<strong>{deleteTarget.name}</strong>?
			{t('common.cannotUndo', 'This action cannot be undone.')}
		</p>
	{/if}
	{#if deleteError}
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{deleteError}</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showDeleteConfirm = false)} disabled={acting}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={confirmDelete} disabled={acting}>
			<Trash2 class="me-2 h-4 w-4" />
			{acting ? t('common.deleting', 'Deleting...') : t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>

<!-- ── View snippets ─────────────────────────────────────────────────────── -->

{#snippet BadgeView({ row }: { row: Badge })}
	<div class="space-y-4">
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.name', 'Name')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.slug', 'Slug')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
			</div>
			<div class="col-span-2">
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.description', 'Description')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.trigger', 'Trigger Type')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.trigger_type}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.trigger_value', 'Trigger Value')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.trigger_value}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.activity_key', 'Activity Key')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.trigger_activity_key || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.category', 'Category')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.category || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.rarity', 'Rarity')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.rarity || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.tier', 'Tier')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.tier || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.badge_group', 'Badge Group')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.badge_group || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.points_reward', 'Points Reward')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.points_reward ?? '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.display_order', 'Display Order')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.display_order}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.flags', 'Flags')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{[row.is_secret && t('gamification.secret', 'Secret'), row.is_limited && t('gamification.limited', 'Limited'), row.is_featured && t('gamification.featured', 'Featured')].filter(Boolean).join(', ') || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.available_from', 'Available From')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.available_from ? new Date(row.available_from).toLocaleDateString(undefined, { timeZone: 'UTC' }) : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.available_until', 'Available Until')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.available_until ? new Date(row.available_until).toLocaleDateString(undefined, { timeZone: 'UTC' }) : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.feature_flag', 'Feature Flag')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.feature_flag || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.system', 'System')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.is_system ? t('common.system', 'System') : t('common.custom', 'Custom')}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.active', 'Active')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.is_active ? t('common.active', 'Active') : t('common.inactive', 'Inactive')}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.created', 'Created')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleString()}</p>
			</div>
		</div>
	</div>
{/snippet}

{#snippet ChallengeView({ row }: { row: Challenge })}
	<div class="space-y-4">
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.name', 'Name')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.slug', 'Slug')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
			</div>
			<div class="col-span-2">
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.description', 'Description')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.activity', 'Activity Key')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.activity_key}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.target', 'Target Count')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.target_count}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.reward', 'Reward Points')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.reward_points}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.reward_badge', 'Reward Badge')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.reward_badge_id ? (badgeOptions.find((o) => o.value === row.reward_badge_id)?.label ?? row.reward_badge_id) : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.starts', 'Start Date')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.starts_at ? new Date(row.starts_at).toLocaleDateString(undefined, { timeZone: 'UTC' }) : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.ends', 'End Date')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.ends_at ? new Date(row.ends_at).toLocaleDateString(undefined, { timeZone: 'UTC' }) : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.feature_flag', 'Feature Flag')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.feature_flag || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.active', 'Active')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.is_active ? t('common.active', 'Active') : t('common.inactive', 'Inactive')}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('gamification.created', 'Created')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleString()}</p>
			</div>
		</div>
	</div>
{/snippet}

<!-- ── Form snippets (rich forms with ActivityKeyAutocomplete) ───────────── -->

{#snippet BadgeForm({ value, editing }: { value: Record<string, any>; editing: any | null })}
	<div class="space-y-3">
		<div>
			<label for="badge-name" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.name', 'Name')}</label>
			<input id="badge-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.name} placeholder={t('app.gamification.ph_top_contributor', 'Top Contributor')} />
		</div>
		<div>
			<label for="badge-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.slug', 'Slug')}</label>
			<input id="badge-slug" type="text" required class="w-full rounded-md border px-3 py-2 text-sm disabled:opacity-60" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.slug} disabled={editing !== null} placeholder="top-contributor" />
			{#if editing}
				<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('gamification.slug_locked', 'Slug cannot be changed after creation')}</p>
			{/if}
		</div>
		<div>
			<label for="badge-desc" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.description', 'Description')}</label>
			<textarea id="badge-desc" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" rows="2" bind:value={value.description}></textarea>
		</div>
		<div>
			<span class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.icon', 'Icon')}</span>
			<div class="flex items-center gap-2">
				{#if value.icon_media_id}
					{#if mediaUrlById[value.icon_media_id]}
						<img src={mediaUrlById[value.icon_media_id]} alt="" class="h-8 w-8 rounded object-cover" />
					{/if}
					<span class="font-mono text-xs" style="color: var(--color-muted)">{value.icon_media_id.slice(0, 8)}…</span>
					<Button variant="ghost" size="sm" onclick={() => (value.icon_media_id = '')}>{t('gamification.remove', 'Remove')}</Button>
				{:else}
					<Button variant="outline" size="sm" onclick={() => {
						pendingMediaSelect = (m) => {
							value.icon_media_id = m.id;
							mediaUrlById = { ...mediaUrlById, [m.id]: m.url };
						};
						badgeIconPickerOpen = true;
					}}>{t('gamification.choose_icon', 'Choose icon')}</Button>
				{/if}
			</div>
		</div>
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
			<div>
				<label for="badge-trigger" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.trigger', 'Trigger Type')}</label>
				<AutocompleteSelect
					id="badge-trigger"
					bind:value={value.trigger_type}
					options={triggerTypeOptions}
					class="w-full"
				/>
			</div>
			<div>
				<label for="badge-trigger-value" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.trigger_value', 'Trigger Value')}</label>
				<input id="badge-trigger-value" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.trigger_value} />
			</div>
		</div>
		<div>
			<label for="badge-activity" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.activity_key', 'Activity Key')}</label>
			<ActivityKeyAutocomplete id="badge-activity" bind:value={value.trigger_activity_key} placeholder="post.create" extraKeys={knownActivityKeys} />
		</div>
		<div>
			<label for="badge-feature-flag" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.feature_flag', 'Feature Flag')}</label>
			<AutocompleteSelect
				id="badge-feature-flag"
				bind:value={value.feature_flag}
				options={featureFlagSelectOptions(value.feature_flag)}
				class="w-full"
			/>
			<p class="mt-0.5 text-xs" style="color: var(--color-muted)">{t('gamification.feature_flag_hint', 'Optional. Item is hidden from public gamification surfaces and does not progress while this feature flag is disabled.')}</p>
		</div>
		<div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
			<div>
				<label for="badge-category" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.category', 'Category')}</label>
				<AutocompleteSelect id="badge-category" bind:value={value.category} options={badgeCategories} class="w-full" />
			</div>
			<div>
				<label for="badge-rarity" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.rarity', 'Rarity')}</label>
				<AutocompleteSelect id="badge-rarity" bind:value={value.rarity} options={rarityOptions} class="w-full" />
			</div>
			<div>
				<label for="badge-tier" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.tier', 'Tier')}</label>
				<AutocompleteSelect id="badge-tier" bind:value={value.tier} options={tierOptions} class="w-full" />
			</div>
		</div>
		<div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
			<div>
				<label for="badge-group" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.badge_group', 'Badge Group')}</label>
				<input id="badge-group" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.badge_group} />
			</div>
			<div>
				<label for="badge-points" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.points_reward', 'Points Reward')}</label>
				<input id="badge-points" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.points_reward} />
			</div>
			<div>
				<label for="badge-accent" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.accent_color', 'Accent Color')}</label>
				<div class="flex items-center gap-2">
					<input id="badge-accent" type="color" class="h-9 w-14 cursor-pointer rounded-md border p-1" style="border-color: var(--color-border); background: var(--color-surface)"
						value={value.accent_color || '#6b7280'}
						oninput={(e) => (value.accent_color = e.currentTarget.value)} />
					{#if value.accent_color}
						<span class="font-mono text-xs" style="color: var(--color-muted)">{value.accent_color}</span>
						<Button variant="ghost" size="sm" onclick={() => (value.accent_color = '')}>{t('gamification.remove', 'Remove')}</Button>
					{:else}
						<span class="text-xs" style="color: var(--color-muted)">{t('gamification.accent_inherit', 'Inherits rarity color')}</span>
					{/if}
				</div>
			</div>
		</div>
		<div class="flex flex-wrap items-center gap-4">
			<div class="flex items-center gap-2">
				<input type="checkbox" id="badge-secret" bind:checked={value.is_secret} />
				<label for="badge-secret" class="text-sm" style="color: var(--color-foreground)">{t('gamification.secret', 'Secret')}</label>
			</div>
			<div class="flex items-center gap-2">
				<input type="checkbox" id="badge-limited" bind:checked={value.is_limited} />
				<label for="badge-limited" class="text-sm" style="color: var(--color-foreground)">{t('gamification.limited', 'Limited')}</label>
			</div>
			<div class="flex items-center gap-2">
				<input type="checkbox" id="badge-featured" bind:checked={value.is_featured} />
				<label for="badge-featured" class="text-sm" style="color: var(--color-foreground)">{t('gamification.featured', 'Featured')}</label>
			</div>
		</div>
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
			<div>
				<label for="badge-available-from" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.available_from', 'Available From')}</label>
				<input id="badge-available-from" type="datetime-local" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" value={toLocalDT(value.available_from)} oninput={(e) => (value.available_from = e.currentTarget.value)} />
			</div>
			<div>
				<label for="badge-available-until" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.available_until', 'Available Until')}</label>
				<input id="badge-available-until" type="datetime-local" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" value={toLocalDT(value.available_until)} oninput={(e) => (value.available_until = e.currentTarget.value)} />
			</div>
		</div>
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
			<div>
				<label for="badge-order" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.display_order', 'Display Order')}</label>
				<input id="badge-order" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.display_order} />
			</div>
			<div class="flex items-center gap-2 pt-6">
				<input type="checkbox" id="badge-active" bind:checked={value.is_active} />
				<label for="badge-active" class="text-sm" style="color: var(--color-foreground)">{t('gamification.active', 'Active')}</label>
			</div>
		</div>
	</div>
{/snippet}

{#snippet ChallengeForm({ value, editing }: { value: Record<string, any>; editing: any | null })}
	<div class="space-y-3">
		<div>
			<label for="challenge-name" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.name', 'Name')}</label>
			<input id="challenge-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.name} placeholder={t('app.gamification.ph_30day_posting_streak', '30-Day Posting Streak')} />
		</div>
		<div>
			<label for="challenge-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.slug', 'Slug')}</label>
			<input id="challenge-slug" type="text" required class="w-full rounded-md border px-3 py-2 text-sm disabled:opacity-60" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.slug} disabled={editing !== null} placeholder="30-day-streak" />
			{#if editing}
				<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('gamification.slug_locked', 'Slug cannot be changed after creation')}</p>
			{/if}
		</div>
		<div>
			<label for="challenge-desc" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.description', 'Description')}</label>
			<textarea id="challenge-desc" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" rows="2" bind:value={value.description}></textarea>
		</div>
		<div>
			<span class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.image', 'Image')}</span>
			<div class="flex items-center gap-2">
				{#if value.image_media_id}
					{#if mediaUrlById[value.image_media_id]}
						<img src={mediaUrlById[value.image_media_id]} alt="" class="h-8 w-8 rounded object-cover" />
					{/if}
					<span class="font-mono text-xs" style="color: var(--color-muted)">{value.image_media_id.slice(0, 8)}…</span>
					<Button variant="ghost" size="sm" onclick={() => (value.image_media_id = '')}>{t('gamification.remove', 'Remove')}</Button>
				{:else}
					<Button variant="outline" size="sm" onclick={() => {
						pendingMediaSelect = (m) => {
							value.image_media_id = m.id;
							mediaUrlById = { ...mediaUrlById, [m.id]: m.url };
						};
						challengeImagePickerOpen = true;
					}}>{t('gamification.choose_image', 'Choose image')}</Button>
				{/if}
			</div>
		</div>
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
			<div>
				<label for="challenge-activity" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.activity', 'Activity Key')}</label>
				<ActivityKeyAutocomplete id="challenge-activity" bind:value={value.activity_key} placeholder="post.create" extraKeys={knownActivityKeys} />
			</div>
			<div>
				<label for="challenge-target" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.target', 'Target Count')}</label>
				<input id="challenge-target" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.target_count} />
			</div>
		</div>
		<div>
			<label for="challenge-feature-flag" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.feature_flag', 'Feature Flag')}</label>
			<AutocompleteSelect
				id="challenge-feature-flag"
				bind:value={value.feature_flag}
				options={featureFlagSelectOptions(value.feature_flag)}
				class="w-full"
			/>
			<p class="mt-0.5 text-xs" style="color: var(--color-muted)">{t('gamification.feature_flag_hint', 'Optional. Item is hidden from public gamification surfaces and does not progress while this feature flag is disabled.')}</p>
		</div>
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
			<div>
				<label for="challenge-reward" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.reward', 'Reward Points')}</label>
				<input id="challenge-reward" type="number" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={value.reward_points} />
			</div>
			<div>
				<label for="challenge-reward-badge" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.reward_badge', 'Reward Badge')}</label>
				<AutocompleteSelect
					id="challenge-reward-badge"
					bind:value={value.reward_badge_id}
					options={badgeOptions}
					class="w-full"
				/>
			</div>
			<div>
				<label for="challenge-starts" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.starts', 'Start Date')}</label>
				<input id="challenge-starts" type="date" required class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" value={toDateInput(value.starts_at)} oninput={(e) => (value.starts_at = e.currentTarget.value)} />
			</div>
		</div>
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
			<div>
				<label for="challenge-ends" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('gamification.ends', 'End Date')}</label>
				<input id="challenge-ends" type="date" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" value={toDateInput(value.ends_at)} oninput={(e) => (value.ends_at = e.currentTarget.value)} />
			</div>
			<div class="flex items-center gap-2 pt-6">
				<input type="checkbox" id="challenge-active" bind:checked={value.is_active} />
				<label for="challenge-active" class="text-sm" style="color: var(--color-foreground)">{t('gamification.active', 'Active')}</label>
			</div>
		</div>
	</div>
{/snippet}

<!-- Media pickers for badge icon / challenge image (§64 media columns) -->
<MediaPickerModal
	bind:open={badgeIconPickerOpen}
	onselect={(m) => {
		badgeIconPickerOpen = false;
		// The pickers write into whichever form is open — resolved via the
		// active tab at pick time.
		pendingMediaSelect?.(m);
	}}
/>
<MediaPickerModal
	bind:open={challengeImagePickerOpen}
	onselect={(m) => {
		challengeImagePickerOpen = false;
		pendingMediaSelect?.(m);
	}}
/>
