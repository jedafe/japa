<script lang="ts">
	// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
	// Renders the claim-ownership form dynamically from the Form Builder (Section 06).
	// No hardcoded fields — all fields come from GET /forms/claim-ownership/embed.
	// For paid forms, shows payment method choice (credits or card gateway).
	// The flow is: submit form → create claim with form_submission_id.
	import { browser } from '$app/environment';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { invalidateAll } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { safeSameOriginHref } from '$lib/utils/safeUrl';
	import { ShieldQuestion, Loader2, Send, CreditCard, Wallet, FileCheck, Clock, X } from 'lucide-svelte';
	import type { FormField, FormSubmissionResponse } from '$lib/types';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import AppealButton from '$lib/components/appeals/AppealButton.svelte';
	import { Modal } from '$lib/components/ui';

	let { data } = $props();

	const org = $derived(data.org);
	const form = $derived(data.form);
	const claims = $derived(data.claims ?? []);
	const credits = $derived(data.credits ?? 0);
	const blocked = $derived(data.blocked ?? false);
	let withdrawing = $state('');
	let claimsError = $state('');
	let withdrawTarget = $state<string | null>(null);
	let resumed = $state(false);

	// Withdraw a pending claim via DELETE /claim-ownership/{id} (claimant-only),
	// then invalidate so the list reflects the server state (Rule 16).
	// The busy flag is cleared explicitly on every exit path — never in a
	// finally that could reset Svelte 5 $state mid-flush.
	async function withdrawClaim(id: string) {
		if (withdrawing) return;
		withdrawing = id;
		claimsError = '';
		const res = await api.delete(`/claim-ownership/${id}`);
		if (res.error) {
			claimsError = res.error.message;
			withdrawing = '';
			return;
		}
		await invalidateAll();
		withdrawing = '';
	}

	// Field values keyed by field label (matches the backend's field_values contract).
	let values = $state<Record<string, string>>({});
	let paymentMethod = $state<'credit' | 'paystack' | 'stripe'>('credit');
	let loading = $state(false);
	let errorKey = $state('');
	let phase = $state<'form' | 'redirecting' | 'pending_payment' | 'creating_claim' | 'processing' | 'done'>('form');
	let submissionId = $state('');
	let checkoutUrl = $state('');

	const sortedFields = $derived(
		form ? [...form.fields].sort((a, b) => a.sort_order - b.sort_order) : []
	);

	const feeCredits = $derived(form?.submission_fee_credits ?? 0);
	const isPaid = $derived(feeCredits > 0);
	const creditsShort = $derived(isPaid && credits < feeCredits);

	const PENDING_KEY = 'jc:claim_pending_submission';
	const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

	// Resume-after-checkout: the provider redirected back to ?paid=1. The
	// webhook flips the submission pending_payment → new (and the link
	// intent auto-creates the claim), which can lag the redirect — retry.
	$effect(() => {
		if (!browser || resumed || !data.resumePayment || !org) return;
		resumed = true;
		const raw = sessionStorage.getItem(PENDING_KEY);
		if (raw) {
			try {
				const m = JSON.parse(raw) as { submission_id?: string; entity?: string };
				if (m.submission_id && (!m.entity || m.entity === `org:${org.slug}`)) {
					submissionId = m.submission_id;
				}
			} catch {
				// malformed marker — fall through to the processing state
			}
		}
		if (submissionId) {
			phase = 'creating_claim';
			void resumeCreateClaim();
		} else {
			phase = 'processing';
		}
	});

	async function resumeCreateClaim() {
		for (let i = 0; i < 8; i++) {
			const res = await api.post('/claim-ownership', {
				organization_id: org!.id,
				form_submission_id: submissionId
			});
			if (res.data) {
				sessionStorage.removeItem(PENDING_KEY);
				phase = 'done';
				await invalidateAll();
				return;
			}
			const code = res.error?.code ?? '';
			if (code === 'already_exists') {
				sessionStorage.removeItem(PENDING_KEY);
				phase = 'done';
				await invalidateAll();
				return;
			}
			// 'validation_error' covers the webhook-not-yet-processed window
			// (submission still pending_payment): keep waiting.
			if (code === 'invalid_status_transition' || code === 'validation_error') {
				await sleep(2500);
				continue;
			}
			errorKey = 'claim_ownership.errors.claim_failed';
			phase = 'processing';
			return;
		}
		phase = 'processing';
	}

	const pageTitle = $derived(t('claim_ownership.title', 'Claim ownership — {site}'));
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
		if (!form || !org) return;
		if (!validate()) return;

		if (isPaid && paymentMethod === 'credit' && creditsShort) {
			errorKey = 'claim_ownership.errors.insufficient_credits';
			return;
		}
		loading = true;
		try {
			// A previous attempt may have already created (and possibly paid
			// for) a submission — retry the claim on it instead of paying again.
			if (submissionId) {
				await createClaim();
				return;
			}
			// Step 1: Submit the form. For paid forms, include payment_method;
			// return_url brings the user back to ?paid=1 after hosted checkout,
			// and the link intent lets the payment webhook auto-create the
			// claim if the user never returns (§06).
			const body: Record<string, unknown> = { field_values: values };
			if (isPaid) {
				body.payment_method = paymentMethod;
				const ret = safeSameOriginHref(`${page.url.origin}/claim-ownership?org=${org.slug}&paid=1`);
				if (ret) body.return_url = ret;
				body.link = { kind: 'org_claim', organization_id: org.id };
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

			// For gateway payments, stash the resume marker and redirect to
			// the checkout URL — the return_url lands back on ?paid=1.
			if (res.data.status === 'pending_payment' && res.data.checkout_url) {
				submissionId = res.data.id;
				checkoutUrl = res.data.checkout_url;
				sessionStorage.setItem(PENDING_KEY, JSON.stringify({ submission_id: res.data.id, entity: `org:${org.slug}` }));
				phase = 'redirecting';
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
		if (!submissionId || !org) return;
		phase = 'creating_claim';
		loading = true;
		try {
			const res = await api.post('/claim-ownership', {
				organization_id: org.id,
				form_submission_id: submissionId
			});
			if (res.data) {
				phase = 'done';
				await invalidateAll();
			} else {
				errorKey = res.error?.code === 'already_exists'
					? 'claim_ownership.errors.pending_exists'
					: 'claim_ownership.errors.claim_failed';
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
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header -->
	<div class="border-b border-[var(--color-border)] px-4 py-3" style="margin-inline: max(1rem, calc((100% - 48rem)/2))">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<ShieldQuestion size={20} />
			{org ? t('claim_ownership.heading', 'Claim ownership') : t('claim_ownership.my_claims_title', 'My ownership claims')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{org
				? t('claim_ownership.subtitle', 'Submit evidence to claim this organization.')
				: t('claim_ownership.my_claims_subtitle', 'Track the status of your organization ownership claims.')}
		</p>
	</div>

	{#if !org}
		<!-- My claims — claimant-scoped list from GET /me/claim-ownership. -->
		<div class="mx-auto w-full max-w-3xl p-4">
			{#if claimsError}
				<p class="mb-3 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
					{claimsError}
				</p>
			{/if}
			{#if claims.length === 0}
				<EmptyState
					icon={FileCheck}
					title={t('claim_ownership.my_claims_empty', 'No ownership claims')}
					body={t('claim_ownership.my_claims_empty_body', 'Claims you submit from an unverified organization profile will appear here.')}
				/>
			{:else}
				<ul class="space-y-2">
					{#each claims as claim (claim.id)}
						<li class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<div class="flex items-start justify-between gap-3">
								<div class="min-w-0">
									<a href="/org/{claim.org_slug}" class="block truncate text-sm font-medium text-[var(--color-text)] hover:text-[var(--color-primary)]">
										{claim.org_name}
									</a>
									<p class="mt-0.5 text-xs text-[var(--color-muted)]">
										{t('claim_ownership.claim_filed', 'Filed {date}').replace('{date}', new Date(claim.created_at).toLocaleDateString())}
										{#if claim.reviewed_at}
											· {t('claim_ownership.claim_reviewed', 'Reviewed {date}').replace('{date}', new Date(claim.reviewed_at).toLocaleDateString())}
										{/if}
									</p>
									{#if claim.resolution_note}
										<p class="mt-1 text-xs text-[var(--color-text-subtle)]">{claim.resolution_note}</p>
									{/if}
								</div>
								<div class="flex shrink-0 items-center gap-2">
									<span class="rounded-full px-2.5 py-0.5 text-xs font-medium {claim.status === 'approved'
										? 'bg-[var(--color-success)]/10 text-[var(--color-success)]'
										: claim.status === 'rejected' || claim.status === 'withdrawn'
											? 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]'
											: 'bg-[var(--color-warning)]/10 text-[var(--color-warning)]'}">
										{t(`claim_ownership.status.${claim.status}`, claim.status)}
									</span>
									{#if claim.status === 'pending'}
										<button
											type="button"
											onclick={() => (withdrawTarget = claim.id)}
											class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-danger)] transition-colors hover:bg-[var(--color-surface-subtle)]"
										>
											<X size={12} />
											{t('claim_ownership.withdraw', 'Withdraw')}
										</button>
									{/if}
									{#if claim.status === 'rejected'}
										<!-- §22: a rejected claim is appealable; POST /appeals opens
											a moderation_appeal + linked staff ticket. -->
										<AppealButton entityType="org_claim" entityId={claim.id} class="!px-2.5 !py-1 text-xs" />
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

		{#if blocked === 'pending_exists'}
			<EmptyState
				icon={Clock}
				title={t('claim_ownership.pending_exists_title', 'Claim already pending')}
				body={t('claim_ownership.pending_exists_body', 'You already have a pending claim for this organization. You can track or withdraw it from My claims.')}
			/>
			<p class="mt-3 text-center">
				<a href="/claim-ownership" class="text-sm text-[var(--color-primary)] hover:underline">{t('claim_ownership.my_claims_link', 'My claims')}</a>
			</p>
		{:else if blocked === 'no_permission'}
			<EmptyState
				icon={ShieldQuestion}
				title={t('claim_ownership.no_permission_title', 'Claims unavailable')}
				body={t('claim_ownership.no_permission_body', 'Your account cannot submit ownership claims.')}
			/>
		{:else if phase === 'processing'}
			<div class="flex flex-col items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center">
				<Clock size={28} class="text-[var(--color-primary)]" />
				<p class="text-base font-medium text-[var(--color-text)]">{t('claim_ownership.processing_title', 'Confirming your payment')}</p>
				<p class="max-w-md text-sm text-[var(--color-text-subtle)]">{t('claim_ownership.processing_body', "We're still confirming your payment. Your claim will appear under My claims once it completes.")}</p>
				{#if errorKey}
					<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">{t(errorKey)}</p>
				{/if}
				<div class="mt-2 flex items-center gap-4">
					{#if submissionId}
						<button
							type="button"
							onclick={() => { errorKey = ''; phase = 'creating_claim'; void resumeCreateClaim(); }}
							class="rounded-full bg-[var(--color-primary)] px-5 py-2 text-sm font-semibold text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
						>
							{t('claim_ownership.retry', 'Try again')}
						</button>
					{/if}
					<a href="/claim-ownership" class="rounded-full border border-[var(--color-border)] px-5 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
						{t('claim_ownership.my_claims_link', 'My claims')}
					</a>
				</div>
			</div>
		{:else if phase === 'done'}
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
							<AutocompleteSelect
								id="field-{field.id}"
								value={values[field.label] ?? ''}
								options={[
									{ value: '', label: t('claim_ownership.select_placeholder', 'Select an option') },
									...field.options.map((opt) => ({ value: opt, label: opt }))
								]}
								onValueChange={(v) => (values = { ...values, [field.label]: v })}
							/>
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
							<label class="flex items-center gap-2 text-sm text-[var(--color-text)] {creditsShort ? 'opacity-60' : ''}">
								<input
									type="radio"
									name="payment_method"
									value="credit"
									checked={paymentMethod === 'credit'}
									disabled={creditsShort}
									onchange={() => (paymentMethod = 'credit')}
									class="h-4 w-4 accent-[var(--color-primary)]"
								/>
								<Wallet size={16} class="text-[var(--color-primary)]" />
								{t('claim_ownership.pay_with_credits', 'Pay with credits ({credits})').replace('{credits}', String(feeCredits))}
								<span class="ms-auto text-xs {creditsShort ? 'text-[var(--color-danger)]' : 'text-[var(--color-muted)]'}">
									{t('claim_ownership.pay_balance', 'Balance: {credits}').replace('{credits}', String(credits))}
								</span>
							</label>
							{#if creditsShort}
								<p class="text-xs text-[var(--color-danger)]">
									{t('claim_ownership.insufficient_balance', 'Your credit balance is too low — choose card payment or top up your wallet.')}
								</p>
							{/if}
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
					<span class="flex items-center gap-3">
						<a
							href="/org/{org.slug}"
							class="text-sm text-[var(--color-muted)] hover:underline"
						>
							← {t('claim_ownership.back', 'Back to organization')}
						</a>
						<a
							href="/claim-ownership"
							class="text-sm text-[var(--color-muted)] hover:underline"
						>
							{t('claim_ownership.my_claims_link', 'My claims')}
						</a>
					</span>
					<button
						type="submit"
						disabled={loading}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						{#if loading}
							<Loader2 size={16} class="animate-spin" />
						{:else}
							<Send class="rtl:-scale-x-100" size={16} />
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
	{/if}
</div>

<!-- Withdraw confirmation — shared Modal instead of native confirm(). -->
<Modal
	open={withdrawTarget !== null}
	onOpenChange={(v) => { if (!v) withdrawTarget = null; }}
	title={t('claim_ownership.withdraw_title', 'Withdraw claim?')}
	icon={ShieldQuestion}
	size="sm"
>
	{#snippet children(close)}
		<p class="mb-5 text-sm text-[var(--color-text-subtle)]">
			{t('claim_ownership.withdraw_body', 'This claim will be withdrawn and removed from the review queue.')}
		</p>
		<div class="flex justify-end gap-2">
			<button type="button" onclick={close} class="rounded-full border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
				{t('claim_ownership.cancel', 'Cancel')}
			</button>
			<button
				type="button"
				disabled={withdrawing !== ''}
				onclick={async () => { const id = withdrawTarget; close(); if (id) await withdrawClaim(id); }}
				class="inline-flex items-center gap-1.5 rounded-full bg-[var(--color-danger)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
			>
				{#if withdrawing}<Loader2 size={14} class="animate-spin" />{/if}
				{t('claim_ownership.withdraw', 'Withdraw')}
			</button>
		</div>
	{/snippet}
</Modal>
