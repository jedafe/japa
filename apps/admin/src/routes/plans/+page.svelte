<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import Modal from '$lib/components/Modal.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Select } from '$lib/components/ui/select';
	import { plansApi, walletApi, type SubscriptionPlan, type PlanFeature } from '$lib/api';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Pencil, Trash2, Plus, X, Check, Star } from 'lucide-svelte';

	type Plan = SubscriptionPlan;

	let plans = $state<Plan[]>([]);
	let currencies = $state<{ code: string; symbol: string; name?: string }[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let selectedIds = $state<Set<string>>(new Set());

	let showModal = $state(false);
	let editingId = $state<string | null>(null);
	let saving = $state(false);
	let showDeleteConfirm = $state(false);
	let deleteTargetId = $state<string | null>(null);

	let form = $state({
		slug: '',
		name: '',
		description: '',
		features: [] as PlanFeature[],
		monthly_amount: 0,
		yearly_amount: 0,
		currency: 'NGN',
		highlighted: false,
		sort_order: 0,
		status: 'active'
	});

	let newFeatureText = $state('');
	let newFeatureIncluded = $state(true);

	const canManage = $derived(hasPermission('payments.plans.manage'));

	// Currency options from the API (Rule 16 — no hardcoded data).
	const currencyOptions = $derived(
		currencies.map((c) => ({ value: c.code, label: c.name ? `${c.name} (${c.code})` : c.code }))
	);
	const currencySymbolMap = $derived(new Map(currencies.map((c) => [c.code, c.symbol])));

	const statuses = [
		{ value: 'active', label: 'Active' },
		{ value: 'inactive', label: 'Inactive' }
	];

	const columns = [
		{ key: 'sort_order', label: '#', sortable: true, class: 'w-10 text-center', render: (v: number) => `<span class="text-xs text-muted">${v}</span>` },
		{ key: 'name', label: 'Name', sortable: true, render: (v: string, row: Plan) => {
			const star = row.highlighted ? '<span class="ml-1 inline-block text-warning">★</span>' : '';
			return `<span class="font-medium">${v}${star}</span>`;
		} },
		{ key: 'slug', label: 'Slug', sortable: true, class: 'font-mono text-xs' },
		{ key: 'monthly_amount', label: 'Monthly', sortable: true, render: (v: number, row: Plan) => {
			if (v === 0) return '<span class="text-success text-xs">Free</span>';
			const sym = currencySymbolMap.get(row.currency) ?? row.currency;
			return `<span class="text-xs">${sym}${(v / 100).toLocaleString()}</span>`;
		} },
		{ key: 'yearly_amount', label: 'Yearly', sortable: true, render: (v: number, row: Plan) => {
			if (v === 0) return '<span class="text-muted text-xs">—</span>';
			const sym = currencySymbolMap.get(row.currency) ?? row.currency;
			return `<span class="text-xs">${sym}${(v / 100).toLocaleString()}</span>`;
		} },
		{ key: 'features', label: 'Features', sortable: false, render: (v: PlanFeature[]) => {
			const count = v?.filter(f => f.included).length ?? 0;
			return `<span class="text-xs text-muted">${count} included</span>`;
		} },
		{ key: 'status', label: 'Status', sortable: true, render: (v: string) => v === 'active'
			? '<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-success/15 text-success">Active</span>'
			: '<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-muted/15 text-muted">Inactive</span>'
		},
		{ key: 'actions', label: '', sortable: false, class: 'w-24', componentSnippet: PlanActionsCell }
	];

	function slugify(text: string): string {
		return text.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
	}

	async function load() {
		loading = true;
		errorMessage = '';
		const [result, currResult] = await Promise.all([
			plansApi.list(),
			walletApi.getCurrencies()
		]);
		if (result.ok) {
			const raw = (result.data ?? []) as Plan[];
			// The backend serializes JSONB features as a raw JSON string ([]byte),
			// so parse it into an array here for the UI.
			plans = raw.map(p => ({
				...p,
				features: typeof p.features === 'string' ? JSON.parse(p.features as unknown as string) : (p.features ?? [])
			}));
		} else {
			errorMessage = result.error ?? 'Failed to load plans';
		}
		if (currResult.ok && currResult.data) {
			currencies = currResult.data;
		}
		loading = false;
	}

	function openCreate() {
		editingId = null;
		form = {
			slug: '', name: '', description: '',
			features: [], monthly_amount: 0, yearly_amount: 0,
			currency: 'NGN', highlighted: false, sort_order: plans.length, status: 'active'
		};
		newFeatureText = '';
		newFeatureIncluded = true;
		showModal = true;
	}

	function openEdit(plan: Plan) {
		editingId = plan.id;
		const features = typeof plan.features === 'string' ? JSON.parse(plan.features as unknown as string) : (plan.features ?? []);
		form = {
			slug: plan.slug,
			name: plan.name,
			description: plan.description ?? '',
			features,
			monthly_amount: plan.monthly_amount,
			yearly_amount: plan.yearly_amount,
			currency: plan.currency,
			highlighted: plan.highlighted,
			sort_order: plan.sort_order,
			status: plan.status
		};
		newFeatureText = '';
		newFeatureIncluded = true;
		showModal = true;
	}

	function addFeature() {
		if (!newFeatureText.trim()) return;
		form.features = [...form.features, { text: newFeatureText.trim(), included: newFeatureIncluded }];
		newFeatureText = '';
		newFeatureIncluded = true;
	}

	function removeFeature(idx: number) {
		form.features = form.features.filter((_, i) => i !== idx);
	}

	function toggleFeature(idx: number) {
		form.features = form.features.map((f, i) => i === idx ? { ...f, included: !f.included } : f);
	}

	async function submit() {
		if (!form.name.trim() || !form.slug.trim()) return;
		saving = true;
		errorMessage = '';
		const slug = form.slug.trim() || slugify(form.name);
		const body = {
			slug,
			name: form.name.trim(),
			description: form.description.trim(),
			features: form.features,
			monthly_amount: form.monthly_amount,
			yearly_amount: form.yearly_amount,
			currency: form.currency,
			highlighted: form.highlighted,
			sort_order: form.sort_order,
			status: form.status
		};
		const result = editingId
			? await plansApi.update(editingId, body)
			: await plansApi.create(body);
		if (result.ok) {
			showModal = false;
			load();
		} else {
			errorMessage = result.error ?? 'Failed to save plan';
		}
		saving = false;
	}

	function confirmDelete(id: string) {
		deleteTargetId = id;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTargetId) return;
		errorMessage = '';
		const result = await plansApi.delete(deleteTargetId);
		if (result.ok) {
			showDeleteConfirm = false;
			deleteTargetId = null;
			load();
		} else {
			errorMessage = result.error ?? 'Failed to delete plan';
		}
	}

	async function bulkDelete() {
		if (selectedIds.size === 0) return;
		errorMessage = '';
		try {
			for (const id of selectedIds) {
				await plansApi.delete(id);
			}
			selectedIds = new Set();
			load();
		} catch {
			errorMessage = 'Failed to delete selected plans';
		}
	}

	const tableData = $derived(plans.map(p => ({ ...p })));

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('nav.plans', 'Subscription Plans')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('nav.plans', 'Subscription Plans')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">Create, edit, and manage subscription plan types, pricing, and feature lists.</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
		onclick={openCreate}
	>
		{t('plans.add', 'Add Plan')}
	</button>
</div>

<div>
	<DataTable
		{columns}
		data={tableData}
		{loading}
		emptyMessage="No subscription plans configured yet."
		selectable
		bind:selectedIds
	>
		{#snippet bulkActions()}
			<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={selectedIds.size === 0}>
				Delete Selected ({selectedIds.size})
			</Button>
		{/snippet}
	</DataTable>
</div>

<!-- Create/Edit Modal -->
<Modal
	bind:open={showModal}
	title={editingId ? 'Edit Plan' : 'Add Plan'}
	onConfirm={submit}
	confirmLabel={saving ? 'Saving...' : (editingId ? 'Update' : 'Create')}
>
	<div class="space-y-3">
		<div>
			<label for="plan-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Name</label>
			<input id="plan-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="Pro" bind:value={form.name} oninput={() => { if (!editingId) form.slug = slugify(form.name); }} />
		</div>
		<div>
			<label for="plan-slug" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Slug</label>
			<input id="plan-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-surface)" placeholder="pro" bind:value={form.slug} />
		</div>
		<div>
			<label for="plan-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Description</label>
			<textarea id="plan-desc" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" rows="2" bind:value={form.description}></textarea>
		</div>

		<!-- Feature list editor -->
		<div>
			<span class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Features</span>
			<div class="space-y-1.5">
				{#each form.features as feature, idx (idx)}
					<div class="flex items-center gap-2">
						<button
							type="button"
							class="shrink-0 rounded p-1"
							onclick={() => toggleFeature(idx)}
							title={feature.included ? 'Included' : 'Not included'}
						>
							{#if feature.included}
								<Check class="h-4 w-4 text-success" />
							{:else}
								<X class="h-4 w-4 text-muted" />
							{/if}
						</button>
						<input
							type="text"
							class="flex-1 rounded-md border px-2 py-1 text-xs"
							style="border-color: var(--color-border); background-color: var(--color-surface)"
							bind:value={form.features[idx].text}
						/>
						<button type="button" class="shrink-0 rounded p-1" style="color: var(--color-danger)" onclick={() => removeFeature(idx)}>
							<Trash2 class="h-3.5 w-3.5" />
						</button>
					</div>
				{/each}
			</div>
			<div class="mt-2 flex items-center gap-2">
				<input
					type="text"
					class="flex-1 rounded-md border px-2 py-1.5 text-xs"
					style="border-color: var(--color-border); background-color: var(--color-surface)"
					placeholder="Add a feature…"
					bind:value={newFeatureText}
					onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addFeature(); } }}
				/>
				<button type="button" class="shrink-0 rounded-md border px-2 py-1.5 text-xs" style="border-color: var(--color-border)" onclick={() => addFeature()}>
					<Plus class="h-3.5 w-3.5" />
				</button>
			</div>
		</div>

		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="plan-monthly" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Monthly (in kobo/cents)</label>
				<input id="plan-monthly" type="number" min="0" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.monthly_amount} />
			</div>
			<div>
				<label for="plan-yearly" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Yearly (in kobo/cents)</label>
				<input id="plan-yearly" type="number" min="0" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.yearly_amount} />
			</div>
		</div>

		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="plan-currency" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Currency</label>
				<Select options={currencyOptions} bind:value={form.currency} />
			</div>
			<div>
				<label for="plan-status" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Status</label>
				<Select options={statuses} bind:value={form.status} />
			</div>
		</div>

		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="plan-sort" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Sort Order</label>
				<input id="plan-sort" type="number" min="0" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.sort_order} />
			</div>
			<div class="flex items-end gap-2 pb-2">
				<input id="plan-highlight" type="checkbox" bind:checked={form.highlighted} />
				<label for="plan-highlight" class="text-sm flex items-center gap-1">
					<Star class="h-3.5 w-3.5" /> Highlighted
				</label>
			</div>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title="Confirm Delete" onConfirm={executeDelete} confirmLabel="Delete">
	<p class="text-sm" style="color: var(--color-muted)">Are you sure you want to delete this plan? This cannot be undone.</p>
</Modal>

{#snippet PlanActionsCell({ row }: { value: any; row: Plan })}
	{#if canManage}
		<div class="flex items-center gap-1">
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)} aria-label="Edit">
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(row.id)} style="color: var(--color-danger)" aria-label="Delete">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		</div>
	{/if}
{/snippet}
