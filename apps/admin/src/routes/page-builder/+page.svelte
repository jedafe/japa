<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /page-builder — migrated to the EntityPage scaffold; CRUD list of pagebuilder blocks.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { pageBuilderAdminApiExtended, pageBuilderApi, apiRaw } from '$lib/api';
	import type { BlockType } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Trash2 } from 'lucide-svelte';

	type Block = {
		id: string;
		name: string;
		slug: string;
		category: string;
		component: string;
		default_config: Record<string, unknown> | null;
		config_schema: Record<string, unknown> | null;
		preview_image: string | null;
		is_enabled: boolean;
		is_system: boolean;
		created_at: string;
		updated_at: string;
	};

	// slugify derives a URL-safe slug from a free-form name so the backend
	// (CreateBlockInput.Slug — required) always receives a valid value even
	// when the admin leaves the Slug field blank.
	function slugify(value: string): string {
		return value.toLowerCase().trim()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-+|-+$/g, '');
	}

	// Block type options fetched from the API registry (Rule 16 — no hardcoded data).
	// The /page-builder/block-types endpoint returns the static block template registry.
	let blockTypeOptions = $state<BlockType[]>([]);

	// Derive categories dynamically from the fetched block types.
	const blockCategories = $derived(
		[...new Set(blockTypeOptions.map((bt) => bt.category))].filter(Boolean).sort()
	);

	$effect(() => {
		(async () => {
			try {
				const result = await pageBuilderApi.getBlockTypes();
				if (result.ok) {
					blockTypeOptions = result.data?.block_types ?? [];
				}
			} catch {
				// Keep empty options on failure; form selects will just have no options.
			}
		})();
	});

	// ── Bulk delete ───── EntityPage auto-reloads + clears selection after a bulk action handler completes.
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
			await Promise.all(idArr.map((id) => pageBuilderAdminApiExtended.deleteBlock(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete blocks');
		} finally {
			bulkDeleting = false;
		}
	}

	// ── Component / category autocomplete helpers (filter pre-loaded options client-side) ──
	function fetchComponentOptions(q: string): Promise<{ id: string; label: string }[]> {
		const opts = blockTypeOptions.map((bt) => ({ value: bt.type, label: bt.name }));
		const query = q.toLowerCase();
		const filtered = query === ''
			? opts
			: opts.filter((o) => o.label.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((o) => ({ id: o.value, label: o.label })));
	}

	function fetchComponentOptionsInitial(): Promise<{ id: string; label: string }[]> {
		return Promise.resolve(blockTypeOptions.map((bt) => ({ id: bt.type, label: bt.name })));
	}

	function getComponentSelectedLabel(id: string): string {
		return blockTypeOptions.find((bt) => bt.type === id)?.name ?? id;
	}

	function fetchCategoryOptions(q: string): Promise<{ id: string; label: string }[]> {
		const opts = blockCategories.map((cat) => ({ value: cat, label: cat }));
		const query = q.toLowerCase();
		const filtered = query === ''
			? opts
			: opts.filter((o) => o.label.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((o) => ({ id: o.value, label: o.label })));
	}

	function fetchCategoryOptionsInitial(): Promise<{ id: string; label: string }[]> {
		return Promise.resolve(blockCategories.map((cat) => ({ id: cat, label: cat })));
	}

	function getCategorySelectedLabel(id: string): string {
		return blockCategories.find((cat) => cat === id) ?? id;
	}

	// `fields` drives EntityForm rendering and edit-form seeding. Component and
	// category are autocomplete fields whose options are populated from the fetched
	// block type registry. Config is built as a $derived so option updates
	// propagate once the registry loads.
	const fields = $derived<FieldDef[]>([
		{ key: 'name', label: t('pageBuilder.name', 'Name'), type: 'text', required: true },
		{ key: 'slug', label: t('pageBuilder.slug', 'Slug'), type: 'text', placeholder: t('pageBuilder.slug_placeholder', 'auto-generated from name') },
		{
			key: 'component',
			label: t('pageBuilder.type', 'Type'),
			type: 'autocomplete',
			placeholder: t('pageBuilder.select_type', 'Select a block type'),
			emptyMessage: t('common.no_results', 'No results found'),
			fetcher: fetchComponentOptions,
			initialFetcher: fetchComponentOptionsInitial,
			getSelectedLabel: getComponentSelectedLabel,
			required: true
		},
		{
			key: 'category',
			label: t('pageBuilder.category', 'Category'),
			type: 'autocomplete',
			placeholder: t('pageBuilder.select_category', 'Select a category'),
			emptyMessage: t('common.no_results', 'No results found'),
			fetcher: fetchCategoryOptions,
			initialFetcher: fetchCategoryOptionsInitial,
			getSelectedLabel: getCategorySelectedLabel
		},
		{ key: 'is_enabled', label: t('pageBuilder.enabled', 'Enabled'), type: 'checkbox' }
	]);

	const config = $derived<EntityPageConfig>({
		title: t('pageBuilder.blocks_title', 'Blocks'),
		description: t('pageBuilder.blocks_description', 'Manage page builder blocks.'),
		createLabel: t('pageBuilder.create_block', 'Create Block'),
		createPermission: 'pagebuilder.blocks.manage',
		editPermission: 'pagebuilder.blocks.manage',
		deletePermission: 'pagebuilder.blocks.manage',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('pageBuilder.name', 'Name'), sortable: true },
			{
				key: 'component',
				label: t('pageBuilder.type', 'Type'),
				sortable: true,
				render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{
				key: 'category',
				label: t('pageBuilder.category', 'Category'),
				sortable: true,
				render: (v: string) => v || '—'
			},
			{
				key: 'is_enabled',
				label: t('pageBuilder.status', 'Status'),
				sortable: true,
				render: (v: boolean) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${v ? 'bg-success/15 text-success dark:text-success' : 'bg-muted/15 text-muted dark:text-muted'}">${v ? t('pageBuilder.enabled', 'Enabled') : t('pageBuilder.disabled', 'Disabled')}</span>`
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
		load: async ({ q, page, pageSize, sort }) => {
			const params = new URLSearchParams();
			params.set('page_size', String(pageSize));
			// EntityPage's page is 0-based; the API's `page` is 1-based.
			params.set('page', String(page + 1));
			if (sort) {
				const [key, dir] = sort.split(':');
				params.set('sort', key);
				if (dir) params.set('dir', dir);
			}
			if (q) params.set('q', q);
			const result = await apiRaw<{ data?: Block[]; total?: number; total_pages?: number }>(
				`/admin/pagebuilder/blocks?${params.toString()}`
			);
			if (result.ok) {
				const d = result.data;
				const items = d.data ?? [];
				return {
					data: items,
					total: d.total ?? items.length,
					totalPages: d.total_pages ?? 1
				};
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load blocks'));
		},
		create: async (body) => {
			const name = String(body.name ?? '').trim();
			const res = await pageBuilderAdminApiExtended.createBlock({
				name,
				slug: String(body.slug ?? '').trim() || slugify(name),
				component: String(body.component ?? '').trim(),
				category: String(body.category ?? '').trim() || undefined,
				is_enabled: body.is_enabled === undefined ? true : !!body.is_enabled
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const name = String(body.name ?? '').trim();
			const res = await pageBuilderAdminApiExtended.updateBlock(id, {
				name,
				slug: String(body.slug ?? '').trim() || slugify(name),
				component: String(body.component ?? '').trim(),
				category: String(body.category ?? '').trim() || undefined,
				is_enabled: !!body.is_enabled
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await pageBuilderAdminApiExtended.deleteBlock(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: BlockView
	});
</script>

<svelte:head>
	<title>{t('pageBuilder.blocks_title', 'Blocks')} — {siteName()} Admin</title>
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

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

<!-- View modal body (mirrors the legacy preview modal) -->
{#snippet BlockView({ row }: { row: Block })}
	<div class="space-y-4">
		<!-- Preview header -->
		<div class="flex items-center gap-3">
			<div class="flex h-12 w-12 items-center justify-center rounded-lg border" style="border-color: var(--color-border); background: var(--color-surface)">
				<span class="text-lg font-bold" style="color: var(--color-primary)">{row.component.charAt(0).toUpperCase()}</span>
			</div>
			<div>
				<p class="text-sm font-semibold" style="color: var(--color-foreground)">{row.name}</p>
				<p class="text-xs" style="color: var(--color-muted)">{row.component}</p>
			</div>
			<div class="ms-auto">
				{#if row.is_enabled}
					<span class="inline-block rounded-full bg-success/15 px-2 py-0.5 text-xs font-medium text-success">{t('pageBuilder.enabled', 'Enabled')}</span>
				{:else}
					<span class="inline-block rounded-full bg-muted/15 px-2 py-0.5 text-xs font-medium text-muted">{t('pageBuilder.disabled', 'Disabled')}</span>
				{/if}
			</div>
		</div>

		<!-- Details grid -->
		<div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
			<div>
				<p class="mb-0.5 text-xs font-medium" style="color: var(--color-muted)">{t('pageBuilder.name', 'Name')}</p>
				<p class="text-sm" style="color: var(--color-foreground)">{row.name}</p>
			</div>
			<div>
				<p class="mb-0.5 text-xs font-medium" style="color: var(--color-muted)">{t('pageBuilder.slug', 'Slug')}</p>
				<p class="font-mono text-xs" style="color: var(--color-foreground)">{row.slug}</p>
			</div>
			<div>
				<p class="mb-0.5 text-xs font-medium" style="color: var(--color-muted)">{t('pageBuilder.type', 'Type')}</p>
				<p class="font-mono text-xs" style="color: var(--color-foreground)">{row.component}</p>
			</div>
			<div>
				<p class="mb-0.5 text-xs font-medium" style="color: var(--color-muted)">{t('pageBuilder.category', 'Category')}</p>
				<p class="text-sm" style="color: var(--color-foreground)">{row.category || '—'}</p>
			</div>
			<div>
				<p class="mb-0.5 text-xs font-medium" style="color: var(--color-muted)">{t('pageBuilder.created', 'Created')}</p>
				<p class="text-xs" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleDateString()}</p>
			</div>
			<div>
				<p class="mb-0.5 text-xs font-medium" style="color: var(--color-muted)">{t('pageBuilder.updated', 'Updated')}</p>
				<p class="text-xs" style="color: var(--color-foreground)">{new Date(row.updated_at).toLocaleDateString()}</p>
			</div>
		</div>

		<!-- Default config -->
		{#if row.default_config && Object.keys(row.default_config).length > 0}
			<div>
				<p class="mb-1 text-xs font-medium" style="color: var(--color-muted)">{t('pageBuilder.default_config', 'Default Config')}</p>
				<pre class="overflow-x-auto rounded-lg border p-3 text-xs" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)">{JSON.stringify(row.default_config, null, 2)}</pre>
			</div>
		{/if}

		<!-- Config schema -->
		{#if row.config_schema && Object.keys(row.config_schema).length > 0}
			<div>
				<p class="mb-1 text-xs font-medium" style="color: var(--color-muted)">{t('pageBuilder.config_schema', 'Config Schema')}</p>
				<pre class="overflow-x-auto rounded-lg border p-3 text-xs" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)">{JSON.stringify(row.config_schema, null, 2)}</pre>
			</div>
		{/if}
	</div>
{/snippet}
