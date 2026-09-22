<script lang="ts">
	// /crs-calculator — CRS ruleset + draw management (§107).
	// Backend: GET/POST/PATCH/DELETE /admin/crs/rulesets, GET/POST/PATCH/DELETE /admin/crs/draws.
	// Ref: sections/107-crs-calculator.md, ADR-118.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { CheckCircle, XCircle, Eye } from 'lucide-svelte';

	type Ruleset = {
		id: string;
		version_label: string;
		effective_date: string;
		status: string;
		source_url?: string;
		ai_generated?: boolean;
		created_at: string;
	};

	type Draw = {
		id: string;
		draw_number?: number;
		draw_date: string;
		category: string;
		crs_cutoff: number;
		invitations_issued: number;
		tie_break_rule?: string;
		source_url?: string;
		created_at: string;
	};

	type Factor = {
		group: string;
		key: string;
		input_descriptor: unknown;
		label: string;
		points_single: number;
		points_with_spouse?: number;
		sort_order: number;
	};

	const canManage = $derived(hasPermission('crs_calculator.manage'));

	let activeTab = $state<'rulesets' | 'draws'>('rulesets');
	let pageError = $state('');

	// ── Rulesets tab ─────────────────────────────────────────────────────────

	let rulesetController = $state<EntityPageController | undefined>(undefined);

	const rulesetFields: FieldDef[] = [
		{ key: 'version_label', label: 'Version label', type: 'text', required: true, placeholder: 'IRCC 2025-01' },
		{ key: 'effective_date', label: 'Effective date', type: 'date', required: true },
		{ key: 'source_url', label: 'Source URL', type: 'url', placeholder: 'https://canada.ca/...' },
	];

	async function loadRulesets(p: { page: number; pageSize: number; q: string; sort: string; filter: Record<string, string> }) {
		const params = new URLSearchParams();
		params.set('page', String(p.page));
		params.set('limit', String(p.pageSize));
		if (p.filter?.status) params.set('status', p.filter.status);
		const res = await api<Ruleset[] | { data?: Ruleset[]; total?: number }>(`/admin/crs/rulesets?${params}`);
		if (res.ok) {
			const raw = res.data;
			const data = Array.isArray(raw) ? raw : (raw?.data ?? []);
			const total = Array.isArray(raw) ? data.length : (raw?.total ?? data.length);
			return { data, total, totalPages: Math.max(1, Math.ceil(total / p.pageSize)) };
		}
		throw new Error(res.error ?? 'Failed to load rulesets');
	}

	async function approveRuleset(row: Ruleset) {
		const res = await api(`/admin/crs/rulesets/${row.id}/approve`, { method: 'PATCH' });
		if (!res.ok) { pageError = res.error ?? 'Approve failed'; return; }
		rulesetController?.reload();
	}

	async function rejectRuleset(row: Ruleset) {
		const res = await api(`/admin/crs/rulesets/${row.id}/reject`, { method: 'PATCH' });
		if (!res.ok) { pageError = res.error ?? 'Reject failed'; return; }
		rulesetController?.reload();
	}

	async function generateDraft() {
		const res = await api('/admin/crs/rulesets/generate', { method: 'POST', body: {} });
		if (res.ok) { rulesetController?.reload(); }
		else { pageError = res.error ?? 'Generate failed'; }
	}

	const rulesetConfig: EntityPageConfig = {
		title: 'CRS Rulesets',
		description: 'Versioned scoring rulesets — one active at a time',
		createPermission: 'crs_calculator.manage',
		editPermission: 'crs_calculator.manage',
		deletePermission: 'crs_calculator.manage',
		fields: rulesetFields,
		columns: [
			{ key: 'version_label', label: 'Version', sortable: true },
			{ key: 'effective_date', label: 'Effective', sortable: true },
			{
				key: 'status', label: 'Status',
				render: (v: string) => v === 'active' ? '<span style="color:var(--color-success)">active</span>' : v === 'ai_draft' ? '<span style="color:var(--color-warning)">ai_draft</span>' : v
			},
			{ key: 'ai_generated', label: 'AI', render: (v: boolean) => v ? '✦' : '—' },
		],
		filters: [
			{ key: 'status', label: 'Status', options: [
				{ value: 'draft', label: 'Draft' }, { value: 'ai_draft', label: 'AI Draft' },
				{ value: 'active', label: 'Active' }, { value: 'superseded', label: 'Superseded' },
				{ value: 'rejected', label: 'Rejected' },
			]},
		],
		load: loadRulesets,
		create: async (body) => {
			const res = await api('/admin/crs/rulesets', { method: 'POST', body });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await api(`/admin/crs/rulesets/${id}`, { method: 'PATCH', body });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/crs/rulesets/${id}`, { method: 'DELETE' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		isDeleted: (row: Ruleset) => row.status === 'rejected',
		actions: [
			{ label: 'View factors', icon: Eye, onclick: (row: Ruleset, ctx?: any) => ctx?.openView(row) },
			{
				label: 'Approve → Active',
				icon: CheckCircle,
				show: (row: Ruleset) => ['draft', 'ai_draft', 'pending'].includes(row.status),
				onclick: (row: Ruleset) => approveRuleset(row),
			},
			{
				label: 'Reject',
				icon: XCircle,
				variant: 'destructive',
				show: (row: Ruleset) => ['draft', 'ai_draft', 'pending'].includes(row.status),
				onclick: (row: Ruleset) => rejectRuleset(row),
			},
		],
	};

	// ── Draws tab ────────────────────────────────────────────────────────────

	let drawController = $state<EntityPageController | undefined>(undefined);

	const drawFields: FieldDef[] = [
		{ key: 'draw_number', label: 'Draw #', type: 'number' },
		{ key: 'draw_date', label: 'Draw date', type: 'date', required: true },
		{ key: 'category', label: 'Category', type: 'text', required: true, placeholder: 'general|cec|french|trade|stem' },
		{ key: 'crs_cutoff', label: 'CRS cutoff', type: 'number', required: true },
		{ key: 'invitations_issued', label: 'ITAs issued', type: 'number', required: true },
		{ key: 'tie_break_rule', label: 'Tie-break', type: 'text', placeholder: '2025-01-15T12:00:00Z' },
		{ key: 'source_url', label: 'Source URL', type: 'url' },
	];

	async function loadDraws(p: { page: number; pageSize: number; q: string; sort: string; filter: Record<string, string> }) {
		const params = new URLSearchParams();
		params.set('page', String(p.page));
		params.set('limit', String(p.pageSize));
		if (p.filter?.category) params.set('category', p.filter.category);
		const res = await api<Draw[] | { data?: Draw[]; total?: number }>(`/admin/crs/draws?${params}`);
		if (res.ok) {
			const raw = res.data;
			const data = Array.isArray(raw) ? raw : (raw?.data ?? []);
			const total = Array.isArray(raw) ? data.length : (raw?.total ?? data.length);
			return { data, total, totalPages: Math.max(1, Math.ceil(total / p.pageSize)) };
		}
		throw new Error(res.error ?? 'Failed to load draws');
	}

	const drawConfig: EntityPageConfig = {
		title: 'CRS Draws',
		description: 'Express Entry draw history — cutoff scores for the trend chart',
		createPermission: 'crs_calculator.manage',
		editPermission: 'crs_calculator.manage',
		deletePermission: 'crs_calculator.manage',
		fields: drawFields,
		columns: [
			{ key: 'draw_number', label: '#', sortable: true },
			{ key: 'draw_date', label: 'Date', sortable: true },
			{ key: 'category', label: 'Category' },
			{ key: 'crs_cutoff', label: 'Cutoff', sortable: true },
			{ key: 'invitations_issued', label: 'ITAs', sortable: true },
		],
		load: loadDraws,
		create: async (body) => {
			const res = await api('/admin/crs/draws', { method: 'POST', body });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await api(`/admin/crs/draws/${id}`, { method: 'PATCH', body });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/crs/draws/${id}`, { method: 'DELETE' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
	};
</script>

<svelte:head>
	<title>CRS Calculator — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{pageError}
	</div>
{/if}

<div class="mb-4 flex gap-2">
	<button class="rounded px-3 py-1 text-sm font-medium" style="background:{activeTab==='rulesets'?'var(--color-primary)':'var(--color-muted-bg)'};color:{activeTab==='rulesets'?'white':'var(--color-foreground)'}" onclick={() => activeTab='rulesets'}>Rulesets</button>
	<button class="rounded px-3 py-1 text-sm font-medium" style="background:{activeTab==='draws'?'var(--color-primary)':'var(--color-muted-bg)'};color:{activeTab==='draws'?'white':'var(--color-foreground)'}" onclick={() => activeTab='draws'}>Draws</button>
	{#if canManage && activeTab === 'rulesets'}
		<button class="ml-auto rounded px-3 py-1 text-sm" style="background:var(--color-warning);color:white" onclick={generateDraft}>Generate AI Draft</button>
	{/if}
</div>

{#if activeTab === 'rulesets'}
	<EntityPage config={rulesetConfig} bind:controller={rulesetController} />
{:else}
	<EntityPage config={drawConfig} bind:controller={drawController} />
{/if}
