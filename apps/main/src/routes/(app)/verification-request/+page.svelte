<script lang="ts">
	// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md (Verification Requests)
	// Renders the request form dynamically from the Form Builder (Section 06)
	// — including multi-step forms (is_multi_step + steps[]) — plus a
	// suggested badge-type select fed by GET /verification-requests/types.
	// Flow: fill steps → review → POST /forms/{id}/submissions →
	// POST /verification-requests. For gateway payments the submission is
	// created as pending_payment and the browser goes to hosted checkout;
	// the backend passes our return_url through so the provider lands the
	// user back here on ?paid=1, where we resume creating the request once
	// the payment webhook flips the submission to 'new'.
	// Design language mirrors /onboarding and JourneyOnboardingModal:
	// branded side panel + numbered stepper + review step + pill actions.
	import { browser } from '$app/environment';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { invalidateAll } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { safeSameOriginHref } from '$lib/utils/safeUrl';
	import {
		BadgeCheck,
		Loader2,
		Send,
		CreditCard,
		Wallet,
		FileCheck,
		Upload,
		Check,
		ChevronLeft,
		ChevronRight,
		ShieldCheck,
		Landmark,
		FileText,
		Fingerprint,
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
	const self = $derived(data.self);
	const form = $derived(data.form);
	const types = $derived(data.types ?? []);
	const requests = $derived(data.requests ?? []);
	const credits = $derived(data.credits ?? 0);
	const blocked = $derived(data.blocked ?? null);
	const isRequestMode = $derived(Boolean(org) || self);

	let withdrawing = $state('');
	let listError = $state('');
	let withdrawTarget = $state<string | null>(null);
	let values = $state<Record<string, unknown>>({});
	let fieldErrors = $state<Record<string, string>>({});
	let mediaField = $state<{ id: string; label: string; type: string } | null>(null);
	let requestedType = $state('');
	let paymentMethod = $state<'credit' | 'paystack' | 'stripe'>('credit');
	let loading = $state(false);
	let errorKey = $state('');
	let phase = $state<'form' | 'redirecting' | 'creating_request' | 'processing' | 'done'>('form');
	let submissionId = $state('');
	let step = $state(0);
	let resumed = $state(false);

	const sortedFields = $derived(form ? [...form.fields].sort((a, b) => a.sort_order - b.sort_order) : []);
	const feeCredits = $derived(form?.submission_fee_credits ?? 0);
	const isPaid = $derived(feeCredits > 0);
	const creditsShort = $derived(isPaid && credits < feeCredits);
	const pageTitle = $derived(t('verification_request.title', 'Request verification — {site}'));

	// Wizard steps: a "Request" step (entity + badge type), one step per
	// Form Builder step (multi-step forms) or a single Details step for
	// flat forms, then a terminal Review step with payment.
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
						title: form?.name || t('verification_request.step_details', 'Details'),
						fields: sortedFields
					}
				]
	);
	const wizardSteps = $derived([
		{ id: 'request', title: t('verification_request.step_request', 'Request'), fields: [] as FormField[] },
		...formSteps,
		{ id: 'review', title: t('verification_request.step_review', 'Review'), fields: [] as FormField[] }
	]);
	const totalSteps = $derived(wizardSteps.length);
	const reviewIndex = $derived(totalSteps - 1);
	const allFields = $derived(wizardSteps.flatMap((s) => s.fields));

	const inputBase =
		'w-full rounded-[var(--radius)] border bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]';
	const inputClass = inputBase + ' border-[var(--color-border)]';

	function fieldInputClass(id: string): string {
		return fieldErrors[id]
			? inputBase + ' border-[var(--color-danger)]'
			: inputClass;
	}

	const PENDING_KEY = 'jc:verification_pending_submission';

	const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

	// Resume-after-checkout: the provider redirected us back to ?paid=1.
	// The payment webhook flips the submission pending_payment → new, which
	// can lag the redirect by a few seconds, so we retry createRequest until
	// the submission becomes consumable.
	$effect(() => {
		if (!browser || resumed || !data.resumePayment || !isRequestMode) return;
		resumed = true;
		const raw = sessionStorage.getItem(PENDING_KEY);
		if (raw) {
			try {
				const m = JSON.parse(raw) as {
					submission_id?: string;
					requested_type?: string;
					entity?: string;
				};
				// Only resume when the marker belongs to this entity — a stale
				// marker from another org/self flow must not mis-link the request.
				const entity = org ? `org:${org.slug}` : 'self';
				if (m.submission_id && (!m.entity || m.entity === entity)) {
					submissionId = m.submission_id;
					if (m.requested_type) requestedType = m.requested_type;
				}
			} catch {
				// malformed marker — fall through to the processing state
			}
		}
		if (submissionId) {
			phase = 'creating_request';
			void resumeCreateRequest();
		} else {
			phase = 'processing';
		}
	});

	async function resumeCreateRequest() {
		for (let i = 0; i < 8; i++) {
			const res = await api.post('/verification-requests', {
				entity_type: org ? 'organization' : 'user',
				organization_id: org?.id,
				requested_type: requestedType || undefined,
				form_submission_id: submissionId
			});
			if (res.data) {
				sessionStorage.removeItem(PENDING_KEY);
				phase = 'done';
				await invalidateAll();
				return;
			}
			const code = res.error?.code ?? '';
			const msg = res.error?.message ?? '';
			if (code === 'already_exists') {
				// A request already exists for this entity — treat as done.
				sessionStorage.removeItem(PENDING_KEY);
				phase = 'done';
				await invalidateAll();
				return;
			}
			// 'invalid_status_transition' while the webhook hasn't flipped the
			// submission yet — keep waiting. 'already verified' errors carry the
			// same code but mention verified in the message: bail immediately.
			if (code === 'invalid_status_transition' && !/verified/i.test(msg)) {
				await sleep(2500);
				continue;
			}
			// Hard failure (forbidden / already verified / not found): keep
			// the session marker so a later visit can still retry.
			errorKey = 'verification_request.errors.request_failed';
			phase = 'processing';
			return;
		}
		// Payment never confirmed within the retry window.
		phase = 'processing';
	}

	async function withdrawRequest(id: string) {
		if (withdrawing) return;
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

	function fieldError(f: FormField): string {
		const v = values[f.label];
		if (f.required && isValueEmpty(v)) {
			return t('verification_request.errors.field_required_inline', 'This field is required');
		}
		if (!isValueEmpty(v) && typeof v === 'string') {
			if (f.type === 'email' && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v.trim())) {
				return t('verification_request.errors.field_email', 'Enter a valid email address');
			}
			if (f.type === 'url') {
				try {
					new URL(v.trim());
				} catch {
					return t('verification_request.errors.field_url', 'Enter a valid URL');
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
			// Focus the first invalid field so keyboard/AT users land on it.
			requestAnimationFrame(() => document.getElementById(focusId)?.focus());
			return false;
		}
		return true;
	}

	function next() {
		errorKey = '';
		if (!validateStep(step)) return;
		if (step < reviewIndex) step++;
	}

	function back() {
		errorKey = '';
		if (step > 0) step--;
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

	function displayValue(f: FormField): string {
		const v = values[f.label];
		if (MEDIA_TYPES.has(f.type)) {
			const n = f.type === 'image_single' ? (mediaSingle(f.label) ? 1 : 0) : mediaItems(f.label).length;
			return n
				? t('verification_request.files_attached', '{n} file(s) attached', { n })
				: '—';
		}
		if (f.type === 'checkbox') {
			return v === 'true'
				? t('verification_request.yes', 'Yes')
				: t('verification_request.no', 'No');
		}
		const s = String(v ?? '').trim();
		return s || '—';
	}

	async function submit() {
		errorKey = '';
		if (!form || !validateStep(step)) return;
		if (isPaid && paymentMethod === 'credit' && creditsShort) {
			errorKey = 'verification_request.errors.insufficient_credits';
			return;
		}

		loading = true;
		try {
			// A previous attempt may have already created (and possibly paid
			// for) a submission — retry the request on it instead of paying again.
			if (submissionId) {
				await createRequest();
				return;
			}
			const body: Record<string, unknown> = { field_values: values };
			if (isPaid) {
				body.payment_method = paymentMethod;
				// §06 return_url: the hosted checkout sends the user back here
				// with ?paid=1 so resumeCreateRequest can link the paid
				// submission to the verification request.
				const ret = safeSameOriginHref(
					`${page.url.origin}/verification-request?${org ? `org=${org.slug}` : 'self=1'}&paid=1`
				);
				if (ret) body.return_url = ret;
				// §06 link intent: the payment webhook auto-creates the request
				// when payment confirms — covers the user never returning.
				body.link = {
					kind: 'verification_request',
					entity_type: org ? 'organization' : 'user',
					organization_id: org?.id ?? '',
					requested_type: requestedType || ''
				};
			}

			const res = await api.post<FormSubmissionResponse>(`/forms/${form.id}/submissions`, body);
			if (!res.data) {
				errorKey = res.error?.code === 'insufficient_credits'
					? 'verification_request.errors.insufficient_credits'
					: res.error?.code === 'payment_intent_failed'
						? 'verification_request.errors.payment_failed'
						: res.error?.code === 'invalid_field_values'
							? 'verification_request.errors.submit_invalid'
							: 'verification_request.errors.submit_failed';
				return;
			}
			if (res.data.status === 'pending_payment' && res.data.checkout_url) {
				// Stash what the resume needs — the checkout round-trip loses
				// this page's in-memory state.
				sessionStorage.setItem(
					PENDING_KEY,
					JSON.stringify({
						submission_id: res.data.id,
						requested_type: requestedType || undefined,
						entity: org ? `org:${org.slug}` : 'self'
					})
				);
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
				await invalidateAll();
			} else {
				errorKey = res.error?.code === 'already_exists'
					? 'verification_request.errors.pending_exists'
					: res.error?.code === 'invalid_status_transition' || res.error?.code === 'forbidden'
						? 'verification_request.errors.not_eligible'
						: 'verification_request.errors.request_failed';
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
										{#if req.requested_type}
											· {req.requested_type.replaceAll('_', ' ')}
										{/if}
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
											onclick={() => (withdrawTarget = req.id)}
											class="inline-flex items-center gap-1 rounded-[var(--radius)] border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-danger)] transition-colors hover:bg-[var(--color-surface-subtle)]"
										>
											<X size={12} />
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
	<div class="mx-auto w-full max-w-4xl p-4 sm:p-6">
		{#if blocked === 'already_verified'}
			<EmptyState
				icon={BadgeCheck}
				title={t('verification_request.already_verified_title', 'Already verified')}
				body={org
					? t('verification_request.already_verified_org_body', 'This organization already has a verification badge.')
					: t('verification_request.already_verified_body', 'Your profile already has a verification badge.')}
			/>
		{:else if blocked === 'pending_exists'}
			<EmptyState
				icon={Clock}
				title={t('verification_request.pending_exists_title', 'Request already pending')}
				body={t('verification_request.pending_exists_body', 'A verification request is already under review. You can track or withdraw it from My requests.')}
			/>
			<p class="mt-3 text-center">
				<a href="/verification-request" class="text-sm text-[var(--color-primary)] hover:underline">{t('verification_request.my_requests_link', 'My requests')}</a>
			</p>
		{:else if blocked === 'not_org_admin'}
			<EmptyState
				icon={ShieldCheck}
				title={t('verification_request.not_org_admin_title', 'Owners and admins only')}
				body={t('verification_request.not_org_admin_body', 'Only the owner or an admin of this organization can request verification.')}
			/>
		{:else if blocked === 'no_permission'}
			<EmptyState
				icon={ShieldCheck}
				title={t('verification_request.no_permission_title', 'Verification unavailable')}
				body={t('verification_request.no_permission_body', 'Your account cannot submit verification requests.')}
			/>
		{:else if phase === 'processing'}
			<div class="flex flex-col items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center">
				<Clock size={28} class="text-[var(--color-primary)]" />
				<p class="text-base font-medium text-[var(--color-text)]">{t('verification_request.processing_title', 'Confirming your payment')}</p>
				<p class="max-w-md text-sm text-[var(--color-text-subtle)]">{t('verification_request.processing_body', "We're still confirming your payment. Your request will appear under My requests once it completes.")}</p>
				{#if errorKey}
					<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">{t(errorKey)}</p>
				{/if}
				<div class="mt-2 flex items-center gap-4">
					{#if submissionId}
						<button
							type="button"
							onclick={() => { errorKey = ''; phase = 'creating_request'; void resumeCreateRequest(); }}
							class="rounded-full bg-[var(--color-primary)] px-5 py-2 text-sm font-semibold text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
						>
							{t('verification_request.retry', 'Try again')}
						</button>
					{/if}
					<a href="/verification-request" class="rounded-full border border-[var(--color-border)] px-5 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
						{t('verification_request.my_requests_link', 'My requests')}
					</a>
				</div>
			</div>
		{:else if phase === 'done'}
			<div class="flex flex-col items-center gap-3 rounded-[var(--radius-card)] border border-green-500/30 bg-green-500/10 p-8 text-center">
				<div class="grid h-14 w-14 place-items-center rounded-full bg-[var(--color-success)]">
					<Check size={28} class="text-white" />
				</div>
				<p class="max-w-lg text-lg font-medium text-green-600">{t('verification_request.request_created', 'Your verification request has been submitted and will be reviewed by our team.')}</p>
				<div class="mt-2 flex items-center gap-4">
					{#if org}
						<a href="/org/{org.slug}" class="rounded-full border border-[var(--color-border)] px-5 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
							{t('verification_request.back_to_org', 'Back to organization')}
						</a>
					{/if}
					<a href="/verification-request" class="text-sm font-medium text-[var(--color-primary)] hover:underline">
						{t('verification_request.my_requests_link', 'My requests')}
					</a>
				</div>
			</div>
		{:else if phase === 'redirecting' || phase === 'creating_request'}
			<div class="flex flex-col items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-8 text-center">
				<Loader2 size={24} class="animate-spin text-[var(--color-primary)]" />
				<p class="text-sm text-[var(--color-text)]">
					{phase === 'redirecting'
						? t('verification_request.redirecting_payment', 'Redirecting to payment provider…')
						: t('verification_request.creating_request', 'Creating request…')}
				</p>
			</div>
		{:else if !form}
			<EmptyState icon={BadgeCheck} title={t('verification_request.form_unavailable', 'Verification requests unavailable')} body={t('verification_request.form_unavailable_body', 'The request form is not available right now. Please try again later.')} />
		{:else}
			<!-- Wizard card — same visual language as /onboarding and the
			     journey modal: brand panel + numbered stepper + pill actions. -->
			<div class="grid grid-cols-1 overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl lg:grid-cols-[280px_1fr]">
				<!-- Brand panel — verification-themed illustration + privacy note. -->
				<aside aria-hidden="true" class="vr-side relative hidden flex-col gap-4 overflow-hidden p-7 lg:flex">
					<h2 class="text-lg leading-snug font-bold text-[var(--color-text)]">
						{t('verification_request.side_title', 'Get your badge verified')}
					</h2>
					<p class="text-sm leading-relaxed text-[var(--color-text-subtle)]">
						{t('verification_request.side_subtitle', 'Provide the documents and details our review team needs to verify your identity.')}
					</p>

					<!-- CSS illustration: badge focal + document chips + dashed route. -->
					<div class="relative mt-2 min-h-44 flex-1">
						<svg class="absolute inset-0 h-full w-full" viewBox="0 0 240 200" fill="none" aria-hidden="true">
							<path d="M30 175 C 60 100, 130 105, 210 40" stroke="var(--color-primary)" stroke-width="1.5" stroke-dasharray="5 6" stroke-linecap="round" opacity="0.45" />
							<circle cx="30" cy="175" r="4.5" fill="var(--color-primary)" opacity="0.5" />
							<circle cx="210" cy="40" r="4.5" fill="var(--color-primary)" opacity="0.5" />
						</svg>
						<div class="vr-focal">
							<BadgeCheck size={56} strokeWidth={1.1} class="text-[var(--color-primary)]" />
						</div>
						<div class="vr-chip" style="top: 6%; inset-inline-start: 14%;"><Fingerprint size={17} class="text-[var(--color-primary)]" /></div>
						<div class="vr-chip" style="top: 34%; inset-inline-end: 6%;"><FileText size={17} class="text-[var(--color-primary)]" /></div>
						<div class="vr-chip" style="bottom: 18%; inset-inline-start: 8%;"><Landmark size={17} class="text-[var(--color-primary)]" /></div>
					</div>

					<div class="mt-auto flex items-start gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)]/80 px-4 py-3 backdrop-blur-sm">
						<ShieldCheck size={18} class="mt-0.5 shrink-0 text-[var(--color-primary)]" />
						<p class="text-xs leading-relaxed text-[var(--color-text-subtle)]">
							<span class="block font-semibold text-[var(--color-text)]">{t('verification_request.privacy_title', 'Your evidence stays private.')}</span>
							{t('verification_request.privacy_body', 'Documents you upload are only visible to our review team.')}
						</p>
					</div>
				</aside>

				<div class="flex min-w-0 flex-col p-5 sm:p-8">
					<!-- Numbered stepper — completed steps are clickable. -->
					<nav aria-label={t('verification_request.steps_label', 'Verification request steps')} class="mb-7 flex items-start">
						{#each wizardSteps as s, i (s.id)}
							{@const n = i + 1}
							<div class="flex flex-1 flex-col items-center gap-1.5 last:flex-none">
								<div class="flex w-full items-center">
									{#if i > 0}<span class="h-px flex-1 {n <= step + 1 ? 'bg-[var(--color-primary)]' : 'bg-[var(--color-border)]'}"></span>{/if}
									<button
										type="button"
										disabled={i >= step}
										onclick={() => { errorKey = ''; step = i; }}
										class="grid h-9 w-9 shrink-0 place-items-center rounded-full text-sm font-semibold transition-colors disabled:cursor-default {i === step
											? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
											: i < step
												? 'bg-[var(--color-primary)]/15 text-[var(--color-primary)] hover:bg-[var(--color-primary)]/25'
												: 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]'}"
										aria-current={i === step ? 'step' : undefined}
										aria-label={s.title}
									>
										{#if i < step}<Check size={15} />{:else}{n}{/if}
									</button>
									{#if i < totalSteps - 1}<span class="h-px flex-1 {n <= step ? 'bg-[var(--color-primary)]' : 'bg-[var(--color-border)]'}"></span>{/if}
								</div>
								<span class="hidden text-[11px] font-medium sm:block {i === step ? 'text-[var(--color-primary)]' : 'text-[var(--color-muted)]'}">{s.title}</span>
							</div>
						{/each}
					</nav>

					<!-- Step 0: Request — entity context + suggested badge type. -->
					{#if step === 0}
						<h2 class="text-xl font-bold text-[var(--color-text)]">{t('verification_request.step1_title', 'What are we verifying?')}</h2>
						<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
							{form.description || t('verification_request.subtitle', 'Submit evidence to request a verification badge. All requests are reviewed by our team.')}
						</p>

						<div class="mt-5 flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4">
							<span class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface)] text-[var(--color-primary)]">
								<BadgeCheck size={18} />
							</span>
							<div class="min-w-0">
								<p class="truncate text-sm font-medium text-[var(--color-text)]">
									{org ? org.name : t('verification_request.profile_request', 'Profile verification')}
								</p>
								{#if org}<p class="truncate text-xs text-[var(--color-muted)]">/{org.slug}</p>{/if}
							</div>
						</div>

						{#if types.length > 0}
							<div class="mt-4 flex flex-col gap-1.5">
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
					{:else if step === reviewIndex}
						<!-- Final step: review answers + payment + submit. -->
						<h2 class="text-xl font-bold text-[var(--color-text)]">{t('verification_request.review_title', 'Review and submit')}</h2>
						<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{t('verification_request.review_subtitle', 'Check your answers before sending the request for review.')}</p>

						<dl class="mt-5 divide-y divide-[var(--color-border)] rounded-[var(--radius)] border border-[var(--color-border)]">
							<div class="flex items-start justify-between gap-4 px-4 py-2.5">
								<dt class="text-xs font-medium text-[var(--color-muted)]">{t('verification_request.summary_entity', 'Verifying')}</dt>
								<dd class="text-end text-sm text-[var(--color-text)]">{org ? org.name : t('verification_request.profile_request', 'Profile verification')}</dd>
							</div>
							<div class="flex items-start justify-between gap-4 px-4 py-2.5">
								<dt class="text-xs font-medium text-[var(--color-muted)]">{t('verification_request.summary_badge', 'Badge type')}</dt>
								<dd class="text-end text-sm text-[var(--color-text)]">{requestedType ? requestedType.replaceAll('_', ' ') : t('verification_request.type_placeholder', 'Let reviewers decide')}</dd>
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
								<p class="mb-1 text-sm font-medium text-[var(--color-text)]">{t('verification_request.payment_required', 'Submission fee')}</p>
								<p class="mb-3 text-xs text-[var(--color-muted)]">{t('verification_request.payment_hint', 'This form requires a submission fee of {credits} credits.').replace('{credits}', String(feeCredits))}</p>
								<div class="flex flex-col gap-2" role="radiogroup" aria-label={t('verification_request.payment_required', 'Submission fee')}>
									<label class="flex items-center gap-2 rounded-[var(--radius)] border px-3 py-2 text-sm transition-colors {paymentMethod === 'credit' ? 'border-[var(--color-primary)] bg-[var(--color-surface)]' : 'border-[var(--color-border)]'} {creditsShort ? 'opacity-60' : 'cursor-pointer'}">
										<input type="radio" name="payment_method" value="credit" checked={paymentMethod === 'credit'} disabled={creditsShort} onchange={() => (paymentMethod = 'credit')} class="h-4 w-4 accent-[var(--color-primary)]" />
										<Wallet size={16} class="shrink-0 text-[var(--color-primary)]" />
										<span class="flex-1 text-[var(--color-text)]">{t('verification_request.pay_with_credits', 'Pay with credits ({credits})').replace('{credits}', String(feeCredits))}</span>
										<span class="text-xs {creditsShort ? 'text-[var(--color-danger)]' : 'text-[var(--color-muted)]'}">
											{t('verification_request.pay_balance', 'Balance: {credits}').replace('{credits}', String(credits))}
										</span>
									</label>
									<label class="flex cursor-pointer items-center gap-2 rounded-[var(--radius)] border px-3 py-2 text-sm transition-colors {paymentMethod === 'paystack' ? 'border-[var(--color-primary)] bg-[var(--color-surface)]' : 'border-[var(--color-border)]'}">
										<input type="radio" name="payment_method" value="paystack" checked={paymentMethod === 'paystack'} onchange={() => (paymentMethod = 'paystack')} class="h-4 w-4 accent-[var(--color-primary)]" />
										<CreditCard size={16} class="shrink-0 text-[var(--color-primary)]" />
										<span class="text-[var(--color-text)]">{t('verification_request.pay_with_gateway', 'Pay with card (Paystack)')}</span>
									</label>
									<label class="flex cursor-pointer items-center gap-2 rounded-[var(--radius)] border px-3 py-2 text-sm transition-colors {paymentMethod === 'stripe' ? 'border-[var(--color-primary)] bg-[var(--color-surface)]' : 'border-[var(--color-border)]'}">
										<input type="radio" name="payment_method" value="stripe" checked={paymentMethod === 'stripe'} onchange={() => (paymentMethod = 'stripe')} class="h-4 w-4 accent-[var(--color-primary)]" />
										<CreditCard size={16} class="shrink-0 text-[var(--color-primary)]" />
										<span class="text-[var(--color-text)]">{t('verification_request.pay_with_stripe', 'Pay with card (Stripe)')}</span>
									</label>
								</div>
								{#if creditsShort}
									<p class="mt-2 flex items-center gap-1.5 text-xs text-[var(--color-danger)]">
										<CircleAlert size={13} />
										{t('verification_request.insufficient_balance', 'Your credit balance is too low — choose card payment or top up your wallet.')}
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
										<textarea id="field-{field.id}" rows="4" value={String(values[field.label] ?? '')} oninput={(e) => (values = { ...values, [field.label]: e.currentTarget.value })} aria-required={field.required} aria-invalid={Boolean(fieldErrors[field.id])} class={fieldInputClass(field.id)}></textarea>
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
										<div class="space-y-2" id="field-{field.id}" tabindex="-1">
											{#if single}
												<div class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm">
													{#if single.url}<img src={single.url} alt={single.alt ?? ''} class="h-10 w-10 rounded object-cover" />{/if}
													<span class="min-w-0 flex-1 truncate text-[var(--color-muted)]">{single.url ?? single.id}</span>
													<button type="button" onclick={() => (values = { ...values, [field.label]: '' })} class="text-[var(--color-danger)]" aria-label={t('verification_request.media_remove', 'Remove')}>
														<X size={14} />
													</button>
												</div>
											{/if}
											{#each items as item, i (item.id)}
												<div class="flex items-center gap-2 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-2 text-sm">
													{#if item.url}<img src={item.url} alt={item.alt ?? ''} class="h-10 w-10 rounded object-cover" />{/if}
													<span class="min-w-0 flex-1 truncate text-[var(--color-muted)]">{item.url ?? item.id}</span>
													<button type="button" onclick={() => (values = { ...values, [field.label]: items.filter((_, j) => j !== i) })} class="text-[var(--color-danger)]" aria-label={t('verification_request.media_remove', 'Remove')}>
														<X size={14} />
													</button>
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
											<input type="checkbox" id="field-{field.id}" checked={values[field.label] === 'true'} onchange={(e) => (values = { ...values, [field.label]: e.currentTarget.checked ? 'true' : '' })} aria-required={field.required} aria-invalid={Boolean(fieldErrors[field.id])} class="h-4 w-4 rounded border-[var(--color-border)] accent-[var(--color-primary)]" />
											{field.label}
											{#if field.required}<span class="text-red-500" aria-hidden="true">*</span>{/if}
										</label>
									{:else}
										<input id="field-{field.id}" type={field.type === 'email' ? 'email' : field.type === 'number' ? 'number' : field.type === 'url' ? 'url' : 'text'} value={String(values[field.label] ?? '')} oninput={(e) => (values = { ...values, [field.label]: e.currentTarget.value })} aria-required={field.required} aria-invalid={Boolean(fieldErrors[field.id])} class={fieldInputClass(field.id)} />
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
					{/if}

					<!-- Actions — pill buttons matching onboarding/journey. -->
					<div class="mt-8 flex items-center justify-between gap-3">
						{#if step === 0}
							<a
								href="/verification-request"
								class="flex items-center gap-1.5 rounded-full border border-[var(--color-border)] px-5 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								{t('verification_request.my_requests_link', 'My requests')}
							</a>
						{:else}
							<button
								type="button"
								onclick={back}
								disabled={loading}
								class="flex items-center gap-1.5 rounded-full border border-[var(--color-border)] px-5 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
							>
								<ChevronLeft class="rtl:-scale-x-100" size={16} />
								{t('verification_request.back', 'Back')}
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
								? (loading ? t('verification_request.creating_request', 'Creating request…') : (form.submit_label || t('verification_request.submit', 'Submit request')))
								: t('verification_request.continue', 'Continue')}
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

<!-- Withdraw confirmation — replaces native confirm() for consistency. -->
<Modal
	open={withdrawTarget !== null}
	onOpenChange={(v) => { if (!v) withdrawTarget = null; }}
	title={t('verification_request.withdraw_title', 'Withdraw request?')}
	icon={BadgeCheck}
	size="sm"
>
	{#snippet children(close)}
		<p class="mb-5 text-sm text-[var(--color-text-subtle)]">
			{t('verification_request.withdraw_body', 'This request will be withdrawn and removed from the review queue.')}
		</p>
		<div class="flex justify-end gap-2">
			<button type="button" onclick={close} class="rounded-full border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
				{t('verification_request.cancel', 'Cancel')}
			</button>
			<button
				type="button"
				disabled={withdrawing !== ''}
				onclick={async () => { const id = withdrawTarget; close(); if (id) await withdrawRequest(id); }}
				class="inline-flex items-center gap-1.5 rounded-full bg-[var(--color-danger)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
			>
				{#if withdrawing}<Loader2 size={14} class="animate-spin" />{/if}
				{t('verification_request.withdraw', 'Withdraw')}
			</button>
		</div>
	{/snippet}
</Modal>

<style>
	.vr-side {
		background:
			radial-gradient(ellipse 80% 55% at 35% 42%, color-mix(in srgb, var(--color-primary) 12%, transparent), transparent),
			color-mix(in srgb, var(--color-primary) 6%, var(--color-surface));
	}
	.vr-focal {
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
		box-shadow: 0 10px 28px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
	.vr-chip {
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
		.vr-chip {
			animation: vr-float 4s ease-in-out infinite;
		}
		.vr-chip:nth-of-type(3) { animation-delay: 0.7s; }
		.vr-chip:nth-of-type(4) { animation-delay: 1.4s; }
	}
	@keyframes vr-float {
		0%, 100% { translate: 0 0; }
		50% { translate: 0 -6px; }
	}
</style>
