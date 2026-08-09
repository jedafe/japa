<script lang="ts">
	// /tools/visa-routes — admin HITL review queue for AI-generated visa route guides.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { visaRoutesAdminApi } from '$lib/api';

	type VisaRouteItem = {
		id: string;
		name: string;
		origin_country: string;
		dest_country: string;
		visa_type: string;
		status: string;
		ai_generated: boolean;
		summary?: string;
		requirements?: string;
		processing_time?: string;
		cost?: string;
		updated_at?: string;
	};

	const canManage = $derived(hasPermission('visa_routes.manage'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);

	async function approveItem(item: VisaRouteItem) {
		acting = true;
		pageError = '';
		try {
			const result = await visaRoutesAdminApi.approve(item.id);
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
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: VisaRouteItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('common.approve', 'Approve'),
				icon: CheckCircle,
				onclick: approveItem,
				show: () => true
			});
		}
		return acts;
	});

	// `fields` is required by EntityPageConfig; readonly page so the form is never shown, but kept for shape completeness.
	const fields: FieldDef[] = [
		{ key: 'name', label: t('app.tools.col_name', 'Name'), type: 'text' },
		{ key: 'origin_country', label: t('app.tools.col_origin_country', 'Origin Country'), type: 'text' },
		{ key: 'dest_country', label: t('app.tools.col_dest_country', 'Destination Country'), type: 'text' },
		{ key: 'visa_type', label: t('app.tools.col_visa_type', 'Visa Type'), type: 'text' },
		{ key: 'summary', label: t('app.tools.col_summary', 'Summary'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.visa_routes_heading', 'Visa Routes Review Queue'),
		description: t('app.tools.visa_routes_desc', 'Review and approve AI-generated visa route guides'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('app.tools.col_name', 'Name'), sortable: true },
			{ key: 'origin_country', label: t('app.tools.col_origin_country', 'Origin'), sortable: true },
			{ key: 'dest_country', label: t('app.tools.col_dest_country', 'Destination'), sortable: true },
			{ key: 'visa_type', label: t('app.tools.col_visa_type', 'Visa Type'), sortable: true },
			{
				key: 'status',
				label: t('app.tools.col_status', 'Status'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'ai_generated',
				label: t('app.tools.col_ai_generated', 'AI Generated'),
				render: (value: boolean) => (value ? t('common.yes', 'Yes') : t('common.no', 'No'))
			}
		],
		actions: rowActions,
		fields,
		load: async () => {
			const result = await visaRoutesAdminApi.list();
			if (result.ok) {
				const data = (result.data as VisaRouteItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: VisaRouteView
	});
</script>

<svelte:head>
	<title>{t('app.tools.visa_routes_page_title', 'Visa Routes Review Queue — JapaConnect Admin')}</title>
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
{#snippet VisaRouteView({ row }: { row: VisaRouteItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_origin_country', 'Origin Country')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.origin_country || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_dest_country', 'Destination Country')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.dest_country || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_visa_type', 'Visa Type')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.visa_type || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_ai_generated', 'AI Generated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.ai_generated ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_summary', 'Summary')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.summary || '—'}</p>
		</div>
	</div>
{/snippet}
