<script lang="ts">
	// /professional-types — migrated to the EntityPage scaffold; simple CRUD entity using `fields` (EntityForm).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { api, professionalAdminApi, type ProfileType } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Trash2 } from 'lucide-svelte';

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
			await Promise.all(idArr.map((id) => professionalAdminApi.deleteType(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete professional types');
		} finally {
			bulkDeleting = false;
		}
	}

	// `fields` drives EntityForm rendering and edit-form seeding.
	const fields: FieldDef[] = [
		{ key: 'name', label: t('professional_types.name', 'Name'), type: 'text', required: true },
		{ key: 'slug', label: t('professional_types.slug', 'Slug'), type: 'text' },
		{ key: 'description', label: t('professional_types.description', 'Description'), type: 'textarea' }
	];

	const config: EntityPageConfig = {
		title: t('professional_types.title', 'Professional Types'),
		description: t('professional_types.description', 'Manage professional profile types, members, and invitations'),
		createLabel: t('professional_types.add', 'Add Profile Type'),
		createPermission: 'professional_profile.type.create',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('professional_types.name', 'Name'), sortable: true },
			{ key: 'slug', label: t('professional_types.slug', 'Slug'), class: 'font-mono text-xs' },
			{
				key: 'description',
				label: t('professional_types.description_label', 'Description'),
				render: (v: string) => v || '—'
			},
			{ key: 'profiles', label: t('professional_types.profiles', 'Profiles'), render: () => '—' }
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
		load: async ({ page, pageSize, q, filter }) => {
			const result = await api<ProfileType[]>('/profiles/types?page_size=1000');
			if (result.ok) {
				let items = (result.data as ProfileType[]) ?? [];
				const query = (q ?? '').toLowerCase();
				if (query) {
					items = items.filter((pt) =>
						[pt.name, pt.slug, pt.description].some((v) =>
							String(v ?? '').toLowerCase().includes(query)
						)
					);
				}
				filter;
				const size = pageSize || 25;
				const offset = (page || 0) * size;
				const total = items.length;
				const totalPages = Math.max(1, Math.ceil(total / size));
				return {
					data: items.slice(offset, offset + size),
					total,
					totalPages
				};
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load professional types'));
		},
		create: async (body) => {
			const res = await professionalAdminApi.createType({
				name: String(body.name ?? '').trim(),
				slug: String(body.slug ?? '').trim(),
				description: String(body.description ?? '').trim()
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await professionalAdminApi.updateType(id, {
				name: String(body.name ?? '').trim(),
				slug: String(body.slug ?? '').trim(),
				description: String(body.description ?? '').trim()
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await professionalAdminApi.deleteType(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: TypeView
	};
</script>

<svelte:head>
	<title>{t('professional_types.title', 'Professional Types')} — JapaConnect Admin</title>
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

<!-- View slide-over body (mirrors the legacy view modal) -->
{#snippet TypeView({ row }: { row: ProfileType })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('professional_types.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('professional_types.slug', 'Slug')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('professional_types.description_label', 'Description')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
		</div>
	</div>
{/snippet}
