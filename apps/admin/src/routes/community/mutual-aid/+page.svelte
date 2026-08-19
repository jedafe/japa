<script lang="ts">
	// /community/mutual-aid — admin HITL review queue for Mutual Aid requests with status update.
	import { onMount } from 'svelte';
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import Autocomplete, { type AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { mutualAidAdminApi , spacesAdminApi, adminCatalogApi, type CatalogOption } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, RefreshCw } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';

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
		{ key: 'title', label: t('app.mutual_aid.col_title', 'Title'), type: 'text' },
		{ key: 'type', label: t('app.mutual_aid.col_type', 'Type'), type: 'text' },
		{ key: 'status', label: t('app.mutual_aid.col_status', 'Status'), type: 'select', options: statusOptions },
		{ key: 'description', label: t('app.mutual_aid.detail_description', 'Description'), type: 'textarea' }
	]);

	
// Destination Spaces for the dest filter (geo-aware pattern). Loaded once
// on mount from /admin/spaces?level=country so the filter options are
// driven by the backend (Rule 16 — no hardcoded lists).
type SpaceOption = { id: string; name: string };
let destOptions = $state<SpaceOption[]>([]);
let selectedDest = $state<string>('');
onMount(async () => {
	try {
		const res = await spacesAdminApi.list('?level=country&limit=200');
		if (res.ok && Array.isArray(res.data)) {
			destOptions = (res.data as Array<{ id: string; name: string }>).map((s) => ({
				id: s.id,
				name: s.name
			}));
		}
	} catch {
		// Non-fatal — the dest filter simply stays empty.
	}
});

// ── Dest autocomplete helpers (filter pre-loaded options client-side) ──
function fetchDestOptions(q: string): Promise<{ id: string; label: string }[]> {
	const opts = destOptions.map((s) => ({ id: s.id, label: s.name }));
	const query = q.toLowerCase();
	const filtered = query === ''
		? opts
		: opts.filter((o) => o.label.toLowerCase().includes(query));
	return Promise.resolve(filtered);
}

function fetchDestOptionsInitial(): Promise<{ id: string; label: string }[]> {
	return Promise.resolve(destOptions.map((s) => ({ id: s.id, label: s.name })));
}

function getDestSelectedLabel(id: string): string {
	return destOptions.find((s) => s.id === id)?.name ?? id;
}

function selectDest(item: AutocompleteItem) {
	selectedDest = item.id;
	entityController?.reload();
}

function clearDest() {
	selectedDest = '';
	entityController?.reload();
}

		const filters = $derived<FilterDef[]>([
		{
			key: 'status',
			label: t('app.mutual_aid.col_status', 'Status'),
			options: statusOptions
		}
	]);

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
			queryParts.push(`page=${params.page ?? 1}`);
			queryParts.push(`limit=${params.pageSize ?? 25}`);
			if (params.q) queryParts.push(`q=${encodeURIComponent(params.q)}`);
			if (params.sort) {
				queryParts.push(`sort=${params.sort}`);
			}
			if (params.filter?.status) queryParts.push(`status=${encodeURIComponent(params.filter.status)}`);
			if (selectedDest) {
				queryParts.push(`dest=${encodeURIComponent(selectedDest)}`);
			}
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
	<title>{t('app.mutual_aid.review_queue_page_title', 'Mutual Aid Review Queue — JapaConnect Admin')}</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<div class="mb-4 flex items-center gap-2">
	<label for="dest-filter" class="text-sm font-medium" style="color: var(--color-foreground)">
		{t('app.mutual_aid.col_destination', 'Destination')}
	</label>
	<div class="w-64">
		<Autocomplete
			id="dest-filter"
			placeholder={t('app.mutual_aid.dest_filter_placeholder', 'Search destination…')}
			emptyMessage={t('common.no_results', 'No results found')}
			fetcher={fetchDestOptions}
			initialFetcher={fetchDestOptionsInitial}
			selectedId={selectedDest || null}
			selectedLabel={selectedDest ? getDestSelectedLabel(selectedDest) : null}
			onSelect={selectDest}
			onClear={clearDest}
		/>
	</div>
</div>

<EntityPage {config} bind:controller={entityController} />

<!-- ── Set Status modal ──────────────────────────────────────────────────── -->
<Modal bind:open={showStatusModal} title={t('app.mutual_aid.modal_set_status', 'Set Status')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.mutual_aid.set_status_confirm', 'Update the status of "{title}".').replace('{title}', actionTarget?.title ?? '')}
		</p>
		<div>
			<label for="status-select" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.mutual_aid.col_status', 'Status')}</label>
			<select
				id="status-select"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={newStatus}
			>
				{#each statusOptions as opt (opt)}
					<option value={opt.value}>{opt.label}</option>
				{/each}
			</select>
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
