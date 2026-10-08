<script lang="ts">
	// Ref: japaconnect-engineering/sections/114-services-marketplace.md
	// §114 — org coupon management: create/edit/suspend/delete plus
	// redemption + attachment rollups. List mutations go through the
	// client API then invalidateAll() re-runs the server load.
	import { page } from '$app/state';
	import { invalidateAll } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { BadgePercent, Plus, Loader2, AlertCircle, Pencil, PauseCircle, PlayCircle, Trash2, X } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { discountLabel } from '$lib/marketplace';
	import { formatMoney, type CurrencyInfo } from '$lib/utils/currency';
	import type { OrgCoupon, OrgCouponInput } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const coupons = $derived(data.coupons ?? []);
	const currencies = $derived((page.data.currencies ?? []) as CurrencyInfo[]);
	const org = $derived(data.org);
	const hasMore = $derived(coupons.length >= data.pageSize);

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	// Form state — a single draft object drives both create and edit;
	// editing holds the row under change (null = creating a new coupon).
	let formOpen = $state(false);
	let editing = $state<OrgCoupon | null>(null);
	let saving = $state(false);
	let formError = $state('');
	let busyId = $state('');

	interface CouponDraft {
		title: string;
		code: string;
		description: string;
		discountType: 'percent' | 'fixed';
		discountValue: string; // percent, or MAJOR units for fixed (×100 on save)
		currency: string;
		appliesTo: string;
		termsUrl: string;
		referralUrl: string;
		validFrom: string; // datetime-local
		expiresAt: string;
		maxRedemptions: string;
		perUserLimit: string;
		minSpend: string; // major units
		firstTimeOnly: boolean;
	}

	function blankDraft(): CouponDraft {
		return {
			title: '', code: '', description: '',
			discountType: 'percent', discountValue: '', currency: '',
			appliesTo: 'both', termsUrl: '', referralUrl: '',
			validFrom: '', expiresAt: '',
			maxRedemptions: '', perUserLimit: '', minSpend: '',
			firstTimeOnly: false
		};
	}
	let draft = $state<CouponDraft>(blankDraft());

	function toLocalInput(iso: string | null | undefined): string {
		if (!iso) return '';
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return '';
		// datetime-local wants YYYY-MM-DDTHH:mm in local time.
		const pad = (n: number) => String(n).padStart(2, '0');
		return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
	}

	function openCreate() {
		editing = null;
		draft = blankDraft();
		formError = '';
		formOpen = true;
	}

	function openEdit(c: OrgCoupon) {
		editing = c;
		const isPercent = c.discount_type !== 'fixed';
		draft = {
			title: c.title ?? '',
			code: c.code ?? '',
			description: '',
			discountType: isPercent ? 'percent' : 'fixed',
			// percent stores the whole number; fixed stores minor units → /100.
			discountValue:
				c.discount_value != null ? String(isPercent ? c.discount_value : c.discount_value / 100) : '',
			currency: c.currency ?? '',
			appliesTo: c.applies_to || 'both',
			termsUrl: c.terms_url ?? '',
			referralUrl: c.referral_url ?? '',
			validFrom: toLocalInput(c.valid_from),
			expiresAt: toLocalInput(c.expires_at),
			maxRedemptions: c.max_redemptions != null ? String(c.max_redemptions) : '',
			perUserLimit: c.per_user_limit != null ? String(c.per_user_limit) : '',
			minSpend: c.min_spend != null ? String(c.min_spend / 100) : '',
			firstTimeOnly: c.first_time_only ?? false
		};
		formError = '';
		formOpen = true;
	}

	// §114 — backend coupon codes match ^[A-Z0-9_-]{3,32}$ (normalized
	// uppercase); client-checks here surface errors before the round trip.
	const CODE_RE = /^[A-Z0-9_-]{3,32}$/;

	function validateDraft(): boolean {
		formError = '';
		if (!draft.title.trim()) {
			formError = t('coupons.err_title', 'Title is required');
			return false;
		}
		if (!CODE_RE.test(draft.code.trim().toUpperCase())) {
			formError = t('coupons.err_code', 'Code must be 3–32 chars: letters, numbers, _ or -');
			return false;
		}
		const val = Number(String(draft.discountValue ?? '').trim());
		if (draft.discountType === 'percent') {
			if (!(val > 0 && val <= 100)) {
				formError = t('coupons.err_percent', 'Percent discount must be between 0 and 100');
				return false;
			}
		} else {
			if (!(val > 0)) {
				formError = t('coupons.err_fixed', 'Fixed discount must be greater than 0');
				return false;
			}
			if (!draft.currency) {
				formError = t('coupons.err_currency', 'Fixed discounts need a currency');
				return false;
			}
		}
		if (draft.validFrom && draft.expiresAt && new Date(draft.expiresAt) <= new Date(draft.validFrom)) {
			formError = t('coupons.err_window', 'Expiry must be after the start date');
			return false;
		}
		return true;
	}

	async function save(e: SubmitEvent) {
		e.preventDefault();
		if (!validateDraft()) return;

		const val = Number(String(draft.discountValue ?? '').trim());
		const body: OrgCouponInput = {
			title: draft.title.trim(),
			code: draft.code.trim().toUpperCase(),
			discount_type: draft.discountType,
			// percent → whole number; fixed → minor units.
			discount_value: draft.discountType === 'percent' ? val : Math.round(val * 100),
			applies_to: draft.appliesTo,
			first_time_only: draft.firstTimeOnly
		};
		if (draft.description.trim()) body.description = draft.description.trim();
		if (draft.currency) body.currency = draft.currency;
		if (draft.termsUrl.trim()) body.terms_url = draft.termsUrl.trim();
		if (draft.referralUrl.trim()) body.referral_url = draft.referralUrl.trim();
		if (draft.validFrom) body.valid_from = new Date(draft.validFrom).toISOString();
		if (draft.expiresAt) body.expires_at = new Date(draft.expiresAt).toISOString();
		if (String(draft.maxRedemptions ?? '').trim()) body.max_redemptions = Number(draft.maxRedemptions);
		if (String(draft.perUserLimit ?? '').trim()) body.per_user_limit = Number(draft.perUserLimit);
		if (String(draft.minSpend ?? '').trim()) body.min_spend = Math.round(Number(draft.minSpend) * 100);

		saving = true;
		const res = editing
			? await api.patch<OrgCoupon>(`/organizations/${org.id}/coupons/${editing.id}`, body)
			: await api.post<OrgCoupon>(`/organizations/${org.id}/coupons`, body);
		saving = false;

		if (res.error) {
			formError =
				res.error.code === 'coupon_code_taken'
					? t('coupons.err_code_taken', 'That code is already in use')
					: (res.error.message ?? t('coupons.err_save', 'Failed to save coupon'));
			return;
		}
		formOpen = false;
		await invalidateAll();
	}

	async function setStatus(c: OrgCoupon, status: 'active' | 'suspended') {
		busyId = c.id;
		await api.patch(`/organizations/${org.id}/coupons/${c.id}`, { status });
		busyId = '';
		await invalidateAll();
	}

	async function remove(c: OrgCoupon) {
		if (!confirm(t('coupons.confirm_delete', 'Delete {code}? Coupons with redemptions are suspended instead.', { code: c.code ?? c.title }))) return;
		busyId = c.id;
		await api.delete(`/organizations/${org.id}/coupons/${c.id}`);
		busyId = '';
		await invalidateAll();
	}

	function statusChip(status: string): string {
		switch (status) {
			case 'active':
				return 'bg-[var(--color-success)]/10 text-[var(--color-success)]';
			case 'suspended':
				return 'bg-amber-500/10 text-amber-600';
			case 'expired':
				return 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]';
			default:
				return 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]';
		}
	}

	function fmtDate(iso: string | null | undefined): string {
		if (!iso) return '';
		const d = new Date(iso);
		return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString();
	}
</script>

<svelte:head>
	<title>{t('coupons.title', 'Coupons — JapaConnect')}</title>
</svelte:head>

<div class="mx-auto w-full max-w-4xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<div class="flex items-center gap-2">
			<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
				<BadgePercent size={20} /> {t('coupons.heading', 'Coupons')}
			</h1>
			<span class="text-sm text-[var(--color-muted)]">· {org.name}</span>
			<button
				type="button"
				onclick={openCreate}
				class="ml-auto inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
			>
				<Plus size={14} /> {t('coupons.new', 'New coupon')}
			</button>
		</div>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{t('coupons.subheading', 'Discount codes travellers redeem on your service packages.')}
		</p>
	</div>

	<div class="space-y-3 p-4">
		<!-- Create/edit form -->
		{#if formOpen}
			<form
				onsubmit={save}
				class="space-y-3 rounded-[var(--radius-card)] border border-[var(--color-primary)]/40 bg-[var(--color-surface)] p-4"
				novalidate
			>
				<div class="flex items-center justify-between">
					<h2 class="text-sm font-semibold text-[var(--color-text)]">
						{editing ? t('coupons.edit_heading', 'Edit coupon') : t('coupons.create_heading', 'New coupon')}
					</h2>
					<button type="button" onclick={() => (formOpen = false)} aria-label={t('common.close', 'Close')} class="text-[var(--color-muted)] hover:text-[var(--color-text)]">
						<X size={16} />
					</button>
				</div>

				<div class="grid gap-3 sm:grid-cols-2">
					<div>
						<label for="cp-title" class="mb-1 block text-xs font-medium text-[var(--color-text)]">{t('coupons.f_title', 'Title')} <span class="text-red-500">*</span></label>
						<input id="cp-title" type="text" bind:value={draft.title} placeholder={t('coupons.f_title_ph', 'e.g. Early-bird 15% off')} class={inputClass} />
					</div>
					<div>
						<label for="cp-code" class="mb-1 block text-xs font-medium text-[var(--color-text)]">{t('coupons.f_code', 'Code')} <span class="text-red-500">*</span></label>
						<input id="cp-code" type="text" bind:value={draft.code} placeholder="EARLY15" class="{inputClass} font-mono uppercase" maxlength="32" />
					</div>
					<div>
						<label for="cp-type" class="mb-1 block text-xs font-medium text-[var(--color-text)]">{t('coupons.f_type', 'Discount type')}</label>
						<select id="cp-type" bind:value={draft.discountType} class={inputClass}>
							<option value="percent">{t('coupons.type_percent', 'Percent off')}</option>
							<option value="fixed">{t('coupons.type_fixed', 'Fixed amount off')}</option>
						</select>
					</div>
					<div>
						<label for="cp-value" class="mb-1 block text-xs font-medium text-[var(--color-text)]">
							{draft.discountType === 'percent' ? t('coupons.f_value_pct', 'Percent (1–100)') : t('coupons.f_value_fixed', 'Amount')} <span class="text-red-500">*</span>
						</label>
						<div class="flex gap-2">
							<input id="cp-value" type="number" min="0" step={draft.discountType === 'percent' ? '1' : '0.01'} bind:value={draft.discountValue} class={inputClass} />
							{#if draft.discountType === 'fixed'}
								<select bind:value={draft.currency} class="{inputClass} w-28 shrink-0" aria-label={t('coupons.f_currency', 'Currency')}>
									<option value="">{t('coupons.f_currency', 'Currency')}</option>
									{#each currencies as c (c.code)}
										<option value={c.code}>{c.code}</option>
									{/each}
								</select>
							{/if}
						</div>
					</div>
					<div>
						<label for="cp-applies" class="mb-1 block text-xs font-medium text-[var(--color-text)]">{t('coupons.f_applies', 'Applies to')}</label>
						<select id="cp-applies" bind:value={draft.appliesTo} class={inputClass}>
							<option value="both">{t('coupons.applies_both', 'Services & packages')}</option>
							<option value="package">{t('coupons.applies_package', 'Packages only')}</option>
							<option value="service">{t('coupons.applies_service', 'Services only')}</option>
						</select>
					</div>
					<div>
						<label for="cp-minspend" class="mb-1 block text-xs font-medium text-[var(--color-text)]">{t('coupons.f_minspend', 'Minimum spend (optional)')}</label>
						<input id="cp-minspend" type="number" min="0" step="0.01" bind:value={draft.minSpend} placeholder="0.00" class={inputClass} />
					</div>
					<div>
						<label for="cp-from" class="mb-1 block text-xs font-medium text-[var(--color-text)]">{t('coupons.f_valid_from', 'Valid from')}</label>
						<input id="cp-from" type="datetime-local" bind:value={draft.validFrom} class={inputClass} />
					</div>
					<div>
						<label for="cp-until" class="mb-1 block text-xs font-medium text-[var(--color-text)]">{t('coupons.f_expires', 'Expires at')}</label>
						<input id="cp-until" type="datetime-local" bind:value={draft.expiresAt} class={inputClass} />
					</div>
					<div>
						<label for="cp-max" class="mb-1 block text-xs font-medium text-[var(--color-text)]">{t('coupons.f_max_redemptions', 'Max redemptions (optional)')}</label>
						<input id="cp-max" type="number" min="1" bind:value={draft.maxRedemptions} class={inputClass} />
					</div>
					<div>
						<label for="cp-peruser" class="mb-1 block text-xs font-medium text-[var(--color-text)]">{t('coupons.f_per_user', 'Per-user limit (optional)')}</label>
						<input id="cp-peruser" type="number" min="1" bind:value={draft.perUserLimit} class={inputClass} />
					</div>
					<div>
						<label for="cp-terms" class="mb-1 block text-xs font-medium text-[var(--color-text)]">{t('coupons.f_terms', 'Terms URL (optional)')}</label>
						<input id="cp-terms" type="url" bind:value={draft.termsUrl} placeholder="https://…" class={inputClass} />
					</div>
					<div>
						<label for="cp-ref" class="mb-1 block text-xs font-medium text-[var(--color-text)]">{t('coupons.f_referral', 'Referral URL (optional)')}</label>
						<input id="cp-ref" type="url" bind:value={draft.referralUrl} placeholder="https://…" class={inputClass} />
					</div>
				</div>

				<div>
					<label for="cp-desc" class="mb-1 block text-xs font-medium text-[var(--color-text)]">{t('coupons.f_description', 'Description (optional)')}</label>
					<textarea id="cp-desc" bind:value={draft.description} rows={2} class="{inputClass} resize-y"></textarea>
				</div>

				<label class="flex items-center gap-2 text-xs text-[var(--color-text)]">
					<input type="checkbox" bind:checked={draft.firstTimeOnly} class="h-3.5 w-3.5 rounded border-[var(--color-border)]" />
					{t('coupons.f_first_time', 'First-time bookers only')}
				</label>

				{#if formError}
					<p class="flex items-center gap-1.5 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-xs text-red-500" role="alert">
						<AlertCircle size={13} /> {formError}
					</p>
				{/if}

				<div class="flex justify-end gap-2">
					<button
						type="button"
						onclick={() => (formOpen = false)}
						class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					>
						{t('common.cancel', 'Cancel')}
					</button>
					<button
						type="submit"
						disabled={saving}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						{#if saving}<Loader2 size={13} class="animate-spin" />{/if}
						{editing ? t('common.save', 'Save') : t('coupons.create', 'Create coupon')}
					</button>
				</div>
			</form>
		{/if}

		<!-- Coupon list -->
		{#if coupons.length === 0}
			<EmptyState
				title={t('coupons.empty_title', 'No coupons yet')}
				body={t('coupons.empty_body', 'Create a discount code and attach it to your service packages.')}
			/>
		{:else}
			{#each coupons as c (c.id)}
				<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<div class="flex items-start justify-between gap-3">
						<div class="min-w-0 flex-1">
							<div class="flex flex-wrap items-center gap-2">
								{#if c.code}
									<code class="rounded-[var(--radius)] border border-dashed border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2 py-0.5 font-mono text-sm font-semibold text-[var(--color-text)]">{c.code}</code>
								{/if}
								<span class="text-sm font-medium text-[var(--color-text)]">{c.title}</span>
								<span class="text-xs font-semibold text-[var(--color-primary)]">{discountLabel(c, currencies)}</span>
								<span class="rounded-full px-2 py-0.5 text-xs font-medium {statusChip(c.status)}">
									{t(`coupons.status_${c.status}`, c.status)}
								</span>
							</div>
							<div class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--color-muted)]">
								<span>{t(`coupons.applies_${c.applies_to || 'both'}`, c.applies_to || 'both')}</span>
								{#if c.min_spend != null && c.min_spend > 0}
									<span>{t('coupons.min_spend_label', 'min')} {formatMoney(c.min_spend, c.currency, currencies, { minor: true })}</span>
								{/if}
								{#if c.valid_from || c.expires_at}
									<span>{fmtDate(c.valid_from) || '…'} → {fmtDate(c.expires_at) || '…'}</span>
								{/if}
								<span>{t('coupons.redeemed_count', '{count} redeemed', { count: c.redemption_count ?? 0 })}</span>
								<span>{t('coupons.listings_count', '{count} listings', { count: c.listing_count ?? 0 })}</span>
								{#if c.first_time_only}
									<span>· {t('coupons.first_time_badge', 'first-time only')}</span>
								{/if}
							</div>
						</div>
						<div class="flex shrink-0 items-center gap-1">
							{#if busyId === c.id}
								<Loader2 size={16} class="animate-spin text-[var(--color-muted)]" />
							{:else}
								<button
									type="button"
									onclick={() => openEdit(c)}
									class="rounded-[var(--radius)] p-1.5 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
									aria-label={t('common.edit', 'Edit')}
								>
									<Pencil size={15} />
								</button>
								{#if c.status === 'active'}
									<button
										type="button"
										onclick={() => setStatus(c, 'suspended')}
										class="rounded-[var(--radius)] p-1.5 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-amber-600"
										aria-label={t('coupons.action_suspend', 'Suspend')}
										title={t('coupons.action_suspend', 'Suspend')}
									>
										<PauseCircle size={15} />
									</button>
								{:else if c.status === 'suspended'}
									<button
										type="button"
										onclick={() => setStatus(c, 'active')}
										class="rounded-[var(--radius)] p-1.5 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-success)]"
										aria-label={t('coupons.action_reactivate', 'Reactivate')}
										title={t('coupons.action_reactivate', 'Reactivate')}
									>
										<PlayCircle size={15} />
									</button>
								{/if}
								<button
									type="button"
									onclick={() => remove(c)}
									class="rounded-[var(--radius)] p-1.5 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-danger)]"
									aria-label={t('common.delete', 'Delete')}
								>
									<Trash2 size={15} />
								</button>
							{/if}
						</div>
					</div>
				</div>
			{/each}

			<!-- The route returns page/page_size without a total — next is
				enabled while a full page came back. -->
			{#if data.currentPage > 1 || hasMore}
				<div class="flex items-center justify-center gap-2 pt-2">
					{#if data.currentPage > 1}
						<a
							href="?page={data.currentPage - 1}"
							class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
						>
							{t('common.previous', 'Previous')}
						</a>
					{/if}
					<span class="text-xs text-[var(--color-muted)]">{t('common.page', 'Page')} {data.currentPage}</span>
					{#if hasMore}
						<a
							href="?page={data.currentPage + 1}"
							class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
						>
							{t('common.next', 'Next')}
						</a>
					{/if}
				</div>
			{/if}
		{/if}
	</div>
</div>
