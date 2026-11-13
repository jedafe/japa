<script lang="ts">
	import { siteName } from '#lib/stores/site.svelte.js';
	import { t } from '#lib/stores/i18n.svelte.js';
	// /eca-guide — ECA provider + guide management + community update moderation (§109).
	// Backend: GET/POST/PATCH/DELETE /admin/eca-providers, GET/POST/PATCH/DELETE /admin/eca-guides,
	//          GET/PATCH /admin/eca-guides/updates.
	// Ref: sections/109-eca-guide.md, ADR-120.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '#lib/components/entity/index.js';
	import { api, apiRaw } from '#lib/api.js';
	import { loadCurrencies, currencyOptions, currencyLabel, currencySymbol, type CurrencyInfo } from '#lib/utils/currency.js';
	import { hasPermission } from '#lib/stores/permissions.svelte.js';
	import { initTabParam, bindTabParam } from '#lib/utils/url-sync.svelte.js';
	import { CheckCircle, XCircle, Eye } from 'lucide-svelte';
	import AutocompleteSelect from '#lib/components/ui/AutocompleteSelect.svelte';

	type Provider = {
		id: string;
		slug: string;
		name: string;
		website?: string;
		is_active?: boolean;
		created_at: string;
	};

	type Guide = {
		id: string;
		destination_space_id: string;
		destination_space_name?: string;
		provider_id: string;
		provider_name?: string;
		purpose: string;
		pathway_steps?: unknown;
		documents_required?: unknown;
		accepted_for?: unknown;
		fee_estimate?: string;
		currency?: string;
		timeline_estimate?: string;
		official_url?: string;
		status: string;
		is_active?: boolean;
		ai_generated?: boolean;
		created_at: string;
	};

	type CommunityUpdate = {
		id: string;
		guide_id: string;
		user_name?: string;
		step_index?: number;
		fee_estimate?: string;
		turnaround_days?: number;
		note?: string;
		status: string;
		created_at: string;
		destination_space_name?: string;
		provider_name?: string;
		guide_purpose?: string;
	};

	const canManage = $derived(hasPermission('eca_guide.manage'));

	const ECA_TABS = ['providers', 'guides', 'updates'] as const;
	let activeTab = $state<'providers' | 'guides' | 'updates'>(initTabParam('providers', { valid: ECA_TABS }));
	bindTabParam(() => activeTab, (v) => (activeTab = v), { valid: ECA_TABS, preserve: [] });
	let pageError = $state('');
	// §58 — managed currency list for the guide currency select.
	let currencies = $state<CurrencyInfo[]>([]);
	$effect(() => { loadCurrencies().then((list) => (currencies = list)); });

	// Reference lists for select fields — destination countries (public
	// /geo/countries) and ECA providers (/admin/eca-providers). Loaded once;
	// empty lists degrade the fields to free-text ids.
	type GeoOption = { id: string; name: string };
	let geoOptions = $state<GeoOption[]>([]);
	let providerOptions = $state<Provider[]>([]);
	$effect(() => {
		api<GeoOption[]>('/geo/countries').then((res) => {
			if (res.ok) geoOptions = res.data ?? [];
		});
		api<Provider[]>('/admin/eca-providers').then((res) => {
			if (res.ok) providerOptions = res.data ?? [];
		});
	});

	// ── Providers tab ─────────────────────────────────────────────────────────

	let providerController = $state<EntityPageController | undefined>(undefined);

	const providerFields: FieldDef[] = [
		{ key: 'slug', label: t('eca.col_slug', 'Slug'), type: 'text', required: true, placeholder: 'wes|iqas|ces' },
		{ key: 'name', label: t('eca.col_name', 'Name'), type: 'text', required: true, placeholder: 'World Education Services' },
		{ key: 'website', label: t('eca.col_website', 'Website'), type: 'url', placeholder: 'https://wes.org' },
		{ key: 'is_active', label: t('common.active', 'Active'), type: 'checkbox' },
	];

	async function loadProviders(p: { page: number; pageSize: number; q: string; sort: string; filter: Record<string, string> }) {
		const params = new URLSearchParams();
		params.set('page', String(p.page + 1));
		params.set('limit', String(p.pageSize));
		if (p.q) params.set('q', p.q);
		if (p.sort) params.set('sort', p.sort);
		// apiRaw keeps the { data, pagination } envelope for server pagination.
		const res = await apiRaw<{ data?: Provider[]; pagination?: { total: number } }>(
			`/admin/eca-providers?${params}`
		);
		if (res.ok) {
			const data = res.data?.data ?? [];
			const total = res.data?.pagination?.total ?? data.length;
			return { data, total, totalPages: Math.max(1, Math.ceil(total / p.pageSize)) };
		}
		throw new Error(res.error ?? 'Failed to load providers');
	}

	const providerConfig: EntityPageConfig = {
		title: t('eca.providers_title', 'ECA Providers'),
		description: t('eca.providers_desc', 'Credential-assessment organisations (WES, IQAS, CES, etc.)'),
		createPermission: 'eca_guide.manage',
		editPermission: 'eca_guide.manage',
		deletePermission: 'eca_guide.manage',
		fields: providerFields,
		columns: [
			{ key: 'name', label: t('eca.col_name', 'Name'), sortable: true },
			{ key: 'slug', label: t('eca.col_slug', 'Slug') },
			{ key: 'website', label: t('eca.col_website', 'Website'), render: (v?: string) => v ? `<a href="${v}" target="_blank" rel="noopener noreferrer" style="color:var(--color-primary)">${v}</a>` : '—' },
			{ key: 'is_active', label: t('common.active', 'Active'), render: (v?: boolean) => v === false ? '—' : '✓' },
		],
		load: loadProviders,
		create: async (body) => {
			const res = await api('/admin/eca-providers', { method: 'POST', body });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await api(`/admin/eca-providers/${id}`, { method: 'PATCH', body });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/eca-providers/${id}`, { method: 'DELETE' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		isDeleted: (row: Provider) => row.is_active === false,
	};

	// ── Guides tab ────────────────────────────────────────────────────────────

	let guideController = $state<EntityPageController | undefined>(undefined);

	const guideFields = $derived<FieldDef[]>([
		geoOptions.length > 0
			? { key: 'destination_space_id', label: t('eca.destination', 'Destination'), type: 'select', required: true,
				options: geoOptions.map((g) => ({ value: g.id, label: g.name })) }
			: { key: 'destination_space_id', label: t('eca.destination_id', 'Destination space ID'), type: 'text', required: true },
		providerOptions.length > 0
			? { key: 'provider_id', label: t('eca.provider', 'Provider'), type: 'select', required: true,
				options: providerOptions.map((p) => ({ value: p.id, label: p.name })) }
			: { key: 'provider_id', label: t('eca.provider_id', 'Provider ID'), type: 'text', required: true },
		{ key: 'purpose', label: t('eca.purpose', 'Purpose'), type: 'select', required: true, options: [
			{ value: 'immigration', label: t('eca.purpose_immigration', 'Immigration') }, { value: 'study', label: t('eca.purpose_study', 'Study') },
			{ value: 'employment', label: t('eca.purpose_employment', 'Employment') }, { value: 'professional_licensure', label: t('eca.purpose_licensure', 'Professional licensure') },
		]},
		{ key: 'pathway_steps', label: t('eca.field_pathway_steps', 'Pathway steps (JSON)'), type: 'json', required: true },
		{ key: 'documents_required', label: t('eca.field_documents', 'Documents required (JSON)'), type: 'json' },
		{ key: 'accepted_for', label: t('eca.field_accepted', 'Accepted for (JSON)'), type: 'json' },
		{ key: 'fee_estimate', label: t('eca.field_fee', 'Fee estimate'), type: 'text' },
		currencies.length > 0
			? { key: 'currency', label: t('eca.col_currency', 'Currency'), type: 'select', options: currencyOptions(currencies) }
			: { key: 'currency', label: t('eca.col_currency', 'Currency'), type: 'text' },
		{ key: 'timeline_estimate', label: t('eca.field_timeline', 'Timeline estimate'), type: 'text' },
		{ key: 'official_url', label: t('eca.field_official_url', 'Official URL'), type: 'url' },
	]);

	// pathway_steps / documents_required / accepted_for are jsonb columns —
	// the json editor fields submit JSON strings, so parse them back to real JSON
	// before they hit the API (an unparseable value passes through and the
	// backend validation reports it).
	function parseJsonFields(body: Record<string, unknown>): Record<string, unknown> {
		const out = { ...body };
		for (const key of ['pathway_steps', 'documents_required', 'accepted_for'] as const) {
			const v = out[key];
			if (typeof v !== 'string') continue;
			const s = v.trim();
			if (!s) { delete out[key]; continue; }
			try { out[key] = JSON.parse(s); } catch { /* leave as-is */ }
		}
		return out;
	}

	async function loadGuides(p: { page: number; pageSize: number; q: string; sort: string; filter: Record<string, string> }) {
		const params = new URLSearchParams();
		params.set('page', String(p.page + 1));
		params.set('limit', String(p.pageSize));
		if (p.q) params.set('q', p.q);
		if (p.sort) params.set('sort', p.sort);
		if (p.filter?.status) params.set('status', p.filter.status);
		if (p.filter?.dest) params.set('dest', p.filter.dest);
		// apiRaw keeps the { data, pagination } envelope — api() would strip
		// to body.data and lose the total needed for pagination.
		const res = await apiRaw<{ data?: Guide[]; pagination?: { total: number } }>(`/admin/eca-guides?${params}`);
		if (res.ok) {
			// Stringify jsonb fields so the json editor inputs in EntityForm seed
			// editable JSON instead of "[object Object]" (parseJsonFields
			// converts them back on save).
			const data = (res.data?.data ?? []).map((g) => ({
				...g,
				pathway_steps: g.pathway_steps ? JSON.stringify(g.pathway_steps, null, 2) : '',
				documents_required: g.documents_required ? JSON.stringify(g.documents_required, null, 2) : '',
				accepted_for: g.accepted_for ? JSON.stringify(g.accepted_for, null, 2) : ''
			}));
			const total = res.data?.pagination?.total ?? data.length;
			return { data, total, totalPages: Math.max(1, Math.ceil(total / p.pageSize)) };
		}
		throw new Error(res.error ?? 'Failed to load guides');
	}

	async function approveGuide(row: Guide) {
		const res = await api(`/admin/eca-guides/${row.id}/approve`, { method: 'PATCH' });
		if (!res.ok) { pageError = res.error ?? 'Approve failed'; return; }
		guideController?.reload();
	}

	async function rejectGuide(row: Guide) {
		const res = await api(`/admin/eca-guides/${row.id}/reject`, { method: 'PATCH' });
		if (!res.ok) { pageError = res.error ?? 'Reject failed'; return; }
		guideController?.reload();
	}

	// POST /admin/eca-guides/generate requires destination_space_id +
	// provider_id (+ optional purpose) — the draft form collects them
	// before enqueueing.
	let showGenerateForm = $state(false);
	let genDestId = $state('');
	let genProviderId = $state('');
	let genPurpose = $state('');
	let generating = $state(false);

	const purposeOptions = [
		{ value: '', label: t('eca.purpose_any', 'Any purpose') },
		{ value: 'immigration', label: t('eca.purpose_immigration', 'Immigration') },
		{ value: 'study', label: t('eca.purpose_study', 'Study') },
		{ value: 'employment', label: t('eca.purpose_employment', 'Employment') },
		{ value: 'professional_licensure', label: t('eca.purpose_licensure', 'Professional licensure') }
	];

	async function generateGuideDraft() {
		if (!genDestId || !genProviderId || generating) return;
		generating = true;
		pageError = '';
		const res = await api('/admin/eca-guides/generate', {
			method: 'POST',
			body: {
				destination_space_id: genDestId,
				provider_id: genProviderId,
				...(genPurpose ? { purpose: genPurpose } : {})
			}
		});
		generating = false;
		if (res.ok) {
			showGenerateForm = false;
			guideController?.reload();
		} else {
			pageError = res.error ?? 'Generate failed';
		}
	}

	const guideConfig = $derived<EntityPageConfig>({
		title: t('eca.guides_title', 'ECA Guides'),
		description: t('eca.guides_desc', 'Step-by-step credential-assessment guides per provider + destination'),
		createPermission: 'eca_guide.manage',
		editPermission: 'eca_guide.manage',
		deletePermission: 'eca_guide.manage',
		fields: guideFields,
		columns: [
			{ key: 'destination_space_name', label: t('eca.destination', 'Destination'), sortable: true },
			{ key: 'provider_name', label: t('eca.provider', 'Provider'), sortable: true },
			{ key: 'purpose', label: t('eca.purpose', 'Purpose') },
			{ key: 'fee_estimate', label: t('eca.col_fee', 'Fee'), render: (v: string, row: Guide) => {
				const fee = v ?? '';
				if (!fee) return '—';
				const sym = currencySymbol(row.currency, currencies);
				// fee_estimate may already embed the code (e.g. "200 USD").
				if (!sym || (row.currency && fee.toUpperCase().includes(row.currency.toUpperCase()))) return fee;
				return `${sym} ${fee}`;
			} },
			{ key: 'currency', label: t('eca.col_currency', 'Currency'), render: (v: string) => currencyLabel(v, currencies) },
			{
				key: 'status', label: t('common.status', 'Status'),
				render: (v: string) => v === 'published' ? '<span style="color:var(--color-success)">published</span>' : v === 'ai_draft' ? '<span style="color:var(--color-warning)">ai_draft</span>' : v
			},
		],
		filters: [
			{ key: 'status', label: t('common.status', 'Status'), options: [
				{ value: 'draft', label: t('common.draft', 'Draft') }, { value: 'ai_draft', label: t('common.ai_draft', 'AI Draft') },
				{ value: 'pending', label: t('common.pending', 'Pending') }, { value: 'published', label: t('common.published', 'Published') },
				{ value: 'rejected', label: t('common.rejected', 'Rejected') },
			]},
		],
		load: loadGuides,
		create: async (body) => {
			const res = await api('/admin/eca-guides', { method: 'POST', body: parseJsonFields(body) });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await api(`/admin/eca-guides/${id}`, { method: 'PATCH', body: parseJsonFields(body) });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/eca-guides/${id}`, { method: 'DELETE' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		isDeleted: (row: Guide) => row.status === 'rejected',
		actions: [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: Guide, ctx?: any) => ctx?.openView(row) },
			{
				label: t('eca.approve_published', 'Approve → Published'),
				icon: CheckCircle,
				show: (row: Guide) => ['draft', 'ai_draft', 'pending'].includes(row.status),
				onclick: (row: Guide) => approveGuide(row),
			},
			{
				label: t('common.reject', 'Reject'),
				icon: XCircle,
				variant: 'destructive',
				show: (row: Guide) => ['draft', 'ai_draft', 'pending'].includes(row.status),
				onclick: (row: Guide) => rejectGuide(row),
			},
			{
				label: t('common.delete', 'Delete'),
				icon: XCircle,
				variant: 'destructive',
				show: (_row: Guide) => canManage,
				onclick: (row: Guide, ctx?: any) => ctx?.openDelete(row),
			},
		],
	});

	// ── Community updates tab ─────────────────────────────────────────────────

	let updateController = $state<EntityPageController | undefined>(undefined);

	async function loadUpdates(p: { page: number; pageSize: number; q: string; sort: string; filter: Record<string, string> }) {
		const params = new URLSearchParams();
		params.set('page', String(p.page + 1));
		params.set('limit', String(p.pageSize));
		if (p.q) params.set('q', p.q);
		if (p.sort) params.set('sort', p.sort);
		if (p.filter?.status) params.set('status', p.filter.status);
		// apiRaw keeps the { data, pagination } envelope — api() would strip
		// to body.data and lose the total needed for pagination.
		const res = await apiRaw<{ data?: CommunityUpdate[]; pagination?: { total: number } }>(`/admin/eca-guides/updates?${params}`);
		if (res.ok) {
			const data = res.data?.data ?? [];
			const total = res.data?.pagination?.total ?? data.length;
			return { data, total, totalPages: Math.max(1, Math.ceil(total / p.pageSize)) };
		}
		throw new Error(res.error ?? 'Failed to load updates');
	}

	async function moderateUpdate(row: CommunityUpdate, status: 'approved' | 'rejected') {
		const res = await api(`/admin/eca-guides/updates/${row.id}`, { method: 'PATCH', body: { status } });
		if (!res.ok) { pageError = res.error ?? 'Moderation failed'; return; }
		updateController?.reload();
	}

	const updateConfig: EntityPageConfig = {
		title: t('eca.updates_title', 'Community Updates'),
		description: t('eca.updates_desc', 'Fee/turnaround reports submitted by members — moderate before publishing'),
		readonly: true,
		fields: [],
		columns: [
			{
				key: 'destination_space_name',
				label: t('eca.col_guide', 'Guide'),
				render: (_v: string, row: CommunityUpdate) =>
					[row.provider_name, row.destination_space_name].filter(Boolean).join(' — ') || row.guide_id
			},
			{ key: 'guide_purpose', label: t('eca.purpose', 'Purpose'), render: (v: string) => v || '—' },
			{ key: 'user_name', label: t('eca.col_reporter', 'Reporter') },
			{ key: 'step_index', label: t('eca.col_step', 'Step'), render: (v?: number) => v == null ? '—' : String(v) },
			{ key: 'fee_estimate', label: t('eca.col_fee', 'Fee') },
			{ key: 'turnaround_days', label: t('eca.col_days', 'Days') },
			{
				key: 'status', label: t('common.status', 'Status'),
				render: (v: string) => v === 'approved' ? '<span style="color:var(--color-success)">approved</span>' : v === 'pending' ? '<span style="color:var(--color-warning)">pending</span>' : v
			},
		],
		filters: [
			{ key: 'status', label: t('common.status', 'Status'), options: [
				{ value: 'pending', label: t('common.pending', 'Pending') }, { value: 'approved', label: t('common.approved', 'Approved') },
				{ value: 'rejected', label: t('common.rejected', 'Rejected') },
			]},
		],
		load: loadUpdates,
		actions: [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: CommunityUpdate, ctx?: any) => ctx?.openView(row) },
			{
				label: t('common.approve', 'Approve'),
				icon: CheckCircle,
				show: (row: CommunityUpdate) => row.status === 'pending',
				onclick: (row: CommunityUpdate) => moderateUpdate(row, 'approved'),
			},
			{
				label: t('common.reject', 'Reject'),
				icon: XCircle,
				variant: 'destructive',
				show: (row: CommunityUpdate) => row.status === 'pending',
				onclick: (row: CommunityUpdate) => moderateUpdate(row, 'rejected'),
			},
		],
	};
</script>

<svelte:head>
	<title>ECA Guide — {siteName()} Admin</title>
</svelte:head>

{#if pageError}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{pageError}
	</div>
{/if}

<div class="mb-4 flex flex-wrap gap-2">
	{#each ECA_TABS as tab}
		<button class="rounded px-3 py-1 text-sm font-medium" style="background:{activeTab===tab?'var(--color-primary)':'var(--color-muted-bg)'};color:{activeTab===tab?'var(--color-primary-contrast)':'var(--color-foreground)'}" onclick={() => activeTab=tab}>{t(`eca.tab_${tab}`, tab)}</button>
	{/each}
	{#if canManage && activeTab === 'guides'}
		<button class="ms-auto rounded px-3 py-1 text-sm" style="background:var(--color-warning);color:var(--color-primary-contrast)" onclick={() => (showGenerateForm = !showGenerateForm)}>{t('eca.generate_draft', 'Generate AI Draft')}</button>
	{/if}
</div>

{#if canManage && activeTab === 'guides' && showGenerateForm}
	<div class="mb-4 flex flex-wrap items-end gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
		<div class="min-w-52 flex-1">
			<label class="mb-1 block text-xs font-medium" for="gen-dest">{t('eca.destination', 'Destination')}</label>
			<AutocompleteSelect
				id="gen-dest"
				bind:value={genDestId}
				options={geoOptions.map((g) => ({ value: g.id, label: g.name }))}
				placeholder={t('eca.destination_ph', 'Select destination…')}
			/>
		</div>
		<div class="min-w-52 flex-1">
			<label class="mb-1 block text-xs font-medium" for="gen-provider">{t('eca.provider', 'Provider')}</label>
			<AutocompleteSelect
				id="gen-provider"
				bind:value={genProviderId}
				options={providerOptions.map((p) => ({ value: p.id, label: p.name }))}
				placeholder={t('eca.provider_ph', 'Select provider…')}
			/>
		</div>
		<div class="min-w-52 flex-1">
			<label class="mb-1 block text-xs font-medium" for="gen-purpose">{t('eca.purpose', 'Purpose')}</label>
			<AutocompleteSelect
				id="gen-purpose"
				bind:value={genPurpose}
				options={purposeOptions}
				placeholder={t('eca.purpose_ph', 'Any purpose')}
			/>
		</div>
		<button
			class="rounded px-3 py-2 text-sm font-medium text-[var(--color-primary-contrast)]"
			style="background:var(--color-primary);opacity:{!genDestId || !genProviderId || generating ? 0.5 : 1}"
			disabled={!genDestId || !genProviderId || generating}
			onclick={generateGuideDraft}
		>
			{generating ? t('eca.queueing', 'Queueing…') : t('eca.queue_draft', 'Queue draft')}
		</button>
	</div>
{/if}

{#if activeTab === 'providers'}
	<EntityPage config={providerConfig} bind:controller={providerController} />
{:else if activeTab === 'guides'}
	<EntityPage config={guideConfig} bind:controller={guideController} />
{:else}
	<EntityPage config={updateConfig} bind:controller={updateController} />
{/if}
