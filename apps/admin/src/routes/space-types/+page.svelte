<script lang="ts">
	// /space-types — migrated to the EntityPage scaffold; simple CRUD entity using `fields` (EntityForm).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { spaceTypesApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Trash2 } from 'lucide-svelte';

	type SpaceType = {
		id: string;
		name: string;
		slug: string;
		description: string;
		is_university: boolean;
		icon: string;
	};

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
			await Promise.all(idArr.map((id) => spaceTypesApi.delete(id)));
			// EntityPage auto-reloads + clears selection after this returns.
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('app.space_types.failed_to_delete_selected', 'Failed to delete selected items');
		} finally {
			bulkDeleting = false;
		}
	}

	function slugify(text: string): string {
		return text
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

	// `fields` drives EntityForm rendering and edit-form seeding.
	const fields: FieldDef[] = [
		{ key: 'name', label: t('space-types.name', 'Name'), type: 'text', placeholder: 'University', required: true },
		{ key: 'slug', label: t('space-types.slug', 'Slug'), type: 'text', placeholder: 'university', help: t('app.space_types.slug_hint', 'Leave blank to auto-generate from name.'), readonlyInEdit: true },
		{ key: 'description', label: t('space-types.description', 'Description'), type: 'textarea', placeholder: '' },
		{ key: 'icon', label: t('space-types.icon_emoji_or_name', 'Icon (emoji or name)'), type: 'text', placeholder: '🎓' },
		{ key: 'is_university', label: t('space-types.university_type', 'University type'), type: 'checkbox' }
	];

	const config: EntityPageConfig = {
		title: t('app.space_types.page_title', 'Space Types'),
		description: t('space-types.manage_space_type_categories_including_u', 'Manage space type categories including university types.'),
		createLabel: t('app.space_types.add_space_type', 'Add Space Type'),
		createPermission: 'space_types.create',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('space-types.name', 'Name'), sortable: true },
			{
				key: 'slug',
				label: t('space-types.slug', 'Slug'),
				render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{
				key: 'description',
				label: t('space-types.description', 'Description'),
				render: (v: string) => (v ? `<span class="text-sm">${v}</span>` : '—')
			},
			{ key: 'is_university', label: t('space-types.university', 'University'), componentSnippet: UniversityCell },
			{ key: 'icon', label: t('space-types.icon', 'Icon'), componentSnippet: IconCell }
		],
		filters: [
			{
				key: 'is_university',
				label: t('app.space_types.filter_university', 'University'),
				options: [
					{ value: 'true', label: t('app.space_types.filter_yes', 'Yes') },
					{ value: 'false', label: t('app.space_types.filter_no', 'No') }
				]
			}
		],
		bulkActions: [
			{
				label: t('app.space_types.delete_selected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ q, filter }) => {
			const params = new URLSearchParams();
			if (filter.is_university) params.set('is_university', filter.is_university);
			if (q) params.set('q', q);
			const result = await spaceTypesApi.list(params.toString() || undefined);
			if (result.ok) {
				const d = result.data as SpaceType[] | { items: SpaceType[]; total_pages: number };
				const data = Array.isArray(d) ? d : (d.items ?? []);
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load space types'));
		},
		create: async (body) => {
			const slug = String(body.slug ?? '').trim() || slugify(String(body.name ?? ''));
			const res = await spaceTypesApi.create({
				name: String(body.name ?? '').trim(),
				slug,
				description: String(body.description ?? '').trim(),
				is_university: !!body.is_university,
				icon: String(body.icon ?? '').trim()
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const slug = String(body.slug ?? '').trim() || slugify(String(body.name ?? ''));
			const res = await spaceTypesApi.update(id, {
				name: String(body.name ?? '').trim(),
				slug,
				description: String(body.description ?? '').trim(),
				is_university: !!body.is_university,
				icon: String(body.icon ?? '').trim()
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await spaceTypesApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: SpaceTypeView
	};
</script>

<svelte:head>
	<title>{t('app.space_types.page_title', 'Space Types')} — JapaConnect Admin</title>
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

{#snippet IconCell({ value }: { value: any; row: SpaceType })}
	<span class="text-lg">{value || '—'}</span>
{/snippet}

{#snippet UniversityCell({ value }: { value: boolean; row: SpaceType })}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {value ? 'bg-info/15 text-info' : 'bg-muted/15 text-muted'}">
		{value ? t('app.space_types.yes', 'Yes') : t('app.space_types.no', 'No')}
	</span>
{/snippet}

<!-- View slide-over body (mirrors the legacy view modal) -->
{#snippet SpaceTypeView({ row }: { row: SpaceType })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.space_types.name_label', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.space_types.slug_label', 'Slug')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.space_types.description_label', 'Description')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.space_types.university_label', 'University')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_university ? t('app.space_types.yes', 'Yes') : t('app.space_types.no', 'No')}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.space_types.icon_label', 'Icon')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.icon || '—'}</p>
			</div>
		</div>
	</div>
{/snippet}
