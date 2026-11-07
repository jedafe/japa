<script lang="ts">
	import SeoHead from '$lib/seo/SeoHead.svelte';
	// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
	// Renders the claim-ownership form dynamically from the Form Builder
	// (Section 06) — including multi-step forms (is_multi_step + steps[]).
	// Flow: fill steps → review → POST /forms/{id}/submissions →
	// POST /claim-ownership. For gateway payments the submission is created
	// as pending_payment and the browser goes to hosted checkout; the
	// backend passes our return_url through so the provider lands the user
	// back here on ?paid=1, where we resume creating the claim once the
	// payment webhook flips the submission to 'new'.
	// Design language mirrors /verification-request: branded side panel +
	// numbered stepper + review step + pill actions — with a claim flavor
	// (key/handshake iconography instead of the badge-document theme).
	import { browser } from '$app/environment';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { invalidateAll } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { safeSameOriginHref } from '$lib/utils/safeUrl';
	import {
		ShieldQuestion,
		Loader2,
		Send,
		CreditCard,
		Wallet,
		FileCheck,
		Upload,
		Check,
		ChevronLeft,
		ChevronRight,
		KeyRound,
		Building2,
		Handshake,
		ScrollText,
		Clock,
		CircleAlert,
		X
	} from 'lucide-svelte';
	import type { FormField, FormSubmissionResponse, MediaAsset } from '$lib/types';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import AppealButton from '$lib/components/appeals/AppealButton.svelte';
	import MediaLibraryModal from '$lib/components/media/MediaLibraryModal.svelte';
	import { Modal } from '$lib/components/ui';

	let { data } = $props();

	const org = $derived(data.org);
	const form = $derived(data.form);
	const claims = $derived(data.claims ?? []);
	const credits = $derived(data.credits ?? 0);
	const blocked = $derived(data.blocked ?? null);

	let withdrawing = $state('');
	let claimsError = $state('');
	let withdrawTarget = $state<string | null>(null);
	let values = $state<Record<string, unknown>>({});
	let fieldErrors = $state<Record<string, string>>({});
	let mediaField = $state<{ id: string; type: string } | null>(null);
	let paymentMethod = $state<'credit' | 'paystack' | 'stripe'>('credit');
	let loading = $state(false);
	let errorKey = $state('');
	let phase = $state<'form' | 'redirecting' | 'creating_claim' | 'processing' | 'done'>('form');
	let submissionId = $state('');
	let step = $state(0);
	let resumed = $state(false);

	const sortedFields = $derived(form ? [...form.fields].sort((a, b) => a.sort_order - b.sort_order) : []);
	const feeCredits = $derived(form?.submission_fee_credits ?? 0);
	const isPaid = $derived(feeCredits > 0);
	const creditsShort = $derived(isPaid && credits < feeCredits);
	const pageTitle = $derived(t('claim_ownership.title', 'Claim ownership — {site}'));
	const pageDescription = $derived(
		t('claim_ownership.description', 'Submit evidence to claim this organization.')
	);

	// Wizard steps: a "Claim" step (entity context), one step per Form
	// Builder step (multi-step forms) or a single Details step for flat
	// forms, then a terminal Review step with payment.
	const formSteps = $derived(
		form?.is_multi_step && form.steps?.length
			? [...form.steps]
					.sort((a, b) => a.position - b.position)
					.map((s) => ({
						id: s.id,
						title: s.title,
						fields: [...s.fields].sort((a, b) => a.sort_order - b.sort_order)
					}))
					.filter((s) => s.fields.length > 0)
			: [
					{
						id: 'details',
						title: form?.name || t('claim_ownership.step_details', 'Details'),
						fields: sortedFields
					}
				]
	);
	const wizardSteps = $derived([
		{ id: 'claim', title: t('claim_ownership.step_claim', 'Claim'), fields: [] as FormField[] },
		...formSteps,
		{ id: 'review', title: t('claim_ownership.step_review', 'Review'), fields: [] as FormField[] }
	]);
	const totalSteps = $derived(wizardSteps.length);
	const reviewIndex = $derived(totalSteps - 1);
	const allFields = $derived(wizardSteps.flatMap((s) => s.fields));

	const inputBase =
		'w-full rounded-[var(--radius)] border bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]';
	const inputClass = inputBase + ' border-[var(--color-border)]';

	function fieldInputClass(id: string): string {
		return fieldErrors[id] ? inputBase + ' border-[var(--color-danger)]' : inputClass;
	}

	const PENDING_KEY = 'jc:claim_pending_submission';
	const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

	// Pending-submission marker: restored on ANY visit to this org's claim
	// form — not only the ?paid=1 return — so a claimant who abandoned or
	// lost the checkout round-trip links their existing submission on
	// submit instead of paying for a second one. The auto-resume itself
	// still only fires on the ?paid=1 provider redirect, where the payment
	// webhook flipping the submission pending_payment → new can lag the
	// redirect by a few seconds (hence the retry loop).
	$effect(() => {
		if (!browser || resumed || !org) return;
		resumed = true;
		const raw = sessionStorage.getItem(PENDING_KEY);
		if (raw) {
			try {
				const m = JSON.parse(raw) as { submission_id?: string; entity?: string };
				if (m.submission_id && (!m.entity || m.entity === `org:${org.slug}`)) {
					submissionId = m.submission_id;
				}
			} catch {
				// malformed marker — nothing to restore
			}
		}
		if (!data.resumePayment) return;
		if (submissionId) {
			phase = 'creating_claim';
			void resumeCreateClaim();
		} else {
			// Back from checkout with no marker to link — say so instead of
			// showing "confirming your payment" forever.
			errorKey = 'claim_ownership.errors.payment_unmatched';
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
			// 'validation_error'/'invalid_status_transition' cover the
			// webhook-not-yet-processed window (submission still
			// pending_payment): keep waiting.
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

	// Withdraw a pending claim via DELETE /claim-ownership/{id} (claimant-only),
	// then invalidate so the list reflects the server state (Rule 16).
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

	const MEDIA_TYPES = new Set(['media', 'image_single', 'image_multi']);

	function mediaItems(id: string): { id: string; url?: string; alt?: string }[] {
		const v = values[id];
		return Array.isArray(v) ? v : [];
	}

	function mediaSingle(id: string): { id: string; url?: string; alt?: string } | null {
		const v = values[id];
		return v && typeof v === 'object' && !Array.isArray(v) ? (v as { id: string; url?: string; alt?: string }) : null;
	}

	function isValueEmpty(v: unknown): boolean {
		if (v == null) return true;
		if (typeof v === 'string') return v.trim() === '';
		if (Array.isArray(v)) return v.length === 0;
		return false;
	}

	function fieldError(f: FormField): string {
		const v = values[f.id];
		if (f.required && isValueEmpty(v)) {
			return t('claim_ownership.errors.field_required_inline', 'This field is required');
		}
		if (!isValueEmpty(v) && typeof v === 'string') {
			if (f.type === 'email' && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v.trim())) {
				return t('claim_ownership.errors.field_email', 'Enter a valid email address');
			}
			if (f.type === 'url') {
				try {
					new URL(v.trim());
				} catch {
					return t('claim_ownership.errors.field_url', 'Enter a valid URL');
				}
			}
		}
		return '';
	}

	function validateStep(i: number): boolean {
		const s = wizardSteps[i];
		fieldErrors = {};
		let firstBad: string | null = null;
		for (const f of s.fields) {
			const err = fieldError(f);
			if (err) {
				fieldErrors[f.id] = err;
				firstBad ??= `field-${f.id}`;
			}
		}
		const focusId = firstBad;
		if (focusId) {
			requestAnimationFrame(() => document.getElementById(focusId)?.focus());
			return false;
		}
		return true;
	}

	function discardPendingSubmission() {
		sessionStorage.removeItem(PENDING_KEY);
		submissionId = '';
		errorKey = '';
	}

	let wizardCard = $state<HTMLElement | null>(null);

	function scrollToWizard() {
		const reduce =
			typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
		requestAnimationFrame(() =>
			wizardCard?.scrollIntoView({ block: 'start', behavior: reduce ? 'auto' : 'smooth' })
		);
	}

	function next() {
		errorKey = '';
		if (!validateStep(step)) return;
		if (step < reviewIndex) {
			step++;
			scrollToWizard();
		}
	}

	function back() {
		errorKey = '';
		if (step > 0) {
			step--;
			scrollToWizard();
		}
	}

	function onMediaSelect(asset: MediaAsset) {
		if (!mediaField) return;
		const item = { id: asset.id, url: asset.url, alt: asset.alt_text ?? undefined };
		if (mediaField.type === 'image_single') {
			values = { ...values, [mediaField.id]: item };
		} else {
			values = { ...values, [mediaField.id]: [...mediaItems(mediaField.id), item] };
		}
		mediaField = null;
	}

	function displayValue(f: FormField): string {
		const v = values[f.id];
		if (MEDIA_TYPES.has(f.type)) {
			const n = f.type === 'image_single' ? (mediaSingle(f.id) ? 1 : 0) : mediaItems(f.id).length;
			return n ? t('claim_ownership.files_attached', '{n} file(s) attached', { n }) : '—';
		}
		if (f.type === 'checkbox') {
			return v === 'true' ? t('claim_ownership.yes', 'Yes') : t('claim_ownership.no', 'No');
		}
		const s = String(v ?? '').trim();
		return s || '—';
	}

	async function submit() {
		errorKey = '';
		if (!form || !org || !validateStep(step)) return;
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
			// Values are keyed by field id in memory (admin-added duplicate
			// labels can't collide); the API contract is label→value — the
			// keys reviewers see — so translate once here.
			const fieldValues: Record<string, unknown> = {};
			for (const f of allFields) fieldValues[f.label] = values[f.id];
			const body: Record<string, unknown> = { field_values: fieldValues };
			if (isPaid) {
				body.payment_method = paymentMethod;
				// §06 return_url: the hosted checkout sends the user back here
				// with ?paid=1 so resumeCreateClaim can link the paid
				// submission to the claim.
				const ret = safeSameOriginHref(
					`${page.url.origin}/claim-ownership?org=${org.slug}&paid=1`
				);
				if (ret) body.return_url = ret;
				// §06 link intent: the payment webhook auto-creates the claim
				// when payment confirms — covers the user never returning.
				body.link = { kind: 'org_claim', organization_id: org.id };
			}

			const res = await api.post<FormSubmissionResponse>(`/forms/${form.id}/submissions`, body);
			if (!res.data) {
				errorKey = res.error?.code === 'insufficient_credits'
					? 'claim_ownership.errors.insufficient_credits'
					: res.error?.code === 'payment_intent_failed'
						? 'claim_ownership.errors.payment_failed'
						: res.error?.code === 'invalid_field_values'
							? 'claim_ownership.errors.submit_invalid'
							: 'claim_ownership.errors.submit_failed';
				return;
			}
			if (res.data.status === 'pending_payment' && res.data.checkout_url) {
				// Stash what the resume needs — the checkout round-trip loses
				// this page's in-memory state.
				sessionStorage.setItem(
					PENDING_KEY,
					JSON.stringify({ submission_id: res.data.id, entity: `org:${org.slug}` })
				);
				submissionId = res.data.id;
				phase = 'redirecting';
				window.location.href = res.data.checkout_url;
				return;
			}
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
				const code = res.error?.code ?? '';
				const msg = res.error?.message ?? '';
				errorKey = code === 'already_exists'
					? 'claim_ownership.errors.pending_exists'
					: (code === 'invalid_status_transition' || code === 'validation_error') && /payment/i.test(msg)
						? 'claim_ownership.errors.payment_not_completed'
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

<SeoHead title={pageTitle} description={pageDescription} ogTitle={pageTitle} robots="`noindex, nofollow`" />

<div class="mx-auto w-full max-w-full">
	<div class="border-b border-[var(--color-border)] px-4 py-3" style="margin-inline: max(1rem, calc((100% - 48rem)/2))">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<KeyRound size={20} />
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
									<a href="/org/{claim.org_slug}" class="block truncate text-sm font-medium text-[var(--color-text)] hover:text-[var(--color-primary-text)]">
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
	<div class="mx-auto w-full max-w-4xl p-4 sm:p-6">
		{#if blocked === 'pending_exists'}
			<EmptyState
				icon={Clock}
				title={t('claim_ownership.pending_exists_title', 'Claim already pending')}
				body={t('claim_ownership.pending_exists_body', 'You already have a pending claim for this organization. You can track or withdraw it from My claims.')}
			/>
			<p class="mt-3 text-center">
				<a href="/claim-ownership" class="text-sm text-[var(--color-primary-text)] hover:underline">{t('claim_ownership.my_claims_link', 'My claims')}</a>
			</p>
		{:else if blocked === 'no_permission'}
			<EmptyState
				icon={ShieldQuestion}
				title={t('claim_ownership.no_permission_title', 'Claims unavailable')}
				body={t('claim_ownership.no_permission_body', 'Your account cannot submit ownership claims.')}
			/>
		{:else if phase === 'processing'}
			<div class="flex flex-col items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center">
				<Clock size={28} class="text-[var(--color-primary-text)]" />
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
			<div class="flex flex-col items-center gap-3 rounded-[var(--radius-card)] border border-green-500/30 bg-green-500/10 p-8 text-center">
				<div class="grid h-14 w-14 place-items-center rounded-full bg-[var(--color-success)]">
					<Check size={28} class="text-white" />
				</div>
				<p class="max-w-lg text-lg font-medium text-green-600">{t('claim_ownership.claim_created')}</p>
				<div class="mt-2 flex items-center gap-4">
					<a href="/org/{org.slug}" class="rounded-full border border-[var(--color-border)] px-5 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
						{t('claim_ownership.back', 'Back to organization')}
					</a>
					<a href="/claim-ownership" class="text-sm font-medium text-[var(--color-primary-text)] hover:underline">
						{t('claim_ownership.my_claims_link', 'My claims')}
					</a>
				</div>
			</div>
		{:else if phase === 'redirecting' || phase === 'creating_claim'}
			<div class="flex flex-col items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center">
				<Loader2 size={24} class="animate-spin text-[var(--color-primary-text)]" />
				<p class="text-sm text-[var(--color-text)]">
					{phase === 'redirecting'
						? t('claim_ownership.redirecting_payment', 'Redirecting to payment provider…')
						: t('claim_ownership.creating_claim')}
				</p>
			</div>
		{:else if !form}
			<EmptyState
				icon={ShieldQuestion}
				title={t('claim_ownership.form_unavailable')}
				body={t('claim_ownership.form_unavailable_body')}
			/>
		{:else}
			<!-- Wizard card — same visual language as /verification-request:
			     brand panel + numbered stepper + pill actions, claim flavor. -->
			<div bind:this={wizardCard} class="grid scroll-mt-4 grid-cols-1 overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl lg:grid-cols-[280px_1fr]">
				<!-- Brand panel — claim-themed illustration + privacy note. -->
				<aside aria-hidden="true" class="co-side relative hidden flex-col gap-4 overflow-hidden p-7 lg:flex">
					<h2 class="text-lg leading-snug font-bold text-[var(--color-text)]">
						{t('claim_ownership.side_title', 'Prove it\'s yours')}
					</h2>
					<p class="text-sm leading-relaxed text-[var(--color-text-subtle)]">
						{t('claim_ownership.side_subtitle', 'Show the link between you and this organization so our team can hand over the keys.')}
					</p>

					<!-- CSS illustration: key focal + building/handshake chips + dashed route. -->
					<div class="relative mt-2 min-h-44 flex-1">
						<svg class="absolute inset-0 h-full w-full" viewBox="0 0 240 200" fill="none" aria-hidden="true">
							<path d="M30 40 C 60 110, 140 95, 210 160" stroke="var(--color-primary)" stroke-width="1.5" stroke-dasharray="5 6" stroke-linecap="round" opacity="0.45" />
							<circle cx="30" cy="40" r="4.5" fill="var(--color-primary)" opacity="0.5" />
							<circle cx="210" cy="160" r="4.5" fill="var(--color-primary)" opacity="0.5" />
						</svg>
						<div class="co-focal">
							<KeyRound size={56} strokeWidth={1.1} class="text-[var(--color-primary-text)]" />
						</div>
						<div class="co-chip" style="top: 8%; inset-inline-start: 10%;"><Building2 size={17} class="text-[var(--color-primary-text)]" /></div>
						<div class="co-chip" style="top: 30%; inset-inline-end: 8%;"><ScrollText size={17} class="text-[var(--color-primary-text)]" /></div>
						<div class="co-chip" style="bottom: 14%; inset-inline-start: 12%;"><Handshake size={17} class="text-[var(--color-primary-text)]" /></div>
					</div>

					<div class="mt-auto flex items-start gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]/80 px-4 py-3 backdrop-blur-sm">
						<ShieldQuestion size={18} class="mt-0.5 shrink-0 text-[var(--color-primary-text)]" />
						<p class="text-xs leading-relaxed text-[var(--color-text-subtle)]">
							<span class="block font-semibold text-[var(--color-text)]">{t('claim_ownership.privacy_title', 'Claims are reviewed, not published.')}</span>
							{t('claim_ownership.privacy_body', 'Your evidence is only visible to our review team while they decide.')}
						</p>
					</div>
				</aside>

				<div class="flex min-w-0 flex-col p-5 sm:p-8">
					<!-- Numbered stepper — completed steps are clickable. -->
					<nav aria-label={t('claim_ownership.steps_label', 'Claim steps')} class="mb-7 flex items-start">
						{#each wizardSteps as s, i (s.id)}
							{@const n = i + 1}
							<div class="flex flex-1 flex-col items-center gap-1.5">
								<div class="flex w-full items-center">
									{#if i > 0}<span class="h-px flex-1 {n <= step + 1 ? 'bg-[var(--color-primary)]' : 'bg-[var(--color-border)]'}"></span>{:else}<span class="flex-1" aria-hidden="true"></span>{/if}
									<button
										type="button"
										disabled={i >= step}
										onclick={() => { errorKey = ''; step = i; scrollToWizard(); }}
										class="grid h-9 w-9 shrink-0 place-items-center rounded-full text-sm font-semibold transition-colors disabled:cursor-default {i === step
											? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
											: i < step
												? 'bg-[var(--color-primary)]/15 text-[var(--color-primary-text)] hover:bg-[var(--color-primary)]/25'
												: 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]'}"
										aria-current={i === step ? 'step' : undefined}
										aria-label={s.title}
									>
										{#if i < step}<Check size={15} />{:else}{n}{/if}
									</button>
									{#if i < totalSteps - 1}<span class="h-px flex-1 {n <= step ? 'bg-[var(--color-primary)]' : 'bg-[var(--color-border)]'}"></span>{:else}<span class="flex-1" aria-hidden="true"></span>{/if}
								</div>
								<span class="hidden text-center text-[11px] font-medium sm:block {i === step ? 'text-[var(--color-primary-text)]' : 'text-[var(--color-muted)]'}">{s.title}</span>
							</div>
						{/each}
					</nav>

					<!-- Step 0: Claim — the org being claimed + what happens next. -->
					{#if step === 0}
						<h2 class="text-xl font-bold text-[var(--color-text)]">{t('claim_ownership.step1_title', 'What are you claiming?')}</h2>
						<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
							{form.description || t('claim_ownership.subtitle', 'Submit evidence to claim this organization.')}
						</p>

						<div class="mt-5 flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4">
							<span class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface)] text-[var(--color-primary-text)]">
								<Building2 size={18} />
							</span>
							<div class="min-w-0">
								<p class="truncate text-sm font-medium text-[var(--color-text)]">{org.name}</p>
								<p class="truncate text-xs text-[var(--color-muted)]">/{org.slug}</p>
							</div>
						</div>
					{:else if step === reviewIndex}
						<!-- Final step: review answers + payment + submit. -->
						<h2 class="text-xl font-bold text-[var(--color-text)]">{t('claim_ownership.review_title', 'Review and submit')}</h2>
						<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{t('claim_ownership.review_subtitle', 'Check your answers before sending the claim for review.')}</p>

						<dl class="mt-5 divide-y divide-[var(--color-border)] rounded-[var(--radius)] border border-[var(--color-border)]">
							<div class="flex items-start justify-between gap-4 px-4 py-2.5">
								<dt class="text-xs font-medium text-[var(--color-muted)]">{t('claim_ownership.summary_entity', 'Claiming')}</dt>
								<dd class="text-end text-sm text-[var(--color-text)]">{org.name}</dd>
							</div>
							{#each allFields as f (f.id)}
								<div class="flex items-start justify-between gap-4 px-4 py-2.5">
									<dt class="max-w-[45%] text-xs font-medium text-[var(--color-muted)]">{f.label}</dt>
									<dd class="min-w-0 text-end text-sm break-words text-[var(--color-text)]">{displayValue(f)}</dd>
								</div>
							{/each}
						</dl>

						{#if isPaid}
							<div class="mt-4 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4">
								<p class="mb-1 text-sm font-medium text-[var(--color-text)]">{t('claim_ownership.payment_required', 'Submission fee')}</p>
								<p class="mb-3 text-xs text-[var(--color-muted)]">{t('claim_ownership.payment_hint', 'This form requires a submission fee of {credits} credits.').replace('{credits}', String(feeCredits))}</p>
								<div class="flex flex-col gap-2" role="radiogroup" aria-label={t('claim_ownership.payment_required', 'Submission fee')}>
									<label class="flex items-center gap-2 rounded-[var(--radius)] border px-3 py-2 text-sm transition-colors {paymentMethod === 'credit' ? 'border-[var(--color-primary)] bg-[var(--color-surface)]' : 'border-[var(--color-border)]'} {creditsShort ? 'opacity-60' : 'cursor-pointer'}">
										<input type="radio" name="payment_method" value="credit" checked={paymentMethod === 'credit'} disabled={creditsShort} onchange={() => (paymentMethod = 'credit')} class="h-4 w-4 accent-[var(--color-primary)]" />
										<Wallet size={16} class="shrink-0 text-[var(--color-primary-text)]" />
										<span class="flex-1 text-[var(--color-text)]">{t('claim_ownership.pay_with_credits', 'Pay with credits ({credits})').replace('{credits}', String(feeCredits))}</span>
										<span class="text-xs {creditsShort ? 'text-[var(--color-danger)]' : 'text-[var(--color-muted)]'}">
											{t('claim_ownership.pay_balance', 'Balance: {credits}').replace('{credits}', String(credits))}
										</span>
									</label>
									<label class="flex cursor-pointer items-center gap-2 rounded-[var(--radius)] border px-3 py-2 text-sm transition-colors {paymentMethod === 'paystack' ? 'border-[var(--color-primary)] bg-[var(--color-surface)]' : 'border-[var(--color-border)]'}">
										<input type="radio" name="payment_method" value="paystack" checked={paymentMethod === 'paystack'} onchange={() => (paymentMethod = 'paystack')} class="h-4 w-4 accent-[var(--color-primary)]" />
										<CreditCard size={16} class="shrink-0 text-[var(--color-primary-text)]" />
										<span class="text-[var(--color-text)]">{t('claim_ownership.pay_with_gateway', 'Pay with card (Paystack)')}</span>
									</label>
									<label class="flex cursor-pointer items-center gap-2 rounded-[var(--radius)] border px-3 py-2 text-sm transition-colors {paymentMethod === 'stripe' ? 'border-[var(--color-primary)] bg-[var(--color-surface)]' : 'border-[var(--color-border)]'}">
										<input type="radio" name="payment_method" value="stripe" checked={paymentMethod === 'stripe'} onchange={() => (paymentMethod = 'stripe')} class="h-4 w-4 accent-[var(--color-primary)]" />
										<CreditCard size={16} class="shrink-0 text-[var(--color-primary-text)]" />
										<span class="text-[var(--color-text)]">{t('claim_ownership.pay_with_stripe', 'Pay with card (Stripe)')}</span>
									</label>
								</div>
								{#if creditsShort}
									<p class="mt-2 flex items-center gap-1.5 text-xs text-[var(--color-danger)]">
										<CircleAlert size={13} />
										{t('claim_ownership.insufficient_balance', 'Your credit balance is too low — choose card payment or top up your wallet.')}
									</p>
								{/if}
							</div>
						{/if}
					{:else}
						<!-- Form Builder step fields. -->
						{@const s = wizardSteps[step]}
						<h2 class="text-xl font-bold text-[var(--color-text)]">{s.title}</h2>

						<div class="mt-5 space-y-4">
							{#each s.fields as field (field.id)}
								<div class="flex flex-col gap-1.5">
									{#if field.type !== 'checkbox'}
										<label for="field-{field.id}" class="text-sm font-medium text-[var(--color-text)]">
											{field.label}
											{#if field.required}<span class="text-red-500" aria-hidden="true">*</span>{/if}
										</label>
									{/if}
									{#if field.type === 'textarea' || field.type === 'long_text'}
										<textarea id="field-{field.id}" rows="4" value={String(values[field.id] ?? '')} oninput={(e) => (values = { ...values, [field.id]: e.currentTarget.value })} aria-required={field.required} aria-invalid={Boolean(fieldErrors[field.id])} class={fieldInputClass(field.id)}></textarea>
									{:else if field.type === 'select' && field.options && field.options.length > 0}
										<AutocompleteSelect
											id="field-{field.id}"
											value={String(values[field.id] ?? '')}
											options={[{ value: '', label: t('claim_ownership.select_placeholder', 'Select an option') }, ...field.options.map((opt) => ({ value: opt, label: opt }))]}
											onValueChange={(v) => (values = { ...values, [field.id]: v })}
										/>
									{:else if MEDIA_TYPES.has(field.type)}
										{@const single = mediaSingle(field.id)}
										{@const items = mediaItems(field.id)}
										<div class="space-y-2" id="field-{field.id}" tabindex="-1">
											{#if single}
												<div class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm">
													{#if single.url}<img src={single.url} alt={single.alt ?? ''} class="h-10 w-10 rounded object-cover" />{/if}
													<span class="min-w-0 flex-1 truncate text-[var(--color-muted)]">{single.url ?? single.id}</span>
													<button type="button" onclick={() => (values = { ...values, [field.id]: '' })} class="text-[var(--color-danger)]" aria-label={t('claim_ownership.media_remove', 'Remove')}>
														<X size={14} />
													</button>
												</div>
											{/if}
											{#each items as item, i (item.id)}
												<div class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm">
													{#if item.url}<img src={item.url} alt={item.alt ?? ''} class="h-10 w-10 rounded object-cover" />{/if}
													<span class="min-w-0 flex-1 truncate text-[var(--color-muted)]">{item.url ?? item.id}</span>
													<button type="button" onclick={() => (values = { ...values, [field.id]: items.filter((_, j) => j !== i) })} class="text-[var(--color-danger)]" aria-label={t('claim_ownership.media_remove', 'Remove')}>
														<X size={14} />
													</button>
												</div>
											{/each}
											<button
												type="button"
												onclick={() => (mediaField = { id: field.id, type: field.type })}
												class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-dashed border-[var(--color-border)] px-3 py-2 text-sm text-[var(--color-muted)] transition-colors hover:border-[var(--color-primary)] hover:text-[var(--color-primary-text)]"
											>
												<Upload size={14} />
												{field.type === 'image_single' && single ? t('claim_ownership.media_replace', 'Replace image') : field.type === 'image_single' ? t('claim_ownership.media_pick_image', 'Upload image') : t('claim_ownership.media_add', 'Upload file')}
											</button>
										</div>
									{:else if field.type === 'checkbox'}
										<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
											<input type="checkbox" id="field-{field.id}" checked={values[field.id] === 'true'} onchange={(e) => (values = { ...values, [field.id]: e.currentTarget.checked ? 'true' : '' })} aria-required={field.required} aria-invalid={Boolean(fieldErrors[field.id])} class="h-4 w-4 rounded border-[var(--color-border)] accent-[var(--color-primary)]" />
											{field.label}
											{#if field.required}<span class="text-red-500" aria-hidden="true">*</span>{/if}
										</label>
									{:else}
										<input id="field-{field.id}" type={field.type === 'email' ? 'email' : field.type === 'number' ? 'number' : field.type === 'url' ? 'url' : 'text'} value={String(values[field.id] ?? '')} oninput={(e) => (values = { ...values, [field.id]: e.currentTarget.value })} aria-required={field.required} aria-invalid={Boolean(fieldErrors[field.id])} class={fieldInputClass(field.id)} />
									{/if}
									{#if fieldErrors[field.id]}
										<p class="text-xs text-[var(--color-danger)]" role="alert">{fieldErrors[field.id]}</p>
									{/if}
								</div>
							{/each}
						</div>
					{/if}

					{#if errorKey}
						<p class="mt-4 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">{t(errorKey)}</p>
						{#if submissionId && errorKey === 'claim_ownership.errors.payment_not_completed'}
							<button type="button" onclick={discardPendingSubmission} class="mt-2 text-sm font-medium text-[var(--color-primary-text)] hover:underline">
								{t('claim_ownership.start_over', 'Discard it and start a new submission')}
							</button>
						{/if}
					{/if}

					<!-- Actions — pill buttons matching verification-request. -->
					<div class="mt-8 flex items-center justify-between gap-3">
						{#if step === 0}
							<a
								href="/org/{org.slug}"
								class="flex items-center gap-1.5 rounded-full border border-[var(--color-border)] px-5 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								{t('claim_ownership.back', 'Back to organization')}
							</a>
						{:else}
							<button
								type="button"
								onclick={back}
								disabled={loading}
								class="flex items-center gap-1.5 rounded-full border border-[var(--color-border)] px-5 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
							>
								<ChevronLeft class="rtl:-scale-x-100" size={16} />
								{t('claim_ownership.back_step', 'Back')}
							</button>
						{/if}
						<button
							type="button"
							onclick={step === reviewIndex ? submit : next}
							disabled={loading}
							class="flex items-center gap-2 rounded-full bg-[var(--color-primary)] px-6 py-2.5 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
						>
							{#if loading}
								<Loader2 size={16} class="animate-spin" />
							{:else if step === reviewIndex}
								<Send class="rtl:-scale-x-100" size={16} />
							{/if}
							{step === reviewIndex
								? (loading ? t('claim_ownership.creating_claim') : (form.submit_label || t('claim_ownership.submit', 'Submit claim')))
								: t('claim_ownership.continue', 'Continue')}
							{#if step < reviewIndex}<ChevronRight class="rtl:-scale-x-100" size={16} />{/if}
						</button>
					</div>
				</div>
			</div>
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

<style>
	.co-side {
		background:
			radial-gradient(ellipse 80% 55% at 65% 40%, color-mix(in srgb, var(--color-secondary) 14%, transparent), transparent),
			color-mix(in srgb, var(--color-secondary) 7%, var(--color-surface));
	}
	.co-focal {
		position: absolute;
		top: 50%;
		inset-inline-start: 50%;
		translate: -50% -50%;
		display: grid;
		place-items: center;
		width: 7rem;
		height: 7rem;
		border-radius: 9999px;
		background: var(--color-surface);
		box-shadow: 0 10px 28px color-mix(in srgb, var(--color-secondary) 18%, transparent);
	}
	.co-chip {
		position: absolute;
		display: grid;
		place-items: center;
		width: 2.4rem;
		height: 2.4rem;
		border-radius: 0.75rem;
		background: var(--color-surface);
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
	}
	@media (prefers-reduced-motion: no-preference) {
		.co-chip {
			animation: co-float 4s ease-in-out infinite;
		}
		.co-chip:nth-of-type(3) { animation-delay: 0.7s; }
		.co-chip:nth-of-type(4) { animation-delay: 1.4s; }
	}
	@keyframes co-float {
		0%, 100% { translate: 0 0; }
		50% { translate: 0 -6px; }
	}
</style>
