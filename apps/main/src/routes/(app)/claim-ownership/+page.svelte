<script lang="ts">
	// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
	// Renders the claim-ownership form dynamically from the Form Builder (Section 06).
	// No hardcoded fields — all fields come from GET /forms/claim-ownership/embed.
	// For paid forms, shows payment method choice (credits or card gateway).
	// The flow is: submit form → create claim with form_submission_id.
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { api } from '$lib/api/client';
	import { ShieldQuestion, Loader2, Send, CreditCard, Wallet } from 'lucide-svelte';
	import type { FormField, FormSubmissionResponse } from '$lib/types';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';

	let { data } = $props();

	const org = $derived(data.org);
	const form = $derived(data.form);

	// Field values keyed by field label (matches the backend's field_values contract).
	let values = $state<Record<string, string>>({});
	let paymentMethod = $state<'credit' | 'paystack' | 'stripe'>('credit');
	let loading = $state(false);
	let errorKey = $state('');
	let phase = $state<'form' | 'redirecting' | 'pending_payment' | 'creating_claim' | 'done'>('form');
	let submissionId = $state('');
	let checkoutUrl = $state('');

	const sortedFields = $derived(
		form ? [...form.fields].sort((a, b) => a.sort_order - b.sort_order) : []
	);

	const feeCredits = $derived(form?.submission_fee_credits ?? 0);
	const isPaid = $derived(feeCredits > 0);

	const pageTitle = $derived(t('claim_ownership.title', 'Claim ownership — JapaConnect'));
	const pageDescription = $derived(
		t('claim_ownership.description', 'Submit evidence to claim this organization.')
	);

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]';

	function validate(): boolean {
		if (!form) return false;
		for (const f of sortedFields) {
			if (f.required && !values[f.label]?.trim()) {
				errorKey = 'claim_ownership.errors.field_required';
				return false;
			}
		}
		return true;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (!form) return;
		if (!validate()) return;

		loading = true;
		try {
			// Step 1: Submit the form. For paid forms, include payment_method.
			const body: Record<string, unknown> = { field_values: values };
			if (isPaid) {
				body.payment_method = paymentMethod;
			}

			const res = await api.post<FormSubmissionResponse>(
				`/forms/${form.id}/submissions`,
				body
			);

			if (!res.data) {
				if (res.error?.code === 'insufficient_credits') {
					errorKey = 'claim_ownership.errors.insufficient_credits';
				} else if (res.error?.code === 'payment_intent_failed') {
					errorKey = 'claim_ownership.errors.payment_failed';
				} else {
					errorKey = 'claim_ownership.errors.submit_failed';
				}
				return;
			}

			// For gateway payments, redirect to checkout URL.
			if (res.data.status === 'pending_payment' && res.data.checkout_url) {
				submissionId = res.data.id;
				checkoutUrl = res.data.checkout_url;
				phase = 'redirecting';
				// Redirect to the payment provider's checkout page.
				window.location.href = res.data.checkout_url;
				return;
			}

			// For free or credit-paid submissions, create the claim immediately.
			submissionId = res.data.id;
			await createClaim();
		} catch {
			errorKey = 'claim_ownership.errors.submit_failed';
		} finally {
			loading = false;
		}
	}

	async function createClaim() {
		if (!submissionId) return;
		phase = 'creating_claim';
		loading = true;
		try {
			const res = await api.post('/claim-ownership', {
				organization_id: org.id,
				form_submission_id: submissionId
			});
			if (res.data) {
				phase = 'done';
			} else {
				errorKey = 'claim_ownership.errors.claim_failed';
				phase = 'form';
			}
		} catch {
			errorKey = 'claim_ownership.errors.claim_failed';
			phase = 'form';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>{pageTitle}</title>
	<meta name="description" content={pageDescription} />
	<meta property="og:title" content={pageTitle} />
	<meta property="og:description" content={pageDescription} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header -->
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<ShieldQuestion size={20} />
			{t('claim_ownership.heading', 'Claim ownership')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{t('claim_ownership.subtitle', 'Submit evidence to claim this organization.')}
		</p>
	</div>

	<div class="p-3">
		<!-- Org context card -->
		<div
			class="mb-4 flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
		>
			<span
				class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
			>
				<ShieldQuestion size={18} />
			</span>
			<div class="min-w-0">
				<p class="truncate text-sm font-medium text-[var(--color-text)]">{org.name}</p>
				<p class="truncate text-xs text-[var(--color-muted)]">/{org.slug}</p>
			</div>
		</div>

		{#if phase === 'done'}
			<div
				class="rounded-[var(--radius-card)] border border-green-500/30 bg-green-500/10 p-6 text-center"
			>
				<p class="text-lg font-medium text-green-600">
					{t('claim_ownership.claim_created')}
				</p>
				<a
					href="/org/{org.slug}"
					class="mt-4 inline-block rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					← {t('claim_ownership.back', 'Back to organization')}
				</a>
			</div>
		{:else if phase === 'redirecting'}
			<div
				class="flex flex-col items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 text-center"
			>
				<Loader2 size={24} class="animate-spin text-[var(--color-primary)]" />
				<p class="text-sm text-[var(--color-text)]">
					{t('claim_ownership.redirecting_payment', 'Redirecting to payment provider…')}
				</p>
			</div>
		{:else if !form}
			<EmptyState
				icon={ShieldQuestion}
				title={t('claim_ownership.form_unavailable')}
				body={t('claim_ownership.form_unavailable_body')}
			/>
		{:else}
			<form
				onsubmit={submit}
				class="space-y-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
				novalidate
			>
				{#if form.description}
					<p class="text-sm text-[var(--color-text-subtle)]">{form.description}</p>
				{/if}

				<!-- Dynamic form fields from the Form Builder (Section 06) -->
				{#each sortedFields as field (field.id)}
					<div class="flex flex-col gap-1.5">
						<label for="field-{field.id}" class="text-sm font-medium text-[var(--color-text)]">
							{field.label}
							{#if field.required}<span class="text-red-500" aria-hidden="true">*</span>{/if}
						</label>
						{#if field.type === 'textarea' || field.type === 'long_text'}
							<textarea
								id="field-{field.id}"
								rows="4"
								value={values[field.label] ?? ''}
								oninput={(e) => (values = { ...values, [field.label]: e.currentTarget.value })}
								class={inputClass}
							></textarea>
						{:else if field.type === 'select' && field.options && field.options.length > 0}
							<select
								id="field-{field.id}"
								value={values[field.label] ?? ''}
								onchange={(e) => (values = { ...values, [field.label]: e.currentTarget.value })}
								class={inputClass}
							>
								<option value="">{t('claim_ownership.select_placeholder', 'Select an option')}</option>
								{#each field.options as opt (opt)}
									<option value={opt}>{opt}</option>
								{/each}
							</select>
						{:else if field.type === 'checkbox'}
							<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
								<input
									type="checkbox"
									id="field-{field.id}"
									checked={values[field.label] === 'true'}
									onchange={(e) => (values = { ...values, [field.label]: e.currentTarget.checked ? 'true' : '' })}
									class="h-4 w-4 rounded border-[var(--color-border)]"
								/>
								{field.label}
							</label>
						{:else}
							<input
								id="field-{field.id}"
								type={field.type === 'email' ? 'email' : field.type === 'number' ? 'number' : field.type === 'url' ? 'url' : 'text'}
								value={values[field.label] ?? ''}
								oninput={(e) => (values = { ...values, [field.label]: e.currentTarget.value })}
								class={inputClass}
							/>
						{/if}
					</div>
				{/each}

				<!-- Payment method selection for paid forms (Section 06 Paid Form Submissions) -->
				{#if isPaid}
					<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4">
						<p class="mb-1 text-sm font-medium text-[var(--color-text)]">
							{t('claim_ownership.payment_required', 'Submission fee')}
						</p>
						<p class="mb-3 text-xs text-[var(--color-muted)]">
							{t('claim_ownership.payment_hint', 'This form requires a submission fee of {credits} credits.').replace('{credits}', String(feeCredits))}
						</p>
						<div class="flex flex-col gap-2">
							<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
								<input
									type="radio"
									name="payment_method"
									value="credit"
									checked={paymentMethod === 'credit'}
									onchange={() => (paymentMethod = 'credit')}
									class="h-4 w-4"
								/>
								<Wallet size={16} class="text-[var(--color-primary)]" />
								{t('claim_ownership.pay_with_credits', 'Pay with credits ({credits})').replace('{credits}', String(feeCredits))}
							</label>
							<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
								<input
									type="radio"
									name="payment_method"
									value="paystack"
									checked={paymentMethod === 'paystack'}
									onchange={() => (paymentMethod = 'paystack')}
									class="h-4 w-4"
								/>
								<CreditCard size={16} class="text-[var(--color-primary)]" />
								{t('claim_ownership.pay_with_gateway', 'Pay with card (Paystack)')}
							</label>
							<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
								<input
									type="radio"
									name="payment_method"
									value="stripe"
									checked={paymentMethod === 'stripe'}
									onchange={() => (paymentMethod = 'stripe')}
									class="h-4 w-4"
								/>
								<CreditCard size={16} class="text-[var(--color-primary)]" />
								{t('claim_ownership.pay_with_stripe', 'Pay with card (Stripe)')}
							</label>
						</div>
					</div>
				{/if}

				{#if errorKey}
					<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
						{t(errorKey)}
					</p>
				{/if}

				<div class="flex items-center justify-between gap-3">
					<a
						href="/org/{org.slug}"
						class="text-sm text-[var(--color-muted)] hover:underline"
					>
						← {t('claim_ownership.back', 'Back to organization')}
					</a>
					<button
						type="submit"
						disabled={loading}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						{#if loading}
							<Loader2 size={16} class="animate-spin" />
						{:else}
							<Send size={16} />
						{/if}
						{loading
							? (phase === 'creating_claim'
								? t('claim_ownership.creating_claim')
								: '...')
							: (form.submit_label || t('claim_ownership.submit', 'Submit claim'))}
					</button>
				</div>
			</form>
		{/if}
	</div>
</div>
