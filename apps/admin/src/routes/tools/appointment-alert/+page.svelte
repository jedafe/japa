<script lang="ts">
	// /tools/appointment-alert — admin management for embassy appointment alerter (§91, ADR-099).
	// Full CRUD for monitored embassies + immediate poll trigger.
	// Ref: japaconnect-engineering/sections/91-embassy-appointment-alerter.md
	import { onMount } from 'svelte';
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, RefreshCw, Pencil } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';
	import { appointmentAlertAdminApi , spacesAdminApi } from '$lib/api';

	// EmbassyDTO — matches the backend EmbassyDTO shape (§91).
	// Ref: internal/appointmentalert/service.go EmbassyDTO
	type EmbassyItem = {
		id: string;
		destination_space_id: string;
		destination_space_name?: string | null;
		slug: string;
		name: string;
		booking_url: string;
		poll_interval_seconds: number;
		is_active: boolean;
		last_polled_at?: string | null;
		created_at?: string;
		updated_at?: string;
	};

	const canManage = $derived(hasPermission('embassy_monitor.manage'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);

	// ── Poll action ─────────────────────────────────────────────────────────
	async function pollEmbassy(item: EmbassyItem) {
		acting = true;
		pageError = '';
		try {
			const result = await appointmentAlertAdminApi.pollEmbassy(item.id);
			if (result.ok) {
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: EmbassyItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('common.edit', 'Edit'),
				icon: Pencil,
				onclick: (_row: EmbassyItem, ctx) => ctx?.openEdit(_row)
			});
			acts.push({
				label: t('app.tools.poll_now', 'Poll Now'),
				icon: RefreshCw,
				onclick: pollEmbassy,
				show: () => true
			});
		}
		return acts;
	});

	// ── Form fields (drives EntityForm for create/edit) ─────────────────────
	const fields: FieldDef[] = [
		{ key: 'slug', label: t('app.tools.col_slug', 'Slug'), type: 'text' },
		{ key: 'name', label: t('app.tools.col_name', 'Name'), type: 'text' },
		{ key: 'destination_space_id', label: t('app.tools.col_destination', 'Destination Space ID'), type: 'text' },
		{ key: 'booking_url', label: t('app.tools.col_booking_url', 'Booking URL'), type: 'url' },
		{ key: 'poll_interval_seconds', label: t('app.tools.col_poll_interval', 'Poll Interval (s)'), type: 'number' },
		{ key: 'is_active', label: t('app.tools.col_is_active', 'Active'), type: 'checkbox' }
	];

	
// Destination Spaces for the dest filter (geo-aware pattern).
type SpaceOption = { id: string; name: string };
let destOptions = $state<SpaceOption[]>([]);
onMount(async () => {
	try {
		const res = await spacesAdminApi.list('?level=country&limit=200');
		if (res.ok && Array.isArray(res.data)) {
			destOptions = (res.data as Array<{ id: string; name: string }>).map((s) => ({
				id: s.id, name: s.name
			}));
		}
	} catch { /* Non-fatal */ }
});

		const filters = $derived<FilterDef[]>([
		{
			key: 'status',
			label: t('app.tools.col_status', 'Status'),
			options: [
				{ value: 'active', label: 'Active' },
				{ value: 'inactive', label: 'Inactive' }
			]
		},
		{
			key: 'dest',
			label: t('app.tools.col_destination', 'Destination'),
			options: destOptions.map((s) => ({ value: s.id, label: s.name }))
		}
	]);

	const config = $derived<EntityPageConfig>({
		filters,
		title: t('app.tools.appointment_alert_heading', 'Embassy Appointment Alerter'),
		description: t('app.tools.appointment_alert_desc', 'Manage monitored embassies and trigger availability polling'),
		createLabel: t('app.tools.add_embassy', 'Add Embassy'),
		createPermission: 'embassy_monitor.manage',
		editPermission: 'embassy_monitor.manage',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('app.tools.col_name', 'Name'), sortable: true },
			{ key: 'slug', label: t('app.tools.col_slug', 'Slug'), sortable: true },
			{
				key: 'destination_space_name',
				label: t('app.tools.col_destination', 'Destination'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'poll_interval_seconds',
				label: t('app.tools.col_poll_interval', 'Poll Interval'),
				render: (value: number) => `${value}s`
			},
			{
				key: 'is_active',
				label: t('app.tools.col_is_active', 'Active'),
				render: (value: boolean) => (value ? t('common.enabled', 'Enabled') : t('common.disabled', 'Disabled'))
			},
			{
				key: 'last_polled_at',
				label: t('app.tools.col_last_polled', 'Last Polled'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			// Build server-side pagination query params from EntityPage params.
			// The backend accepts ?page=&limit=&dest=&active= (§91).
			const qp = new URLSearchParams();
			qp.set('page', String(params.page ?? 1));
			qp.set('limit', String(params.pageSize ?? 25));
			if (params.q) qp.set('q', params.q);
			// Filter: pass through any filter keys (e.g. ?dest=, ?active=).
			if (params.filter) {
				for (const [k, v] of Object.entries(params.filter)) {
					if (v != null && v !== '') qp.set(k, String(v));
				}
			}
			const result = await appointmentAlertAdminApi.listEmbassies(qp.toString());
			if (result.ok) {
				const data = (result.data.data as EmbassyItem[]) ?? [];
				const total = result.data.pagination?.total ?? data.length;
				const limit = result.data.pagination?.limit ?? params.pageSize ?? 25;
				const totalPages = limit > 0 ? Math.max(1, Math.ceil(total / limit)) : 1;
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		create: async (body) => {
			const result = await appointmentAlertAdminApi.createEmbassy(body as Record<string, unknown>);
			return result.ok ? { ok: true } : { ok: false, error: result.error };
		},
		update: async (id, body) => {
			const result = await appointmentAlertAdminApi.updateEmbassy(id, body as Record<string, unknown>);
			return result.ok ? { ok: true } : { ok: false, error: result.error };
		},
		viewSnippet: EmbassyView
	});
</script>

<svelte:head>
	<title>{t('app.tools.appointment_alert_page_title', 'Embassy Appointment Alerter — JapaConnect Admin')}</title>
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

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet EmbassyView({ row }: { row: EmbassyItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_slug', 'Slug')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.slug}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_destination', 'Destination')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.destination_space_name || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_booking_url', 'Booking URL')}</p>
			<p class="mt-1 break-all text-sm" style="color: var(--color-foreground)">{row.booking_url || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_poll_interval', 'Poll Interval')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.poll_interval_seconds}s</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_is_active', 'Active')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_active ? t('common.enabled', 'Enabled') : t('common.disabled', 'Disabled')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_last_polled', 'Last Polled')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.last_polled_at ? new Date(row.last_polled_at).toLocaleString() : '—'}</p>
		</div>
	</div>
{/snippet}
