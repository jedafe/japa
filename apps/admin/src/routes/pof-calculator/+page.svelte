<script lang="ts">
	// /pof-calculator — POF requirement management (§108).
	// Backend: GET/POST/PATCH/DELETE /admin/pof-requirements, POST .../generate.
	// Ref: sections/108-pof-calculator.md, ADR-119, ADR-088 (HITL review).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { api, apiRaw, adminCatalogApi, type CatalogOption } from '$lib/api';
	import { useCountries } from '$lib/utils/countries.svelte';
	import { loadCurrencies, currencyOptions, currencyLabel, type CurrencyInfo } from '$lib/utils/currency';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { CheckCircle, XCircle, Eye, Pencil, Trash2, Sparkles } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type Requirement = {
		id: string;
		destination_space_id: string;
		destination_space_name?: string;
		destination_space_code?: string;
		visa_type: string;
		program_label: string;
		amount_schedule: unknown;
		currency: string;
		funds_must_be_liquid?: boolean;
		accepted_proof?: unknown;
		notes?: string;
		effective_date?: string;
		source_url?: string;
		source_url_last_checked?: string;
		status: string;
		is_active?: boolean;
		ai_generated?: boolean;
		is_japabot_indexed?: boolean;
		created_at: string;
		updated_at?: string;
	};

	const canManage = $derived(hasPermission('pof_calculator.manage'));

	// Canonical visa_type values from §108 — used only as fallback while the
	// tools.pof_calculator.visa_type catalog loads. Distinct values seen in
	// loaded rows are merged in so off-enum legacy rows can still be
	// re-selected while editing.
	const SPEC_VISA_TYPES = ['express_entry', 'study_permit', 'work_visa', 'family', 'visitor', 'job_seeker', 'other'];
	let knownVisaTypes = $state<string[]>([]);
	let visaTypeOptions = $state<CatalogOption[]>(
		SPEC_VISA_TYPES.map((v) => ({ value: v, label: humanize(v) }))
	);

	// ── Country selector (250 primary geo space countries) ────────────────
	const countryUtils = useCountries();
	$effect(() => {
		countryUtils.load();
	});

	let pageError = $state('');
	let acting = $state(false);
	let controller = $state<EntityPageController | undefined>(undefined);
	// §58 — managed currency list for the currency select + display labels.
	let currencies = $state<CurrencyInfo[]>([]);
	$effect(() => { loadCurrencies().then((list) => (currencies = list)); });

	// Status options — catalog-driven with spec fallback (ADR-088 statuses).
	let statusOptions = $state<CatalogOption[]>([
		{ value: 'draft', label: 'Draft' },
		{ value: 'ai_draft', label: 'AI Draft' },
		{ value: 'pending', label: 'Pending' },
		{ value: 'published', label: 'Published' },
		{ value: 'rejected', label: 'Rejected' }
	]);
	$effect(() => {
		void (async () => {
			const res = await adminCatalogApi.get('tools.pof_calculator.status');
			if (res.ok && res.data?.length) statusOptions = res.data;
		})();
	});
	$effect(() => {
		void (async () => {
			const res = await adminCatalogApi.get('tools.pof_calculator.visa_type');
			if (res.ok && res.data?.length) visaTypeOptions = res.data;
		})();
	});

	function humanize(value: string): string {
		return value
			.split(/[_-]+/)
			.filter(Boolean)
			.map((w) => w.charAt(0).toUpperCase() + w.slice(1))
			.join(' ');
	}

	// ── Autocomplete helpers ──────────────────────────────────────────────
	const countryReady = $derived(countryUtils.ready);

	function fetchCountryId(q: string) {
		return countryUtils.fetcher(q);
	}
	function fetchCountryIdInitial() {
		return countryUtils.initialFetcher();
	}
	function getCountryIdLabel(id: string) {
		return countryUtils.getSelectedLabel(id);
	}

	function visaTypeItems(q: string): AutocompleteItem[] {
		const needle = q.trim().toLowerCase();
		const opts = new Map<string, string>(
			visaTypeOptions.map((o) => [o.value, o.label])
		);
		for (const v of knownVisaTypes) if (!opts.has(v)) opts.set(v, humanize(v));
		return [...opts.entries()]
			.filter(([v, l]) => !needle || v.toLowerCase().includes(needle) || l.toLowerCase().includes(needle))
			.map(([v, l]) => ({ id: v, label: l }));
	}
	function fetchVisaType(q: string) {
		return Promise.resolve(visaTypeItems(q));
	}
	function fetchVisaTypeInitial() {
		return Promise.resolve(visaTypeItems(''));
	}
	function getVisaTypeLabel(id: string) {
		return visaTypeOptions.find((o) => o.value === id)?.label ?? humanize(id);
	}

	// ── Form fields (drives EntityForm for create/edit) ───────────────────
	const fields = $derived.by<FieldDef[]>(() => {
		countryReady; // re-derive when the geo space countries load
		knownVisaTypes; // re-derive when list rows reveal new visa types
		return [
			{
				key: 'destination_space_id',
				label: t('app.tools.col_dest_country', 'Destination Country'),
				type: 'autocomplete',
				required: true,
				placeholder: t('app.tools.country_placeholder', 'Search country…'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: fetchCountryId,
				initialFetcher: fetchCountryIdInitial,
				getSelectedLabel: getCountryIdLabel
			},
			{
				key: 'visa_type',
				label: t('app.tools.col_visa_type', 'Visa Type'),
				type: 'autocomplete',
				required: true,
				placeholder: t('app.tools.visa_type_placeholder', 'Search visa type…'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: fetchVisaType,
				initialFetcher: fetchVisaTypeInitial,
				getSelectedLabel: getVisaTypeLabel
			},
			{ key: 'program_label', label: t('app.tools.col_program_label', 'Program label'), type: 'text', required: true },
			currencies.length > 0
				? { key: 'currency', label: t('app.tools.col_currency', 'Currency'), type: 'select' as const, required: true, options: currencyOptions(currencies) }
				: { key: 'currency', label: t('app.tools.col_currency', 'Currency'), type: 'text' as const, required: true, placeholder: 'CAD|GBP|EUR' },
			{ key: 'effective_date', label: t('app.tools.col_effective_date', 'Effective date'), type: 'date', required: true },
			{ key: 'source_url', label: t('app.tools.col_source_url', 'Source URL'), type: 'url' },
			{
				key: 'amount_schedule',
				label: t('app.tools.col_amount_schedule', 'Amount schedule (JSON)'),
				type: 'json',
				required: true,
				placeholder: '[{"members":1,"amount":14500}] or {"base":13347,"base_members":1,"per_additional":680}',
				help: t('app.tools.amount_schedule_help', 'Band array or formula object — resolved per household size')
			},
			{ key: 'funds_must_be_liquid', label: t('app.tools.col_funds_liquid', 'Funds must be liquid'), type: 'checkbox' },
			{ key: 'accepted_proof', label: t('app.tools.col_accepted_proof', 'Accepted proof (JSON array)'), type: 'json', placeholder: '["bank_statement","gift_deed"]' },
			{ key: 'notes', label: t('app.tools.col_notes', 'Notes'), type: 'textarea' },
			{ key: 'is_active', label: t('app.tools.col_is_active', 'Active'), type: 'checkbox', help: t('app.tools.is_active_help', 'Inactive requirements are hidden from the public calculator') }
		];
	});

	// ── JSON field (de)serialization ──────────────────────────────────────
	// amount_schedule/accepted_proof are JSONB columns. EntityPage seeds the
	// edit form straight from the row, so rows are mapped to pretty-printed
	// JSON text for the editors; normalizeBody parses them back on submit.
	function toJsonText(v: unknown): string {
		if (v == null) return '';
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

	function proofList(v: unknown): string[] {
		if (Array.isArray(v)) return v.map(String);
		if (typeof v === 'string' && v.trim()) {
			try {
				const p = JSON.parse(v);
				return Array.isArray(p) ? p.map(String) : [v];
			} catch {
				return [v];
			}
		}
		return [];
	}

	// ── JSON field normalization ──────────────────────────────────────────
	// amount_schedule/accepted_proof are JSONB columns; EntityForm gives raw
	// editor strings which must be parsed before hitting the API (a quoted
	// JSON string would fail backend schedule validation).
	function normalizeBody(body: Record<string, unknown>): { body?: Record<string, unknown>; error?: string } {
		const out = { ...body };
		for (const key of ['amount_schedule', 'accepted_proof']) {
			const v = out[key];
			if (typeof v === 'string') {
				const trimmed = v.trim();
				if (!trimmed) {
					delete out[key];
					continue;
				}
				try {
					out[key] = JSON.parse(trimmed);
				} catch {
					return { error: t('app.tools.invalid_json', '{field} must be valid JSON').replace('{field}', key) };
				}
			}
		}
		if (out.is_active === undefined) delete out.is_active;
		return { body: out };
	}

	// ── Generate AI draft modal ───────────────────────────────────────────
	let showGenerateModal = $state(false);
	let generateDestSpaceId = $state('');
	let generateVisaType = $state('');

	function openGenerate() {
		generateDestSpaceId = '';
		generateVisaType = '';
		showGenerateModal = true;
	}

	async function confirmGenerate() {
		if (!generateDestSpaceId) {
			pageError = t('app.tools.dest_required', 'Destination is required');
			return;
		}
		acting = true;
		pageError = '';
		try {
			const res = await api('/admin/pof-requirements/generate', {
				method: 'POST',
				body: {
					destination_space_id: generateDestSpaceId,
					...(generateVisaType ? { visa_type: generateVisaType } : {})
				}
			});
			if (res.ok) {
				showGenerateModal = false;
				controller?.reload();
			} else {
				pageError = res.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	// ── HITL actions ──────────────────────────────────────────────────────
	async function approve(row: Requirement) {
		acting = true;
		pageError = '';
		try {
			const res = await api(`/admin/pof-requirements/${row.id}/approve`, { method: 'PATCH' });
			if (!res.ok) { pageError = res.error ?? t('common.error', 'An error occurred'); return; }
			controller?.reload();
		} finally {
			acting = false;
		}
	}

	async function reject(row: Requirement) {
		acting = true;
		pageError = '';
		try {
			const res = await api(`/admin/pof-requirements/${row.id}/reject`, { method: 'PATCH' });
			if (!res.ok) { pageError = res.error ?? t('common.error', 'An error occurred'); return; }
			controller?.reload();
		} finally {
			acting = false;
		}
	}

	// ── Row actions ───────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: Requirement, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('common.edit', 'Edit'),
				icon: Pencil,
				onclick: (row: Requirement, ctx) => ctx?.openEdit(row)
			});
			acts.push({
				label: t('common.approve', 'Approve'),
				icon: CheckCircle,
				show: (row: Requirement) => ['draft', 'ai_draft', 'pending'].includes(row.status),
				onclick: (row: Requirement) => approve(row)
			});
			acts.push({
				label: t('common.reject', 'Reject'),
				icon: XCircle,
				variant: 'destructive',
				show: (row: Requirement) => ['draft', 'ai_draft', 'pending'].includes(row.status),
				onclick: (row: Requirement) => reject(row)
			});
			acts.push({
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				show: (row: Requirement) => row.status !== 'published',
				onclick: (row: Requirement, ctx) => ctx?.openDelete(row)
			});
		}
		return acts;
	});

	// ── Filters ───────────────────────────────────────────────────────────
	const statusFilterOptions = $derived<CatalogOption[]>([
		{ value: '', label: t('common.all', 'All') },
		...statusOptions
	]);

	const config = $derived<EntityPageConfig>({
		title: t('app.tools.pof_requirements', 'POF Requirements'),
		description: t('app.tools.pof_requirements_desc', 'Proof-of-funds amounts per destination + visa type (§108)'),
		createPermission: 'pof_calculator.manage',
		editPermission: 'pof_calculator.manage',
		deletePermission: 'pof_calculator.manage',
		pageSize: 25,
		fields,
		columns: [
			{ key: 'destination_space_name', label: t('app.tools.col_destination', 'Destination'), sortable: true, render: (v: string) => v || '—' },
			{ key: 'visa_type', label: t('app.tools.col_visa_type', 'Visa Type'), sortable: true, render: (v: string) => humanize(v || '') },
			{ key: 'program_label', label: t('app.tools.col_program_label', 'Program'), sortable: true },
			{ key: 'currency', label: t('app.tools.col_currency', 'Currency'), sortable: true, render: (v: string) => currencyLabel(v, currencies) },
			{ key: 'effective_date', label: t('app.tools.col_effective_date', 'Effective'), sortable: true, render: (v: string) => v || '—' },
			{
				key: 'status', label: t('app.tools.col_status', 'Status'), sortable: true,
				render: (v: string) => v === 'published' ? '<span style="color:var(--color-success)">published</span>' : v === 'ai_draft' ? '<span style="color:var(--color-warning)">ai_draft</span>' : (v || '—')
			},
			{ key: 'ai_generated', label: t('app.tools.col_ai_generated', 'AI'), render: (v: boolean) => v ? '✦' : '—' },
			{ key: 'is_active', label: t('app.tools.col_is_active', 'Active'), render: (v: boolean) => v ? t('common.yes', 'Yes') : t('common.no', 'No') },
			{ key: 'updated_at', label: t('app.tools.col_updated_at', 'Updated'), sortable: true, render: (v: string) => v ? new Date(v).toLocaleDateString() : '—' }
		],
		filters: [
			{ key: 'status', label: t('app.tools.col_status', 'Status'), options: statusFilterOptions },
			{
				key: 'dest',
				label: t('app.tools.col_destination', 'Destination'),
				type: 'autocomplete',
				placeholder: t('app.tools.country_placeholder', 'Search country…'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: fetchCountryId,
				initialFetcher: fetchCountryIdInitial,
				getSelectedLabel: getCountryIdLabel
			},
			{
				key: 'visa_type',
				label: t('app.tools.col_visa_type', 'Visa Type'),
				type: 'autocomplete',
				placeholder: t('app.tools.visa_type_placeholder', 'Search visa type…'),
				emptyMessage: t('common.no_results', 'No results found'),
				fetcher: fetchVisaType,
				initialFetcher: fetchVisaTypeInitial,
				getSelectedLabel: getVisaTypeLabel
			}
		],
		load: async (params) => {
			// Server-side pagination — pass page/limit/q/sort/filters to the
			// backend (geo-aware-tool-pattern.md admin list contract).
			const queryParts: string[] = [];
			queryParts.push(`page=${(params.page ?? 0) + 1}`);
			queryParts.push(`limit=${params.pageSize ?? 25}`);
			if (params.q) queryParts.push(`q=${encodeURIComponent(params.q)}`);
			if (params.sort) queryParts.push(`sort=${encodeURIComponent(params.sort)}`);
			if (params.filter?.status) queryParts.push(`status=${encodeURIComponent(params.filter.status)}`);
			if (params.filter?.dest) queryParts.push(`dest=${encodeURIComponent(params.filter.dest)}`);
			if (params.filter?.visa_type) queryParts.push(`visa_type=${encodeURIComponent(params.filter.visa_type)}`);

			const res = await apiRaw<{ data: Requirement[]; pagination?: { page: number; limit: number; total: number } }>(
				`/admin/pof-requirements?${queryParts.join('&')}`
			);
			if (!res.ok) {
				throw new Error(res.error ?? t('common.error', 'An error occurred'));
			}
			const data = (res.data?.data ?? []).map((r) => ({
				...r,
				amount_schedule: toJsonText(r.amount_schedule),
				accepted_proof: toJsonText(r.accepted_proof)
			}));
			const total = res.data?.pagination?.total ?? data.length;
			// Merge visa types seen in the data so the pickers can re-select
			// off-enum legacy values.
			const seen = new Set(knownVisaTypes);
			for (const r of data) if (r.visa_type) seen.add(r.visa_type);
			knownVisaTypes = [...seen];
			return { data, total, totalPages: Math.max(1, Math.ceil(total / (params.pageSize ?? 25))) };
		},
		create: async (body) => {
			const norm = normalizeBody(body);
			if (norm.error) return { ok: false, error: norm.error };
			const res = await api('/admin/pof-requirements', { method: 'POST', body: norm.body });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const norm = normalizeBody(body);
			if (norm.error) return { ok: false, error: norm.error };
			const res = await api(`/admin/pof-requirements/${id}`, { method: 'PATCH', body: norm.body });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/pof-requirements/${id}`, { method: 'DELETE' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		actions: rowActions,
		viewSnippet: RequirementView
	});
</script>

<svelte:head>
	<title>{t('app.tools.pof_page_title', 'POF Requirements — {site} Admin')}</title>
</svelte:head>

{#if pageError}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{pageError}
	</div>
{/if}

{#if canManage}
	<div class="mb-4 flex justify-end">
		<Button onclick={openGenerate}>
			<Sparkles size={16} />
			{t('app.tools.generate', 'Generate')}
		</Button>
	</div>
{/if}

<EntityPage {config} bind:controller />

<!-- ── Generate modal ──────────────────────────────────────────────────── -->
<Modal bind:open={showGenerateModal} title={t('app.tools.generate_pof_requirement', 'Generate POF Requirement Draft')}>
	<div class="space-y-4">
		<div>
			<label for="gen-dest" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_dest_country', 'Destination Country')}</label>
			<Autocomplete
				id="gen-dest"
				placeholder={t('app.tools.country_placeholder', 'Search country…')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={countryUtils.fetcher}
				initialFetcher={countryUtils.initialFetcher}
				selectedId={generateDestSpaceId || null}
				selectedLabel={generateDestSpaceId ? countryUtils.getSelectedLabel(generateDestSpaceId) : null}
				onSelect={(item: AutocompleteItem) => { generateDestSpaceId = item.id; }}
				onClear={() => { generateDestSpaceId = ''; }}
			/>
		</div>
		<div>
			<label for="gen-visa-type" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.tools.col_visa_type', 'Visa Type (optional)')}</label>
			<Autocomplete
				id="gen-visa-type"
				placeholder={t('app.tools.visa_type_placeholder', 'Search visa type…')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={fetchVisaType}
				initialFetcher={fetchVisaTypeInitial}
				selectedId={generateVisaType || null}
				selectedLabel={generateVisaType ? humanize(generateVisaType) : null}
				onSelect={(item: AutocompleteItem) => { generateVisaType = item.id; }}
				onClear={() => { generateVisaType = ''; }}
			/>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showGenerateModal = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button onclick={confirmGenerate} disabled={acting || !generateDestSpaceId}>
			{acting ? t('app.tools.generating', 'Generating...') : t('app.tools.generate', 'Generate')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ────────────────────────────────────────────── -->
{#snippet RequirementView({ row }: { row: Requirement })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_program_label', 'Program')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.program_label}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_dest_country', 'Destination')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.destination_space_name || row.destination_space_id}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_visa_type', 'Visa Type')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{humanize(row.visa_type || '—')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_amount_schedule', 'Amount schedule')}</p>
			<pre class="mt-1 overflow-x-auto rounded-lg border p-2 text-xs" style="border-color: var(--color-border); color: var(--color-foreground)">{toJsonText(row.amount_schedule)}</pre>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_currency', 'Currency')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{currencyLabel(row.currency, currencies)}</p>
		</div>
		{#if row.accepted_proof}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_accepted_proof', 'Accepted proof')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{proofList(row.accepted_proof).map(humanize).join(', ')}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_funds_liquid', 'Funds must be liquid')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.funds_must_be_liquid ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_effective_date', 'Effective date')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.effective_date || '—'}</p>
		</div>
		{#if row.source_url}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_source_url', 'Source URL')}</p>
				<a href={row.source_url} target="_blank" rel="noopener noreferrer" class="mt-1 block text-sm underline" style="color: var(--color-primary)">{row.source_url}</a>
			</div>
		{/if}
		{#if row.source_url_last_checked}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_source_checked', 'Source last checked')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.source_url_last_checked}</p>
			</div>
		{/if}
		{#if row.notes}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_notes', 'Notes')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.notes}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_ai_generated', 'AI Generated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.ai_generated ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_is_active', 'Active')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_active ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.tools.col_updated_at', 'Updated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.updated_at ? new Date(row.updated_at).toLocaleString() : '—'}</p>
		</div>
	</div>
{/snippet}
