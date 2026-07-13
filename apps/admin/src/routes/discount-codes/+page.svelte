<script lang="ts">
	import { onMount } from 'svelte';
	import { api, discountCodesAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, Pencil, Trash2, Eye } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

	type DiscountCode = {
		id: string;
		code: string;
		description?: string;
		discount_type: string;
		discount_value: number;
		is_active: boolean;
		valid_from?: string;
		valid_until?: string;
		usage_limit?: number;
		used_count: number;
		created_at: string;
	};

	let codes = $state<DiscountCode[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let showCreateModal = $state(false);
	let showUsageModal = $state(false);
	let usageCode = $state<DiscountCode | null>(null);
	let usageData = $state<{ users: Array<{ user_id: string; used_at: string }>; total: number } | null>(null);
	let saving = $state(false);

	let code = $state('');
	let description = $state('');
	let discountType = $state('percentage');
	let discountValue = $state('');
	let validFrom = $state('');
	let validUntil = $state('');
	let usageLimit = $state('');
	let isActive = $state(true);

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<DiscountCode[]>('/admin/discount-codes');
			if (result.ok) {
				codes = result.data ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		code = '';
		description = '';
		discountType = 'percentage';
		discountValue = '';
		validFrom = '';
		validUntil = '';
		usageLimit = '';
		isActive = true;
		showCreateModal = true;
	}

	async function createCode() {
		if (!code.trim() || !discountValue) return;
		saving = true;
		errorMessage = '';
		try {
			const body: Record<string, unknown> = {
				code: code.trim().toUpperCase(),
				description: description.trim() || undefined,
				discount_type: discountType,
				discount_value: Number(discountValue),
				is_active: isActive
			};
			if (validFrom) body.valid_from = validFrom;
			if (validUntil) body.valid_until = validUntil;
			if (usageLimit) body.usage_limit = Number(usageLimit);

			const result = await api('/admin/discount-codes', { method: 'POST', body });
			if (result.ok) {
				showCreateModal = false;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			saving = false;
		}
	}

	async function updateCode(id: string) {
		errorMessage = '';
		try {
			const body: Record<string, unknown> = {
				code: code.trim().toUpperCase(),
				description: description.trim() || undefined,
				discount_type: discountType,
				discount_value: Number(discountValue),
				is_active: isActive
			};
			if (validFrom) body.valid_from = validFrom;
			if (validUntil) body.valid_until = validUntil;
			if (usageLimit) body.usage_limit = Number(usageLimit);

			const result = await api(`/admin/discount-codes/${id}`, { method: 'PATCH', body });
			if (result.ok) {
				showCreateModal = false;
				load();
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function deleteCode(id: string) {
		errorMessage = '';
		try {
			const result = await api(`/admin/discount-codes/${id}`, { method: 'DELETE' });
			if (result.ok) {
				codes = codes.filter((c) => c.id !== id);
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		}
	}

	async function viewUsage(codeItem: DiscountCode) {
		errorMessage = '';
		try {
			const result = await api<{ users: Array<{ user_id: string; used_at: string }>; total: number }>(`/admin/discount-codes/${codeItem.id}/usage`);
			if (result.ok) {
				usageCode = codeItem;
				usageData = result.data;
				showUsageModal = true;
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		}
	}

	function formatDiscount(codeItem: DiscountCode) {
		return codeItem.discount_type === 'percentage'
			? `${codeItem.discount_value}%`
			: `${codeItem.discount_value} credits`;
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('discounts.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('discounts.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('discounts.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button class="btn btn-primary flex items-center gap-1.5 text-xs" onclick={openCreate}>
		<Plus class="h-3 w-3" />
		{t('discounts.create')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('discounts.code')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('discounts.discount')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('discounts.usage')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('discounts.status')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each codes as codeItem (codeItem.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium font-mono text-xs">{codeItem.code}</td>
						<td class="px-4 py-3 text-xs">{formatDiscount(codeItem)}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">
							{codeItem.used_count}{codeItem.usage_limit ? ` / ${codeItem.usage_limit}` : ''}
						</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {codeItem.is_active ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-slate-500/15 text-slate-700 dark:text-slate-400'}">
								{codeItem.is_active ? t('discounts.active') : t('discounts.inactive')}
							</span>
						</td>
						<td class="px-4 py-3 text-right">
							<div class="flex items-center justify-end gap-1">
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => viewUsage(codeItem)}>
									<Eye class="h-3 w-3" style="color: var(--color-info)" />
								</button>
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => { code = codeItem.code; description = codeItem.description || ''; discountType = codeItem.discount_type; discountValue = String(codeItem.discount_value); validFrom = codeItem.valid_from || ''; validUntil = codeItem.valid_until || ''; usageLimit = codeItem.usage_limit ? String(codeItem.usage_limit) : ''; isActive = codeItem.is_active; showCreateModal = true; }}>
									<Pencil class="h-3 w-3" style="color: var(--color-primary)" />
								</button>
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => deleteCode(codeItem.id)}>
									<Trash2 class="h-3 w-3" style="color: var(--color-danger)" />
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('discounts.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Create/Edit Modal -->
<Modal bind:open={showCreateModal} title={usageCode ? t('discounts.edit') : t('discounts.create')} onConfirm={() => usageCode ? updateCode(usageCode.id) : createCode()} confirmLabel={saving ? t('common.saving') : usageCode ? t('common.save') : t('common.create')}>
	<div class="space-y-4">
		<div>
			<label for="dc-code" class="mb-1 block text-sm font-medium">{t('discounts.code')}</label>
			<input id="dc-code" type="text" bind:value={code} required class="w-full rounded-lg border px-3 py-2 text-sm uppercase" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div>
			<label for="dc-desc" class="mb-1 block text-sm font-medium">{t('discounts.description')}</label>
			<input id="dc-desc" type="text" bind:value={description} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="dc-type" class="mb-1 block text-sm font-medium">{t('discounts.type')}</label>
				<select id="dc-type" bind:value={discountType} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)">
					<option value="percentage">{t('discounts.percentage')}</option>
					<option value="fixed">{t('discounts.fixed')}</option>
				</select>
			</div>
			<div>
				<label for="dc-value" class="mb-1 block text-sm font-medium">{t('discounts.value')}</label>
				<input id="dc-value" type="number" bind:value={discountValue} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="dc-from" class="mb-1 block text-sm font-medium">{t('discounts.valid_from')}</label>
				<input id="dc-from" type="date" bind:value={validFrom} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
			</div>
			<div>
				<label for="dc-until" class="mb-1 block text-sm font-medium">{t('discounts.valid_until')}</label>
				<input id="dc-until" type="date" bind:value={validUntil} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="dc-limit" class="mb-1 block text-sm font-medium">{t('discounts.usage_limit')}</label>
				<input id="dc-limit" type="number" bind:value={usageLimit} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
			</div>
			<div class="flex items-center gap-2 pt-6">
				<input id="dc-active" type="checkbox" bind:checked={isActive} class="rounded" />
				<label for="dc-active" class="text-sm">{t('discounts.active')}</label>
			</div>
		</div>
	</div>
</Modal>

<!-- Usage Modal -->
<Modal bind:open={showUsageModal} title={t('discounts.usage')} onConfirm={() => showUsageModal = false} confirmLabel={t('common.close')}>
	{#if usageData}
		<p class="text-sm mb-3" style="color: var(--color-muted)">{t('discounts.usage_total')}: {usageData.total}</p>
		{#if usageData.users.length === 0}
			<p class="text-sm" style="color: var(--color-muted)">{t('discounts.usage_empty')}</p>
		{:else}
			<div class="max-h-64 overflow-y-auto rounded-md border" style="border-color: var(--color-border)">
				<table class="w-full text-sm">
					<thead>
						<tr style="border-bottom: 1px solid var(--color-border)">
							<th class="px-4 py-2 text-left font-medium text-xs" style="color: var(--color-muted)">User</th>
							<th class="px-4 py-2 text-left font-medium text-xs" style="color: var(--color-muted)">{t('discounts.used_at')}</th>
						</tr>
					</thead>
					<tbody>
						{#each usageData.users as u (u.user_id)}
							<tr class="border-t" style="border-color: var(--color-border)">
								<td class="px-4 py-2 text-xs font-mono">{u.user_id}</td>
								<td class="px-4 py-2 text-xs" style="color: var(--color-muted)">{new Date(u.used_at).toLocaleString()}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	{/if}
</Modal>
