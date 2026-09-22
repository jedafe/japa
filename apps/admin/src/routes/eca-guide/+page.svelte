<script lang="ts">
	// /eca-guide — ECA provider + guide management + community update moderation (§109).
	// Backend: GET/POST/PATCH/DELETE /admin/eca-providers, GET/POST/PATCH/DELETE /admin/eca-guides,
	//          GET/PATCH /admin/eca-guides/updates.
	// Ref: sections/109-eca-guide.md, ADR-120.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { api } from '$lib/api';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { CheckCircle, XCircle, Eye } from 'lucide-svelte';

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
		destination_space_name?: string;
		provider_name?: string;
		purpose: string;
		pathway_steps?: unknown;
		fee_estimate?: string;
		currency?: string;
		timeline_estimate?: string;
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
	};

	const canManage = $derived(hasPermission('eca_guide.manage'));

	let activeTab = $state<'providers' | 'guides' | 'updates'>('providers');
	let pageError = $state('');

	// ── Providers tab ─────────────────────────────────────────────────────────

	let providerController = $state<EntityPageController | undefined>(undefined);

	const providerFields: FieldDef[] = [
		{ key: 'slug', label: 'Slug', type: 'text', required: true, placeholder: 'wes|iqas|ces' },
		{ key: 'name', label: 'Name', type: 'text', required: true, placeholder: 'World Education Services' },
		{ key: 'website', label: 'Website', type: 'url', placeholder: 'https://wes.org' },
		{ key: 'is_active', label: 'Active', type: 'checkbox' },
	];

	async function loadProviders(p: { page: number; pageSize: number; q: string; sort: string; filter: Record<string, string> }) {
		const res = await api<Provider[]>(`/admin/eca-providers`);
		if (res.ok) {
			const data = res.data ?? [];
			return { data, total: data.length, totalPages: 1 };
		}
		throw new Error(res.error ?? 'Failed to load providers');
	}

	const providerConfig: EntityPageConfig = {
		title: 'ECA Providers',
		description: 'Credential-assessment organisations (WES, IQAS, CES, etc.)',
		createPermission: 'eca_guide.manage',
		editPermission: 'eca_guide.manage',
		deletePermission: 'eca_guide.manage',
		fields: providerFields,
		columns: [
			{ key: 'name', label: 'Name', sortable: true },
			{ key: 'slug', label: 'Slug' },
			{ key: 'website', label: 'Website', render: (v?: string) => v ? `<a href="${v}" target="_blank" style="color:var(--color-primary)">${v}</a>` : '—' },
			{ key: 'is_active', label: 'Active', render: (v?: boolean) => v === false ? '—' : '✓' },
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

	const guideFields: FieldDef[] = [
		{ key: 'destination_space_id', label: 'Destination space ID', type: 'text', required: true },
		{ key: 'provider_id', label: 'Provider ID', type: 'text', required: true },
		{ key: 'purpose', label: 'Purpose', type: 'select', required: true, options: [
			{ value: 'express_entry', label: 'Express Entry' }, { value: 'study_admission', label: 'Study Admission' },
			{ value: 'employment', label: 'Employment' }, { value: 'licensing_support', label: 'Licensing Support' },
		]},
		{ key: 'pathway_steps', label: 'Pathway steps (JSON)', type: 'textarea', required: true },
		{ key: 'fee_estimate', label: 'Fee estimate', type: 'text' },
		{ key: 'currency', label: 'Currency', type: 'text' },
		{ key: 'timeline_estimate', label: 'Timeline estimate', type: 'text' },
		{ key: 'official_url', label: 'Official URL', type: 'url' },
	];

	async function loadGuides(p: { page: number; pageSize: number; q: string; sort: string; filter: Record<string, string> }) {
		const params = new URLSearchParams();
		params.set('page', String(p.page));
		params.set('limit', String(p.pageSize));
		if (p.filter?.status) params.set('status', p.filter.status);
		if (p.filter?.dest) params.set('dest', p.filter.dest);
		const res = await api<{ data?: Guide[]; total?: number } | Guide[]>(`/admin/eca-guides?${params}`);
		if (res.ok) {
			const raw = res.data;
			const data = Array.isArray(raw) ? raw : (raw?.data ?? []);
			const total = Array.isArray(raw) ? data.length : (raw?.total ?? data.length);
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

	async function generateGuideDraft() {
		const res = await api('/admin/eca-guides/generate', { method: 'POST', body: {} });
		if (res.ok) { guideController?.reload(); }
		else { pageError = res.error ?? 'Generate failed'; }
	}

	const guideConfig: EntityPageConfig = {
		title: 'ECA Guides',
		description: 'Step-by-step credential-assessment guides per provider + destination',
		createPermission: 'eca_guide.manage',
		editPermission: 'eca_guide.manage',
		deletePermission: 'eca_guide.manage',
		fields: guideFields,
		columns: [
			{ key: 'destination_space_name', label: 'Destination', sortable: true },
			{ key: 'provider_name', label: 'Provider', sortable: true },
			{ key: 'purpose', label: 'Purpose' },
			{ key: 'fee_estimate', label: 'Fee' },
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
		load: loadGuides,
		create: async (body) => {
			const res = await api('/admin/eca-guides', { method: 'POST', body });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await api(`/admin/eca-guides/${id}`, { method: 'PATCH', body });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/eca-guides/${id}`, { method: 'DELETE' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		isDeleted: (row: Guide) => row.status === 'rejected',
		actions: [
			{ label: 'View', icon: Eye, onclick: (row: Guide, ctx?: any) => ctx?.openView(row) },
			{
				label: 'Approve → Published',
				icon: CheckCircle,
				show: (row: Guide) => ['draft', 'ai_draft', 'pending'].includes(row.status),
				onclick: (row: Guide) => approveGuide(row),
			},
			{
				label: 'Reject',
				icon: XCircle,
				variant: 'destructive',
				show: (row: Guide) => ['draft', 'ai_draft', 'pending'].includes(row.status),
				onclick: (row: Guide) => rejectGuide(row),
			},
		],
	};

	// ── Community updates tab ─────────────────────────────────────────────────

	let updateController = $state<EntityPageController | undefined>(undefined);

	async function loadUpdates(p: { page: number; pageSize: number; q: string; sort: string; filter: Record<string, string> }) {
		const params = new URLSearchParams();
		params.set('page', String(p.page));
		params.set('limit', String(p.pageSize));
		if (p.filter?.status) params.set('status', p.filter.status);
		const res = await api<{ data?: CommunityUpdate[]; pagination?: { total: number } } | CommunityUpdate[]>(`/admin/eca-guides/updates?${params}`);
		if (res.ok) {
			const raw = res.data;
			const data = Array.isArray(raw) ? raw : (raw?.data ?? []);
			const total = Array.isArray(raw) ? data.length : (raw?.pagination?.total ?? data.length);
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
		title: 'Community Updates',
		description: 'Fee/turnaround reports submitted by members — moderate before publishing',
		readonly: true,
		fields: [],
		columns: [
			{ key: 'guide_id', label: 'Guide' },
			{ key: 'user_name', label: 'Reporter' },
			{ key: 'step_index', label: 'Step' },
			{ key: 'fee_estimate', label: 'Fee' },
			{ key: 'turnaround_days', label: 'Days' },
			{
				key: 'status', label: 'Status',
				render: (v: string) => v === 'approved' ? '<span style="color:var(--color-success)">approved</span>' : v === 'pending' ? '<span style="color:var(--color-warning)">pending</span>' : v
			},
		],
		filters: [
			{ key: 'status', label: 'Status', options: [
				{ value: 'pending', label: 'Pending' }, { value: 'approved', label: 'Approved' },
				{ value: 'rejected', label: 'Rejected' },
			]},
		],
		load: loadUpdates,
		actions: [
			{ label: 'View', icon: Eye, onclick: (row: CommunityUpdate, ctx?: any) => ctx?.openView(row) },
			{
				label: 'Approve',
				icon: CheckCircle,
				show: (row: CommunityUpdate) => row.status === 'pending',
				onclick: (row: CommunityUpdate) => moderateUpdate(row, 'approved'),
			},
			{
				label: 'Reject',
				icon: XCircle,
				variant: 'destructive',
				show: (row: CommunityUpdate) => row.status === 'pending',
				onclick: (row: CommunityUpdate) => moderateUpdate(row, 'rejected'),
			},
		],
	};
</script>

<svelte:head>
	<title>ECA Guide — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{pageError}
	</div>
{/if}

<div class="mb-4 flex gap-2">
	{#each ['providers','guides','updates'] as tab}
		<button class="rounded px-3 py-1 text-sm font-medium" style="background:{activeTab===tab?'var(--color-primary)':'var(--color-muted-bg)'};color:{activeTab===tab?'white':'var(--color-foreground)'}" onclick={() => activeTab=tab as any}>{tab}</button>
	{/each}
	{#if canManage && activeTab === 'guides'}
		<button class="ml-auto rounded px-3 py-1 text-sm" style="background:var(--color-warning);color:white" onclick={generateGuideDraft}>Generate AI Draft</button>
	{/if}
</div>

{#if activeTab === 'providers'}
	<EntityPage config={providerConfig} bind:controller={providerController} />
{:else if activeTab === 'guides'}
	<EntityPage config={guideConfig} bind:controller={guideController} />
{:else}
	<EntityPage config={updateConfig} bind:controller={updateController} />
{/if}
