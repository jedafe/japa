<script lang="ts">
	// /community/scam-alerts — admin HITL review queue for Scam Alerts with pin/unpin.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { scamAlertAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Pin, PinOff } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type ScamAlertItem = {
		id: string;
		title: string;
		severity: string;
		status: string;
		created_at: string;
		is_pinned: boolean;
		description?: string;
	};

	const canManage = $derived(hasPermission('scam_alert.review'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);

	async function pinAlert(item: ScamAlertItem) {
		acting = true;
		pageError = '';
		try {
			const result = await scamAlertAdminApi.pin(item.id);
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

	async function unpinAlert(item: ScamAlertItem) {
		acting = true;
		pageError = '';
		try {
			const result = await scamAlertAdminApi.unpin(item.id);
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
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: ScamAlertItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('app.scam_alert.pin', 'Pin'),
				icon: Pin,
				onclick: pinAlert,
				show: (row: ScamAlertItem) => !row.is_pinned
			});
			acts.push({
				label: t('app.scam_alert.unpin', 'Unpin'),
				icon: PinOff,
				onclick: unpinAlert,
				show: (row: ScamAlertItem) => !!row.is_pinned
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'title', label: t('app.scam_alert.col_title', 'Title'), type: 'text' },
		{ key: 'severity', label: t('app.scam_alert.col_severity', 'Severity'), type: 'text' },
		{ key: 'status', label: t('app.scam_alert.col_status', 'Status'), type: 'text' },
		{ key: 'description', label: t('app.scam_alert.detail_description', 'Description'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.scam_alert.review_queue_heading', 'Scam Alerts Review Queue'),
		description: t('app.scam_alert.review_queue_desc', 'Review and pin scam alerts for the community'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('app.scam_alert.col_title', 'Title'), sortable: true },
			{
				key: 'severity',
				label: t('app.scam_alert.col_severity', 'Severity'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'status',
				label: t('app.scam_alert.col_status', 'Status'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'created_at',
				label: t('app.scam_alert.col_created_at', 'Created'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async () => {
			const result = await scamAlertAdminApi.list();
			if (result.ok) {
				const data = (result.data as ScamAlertItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: ScamAlertView
	});
</script>

<svelte:head>
	<title>{t('app.scam_alert.review_queue_page_title', 'Scam Alerts Review Queue — JapaConnect Admin')}</title>
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
{#snippet ScamAlertView({ row }: { row: ScamAlertItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.scam_alert.col_title', 'Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.scam_alert.col_severity', 'Severity')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.severity || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.scam_alert.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.scam_alert.col_created_at', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.scam_alert.detail_pinned', 'Pinned')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_pinned ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.scam_alert.detail_description', 'Description')}</p>
			<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
		</div>
	</div>
{/snippet}
