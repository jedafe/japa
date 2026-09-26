<script lang="ts">
	// §80 Wiki Types & Field-Group Pages — admin management for wiki types
	// (space_types carrying a wiki_type_configs row, with parent edges that
	// express the geo chain + child types like university) and each type's
	// category template (slug + category token + optional field-group binding).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Button } from '$lib/components/ui/button';
	import {
		wikiAdminApi,
		fieldGroupsAdminApiExtended,
		spaceTypesApi,
		type WikiTypeItem,
		type WikiCategoryItem
	} from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Pencil, Trash2, FolderTree, X } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

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
				{ value: '', label: t('app.wiki.group_none', '— Prose only (no group) —') },
				...rows.map((g) => ({ value: g.id, label: g.name }))
			];
		}
	}

	// ── Types table ─────────────────────────────────────────────────────────
	let typesController = $state<EntityPageController | undefined>(undefined);
	let catsController = $state<EntityPageController | undefined>(undefined);
	let selectedType = $state<WikiTypeItem | null>(null);

	// Map a type row: EntityPage's id-based row actions need `id`.
	function mapType(row: WikiTypeItem) {
		return { ...row, id: row.space_type_id, parents: (row.parent_type_ids ?? []).length };
	}

	const typeFields: FieldDef[] = [
		{ key: 'space_type_id', label: t('app.wiki.type_space_type', 'Space type'), type: 'text', required: true },
		{ key: 'label', label: t('app.wiki.type_label', 'Label'), type: 'text', placeholder: 'University Wikis' },
		{ key: 'icon', label: t('app.wiki.type_icon', 'Icon'), type: 'text' },
		{ key: 'position', label: t('app.wiki.type_position', 'Position'), type: 'number' },
		{ key: 'enabled', label: t('app.wiki.type_enabled', 'Enabled'), type: 'checkbox' },
		{ key: 'seed_stubs', label: t('app.wiki.type_seed_stubs', 'Seed stub pages'), type: 'checkbox' }
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

	const typeConfig: EntityPageConfig = {
		title: t('app.wiki.types_title', 'Wiki Types'),
		description: t(
			'app.wiki.types_desc',
			'Space types that carry a wiki. The geo chain (country → state → city) is primary; other types nest under it via parent edges.'
		),
		createLabel: t('app.wiki.type_create', 'New wiki type'),
		createPermission: 'wiki.manage',
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
		fields: typeFields,
		formSnippet: TypeForm,
		load: async ({ page, pageSize, q }) => {
			const res = await wikiAdminApi.listTypes();
			if (!res.ok) throw new Error(res.error ?? t('common.error', 'Failed to load wiki types'));
			let rows = ((res.data as WikiTypeItem[]) ?? []).map(mapType);
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
				parent_type_ids: (body.parent_type_ids as string[]) ?? []
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await wikiAdminApi.deleteType(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		}
	};

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
			key: 'field_group_id',
			label: t('app.wiki.cat_group', 'Field group'),
			type: 'select',
			options: [],
			help: t('app.wiki.cat_group_help', 'Bound group = the page’s shared structured schema. Empty keeps the page prose-only.')
		},
		{ key: 'position', label: t('app.wiki.cat_position', 'Position'), type: 'number' }
	];

	const catActions: ActionItem[] = [
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
									: `<span class="text-xs" style="color: var(--color-muted)">${t('app.wiki.prose_only', 'prose only')}</span>`
						},
						{ key: 'position', label: t('app.wiki.cat_position', 'Pos'), width: 64 },
						{ key: 'is_system', label: t('app.wiki.cat_system', 'System'), componentSnippet: BoolCell }
					],
					actions: catActions,
					fields: catFields.map((f) =>
						f.key === 'field_group_id' ? { ...f, options: groupOptions } : f
					),
					load: async ({ page, pageSize, q }) => {
						const res = await wikiAdminApi.listTypeCategories(sel.space_type_id);
						if (!res.ok) throw new Error(res.error ?? t('common.error', 'Failed to load categories'));
						let rows = (res.data as WikiCategoryItem[]) ?? [];
						const query = (q ?? '').trim().toLowerCase();
						if (query) rows = rows.filter((r) => r.slug.includes(query) || r.label.toLowerCase().includes(query));
						const total = rows.length;
						const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
						const start = page * pageSize;
						return { data: rows.slice(start, start + pageSize), total, totalPages };
					},
					create: async (body) => {
						const res = await wikiAdminApi.createCategory(sel.space_type_id, {
							slug: String(body.slug ?? '').trim(),
							category: String(body.category ?? '').trim(),
							label: String(body.label ?? '').trim(),
							field_group_id: String(body.field_group_id ?? ''),
							position: Number(body.position ?? 0)
						});
						return res.ok ? { ok: true } : { ok: false, error: res.error };
					},
					update: async (id, body) => {
						// Slug/category are immutable — the PATCH contract carries
						// label, position, and the group binding ("" clears it).
						const res = await wikiAdminApi.updateCategory(id, {
							label: String(body.label ?? '').trim(),
							position: Number(body.position ?? 0),
							field_group_id: String(body.field_group_id ?? '')
						});
						return res.ok ? { ok: true } : { ok: false, error: res.error };
					},
					remove: async (id) => {
						const res = await wikiAdminApi.deleteCategory(id);
						return res.ok ? { ok: true } : { ok: false, error: res.error };
					}
		};
	});
</script>

<div class="space-y-6">
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
			<select
				id="wt-type"
				class="w-full rounded-lg border p-2 text-sm disabled:cursor-not-allowed disabled:opacity-50"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				disabled={editing}
				value={String(value.space_type_id ?? '')}
				onchange={(e) => (value.space_type_id = e.currentTarget.value)}
			>
				<option value="">{t('app.wiki.type_pick', 'Select a space type…')}</option>
				{#each spaceTypeOptions as o (o.value)}
					<option value={o.value}>{o.label}</option>
				{/each}
				{#if editing && value.space_type_id && !spaceTypeOptions.some((o) => o.value === value.space_type_id)}
					<option value={String(value.space_type_id)}>{value.space_type_slug ?? value.space_type_id}</option>
				{/if}
			</select>
			{#if editing}
				<p class="mt-1 text-xs" style="color: var(--color-muted)">{t('app.wiki.type_immutable', 'The space type cannot be changed.')}</p>
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
