<script lang="ts">
	// /tools/appointment-alert — admin management for embassy appointment alerter (§91).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, RefreshCw, Building } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { appointmentAlertAdminApi } from '$lib/api';

	type EmbassyItem = {
		id: string;
		name: string;
		country: string;
		city: string;
		website?: string;
		booking_url?: string;
		polling_enabled?: boolean;
		last_polled_at?: string;
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
				label: t('app.tools.poll_now', 'Poll Now'),
				icon: RefreshCw,
				onclick: pollEmbassy,
				show: () => true
			});
		}
		return acts;
	});

	// `fields` is required by EntityPageConfig; readonly page so the form is never shown.
	const fields: FieldDef[] = [
		{ key: 'name', label: t('app.tools.col_name', 'Name'), type: 'text' },
		{ key: 'country', label: t('app.tools.col_country', 'Country'), type: 'text' },
		{ key: 'city', label: t('app.tools.col_city', 'City'), type: 'text' },
		{ key: 'website', label: t('app.tools.col_website', 'Website'), type: 'url' },
		{ key: 'booking_url', label: t('app.tools.col_booking_url', 'Booking URL'), type: 'url' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.appointment_alert_heading', 'Embassy Appointment Alerter'),
		description: t('app.tools.appointment_alert_desc', 'Manage embassies and trigger availability polling'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('app.tools.col_name', 'Name'), sortable: true },
			{ key: 'country', label: t('app.tools.col_country', 'Country'), sortable: true },
			{ key: 'city', label: t('app.tools.col_city', 'City'), sortable: true },
			{
				key: 'polling_enabled',
				label: t('app.tools.col_polling', 'Polling'),
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
		load: async () => {
			const result = await appointmentAlertAdminApi.listEmbassies();
			if (result.ok) {
				const data = (result.data as EmbassyItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
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
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_country', 'Country')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.country || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_city', 'City')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.city || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_website', 'Website')}</p>
			<p class="mt-1 break-all text-sm" style="color: var(--color-foreground)">{row.website || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_booking_url', 'Booking URL')}</p>
			<p class="mt-1 break-all text-sm" style="color: var(--color-foreground)">{row.booking_url || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_polling', 'Polling')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.polling_enabled ? t('common.enabled', 'Enabled') : t('common.disabled', 'Disabled')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_last_polled', 'Last Polled')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.last_polled_at ? new Date(row.last_polled_at).toLocaleString() : '—'}</p>
		</div>
	</div>
{/snippet}
