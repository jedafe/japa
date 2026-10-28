<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /intelligence/review — intelligence review queue moderation page.
	// Lists extracted intel records pending review, with approve/reject/edit
	// actions. Backed by /intel/records (defaults to review_required) and
	// /intel/review/{id}/*.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { Button } from '$lib/components/ui/button';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Check, X, Eye, Pencil, Wand } from 'lucide-svelte';
	import { goto } from '$app/navigation';
	import type { ActionItem, FilterDef } from '$lib/components/ui/data-table';
	import { intelReviewApi, adminCatalogApi, type CatalogOption } from '$lib/api';

	type ReviewRecord = {
		id: string;
		source_id: string;
		raw_document_id?: string;
		entity_type: string;
		external_key?: string;
		// JSONB columns serialize as inline JSON objects, not base64.
		extracted?: unknown;
		confidence?: unknown;
		evidence?: unknown;
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
	const canCreate = $derived(hasPermission('studio.create') || hasPermission('studio.manage'));

	let entityController = $state<EntityPageController | undefined>(undefined);

	// Fallback option sets — the authoritative lists come from
	// /admin/catalogs/intelligence.review.{status,entity_type} (mirroring the
	// intel_records.status CHECK constraint and the extractor schema registry);
	// these mirror the same values so the filters still work if the catalog
	// endpoint is unreachable.
	let intelReviewStatusOptions = $state<CatalogOption[]>([
		{ value: 'review_required', label: t('intel.status_review_required', 'Review Required') },
		{ value: 'extracted', label: t('intel.status_extracted', 'Extracted') },
		{ value: 'deduped', label: t('intel.status_deduped', 'Deduped') },
		{ value: 'auto_published', label: t('intel.status_auto_published', 'Auto Published') },
		{ value: 'approved', label: t('intel.status_approved', 'Approved') },
		{ value: 'rejected', label: t('intel.status_rejected', 'Rejected') },
		{ value: 'published', label: t('intel.status_published', 'Published') },
		{ value: 'stale', label: t('intel.status_stale', 'Stale') },
		{ value: 'off_topic', label: t('intel.status_off_topic', 'Off Topic') }
	]);

	let intelReviewEntityTypeOptions = $state<CatalogOption[]>([
		{ value: 'scholarship', label: t('intel.entity_scholarship', 'Scholarship') },
		{ value: 'job', label: t('intel.entity_job', 'Job') },
		{ value: 'visa', label: t('intel.entity_visa', 'Visa') },
		{ value: 'housing', label: t('intel.entity_housing', 'Housing') },
		{ value: 'remittance', label: t('intel.entity_remittance', 'Remittance') },
		{ value: 'cost_of_living', label: t('intel.entity_cost_of_living', 'Cost of Living') },
		{ value: 'travel_routes', label: t('intel.entity_travel_routes', 'Travel Routes') },
		{ value: 'grants', label: t('intel.entity_grants', 'Grants') },
		{ value: 'admission', label: t('intel.entity_admission', 'Admission') },
		{ value: 'banking', label: t('intel.entity_banking', 'Banking') },
		{ value: 'work_career', label: t('intel.entity_work_career', 'Work & Career') },
		{ value: 'health_insurance', label: t('intel.entity_health_insurance', 'Health Insurance') },
		{ value: 'culture_lifestyle', label: t('intel.entity_culture_lifestyle', 'Culture & Lifestyle') },
		{ value: 'internship', label: t('intel.entity_internship', 'Internship') },
		{ value: 'import_export', label: t('intel.entity_import_export', 'Import & Export') },
		{ value: 'credential_pathway', label: t('intel.entity_credential_pathway', 'Credential Pathway') },
		{ value: 'news', label: t('intel.entity_news', 'News') },
		{ value: 'article', label: t('intel.entity_article', 'Article') },
		{ value: 'event', label: t('intel.entity_event', 'Event') },
		{ value: 'dataset', label: t('intel.entity_dataset', 'Dataset') },
		{ value: 'wiki_page', label: t('intel.entity_wiki_page', 'Wiki Page') },
		{ value: 'language_test_requirement', label: t('intel.entity_language_test_requirement', 'Language Test Requirement') },
		{ value: 'tax_obligation_guide', label: t('intel.entity_tax_obligation_guide', 'Tax Obligation Guide') },
		{ value: 'community_organisation', label: t('intel.entity_community_organisation', 'Community Organisation') }
	]);

	$effect(() => {
		void (async () => {
			const [statusRes, typeRes] = await Promise.all([
				adminCatalogApi.get('intelligence.review.status'),
				adminCatalogApi.get('intelligence.review.entity_type')
			]);
			if (statusRes.ok && statusRes.data?.length) intelReviewStatusOptions = statusRes.data;
			if (typeRes.ok && typeRes.data?.length) intelReviewEntityTypeOptions = typeRes.data;
		})();
	});

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
		editExtracted = jsonBlobToText(rec.extracted);
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

	// Serialize a JSONB blob (already an inline JSON object from the API) to a
	// readable pretty-printed string. The backend sends json.RawMessage for
	// these columns, so atob() decoding was wrong — it turned the object into
	// "[object Object]" and broke the edit seed.
	function jsonBlobToText(v: unknown): string {
		if (v == null || v === '') return '';
		if (typeof v === 'string') {
			try {
				return JSON.stringify(JSON.parse(v), null, 2);
			} catch {
				return v;
			}
		}
		try {
			return JSON.stringify(v, null, 2);
		} catch {
			return String(v);
		}
	}

	function decodeExtracted(rec: ReviewRecord): string {
		return jsonBlobToText(rec.extracted) || '—';
	}

	function decodeConfidence(rec: ReviewRecord): string {
		return jsonBlobToText(rec.confidence) || '—';
	}

	// Map an intel entity_type to a Studio content_type. Most guide-like
	// entities funnel into the generic "guide" content type; a few have
	// dedicated Studio content types.
	const ENTITY_TO_CONTENT_TYPE: Record<string, string> = {
		scholarship: 'guide',
		job: 'job_listing',
		visa: 'visa_route',
		housing: 'housing_guide',
		wiki_page: 'wiki_page',
		language_test_requirement: 'language_test_requirement',
		tax_obligation_guide: 'tax_obligation_guide',
		community_organisation: 'community_organisation',
		cost_of_living: 'comparison',
		remittance: 'guide',
		// Guide content types — these have no projection table; the
		// guide_content promoter writes directly to contents. They map
		// to the generic 'guide' Studio content type with their category.
		travel_routes: 'guide',
		grants: 'guide',
		admission: 'guide',
		banking: 'guide',
		work_career: 'guide',
		health_insurance: 'guide',
		culture_lifestyle: 'guide',
		internship: 'job_listing',
		import_export: 'guide',
		credential_pathway: 'guide',
		article: 'guide',
		event: 'guide',
		dataset: 'guide',
		// News promotes to contents with content_type='news' — not a guide.
		news: 'news'
	};

	function contentTypeForEntity(entityType: string): string {
		return ENTITY_TO_CONTENT_TYPE[entityType] ?? 'guide';
	}

	// Derive a topic prompt from a record. Prefer a human-readable title
	// from the extracted payload; fall back to the external key.
	function topicPromptForRecord(rec: ReviewRecord): string {
		let title = '';
		try {
			let parsed: unknown = rec.extracted;
			if (typeof parsed === 'string') parsed = parsed ? JSON.parse(parsed) : null;
			if (parsed && typeof parsed === 'object') {
				const p = parsed as Record<string, unknown>;
				title = String(p.title ?? p.name ?? '');
			}
		} catch {
			// ignore — fall through to external_key
		}
		if (!title) title = rec.external_key || '';
		return title;
	}

	function promoteToStudio(rec: ReviewRecord) {
		const params = new URLSearchParams();
		params.set('intel_record_id', rec.id);
		params.set('content_type', contentTypeForEntity(rec.entity_type));
		const topic = topicPromptForRecord(rec);
		if (topic) params.set('topic_prompt', topic);
		goto(`/studio?${params.toString()}`);
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
		// Promote an extracted intel record into a Studio job. Navigates to
		// the Studio page with query params that pre-fill the create-job form.
		if (canCreate) {
			acts.push({
				label: t('intel.promote_to_studio', 'Promote to Studio'),
				icon: Wand,
				onclick: (row: ReviewRecord) => promoteToStudio(row),
				show: (row: ReviewRecord) => row.status === 'extracted'
			});
		}
		return acts;
	});

	// Wraps a static CatalogOption list as Autocomplete data source — the
	// filter panel requires every dropdown to be an Autocomplete, and the
	// option sets here are catalog-sized (small, already loaded), so
	// filtering happens client-side over the loaded options.
	function optionFilter(options: CatalogOption[]) {
		return {
			fetcher: async (q: string) => {
				const needle = q.trim().toLowerCase();
				return options
					.filter((o) => !needle || o.label.toLowerCase().includes(needle) || o.value.includes(needle))
					.map((o) => ({ id: o.value, label: o.label }));
			},
			initialFetcher: async () => options.map((o) => ({ id: o.value, label: o.label })),
			getSelectedLabel: (id: string) => options.find((o) => o.value === id)?.label ?? id
		};
	}

	const filters = $derived<FilterDef[]>([
		{
			key: 'status',
			label: t('intel.status', 'Status'),
			type: 'autocomplete',
			placeholder: t('intel.status', 'Status'),
			...optionFilter(intelReviewStatusOptions)
		},
		{
			key: 'entity_type',
			label: t('intel.entity_type', 'Entity Type'),
			type: 'autocomplete',
			placeholder: t('intel.entity_type', 'Entity Type'),
			...optionFilter(intelReviewEntityTypeOptions)
		}
	]);

	const fields: FieldDef[] = [
		{ key: 'extracted', label: t('intel.extracted', 'Extracted Data'), type: 'textarea', help: t('intel.extracted_hint', 'What was extracted — internal use.') }
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
				sortable: true,
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
						extracted: 'bg-primary/15 text-primary border-primary',
						deduped: 'bg-primary/15 text-primary border-primary',
						auto_published: 'bg-success/15 text-success border-success',
						approved: 'bg-success/15 text-success border-success',
						published: 'bg-success/15 text-success border-success',
						rejected: 'bg-danger/15 text-danger border-danger',
						stale: 'bg-muted text-muted-foreground border-border',
						off_topic: 'bg-muted text-muted-foreground border-border'
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
		load: async ({ q, filter, page, pageSize, sort }) => {
			const params = new URLSearchParams();
			if (q) params.set('q', q);
			if (filter.status) params.set('status', filter.status);
			if (filter.entity_type) params.set('entity_type', filter.entity_type);
			// sort arrives as "key:dir" (EntityPage convention); the API takes
			// separate sort/dir params over a whitelisted column set.
			if (sort) {
				const [sortKey, sortDir] = sort.split(':');
				if (sortKey) params.set('sort', sortKey);
				if (sortDir === 'desc') params.set('dir', 'desc');
			}
			params.set('page_offset', String(page * pageSize));
			params.set('page_size', String(pageSize));
			// /intel/records (not /intel/review/queue) — the status filter needs
			// arbitrary statuses; the endpoint defaults to review_required when
			// no status is selected, preserving the queue behaviour.
			const result = await intelReviewApi.list(params.toString());
			if (result.ok) {
				const body = result.data as {
					data?: ReviewRecord[];
					meta?: { total?: number; count?: number; page_size?: number; page_offset?: number };
				};
				const data = (body?.data as ReviewRecord[]) ?? [];
				// meta.total is the matching-row count for pagination;
				// meta.count is the current page length.
				const total = body?.meta?.total ?? body?.meta?.count ?? data.length;
				const totalPages = Math.max(1, Math.ceil(total / pageSize));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('intel.load_error', 'Failed to load review queue'));
		},
		viewSnippet: ReviewView
	});
</script>

<svelte:head>
	<title>{t('intel.review_title', 'Intelligence Review Queue')} — {siteName()} Admin</title>
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
	{#if pageError}
		<p class="mt-3 text-sm text-danger">{pageError}</p>
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

<!-- ── Edit slide-over ───── -->
<SlideOver bind:open={showEditPanel} title={t('intel.edit_extracted', 'Edit Extracted Data')} size="lg">
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
					rows="14"
					class="mt-1 w-full rounded-md border p-3 font-mono text-xs"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				></textarea>
			</div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => (showEditPanel = false)} disabled={editSaving}>
				{t('common.cancel', 'Cancel')}
			</Button>
			<Button onclick={saveEdit} disabled={editSaving}>
				{editSaving ? t('common.processing', 'Processing...') : t('common.save', 'Save Changes')}
			</Button>
		</div>
	{/snippet}
</SlideOver>

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
