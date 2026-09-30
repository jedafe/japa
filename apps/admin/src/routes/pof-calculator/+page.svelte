<script lang="ts">
	// /pof-calculator — POF requirement management (§108).
	// Backend: GET/POST/PATCH/DELETE /admin/pof-requirements, POST .../generate.
	// Ref: sections/108-pof-calculator.md, ADR-119.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { api } from '$lib/api';
	import { loadCurrencies, currencyOptions, currencyLabel, type CurrencyInfo } from '$lib/utils/currency';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { CheckCircle, XCircle, Eye } from 'lucide-svelte';

	type Requirement = {
		id: string;
		destination_space_name?: string;
		visa_type: string;
		program_label: string;
		amount_schedule: unknown;
		currency: string;
		funds_must_be_liquid?: boolean;
		accepted_proof?: unknown;
		notes?: string;
		effective_date?: string;
		source_url?: string;
		status: string;
		ai_generated?: boolean;
		created_at: string;
	};

	const canManage = $derived(hasPermission('pof_calculator.manage'));

	let pageError = $state('');
	let controller = $state<EntityPageController | undefined>(undefined);
	// §58 — managed currency list for the currency select + display labels.
	let currencies = $state<CurrencyInfo[]>([]);
	$effect(() => { loadCurrencies().then((list) => (currencies = list)); });

	const fields = $derived<FieldDef[]>([
		{ key: 'destination_space_id', label: 'Destination space ID', type: 'text', required: true, placeholder: 'UUID of the space' },
		{ key: 'visa_type', label: 'Visa type', type: 'text', required: true, placeholder: 'study_permit|express_entry|work_visa' },
		{ key: 'program_label', label: 'Program label', type: 'text', required: true },
		currencies.length > 0
			? { key: 'currency', label: 'Currency', type: 'select', required: true, options: currencyOptions(currencies) }
			: { key: 'currency', label: 'Currency', type: 'text', required: true, placeholder: 'CAD|GBP|EUR' },
		{ key: 'effective_date', label: 'Effective date', type: 'date' },
		{ key: 'source_url', label: 'Source URL', type: 'url' },
		{ key: 'amount_schedule', label: 'Amount schedule (JSON)', type: 'textarea', required: true, placeholder: '[{"members":1,"amount":14500}] or {"base":13347,"per_dependant":680}' },
		{ key: 'funds_must_be_liquid', label: 'Funds must be liquid', type: 'checkbox' },
		{ key: 'accepted_proof', label: 'Accepted proof (JSON array)', type: 'textarea', placeholder: '["bank_statement","gift_deed"]' },
		{ key: 'notes', label: 'Notes', type: 'textarea' },
	]);

	async function load(p: { page: number; pageSize: number; q: string; sort: string; filter: Record<string, string> }) {
		const params = new URLSearchParams();
		params.set('page', String(p.page));
		params.set('limit', String(p.pageSize));
		if (p.filter?.status) params.set('status', p.filter.status);
		if (p.filter?.dest) params.set('dest', p.filter.dest);
		const res = await api<{ data?: Requirement[]; total?: number } | Requirement[]>(`/admin/pof-requirements?${params}`);
		if (res.ok) {
			const raw = res.data;
			const data = Array.isArray(raw) ? raw : (raw?.data ?? []);
			const total = Array.isArray(raw) ? data.length : (raw?.total ?? data.length);
			return { data, total, totalPages: Math.max(1, Math.ceil(total / p.pageSize)) };
		}
		throw new Error(res.error ?? 'Failed to load requirements');
	}

	async function approve(row: Requirement) {
		const res = await api(`/admin/pof-requirements/${row.id}/approve`, { method: 'PATCH' });
		if (!res.ok) { pageError = res.error ?? 'Approve failed'; return; }
		controller?.reload();
	}

	async function reject(row: Requirement) {
		const res = await api(`/admin/pof-requirements/${row.id}/reject`, { method: 'PATCH' });
		if (!res.ok) { pageError = res.error ?? 'Reject failed'; return; }
		controller?.reload();
	}

	async function generateDraft() {
		const res = await api('/admin/pof-requirements/generate', { method: 'POST', body: {} });
		if (res.ok) { controller?.reload(); }
		else { pageError = res.error ?? 'Generate failed'; }
	}

	const config = $derived<EntityPageConfig>({
		title: 'POF Requirements',
		description: 'Proof-of-funds amounts per destination + visa type (§108)',
		createPermission: 'pof_calculator.manage',
		editPermission: 'pof_calculator.manage',
		deletePermission: 'pof_calculator.manage',
		fields,
		columns: [
			{ key: 'destination_space_name', label: 'Destination', sortable: true },
			{ key: 'visa_type', label: 'Visa type', sortable: true },
			{ key: 'program_label', label: 'Program' },
			{ key: 'currency', label: 'Currency', render: (v: string) => currencyLabel(v, currencies) },
			{ key: 'effective_date', label: 'Effective', sortable: true },
			{
				key: 'status', label: 'Status',
				render: (v: string) => v === 'published' ? '<span style="color:var(--color-success)">published</span>' : v === 'ai_draft' ? '<span style="color:var(--color-warning)">ai_draft</span>' : v
			},
			{ key: 'ai_generated', label: 'AI', render: (v: boolean) => v ? '✦' : '—' },
		],
		filters: [
			{ key: 'status', label: 'Status', options: [
				{ value: 'draft', label: 'Draft' }, { value: 'ai_draft', label: 'AI Draft' },
				{ value: 'pending', label: 'Pending' }, { value: 'published', label: 'Published' },
				{ value: 'rejected', label: 'Rejected' },
			]},
		],
		load,
		create: async (body) => {
			const res = await api('/admin/pof-requirements', { method: 'POST', body });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await api(`/admin/pof-requirements/${id}`, { method: 'PATCH', body });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await api(`/admin/pof-requirements/${id}`, { method: 'DELETE' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		isDeleted: (row: Requirement) => row.status === 'rejected',
		actions: [
			{ label: 'View', icon: Eye, onclick: (row: Requirement, ctx?: any) => ctx?.openView(row) },
			{
				label: 'Approve → Published',
				icon: CheckCircle,
				show: (row: Requirement) => ['draft', 'ai_draft', 'pending'].includes(row.status),
				onclick: (row: Requirement) => approve(row),
			},
			{
				label: 'Reject',
				icon: XCircle,
				variant: 'destructive',
				show: (row: Requirement) => ['draft', 'ai_draft', 'pending'].includes(row.status),
				onclick: (row: Requirement) => reject(row),
			},
		],
	});
</script>

<svelte:head>
	<title>POF Calculator — JapaConnect Admin</title>
</svelte:head>

{#if pageError}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{pageError}
	</div>
{/if}

{#if canManage}
	<div class="mb-4 flex justify-end">
		<button class="rounded px-3 py-1 text-sm" style="background:var(--color-warning);color:white" onclick={generateDraft}>Generate AI Draft</button>
	</div>
{/if}

<EntityPage {config} bind:controller />
