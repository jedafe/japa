<script lang="ts">
	// Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md
	// Shared deal (coupon) form: staff create/edit via /admin/coupons and
	// organization submissions via /organizations/{id}/deals.
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { slugify } from '$lib/studio/prompt';
	import { Plus, Loader2, AlertCircle, Trash2 } from 'lucide-svelte';
	import type { Coupon, DealPartnerRef } from '$lib/types';
	import {
		buildCouponPayload,
		buildProviderPayload,
		couponToFormValues,
		emptyDealFormValues,
		emptyNewProviderValues,
		type DealFormProps,
		type NewProviderValues
	} from './dealForm';

	let {
		mode,
		audience,
		coupon,
		providers = [],
		canManageProviders = false,
		orgId,
		currencies,
		onsaved,
		oncancel
	}: DealFormProps = $props();

	// The coupon prop only seeds the initial form state (untrack — prop changes
	// after mount must not clobber in-progress edits).
	let v = $state(untrack(() => (coupon ? couponToFormValues(coupon) : emptyDealFormValues())));

	// Slug auto-fills from the title until the user edits the slug field;
	// in edit mode the seeded slug is never auto-overwritten.
	let slugTouched = $state(untrack(() => Boolean(coupon?.slug)));

	// Inline "+ New provider" block (staff only, permission-gated upstream).
	let npOpen = $state(false);
	let np = $state<NewProviderValues>(emptyNewProviderValues());
	let npSlugTouched = $state(false);

	let submitting = $state(false);
	let errorMsg = $state('');

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';
	const labelClass = 'mb-1 block text-sm font-medium text-[var(--color-text)]';

	// Org-linked providers stay inactive until their first deal is approved
	// (§55 — see providerForOrg in internal/coupons/service.go), and
	// ListActiveDealPartners only returns active rows. A staff edit of such a
	// deal would otherwise render the provider select blank and a save would
	// NULL provider_id — prepend a sentinel option so the select keeps it.
	const providerOptions = $derived.by((): DealPartnerRef[] => {
		const currentId = coupon?.provider_id;
		if (currentId && !providers.some((p) => p.id === currentId)) {
			const label =
				coupon?.provider_name ?? t('deal.form.provider_current', 'Current provider');
			return [
				{
					id: currentId,
					slug: '',
					is_featured: false,
					name: `${label} (${t('deal.form.provider_inactive', 'inactive')})`
				},
				...providers
			];
		}
		return providers;
	});

	// Rule 16: provider options come from the providers prop; the category
	// datalist derives from those providers — no hardcoded option lists.
	const providerCategories = $derived(
		[
			...new Set(
				providers
					.map((p) => p.category)
					.filter((c): c is string => typeof c === 'string' && c.length > 0)
			)
		].sort((a, b) => a.localeCompare(b))
	);

	// Lifecycle states pending/expired are server-managed, not form options —
	// but when editing a coupon already in one, surface it so a save doesn't
	// silently change status.
	const statusOptions = $derived(
		v.status && !['active', 'suspended'].includes(v.status)
			? ['active', 'suspended', v.status]
			: ['active', 'suspended']
	);

	function onTitleInput() {
		if (!slugTouched) v.slug = slugify(v.title);
	}

	function onSlugInput() {
		// Clearing the field re-enables title-driven auto-fill.
		slugTouched = v.slug.trim() !== '';
	}

	function onProviderNameInput() {
		if (!npSlugTouched) np.slug = slugify(np.name);
	}

	function onProviderSlugInput() {
		npSlugTouched = np.slug.trim() !== '';
	}

	function statusLabel(status: string): string {
		if (status === 'active') return t('deal.form.status_active', 'Active');
		if (status === 'suspended') return t('deal.form.status_suspended', 'Suspended');
		// pending/expired are server-managed lifecycle states surfaced read-only
		// when editing a coupon already in them.
		if (status === 'pending') return t('deals.status.pending', 'Pending');
		if (status === 'expired') return t('deals.status.expired', 'Expired');
		return status;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorMsg = '';
		if (!v.title.trim()) {
			errorMsg = t('deal.form.errors.title_required', 'Title is required');
			return;
		}
		if (audience === 'org' && !orgId) {
			errorMsg = t('deal.form.errors.org_required', 'Organization is required');
			return;
		}

		submitting = true;
		try {
			let providerId = v.provider_id;

			// Staff may attach a brand-new provider inline — create it first so
			// the coupon can reference the returned id.
			if (audience === 'staff' && canManageProviders && npOpen && np.name.trim()) {
				if (!np.slug.trim()) np.slug = slugify(np.name);
				const pres = await api.post<{ id: string }>(
					'/admin/affiliate-providers',
					buildProviderPayload(np)
				);
				if (!pres.data) {
					errorMsg =
						pres.error?.message ??
						t('deal.form.errors.provider_failed', 'Failed to create provider');
					return;
				}
				providerId = pres.data.id;
			}

			const payload = buildCouponPayload({ ...v, provider_id: providerId }, audience);

			let res;
			if (mode === 'edit' && coupon) {
				res = await api.patch<Coupon>(`/admin/coupons/${coupon.id}`, payload);
			} else if (audience === 'org') {
				res = await api.post<Coupon>(`/organizations/${orgId}/deals`, payload);
			} else {
				res = await api.post<Coupon>('/admin/coupons', payload);
			}

			if (res.data) {
				onsaved?.(res.data);
			} else {
				errorMsg =
					res.error?.message ?? t('deal.form.errors.save_failed', 'Failed to save deal');
			}
		} catch (err) {
			errorMsg =
				err instanceof Error
					? err.message
					: t('deal.form.errors.save_failed', 'Failed to save deal');
		} finally {
			submitting = false;
		}
	}

	const submitLabel = $derived(
		mode === 'edit'
			? t('deal.form.submit_edit', 'Save changes')
			: audience === 'org'
				? t('deal.form.submit_org', 'Submit deal')
				: t('deal.form.submit_create', 'Create deal')
	);
</script>

<form onsubmit={submit} class="space-y-5" novalidate>
	<!-- Provider (staff only) -->
	{#if audience === 'staff'}
		<div>
			<label for="deal-provider" class={labelClass}>
				{t('deal.form.provider', 'Provider')}
			</label>
			<div class="flex items-center gap-2">
				<select id="deal-provider" bind:value={v.provider_id} class={inputClass}>
					<option value="">{t('deal.form.provider_none', 'No provider (internal deal)')}</option>
					{#each providerOptions as p (p.id)}
						<option value={p.id}>{p.name}{p.category ? ` — ${p.category}` : ''}</option>
					{/each}
				</select>
				{#if canManageProviders}
					<button
						type="button"
						onclick={() => (npOpen = !npOpen)}
						class="shrink-0 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
						aria-expanded={npOpen}
					>
						<Plus size={14} /> {t('deal.form.provider_new', 'New provider')}
					</button>
				{/if}
			</div>
		</div>

		{#if canManageProviders && npOpen}
			<div
				class="space-y-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4"
			>
				<p class="text-sm font-medium text-[var(--color-text)]">
					{t('deal.form.provider_new_heading', 'New provider')}
				</p>
				<div class="grid gap-4 sm:grid-cols-2">
					<div>
						<label for="np-name" class={labelClass}>
							{t('deal.form.provider_name', 'Provider name')} <span class="text-red-500">*</span>
						</label>
						<input
							id="np-name"
							type="text"
							bind:value={np.name}
							oninput={onProviderNameInput}
							class={inputClass}
						/>
					</div>
					<div>
						<label for="np-slug" class={labelClass}>{t('deal.form.slug', 'Slug')}</label>
						<input
							id="np-slug"
							type="text"
							bind:value={np.slug}
							oninput={onProviderSlugInput}
							class={inputClass}
						/>
					</div>
					<div>
						<label for="np-category" class={labelClass}>
							{t('deal.form.provider_category', 'Category')}
						</label>
						<input
							id="np-category"
							type="text"
							list="deal-provider-categories"
							bind:value={np.category}
							class={inputClass}
						/>
						<datalist id="deal-provider-categories">
							{#each providerCategories as cat (cat)}
								<option value={cat}></option>
							{/each}
						</datalist>
					</div>
					<div>
						<label for="np-base-url" class={labelClass}>
							{t('deal.form.provider_base_url', 'Base URL')}
						</label>
						<input
							id="np-base-url"
							type="url"
							bind:value={np.base_url}
							placeholder="https://example.com"
							class={inputClass}
						/>
					</div>
				</div>
				<div>
					<label for="np-description" class={labelClass}>
						{t('deal.form.provider_description', 'Provider description')}
					</label>
					<textarea id="np-description" bind:value={np.description} rows={2} class={inputClass}
					></textarea>
				</div>
			</div>
		{/if}
	{/if}

	<!-- Title + slug -->
	<div class="grid gap-4 sm:grid-cols-2">
		<div class="sm:col-span-2">
			<label for="deal-title" class={labelClass}>
				{t('deal.form.title', 'Title')} <span class="text-red-500">*</span>
			</label>
			<input
				id="deal-title"
				type="text"
				bind:value={v.title}
				oninput={onTitleInput}
				placeholder={t('deal.form.title_placeholder', 'e.g. 20% off airport transfers')}
				class={inputClass}
			/>
		</div>
		<div>
			<label for="deal-slug" class={labelClass}>{t('deal.form.slug', 'Slug')}</label>
			<input
				id="deal-slug"
				type="text"
				bind:value={v.slug}
				oninput={onSlugInput}
				class={inputClass}
			/>
		</div>
		<div>
			<label for="deal-code" class={labelClass}>{t('deal.form.code', 'Coupon code')}</label>
			<input
				id="deal-code"
				type="text"
				bind:value={v.code}
				placeholder={t('deal.form.code_placeholder', 'Optional — e.g. SAVE20')}
				class={inputClass}
			/>
		</div>
	</div>

	<!-- Discount -->
	<div class="grid gap-4 sm:grid-cols-3">
		<div>
			<label for="deal-discount-type" class={labelClass}>
				{t('deal.form.discount_type', 'Discount type')}
			</label>
			<select id="deal-discount-type" bind:value={v.discount_type} class={inputClass}>
				<option value="">{t('deal.form.discount_none', 'None / see description')}</option>
				<option value="percent">{t('deal.form.discount_percent', 'Percent off')}</option>
				<option value="fixed">{t('deal.form.discount_fixed', 'Fixed amount off')}</option>
				<option value="free_shipping">{t('deal.form.discount_free_shipping', 'Free shipping')}</option>
			</select>
		</div>
		<div>
			<label for="deal-discount-value" class={labelClass}>
				{t('deal.form.discount_value', 'Discount value')}
			</label>
			<input
				id="deal-discount-value"
				type="number"
				min="0"
				step="any"
				bind:value={v.discount_value}
				class={inputClass}
			/>
		</div>
		<div>
			<label for="deal-currency" class={labelClass}>{t('deal.form.currency', 'Currency')}</label>
			{#if currencies && currencies.length > 0}
				<input
					id="deal-currency"
					type="text"
					list="deal-currencies"
					bind:value={v.currency}
					placeholder="USD"
					class={inputClass}
				/>
				<datalist id="deal-currencies">
					{#each currencies as c (c.code)}
						<option value={c.code}>{c.code} — {c.name}</option>
					{/each}
				</datalist>
			{:else}
				<input
					id="deal-currency"
					type="text"
					bind:value={v.currency}
					placeholder="USD"
					class={inputClass}
				/>
			{/if}
		</div>
	</div>

	<!-- URLs -->
	<div class="grid gap-4 sm:grid-cols-2">
		<div>
			<label for="deal-referral-url" class={labelClass}>
				{t('deal.form.referral_url', 'Referral / claim URL')}
			</label>
			<input
				id="deal-referral-url"
				type="url"
				bind:value={v.referral_url}
				placeholder="https://partner.example/offer"
				class={inputClass}
			/>
		</div>
		<div>
			<label for="deal-terms-url" class={labelClass}>
				{t('deal.form.terms_url', 'Terms URL')}
			</label>
			<input
				id="deal-terms-url"
				type="url"
				bind:value={v.terms_url}
				placeholder="https://partner.example/terms"
				class={inputClass}
			/>
		</div>
	</div>

	<!-- Description -->
	<div>
		<label for="deal-description" class={labelClass}>
			{t('deal.form.description', 'Description')}
		</label>
		<textarea
			id="deal-description"
			bind:value={v.description}
			rows={3}
			placeholder={t('deal.form.description_placeholder', 'Short description shown on the deal card…')}
			class={inputClass}
		></textarea>
	</div>

	<!-- Validity -->
	<div class="grid gap-4 sm:grid-cols-2">
		<div>
			<label for="deal-valid-from" class={labelClass}>
				{t('deal.form.valid_from', 'Valid from')}
			</label>
			<input id="deal-valid-from" type="datetime-local" bind:value={v.valid_from} class={inputClass} />
		</div>
		<div>
			<label for="deal-expires-at" class={labelClass}>
				{t('deal.form.expires_at', 'Expires at')}
			</label>
			<input id="deal-expires-at" type="datetime-local" bind:value={v.expires_at} class={inputClass} />
		</div>
	</div>

	<!-- Staff-only moderation controls -->
	{#if audience === 'staff'}
		<div class="grid gap-4 sm:grid-cols-2">
			<div>
				<label for="deal-status" class={labelClass}>{t('deal.form.status', 'Status')}</label>
				<select id="deal-status" bind:value={v.status} class={inputClass}>
					{#each statusOptions as s (s)}
						<option value={s}>{statusLabel(s)}</option>
					{/each}
				</select>
			</div>
			<div class="flex items-end pb-2">
				<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
					<input
						type="checkbox"
						bind:checked={v.is_verified}
						class="h-4 w-4 rounded border-[var(--color-border)]"
					/>
					{t('deal.form.is_verified', 'Verified deal')}
				</label>
			</div>
		</div>
	{/if}

	<!-- Details editor (coupons.details JSONB — /deal page content) -->
	<fieldset class="space-y-4 rounded-[var(--radius)] border border-[var(--color-border)] p-4">
		<legend class="px-1 text-sm font-medium text-[var(--color-text)]">
			{t('deal.form.details_heading', 'Deal page details')}
		</legend>

		<div>
			<label for="deal-summary" class={labelClass}>{t('deal.form.summary', 'Summary')}</label>
			<textarea
				id="deal-summary"
				bind:value={v.details.summary}
				rows={3}
				placeholder={t('deal.form.summary_placeholder', 'Editorial summary shown on the deal page…')}
				class={inputClass}
			></textarea>
		</div>

		<!-- Sections repeater -->
		<div>
			<span class={labelClass}>{t('deal.form.sections', 'Sections')}</span>
			<div class="space-y-2">
				{#each v.details.sections as section, idx (idx)}
					<div
						class="space-y-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
					>
						<div class="flex items-center gap-2">
							<input
								type="text"
								bind:value={section.title}
								placeholder={t('deal.form.section_title', 'Section title')}
								aria-label={t('deal.form.section_title', 'Section title')}
								class={inputClass}
							/>
							<button
								type="button"
								onclick={() => (v.details.sections = v.details.sections.filter((_, i) => i !== idx))}
								class="shrink-0 text-[var(--color-muted)] hover:text-[var(--color-danger)]"
								aria-label={t('common.remove', 'Remove')}
							>
								<Trash2 size={16} />
							</button>
						</div>
						<textarea
							bind:value={section.body}
							rows={2}
							placeholder={t('deal.form.section_body', 'Section body')}
							aria-label={t('deal.form.section_body', 'Section body')}
							class={inputClass}
						></textarea>
					</div>
				{/each}
			</div>
			<button
				type="button"
				onclick={() => (v.details.sections = [...v.details.sections, { title: '', body: '' }])}
				class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				<Plus size={14} /> {t('deal.form.add_section', 'Add section')}
			</button>
		</div>

		<!-- FAQ repeater -->
		<div>
			<span class={labelClass}>{t('deal.form.faq', 'FAQ')}</span>
			<div class="space-y-2">
				{#each v.details.faq as item, idx (idx)}
					<div
						class="space-y-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
					>
						<div class="flex items-center gap-2">
							<input
								type="text"
								bind:value={item.q}
								placeholder={t('deal.form.question', 'Question')}
								aria-label={t('deal.form.question', 'Question')}
								class={inputClass}
							/>
							<button
								type="button"
								onclick={() => (v.details.faq = v.details.faq.filter((_, i) => i !== idx))}
								class="shrink-0 text-[var(--color-muted)] hover:text-[var(--color-danger)]"
								aria-label={t('common.remove', 'Remove')}
							>
								<Trash2 size={16} />
							</button>
						</div>
						<textarea
							bind:value={item.a}
							rows={2}
							placeholder={t('deal.form.answer', 'Answer')}
							aria-label={t('deal.form.answer', 'Answer')}
							class={inputClass}
						></textarea>
					</div>
				{/each}
			</div>
			<button
				type="button"
				onclick={() => (v.details.faq = [...v.details.faq, { q: '', a: '' }])}
				class="mt-2 inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				<Plus size={14} /> {t('deal.form.add_question', 'Add question')}
			</button>
		</div>

		<!-- Trial group -->
		<div>
			<span class={labelClass}>{t('deal.form.trial_heading', 'Trial / onboarding')}</span>
			<div class="space-y-2">
				<div class="grid gap-4 sm:grid-cols-2">
					<input
						type="text"
						bind:value={v.details.trial.title}
						placeholder={t('deal.form.trial_title', 'Trial title')}
						aria-label={t('deal.form.trial_title', 'Trial title')}
						class={inputClass}
					/>
					<input
						type="url"
						bind:value={v.details.trial.url}
						placeholder={t('deal.form.trial_url', 'Trial URL')}
						aria-label={t('deal.form.trial_url', 'Trial URL')}
						class={inputClass}
					/>
				</div>
				<textarea
					bind:value={v.details.trial.body}
					rows={2}
					placeholder={t('deal.form.trial_body', 'Trial details')}
					aria-label={t('deal.form.trial_body', 'Trial details')}
					class={inputClass}
				></textarea>
			</div>
		</div>
	</fieldset>

	<!-- Error -->
	{#if errorMsg}
		<div
			class="flex items-center gap-2 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500"
			role="alert"
		>
			<AlertCircle size={16} /> {errorMsg}
		</div>
	{/if}

	<!-- Actions -->
	<div class="flex justify-end gap-2">
		{#if oncancel}
			<button
				type="button"
				onclick={oncancel}
				disabled={submitting}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
			>
				{t('common.cancel', 'Cancel')}
			</button>
		{/if}
		<button
			type="submit"
			disabled={submitting}
			class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
		>
			{#if submitting}
				<Loader2 size={16} class="animate-spin" />
			{:else}
				<Plus size={16} />
			{/if}
			{submitting ? t('common.submitting', 'Submitting…') : submitLabel}
		</button>
	</div>
</form>
