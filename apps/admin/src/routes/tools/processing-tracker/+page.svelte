<script lang="ts">
	// /tools/processing-tracker — management page for user-submitted visa processing timeline entries.
	// Ref: japaconnect-engineering/sections/83-visa-timeline-tracker.md (ADR-091)
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Trash2 } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';
	import { processingTrackerAdminApi } from '$lib/api';
	import { useCountries } from '$lib/utils/countries.svelte';

	// TimelineDTO shape from GET /admin/visa-timelines (§83). The backend
	// returns the full TimelineDTO with joined milestones; we derive the
	// admin-facing columns from it.
	type Milestone = {
		id: string;
		milestone_type: string;
		occurred_at: string;
		notes?: string;
	};

	type TimelineDTO = {
		id: string;
		user_id: string;
		origin_space_id?: string;
		destination_space_id: string;
		visa_category: string;
		consulate?: string;
		application_method?: string;
		outcome?: string;
		is_shared: boolean;
		anonymised_at?: string;
		created_at: string;
		updated_at: string;
		milestones?: Milestone[];
	};

	// AdminListResponse is the envelope from GET /admin/visa-timelines.
	type AdminListResponse = {
		data: TimelineDTO[];
		pagination: { page: number; limit: number; total: number };
	};

	const canManage = $derived(hasPermission('visa_timeline.manage_any'));

	// ── State ───────────────────────────────────────────────────────────────
	
	// ── Country selector (250 primary geo space countries) ───────────────────
	const countryUtils = useCountries();
	$effect(() => {
		countryUtils.load();
	});

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<TimelineDTO | null>(null);
	let showDeleteModal = $state(false);

	function openDelete(item: TimelineDTO) {
		actionTarget = item;
		showDeleteModal = true;
	}

	async function confirmDelete() {
		if (!actionTarget) return;
		acting = true;
		pageError = '';
		try {
			const result = await processingTrackerAdminApi.delete(actionTarget.id);
			if (result.ok) {
				showDeleteModal = false;
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

	// Derive processing days from the milestone span (earliest applied →
	// latest decision/visa_issued), matching the backend RefreshStats logic.
	function processingDays(tl: TimelineDTO): number | null {
		const ms = tl.milestones ?? [];
		const applied = ms
			.filter((m) => m.milestone_type === 'applied')
			.sort((a, b) => new Date(a.occurred_at).getTime() - new Date(b.occurred_at).getTime())[0];
		const decision = ms
			.filter((m) => m.milestone_type === 'decision' || m.milestone_type === 'visa_issued')
			.sort((a, b) => new Date(b.occurred_at).getTime() - new Date(a.occurred_at).getTime())[0];
		if (!applied || !decision) return null;
		const days = Math.round(
			(new Date(decision.occurred_at).getTime() - new Date(applied.occurred_at).getTime()) / 86400000
		);
		return days >= 0 ? days : null;
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: TimelineDTO, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: openDelete,
				show: () => true
			});
		}
		return acts;
	});

	// `fields` is required by EntityPageConfig; readonly page so the form is never shown, but kept for shape completeness.
	const fields = $derived.by<FieldDef[]>(() => {
		countryUtils.ready;
		return [
			{ key: 'visa_category', label: t('app.tools.col_visa_category', 'Visa Category'), type: 'text' },
			{ key: 'destination_space_id', label: t('app.tools.col_dest_country', 'Destination'), type: 'autocomplete', placeholder: t('app.tools.country_placeholder', 'Search country…'), emptyMessage: t('common.no_results', 'No results found'), fetcher: countryUtils.fetcher, initialFetcher: countryUtils.initialFetcher, getSelectedLabel: countryUtils.getSelectedLabel },
			{ key: 'outcome', label: t('app.tools.col_status', 'Status'), type: 'text' }
		];
	});

		const filters = $derived.by<FilterDef[]>(() => {
		countryUtils.ready;
		return [
			{
				key: 'dest',
				label: t('app.tools.col_destination', 'Destination'),
				type: 'autocomplete',
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
		title: t('app.tools.processing_tracker_heading', 'Processing Time Tracker'),
		description: t('app.tools.processing_tracker_desc', 'Manage user-submitted visa processing timeline entries'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'visa_category', label: t('app.tools.col_visa_category', 'Visa Category'), sortable: true },
			{ key: 'destination_space_id', label: t('app.tools.col_dest_country', 'Destination'), sortable: true },
			{
				key: 'consulate',
				label: t('app.tools.col_consulate', 'Consulate'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'created_at',
				label: t('app.tools.col_submitted_date', 'Submitted'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			},
			{
				key: 'outcome',
				label: t('app.tools.col_status', 'Status'),
				sortable: true,
				render: (value: string) => value || 'pending'
			},
			{
				key: 'is_shared',
				label: t('app.tools.col_shared', 'Shared'),
				sortable: true,
				render: (value: boolean) => (value ? '✓' : '—')
			},
			{
				key: 'processing_days',
				label: t('app.tools.col_processing_days', 'Processing Days'),
				sortable: true,
				render: (_value: unknown, row: TimelineDTO) => {
					const d = processingDays(row);
					return d != null ? String(d) : '—';
				}
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			const qs = new URLSearchParams();
			if (params.page) qs.set('page', String(params.page));
			if (params.pageSize) qs.set('limit', String(params.pageSize));
			if (params.q) qs.set('q', params.q);
			if (params.sort) qs.set('sort', params.sort);
			if (params.filter?.dest) qs.set('dest', params.filter.dest);
			const result = await processingTrackerAdminApi.list(qs.toString());
			if (result.ok) {
				const envelope = result.data as AdminListResponse;
				const data = envelope?.data ?? [];
				const total = envelope?.pagination?.total ?? data.length;
				const totalPages = Math.max(1, Math.ceil(total / (params.pageSize || 25)));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: TrackerView
	});
</script>

<svelte:head>
	<title>{t('app.tools.processing_tracker_page_title', 'Processing Time Tracker — JapaConnect Admin')}</title>
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

<!-- ── Delete confirmation modal ─────────────────────────────────────────── -->
<Modal bind:open={showDeleteModal} title={t('app.tools.modal_delete_entry', 'Delete Entry')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.tools.delete_confirm', 'Are you sure you want to delete this processing timeline entry? This cannot be undone.')}
		</p>
		{#if actionTarget}
			<div
				class="rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
			>
				<p><strong>{actionTarget.visa_category || '—'}</strong></p>
				<p class="mt-1" style="color: var(--color-muted)">{actionTarget.destination_space_id || '—'} · {actionTarget.consulate || '—'}</p>
			</div>
		{/if}
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmDelete} disabled={acting}>
			{acting ? t('common.deleting', 'Deleting...') : t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet TrackerView({ row }: { row: TimelineDTO })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_visa_category', 'Visa Category')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.visa_category || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_dest_country', 'Destination')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.destination_space_id || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_consulate', 'Consulate')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.consulate || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_submitted_date', 'Submitted Date')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.outcome || 'pending'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_shared', 'Shared')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_shared ? '✓' : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_processing_days', 'Processing Days')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{processingDays(row) != null ? String(processingDays(row)) : '—'}</p>
		</div>
		{#if row.milestones && row.milestones.length > 0}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_milestones', 'Milestones')}</p>
				<ul class="mt-1 space-y-1 text-sm" style="color: var(--color-foreground)">
					{#each row.milestones as m (m.id)}
						<li>{m.milestone_type.replace('_', ' ')} — {new Date(m.occurred_at).toLocaleDateString()}{#if m.notes} · {m.notes}{/if}</li>
					{/each}
				</ul>
			</div>
		{/if}
	</div>
{/snippet}
