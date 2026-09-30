<script lang="ts">
	// §80 Wiki Types & Field-Group Pages — admin management for wiki types
	// (space_types carrying a wiki_type_configs row, with parent edges that
	// express the geo chain + child types like university) and each type's
	// category template (slug + category token + optional field-group binding).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Button } from '$lib/components/ui/button';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import {
		wikiAdminApi,
		fieldGroupsAdminApiExtended,
		spaceTypesApi,
		type WikiTypeItem,
		type WikiCategoryItem,
		type WikiPageTemplateItem
	} from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Pencil, Trash2, FolderTree, Layers, X, CheckCircle, XCircle } from 'lucide-svelte';
	import type { ActionItem, BulkAction } from '$lib/components/ui/data-table';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	const canManage = $derived(hasPermission('wiki.manage'));

	// ── Lookup sources (loaded once) ────────────────────────────────────────
	// Space types for the create form's type picker; field groups for the
	// category form's binding select; the loaded types list doubles as the
	// parent-edge checkbox source.
	let spaceTypeOptions = $state<{ value: string; label: string }[]>([]);
	let groupOptions = $state<{ value: string; label: string }[]>([]);
	let knownTypes = $state<WikiTypeItem[]>([]);

	$effect(() => {
		void loadSpaceTypes();
		void loadGroups();
	});

	async function loadSpaceTypes() {
		const res = await spaceTypesApi.list();
		if (res.ok) {
			const rows = (res.data as { id: string; name: string; slug: string }[]) ?? [];
			spaceTypeOptions = rows.map((s) => ({ value: s.id, label: `${s.name} (${s.slug})` }));
		}
	}

	async function loadGroups() {
		const res = await fieldGroupsAdminApiExtended.list('page_size=200');
		if (res.ok) {
			const rows = (res.data?.data as { id: string; name: string; slug: string }[]) ?? [];
			groupOptions = [
				{ value: '', label: t('app.wiki.group_auto', '— Auto-create dedicated group —') },
				...rows.map((g) => ({ value: g.id, label: g.name }))
			];
		}
	}

	// ── Autocomplete adapters for the space-type / field-group pickers ─────
	// Both lists are loaded once at mount and filtered client-side — the
	// dropdown is a search box, so the fetcher just narrows the cached list.
	function filterOptions(opts: { value: string; label: string }[], q: string): AutocompleteItem[] {
		const needle = q.trim().toLowerCase();
		return opts
			.filter((o) => o.value)
			.map((o) => ({ id: o.value, label: o.label }))
			.filter((i) => !needle || i.label.toLowerCase().includes(needle));
	}

	async function spaceTypeFetcher(q: string): Promise<AutocompleteItem[]> {
		return filterOptions(spaceTypeOptions, q);
	}

	async function groupFetcher(q: string): Promise<AutocompleteItem[]> {
		return filterOptions(groupOptions, q);
	}

	function spaceTypeLabel(id: unknown): string | null {
		const sid = String(id ?? '');
		if (!sid) return null;
		return spaceTypeOptions.find((o) => o.value === sid)?.label ?? sid;
	}

	// ── Types table ─────────────────────────────────────────────────────────
	let typesController = $state<EntityPageController | undefined>(undefined);
	let catsController = $state<EntityPageController | undefined>(undefined);
	let tplsController = $state<EntityPageController | undefined>(undefined);
	let selectedType = $state<WikiTypeItem | null>(null);
	// §80 page templates — the category row whose template pages are open.
	let selectedCategory = $state<WikiCategoryItem | null>(null);

	// Map a type row: EntityPage's id-based row actions need `id`.
	function mapType(row: WikiTypeItem) {
		return { ...row, id: row.space_type_id, parents: (row.parent_type_ids ?? []).length };
	}

	// ── Bulk enable/disable ────────────────────────────────────────────────
	// BulkAction.onclick receives only ids — resolve them against a map of
	// every row seen this session (BulkBar's selectedRows covers just the
	// current page).
	const typeRowMap = new Map<string, WikiTypeItem>();
	const typeRowsFor = (ids: Set<string>): WikiTypeItem[] =>
		Array.from(ids)
			.map((id) => typeRowMap.get(id))
			.filter((r): r is WikiTypeItem => !!r);

	let pageError = $state('');
	let bulkActing = $state(false);

	// Sequential per-row PATCHes — no bulk route; per-row failures are
	// collected (same rule as EntityPage's built-in bulk delete) and the
	// scaffold reloads + clears the selection when this resolves.
	async function bulkSetEnabled(ids: Set<string>, enabled: boolean) {
		const rows = typeRowsFor(ids).filter((r) => r.enabled !== enabled);
		if (bulkActing || rows.length === 0) return;
		bulkActing = true;
		pageError = '';
		try {
			let failed = 0;
			let lastErr = '';
			for (const row of rows) {
				const res = await wikiAdminApi.updateType(row.space_type_id, { enabled });
				if (!res.ok) {
					failed++;
					lastErr = res.error ?? '';
				}
			}
			if (failed > 0) {
				pageError =
					t('app.wiki.bulk_partial', '{failed} of {n} items could not be completed')
						.replace('{failed}', String(failed))
						.replace('{n}', String(rows.length)) + (lastErr ? ` — ${lastErr}` : '');
			}
		} finally {
			bulkActing = false;
		}
	}

	const typeFields: FieldDef[] = [
		{ key: 'space_type_id', label: t('app.wiki.type_space_type', 'Space type'), type: 'text', required: true },
		{ key: 'label', label: t('app.wiki.type_label', 'Label'), type: 'text', placeholder: 'University Wikis' },
		{ key: 'icon', label: t('app.wiki.type_icon', 'Icon'), type: 'text' },
		{ key: 'position', label: t('app.wiki.type_position', 'Position'), type: 'number' },
		{ key: 'enabled', label: t('app.wiki.type_enabled', 'Enabled'), type: 'checkbox' },
		{ key: 'seed_stubs', label: t('app.wiki.type_seed_stubs', 'Seed stub pages'), type: 'checkbox' },
		// Seed-only: EntityPage seeds the edit form from fields keys — without
		// this entry parent_type_ids is absent from formValue on edit and the
		// PATCH below would clear every parent edge. The TypeForm snippet owns
		// its rendering (checkbox group), so no input is drawn for it.
		{ key: 'parent_type_ids', label: t('app.wiki.type_parents', 'Parents'), type: 'text' }
	];

	function toggleParent(value: Record<string, unknown>, id: string, checked: boolean) {
		const cur = (value.parent_type_ids as string[]) ?? [];
		value.parent_type_ids = checked ? [...cur, id] : cur.filter((p) => p !== id);
	}

	const typeActions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row, ctx) => ctx?.openView(row) },
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row, ctx) => ctx?.openEdit(row),
			show: () => canManage
		},
		{
			label: t('app.wiki.manage_categories', 'Categories'),
			icon: FolderTree,
			onclick: (row) => {
				selectedType = row as WikiTypeItem;
				selectedCategory = null;
			}
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row, ctx) => ctx?.openDelete(row),
			show: () => canManage
		}
	];

	// Derived so the canManage-gated bulk actions react once permissions load.
	const typeConfig = $derived.by<EntityPageConfig>(() => ({
		title: t('app.wiki.types_title', 'Wiki Types'),
		description: t(
			'app.wiki.types_desc',
			'Space types that carry a wiki. The geo chain (country → state → city) is primary; other types nest under it via parent edges.'
		),
		createLabel: t('app.wiki.type_create', 'New wiki type'),
		createPermission: 'wiki.manage',
		// Nested under the page's tabs — keep table state out of the shared URL.
		urlSync: false,
		pageSize: 25,
		columns: [
			{
				key: 'space_type_slug',
				label: t('app.wiki.type_slug', 'Type'),
				render: (v: string, row: WikiTypeItem) =>
					`<span class="font-mono text-xs">${v}</span><span class="block text-xs" style="color: var(--color-muted)">${row.space_type_name ?? ''}</span>`
			},
			{
				key: 'label',
				label: t('app.wiki.type_label', 'Label'),
				render: (v: string, row: WikiTypeItem) => v || row.space_type_name || '—'
			},
			{ key: 'enabled', label: t('app.wiki.type_enabled', 'Enabled'), componentSnippet: BoolCell },
			{ key: 'seed_stubs', label: t('app.wiki.type_seed_stubs', 'Seed stubs'), componentSnippet: BoolCell },
			{ key: 'position', label: t('app.wiki.type_position', 'Pos'), width: 64 },
			{ key: 'parents', label: t('app.wiki.type_parents', 'Parents') }
		],
		actions: typeActions,
		bulkActions: canManage
			? [
					{
						label: t('app.wiki.bulk_enable', 'Enable Selected'),
						icon: CheckCircle,
						onclick: (ids: Set<string>) => bulkSetEnabled(ids, true),
						disabled: (ids: Set<string>) =>
							bulkActing || !typeRowsFor(ids).some((r) => !r.enabled)
					},
					{
						label: t('app.wiki.bulk_disable', 'Disable Selected'),
						icon: XCircle,
						variant: 'destructive',
						onclick: (ids: Set<string>) => bulkSetEnabled(ids, false),
						disabled: (ids: Set<string>) =>
							bulkActing || !typeRowsFor(ids).some((r) => r.enabled)
					}
				]
			: [],
		fields: typeFields,
		formSnippet: TypeForm,
		load: async ({ page, pageSize, q }) => {
			const res = await wikiAdminApi.listTypes();
			if (!res.ok) throw new Error(res.error ?? t('common.error', 'Failed to load wiki types'));
			let rows = ((res.data as WikiTypeItem[]) ?? []).map(mapType);
			for (const r of rows) typeRowMap.set(r.id, r);
			knownTypes = res.data ?? [];
			const query = (q ?? '').trim().toLowerCase();
			if (query) {
				rows = rows.filter(
					(r) =>
						r.space_type_slug.toLowerCase().includes(query) ||
						(r.label ?? '').toLowerCase().includes(query) ||
						(r.space_type_name ?? '').toLowerCase().includes(query)
				);
			}
			const total = rows.length;
			const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
			const start = page * pageSize;
			return { data: rows.slice(start, start + pageSize), total, totalPages };
		},
		create: async (body) => {
			const res = await wikiAdminApi.createType({
				space_type_id: String(body.space_type_id ?? ''),
				enabled: body.enabled === undefined ? true : !!body.enabled,
				label: String(body.label ?? '').trim() || null,
				icon: String(body.icon ?? '').trim() || null,
				seed_stubs: !!body.seed_stubs,
				position: Number(body.position ?? 0),
				parent_type_ids: (body.parent_type_ids as string[]) ?? []
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await wikiAdminApi.updateType(id, {
				enabled: !!body.enabled,
				label: String(body.label ?? '').trim() || null,
				icon: String(body.icon ?? '').trim() || null,
				seed_stubs: !!body.seed_stubs,
				position: Number(body.position ?? 0),
				// Omit rather than send [] when the field was never seeded —
				// a nil list is a no-op server-side, an empty list clears edges.
				parent_type_ids: body.parent_type_ids as string[] | undefined
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await wikiAdminApi.deleteType(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		}
	}));

	// ── Categories table (per selected type) ────────────────────────────────

	const catFields: FieldDef[] = [
		{
			key: 'slug',
			label: t('app.wiki.cat_slug', 'Page slug'),
			type: 'text',
			required: true,
			readonlyInEdit: true,
			placeholder: 'cost-of-living',
			help: t('app.wiki.cat_slug_help', 'URL segment — /wiki/{entity}/{slug}. Lowercase, hyphenated; immutable once created.')
		},
		{
			key: 'category',
			label: t('app.wiki.cat_token', 'Category token'),
			type: 'text',
			readonlyInEdit: true,
			placeholder: 'cost_of_living',
			help: t('app.wiki.cat_token_help', 'wiki_pages.category token (lowercase, underscored). Blank = derived from the slug.')
		},
		{ key: 'label', label: t('app.wiki.cat_label', 'Label'), type: 'text', required: true },
		{
			key: 'icon',
			label: t('app.wiki.cat_icon', 'Icon'),
			type: 'text',
			placeholder: 'wallet',
			help: t('app.wiki.cat_icon_help', 'Lucide icon name (kebab-case) shown on the category tab and card — e.g. wallet, globe, landmark. Blank falls back to the default category icon.')
		},
		{
			key: 'field_group_id',
			label: t('app.wiki.cat_group', 'Field group'),
			type: 'autocomplete',
			placeholder: t('app.wiki.cat_group_ph', 'Search field groups…'),
			emptyMessage: t('common.no_results', 'No results found'),
			fetcher: groupFetcher,
			initialFetcher: async () => filterOptions(groupOptions, ''),
			// The '' auto-create sentinel isn't a selectable item — clearing the
			// picker means "auto-create a dedicated group" (create) or "leave
			// the binding unchanged" (edit).
			getSelectedLabel: (id: string) => groupOptions.find((o) => o.value === id)?.label ?? id,
			help: t('app.wiki.cat_group_help', 'The bound group is the page’s shared structured schema — a dedicated group is created automatically when none is picked. Only pick an existing group to deliberately share a schema.')
		},
		{ key: 'position', label: t('app.wiki.cat_position', 'Position'), type: 'number' },
		// Seed-only keys: EntityPage seeds the edit form from fields keys and
		// the CategoryForm snippet owns the rollup rendering — same pattern
		// as the types table's parent_type_ids.
		{ key: 'rollup_enabled', label: t('app.wiki.cat_rollup', 'Rollup'), type: 'checkbox' },
		{ key: 'rollup_mode', label: t('app.wiki.rollup_mode', 'Rollup mode'), type: 'text' },
		{ key: 'rollup_metrics', label: t('app.wiki.rollup_metrics', 'Metrics'), type: 'text' },
		{ key: 'rollup_levels', label: t('app.wiki.rollup_levels', 'Levels'), type: 'text' },
		{ key: 'rollup_section_limit', label: t('app.wiki.rollup_section_limit', 'Section limit'), type: 'number' }
	];

	const ROLLUP_METRICS = ['min', 'avg', 'max'] as const;

	// The type's declared child wiki types — the valid `levels` choices for a
	// rollup on this type (empty selection = every declared child type).
	function childTypesOf(type: WikiTypeItem | null): WikiTypeItem[] {
		if (!type) return [];
		return knownTypes.filter((pt) => (pt.parent_type_ids ?? []).includes(type.space_type_id));
	}

	function toggleRollupList(value: Record<string, unknown>, key: string, item: string, checked: boolean) {
		const cur = (value[key] as string[]) ?? [];
		value[key] = checked ? [...cur, item] : cur.filter((x) => x !== item);
	}

	// Assemble the JSONB payload from the flat form keys — disabled clears
	// the config (null), enabled emits {enabled, mode?, metrics[], levels?,
	// section_limit?} matching validateRollupInput's accepted shape.
	function rollupFromForm(body: Record<string, unknown>): Record<string, unknown> | null {
		if (!body.rollup_enabled) return null;
		const directory = String(body.rollup_mode ?? '') === 'directory';
		const cfg: Record<string, unknown> = { enabled: true, metrics: [] };
		if (directory) {
			cfg.mode = 'directory';
		} else {
			cfg.metrics = ((body.rollup_metrics as string[]) ?? []).filter((m) =>
				(ROLLUP_METRICS as readonly string[]).includes(m)
			);
			const lim = Number(body.rollup_section_limit ?? 0);
			if (lim > 0) cfg.section_limit = lim;
		}
		const levels = (body.rollup_levels as string[]) ?? [];
		if (levels.length) cfg.levels = levels;
		return cfg;
	}

	// Flatten a row's stored rollup JSONB into the form's seed keys.
	function flattenRollup(r: WikiCategoryItem) {
		const cfg = (r.rollup ?? null) as {
			enabled?: boolean;
			mode?: string;
			metrics?: string[];
			levels?: string[];
			section_limit?: number;
		} | null;
		return {
			...r,
			icon: r.icon ?? '',
			rollup_enabled: !!cfg?.enabled,
			rollup_mode: cfg?.mode === 'directory' ? 'directory' : 'aggregate',
			rollup_metrics: cfg?.metrics ?? [],
			rollup_levels: cfg?.levels ?? [],
			rollup_section_limit: cfg?.section_limit ?? 0
		};
	}

	const catActions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row, ctx) => ctx?.openView(row) },
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row, ctx) => ctx?.openEdit(row),
			show: () => canManage
		},
		{
			label: t('app.wiki.manage_templates', 'Pages'),
			icon: Layers,
			onclick: (row) => {
				selectedCategory = row as WikiCategoryItem;
			}
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row, ctx) => ctx?.openDelete(row),
			// System categories are undeletable (backend also rejects them).
			show: (row) => canManage && !(row as WikiCategoryItem).is_system
		}
	];

	const catConfig = $derived.by<EntityPageConfig | null>(() => {
		const sel = selectedType;
		if (!sel) return null;
		return {
					title: t('app.wiki.categories_title', 'Categories — {type}').replace(
						'{type}',
						sel.label || sel.space_type_slug
					),
					description: t(
						'app.wiki.categories_desc',
						'Page template for this wiki type. Each category becomes a tab/page on every entity of the type.'
					),
					createLabel: t('app.wiki.cat_create', 'Add category'),
					createPermission: 'wiki.manage',
					pageSize: 25,
					columns: [
						{ key: 'slug', label: t('app.wiki.cat_slug', 'Slug'), render: (v: string) => `<span class="font-mono text-xs">${v}</span>` },
						{ key: 'category', label: t('app.wiki.cat_token', 'Token'), render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>` },
						{ key: 'label', label: t('app.wiki.cat_label', 'Label') },
						{
							key: 'field_group_name',
							label: t('app.wiki.cat_group', 'Field group'),
							render: (v: string, row: WikiCategoryItem) =>
								v
									? `${v}<span class="block font-mono text-xs" style="color: var(--color-muted)">${row.field_group_slug ?? ''}</span>`
									: `<span class="text-xs" style="color: var(--color-muted)">${t('app.wiki.unbound', 'unbound')}</span>`
						},
						{
							key: 'rollup',
							label: t('app.wiki.cat_rollup_col', 'Rollup'),
							render: (v: unknown) => {
								const cfg = v as { enabled?: boolean; mode?: string } | null;
								if (!cfg?.enabled) {
									return `<span class="text-xs" style="color: var(--color-muted)">—</span>`;
								}
								const label = cfg.mode === 'directory'
									? t('app.wiki.rollup_directory', 'directory')
									: t('app.wiki.rollup_on', 'on');
								return `<span class="text-xs font-medium" style="color: var(--color-primary)">${label}</span>`;
							}
						},
						{ key: 'position', label: t('app.wiki.cat_position', 'Pos'), width: 64 },
						{ key: 'is_system', label: t('app.wiki.cat_system', 'System'), componentSnippet: BoolCell }
					],
					actions: catActions,
					fields: catFields,
					formSnippet: CategoryForm,
					// Nested under the types table — keep its list state out of the
					// URL so the two EntityPages don't fight over ?q/?page.
					urlSync: false,
					load: async ({ page, pageSize, q }) => {
						const res = await wikiAdminApi.listTypeCategories(sel.space_type_id);
						if (!res.ok) throw new Error(res.error ?? t('common.error', 'Failed to load categories'));
						// Flatten the rollup JSONB into the form's seed keys —
						// CategoryForm renders it as structured controls.
						let rows = ((res.data as WikiCategoryItem[]) ?? []).map(flattenRollup);
						const query = (q ?? '').trim().toLowerCase();
						if (query)
							rows = rows.filter(
								(r) =>
									r.slug.toLowerCase().includes(query) ||
									r.label.toLowerCase().includes(query)
							);
						const total = rows.length;
						const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
						const start = page * pageSize;
						return { data: rows.slice(start, start + pageSize), total, totalPages };
					},
					create: async (body) => {
						const rollup = rollupFromForm(body);
						const res = await wikiAdminApi.createCategory(sel.space_type_id, {
							slug: String(body.slug ?? '').trim(),
							category: String(body.category ?? '').trim(),
							label: String(body.label ?? '').trim(),
							icon: String(body.icon ?? '').trim() || null,
							field_group_id: String(body.field_group_id ?? ''),
							position: Number(body.position ?? 0),
							...(rollup !== null ? { rollup } : {})
						});
						return res.ok ? { ok: true } : { ok: false, error: res.error };
					},
					update: async (id, body) => {
						// Slug/category are immutable — the PATCH contract carries
						// label, icon, position, and the group binding. The binding
						// is mandatory (NOT NULL), so an empty selection is
						// omitted — the server rejects "" rather than clearing.
						// Rollup and icon are three-state: the seeded controls
						// round-trip the stored values, disabling/emptying clears.
						const res = await wikiAdminApi.updateCategory(id, {
							label: String(body.label ?? '').trim(),
							icon: String(body.icon ?? '').trim() || null,
							position: Number(body.position ?? 0),
							field_group_id: String(body.field_group_id ?? '').trim() || undefined,
							rollup: rollupFromForm(body)
						});
						return res.ok ? { ok: true } : { ok: false, error: res.error };
					},
					remove: async (id) => {
						const res = await wikiAdminApi.deleteCategory(id);
						return res.ok ? { ok: true } : { ok: false, error: res.error };
					}
		};
	});

	// ── Page templates table (per selected category) ─────────────────────
	// §80 page templates: sibling wiki pages under one category, each bound
	// to a shared provisioned group so item keys stay JCE-comparable.

	const tplFields: FieldDef[] = [
		{
			key: 'slug',
			label: t('app.wiki.tpl_slug', 'Page slug'),
			type: 'text',
			required: true,
			readonlyInEdit: true,
			placeholder: 'prices',
			help: t('app.wiki.tpl_slug_help', 'URL segment — /wiki/{entity}/{slug}. Lowercase, hyphenated; immutable once created.')
		},
		{ key: 'label', label: t('app.wiki.tpl_label', 'Label'), type: 'text', required: true, placeholder: 'Prices' },
		{
			key: 'field_group_id',
			label: t('app.wiki.cat_group', 'Field group'),
			type: 'autocomplete',
			placeholder: t('app.wiki.cat_group_ph', 'Search field groups…'),
			emptyMessage: t('common.no_results', 'No results found'),
			fetcher: groupFetcher,
			initialFetcher: async () => filterOptions(groupOptions, ''),
			getSelectedLabel: (id: string) => groupOptions.find((o) => o.value === id)?.label ?? id,
			help: t('app.wiki.tpl_group_help', 'Blank = auto-provision a shared wiki_{type}_page_{slug} group — keep it shared so item keys stay comparable across spaces.')
		},
		{ key: 'position', label: t('app.wiki.cat_position', 'Position'), type: 'number' }
	];

	const tplActions: ActionItem[] = [
		{ label: t('common.view', 'View'), icon: Eye, onclick: (row, ctx) => ctx?.openView(row) },
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row, ctx) => ctx?.openEdit(row),
			show: () => canManage
		},
		{
			label: t('common.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row, ctx) => ctx?.openDelete(row),
			// System templates are undeletable (backend also rejects them).
			show: (row) => canManage && !(row as WikiPageTemplateItem).is_system
		}
	];

	const tplConfig = $derived.by<EntityPageConfig | null>(() => {
		const sel = selectedCategory;
		if (!sel) return null;
		return {
			title: t('app.wiki.templates_title', 'Pages — {category}').replace('{category}', sel.label || sel.slug),
			description: t(
				'app.wiki.templates_desc',
				'Sibling pages under this category (e.g. "Prices in {city}"). Each may bind its own field group.'
			),
			createLabel: t('app.wiki.tpl_create', 'Add page template'),
			createPermission: 'wiki.manage',
			pageSize: 25,
			columns: [
				{ key: 'slug', label: t('app.wiki.tpl_slug', 'Slug'), render: (v: string) => `<span class="font-mono text-xs">${v}</span>` },
				{ key: 'label', label: t('app.wiki.tpl_label', 'Label') },
				{
					key: 'field_group_name',
					label: t('app.wiki.cat_group', 'Field group'),
					render: (v: string, row: WikiPageTemplateItem) =>
						v
							? `${v}<span class="block font-mono text-xs" style="color: var(--color-muted)">${row.field_group_slug ?? ''}</span>`
							: `<span class="text-xs" style="color: var(--color-muted)">${t('app.wiki.unbound', 'unbound')}</span>`
				},
				{ key: 'position', label: t('app.wiki.cat_position', 'Pos'), width: 64 },
				{ key: 'is_system', label: t('app.wiki.cat_system', 'System'), componentSnippet: BoolCell }
			],
			actions: tplActions,
			fields: tplFields,
			urlSync: false,
			load: async ({ page, pageSize, q }) => {
				const res = await wikiAdminApi.listPageTemplates(sel.id);
				if (!res.ok) throw new Error(res.error ?? t('common.error', 'Failed to load page templates'));
				let rows = (res.data as WikiPageTemplateItem[]) ?? [];
				const query = (q ?? '').trim().toLowerCase();
				if (query)
					rows = rows.filter(
						(r) => r.slug.toLowerCase().includes(query) || r.label.toLowerCase().includes(query)
					);
				const total = rows.length;
				const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
				const start = page * pageSize;
				return { data: rows.slice(start, start + pageSize), total, totalPages };
			},
			create: async (body) => {
				const res = await wikiAdminApi.createPageTemplate(sel.id, {
					slug: String(body.slug ?? '').trim(),
					label: String(body.label ?? '').trim(),
					field_group_id: String(body.field_group_id ?? '').trim() || undefined,
					position: Number(body.position ?? 0)
				});
				return res.ok ? { ok: true } : { ok: false, error: res.error };
			},
			update: async (id, body) => {
				// Slug immutable; group binding omitted when empty (the server
				// keeps the current binding rather than rejecting "").
				const res = await wikiAdminApi.updatePageTemplate(id, {
					label: String(body.label ?? '').trim(),
					position: Number(body.position ?? 0),
					field_group_id: String(body.field_group_id ?? '').trim() || undefined
				});
				return res.ok ? { ok: true } : { ok: false, error: res.error };
			},
			remove: async (id) => {
				const res = await wikiAdminApi.deletePageTemplate(id);
				return res.ok ? { ok: true } : { ok: false, error: res.error };
			}
		};
	});
</script>

<div class="space-y-6">
	{#if pageError}
		<div
			class="rounded-lg p-3 text-sm"
			style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
		>
			{pageError}
		</div>
	{/if}
	<EntityPage config={typeConfig} bind:controller={typesController} />

	{#if selectedType && catConfig}
		<section class="rounded-[12px] border p-4" style="border-color: var(--color-border); background: var(--color-surface)">
			<div class="mb-3 flex items-center justify-between">
				<p class="text-xs" style="color: var(--color-muted)">
					{t('app.wiki.categories_hint', 'Editing the category template for this wiki type.')}
				</p>
				<Button variant="ghost" onclick={() => (selectedType = null)}>
					<X size={14} class="mr-1" />{t('common.close', 'Close')}
				</Button>
			</div>
			{#key selectedType.space_type_id}
				<EntityPage config={catConfig} bind:controller={catsController} />
			{/key}
		</section>
	{/if}

	{#if selectedCategory && tplConfig}
		<section class="rounded-[12px] border p-4" style="border-color: var(--color-border); background: var(--color-surface)">
			<div class="mb-3 flex items-center justify-between">
				<p class="text-xs" style="color: var(--color-muted)">
					{t('app.wiki.templates_hint', 'Editing the page templates under this category.')}
				</p>
				<Button variant="ghost" onclick={() => (selectedCategory = null)}>
					<X size={14} class="mr-1" />{t('common.close', 'Close')}
				</Button>
			</div>
			{#key selectedCategory.id}
				<EntityPage config={tplConfig} bind:controller={tplsController} />
			{/key}
		</section>
	{/if}
</div>

{#snippet BoolCell({ value }: { value: boolean; row: unknown })}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {value ? 'bg-success/15 text-success' : 'bg-muted/15 text-muted'}">
		{value ? t('common.yes', 'Yes') : t('common.no', 'No')}
	</span>
{/snippet}

<!-- Type create/edit form: EntityForm has no multi-select, so the parent
	 edge list renders as a checkbox group of the other wiki types. -->
{#snippet TypeForm({ value, editing }: { value: Record<string, unknown>; editing: boolean })}
	<div class="space-y-4">
		<div>
			<label for="wt-type" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
				{t('app.wiki.type_space_type', 'Space type')} *
			</label>
			{#if editing}
				<!-- space_type_id is immutable on edit — render the resolved
				     label as a read-only field rather than a disabled dropdown. -->
				<input
					id="wt-type"
					type="text"
					disabled
					class="w-full cursor-not-allowed rounded-lg border p-2 text-sm opacity-70"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					value={spaceTypeLabel(value.space_type_id) ?? ''}
				/>
				<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('app.wiki.type_immutable', 'The space type cannot be changed.')}</p>
			{:else}
				<Autocomplete
					id="wt-type"
					placeholder={t('app.wiki.type_pick', 'Search space types…')}
					emptyMessage={t('common.no_results', 'No results found')}
					fetcher={spaceTypeFetcher}
					initialFetcher={async () => filterOptions(spaceTypeOptions, '')}
					selectedId={String(value.space_type_id ?? '') || null}
					selectedLabel={spaceTypeLabel(value.space_type_id)}
					onSelect={(item) => (value.space_type_id = item.id)}
					onClear={() => (value.space_type_id = '')}
				/>
			{/if}
		</div>
		<div>
			<label for="wt-label" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.wiki.type_label', 'Label')}</label>
			<input
				id="wt-label" type="text" class="w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				value={String(value.label ?? '')} oninput={(e) => (value.label = e.currentTarget.value)}
			/>
		</div>
		<div>
			<label for="wt-icon" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.wiki.type_icon', 'Icon')}</label>
			<input
				id="wt-icon" type="text" class="w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				value={String(value.icon ?? '')} oninput={(e) => (value.icon = e.currentTarget.value)}
			/>
		</div>
		<div>
			<label for="wt-position" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.wiki.type_position', 'Position')}</label>
			<input
				id="wt-position" type="number" class="w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				value={Number(value.position ?? 0)} oninput={(e) => (value.position = Number(e.currentTarget.value))}
			/>
		</div>
		<label class="flex items-center gap-2 text-sm" style="color: var(--color-foreground)">
			<input
				type="checkbox" class="h-4 w-4 rounded"
				checked={value.enabled === undefined ? true : !!value.enabled}
				onchange={(e) => (value.enabled = e.currentTarget.checked)}
			/>
			{t('app.wiki.type_enabled', 'Enabled')}
		</label>
		<label class="flex items-center gap-2 text-sm" style="color: var(--color-foreground)">
			<input
				type="checkbox" class="h-4 w-4 rounded"
				checked={!!value.seed_stubs}
				onchange={(e) => (value.seed_stubs = e.currentTarget.checked)}
			/>
			{t('app.wiki.type_seed_stubs', 'Seed stub pages for spaces of this type')}
		</label>
		<div>
			<p class="mb-1 text-sm font-medium" style="color: var(--color-foreground)">
				{t('app.wiki.type_parents', 'Parent wiki types')}
			</p>
			{#if knownTypes.filter((pt) => pt.space_type_id !== value.space_type_id).length === 0}
				<p class="text-xs" style="color: var(--color-muted)">{t('app.wiki.type_no_parents', 'No other wiki types yet.')}</p>
			{:else}
				<div class="space-y-1.5">
					{#each knownTypes.filter((pt) => pt.space_type_id !== value.space_type_id) as pt (pt.space_type_id)}
						<label class="flex items-center gap-2 text-sm" style="color: var(--color-foreground)">
							<input
								type="checkbox" class="h-4 w-4 rounded"
								checked={((value.parent_type_ids as string[]) ?? []).includes(pt.space_type_id)}
								onchange={(e) => toggleParent(value, pt.space_type_id, e.currentTarget.checked)}
							/>
							{pt.label || pt.space_type_name}
							<span class="font-mono text-xs" style="color: var(--color-muted)">{pt.space_type_slug}</span>
						</label>
					{/each}
				</div>
			{/if}
		</div>
	</div>
{/snippet}

<!-- Category create/edit form: the rollup JSONB is edited as structured
	 controls (enabled → mode → metrics/levels/section_limit), so all inputs
	 are rendered here rather than via EntityForm. -->
{#snippet CategoryForm({ value, editing }: { value: Record<string, unknown>; editing: boolean })}
	{@const kids = childTypesOf(selectedType)}
	<div class="space-y-4">
		<div>
			<label for="wc-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
				{t('app.wiki.cat_slug', 'Page slug')} *
			</label>
			<input
				id="wc-slug" type="text" required disabled={editing} placeholder="cost-of-living"
				class="w-full rounded-lg border p-2 text-sm {editing ? 'cursor-not-allowed opacity-70' : ''}"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				value={String(value.slug ?? '')} oninput={(e) => (value.slug = e.currentTarget.value)}
			/>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">
				{t('app.wiki.cat_slug_help', 'URL segment — /wiki/{entity}/{slug}. Lowercase, hyphenated; immutable once created.')}
			</p>
		</div>
		<div>
			<label for="wc-token" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
				{t('app.wiki.cat_token', 'Category token')}
			</label>
			<input
				id="wc-token" type="text" disabled={editing} placeholder="cost_of_living"
				class="w-full rounded-lg border p-2 text-sm {editing ? 'cursor-not-allowed opacity-70' : ''}"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				value={String(value.category ?? '')} oninput={(e) => (value.category = e.currentTarget.value)}
			/>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">
				{t('app.wiki.cat_token_help', 'wiki_pages.category token (lowercase, underscored). Blank = derived from the slug.')}
			</p>
		</div>
		<div>
			<label for="wc-label" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
				{t('app.wiki.cat_label', 'Label')} *
			</label>
			<input
				id="wc-label" type="text" required
				class="w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				value={String(value.label ?? '')} oninput={(e) => (value.label = e.currentTarget.value)}
			/>
		</div>
		<div>
			<label for="wc-icon" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
				{t('app.wiki.cat_icon', 'Icon')}
			</label>
			<input
				id="wc-icon" type="text" placeholder="wallet"
				class="w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				value={String(value.icon ?? '')} oninput={(e) => (value.icon = e.currentTarget.value)}
			/>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">
				{t('app.wiki.cat_icon_help', 'Lucide icon name (kebab-case) shown on the category tab and card — e.g. wallet, globe, landmark. Blank falls back to the default category icon.')}
			</p>
		</div>
		<div>
			<label for="wc-group" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
				{t('app.wiki.cat_group', 'Field group')}
			</label>
			<Autocomplete
				id="wc-group"
				placeholder={t('app.wiki.cat_group_ph', 'Search field groups…')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={groupFetcher}
				initialFetcher={async () => filterOptions(groupOptions, '')}
				selectedId={String(value.field_group_id ?? '') || null}
				selectedLabel={groupOptions.find((o) => o.value === value.field_group_id)?.label}
				onSelect={(item) => (value.field_group_id = item.id)}
				onClear={() => (value.field_group_id = '')}
			/>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">
				{t('app.wiki.cat_group_help', 'The bound group is the page’s shared structured schema — a dedicated group is created automatically when none is picked. Only pick an existing group to deliberately share a schema.')}
			</p>
		</div>
		<div>
			<label for="wc-position" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
				{t('app.wiki.cat_position', 'Position')}
			</label>
			<input
				id="wc-position" type="number"
				class="w-full rounded-lg border p-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				value={Number(value.position ?? 0)} oninput={(e) => (value.position = Number(e.currentTarget.value))}
			/>
		</div>
		<div class="rounded-lg border p-3" style="border-color: var(--color-border)">
			<label class="flex items-center gap-2 text-sm font-medium" style="color: var(--color-foreground)">
				<input
					type="checkbox" class="h-4 w-4 rounded"
					checked={!!value.rollup_enabled}
					onchange={(e) => (value.rollup_enabled = e.currentTarget.checked)}
				/>
				{t('app.wiki.rollup_enabled', 'Roll up child wikis onto this category’s page')}
			</label>
			{#if value.rollup_enabled}
				<div class="mt-3 space-y-3 pl-6">
					<div>
						<label for="wc-rollup-mode" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
							{t('app.wiki.rollup_mode', 'Mode')}
						</label>
						<AutocompleteSelect
							id="wc-rollup-mode"
							value={String(value.rollup_mode ?? 'aggregate')}
							onValueChange={(v) => (value.rollup_mode = v)}
							options={[{ value: 'aggregate', label: t('app.wiki.rollup_mode_aggregate', 'Aggregate field values') }, { value: 'directory', label: t('app.wiki.rollup_mode_directory', 'Directory — list child wiki spaces') }]}
							class="w-full"
						/>
						<p class="mt-1 text-xs" style="color: var(--color-muted)">
							{value.rollup_mode === 'directory'
								? t('app.wiki.rollup_mode_directory_help', 'The category page lists child wiki spaces (e.g. a country’s states) with scoped search.')
								: t('app.wiki.rollup_mode_aggregate_help', 'Descendant spaces’ numeric/price-range field values fold up onto this category’s page.')}
						</p>
					</div>
					{#if value.rollup_mode !== 'directory'}
						<div>
							<p class="mb-1 text-sm font-medium" style="color: var(--color-foreground)">
								{t('app.wiki.rollup_metrics', 'Metrics')}
							</p>
							<div class="flex gap-4">
								{#each ROLLUP_METRICS as m (m)}
									<label class="flex items-center gap-2 text-sm" style="color: var(--color-foreground)">
										<input
											type="checkbox" class="h-4 w-4 rounded"
											checked={((value.rollup_metrics as string[]) ?? []).includes(m)}
											onchange={(e) => toggleRollupList(value, 'rollup_metrics', m, e.currentTarget.checked)}
										/>
										{m}
									</label>
								{/each}
							</div>
						</div>
						<div>
							<label for="wc-rollup-limit" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
								{t('app.wiki.rollup_section_limit', 'Section limit')}
							</label>
							<input
								id="wc-rollup-limit" type="number" min="0"
								class="w-full rounded-lg border p-2 text-sm"
								style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
								value={Number(value.rollup_section_limit ?? 0)}
								oninput={(e) => (value.rollup_section_limit = Number(e.currentTarget.value))}
							/>
							<p class="mt-1 text-xs" style="color: var(--color-muted)">
								{t('app.wiki.rollup_section_limit_help', 'Max rows per field section on the page — 0 = no cap.')}
							</p>
						</div>
					{/if}
					<div>
						<p class="mb-1 text-sm font-medium" style="color: var(--color-foreground)">
							{t('app.wiki.rollup_levels', 'Child levels')}
						</p>
						{#if kids.length === 0}
							<p class="text-xs" style="color: var(--color-muted)">
								{t('app.wiki.rollup_no_levels', 'This type has no declared child wiki types — every descendant type is included.')}
							</p>
						{:else}
							<div class="space-y-1.5">
								{#each kids as kid (kid.space_type_id)}
									<label class="flex items-center gap-2 text-sm" style="color: var(--color-foreground)">
										<input
											type="checkbox" class="h-4 w-4 rounded"
											checked={((value.rollup_levels as string[]) ?? []).includes(kid.space_type_slug)}
											onchange={(e) => toggleRollupList(value, 'rollup_levels', kid.space_type_slug, e.currentTarget.checked)}
										/>
										{kid.label || kid.space_type_name}
										<span class="font-mono text-xs" style="color: var(--color-muted)">{kid.space_type_slug}</span>
									</label>
								{/each}
							</div>
							<p class="mt-1 text-xs" style="color: var(--color-muted)">
								{t('app.wiki.rollup_levels_help', 'Restrict which descendant types fold in — none checked = every declared child type.')}
							</p>
						{/if}
					</div>
				</div>
			{/if}
		</div>
	</div>
{/snippet}
