<script lang="ts">
	// Ref: japaconnect-engineering/sections/114-services-marketplace.md
	// §114 — attach the selling org's coupons to this listing. Selection is
	// collected here; the parent submits it via PUT /services/{id}/coupons
	// (replace-on-save) after the listing create/patch succeeds.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { BadgePercent, Loader2, AlertCircle } from 'lucide-svelte';
	import type { ListingCoupon, OrgCoupon } from '$lib/types';
	import { discountLabel } from '$lib/marketplace';
	import { couponAppliesToKind } from './serviceForm';
	import { page } from '$app/state';
	import type { CurrencyInfo } from '$lib/utils/currency';

	interface Props {
		/** Selling org UUID — coupons are org-owned; the attach endpoint
		 *  rejects ids owned by any other org. '' hides the list. */
		orgId: string;
		/** Listing content id (edit flow) — seeds the attached set from
		 *  GET /services/{id}/coupons so already-attached coupons stay
		 *  checked. '' on the create flow (nothing attached yet). */
		listingContentId?: string;
		listingKind: 'service' | 'package';
		selectedIds?: string[];
		/** Flips true once the attached-set seed has resolved (edit flow) —
		 *  the parent must not PUT a [] selection gathered before the seed
		 *  arrived or it would silently detach every coupon. */
		ready?: boolean;
	}

	let { orgId, listingContentId = '', listingKind, selectedIds = $bindable([]), ready = $bindable(!listingContentId) }: Props = $props();

	// Client cap mirrors the seeded default
	// service_packages.coupons.max_per_listing=3 — the PUT re-validates.
	const MAX_ATTACH = 3;

	const currencies = $derived((page.data.currencies ?? []) as CurrencyInfo[]);

	let coupons = $state<OrgCoupon[] | null>(null);
	let loadError = $state(false);
	// The listing's attached set is fetched once per listing id — suspended/
	// expired attachments stay checked (the public payload hides them, so
	// seeding from svc.coupons would silently detach them on save).
	let seededFor = $state('');

	$effect(() => {
		const id = orgId;
		coupons = null;
		loadError = false;
		// An org switch invalidates every prior selection — attached coupons
		// must belong to the listing's org.
		selectedIds = [];
		if (!id) return;
		(async () => {
			const res = await api.get<OrgCoupon[]>(`/organizations/${id}/coupons?page_size=100`);
			if (orgId !== id) return; // stale response after a fast re-pick
			if (res.data) coupons = res.data;
			else loadError = true;
		})();
	});

	$effect(() => {
		if (!listingContentId || seededFor === listingContentId) return;
		seededFor = listingContentId;
		(async () => {
			const res = await api.get<ListingCoupon[]>(`/services/${listingContentId}/coupons`);
			if (res.data) selectedIds = res.data.map((c) => c.id);
			// Ready either way — a failed seed leaves the parent's PUT gate
			// closed only if we keep ready=false; flag the failure instead so
			// the selector itself stays honest and the PUT stays skipped.
			if (res.error) {
				loadError = true;
			} else {
				ready = true;
			}
		})();
	});

	// Only active coupons whose applies_to covers this listing kind are
	// attachable — the backend rejects the rest; suspended/expired stay
	// visible (disabled) so an attached-but-inactive coupon is explainable.
	const selectable = $derived(
		(coupons ?? []).filter((c) => c.status === 'active' && couponAppliesToKind(c.applies_to, listingKind))
	);
	const ineligible = $derived(
		(coupons ?? []).filter((c) => c.status === 'active' && !couponAppliesToKind(c.applies_to, listingKind))
	);

	function toggle(id: string) {
		if (selectedIds.includes(id)) {
			selectedIds = selectedIds.filter((x) => x !== id);
		} else if (selectedIds.length < MAX_ATTACH) {
			selectedIds = [...selectedIds, id];
		}
	}

	function expiryLabel(c: OrgCoupon): string {
		if (!c.expires_at) return '';
		const d = new Date(c.expires_at);
		if (Number.isNaN(d.getTime())) return '';
		return ` · ${t('services.coupon_expires', 'expires')} ${d.toLocaleDateString()}`;
	}
</script>

<div>
	<span class="mb-1 flex items-center gap-1.5 text-sm font-medium text-[var(--color-text)]">
		<BadgePercent size={15} class="text-[var(--color-primary)]" />
		{t('services.coupons_label', 'Coupons & deals')}
	</span>
	<p class="mb-2 text-xs text-[var(--color-muted)]">
		{t('services.coupons_hint', 'Attach up to {max} of your organization’s coupons — travellers see the discount on the listing.', { max: MAX_ATTACH })}
	</p>

	{#if !orgId}
		<p class="text-xs text-[var(--color-muted)]">{t('services.coupons_pick_org', 'Select a selling organization first.')}</p>
	{:else if loadError}
		<p class="flex items-center gap-1.5 text-xs text-[var(--color-danger)]" role="alert">
			<AlertCircle size={13} /> {t('services.coupons_load_error', 'Could not load organization coupons')}
		</p>
	{:else if coupons === null}
		<p class="flex items-center gap-1.5 text-xs text-[var(--color-muted)]">
			<Loader2 size={13} class="animate-spin" /> {t('common.loading', 'Loading…')}
		</p>
	{:else if coupons.length === 0}
		<p class="text-xs text-[var(--color-muted)]">{t('services.coupons_none', 'This organization has no coupons yet.')}</p>
	{:else}
		<div class="space-y-1.5">
			{#each selectable as c (c.id)}
				<label
					class="flex cursor-pointer items-center gap-2 rounded-[var(--radius)] border px-3 py-2 text-sm transition-colors {selectedIds.includes(c.id)
						? 'border-[var(--color-primary)] bg-[var(--color-primary)]/5'
						: 'border-[var(--color-border)] bg-[var(--color-surface)] hover:bg-[var(--color-surface-subtle)]'}"
				>
					<input
						type="checkbox"
						checked={selectedIds.includes(c.id)}
						disabled={!selectedIds.includes(c.id) && selectedIds.length >= MAX_ATTACH}
						onchange={() => toggle(c.id)}
						class="h-3.5 w-3.5 rounded border-[var(--color-border)]"
					/>
					{#if c.code}
						<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 font-mono text-xs font-semibold text-[var(--color-text)]">{c.code}</span>
					{/if}
					<span class="flex-1 truncate text-[var(--color-text)]">{c.title}</span>
					<span class="text-xs font-medium text-[var(--color-primary)]">
						{discountLabel(c, currencies)}{expiryLabel(c)}
					</span>
				</label>
			{/each}
			{#each ineligible as c (c.id)}
				<div
					class="flex items-center gap-2 rounded-[var(--radius)] border border-dashed border-[var(--color-border)] px-3 py-2 text-sm opacity-60"
					title={t('services.coupon_wrong_kind_title', 'This coupon does not apply to this listing type')}
				>
					<input type="checkbox" disabled class="h-3.5 w-3.5 rounded border-[var(--color-border)]" />
					{#if c.code}
						<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 font-mono text-xs">{c.code}</span>
					{/if}
					<span class="flex-1 truncate text-[var(--color-muted)]">{c.title}</span>
					<span class="text-xs text-[var(--color-muted)]">{t('services.coupon_wrong_kind', 'Not for this listing type')}</span>
				</div>
			{/each}
		</div>
	{/if}
</div>
