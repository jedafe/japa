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
		userManagementApi,
		type ComparisonType,
		type ComparisonTypeDetail,
		type ComparisonTypeAccessRow,
		type CustomFieldGroupOption,
		type UserGroup
	} from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { GitCompare, Shield, Trash2, Eye, Pencil } from 'lucide-svelte';

	const canCreateType = $derived(hasPermission('comparison.type.create'));
	const canEditType = $derived(hasPermission('comparison.type.edit'));
	const canDeleteType = $derived(hasPermission('comparison.type.delete'));
	const canManageAccess = $derived(hasPermission('comparison.type.access.manage'));

	// ── Dependent form options (field groups + user groups) ──────────────────
	let fieldGroups = $state<CustomFieldGroupOption[]>([]);
	let userGroups = $state<UserGroup[]>([]);

	async function loadOptions() {
		const [groupsResult, userGroupsResult] = await Promise.all([
			comparisonAdminApi.listFieldGroups(),
			userManagementApi.listGroups()
		]);
		if (groupsResult.ok) fieldGroups = groupsResult.data ?? [];
		if (userGroupsResult.ok) userGroups = userGroupsResult.data?.groups ?? [];
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
		load: async ({ q, filter }) => {
			const result = await comparisonAdminApi.listTypes();
			if (result.ok) {
				let data = (result.data as ComparisonType[]) ?? [];
				if (q) {
					const lq = q.toLowerCase();
					data = data.filter(
						(ct) =>
							ct.name.toLowerCase().includes(lq) ||
							ct.slug.toLowerCase().includes(lq) ||
							(ct.description ?? '').toLowerCase().includes(lq)
					);
				}
				if (filter.is_system) {
					const val = filter.is_system === 'true';
					data = data.filter((ct) => ct.is_system === val);
				}
				return { data, total: data.length, totalPages: 1 };
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
				item_label: itemLabel
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
				item_label: itemLabel
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
	<title>{t('comparison_types.title', 'Comparison Types')} — JapaConnect Admin</title>
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
	</div>
{/snippet}

<!-- Create/Edit form body (rich form: dependent field-group selector + locked fields on edit) -->
{#snippet ComparisonTypeForm({ value, editing }: { value: Record<string, any>; editing: any | null })}
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
		<div class="flex items-center gap-2">
			<input id="ct-system" type="checkbox" bind:checked={value.is_system} disabled={!!editing} />
			<label for="ct-system" class="text-sm" style="color: var(--color-foreground)">
				{t('comparison_types.system', 'System')}
			</label>
		</div>
	</div>
{/snippet}
