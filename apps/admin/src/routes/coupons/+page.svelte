<script lang="ts">
	import { onMount } from 'svelte';
	import { api, couponsAdminApiExtended } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, Pencil, Trash2 } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

	type Coupon = {
		id: string;
		code: string;
		discount_type: string;
		discount_value: number;
		is_active: boolean;
		valid_from?: string;
		valid_until?: string;
		usage_limit?: number;
		used_count: number;
		created_at: string;
	};

	let coupons = $state<Coupon[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let showCreateModal = $state(false);
	let showEditModal = $state(false);
	let editingCoupon = $state<Coupon | null>(null);
	let saving = $state(false);

	let modalOpen = $state(false);

	let code = $state('');
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
			const result = await api<Coupon[]>('/admin/coupons');
			if (result.ok) {
				coupons = result.data ?? [];
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
		discountType = 'percentage';
		discountValue = '';
		validFrom = '';
		validUntil = '';
		usageLimit = '';
		isActive = true;
		modalOpen = true;
	}

	function openEdit(coupon: Coupon) {
		editingCoupon = coupon;
		code = coupon.code;
		discountType = coupon.discount_type;
		discountValue = String(coupon.discount_value);
		validFrom = coupon.valid_from || '';
		validUntil = coupon.valid_until || '';
		usageLimit = coupon.usage_limit ? String(coupon.usage_limit) : '';
		isActive = coupon.is_active;
		modalOpen = true;
	}

	async function createCoupon() {
		if (!code.trim() || !discountValue) return;
		saving = true;
		errorMessage = '';
		try {
			const body: Record<string, unknown> = {
				code: code.trim().toUpperCase(),
				discount_type: discountType,
				discount_value: Number(discountValue),
				is_active: isActive
			};
			if (validFrom) body.valid_from = validFrom;
			if (validUntil) body.valid_until = validUntil;
			if (usageLimit) body.usage_limit = Number(usageLimit);

			const result = await api('/admin/coupons', { method: 'POST', body });
			if (result.ok) {
				modalOpen = false;
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

	async function updateCoupon() {
		if (!editingCoupon || !code.trim() || !discountValue) return;
		saving = true;
		errorMessage = '';
		try {
			const body: Record<string, unknown> = {
				code: code.trim().toUpperCase(),
				discount_type: discountType,
				discount_value: Number(discountValue),
				is_active: isActive
			};
			if (validFrom) body.valid_from = validFrom;
			if (validUntil) body.valid_until = validUntil;
			if (usageLimit) body.usage_limit = Number(usageLimit);

			const result = await api(`/admin/coupons/${editingCoupon.id}`, { method: 'PATCH', body });
			if (result.ok) {
				modalOpen = false;
				editingCoupon = null;
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

	async function deleteCoupon(id: string) {
		errorMessage = '';
		try {
			const result = await api(`/admin/coupons/${id}`, { method: 'DELETE' });
			if (result.ok) {
				coupons = coupons.filter((c) => c.id !== id);
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		}
	}

	function formatDiscount(coupon: Coupon) {
		return coupon.discount_type === 'percentage'
			? `${coupon.discount_value}%`
			: `${coupon.discount_value} credits`;
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('coupons.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('coupons.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('coupons.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button class="btn btn-primary flex items-center gap-1.5 text-xs" onclick={openCreate}>
		<Plus class="h-3 w-3" />
		{t('coupons.create')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('coupons.code')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('coupons.discount')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('coupons.usage')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('coupons.status')}</th>
					<th class="px-4 py-3 text-right font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('common.actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each coupons as coupon (coupon.id)}
					<tr class="border-t" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium font-mono text-xs">{coupon.code}</td>
						<td class="px-4 py-3 text-xs">{formatDiscount(coupon)}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">
							{coupon.used_count}{coupon.usage_limit ? ` / ${coupon.usage_limit}` : ''}
						</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {coupon.is_active ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-slate-500/15 text-slate-700 dark:text-slate-400'}">
								{coupon.is_active ? t('coupons.active') : t('coupons.inactive')}
							</span>
						</td>
						<td class="px-4 py-3 text-right">
							<div class="flex items-center justify-end gap-1">
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => openEdit(coupon)}>
									<Pencil class="h-3 w-3" style="color: var(--color-primary)" />
								</button>
								<button class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" onclick={() => deleteCoupon(coupon.id)}>
									<Trash2 class="h-3 w-3" style="color: var(--color-danger)" />
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('coupons.empty')}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Create/Edit Modal -->
<Modal bind:open={modalOpen} title={editingCoupon ? t('coupons.edit') : t('coupons.create')} onConfirm={editingCoupon ? updateCoupon : createCoupon} confirmLabel={saving ? t('common.saving') : editingCoupon ? t('common.save') : t('common.create')}>
	<div class="space-y-4">
		<div>
			<label for="cp-code" class="mb-1 block text-sm font-medium">{t('coupons.code')}</label>
			<input id="cp-code" type="text" bind:value={code} required class="w-full rounded-lg border px-3 py-2 text-sm uppercase" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="cp-type" class="mb-1 block text-sm font-medium">{t('coupons.type')}</label>
				<select id="cp-type" bind:value={discountType} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)">
					<option value="percentage">{t('coupons.percentage')}</option>
					<option value="fixed">{t('coupons.fixed')}</option>
				</select>
			</div>
			<div>
				<label for="cp-value" class="mb-1 block text-sm font-medium">{t('coupons.value')}</label>
				<input id="cp-value" type="number" bind:value={discountValue} required class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="cp-from" class="mb-1 block text-sm font-medium">{t('coupons.valid_from')}</label>
				<input id="cp-from" type="date" bind:value={validFrom} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
			</div>
			<div>
				<label for="cp-until" class="mb-1 block text-sm font-medium">{t('coupons.valid_until')}</label>
				<input id="cp-until" type="date" bind:value={validUntil} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="cp-limit" class="mb-1 block text-sm font-medium">{t('coupons.usage_limit')}</label>
				<input id="cp-limit" type="number" bind:value={usageLimit} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)" />
			</div>
			<div class="flex items-center gap-2 pt-6">
				<input id="cp-active" type="checkbox" bind:checked={isActive} class="rounded" />
				<label for="cp-active" class="text-sm">{t('coupons.active')}</label>
			</div>
		</div>
	</div>
</Modal>
