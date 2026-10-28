<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /roadmap/categories — EntityPage scaffold (§113). Simple CRUD entity
	// using declarative `fields` (EntityForm); the list endpoint returns all
	// categories unpaginated, so search/pagination are applied client-side
	// (mirrors /share-platforms).
	import { EntityPage, type EntityPageConfig, type FieldDef } from '$lib/components/entity';
	import { roadmapCategoriesAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type RoadmapCategory = {
		id: string;
		slug: string;
		name: string;
		description: string | null;
		color: string | null;
		sort_order: number;
		created_at: string;
		updated_at: string;
		deleted_at: string | null;
	};

	const fields: FieldDef[] = [
		{ key: 'slug', label: t('roadmap.slug', 'Slug'), type: 'text', help: t('roadmap.slug_hint', 'URL-safe identifier — lowercase, hyphens, no spaces.'), placeholder: 'features' },
		{ key: 'name', label: t('roadmap.name', 'Name'), type: 'text', help: t('roadmap.name_hint', 'The name shown in admin lists and on the site where public.'), placeholder: 'Features', required: true },
		{ key: 'description', label: t('roadmap.description_label', 'Description'), type: 'textarea', help: t('roadmap.description_hint', 'Shown in detail views — keep it concise.') },
		{ key: 'color', label: t('roadmap.color', 'Color'), type: 'color', help: t('roadmap.color_hint', 'Badge color shown on the roadmap.') },
		{ key: 'sort_order', label: t('roadmap.sort_order', 'Sort Order'), type: 'number', help: t('roadmap.sort_order_hint', 'Lower numbers appear first.') }
	];

	const config: EntityPageConfig = {
		title: t('roadmap.categories', 'Roadmap Categories'),
		description: t('roadmap.categories_description', 'Manage categories used to group roadmap items.'),
		createLabel: t('roadmap.create_category', 'Create Category'),
		createPermission: 'roadmap.edit',
		pageSize: 25,
		columns: [
			{
				key: 'slug',
				label: t('roadmap.slug', 'Slug'),
				render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{ key: 'name', label: t('roadmap.name', 'Name') },
			{ key: 'color', label: t('roadmap.color', 'Color'), componentSnippet: ColorCell },
			{ key: 'sort_order', label: t('roadmap.sort_order', 'Sort Order') }
		],
		fields,
		load: async ({ page, pageSize, q }) => {
			// The backend returns every category without pagination — fetch all
			// and apply client-side search/pagination (mirrors /share-platforms).
			const result = await roadmapCategoriesAdminApi.list();
			if (result.ok) {
				let data = (result.data as RoadmapCategory[]) ?? [];
				const query = (q ?? '').trim().toLowerCase();
				if (query) {
					data = data.filter((c) =>
						c.slug.toLowerCase().includes(query) ||
						c.name.toLowerCase().includes(query)
					);
				}
				const total = data.length;
				const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
				const start = page * pageSize;
				return { data: data.slice(start, start + pageSize), total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load roadmap categories'));
		},
		create: async (body) => {
			const res = await roadmapCategoriesAdminApi.create({
				slug: String(body.slug ?? '').trim(),
				name: String(body.name ?? '').trim(),
				description: String(body.description ?? '').trim(),
				color: String(body.color ?? '').trim() || undefined,
				sort_order: Number(body.sort_order ?? 0)
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await roadmapCategoriesAdminApi.update(id, {
				slug: String(body.slug ?? '').trim(),
				name: String(body.name ?? '').trim(),
				description: String(body.description ?? '').trim(),
				color: String(body.color ?? '').trim() || undefined,
				sort_order: Number(body.sort_order ?? 0)
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await roadmapCategoriesAdminApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		}
	};
</script>

<svelte:head>
	<title>{t('roadmap.categories', 'Roadmap Categories')} — {siteName()} Admin</title>
</svelte:head>

<EntityPage {config} />

{#snippet ColorCell({ value }: { value: string | null; row: RoadmapCategory })}
	{#if value}
		<div class="flex items-center gap-2">
			<span class="inline-block h-4 w-4 rounded-full border" style="background-color: {value}; border-color: var(--color-border)"></span>
			<span class="font-mono text-xs" style="color: var(--color-muted)">{value}</span>
		</div>
	{:else}
		<span style="color: var(--color-muted)">—</span>
	{/if}
{/snippet}
