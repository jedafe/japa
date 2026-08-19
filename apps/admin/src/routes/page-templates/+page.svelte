<script lang="ts">
	// /page-templates — migrated to the EntityPage scaffold; simple CRUD entity using declarative `fields` (EntityForm).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { pageTemplatesAdminApiExtended, apiRaw } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Trash2 } from 'lucide-svelte';

	type PageTemplate = {
		id: string;
		tenant_id: string;
		name: string;
		slug: string;
		description?: string;
		page_type: string;
		blocks: unknown[];
		is_system: boolean;
		created_at: string;
		updated_at: string;
	};

	// Rule 16: page-type categories are loaded from GET /admin/pagebuilder/page-types
	// (shared catalog with the page builder) instead of being hardcoded.
	let pageTypeOptions = $state<{ value: string; label: string }[]>([]);

	$effect(() => {
		void (async () => {
			const res = await pageTemplatesAdminApiExtended.getPageTypes();
			if (res.ok && res.data) {
				pageTypeOptions = res.data.map((pt) => {
					const key = pt.value ? pt.value.charAt(0).toUpperCase() + pt.value.slice(1) : pt.value;
					return { value: pt.value, label: t(`pageTemplates.category${key}`, pt.label) };
				});
			}
		})();
	});

	const categoryOptions = $derived(pageTypeOptions);

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
			await Promise.all(idArr.map((id) => pageTemplatesAdminApiExtended.delete(id)));
			// EntityPage auto-reloads + clears selection after this returns.
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete page templates');
		} finally {
			bulkDeleting = false;
		}
	}

	// `fields` drives EntityForm rendering and edit-form seeding. The slug is server-generated
	// from `name` (shown as a read-only preview in the legacy modal) and is not submitted, so it
	// is intentionally omitted from the editable field set.
	const fields = $derived<FieldDef[]>([
		{ key: 'name', label: t('pageTemplates.name', 'Name'), type: 'text', required: true },
		{
			key: 'page_type',
			label: t('pageTemplates.category', 'Category'),
			type: 'select',
			options: categoryOptions,
			required: true
		},
		{ key: 'description', label: t('pageTemplates.description', 'Description'), type: 'textarea' }
	]);

	const config = $derived<EntityPageConfig>({
		title: t('pageTemplates.title', 'Page Templates'),
		description: t('pageTemplates.description', 'Manage reusable page templates for the page builder.'),
		createLabel: t('pageTemplates.create', 'Create'),
		createPermission: 'pagebuilder.templates.manage',
		pageSize: 25,
		columns: [
			{
				key: 'name',
				label: t('pageTemplates.name', 'Name'),
				sortable: true,
				render: (v: string) => `<span class="font-medium">${v}</span>`
			},
			{
				key: 'slug',
				label: t('pageTemplates.slug', 'Slug'),
				sortable: true,
				render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{
				key: 'page_type',
				label: t('pageTemplates.category', 'Category'),
				sortable: true,
				render: (v: string) => {
					const label = categoryOptions.find((c) => c.value === v)?.label ?? v;
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-primary/15 text-primary">${label}</span>`;
				}
			},
			{
				key: 'blocks',
				label: t('pageTemplates.blocks', 'Blocks'),
				render: (v: unknown[]) => String(Array.isArray(v) ? v.length : 0)
			},
			{
				key: 'is_system',
				label: t('pageTemplates.status', 'Status'),
				sortable: true,
				render: (v: boolean) => {
					const cls = v ? 'bg-warning/15 text-warning' : 'bg-success/15 text-success';
					const text = v ? t('pageTemplates.system', 'System') : t('pageTemplates.custom', 'Custom');
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${text}</span>`;
				}
			}
		],
		filters: [
			{
				key: 'page_type',
				label: t('pageTemplates.category', 'Category'),
				options: categoryOptions
			},
			{
				key: 'is_system',
				label: t('pageTemplates.type', 'Type'),
				options: [
					{ value: 'true', label: t('pageTemplates.system', 'System') },
					{ value: 'false', label: t('pageTemplates.custom', 'Custom') }
				]
			}
		],
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete,
				// Mirror the legacy single-delete guard: system templates are not deletable.
				disabled: (_ids: Set<string>, selectedRows?: any[]) =>
					Array.isArray(selectedRows) && selectedRows.some((r) => r?.is_system)
			}
		],
		fields,
		load: async ({ q, filter, page, pageSize }) => {
			const params = new URLSearchParams();
			params.set('page_size', String(pageSize));
			params.set('page_offset', String(page));
			if (filter.page_type) params.set('category', filter.page_type);
			if (filter.is_system) params.set('is_system', filter.is_system);
			if (q) params.set('q', q);
			const result = await apiRaw<{ data?: PageTemplate[]; total?: number; total_pages?: number }>(
				`/admin/page-templates?${params.toString()}`
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
			throw new Error(result.error ?? t('common.error', 'Failed to load page templates'));
		},
		create: async (body) => {
			const res = await pageTemplatesAdminApiExtended.create({
				name: String(body.name ?? '').trim(),
				description: String(body.description ?? '').trim() || undefined,
				page_type: String(body.page_type ?? 'custom')
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await pageTemplatesAdminApiExtended.update(id, {
				name: String(body.name ?? '').trim(),
				description: String(body.description ?? '').trim() || undefined,
				page_type: String(body.page_type ?? 'custom')
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await pageTemplatesAdminApiExtended.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: TemplateView
	});
</script>

<svelte:head>
	<title>{t('pageTemplates.title', 'Page Templates')} — JapaConnect Admin</title>
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

<!-- View slide-over body (mirrors the legacy view fields) -->
{#snippet TemplateView({ row }: { row: PageTemplate })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('pageTemplates.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('pageTemplates.slug', 'Slug')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('pageTemplates.category', 'Category')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{categoryOptions.find((c) => c.value === row.page_type)?.label ?? row.page_type}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('pageTemplates.description', 'Description')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('pageTemplates.blocks', 'Blocks')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{Array.isArray(row.blocks) ? row.blocks.length : 0}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('pageTemplates.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.is_system ? t('pageTemplates.system', 'System') : t('pageTemplates.custom', 'Custom')}
			</p>
		</div>
	</div>
{/snippet}
