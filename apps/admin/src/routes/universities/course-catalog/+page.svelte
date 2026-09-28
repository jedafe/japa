<script lang="ts">
	// /universities/course-catalog — canonical institutional course catalog
	// (university_course_catalog, Section 39). EntityPage scaffold.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { courseCatalogAdminApi, type CourseCatalogEntry } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Trash2 } from 'lucide-svelte';

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0 || bulkDeleting) return;
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => courseCatalogAdminApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete courses');
		} finally {
			bulkDeleting = false;
		}
	}

	// ── Bulk force delete — single-row force delete is handled by the
	// scaffold's built-in Hard Delete action (config.hardRemove). ──
	// BulkAction.onclick receives only ids — resolve them against a map of
	// every row seen this session so deleted-row filtering keeps working.
	const catalogRowMap = new Map<string, CourseCatalogEntry>();
	const catalogRowsFor = (ids: Set<string>): CourseCatalogEntry[] =>
		Array.from(ids)
			.map((id) => catalogRowMap.get(id))
			.filter((r): r is CourseCatalogEntry => !!r);

	let forceTargets = $state<CourseCatalogEntry[]>([]);
	let showForceModal = $state(false);

	function openBulkForceDelete(ids: Set<string>) {
		const rows = catalogRowsFor(ids).filter((r) => !!r.deleted_at);
		if (rows.length === 0) return;
		forceTargets = rows;
		showForceModal = true;
	}

	async function confirmForceDelete() {
		if (bulkDeleting || forceTargets.length === 0) return;
		bulkDeleting = true;
		pageError = '';
		try {
			const targets = forceTargets;
			let failed = 0;
			let lastErr = '';
			for (const row of targets) {
				const res = await courseCatalogAdminApi.hardDelete(row.id);
				if (!res.ok) {
					failed++;
					lastErr = res.error ?? '';
				}
			}
			showForceModal = false;
			forceTargets = [];
			if (failed > 0) {
				pageError =
					t('common.bulkDeletePartial', '{failed} of {n} items could not be deleted')
						.replace('{failed}', String(failed))
						.replace('{n}', String(targets.length)) + (lastErr ? ` — ${lastErr}` : '');
			}
			entityController?.reload();
		} finally {
			bulkDeleting = false;
		}
	}

	const fields: FieldDef[] = [
		{ key: 'title', label: t('courses.title', 'Course Title'), type: 'text', required: true },
		{ key: 'slug', label: t('courses.slug', 'Slug'), type: 'text', placeholder: t('courses.slug_hint', 'Auto-generated from title if empty') },
		{ key: 'description', label: t('courses.description', 'Description'), type: 'textarea' },
		{ key: 'degree_type', label: t('courses.degree_type', 'Degree Type'), type: 'text', placeholder: 'BSc / MSc / MBA / PhD' },
		{ key: 'department', label: t('courses.department', 'Department'), type: 'text' },
		{ key: 'field_of_study', label: t('courses.field_of_study', 'Field of Study'), type: 'text' },
		{ key: 'language', label: t('courses.language', 'Language'), type: 'text', placeholder: 'English' },
		{ key: 'default_duration_years', label: t('courses.default_duration', 'Default Duration (years)'), type: 'number' },
		{ key: 'is_published', label: t('courses.published', 'Published'), type: 'checkbox' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('courses.catalog_title', 'Course Catalog'),
		description: t('courses.catalog_desc', 'Canonical university courses — link them to universities via Course Offerings'),
		createLabel: t('courses.add_course', 'Add Course'),
		createPermission: 'universities.course_catalog.create',
		editPermission: 'universities.course_catalog.update',
		deletePermission: 'universities.course_catalog.delete',
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('courses.title', 'Course'), sortable: true, sortKey: 'title', render: (v: string) => `<span class="font-medium">${v ?? ''}</span>` },
			{ key: 'slug', label: t('courses.slug', 'Slug'), render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v ?? ''}</span>` },
			{ key: 'degree_type', label: t('courses.degree_type', 'Degree'), render: (v: string | null) => v ? `<span class="text-xs">${v}</span>` : '—' },
			{ key: 'field_of_study', label: t('courses.field_of_study', 'Field'), render: (v: string | null) => v ? `<span class="text-xs">${v}</span>` : '—' },
			{ key: 'offering_count', label: t('courses.offerings', 'Offerings'), render: (v: number) => `<span class="text-xs">${v ?? 0}</span>` },
			{
				key: 'is_published',
				label: t('courses.status', 'Status'),
				render: (v: boolean, row: CourseCatalogEntry) => row.deleted_at
					? `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, var(--color-danger) 15%, transparent); color: var(--color-danger)">${t('common.filter_deleted', 'Deleted')}</span>`
					: v
					? `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, var(--color-success) 15%, transparent); color: var(--color-success)">${t('courses.published', 'Published')}</span>`
					: `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, var(--color-muted) 15%, transparent); color: var(--color-muted)">${t('courses.unpublished', 'Unpublished')}</span>`
			}
		],
		filters: [
			{
				key: 'is_published',
				label: t('courses.status', 'Status'),
				options: [
					{ value: 'true', label: t('courses.published', 'Published') },
					{ value: 'false', label: t('courses.unpublished', 'Unpublished') }
				]
			},
			// Soft-deleted courses are hidden by default — this filter surfaces
			// them so the Hard Delete action has targets.
			{
				key: 'deleted',
				label: t('common.filter_deleted', 'Deleted'),
				options: [
					{ value: 'only', label: t('common.filter_deleted_only', 'Deleted only') },
					{ value: 'all', label: t('common.filter_deleted_all', 'Include deleted') }
				]
			}
		],
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete,
				// Rows already soft-deleted can't be deleted again.
				disabled: (_ids, rows) => (rows ?? []).some((r) => !!r.deleted_at)
			},
			{
				label: t('common.force_delete', 'Force Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openBulkForceDelete,
				disabled: (ids) => bulkDeleting || !catalogRowsFor(ids).some((r) => !!r.deleted_at)
			}
		],
		fields,
		load: async ({ page, pageSize, q, filter, sort }) => {
			const params = new URLSearchParams();
			params.set('page', String(page + 1));
			params.set('limit', String(pageSize));
			if (filter.is_published) params.set('is_published', filter.is_published);
			if (filter.deleted) params.set('deleted', filter.deleted);
			if (q) params.set('q', q);
			if (sort) {
				const [key] = sort.split(':');
				if (key === 'title') params.set('sort', 'title');
			}
			const result = await courseCatalogAdminApi.list(params.toString());
			if (result.ok) {
				const data = result.data.data ?? [];
				for (const r of data) catalogRowMap.set(r.id, r);
				const total = result.data.meta?.total ?? data.length;
				const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load course catalog'));
		},
		create: async (body) => {
			const res = await courseCatalogAdminApi.create({
				title: String(body.title ?? '').trim(),
				slug: String(body.slug ?? '').trim() || undefined,
				description: body.description ? String(body.description) : undefined,
				degree_type: body.degree_type ? String(body.degree_type) : undefined,
				department: body.department ? String(body.department) : undefined,
				field_of_study: body.field_of_study ? String(body.field_of_study) : undefined,
				language: body.language ? String(body.language) : undefined,
				default_duration_years: body.default_duration_years ? Number(body.default_duration_years) : undefined,
				is_published: body.is_published === undefined ? true : !!body.is_published
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await courseCatalogAdminApi.update(id, {
				title: String(body.title ?? '').trim(),
				slug: String(body.slug ?? '').trim() || undefined,
				description: body.description === undefined ? undefined : String(body.description || '') || null,
				degree_type: body.degree_type === undefined ? undefined : String(body.degree_type || '') || null,
				department: body.department === undefined ? undefined : String(body.department || '') || null,
				field_of_study: body.field_of_study === undefined ? undefined : String(body.field_of_study || '') || null,
				language: body.language ? String(body.language) : 'English',
				default_duration_years: body.default_duration_years ? Number(body.default_duration_years) : undefined,
				is_published: body.is_published === undefined ? true : !!body.is_published
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await courseCatalogAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		// Permanent removal — the scaffold shows "Hard Delete" only for rows
		// where isDeleted(row) is true (default: deleted_at != null).
		hardRemove: async (id) => {
			const res = await courseCatalogAdminApi.hardDelete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: CatalogView
	});
</script>

<svelte:head>
	<title>{t('courses.catalog_title', 'Course Catalog')} — JapaConnect Admin</title>
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

<!-- Bulk Force Delete confirmation — permanent erasure of soft-deleted rows -->
<Modal bind:open={showForceModal} title={t('common.force_delete', 'Force Delete')} size="sm">
	{#if forceTargets.length > 0}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('common.force_delete_confirm_bulk', 'Permanently delete {count} selected items? This cannot be undone.').replace('{count}', String(forceTargets.length))}
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showForceModal = false)} disabled={bulkDeleting}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmForceDelete} disabled={bulkDeleting}>{bulkDeleting ? t('common.deleting', 'Deleting...') : t('common.force_delete', 'Force Delete')}</Button>
	</div>
</Modal>

{#snippet CatalogView({ row }: { row: CourseCatalogEntry })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.title', 'Course Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.slug', 'Slug')}</p>
			<p class="mt-1 font-mono text-sm" style="color: var(--color-foreground)">{row.slug}</p>
		</div>
		{#if row.description}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.description', 'Description')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description}</p>
			</div>
		{/if}
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.degree_type', 'Degree Type')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.degree_type ?? '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.field_of_study', 'Field of Study')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.field_of_study ?? '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.department', 'Department')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.department ?? '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.default_duration', 'Default Duration')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.default_duration_years ? `${row.default_duration_years} yrs` : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.language', 'Language')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.language}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.offerings', 'Offerings')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.offering_count}</p>
			</div>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('courses.status', 'Status')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.is_published ? t('courses.published', 'Published') : t('courses.unpublished', 'Unpublished')}</p>
		</div>
	</div>
{/snippet}
