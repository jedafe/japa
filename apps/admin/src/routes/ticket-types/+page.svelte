<script lang="ts">
	// /ticket-types — migrated to the EntityPage scaffold; simple server-paginated CRUD entity using `fields` (EntityForm).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { ticketTypesApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Trash2 } from 'lucide-svelte';

	type TicketType = {
		id: string;
		name: string;
		slug: string;
		description: string;
		is_public: boolean;
		is_system: boolean;
		default_priority: string;
		field_group_id?: string;
		auto_assign_group_id?: string;
		sla_hours?: number;
		created_at: string;
		updated_at: string;
	};

	const priorities = ['low', 'medium', 'high', 'urgent'];

	function slugify(text: string): string {
		return text
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

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
			await Promise.all(idArr.map((id) => ticketTypesApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('app.ticket_types.failed_to_delete_selected', 'Failed to delete selected items');
		} finally {
			bulkDeleting = false;
		}
	}

	// `fields` drives EntityForm rendering and edit-form seeding.
	const fields: FieldDef[] = [
		{ key: 'name', label: t('ticket-types.name', 'Name'), type: 'text', placeholder: 'Bug Report', required: true },
		{ key: 'slug', label: t('ticket-types.slug', 'Slug'), type: 'text', placeholder: 'bug-report' },
		{ key: 'description', label: t('ticket-types.description', 'Description'), type: 'textarea' },
		{
			key: 'default_priority',
			label: t('ticket-types.default_priority', 'Default Priority'),
			type: 'select',
			options: priorities.map((p) => ({ value: p, label: p }))
		},
		{ key: 'is_public', label: t('ticket-types.active', 'Active'), type: 'checkbox' }
	];

	const config: EntityPageConfig = {
		title: t('app.ticket_types.page_title', 'Ticket Types'),
		description: t('ticket-types.manage_ticket_type_categories_and_their_', 'Manage ticket type categories and their defaults.'),
		createLabel: t('app.ticket_types.add_ticket_type', 'Add Ticket Type'),
		createPermission: 'tickets.review',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('ticket-types.name', 'Name'), sortable: true },
			{
				key: 'slug',
				label: t('ticket-types.slug', 'Slug'),
				sortable: true,
				render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{
				key: 'description',
				label: t('ticket-types.description', 'Description'),
				render: (v: string) => `<span style="color: var(--color-muted)">${v || '—'}</span>`
			},
			{ key: 'is_public', label: t('ticket-types.active', 'Active'), sortable: true, componentSnippet: StatusCell },
			{
				key: 'default_priority',
				label: t('ticket-types.priority', 'Priority'),
				sortable: true,
				render: (v: string) => {
					const cls = v === 'urgent' ? 'text-danger dark:text-danger' : v === 'high' ? 'text-warning dark:text-warning' : '';
					return `<span class="text-xs font-medium ${cls}">${v}</span>`;
				}
			}
		],
		filters: [
			{
				key: 'is_public',
				label: t('app.ticket_types.filter_status', 'Status'),
				options: [
					{ value: 'true', label: t('app.ticket_types.filter_active', 'Active') },
					{ value: 'false', label: t('app.ticket_types.filter_inactive', 'Inactive') }
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
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (filter.is_public) params.set('is_public', filter.is_public);
			if (q) params.set('q', q);
			const result = await ticketTypesApi.list(params.toString() || undefined);
			if (result.ok) {
				const data = result.data as { items: TicketType[]; total: number; page: number; total_pages: number };
				return {
					data: data.items ?? [],
					total: data.total ?? 0,
					totalPages: data.total_pages >= 0 ? data.total_pages : 1
				};
			}
			throw new Error(result.error ?? t('common.load_error', 'Failed to load'));
		},
		create: async (body) => {
			const slug = String(body.slug ?? '').trim() || slugify(String(body.name ?? '').trim());
			const res = await ticketTypesApi.create({
				name: String(body.name ?? '').trim(),
				slug,
				description: String(body.description ?? '').trim(),
				default_priority: String(body.default_priority ?? 'medium'),
				is_public: body.is_public === undefined ? true : !!body.is_public
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const slug = String(body.slug ?? '').trim() || slugify(String(body.name ?? '').trim());
			const res = await ticketTypesApi.update(id, {
				name: String(body.name ?? '').trim(),
				slug,
				description: String(body.description ?? '').trim(),
				default_priority: String(body.default_priority ?? 'medium'),
				is_public: body.is_public === undefined ? true : !!body.is_public
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await ticketTypesApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: TicketTypeView
	};
</script>

<svelte:head>
	<title>{t('app.ticket_types.page_title', 'Ticket Types')} — JapaConnect Admin</title>
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

{#snippet StatusCell({ value }: { value: boolean; row: TicketType })}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {value ? 'bg-success/15 text-success' : 'bg-muted/15 text-muted'}">
		{value ? t('app.ticket_types.status_active', 'Active') : t('app.ticket_types.status_inactive', 'Inactive')}
	</span>
{/snippet}

<!-- View slide-over body -->
{#snippet TicketTypeView({ row }: { row: TicketType })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('ticket-types.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('ticket-types.slug', 'Slug')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('ticket-types.description', 'Description')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('ticket-types.default_priority', 'Default Priority')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.default_priority}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('ticket-types.active', 'Active')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_public ? t('app.ticket_types.status_active', 'Active') : t('app.ticket_types.status_inactive', 'Inactive')}</p>
		</div>
	</div>
{/snippet}
