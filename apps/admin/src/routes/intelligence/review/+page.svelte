<script lang="ts">
	// /intelligence/review — intelligence review queue moderation page.
	// Lists extracted intel records pending review, with approve/reject/edit
	// actions. Backed by /intel/review/queue and /intel/review/{id}/*.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Check, X, Eye, Pencil } from 'lucide-svelte';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';
	import { intelReviewApi } from '$lib/api';

	type ReviewRecord = {
		id: string;
		source_id: string;
		raw_document_id?: string;
		entity_type: string;
		external_key?: string;
		extracted?: string;
		confidence?: string;
		evidence?: string;
		confidence_overall: number;
		status: string;
		freshness_score: number;
		extracted_at: string;
		last_verified_at: string;
		promoted_record_id?: string | null;
	};

	const canApprove = $derived(hasPermission('intel.review.approve'));
	const canReject = $derived(hasPermission('intel.review.reject'));
	const canEdit = $derived(hasPermission('intel.review.edit'));

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let actingId = $state('');
	let showActionConfirm = $state(false);
	let actionTarget = $state<ReviewRecord | null>(null);
	let actionType = $state<'approve' | 'reject' | ''>('');
	let showEditPanel = $state(false);
	let editTarget = $state<ReviewRecord | null>(null);
	let editExtracted = $state('');
	let editSaving = $state(false);

	function promptApprove(rec: ReviewRecord) {
		actionTarget = rec;
		actionType = 'approve';
		showActionConfirm = true;
	}

	function promptReject(rec: ReviewRecord) {
		actionTarget = rec;
		actionType = 'reject';
		showActionConfirm = true;
	}

	async function confirmAction() {
		if (!actionTarget || !actionType) return;
		if (actingId) return;
		actingId = actionTarget.id;
		pageError = '';
		try {
			const result =
				actionType === 'approve'
					? await intelReviewApi.approve(actionTarget.id)
					: await intelReviewApi.reject(actionTarget.id);
			if (result.ok) {
				showActionConfirm = false;
				actionTarget = null;
				actionType = '';
				entityController?.reload();
			} else {
				pageError = result.error;
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Action failed');
		} finally {
			actingId = '';
		}
	}

	function openEdit(rec: ReviewRecord) {
		editTarget = rec;
		editExtracted = rec.extracted ?? '';
		showEditPanel = true;
	}

	async function saveEdit() {
		if (!editTarget) return;
		if (editSaving) return;
		editSaving = true;
		pageError = '';
		try {
			// INTEL-009 fix: backend expects { fields: { ... } }, not { extracted }.
			// The editExtracted textarea holds a JSON string; parse it into a
			// field map so the backend can merge it into editorial_overrides.
			let fields: Record<string, unknown> = {};
			try {
				fields = editExtracted ? JSON.parse(editExtracted) : {};
			} catch {
				pageError = t('intel.invalid_json', 'Extracted data must be valid JSON');
				editSaving = false;
				return;
			}
			const result = await intelReviewApi.edit(editTarget.id, { fields });
			if (result.ok) {
				showEditPanel = false;
				editTarget = null;
				entityController?.reload();
			} else {
				pageError = result.error;
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Edit failed');
		} finally {
			editSaving = false;
		}
	}

	// Decode base64 extracted data to a readable JSON string.
	function decodeExtracted(rec: ReviewRecord): string {
		if (!rec.extracted) return '—';
		try {
			return atob(rec.extracted);
		} catch {
			return rec.extracted;
		}
	}

	function decodeConfidence(rec: ReviewRecord): string {
		if (!rec.confidence) return '—';
		try {
			return atob(rec.confidence);
		} catch {
			return rec.confidence;
		}
	}

	// ── Row actions ─────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: ReviewRecord, ctx) => ctx?.openView(row) }
		];
		if (canEdit) {
			acts.push({
				label: t('common.edit', 'Edit'),
				icon: Pencil,
				onclick: (row: ReviewRecord) => openEdit(row)
			});
		}
		if (canApprove) {
			acts.push({
				label: t('intel.approve', 'Approve'),
				icon: Check,
				onclick: (row: ReviewRecord) => promptApprove(row),
				show: (row: ReviewRecord) => row.status === 'review_required'
			});
		}
		if (canReject) {
			acts.push({
				label: t('intel.reject', 'Reject'),
				icon: X,
				variant: 'destructive',
				onclick: (row: ReviewRecord) => promptReject(row),
				show: (row: ReviewRecord) => row.status === 'review_required'
			});
		}
		return acts;
	});

	const filters: FilterDef[] = [
		{
			key: 'status',
			label: t('intel.status', 'Status'),
			options: [
				{ value: 'review_required', label: t('intel.status_review_required', 'Review Required') },
				{ value: 'approved', label: t('intel.status_approved', 'Approved') },
				{ value: 'rejected', label: t('intel.status_rejected', 'Rejected') }
			]
		},
		{
			key: 'entity_type',
			label: t('intel.entity_type', 'Entity Type'),
			options: [
				{ value: 'scholarship', label: 'Scholarship' },
				{ value: 'job', label: 'Job' },
				{ value: 'visa', label: 'Visa' },
				{ value: 'housing', label: 'Housing' },
				{ value: 'remittance', label: 'Remittance' },
				{ value: 'cost_of_living', label: 'Cost of Living' },
				{ value: 'travel_routes', label: 'Travel Routes' },
				{ value: 'grants', label: 'Grants' },
				{ value: 'admission', label: 'Admission' },
				{ value: 'banking', label: 'Banking' },
				{ value: 'work_career', label: 'Work & Career' },
				{ value: 'health_insurance', label: 'Health Insurance' },
				{ value: 'culture_lifestyle', label: 'Culture & Lifestyle' }
			]
		}
	];

	const fields: FieldDef[] = [
		{ key: 'extracted', label: t('intel.extracted', 'Extracted Data'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('intel.review_title', 'Intelligence Review Queue'),
		description: t('intel.review_desc', 'Review, edit, approve, or reject extracted intelligence records.'),
		readonly: true,
		pageSize: 25,
		columns: [
			{
				key: 'entity_type',
				label: t('intel.entity_type', 'Type'),
				sortable: true,
				render: (v: string) => {
					const clsMap: Record<string, string> = {
						scholarship: 'bg-primary/15 text-primary border-primary',
						job: 'bg-success/15 text-success border-success',
						visa: 'bg-warning/15 text-warning border-warning'
					};
					const cls = clsMap[v] ?? 'bg-muted text-muted-foreground border-border';
					return `<span class="inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${cls}">${v}</span>`;
				}
			},
			{
				key: 'external_key',
				label: t('intel.external_key', 'External Key'),
				render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v || '—'}</span>`
			},
			{
				key: 'confidence_overall',
				label: t('intel.confidence', 'Confidence'),
				sortable: true,
				render: (v: number) => {
					if (v == null) return '—';
					const pct = Math.round(v * 100);
					const cls = pct >= 80 ? 'text-success' : pct >= 50 ? 'text-warning' : 'text-danger';
					return `<span class="text-xs font-medium ${cls}">${pct}%</span>`;
				}
			},
			{
				key: 'status',
				label: t('intel.status', 'Status'),
				sortable: true,
				render: (v: string) => {
					const clsMap: Record<string, string> = {
						review_required: 'bg-warning/15 text-warning border-warning',
						approved: 'bg-success/15 text-success border-success',
						rejected: 'bg-danger/15 text-danger border-danger'
					};
					const cls = clsMap[v] ?? 'bg-muted text-muted-foreground border-border';
					return `<span class="inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${cls}">${v.replace(/_/g, ' ')}</span>`;
				}
			},
			{
				key: 'freshness_score',
				label: t('intel.freshness', 'Freshness'),
				sortable: true,
				render: (v: number) => (v != null ? `${v}%` : '—')
			},
			{
				key: 'extracted_at',
				label: t('intel.extracted_at', 'Extracted At'),
				sortable: true,
				render: (v: string) => `<span class="text-muted-foreground text-xs">${v ? new Date(v).toLocaleDateString() : '—'}</span>`
			}
		],
		filters,
		actions: rowActions,
		fields,
		load: async ({ q, filter, page, pageSize }) => {
			const params = new URLSearchParams();
			if (q) params.set('q', q);
			if (filter.status) params.set('status', filter.status);
			if (filter.entity_type) params.set('entity_type', filter.entity_type);
			if (page) params.set('page', String(page));
			if (pageSize) params.set('page_size', String(pageSize));
			const result = await intelReviewApi.queue(params.toString());
			if (result.ok) {
				const body = result.data;
				const data = (body?.data as ReviewRecord[]) ?? [];
				const total = data.length; // queue endpoint may not paginate
				const totalPages = 1;
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('intel.load_error', 'Failed to load review queue'));
		},
		viewSnippet: ReviewView
	});
</script>

<svelte:head>
	<title>{t('intel.review_title', 'Intelligence Review Queue')} — JapaConnect Admin</title>
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

<!-- ── Approve/Reject confirmation ───── -->
<Modal bind:open={showActionConfirm} title={actionType === 'approve' ? t('intel.approve', 'Approve') : t('intel.reject', 'Reject')} size="sm">
	{#if actionTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{actionType === 'approve'
				? t('intel.approve_confirm', 'Approve this')
				: t('intel.reject_confirm', 'Reject this')}
			<strong>{actionTarget.entity_type}</strong>
			({actionTarget.external_key || actionTarget.id.slice(0, 8)})?
		</p>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showActionConfirm = false)} disabled={actingId !== ''}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button
			variant={actionType === 'reject' ? 'destructive' : 'default'}
			onclick={confirmAction}
			disabled={actingId !== ''}
		>
			{actingId !== '' ? t('common.processing', 'Processing...') : t('common.confirm', 'Confirm')}
		</Button>
	</div>
</Modal>

<!-- ── Edit slide-over (page-local Modal for simplicity) ───── -->
<Modal bind:open={showEditPanel} title={t('intel.edit_extracted', 'Edit Extracted Data')} size="lg">
	{#if editTarget}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('intel.entity_type', 'Entity Type')}: {editTarget.entity_type}
				</p>
				<p class="text-xs font-medium uppercase tracking-wider mt-1" style="color: var(--color-muted)">
					{t('intel.external_key', 'External Key')}: {editTarget.external_key || '—'}
				</p>
			</div>
			<div>
				<label class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)" for="extracted-edit">
					{t('intel.extracted', 'Extracted Data (JSON)')}
				</label>
				<textarea
					id="extracted-edit"
					bind:value={editExtracted}
					rows="12"
					class="mt-1 w-full rounded-md border p-3 font-mono text-xs"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				></textarea>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showEditPanel = false)} disabled={editSaving}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button onclick={saveEdit} disabled={editSaving}>
			{editSaving ? t('common.processing', 'Processing...') : t('common.save', 'Save')}
		</Button>
	</div>
</Modal>

<!-- ── View snippet ──────────────────────────────────────────────────────── -->
{#snippet ReviewView({ row }: { row: ReviewRecord })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('intel.entity_type', 'Entity Type')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.entity_type || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('intel.external_key', 'External Key')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.external_key || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('intel.confidence', 'Confidence')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{row.confidence_overall != null ? `${Math.round(row.confidence_overall * 100)}%` : '—'}
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('intel.status', 'Status')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status?.replace(/_/g, ' ') || '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('intel.freshness', 'Freshness')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.freshness_score != null ? `${row.freshness_score}%` : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('intel.extracted_at', 'Extracted At')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.extracted_at ? new Date(row.extracted_at).toLocaleString() : '—'}</p>
			</div>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('intel.extracted', 'Extracted Data')}</p>
			<pre class="mt-1 rounded-md border p-3 text-xs overflow-x-auto" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)">{decodeExtracted(row)}</pre>
		</div>
		{#if row.confidence}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('intel.confidence_detail', 'Confidence Detail')}</p>
				<pre class="mt-1 rounded-md border p-3 text-xs overflow-x-auto" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)">{decodeConfidence(row)}</pre>
			</div>
		{/if}
	</div>
{/snippet}
