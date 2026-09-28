<script lang="ts">
	// /sop-generator — SOP template + sample management (§110).
	// Backend: GET/POST/PATCH/DELETE /admin/sop/templates, GET/POST/PATCH/DELETE /admin/sop/samples.
	// Ref: sections/110-sop-generator.md, ADR-121.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { api } from '$lib/api';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { initTabParam, bindTabParam } from '$lib/utils/url-sync.svelte';
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

	const canManage = $derived(hasPermission('sop_generator.manage'));

	const SOP_TABS = ['templates', 'samples'] as const;
	let activeTab = $state<'templates' | 'samples'>(initTabParam('templates', { valid: SOP_TABS }));
	bindTabParam(() => activeTab, (v) => (activeTab = v), { valid: SOP_TABS, preserve: [] });
	let pageError = $state('');

	// ── Templates tab ─────────────────────────────────────────────────────────

	let tplController = $state<EntityPageController | undefined>(undefined);

	const tplFields: FieldDef[] = [
		{ key: 'destination_space_id', label: 'Destination space ID (blank = generic)', type: 'text', placeholder: 'UUID or leave empty' },
		{ key: 'purpose', label: 'Purpose', type: 'select', required: true, options: [
			{ value: 'study_permit', label: 'Study Permit' }, { value: 'student_visa', label: 'Student Visa' },
			{ value: 'work_visa', label: 'Work Visa' }, { value: 'visitor_visa', label: 'Visitor Visa' },
		]},
		{ key: 'title', label: 'Title', type: 'text', required: true },
		{ key: 'structure', label: 'Structure (JSON)', type: 'textarea', required: true, placeholder: '[{"heading":"Intro","guidance":"..."}]' },
		{ key: 'is_active', label: 'Active', type: 'checkbox' },
	];

	async function loadTemplates(p: { page: number; pageSize: number; q: string; sort: string; filter: Record<string, string> }) {
		const params = new URLSearchParams();
		params.set('page', String(p.page));
		params.set('limit', String(p.pageSize));
		const res = await api<{ data?: Template[]; total?: number } | Template[]>(`/admin/sop/templates?${params}`);
		if (res.ok) {
			const raw = res.data;
			const data = Array.isArray(raw) ? raw : (raw?.data ?? []);
			const total = Array.isArray(raw) ? data.length : (raw?.total ?? data.length);
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
		title: 'SOP Templates',
		description: 'Structured outlines that guide AI generation per purpose',
		createPermission: 'sop_generator.manage',
		editPermission: 'sop_generator.manage',
		deletePermission: 'sop_generator.manage',
		fields: tplFields,
		columns: [
			{ key: 'title', label: 'Title', sortable: true },
			{ key: 'purpose', label: 'Purpose' },
			{
				key: 'status', label: 'Status',
				render: (v: string) => v === 'published' ? '<span style="color:var(--color-success)">published</span>' : v === 'ai_draft' ? '<span style="color:var(--color-warning)">ai_draft</span>' : v
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
		isDeleted: (row: Template) => row.status === 'rejected',
		actions: [
			{ label: 'View', icon: Eye, onclick: (row: Template, ctx?: any) => ctx?.openView(row) },
			{
				label: 'Approve → Published',
				icon: CheckCircle,
				show: (row: Template) => ['draft', 'ai_draft', 'pending'].includes(row.status),
				onclick: (row: Template) => approveTemplate(row),
			},
			{
				label: 'Reject',
				icon: XCircle,
				variant: 'destructive',
				show: (row: Template) => ['draft', 'ai_draft', 'pending'].includes(row.status),
				onclick: (row: Template) => rejectTemplate(row),
			},
		],
	};

	// ── Samples tab ───────────────────────────────────────────────────────────

	let sampleController = $state<EntityPageController | undefined>(undefined);

	const sampleFields: FieldDef[] = [
		{ key: 'destination_space_id', label: 'Destination space ID', type: 'text', required: true },
		{ key: 'program_level', label: 'Program level', type: 'select', required: true, options: [
			{ value: 'undergraduate', label: 'Undergraduate' }, { value: 'postgraduate', label: 'Postgraduate' },
			{ value: 'phd', label: 'PhD' }, { value: 'diploma', label: 'Diploma' },
		]},
		{ key: 'field_of_study', label: 'Field of study', type: 'text', placeholder: 'Computer Science' },
		{ key: 'title', label: 'Title', type: 'text', required: true },
		{ key: 'body', label: 'Sample SOP text', type: 'textarea', required: true },
		{ key: 'word_count', label: 'Word count', type: 'number' },
	];

	async function loadSamples(p: { page: number; pageSize: number; q: string; sort: string; filter: Record<string, string> }) {
		const params = new URLSearchParams();
		params.set('page', String(p.page));
		params.set('limit', String(p.pageSize));
		if (p.filter?.status) params.set('status', p.filter.status);
		const res = await api<{ data?: Sample[]; total?: number } | Sample[]>(`/admin/sop/samples?${params}`);
		if (res.ok) {
			const raw = res.data;
			const data = Array.isArray(raw) ? raw : (raw?.data ?? []);
			const total = Array.isArray(raw) ? data.length : (raw?.total ?? data.length);
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
		title: 'SOP Samples',
		description: 'Published exemplar SOPs per destination + program level',
		createPermission: 'sop_generator.manage',
		editPermission: 'sop_generator.manage',
		deletePermission: 'sop_generator.manage',
		fields: sampleFields,
		columns: [
			{ key: 'title', label: 'Title', sortable: true },
			{ key: 'destination_space_name', label: 'Destination', sortable: true },
			{ key: 'program_level', label: 'Level' },
			{ key: 'field_of_study', label: 'Field' },
			{ key: 'word_count', label: 'Words', sortable: true },
			{
				key: 'status', label: 'Status',
				render: (v: string) => v === 'published' ? '<span style="color:var(--color-success)">published</span>' : v === 'ai_draft' ? '<span style="color:var(--color-warning)">ai_draft</span>' : v
			},
		],
		filters: [
			{ key: 'status', label: 'Status', options: [
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
		isDeleted: (row: Sample) => row.status === 'rejected',
		actions: [
			{ label: 'View', icon: Eye, onclick: (row: Sample, ctx?: any) => ctx?.openView(row) },
			{
				label: 'Approve → Published',
				icon: CheckCircle,
				show: (row: Sample) => ['draft', 'ai_draft', 'pending'].includes(row.status),
				onclick: (row: Sample) => approveSample(row),
			},
			{
				label: 'Reject',
				icon: XCircle,
				variant: 'destructive',
				show: (row: Sample) => ['draft', 'ai_draft', 'pending'].includes(row.status),
				onclick: (row: Sample) => rejectSample(row),
			},
		],
	};
</script>

<svelte:head>
	<title>SOP Generator — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{pageError}
	</div>
{/if}

<div class="mb-4 flex gap-2">
	{#each ['templates','samples'] as tab}
		<button class="rounded px-3 py-1 text-sm font-medium" style="background:{activeTab===tab?'var(--color-primary)':'var(--color-muted-bg)'};color:{activeTab===tab?'white':'var(--color-foreground)'}" onclick={() => activeTab=tab as any}>{tab}</button>
	{/each}
	{#if canManage && activeTab === 'templates'}
		<button class="ml-auto rounded px-3 py-1 text-sm" style="background:var(--color-warning);color:white" onclick={generateTemplateDraft}>Generate AI Draft</button>
	{/if}
</div>

{#if activeTab === 'templates'}
	<EntityPage config={tplConfig} bind:controller={tplController} />
{:else}
	<EntityPage config={sampleConfig} bind:controller={sampleController} />
{/if}
