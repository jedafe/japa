<script lang="ts">
	// /ads/slot-types — migrated to the EntityPage scaffold; declarative CRUD entity.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { adSlotTypesApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Trash2 } from 'lucide-svelte';

	type SlotType = {
		id: string;
		slot_name: string;
		description?: string | null;
		is_active?: boolean;
		max_display_per_page?: number;
		created_at: string;
		updated_at: string;
	};

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
			await Promise.all(idArr.map((id) => adSlotTypesApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete slot types');
		} finally {
			bulkDeleting = false;
		}
	}

	// `slot_name` is the display/storage key; the API create/update body expects `name`.
	const fields: FieldDef[] = [
		{ key: 'slot_name', label: t('slot_types.name', 'Name'), type: 'text', placeholder: t('slot_types.name_placeholder', 'e.g. Leaderboard Banner'), required: true },
		{ key: 'description', label: t('slot_types.description', 'Description'), type: 'textarea', placeholder: t('slot_types.desc_placeholder', 'Optional description for this slot type') },
		{ key: 'max_display_per_page', label: t('slot_types.max_display', 'Max Display Per Page'), type: 'number', min: 0, placeholder: t('slot_types.max_display_placeholder', 'Unlimited') },
		{ key: 'is_active', label: t('slot_types.active', 'Active'), type: 'checkbox' }
	];

	const config: EntityPageConfig = {
		title: t('slot_types.title', 'Slot Types'),
		description: t('slot_types.description', 'Manage ad placement slot types — define dimensions, display limits, and availability'),
		createLabel: t('slot_types.create', 'Create Slot Type'),
		// Ref: japaconnect-engineering/sections/25-ad-placement.md — the
		// backend GET/POST/PUT/DELETE /admin/ads/slot-types* routes all
		// require `ad.slot.manage` (server.go). Gate the UI with the same
		// node so an authorized user sees the controls and an unauthorized
		// one does not get a 403 on submit.
		createPermission: 'ad.slot.manage',
		editPermission: 'ad.slot.manage',
		deletePermission: 'ad.slot.manage',
		pageSize: 25,
		columns: [
			{
				key: 'slot_name',
				label: t('slot_types.col_name', 'Name'),
				sortable: true,
				render: (v: string) => `<span class="font-medium">${v ?? '—'}</span>`
			},
			{
				key: 'description',
				label: t('slot_types.col_description', 'Description'),
				render: (v: string) => `<span class="text-xs text-muted">${v ?? '—'}</span>`
			},
			{
				key: 'max_display_per_page',
				label: t('slot_types.col_max_display', 'Max Display'),
				sortable: true,
				render: (v: number) => (v != null ? String(v) : '—')
			},
			{
				key: 'is_active',
				label: t('common.status', 'Status'),
				sortable: true,
				render: (v: boolean) =>
					v
						? `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold bg-success/15 text-success border-success">${t('common.active', 'Active')}</span>`
						: `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold bg-surface text-foreground border-border">${t('common.inactive', 'Inactive')}</span>`
			}
		],
		filters: [
			{
				key: 'is_active',
				label: t('common.status', 'Status'),
				options: [
					{ value: 'true', label: t('common.active', 'Active') },
					{ value: 'false', label: t('common.inactive', 'Inactive') }
				]
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
		load: async ({ page, pageSize, q, filter }) => {
			// The backend returns every slot type without pagination,
			// so we fetch a large window and apply client-side search/filter/pagination.
			const result = await adSlotTypesApi.list();
			if (result.ok) {
				const resp = result.data as Record<string, unknown> | undefined;
				let data = (Array.isArray(resp) ? resp : (resp?.data as SlotType[]) ?? []) as SlotType[];
				if (filter.is_active) {
					const active = filter.is_active === 'true';
					data = data.filter((s) => s.is_active === active);
				}
				const query = (q ?? '').trim().toLowerCase();
				if (query) {
					data = data.filter((s) =>
						s.slot_name.toLowerCase().includes(query) ||
						(s.description ?? '').toLowerCase().includes(query)
					);
				}
				const total = data.length;
				const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
				const start = page * pageSize;
				return { data: data.slice(start, start + pageSize), total, totalPages };
			}
			throw new Error(result.error ?? t('slot_types.load_error', 'Failed to load slot types'));
		},
		create: async (body) => {
			const payload: Record<string, unknown> = { name: String(body.slot_name ?? '').trim() };
			if (body.description != null && String(body.description).trim()) payload.description = String(body.description).trim();
			if (body.max_display_per_page != null && body.max_display_per_page !== '') payload.max_display_per_page = Number(body.max_display_per_page);
			payload.is_active = !!body.is_active;
			const res = await adSlotTypesApi.create(payload as any);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const payload: Record<string, unknown> = { name: String(body.slot_name ?? '').trim() };
			if (body.description != null && String(body.description).trim()) payload.description = String(body.description).trim();
			if (body.max_display_per_page != null && body.max_display_per_page !== '') payload.max_display_per_page = Number(body.max_display_per_page);
			payload.is_active = !!body.is_active;
			const res = await adSlotTypesApi.update(id, payload as any);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await adSlotTypesApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: SlotTypeView
	};
</script>

<svelte:head>
	<title>{t('slot_types.title', 'Slot Types')} — JapaConnect Admin</title>
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

{#snippet SlotTypeView({ row }: { row: SlotType })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('slot_types.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.slot_name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('slot_types.description', 'Description')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description ?? '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('slot_types.max_display', 'Max Display Per Page')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.max_display_per_page ?? '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_active ? t('common.active', 'Active') : t('common.inactive', 'Inactive')}</p>
		</div>
	</div>
{/snippet}
