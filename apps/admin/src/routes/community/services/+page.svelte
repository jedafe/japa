<script lang="ts">
	// /community/services — admin HITL review queue for service directory listings.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { serviceDirectoryAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type ServiceItem = {
		id: string;
		name: string;
		category: string;
		status: string;
		verified: boolean;
		created_at: string;
		description?: string;
	};

	const canManage = $derived(hasPermission('service.verify'));

	// ── State ────────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);

	async function verifyService(item: ServiceItem) {
		acting = true;
		pageError = '';
		try {
			const result = await serviceDirectoryAdminApi.verify(item.id);
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
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: ServiceItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('app.services.verify', 'Verify'),
				icon: CheckCircle,
				onclick: verifyService,
				show: (row: ServiceItem) => !row.verified
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'name', label: t('app.services.col_name', 'Name'), type: 'text' },
		{ key: 'category', label: t('app.services.col_category', 'Category'), type: 'text' },
		{ key: 'status', label: t('app.services.col_status', 'Status'), type: 'text' },
		{ key: 'description', label: t('app.services.detail_description', 'Description'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.services.review_queue_heading', 'Service Directory Review Queue'),
		description: t('app.services.review_queue_desc', 'Verify community service directory listings'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('app.services.col_name', 'Name'), sortable: true },
			{ key: 'category', label: t('app.services.col_category', 'Category'), sortable: true },
			{ key: 'status', label: t('app.services.col_status', 'Status'), sortable: true },
			{
				key: 'verified',
				label: t('app.services.col_verified', 'Verified'),
				sortable: true,
				render: (value: boolean) => (value ? t('app.services.yes', 'Yes') : t('app.services.no', 'No'))
			},
			{
				key: 'created_at',
				label: t('app.services.col_created', 'Created'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async () => {
			const result = await serviceDirectoryAdminApi.list();
			if (result.ok) {
				const data = (result.data as ServiceItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: ServiceView
	});
</script>

<svelte:head>
	<title>{t('app.services.review_queue_page_title', 'Service Directory Review Queue — JapaConnect Admin')}</title>
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
{#snippet ServiceView({ row }: { row: ServiceItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.services.col_name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.services.col_category', 'Category')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.category || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.services.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.services.col_verified', 'Verified')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.verified ? t('app.services.yes', 'Yes') : t('app.services.no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.services.col_created', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.services.detail_description', 'Description')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
		</div>
	</div>
{/snippet}
