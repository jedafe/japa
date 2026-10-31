<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /sop-generator — SOP template + sample + field management (§110).
	// Backend: GET/POST/PATCH/DELETE /admin/sop/templates, /admin/sop/samples, /admin/sop/fields.
	// Ref: sections/110-sop-generator.md, ADR-121.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { api, apiRaw } from '$lib/api';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { initTabParam, bindTabParam } from '$lib/utils/url-sync.svelte';
	import { useCountries } from '$lib/utils/countries.svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { CheckCircle, XCircle, Eye } from 'lucide-svelte';

	type Template = {
		id: string;
		destination_space_id?: string;
		purpose: string;
		title: string;
		structure: unknown;
		status: string;
		is_active?: boolean;
		ai_generated?: boolean;
		created_at: string;
	};

	type Sample = {
		id: string;
		destination_space_name?: string;
		destination_space_id?: string;
		program_level: string;
		field_of_study?: string;
		title: string;
		body: string;
		word_count?: number;
		status: string;
		ai_generated?: boolean;
		created_at: string;
	};

	type Field = {
		id: string;
		name: string;
		is_active: boolean;
		sort_order: number;
		created_at: string;
	};

	const canManage = $derived(hasPermission('sop_generator.manage'));

	// Country (destination space) picker — Rule 16: countries come from
	// GET /geo/countries via the shared useCountries fetchers.
	const countrySel = useCountries('id');

	const SOP_TABS = ['templates', 'samples', 'fields'] as const;
	let activeTab = $state<'templates' | 'samples' | 'fields'>(initTabParam('templates', { valid: SOP_TABS }));
	bindTabParam(() => activeTab, (v) => (activeTab = v), { valid: SOP_TABS, preserve: [] });
	let pageError = $state('');

	// §110 enums — values are fixed by the API contract.
	const PURPOSE_OPTIONS = [
		{ value: 'study_permit', label: 'Study Permit' },
		{ value: 'student_visa', label: 'Student Visa' },
		{ value: 'work_visa', label: 'Work Visa' },
		{ value: 'visitor_visa', label: 'Visitor Visa' }
	];
	const LEVEL_OPTIONS = [
		{ value: 'diploma', label: 'Diploma' },
		{ value: 'bachelors', label: "Bachelor's" },
		{ value: 'masters', label: "Master's" },
		{ value: 'mba', label: 'MBA' },
		{ value: 'phd', label: 'PhD' }
	];

	// ── Templates tab ─────────────────────────────────────────────────────────

	let tplController = $state<EntityPageController | undefined>(undefined);

	const tplFields: FieldDef[] = [
		{
			key: 'destination_space_id',
			label: t('sop_generator.destination', 'Destination (blank = generic)'),
			type: 'autocomplete',
			placeholder: t('sop_generator.destination_ph', 'Search country…'),
			emptyMessage: t('common.no_results', 'No results found'),
			fetcher: countrySel.fetcher,
			initialFetcher: countrySel.initialFetcher,
			getSelectedLabel: countrySel.getSelectedLabel
		},
		{ key: 'purpose', label: t('sop_generator.purpose', 'Purpose'), type: 'select', required: true, options: PURPOSE_OPTIONS },
		{ key: 'title', label: t('sop_generator.title_label', 'Title'), type: 'text', required: true },
		{
			key: 'structure',
			label: t('sop_generator.structure', 'Structure (JSON)'),
			type: 'json',
			required: true,
			placeholder: '[{"section":"intro","prompt_hint":"Hook + intent","max_words":120}]',
			help: t('sop_generator.structure_help', 'Array of {section, prompt_hint, max_words} objects (§110)')
		},
		{ key: 'is_active', label: t('sop_generator.active', 'Active'), type: 'checkbox' },
	];

	async function loadTemplates(p: { page: number; pageSize: number; q: string; sort: string; filter: Record<string, string> }) {
		const params = new URLSearchParams();
		params.set('page', String(p.page + 1));
		params.set('limit', String(p.pageSize));
		const res = await apiRaw<{ data?: Template[]; pagination?: { total?: number } }>(`/admin/sop/templates?${params}`);
		if (res.ok) {
			const raw = res.data;
			const data = Array.isArray(raw) ? raw : (raw?.data ?? []);
			const total = Array.isArray(raw) ? data.length : (raw?.pagination?.total ?? data.length);
			return { data, total, totalPages: Math.max(1, Math.ceil(total / p.pageSize)) };
		}
		throw new Error(res.error ?? 'Failed to load templates');
	}

	async function approveTemplate(row: Template) {
		const res = await api(`/admin/sop/templates/${row.id}/approve`, { method: 'PATCH' });
		if (!res.ok) { pageError = res.error ?? 'Approve failed'; return; }
		tplController?.reload();
	}

	async function rejectTemplate(row: Template) {
		const res = await api(`/admin/sop/templates/${row.id}/reject`, { method: 'PATCH' });
		if (!res.ok) { pageError = res.error ?? 'Reject failed'; return; }
		tplController?.reload();
	}

	async function generateTemplateDraft() {
		const res = await api('/admin/sop/templates/generate', { method: 'POST', body: {} });
		if (res.ok) { tplController?.reload(); }
		else { pageError = res.error ?? 'Generate failed'; }
	}

	const tplConfig: EntityPageConfig = {
		title: t('sop_generator.templates', 'SOP Templates'),
		description: t('sop_generator.templates_desc', 'Structured outlines that guide AI generation per purpose'),
		createPermission: 'sop_generator.manage',
		editPermission: 'sop_generator.manage',
		deletePermission: 'sop_generator.manage',
		fields: tplFields,
		columns: [
			{ key: 'title', label: t('sop_generator.title_label', 'Title'), sortable: true },
			{ key: 'purpose', label: t('sop_generator.purpose', 'Purpose') },
			{
				key: 'status', label: t('sop_generator.status', 'Status'),
				render: (v: string) => v === 'published' ? '<span style="color:var(--color-success)">published</span>' : v === 'ai_draft' ? '<span style="color:var(--color-warning)">ai_draft</span>' : (v ?? '')
			},
			{ key: 'ai_generated', label: 'AI', render: (v: boolean) => v ? '✦' : '—' },
		],
		load: loadTemplates,
		create: async (body) => {
			const res = await api('/admin/sop/templates', { method: 'POST', body });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await api(`/admin/sop/templates/${id}`, { method: 'PATCH', body });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/sop/templates/${id}`, { method: 'DELETE' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		actions: [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: Template, ctx?: any) => ctx?.openView(row) },
			{
				label: t('sop_generator.approve', 'Approve → Published'),
				icon: CheckCircle,
				show: (row: Template) => ['draft', 'ai_draft', 'pending'].includes(row.status),
				onclick: (row: Template) => approveTemplate(row),
			},
			{
				label: t('common.reject', 'Reject'),
				icon: XCircle,
				variant: 'destructive',
				show: (row: Template) => ['draft', 'ai_draft', 'pending'].includes(row.status),
				onclick: (row: Template) => rejectTemplate(row),
			},
		],
	};

	// ── Samples tab ───────────────────────────────────────────────────────────

	let sampleController = $state<EntityPageController | undefined>(undefined);

	// Managed field-of-study options (sop_fields) — the sample form's field
	// picker is fed by GET /admin/sop/fields so stored values stay curated.
	// Items key by name: samples store the name string in field_of_study.
	let managedFieldItems: { id: string; label: string }[] | null = null;

	async function loadManagedFields(): Promise<{ id: string; label: string }[]> {
		if (managedFieldItems) return managedFieldItems;
		const res = await apiRaw<{ data?: Field[] }>('/admin/sop/fields?limit=100');
		const rows = res.ok ? (Array.isArray(res.data) ? res.data : (res.data?.data ?? [])) : [];
		managedFieldItems = rows.filter((f) => f.is_active).map((f) => ({ id: f.name, label: f.name }));
		return managedFieldItems;
	}

	async function fieldOfStudyFetcher(q: string): Promise<{ id: string; label: string }[]> {
		const items = await loadManagedFields();
		const needle = q.trim().toLowerCase();
		return needle ? items.filter((i) => i.label.toLowerCase().includes(needle)) : items;
	}

	// Fall back to the raw stored value so legacy/free-text names still
	// display on the edit form even when absent from the managed list.
	function fieldOfStudyLabel(name: string): string {
		return managedFieldItems?.find((i) => i.id === name)?.label ?? name;
	}

	const sampleFields: FieldDef[] = [
		{
			key: 'destination_space_id',
			label: t('sop_generator.destination_required', 'Destination'),
			type: 'autocomplete',
			required: true,
			placeholder: t('sop_generator.destination_ph', 'Search country…'),
			emptyMessage: t('common.no_results', 'No results found'),
			fetcher: countrySel.fetcher,
			initialFetcher: countrySel.initialFetcher,
			getSelectedLabel: countrySel.getSelectedLabel
		},
		{ key: 'program_level', label: t('sop_generator.program_level', 'Program level'), type: 'select', required: true, options: LEVEL_OPTIONS },
		{
			key: 'field_of_study',
			label: t('sop_generator.field_of_study', 'Field of study'),
			type: 'autocomplete',
			placeholder: t('sop_generator.field_of_study_ph', 'Search managed fields…'),
			emptyMessage: t('sop_generator.field_none', 'No match — type a new field to create it on save'),
			fetcher: fieldOfStudyFetcher,
			initialFetcher: fieldOfStudyFetcher.bind(null, ''),
			getSelectedLabel: fieldOfStudyLabel,
			// Free text allowed — the backend auto-curates any new value into
			// sop_fields when the sample is saved.
			freeText: true,
			allowFreeText: true
		},
		{ key: 'title', label: t('sop_generator.title_label', 'Title'), type: 'text', required: true },
		{ key: 'body', label: t('sop_generator.sample_body', 'Sample SOP text'), type: 'textarea', required: true },
		{ key: 'word_count', label: t('sop_generator.word_count', 'Word count'), type: 'number' },
	];

	async function loadSamples(p: { page: number; pageSize: number; q: string; sort: string; filter: Record<string, string> }) {
		const params = new URLSearchParams();
		params.set('page', String(p.page + 1));
		params.set('limit', String(p.pageSize));
		if (p.filter?.status) params.set('status', p.filter.status);
		const res = await apiRaw<{ data?: Sample[]; pagination?: { total?: number } }>(`/admin/sop/samples?${params}`);
		if (res.ok) {
			const raw = res.data;
			const data = Array.isArray(raw) ? raw : (raw?.data ?? []);
			const total = Array.isArray(raw) ? data.length : (raw?.pagination?.total ?? data.length);
			return { data, total, totalPages: Math.max(1, Math.ceil(total / p.pageSize)) };
		}
		throw new Error(res.error ?? 'Failed to load samples');
	}

	async function approveSample(row: Sample) {
		const res = await api(`/admin/sop/samples/${row.id}/approve`, { method: 'PATCH' });
		if (!res.ok) { pageError = res.error ?? 'Approve failed'; return; }
		sampleController?.reload();
	}

	async function rejectSample(row: Sample) {
		const res = await api(`/admin/sop/samples/${row.id}/reject`, { method: 'PATCH' });
		if (!res.ok) { pageError = res.error ?? 'Reject failed'; return; }
		sampleController?.reload();
	}

	const sampleConfig: EntityPageConfig = {
		title: t('sop_generator.samples', 'SOP Samples'),
		description: t('sop_generator.samples_desc', 'Published exemplar SOPs per destination + program level'),
		createPermission: 'sop_generator.manage',
		editPermission: 'sop_generator.manage',
		deletePermission: 'sop_generator.manage',
		fields: sampleFields,
		columns: [
			{ key: 'title', label: t('sop_generator.title_label', 'Title'), sortable: true },
			{ key: 'destination_space_name', label: t('sop_generator.destination_col', 'Destination'), sortable: true },
			{ key: 'program_level', label: t('sop_generator.level_col', 'Level') },
			{ key: 'field_of_study', label: t('sop_generator.field_col', 'Field') },
			{ key: 'word_count', label: t('sop_generator.words_col', 'Words'), sortable: true },
			{
				key: 'status', label: t('sop_generator.status', 'Status'),
				render: (v: string) => v === 'published' ? '<span style="color:var(--color-success)">published</span>' : v === 'ai_draft' ? '<span style="color:var(--color-warning)">ai_draft</span>' : (v ?? '')
			},
		],
		filters: [
			{ key: 'status', label: t('sop_generator.status', 'Status'), options: [
				{ value: 'draft', label: 'Draft' }, { value: 'ai_draft', label: 'AI Draft' },
				{ value: 'pending', label: 'Pending' }, { value: 'published', label: 'Published' },
				{ value: 'rejected', label: 'Rejected' },
			]},
		],
		load: loadSamples,
		create: async (body) => {
			const res = await api('/admin/sop/samples', { method: 'POST', body });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await api(`/admin/sop/samples/${id}`, { method: 'PATCH', body });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/sop/samples/${id}`, { method: 'DELETE' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		actions: [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: Sample, ctx?: any) => ctx?.openView(row) },
			{
				label: t('sop_generator.approve', 'Approve → Published'),
				icon: CheckCircle,
				show: (row: Sample) => ['draft', 'ai_draft', 'pending'].includes(row.status),
				onclick: (row: Sample) => approveSample(row),
			},
			{
				label: t('common.reject', 'Reject'),
				icon: XCircle,
				variant: 'destructive',
				show: (row: Sample) => ['draft', 'ai_draft', 'pending'].includes(row.status),
				onclick: (row: Sample) => rejectSample(row),
			},
		],
	};

	// ── Fields tab — curated field-of-study dictionary (sop_fields) ──────────

	let fieldController = $state<EntityPageController | undefined>(undefined);

	const fieldFields: FieldDef[] = [
		{ key: 'name', label: t('sop_generator.field_name', 'Field of study'), type: 'text', required: true, placeholder: 'Computer Science' },
		{ key: 'sort_order', label: t('sop_generator.sort_order', 'Sort order'), type: 'number', help: t('sop_generator.sort_order_help', 'Lower numbers appear first in the public select') },
		{ key: 'is_active', label: t('sop_generator.active', 'Active'), type: 'checkbox' },
	];

	async function loadFields(p: { page: number; pageSize: number; q: string; sort: string; filter: Record<string, string> }) {
		const params = new URLSearchParams();
		params.set('page', String(p.page + 1));
		params.set('limit', String(p.pageSize));
		if (p.q) params.set('q', p.q);
		const res = await apiRaw<{ data?: Field[]; pagination?: { total?: number } }>(`/admin/sop/fields?${params}`);
		if (res.ok) {
			const raw = res.data;
			const data = Array.isArray(raw) ? raw : (raw?.data ?? []);
			const total = Array.isArray(raw) ? data.length : (raw?.pagination?.total ?? data.length);
			return { data, total, totalPages: Math.max(1, Math.ceil(total / p.pageSize)) };
		}
		throw new Error(res.error ?? 'Failed to load fields');
	}

	const fieldConfig: EntityPageConfig = {
		title: t('sop_generator.fields', 'Fields of Study'),
		description: t('sop_generator.fields_desc', 'Curated options for the public SOP sample "Field of study" filter'),
		createPermission: 'sop_generator.manage',
		editPermission: 'sop_generator.manage',
		deletePermission: 'sop_generator.manage',
		fields: fieldFields,
		columns: [
			{ key: 'name', label: t('sop_generator.field_name', 'Field of study'), sortable: true },
			{ key: 'sort_order', label: t('sop_generator.sort_order', 'Sort order'), sortable: true },
			{
				key: 'is_active', label: t('sop_generator.active', 'Active'),
				render: (v: boolean) => v ? '<span style="color:var(--color-success)">active</span>' : '<span style="color:var(--color-muted-foreground)">inactive</span>'
			},
		],
		load: loadFields,
		create: async (body) => {
			const res = await api('/admin/sop/fields', { method: 'POST', body });
			if (res.ok) managedFieldItems = null; // refresh the sample form's picker cache
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await api(`/admin/sop/fields/${id}`, { method: 'PATCH', body });
			if (res.ok) managedFieldItems = null;
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/sop/fields/${id}`, { method: 'DELETE' });
			if (res.ok) managedFieldItems = null;
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		actions: [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: Field, ctx?: any) => ctx?.openView(row) },
		],
	};
</script>

<svelte:head>
	<title>SOP Generator — {siteName()} Admin</title>
</svelte:head>

{#if pageError}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{pageError}
	</div>
{/if}

<div class="mb-4 flex flex-wrap gap-2">
	{#each SOP_TABS as tab}
		<button class="rounded px-3 py-1 text-sm font-medium" style="background:{activeTab===tab?'var(--color-primary)':'var(--color-muted-bg)'};color:{activeTab===tab?'white':'var(--color-foreground)'}" onclick={() => activeTab=tab as any}>{t(`sop_generator.tab_${tab}`, tab)}</button>
	{/each}
	{#if canManage && activeTab === 'templates'}
		<button class="ms-auto rounded px-3 py-1 text-sm" style="background:var(--color-warning);color:white" onclick={generateTemplateDraft}>{t('sop_generator.generate_ai_draft', 'Generate AI Draft')}</button>
	{/if}
</div>

{#if activeTab === 'templates'}
	<EntityPage config={tplConfig} bind:controller={tplController} />
{:else if activeTab === 'samples'}
	<EntityPage config={sampleConfig} bind:controller={sampleController} />
{:else}
	<EntityPage config={fieldConfig} bind:controller={fieldController} />
{/if}
