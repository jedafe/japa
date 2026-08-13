<script lang="ts">
	import { api } from '$lib/api';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import AccessDenied from '$lib/components/AccessDenied.svelte';
	import RawContentPane from '$lib/components/intelligence/RawContentPane.svelte';
	import ExtractionPane from '$lib/components/intelligence/ExtractionPane.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasAnyPermission } from '$lib/stores/auth.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		ArrowLeft,
		CheckCircle2,
		XCircle,
		Pencil,
		Save,
		Loader2,
		Database,
		FileText,
		Sparkles,
		ListTree,
		ExternalLink
	} from 'lucide-svelte';

	// SPA: read the route id from client-side page state (no SSR load()).
	const recordId = $derived(page.params.id);

	const canView = $derived(hasAnyPermission(['intel.review.view', 'intel.review.*']));
	const canApprove = $derived(hasAnyPermission(['intel.review.approve', 'intel.review.*']));
	const canReject = $derived(hasAnyPermission(['intel.review.reject', 'intel.review.*']));
	const canEdit = $derived(hasAnyPermission(['intel.review.edit', 'intel.review.*']));

	interface IntelRecord {
		id: string;
		source_id: string;
		raw_document_id: string;
		entity_type: string;
		external_key: string;
		extracted?: unknown;
		confidence?: unknown;
		evidence?: unknown;
		confidence_overall?: number;
		status: string;
		freshness_score?: number;
		geo_space_ids?: unknown;
		extracted_at?: string;
		last_verified_at?: string;
		promoted_record_id?: string | null;
	}

	interface IntelSource {
		id: string;
		name: string;
		domain: string;
		source_url: string;
		category: string;
		source_type: string;
		trust_level: number;
		crawl_method: string;
		crawl_frequency: string;
		enabled: boolean;
		last_crawled_at?: string | null;
		last_success_at?: string | null;
	}

	interface RawDocument {
		id?: string;
		url?: string;
		content_type?: string;
		// The backend returns the body as base64 (body_b64) to preserve
		// binary content (PDF bytes, etc.) per route catalog §2229. The
		// prior interface expected a plain `body` string and `status_code`/
		// `fetched_at` field names that the backend never sent — contract
		// drift fix.
		body_b64?: string;
		http_status?: number;
		retrieved_at?: string;
		[key: string]: unknown;
	}

	let record = $state<IntelRecord | null>(null);
	let source = $state<IntelSource | null>(null);
	let rawDoc = $state<RawDocument | null>(null);
	let loading = $state(true);
	let errorMessage = $state('');
	let actionLoading = $state<'approve' | 'reject' | 'save' | null>(null);
	let actionMessage = $state('');
	let actionType = $state<'success' | 'error' | ''>('');

	// Edit mode for the structured record pane.
	let editMode = $state(false);
	let editJson = $state('');

	// Structured record (promoted projection) — fetched best-effort when
	// promoted_record_id is present.
	let structuredRecord = $state<Record<string, unknown> | null>(null);

	async function loadAll() {
		loading = true;
		errorMessage = '';
		actionMessage = '';

		const recRes = await api<IntelRecord[]>(
			`/api/v1/intel/review/queue?page_size=100`
		);
		// The review queue returns a list; locate this record by id. If the
		// record is not in the queue (e.g., already approved/rejected), show
		// the not-found state. The prior code fell back to /intel/radar/feed
		// which returns a scholarship-shaped object, not an IntelRecord — the
		// fields don't match (no source_id, raw_document_id, extracted, etc.)
		// so the four-pane moderation view would break silently.
		// The api() helper already unwraps the response `data` field, so
		// recRes.data is the array directly.
		if (recRes.ok && Array.isArray(recRes.data)) {
			const found = (recRes.data as IntelRecord[]).find((r) => r.id === recordId);
			record = found ?? null;
		} else if (!recRes.ok) {
			errorMessage = recRes.error;
		}

		if (record) {
			// Seed the edit buffer with the extracted blob.
			editJson = record.extracted ? safeStringify(record.extracted) : '';

			// Fetch the source and raw document in parallel (best-effort).
			await Promise.allSettled([loadSource(record.source_id), loadRawDoc(record.raw_document_id)]);

			// Fetch the promoted/structured record when available.
			if (record.promoted_record_id) {
				await loadStructured(record.promoted_record_id);
			}
		}

		loading = false;
	}

	async function loadSource(sourceId: string) {
		// Fetch all sources (including disabled) to find the one for this
		// record. There is no single-source GET endpoint, so we fetch the
		// full list and find by id. include_disabled=true ensures disabled
		// sources are still resolved (BUG-004 fix).
		const result = await api<IntelSource[]>(
			'/api/v1/intel/sources?page_size=100&include_disabled=true'
		);
		if (result.ok && Array.isArray(result.data)) {
			source = (result.data as IntelSource[]).find((s) => s.id === sourceId) ?? null;
		}
	}

	async function loadRawDoc(docId: string) {
		// The raw document endpoint is best-effort; the pane handles absence.
		const result = await api<RawDocument>(
			`/api/v1/intel/raw-documents/${docId}`
		);
		if (result.ok && result.data) {
			rawDoc = result.data as RawDocument;
		}
	}

	async function loadStructured(promotedId: string) {
		const result = await api<Record<string, unknown>>(
			`/api/v1/intel/scholarships/${promotedId}`
		);
		if (result.ok && result.data) {
			structuredRecord = result.data as Record<string, unknown>;
		}
	}

	function safeStringify(val: unknown): string {
		if (val == null) return '';
		if (typeof val === 'string') {
			try {
				return JSON.stringify(JSON.parse(val), null, 2);
			} catch {
				return val;
			}
		}
		try {
			return JSON.stringify(val, null, 2);
		} catch {
			return String(val);
		}
	}

	$effect(() => {
		if (recordId) loadAll();
	});

	function notify(type: 'success' | 'error', msg: string) {
		actionType = type;
		actionMessage = msg;
	}

	async function approve() {
		if (!record) return;
		actionLoading = 'approve';
		actionMessage = '';
		const result = await api(`/api/v1/intel/review/${record.id}/approve`, { method: 'POST' });
		actionLoading = null;
		if (result.ok) {
			// Redirect to the review queue after a successful approve. The
			// record is no longer in the review_required queue, so reloading
			// this page would show "Record not found" — confusing UX.
			goto('/intelligence/review');
		} else {
			notify('error', result.error);
		}
	}

	async function reject() {
		if (!record) return;
		actionLoading = 'reject';
		actionMessage = '';
		const result = await api(`/api/v1/intel/review/${record.id}/reject`, { method: 'POST' });
		actionLoading = null;
		if (result.ok) {
			// Redirect to the review queue after a successful reject.
			goto('/intelligence/review');
		} else {
			notify('error', result.error);
		}
	}

	async function saveEdit() {
		if (!record) return;
		actionLoading = 'save';
		actionMessage = '';
		// Persist edits to the extracted blob via the review edit endpoint.
		// The backend expects { fields: { fieldname: value, ... } } which it
		// merges into editorial_overrides (route catalog §2243). The prior
		// code sent { extracted: parsed } which the backend rejected with
		// 'missing_field' — contract drift fix.
		let parsed: unknown;
		try {
			parsed = editJson.trim() ? JSON.parse(editJson) : null;
		} catch {
			actionLoading = null;
			notify('error', t('intel.invalid_json', 'Extracted fields must be valid JSON.'));
			return;
		}
		if (parsed == null || typeof parsed !== 'object' || Array.isArray(parsed)) {
			actionLoading = null;
			notify('error', t('intel.invalid_json', 'Extracted fields must be a JSON object.'));
			return;
		}
		const result = await api(`/api/v1/intel/review/${record.id}/edit`, {
			method: 'PATCH',
			body: { fields: parsed as Record<string, unknown> }
		});
		actionLoading = null;
		if (result.ok) {
			notify('success', t('intel.edit_saved', 'Structured record updated.'));
			editMode = false;
			await loadAll();
		} else {
			notify('error', result.error);
		}
	}

	function formatTime(ts?: string | null): string {
		if (!ts) return '—';
		try {
			return new Date(ts).toLocaleString();
		} catch {
			return ts;
		}
	}

	function trustLabel(level: number): string {
		switch (level) {
			case 1: return t('intel.trust_official', 'Official');
			case 2: return t('intel.trust_trusted', 'Trusted');
			case 3: return t('intel.trust_aggregator', 'Aggregator');
			case 4: return t('intel.trust_community', 'Community');
			default: return String(level);
		}
	}

	const structuredJson = $derived(safeStringify(structuredRecord));
</script>

<PageHeader
	title={t('intel.review_detail_title', 'Review Record')}
	description={t('intel.review_detail_desc', 'Four-pane moderation: source, raw content, extraction, and structured record.')}
/>

{#if !canView}
	<AccessDenied message={t('intel.review_access_denied', 'You need intel.review.view permission to review records.')} />
{:else}
	<div class="mb-4 flex items-center justify-between">
		<button
			class="text-muted-foreground hover:text-foreground flex items-center gap-1.5 text-sm transition-colors"
			onclick={() => goto('/intelligence/review')}
		>
			<ArrowLeft class="size-4" />
			{t('intel.back_to_queue', 'Back to queue')}
		</button>
		{#if record}
			<div class="flex items-center gap-2">
				{#if canApprove}
					<button
						class="bg-emerald-500/10 text-emerald-500 hover:bg-emerald-500/20 flex items-center gap-1.5 rounded-9px px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
						disabled={actionLoading !== null}
						onclick={approve}
					>
						{#if actionLoading === 'approve'}
							<Loader2 class="size-4 animate-spin" />
						{:else}
							<CheckCircle2 class="size-4" />
						{/if}
						{t('intel.approve', 'Approve')}
					</button>
				{/if}
				{#if canReject}
					<button
						class="bg-destructive/10 text-destructive hover:bg-destructive/20 flex items-center gap-1.5 rounded-9px px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50"
						disabled={actionLoading !== null}
						onclick={reject}
					>
						{#if actionLoading === 'reject'}
							<Loader2 class="size-4 animate-spin" />
						{:else}
							<XCircle class="size-4" />
						{/if}
						{t('intel.reject', 'Reject')}
					</button>
				{/if}
			</div>
		{/if}
	</div>

	{#if errorMessage}
		<div class="bg-destructive/10 text-destructive mb-4 rounded-md p-3 text-sm">
			{errorMessage}
		</div>
	{/if}

	{#if actionMessage}
		<div class="mb-4 rounded-md p-3 text-sm {actionType === 'success' ? 'bg-emerald-500/10 text-emerald-500' : 'bg-destructive/10 text-destructive'}">
			{actionMessage}
		</div>
	{/if}

	{#if loading}
		<div class="grid gap-4 lg:grid-cols-2">
			{#each Array(4) as _}
				<div class="bg-muted h-64 animate-pulse rounded-xl"></div>
			{/each}
		</div>
	{:else if !record}
		<div class="text-muted-foreground py-16 text-center">
			<FileText class="mx-auto size-10 opacity-30" />
			<p class="mt-3 text-sm">{t('intel.record_not_found', 'Record not found')}</p>
		</div>
	{:else}
		<!-- Record meta strip -->
		<div class="border-border-card bg-background-alt shadow-card mb-4 grid gap-3 rounded-xl border p-4 sm:grid-cols-4">
			<div>
				<p class="text-muted-foreground text-[10px] font-medium uppercase tracking-wider">{t('intel.external_key', 'External Key')}</p>
				<p class="text-foreground mt-0.5 truncate font-mono text-xs">{record.external_key || '—'}</p>
			</div>
			<div>
				<p class="text-muted-foreground text-[10px] font-medium uppercase tracking-wider">{t('intel.status', 'Status')}</p>
				<p class="mt-0.5">
					<span class="inline-flex items-center gap-1 rounded-full bg-amber-500/10 px-2 py-0.5 text-xs font-medium text-amber-500">{record.status}</span>
				</p>
			</div>
			<div>
				<p class="text-muted-foreground text-[10px] font-medium uppercase tracking-wider">{t('intel.confidence', 'Confidence')}</p>
				<p class="text-foreground mt-0.5 font-mono text-xs">{record.confidence_overall != null ? (record.confidence_overall * 100).toFixed(0) + '%' : '—'}</p>
			</div>
			<div>
				<p class="text-muted-foreground text-[10px] font-medium uppercase tracking-wider">{t('intel.extracted_at', 'Extracted At')}</p>
				<p class="text-foreground mt-0.5 text-xs">{formatTime(record.extracted_at)}</p>
			</div>
		</div>

		<!-- Four-pane grid -->
		<div class="grid gap-4 lg:grid-cols-2">
			<!-- Pane 1: Source -->
			<div class="border-border-card bg-background-alt shadow-card flex flex-col rounded-xl">
				<div class="border-border-card flex items-center gap-2 border-b px-4 py-3">
					<Database class="text-muted-foreground size-4" />
					<h3 class="text-foreground text-sm font-semibold">{t('intel.source_pane', 'Source')}</h3>
				</div>
				<div class="flex-1 overflow-y-auto p-4">
					{#if source}
						<dl class="space-y-2.5 text-sm">
							<div>
								<dt class="text-muted-foreground text-xs font-medium uppercase">{t('intel.field_name', 'Name')}</dt>
								<dd class="text-foreground font-medium">{source.name}</dd>
							</div>
							<div class="grid grid-cols-2 gap-3">
								<div>
									<dt class="text-muted-foreground text-xs font-medium uppercase">{t('intel.field_domain', 'Domain')}</dt>
									<dd class="text-foreground font-mono text-xs">{source.domain}</dd>
								</div>
								<div>
									<dt class="text-muted-foreground text-xs font-medium uppercase">{t('intel.field_category', 'Category')}</dt>
									<dd class="text-foreground capitalize text-xs">{source.category}</dd>
								</div>
							</div>
							<div class="grid grid-cols-2 gap-3">
								<div>
									<dt class="text-muted-foreground text-xs font-medium uppercase">{t('intel.field_crawl_method', 'Method')}</dt>
									<dd class="text-foreground font-mono text-xs uppercase">{source.crawl_method}</dd>
								</div>
								<div>
									<dt class="text-muted-foreground text-xs font-medium uppercase">{t('intel.field_trust_level', 'Trust')}</dt>
									<dd class="text-foreground text-xs">{trustLabel(source.trust_level)}</dd>
								</div>
							</div>
							<div>
								<dt class="text-muted-foreground text-xs font-medium uppercase">{t('intel.field_source_url', 'Source URL')}</dt>
								<dd>
									<a href={source.source_url} target="_blank" rel="noopener noreferrer" class="text-accent-foreground inline-flex items-center gap-1 truncate font-mono text-xs hover:underline">
										{source.source_url}
										<ExternalLink class="size-3 shrink-0" />
									</a>
								</dd>
							</div>
							<div class="grid grid-cols-2 gap-3">
								<div>
									<dt class="text-muted-foreground text-xs font-medium uppercase">{t('intel.last_crawled', 'Last Crawled')}</dt>
									<dd class="text-foreground text-xs">{formatTime(source.last_crawled_at)}</dd>
								</div>
								<div>
									<dt class="text-muted-foreground text-xs font-medium uppercase">{t('intel.field_enabled', 'Enabled')}</dt>
									<dd class="text-xs">
										{#if source.enabled}
											<span class="text-emerald-500">{t('intel.enabled', 'Enabled')}</span>
										{:else}
											<span class="text-muted-foreground">{t('intel.disabled', 'Disabled')}</span>
										{/if}
									</dd>
								</div>
							</div>
						</dl>
					{:else}
						<div class="text-muted-foreground py-8 text-center text-sm">
							<Database class="mx-auto size-8 opacity-30" />
							<p class="mt-2">{t('intel.no_source', 'Source information unavailable')}</p>
						</div>
					{/if}
				</div>
			</div>

			<!-- Pane 2: Raw content -->
			<RawContentPane document={rawDoc} loading={false} />

			<!-- Pane 3: Extraction -->
			<ExtractionPane
				extracted={record.extracted}
				confidence={record.confidence}
				evidence={record.evidence}
				confidenceOverall={record.confidence_overall ?? null}
				loading={false}
			/>

			<!-- Pane 4: Structured record -->
			<div class="border-border-card bg-background-alt shadow-card flex flex-col rounded-xl">
				<div class="border-border-card flex items-center justify-between border-b px-4 py-3">
					<div class="flex items-center gap-2">
						<ListTree class="text-muted-foreground size-4" />
						<h3 class="text-foreground text-sm font-semibold">{t('intel.structured_record', 'Structured Record')}</h3>
					</div>
					{#if canEdit && !editMode && record?.promoted_record_id}
						<button
							class="text-muted-foreground hover:bg-muted hover:text-foreground flex items-center gap-1 rounded-md px-2 py-1 text-xs transition-colors"
							onclick={() => { editMode = true; editJson = structuredJson || safeStringify(record?.extracted); }}
						>
							<Pencil class="size-3" />
							{t('intel.edit', 'Edit')}
						</button>
					{/if}
				</div>
				<div class="flex-1 overflow-y-auto p-4">
					{#if editMode}
						<div>
							<label for="edit-json" class="text-muted-foreground mb-1 block text-xs font-medium">
								{t('intel.edit_extracted', 'Edit extracted fields (JSON)')}
							</label>
							<textarea
								id="edit-json"
								bind:value={editJson}
								rows="14"
								class="border-border-input bg-background text-foreground font-mono w-full rounded-lg border p-3 text-xs leading-relaxed focus:shadow-date-field-focus focus:outline-none"
							></textarea>
							<div class="mt-3 flex justify-end gap-2">
								<button
									class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors"
									onclick={() => { editMode = false; }}
								>
									{t('common.cancel', 'Cancel')}
								</button>
								<button
									class="bg-foreground text-background flex items-center gap-1.5 rounded-9px px-3 py-1.5 text-xs font-medium transition-colors hover:opacity-90 disabled:opacity-50"
									disabled={actionLoading === 'save'}
									onclick={saveEdit}
								>
									{#if actionLoading === 'save'}
										<Loader2 class="size-3.5 animate-spin" />
									{:else}
										<Save class="size-3.5" />
									{/if}
									{t('common.save', 'Save')}
								</button>
							</div>
						</div>
					{:else if structuredJson}
						<div class="border-border-card bg-background overflow-auto rounded-lg border p-3">
							<pre class="text-foreground whitespace-pre-wrap break-words font-mono text-xs leading-relaxed">{structuredJson}</pre>
						</div>
					{:else}
						<div class="text-muted-foreground py-8 text-center text-sm">
							<ListTree class="mx-auto size-8 opacity-30" />
							<p class="mt-2">{t('intel.no_structured_record', 'No structured record promoted yet')}</p>
							<p class="text-muted-foreground mt-1 text-xs">{t('intel.approve_to_promote', 'Approve to promote this record to the structured projection.')}</p>
						</div>
					{/if}
				</div>
			</div>
		</div>
	{/if}
{/if}
