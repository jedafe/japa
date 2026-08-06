<script lang="ts">
	// /widgets — migrated to the EntityPage scaffold; simple CRUD entity using `fields` (EntityForm).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { widgetsAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type Widget = {
		id: string;
		name: string;
		widget_type: string;
		config: Record<string, unknown>;
		is_active: boolean;
		created_at: string;
	};

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');

	// `fields` drives EntityForm rendering and edit-form seeding.
	const fields: FieldDef[] = [
		{ key: 'name', label: t('widgets.name', 'Name'), type: 'text', required: true },
		{ key: 'widget_type', label: t('widgets.type', 'Type'), type: 'text', required: true },
		{ key: 'is_active', label: t('widgets.active', 'Active'), type: 'checkbox' }
	];

	const config: EntityPageConfig = {
		title: t('widgets.title', 'Widgets'),
		description: t('widgets.description', 'Manage widgets available across the platform.'),
		createLabel: t('widgets.create', 'Create'),
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('widgets.name', 'Name'), sortable: true },
			{
				key: 'widget_type',
				label: t('widgets.type', 'Type'),
				sortable: true,
				render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{ key: 'is_active', label: t('widgets.status', 'Status'), componentSnippet: StatusCell }
		],
		filters: [
			{
				key: 'is_active',
				label: t('widgets.status', 'Status'),
				options: [
					{ value: 'true', label: t('widgets.active', 'Active') },
					{ value: 'false', label: t('widgets.inactive', 'Inactive') }
				]
			}
		],
		fields,
		load: async ({ q, filter }) => {
			const params = new URLSearchParams();
			if (filter.is_active) params.set('is_active', filter.is_active);
			if (q) params.set('q', q);
			const result = await widgetsAdminApiExtended.list(params.toString() || undefined);
			if (result.ok) {
				const data = (result.data as Widget[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load widgets'));
		},
		create: async (body) => {
			const res = await widgetsAdminApiExtended.create({
				name: String(body.name ?? '').trim(),
				widget_type: String(body.widget_type ?? '').trim(),
				is_active: body.is_active === undefined ? true : !!body.is_active
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await widgetsAdminApiExtended.update(id, {
				name: String(body.name ?? '').trim(),
				widget_type: String(body.widget_type ?? '').trim(),
				is_active: !!body.is_active
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await widgetsAdminApiExtended.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: WidgetView
	};
</script>

<svelte:head>
	<title>{t('widgets.title', 'Widgets')} — JapaConnect Admin</title>
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

{#snippet StatusCell({ value }: { value: boolean; row: Widget })}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {value ? 'bg-success/15 text-success' : 'bg-muted/15 text-muted'}">
		{value ? t('widgets.active', 'Active') : t('widgets.inactive', 'Inactive')}
	</span>
{/snippet}

<!-- View slide-over body -->
{#snippet WidgetView({ row }: { row: Widget })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('widgets.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('widgets.type', 'Type')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.widget_type}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('widgets.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_active ? t('widgets.active', 'Active') : t('widgets.inactive', 'Inactive')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.created_at', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ?? '—'}</p>
		</div>
	</div>
{/snippet}
