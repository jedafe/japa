<script lang="ts">
	// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md (Verification Requests)
	// Renders the request form dynamically from the Form Builder (Section 06)
	// plus a suggested badge-type select fed by GET /verification-requests/types.
	// Flow: submit form → POST /forms/{id}/submissions → POST /verification-requests.
	import { t } from '$lib/i18n';
	import { invalidateAll } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { BadgeCheck, Loader2, Send, CreditCard, Wallet, FileCheck, Trash2, Upload } from 'lucide-svelte';
	import type { FormSubmissionResponse, MediaAsset } from '$lib/types';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import AppealButton from '$lib/components/appeals/AppealButton.svelte';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';

	let { data } = $props();

	const org = $derived(data.org);
	const self = $derived(data.self);
	const form = $derived(data.form);
	const types = $derived(data.types ?? []);
	const requests = $derived(data.requests ?? []);
	const isRequestMode = $derived(Boolean(org) || self);

	let withdrawing = $state('');
	let listError = $state('');
	let values = $state<Record<string, unknown>>({});
	let mediaField = $state<{ id: string; label: string; type: string } | null>(null);
	let requestedType = $state('');
	let paymentMethod = $state<'credit' | 'paystack' | 'stripe'>('credit');
	let loading = $state(false);
	let errorKey = $state('');
	let phase = $state<'form' | 'redirecting' | 'creating_request' | 'done'>('form');
	let submissionId = $state('');

	const sortedFields = $derived(form ? [...form.fields].sort((a, b) => a.sort_order - b.sort_order) : []);
	const feeCredits = $derived(form?.submission_fee_credits ?? 0);
	const isPaid = $derived(feeCredits > 0);
	const pageTitle = $derived(t('verification_request.title', 'Request verification — {site}'));

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]';

	async function withdrawRequest(id: string) {
		if (withdrawing || !confirm(t('verification_request.withdraw_confirm', 'Withdraw this request?'))) return;
		withdrawing = id;
		listError = '';
		const res = await api.delete(`/verification-requests/${id}`);
		if (res.error) {
			listError = res.error.message;
			withdrawing = '';
			return;
		}
		await invalidateAll();
		withdrawing = '';
	}

	const MEDIA_TYPES = new Set(['media', 'image_single', 'image_multi']);

	function mediaItems(label: string): { id: string; url?: string; alt?: string }[] {
		const v = values[label];
		return Array.isArray(v) ? v : [];
	}

	function mediaSingle(label: string): { id: string; url?: string; alt?: string } | null {
		const v = values[label];
		return v && typeof v === 'object' && !Array.isArray(v) ? (v as { id: string; url?: string; alt?: string }) : null;
	}

	function isValueEmpty(v: unknown): boolean {
		if (v == null) return true;
		if (typeof v === 'string') return v.trim() === '';
		if (Array.isArray(v)) return v.length === 0;
		return false;
	}

	function validate(): boolean {
		if (!form) return false;
		for (const f of sortedFields) {
			if (f.required && isValueEmpty(values[f.label])) {
				errorKey = 'verification_request.errors.field_required';
				return false;
			}
		}
		return true;
	}

	function onMediaSelect(asset: MediaAsset) {
		if (!mediaField) return;
		const item = { id: asset.id, url: asset.url, alt: asset.alt_text ?? undefined };
		if (mediaField.type === 'image_single') {
			values = { ...values, [mediaField.label]: item };
		} else {
			values = { ...values, [mediaField.label]: [...mediaItems(mediaField.label), item] };
		}
		mediaField = null;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (!form || !validate()) return;

		loading = true;
		try {
			const body: Record<string, unknown> = { field_values: values };
			if (isPaid) body.payment_method = paymentMethod;

			const res = await api.post<FormSubmissionResponse>(`/forms/${form.id}/submissions`, body);
			if (!res.data) {
				errorKey = res.error?.code === 'insufficient_credits'
					? 'verification_request.errors.insufficient_credits'
					: res.error?.code === 'payment_intent_failed'
						? 'verification_request.errors.payment_failed'
						: 'verification_request.errors.submit_failed';
				return;
			}
			if (res.data.status === 'pending_payment' && res.data.checkout_url) {
				submissionId = res.data.id;
				phase = 'redirecting';
				window.location.href = res.data.checkout_url;
				return;
			}
			submissionId = res.data.id;
			await createRequest();
		} catch {
			errorKey = 'verification_request.errors.submit_failed';
		} finally {
			loading = false;
		}
	}

	async function createRequest() {
		if (!submissionId) return;
		phase = 'creating_request';
		loading = true;
		try {
			const res = await api.post('/verification-requests', {
				entity_type: org ? 'organization' : 'user',
				organization_id: org?.id,
				requested_type: requestedType || undefined,
				form_submission_id: submissionId
			});
			if (res.data) {
				phase = 'done';
			} else {
				errorKey = 'verification_request.errors.request_failed';
				phase = 'form';
			}
		} catch {
			errorKey = 'verification_request.errors.request_failed';
			phase = 'form';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>{pageTitle}</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<div class="border-b border-[var(--color-border)] px-4 py-3" style="margin-inline: max(1rem, calc((100% - 48rem)/2))">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<BadgeCheck size={20} />
			{isRequestMode
				? t('verification_request.heading', 'Request verification')
				: t('verification_request.my_requests_title', 'My verification requests')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{isRequestMode
				? t('verification_request.subtitle', 'Submit evidence to request a verification badge. All requests are reviewed by our team.')
				: t('verification_request.my_requests_subtitle', 'Track the status of your verification requests.')}
		</p>
	</div>

	{#if !isRequestMode}
		<div class="mx-auto w-full max-w-3xl p-4">
			{#if listError}
				<p class="mb-3 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">{listError}</p>
			{/if}
			<div class="mb-3 flex gap-3 text-sm">
				<a href="/verification-request?self=1" class="text-[var(--color-primary)] hover:underline">
					{t('verification_request.new_profile', 'Request profile verification')}
				</a>
			</div>
			{#if requests.length === 0}
				<EmptyState
					icon={FileCheck}
					title={t('verification_request.my_requests_empty', 'No verification requests')}
					body={t('verification_request.my_requests_empty_body', 'Requests you submit from your settings or an organization you administer will appear here.')}
				/>
			{:else}
				<ul class="space-y-2">
					{#each requests as req (req.id)}
						<li class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<div class="flex items-start justify-between gap-3">
								<div class="min-w-0">
									{#if req.entity_type === 'organization' && req.org_slug}
										<a href="/org/{req.org_slug}" class="block truncate text-sm font-medium text-[var(--color-text)] hover:text-[var(--color-primary)]">{req.org_name}</a>
									{:else}
										<span class="block truncate text-sm font-medium text-[var(--color-text)]">{t('verification_request.profile_request', 'Profile verification')}</span>
									{/if}
									<p class="mt-0.5 text-xs text-[var(--color-muted)]">
										{t('verification_request.request_filed', 'Filed {date}').replace('{date}', new Date(req.created_at).toLocaleDateString())}
										{#if req.reviewed_at}
											· {t('verification_request.request_reviewed', 'Reviewed {date}').replace('{date}', new Date(req.reviewed_at).toLocaleDateString())}
										{/if}
									</p>
									{#if req.resolution_note}
										<p class="mt-1 text-xs text-[var(--color-text-subtle)]">{req.resolution_note}</p>
									{/if}
								</div>
								<div class="flex shrink-0 items-center gap-2">
									<span class="rounded-full px-2.5 py-0.5 text-xs font-medium {req.status === 'approved'
										? 'bg-[var(--color-success)]/10 text-[var(--color-success)]'
										: req.status === 'rejected' || req.status === 'withdrawn'
											? 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]'
											: 'bg-[var(--color-warning)]/10 text-[var(--color-warning)]'}">
										{t(`verification_request.status.${req.status}`, req.status)}
									</span>
									{#if req.status === 'pending'}
										<button
											type="button"
											onclick={() => withdrawRequest(req.id)}
											disabled={withdrawing === req.id}
											class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-danger)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
										>
											{#if withdrawing === req.id}<Loader2 size={12} class="animate-spin" />{:else}<Trash2 size={12} />{/if}
											{t('verification_request.withdraw', 'Withdraw')}
										</button>
									{/if}
									{#if req.status === 'rejected'}
										<AppealButton entityType="verification_request" entityId={req.id} class="!px-2.5 !py-1 text-xs" />
									{/if}
								</div>
							</div>
						</li>
					{/each}
				</ul>
			{/if}
		</div>
	{:else}
	<div class="mx-auto w-full max-w-3xl p-4">
		<div class="mb-4 flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<span class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]">
				<BadgeCheck size={18} />
			</span>
			<div class="min-w-0">
				<p class="truncate text-sm font-medium text-[var(--color-text)]">
					{org ? org.name : t('verification_request.profile_request', 'Profile verification')}
				</p>
				{#if org}<p class="truncate text-xs text-[var(--color-muted)]">/{org.slug}</p>{/if}
			</div>
		</div>

		{#if phase === 'done'}
			<div class="rounded-[var(--radius-card)] border border-green-500/30 bg-green-500/10 p-6 text-center">
				<p class="text-lg font-medium text-green-600">{t('verification_request.request_created', 'Your verification request has been submitted and will be reviewed by our team.')}</p>
				<a href="/verification-request" class="mt-4 inline-block rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
					{t('verification_request.my_requests_link', 'My requests')}
				</a>
			</div>
		{:else if phase === 'redirecting'}
			<div class="flex flex-col items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 text-center">
				<Loader2 size={24} class="animate-spin text-[var(--color-primary)]" />
				<p class="text-sm text-[var(--color-text)]">{t('verification_request.redirecting_payment', 'Redirecting to payment provider…')}</p>
			</div>
		{:else if !form}
			<EmptyState icon={BadgeCheck} title={t('verification_request.form_unavailable', 'Verification requests unavailable')} body={t('verification_request.form_unavailable_body', 'The request form is not available right now. Please try again later.')} />
		{:else}
			<form onsubmit={submit} class="space-y-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4" novalidate>
				{#if form.description}<p class="text-sm text-[var(--color-text-subtle)]">{form.description}</p>{/if}

				{#if types.length > 0}
					<div class="flex flex-col gap-1.5">
						<label for="requested-type" class="text-sm font-medium text-[var(--color-text)]">
							{t('verification_request.requested_type', 'Requested badge type')} <span class="text-xs font-normal text-[var(--color-muted)]">({t('verification_request.optional', 'optional')})</span>
						</label>
						<AutocompleteSelect
							id="requested-type"
							value={requestedType}
							options={[{ value: '', label: t('verification_request.type_placeholder', 'Let reviewers decide') }, ...types.map((v) => ({ value: v, label: v.replaceAll('_', ' ') }))]}
							onValueChange={(v) => (requestedType = v)}
						/>
					</div>
				{/if}

				{#each sortedFields as field (field.id)}
					<div class="flex flex-col gap-1.5">
						<label for="field-{field.id}" class="text-sm font-medium text-[var(--color-text)]">
							{field.label}
							{#if field.required}<span class="text-red-500" aria-hidden="true">*</span>{/if}
						</label>
						{#if field.type === 'textarea' || field.type === 'long_text'}
							<textarea id="field-{field.id}" rows="4" value={String(values[field.label] ?? '')} oninput={(e) => (values = { ...values, [field.label]: e.currentTarget.value })} class={inputClass}></textarea>
						{:else if field.type === 'select' && field.options && field.options.length > 0}
							<AutocompleteSelect
								id="field-{field.id}"
								value={String(values[field.label] ?? '')}
								options={[{ value: '', label: t('verification_request.select_placeholder', 'Select an option') }, ...field.options.map((opt) => ({ value: opt, label: opt }))]}
								onValueChange={(v) => (values = { ...values, [field.label]: v })}
							/>
						{:else if MEDIA_TYPES.has(field.type)}
							{@const single = mediaSingle(field.label)}
							{@const items = mediaItems(field.label)}
							<div class="space-y-2">
								{#if single}
									<div class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm">
										{#if single.url}<img src={single.url} alt={single.alt ?? ''} class="h-10 w-10 rounded object-cover" />{/if}
										<span class="min-w-0 flex-1 truncate text-[var(--color-muted)]">{single.url ?? single.id}</span>
										<button type="button" onclick={() => (values = { ...values, [field.label]: '' })} class="text-[var(--color-danger)]"><Trash2 size={14} /></button>
									</div>
								{/if}
								{#each items as item, i (item.id)}
									<div class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm">
										{#if item.url}<img src={item.url} alt={item.alt ?? ''} class="h-10 w-10 rounded object-cover" />{/if}
										<span class="min-w-0 flex-1 truncate text-[var(--color-muted)]">{item.url ?? item.id}</span>
										<button type="button" onclick={() => (values = { ...values, [field.label]: items.filter((_, j) => j !== i) })} class="text-[var(--color-danger)]"><Trash2 size={14} /></button>
									</div>
								{/each}
								<button
									type="button"
									onclick={() => (mediaField = { id: field.id, label: field.label, type: field.type })}
									class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-dashed border-[var(--color-border)] px-3 py-2 text-sm text-[var(--color-muted)] transition-colors hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]"
								>
									<Upload size={14} />
									{field.type === 'image_single' && single ? t('verification_request.media_replace', 'Replace image') : field.type === 'image_single' ? t('verification_request.media_pick_image', 'Upload image') : t('verification_request.media_add', 'Upload file')}
								</button>
							</div>
						{:else if field.type === 'checkbox'}
							<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
								<input type="checkbox" id="field-{field.id}" checked={values[field.label] === 'true'} onchange={(e) => (values = { ...values, [field.label]: e.currentTarget.checked ? 'true' : '' })} class="h-4 w-4 rounded border-[var(--color-border)]" />
								{field.label}
							</label>
						{:else}
							<input id="field-{field.id}" type={field.type === 'email' ? 'email' : field.type === 'number' ? 'number' : field.type === 'url' ? 'url' : 'text'} value={String(values[field.label] ?? '')} oninput={(e) => (values = { ...values, [field.label]: e.currentTarget.value })} class={inputClass} />
						{/if}
					</div>
				{/each}

				{#if isPaid}
					<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4">
						<p class="mb-1 text-sm font-medium text-[var(--color-text)]">{t('verification_request.payment_required', 'Submission fee')}</p>
						<p class="mb-3 text-xs text-[var(--color-muted)]">{t('verification_request.payment_hint', 'This form requires a submission fee of {credits} credits.').replace('{credits}', String(feeCredits))}</p>
						<div class="flex flex-col gap-2">
							<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
								<input type="radio" name="payment_method" value="credit" checked={paymentMethod === 'credit'} onchange={() => (paymentMethod = 'credit')} class="h-4 w-4" />
								<Wallet size={16} class="text-[var(--color-primary)]" />
								{t('verification_request.pay_with_credits', 'Pay with credits ({credits})').replace('{credits}', String(feeCredits))}
							</label>
							<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
								<input type="radio" name="payment_method" value="paystack" checked={paymentMethod === 'paystack'} onchange={() => (paymentMethod = 'paystack')} class="h-4 w-4" />
								<CreditCard size={16} class="text-[var(--color-primary)]" />
								{t('verification_request.pay_with_gateway', 'Pay with card (Paystack)')}
							</label>
							<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
								<input type="radio" name="payment_method" value="stripe" checked={paymentMethod === 'stripe'} onchange={() => (paymentMethod = 'stripe')} class="h-4 w-4" />
								<CreditCard size={16} class="text-[var(--color-primary)]" />
								{t('verification_request.pay_with_stripe', 'Pay with card (Stripe)')}
							</label>
						</div>
					</div>
				{/if}

				{#if errorKey}
					<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">{t(errorKey)}</p>
				{/if}

				<div class="flex items-center justify-between gap-3">
					<a href="/verification-request" class="text-sm text-[var(--color-muted)] hover:underline">{t('verification_request.my_requests_link', 'My requests')}</a>
					<button type="submit" disabled={loading} class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50">
						{#if loading}<Loader2 size={16} class="animate-spin" />{:else}<Send class="rtl:-scale-x-100" size={16} />{/if}
						{loading ? (phase === 'creating_request' ? t('verification_request.creating_request', 'Creating request…') : '…') : (form.submit_label || t('verification_request.submit', 'Submit request'))}
					</button>
				</div>
			</form>
		{/if}
	</div>
	{/if}
</div>

{#if mediaField}
	<MediaLibraryModal
		allowedType={mediaField.type === 'media' ? '' : 'image'}
		uploadContext="document"
		onSelect={onMediaSelect}
		onClose={() => (mediaField = null)}
	/>
{/if}
