<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	// §22 Appeals — reusable appeal submission control. Mount wherever a
	// rejected/appealable decision is surfaced (service claims, ownership
	// claims, video channel claims, moderation decisions). Submits
	// POST /api/v1/appeals which records the moderation_appeal and opens a
	// linked 'appeal' ticket the user can track from /dashboard/tickets.
	//
	// Design mirrors the onboarding/journey wizards (JourneyOnboardingModal,
	// /onboarding): stepper header, decorative aside, field-* inputs,
	// privacy note, pill buttons.
	//
	// Ref: japaconnect-engineering/sections/22-admin-panel.md (Appeals)
	import { page } from '$app/state';
	import { Modal } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import ThemeLogo from '$lib/components/branding/ThemeLogo.svelte';
	import {
		AlertCircle, Check, CheckCircle2, ChevronRight, FileText, Gavel,
		Loader2, MessageSquare, Scale, ShieldCheck, X
	} from 'lucide-svelte';

	interface AppealResult {
		id: string;
		status: string;
		ticket_id?: string;
		ticket_number?: number;
	}

	interface Props {
		/** moderation_appeals.entity_type — e.g. 'service_claim', 'org_claim'. */
		entityType: string;
		/** Subject UUID — e.g. the service listing content id or claim id. */
		entityId: string;
		/** Human label of the decision being appealed (e.g. the eligible-item
		 * label) — shown as context so the user knows what they contest. */
		subject?: string;
		/** Button label override. */
		label?: string;
		/** Extra classes merged onto the trigger button. */
		class?: string;
		/** Called after a successful submit — lets lists drop the item. */
		onSubmitted?: (result: AppealResult) => void;
	}
	let { entityType, entityId, subject, label, class: cls = '', onSubmitted }: Props = $props();

	const MAX_REASON = 4000;
	const textareaId = `appeal-reason-${Math.random().toString(36).slice(2, 9)}`;

	let open = $state(false);
	let step = $state(1);
	let reason = $state('');
	let submitting = $state(false);
	let error = $state('');
	let done = $state<AppealResult | null>(null);

	const steps = $derived([
		{ icon: Gavel, label: t('appeals.step_appeal', 'Your appeal') },
		{ icon: FileText, label: t('appeals.step_review', 'Review') },
		{ icon: CheckCircle2, label: t('appeals.step_done', 'Done') }
	]);
	const canContinue = $derived(step !== 1 || reason.trim() !== '');
	const logoUrl = $derived(
		(page.data.siteSettings as Record<string, string> | undefined)?.['appearance.logo_url'] ?? ''
	);
	const logoDarkUrl = $derived(
		(page.data.siteSettings as Record<string, string> | undefined)?.['appearance.logo_url_dark'] ?? ''
	);

	function openModal() {
		done = null;
		error = '';
		reason = '';
		step = 1;
		open = true;
	}

	function next() {
		if (!canContinue) {
			error = t('appeals.reason_required', 'Please explain why the decision should be reconsidered.');
			return;
		}
		error = '';
		if (step === 1) step = 2;
		else void submit();
	}

	function back() {
		error = '';
		if (step > 1) step--;
	}

	async function submit() {
		if (submitting) return;
		if (!reason.trim()) {
			error = t('appeals.reason_required', 'Please explain why the decision should be reconsidered.');
			step = 1;
			return;
		}
		submitting = true;
		error = '';
		const res = await api.post<AppealResult>('/appeals', {
			entity_type: entityType,
			entity_id: entityId,
			reason: reason.trim()
		});
		submitting = false;
		if (res.data) {
			done = res.data;
			step = 3;
			onSubmitted?.(res.data);
		} else if (res.error?.code === 'duplicate_appeal') {
			error = t('appeals.duplicate', 'You already have a pending appeal for this decision.');
			step = 1;
		} else {
			error = res.error?.message ?? t('appeals.failed', 'Failed to submit appeal.');
			step = 1;
		}
	}
</script>

<button
	type="button"
	onclick={openModal}
	class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] {cls}"
>
	<Scale size={15} aria-hidden="true" />
	{label ?? t('appeals.appeal', 'Appeal')}
</button>

<Modal
	bind:open
	onOpenChange={(v) => { if (!v) step = 1; }}
	hideHeader
	padded={false}
	ariaTitle={t('appeals.dialog_title', 'Appeal this decision')}
	size="max-w-3xl"
	class="max-sm:bottom-0 max-sm:start-0 max-sm:end-0 max-sm:top-auto max-sm:w-full max-sm:max-w-none max-sm:translate-x-0 max-sm:translate-y-0 max-sm:rounded-t-2xl max-sm:rounded-b-none"
>
	{#snippet children(close)}
		<!-- Header: brand + stepper + close (journey-modal pattern) -->
		<div class="flex items-center gap-4 border-b border-[var(--color-border)] px-5 py-4 sm:px-7">
			<ThemeLogo {logoUrl} {logoDarkUrl} alt={t('app.name', '{site}')} class="h-6 w-auto shrink-0" />
			<nav aria-label={t('appeals.steps_label', 'Appeal steps')} class="mx-auto flex items-center">
				{#each steps as s, i (i)}
					{@const n = i + 1}
					<span class="flex flex-col items-center gap-1.5">
						<span
							class="grid h-9 w-9 place-items-center rounded-full transition-colors {n === step
								? 'bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
								: n < step
									? 'bg-[var(--color-surface-subtle)] text-[var(--color-primary)]'
									: 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)]'}"
							aria-current={n === step ? 'step' : undefined}
						>
							{#if n < step}<Check size={15} />{:else}<s.icon size={16} />{/if}
						</span>
						<span class="hidden text-[11px] font-medium sm:block {n === step ? 'text-[var(--color-primary)]' : 'text-[var(--color-muted)]'}">{s.label}</span>
					</span>
					{#if i < steps.length - 1}
						<span class="mx-2 mb-0 h-px w-8 sm:mb-5 sm:w-12 {n < step ? 'bg-[var(--color-primary)]' : 'bg-[var(--color-border)]'}"></span>
					{/if}
				{/each}
			</nav>
			<button
				type="button"
				onclick={close}
				aria-label={t('common.close', 'Close')}
				class="grid h-8 w-8 shrink-0 place-items-center rounded-full text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
			>
				<X size={16} />
			</button>
		</div>

		<div class="grid grid-cols-1 sm:grid-cols-[220px_1fr]">
			<!-- Decorative panel — same CSS-illustration pattern as journey/onboarding -->
			<aside aria-hidden="true" class="appeal-scene relative hidden overflow-hidden sm:block">
				<svg class="appeal-arc" viewBox="0 0 200 220" fill="none">
					<path d="M30 200 C 30 120, 90 90, 160 40" stroke="var(--color-primary)" stroke-width="1.5" stroke-dasharray="5 6" stroke-linecap="round" opacity="0.45" />
					<circle cx="30" cy="200" r="4" fill="var(--color-primary)" opacity="0.5" />
					<circle cx="160" cy="40" r="4" fill="var(--color-primary)" opacity="0.5" />
				</svg>
				<div class="appeal-focal">
					<Scale size={52} strokeWidth={1.2} class="text-[var(--color-primary)]" />
				</div>
				<div class="appeal-chip" style="top: 14%; inset-inline-start: 10%;">
					<Gavel size={18} class="text-[var(--color-primary)]" />
				</div>
				<div class="appeal-chip" style="top: 44%; inset-inline-start: 6%;">
					<FileText size={18} class="text-[var(--color-primary)]" />
				</div>
				<div class="appeal-chip" style="bottom: 16%; inset-inline-end: 12%;">
					<MessageSquare size={18} class="text-[var(--color-primary)]" />
				</div>
			</aside>

			<!-- Step content -->
			<div class="p-6 sm:p-8">
				{#if step === 1}
					<h2 class="text-xl font-bold text-[var(--color-text)]">{t('appeals.dialog_title', 'Appeal this decision')}</h2>
					<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{t('appeals.dialog_subtitle', 'Tell our team why this decision should be reconsidered. A staff member will review your case.')}</p>

					{#if subject}
						<div class="mt-5 flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-4 py-3">
							<Gavel size={16} class="shrink-0 text-[var(--color-primary)]" aria-hidden="true" />
							<div class="min-w-0">
								<p class="text-xs font-medium uppercase tracking-wider text-[var(--color-muted)]">{t('appeals.entity_type', 'Decision')}</p>
								<p class="truncate text-sm font-medium text-[var(--color-text)]">{subject}</p>
							</div>
						</div>
					{/if}

					<div class="mt-5">
						<div class="field-row">
							<label class="field-label" for={textareaId}>{t('appeals.reason_label', 'Why should this decision be reconsidered?')}</label>
							<textarea
								id={textareaId}
								rows="5"
								bind:value={reason}
								maxlength={MAX_REASON}
								placeholder={t('appeals.reason_placeholder', 'Explain briefly why the decision was wrong — links or reference details help the review.')}
								aria-describedby="appeal-reason-hint"
								class="field-input !h-auto py-2.5"
							></textarea>
							<HintIcon id="appeal-reason-hint" text={t('appeals.reason_hint', 'Be specific — cite the policy or evidence that supports your case.')} />
							<p class="text-end text-[11px] text-[var(--color-muted)]">{reason.length} / {MAX_REASON}</p>
						</div>
					</div>
				{:else if step === 2}
					<h2 class="text-xl font-bold text-[var(--color-text)]">{t('appeals.review_title', 'Review your appeal')}</h2>
					<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{t('appeals.review_subtitle', 'Check the details before submitting — a staff thread is opened for follow-up.')}</p>

					<dl class="mt-6 space-y-3">
						{#if subject}
							<div class="summary-row"><dt>{t('appeals.entity_type', 'Decision')}</dt><dd>{subject}</dd></div>
						{/if}
						<div class="summary-row"><dt>{t('appeals.reason_label', 'Why should this decision be reconsidered?')}</dt><dd class="whitespace-pre-wrap">{reason.trim()}</dd></div>
					</dl>
				{:else}
					<div class="flex flex-col items-center gap-3 py-6 text-center">
						<CheckCircle2 size={40} class="text-[var(--color-success)]" aria-hidden="true" />
						<h2 class="text-xl font-bold text-[var(--color-text)]">{t('appeals.done_title', 'Appeal submitted')}</h2>
						<p class="text-sm text-[var(--color-text-subtle)]">
							{t('appeals.submitted', 'Appeal submitted — our team will review it.')}
						</p>
						{#if done?.ticket_id}
							<a
								href="/dashboard/tickets/{done.ticket_number ? `JC-${done.ticket_number}` : done.ticket_id}"
								class="text-sm font-medium text-[var(--color-primary)] hover:underline"
							>
								{t('appeals.track_ticket', 'Track it in your support tickets')}
							</a>
						{/if}
					</div>
				{/if}

				{#if error}
					<p role="alert" class="mt-4 flex items-center gap-1.5 rounded bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950 dark:text-red-200">
						<AlertCircle size={14} class="shrink-0" aria-hidden="true" /> {error}
					</p>
				{/if}

				{#if step !== 3}
					<div class="mt-6 flex items-start gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-4 py-3">
						<ShieldCheck size={20} class="mt-0.5 shrink-0 text-[var(--color-primary)]" />
						<p class="text-xs leading-relaxed text-[var(--color-text-subtle)]">
							<span class="font-semibold text-[var(--color-text)]">{t('appeals.fair_title', 'Every appeal gets a human review.')}</span>
							{t('appeals.fair_body', 'A staff member reads each case; you can follow the conversation on the linked ticket.')}
						</p>
					</div>
				{/if}

				<div class="mt-6 flex items-center justify-between gap-3">
					{#if step === 3}
						<span></span>
						<button
							type="button"
							onclick={close}
							class="rounded-full bg-[var(--color-primary)] px-5 py-2 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover,var(--color-primary))]"
						>
							{t('common.close', 'Close')}
						</button>
					{:else}
						<button
							type="button"
							onclick={step === 1 ? close : back}
							class="rounded-full border border-[var(--color-border)] px-5 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							{step === 1 ? t('common.cancel', 'Cancel') : t('common.back', 'Back')}
						</button>
						<button
							type="button"
							class="flex items-center gap-2 rounded-full bg-[var(--color-primary)] px-5 py-2 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover,var(--color-primary))] disabled:opacity-50"
							onclick={next}
							disabled={submitting}
						>
							{#if submitting}
								<Loader2 size={15} class="motion-safe:animate-spin" aria-hidden="true" />
								{t('appeals.submitting', 'Submitting...')}
							{:else}
								{step === 2 ? t('appeals.submit', 'Submit appeal') : t('common.continue', 'Continue')}
								<ChevronRight class="rtl:-scale-x-100" size={15} />
							{/if}
						</button>
					{/if}
				</div>
			</div>
		</div>
	{/snippet}
</Modal>

<style>
	.appeal-scene {
		background:
			radial-gradient(ellipse 75% 55% at 30% 45%, color-mix(in srgb, var(--color-primary) 12%, transparent), transparent),
			color-mix(in srgb, var(--color-primary) 5%, var(--color-surface));
	}
	.appeal-arc {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
	}
	.appeal-focal {
		position: absolute;
		top: 50%;
		inset-inline-start: 50%;
		translate: -50% -50%;
		display: grid;
		place-items: center;
		width: 6.5rem;
		height: 6.5rem;
		border-radius: 9999px;
		background: var(--color-surface);
		box-shadow: 0 8px 24px color-mix(in srgb, var(--color-primary) 15%, transparent);
	}
	.appeal-chip {
		position: absolute;
		display: grid;
		place-items: center;
		width: 2.5rem;
		height: 2.5rem;
		border-radius: 0.75rem;
		background: var(--color-surface);
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
	}
	@media (prefers-reduced-motion: no-preference) {
		.appeal-chip {
			animation: appeal-float 4s ease-in-out infinite;
		}
		.appeal-chip:nth-of-type(2) { animation-delay: 0.7s; }
		.appeal-chip:nth-of-type(3) { animation-delay: 1.4s; }
	}
	@keyframes appeal-float {
		0%, 100% { translate: 0 0; }
		50% { translate: 0 -6px; }
	}
	.field-row {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}
	.field-label {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.025em;
		color: var(--color-text-subtle);
	}
	.field-input {
		width: 100%;
		border-radius: var(--radius);
		border: 1px solid var(--color-border);
		background-color: var(--color-surface);
		padding: 0.5rem 0.75rem;
		font-size: 0.875rem;
		color: var(--color-text);
	}
	.field-input::placeholder {
		color: var(--color-muted);
	}
	.field-input:focus {
		outline: 2px solid var(--color-primary);
		outline-offset: -2px;
	}
	.summary-row {
		display: flex;
		flex-direction: column;
		gap: 0.125rem;
	}
	.summary-row dt {
		font-size: 0.7rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.025em;
		color: var(--color-muted);
	}
	.summary-row dd {
		font-size: 0.875rem;
		font-weight: 500;
		color: var(--color-text);
	}
</style>
