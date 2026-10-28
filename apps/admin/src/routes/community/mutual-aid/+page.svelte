<script lang="ts">
	// /community/mutual-aid — admin HITL review queue for Mutual Aid requests with status update.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { mutualAidAdminApi, adminCatalogApi, type CatalogOption } from '$lib/api';
	import { useCountries } from '$lib/utils/countries.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, RefreshCw } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	type MutualAidItem = {
		id: string;
		title: string;
		type: string;
		status: string;
		created_at: string;
		description?: string;
		// ADR-088 AI generation columns
		ai_generated?: boolean;
		ai_model?: string;
		hitl_reviewed_by?: string;
		ai_generated_at?: string;
	};

	const canManage = $derived(hasPermission('mutual_aid.moderate'));

	// ── Country selector (250 primary geo space countries) ───────────────────
	const countryUtils = useCountries();
	$effect(() => {
		countryUtils.load();
	});

	let statusOptions = $state<CatalogOption[]>([
		{ value: 'active', label: t('mutual_aid.status.active', 'Active') },
		{ value: 'fulfilled', label: t('mutual_aid.status.fulfilled', 'Fulfilled') },
		{ value: 'removed', label: t('mutual_aid.status.removed', 'Removed') },
		{ value: 'expired', label: t('mutual_aid.status.expired', 'Expired') }
	]);

	$effect(() => {
		void (async () => {
			const res = await adminCatalogApi.get('community.mutual_aid.status');
			if (res.ok && res.data?.length) statusOptions = res.data;
		})();
	});

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<MutualAidItem | null>(null);
	let showStatusModal = $state(false);
	let newStatus = $state('active');

	function openSetStatus(item: MutualAidItem) {
		actionTarget = item;
		newStatus = item.status || 'active';
		showStatusModal = true;
	}

	async function confirmSetStatus() {
		if (!actionTarget) return;
		// ADR-088 HITL gate: AI-generated content cannot be approved without HITL review.
		if (actionTarget.ai_generated && !actionTarget.hitl_reviewed_by) {
			pageError = t('app.mutual_aid.hitl_required', 'Cannot approve AI-generated content without HITL review');
			return;
		}
		acting = true;
		pageError = '';
		try {
			const result = await mutualAidAdminApi.updateStatus(actionTarget.id, { status: newStatus });
			if (result.ok) {
				showStatusModal = false;
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
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: MutualAidItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('app.mutual_aid.set_status', 'Set Status'),
				icon: RefreshCw,
				onclick: openSetStatus,
				show: () => true
			});
		}
		return acts;
	});

	const fields = $derived<FieldDef[]>([
		{ key: 'title', label: t('app.mutual_aid.col_title', 'Title'), type: 'text', help: t('app.mutual_aid.title_hint', 'The title shown in listings and detail views.'),},
		{ key: 'type', label: t('app.mutual_aid.col_type', 'Type'), type: 'text', help: t('app.mutual_aid.type_hint', 'The type — controls how it’s handled.') },
		{ key: 'status', label: t('app.mutual_aid.col_status', 'Status'), type: 'select', help: t('app.mutual_aid.status_hint', 'The record state — controls visibility and actions.'), options: statusOptions },
		{ key: 'description', label: t('app.mutual_aid.detail_description', 'Description'), type: 'textarea', help: t('app.mutual_aid.description_hint', 'Shown in detail views — keep it concise.') }
	]);

	const filters = $derived.by<FilterDef[]>(() => {
		countryUtils.ready;
		return [
			{
				key: 'status',
				label: t('app.mutual_aid.col_status', 'Status'),
				options: statusOptions
			},
			{
				key: 'dest',
				label: t('app.mutual_aid.col_destination', 'Destination'),
				type: 'autocomplete',
			help: t('app.mutual_aid.dest_hint', 'The destination country this applies to.'),
				placeholder: t('app.tools.country_placeholder', 'Search country…'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: countryUtils.fetcher,
				initialFetcher: countryUtils.initialFetcher,
				getSelectedLabel: countryUtils.getSelectedLabel
			}
		];
	});

	const config = $derived<EntityPageConfig>({
		filters,
		title: t('app.mutual_aid.review_queue_heading', 'Mutual Aid Review Queue'),
		description: t('app.mutual_aid.review_queue_desc', 'Review and moderate mutual aid requests'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('app.mutual_aid.col_title', 'Title'), sortable: true },
			{
				key: 'type',
				label: t('app.mutual_aid.col_type', 'Type'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'status',
				label: t('app.mutual_aid.col_status', 'Status'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'created_at',
				label: t('app.mutual_aid.col_created_at', 'Created'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			// Build query params for server-side pagination, sorting, and search.
			const queryParts: string[] = [];
			queryParts.push(`page=${(params.page ?? 0) + 1}`);
			queryParts.push(`limit=${params.pageSize ?? 25}`);
			if (params.q) queryParts.push(`q=${encodeURIComponent(params.q)}`);
			if (params.sort) {
				queryParts.push(`sort=${params.sort}`);
			}
			if (params.filter?.status) queryParts.push(`status=${encodeURIComponent(params.filter.status)}`);
			if (params.filter?.dest) queryParts.push(`dest=${encodeURIComponent(params.filter.dest)}`);
			const qs = queryParts.join('&');
			const result = await mutualAidAdminApi.list(qs);
			if (result.ok) {
				// BUG-007: apiRaw returns the full body { data, meta }.
				const body = result.data as { data?: MutualAidItem[]; meta?: { total?: number; total_pages?: number } };
				const data = body.data ?? (Array.isArray(body) ? (body as MutualAidItem[]) : []);
				const total = body.meta?.total ?? data.length;
				const totalPages = body.meta?.total_pages ?? Math.max(1, Math.ceil(total / (params.pageSize ?? 25)));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: MutualAidView
	});
</script>

<svelte:head>
	<title>{t('app.mutual_aid.review_queue_page_title', 'Mutual Aid Review Queue — {site} Admin')}</title>
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

<!-- ── Set Status modal ──────────────────────────────────────────────────── -->
<Modal bind:open={showStatusModal} title={t('app.mutual_aid.modal_set_status', 'Set Status')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.mutual_aid.set_status_confirm', 'Update the status of "{title}".').replace('{title}', actionTarget?.title ?? '')}
		</p>
		<div>
			<label for="status-select" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.mutual_aid.col_status', 'Status')}</label>
			<AutocompleteSelect
				id="status-select"
				bind:value={newStatus}
				options={statusOptions}
				class="w-full"
			/>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showStatusModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmSetStatus} disabled={acting}>
			{acting ? t('app.mutual_aid.updating', 'Updating...') : t('common.update', 'Update')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet MutualAidView({ row }: { row: MutualAidItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.mutual_aid.col_title', 'Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.mutual_aid.col_type', 'Type')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.type || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.mutual_aid.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.mutual_aid.col_created_at', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.mutual_aid.detail_description', 'Description')}</p>
			<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
		</div>
		{#if row.ai_generated}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.common.ai_generated', 'AI Generated')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{t('common.yes', 'Yes')} ({row.ai_model ?? '—'})</p>
			</div>
		{/if}
		{#if row.hitl_reviewed_by}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.common.hitl_reviewed_by', 'HITL Reviewed By')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.hitl_reviewed_by}</p>
			</div>
		{/if}
	</div>
{/snippet}
