<script lang="ts">
	import { onMount } from 'svelte';
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { toolOgCard } from '#lib/seo/og.js';
	import { t, i18n } from '#lib/i18n/index.js';
	import { page } from '$app/state';
	import { Stamp, ArrowLeft, Download, AlertTriangle, MessageSquarePlus } from 'lucide-svelte';
	import { api } from '#lib/api/client.js';
	import BodyField from '#lib/components/ui/BodyField.svelte';
	import { docTypeLabel, isStepObject, stepFields, stepHeadline, stepValueText } from '#lib/attestation/steps.js';
	import type { AttestationCommunityUpdate } from '#lib/types/index.js';
	import type { PageData } from './$types';

	/**
	 * Attestation Guide detail page — Section 97, ADR-105.
	 * PDF export (§97 "exportable for in-person use") is delivered via the
	 * browser's print pipeline: a print stylesheet renders a clean document
	 * and ?print=1 auto-opens the print dialog so users can save as PDF.
	 *
	 * Ref: japaconnect-engineering/sections/97-attestation-guide.md
	 */
	let { data }: { data: PageData } = $props();

	const pathway = $derived(data.pathway!);
	const authed = $derived(data.authed);

	const title = $derived(`${docTypeLabel(pathway.document_type)} — ${t('attestation_guide.title', 'Attestation Guide')}`);
	const description = $derived(t('attestation_guide.subtitle', 'Step-by-step document attestation pathways.'));

	// ?print=1 — open the print dialog once content is mounted (invoked from
	// the list page's "Export PDF" link).
	onMount(() => {
		if (page.url.searchParams.get('print') === '1') {
			window.print();
		}
	});

	// Community update form state
	let showUpdateForm = $state(false);
	let updateStepIndex = $state('');
	let updateFeeEstimate = $state('');
	let updateTurnaroundDays = $state('');
	let updateNote = $state('');
	let submittingUpdate = $state(false);
	let updateError = $state<string | null>(null);
	let updateSubmitted = $state(false);

	function openUpdateForm() {
		if (!authed) {
			const next = encodeURIComponent(page.url.pathname);
			window.location.href = `/login?next=${next}`;
			return;
		}
		showUpdateForm = true;
		updateError = null;
	}

	async function submitUpdate() {
		// Mirror of the backend rule: a submission must carry a fee,
		// turnaround, or note (a step selection alone is a locator).
		if (!updateFeeEstimate.trim() && !updateTurnaroundDays && !updateNote.trim()) {
			updateError = t(
				'attestation_guide.update_required',
				'Add a fee, turnaround, or note before submitting.'
			);
			return;
		}
		submittingUpdate = true;
		updateError = null;
		const body: Record<string, unknown> = {};
		if (updateStepIndex !== '') body.step_index = parseInt(updateStepIndex, 10);
		if (updateFeeEstimate.trim()) body.fee_estimate = updateFeeEstimate.trim();
		if (updateTurnaroundDays !== '') body.turnaround_days = parseInt(updateTurnaroundDays, 10);
		if (updateNote.trim()) body.note = updateNote.trim();
		const res = await api.post<AttestationCommunityUpdate>(`/attestation/${pathway.id}/updates`, body);
		submittingUpdate = false;
		if (res.error) {
			updateError = res.error.message;
			return;
		}
		showUpdateForm = false;
		updateSubmitted = true;
	}

	function formatDate(iso?: string | null): string {
		if (!iso) return '';
		try {
			return new Date(iso).toLocaleDateString(i18n.locale, { year: 'numeric', month: 'short', day: 'numeric' });
		} catch {
			return iso;
		}
	}

	const STALENESS_THRESHOLD_DAYS = 180;

	function isStale(iso?: string | null): boolean {
		if (!iso) return true;
		try {
			const verified = new Date(iso);
			const daysOld = (Date.now() - verified.getTime()) / (1000 * 60 * 60 * 24);
			return daysOld > STALENESS_THRESHOLD_DAYS;
		} catch {
			return true;
		}
	}
</script>

<SeoHead title={title} description={description} ogTitle={title}
	ogImage={toolOgCard(page.url.origin, page.url.pathname)}
/>

<div class="mx-auto w-full max-w-3xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3 print:hidden">
		<a
			href={pathway.destination_space_id
				? `/tools/attestation-guide?dest=${pathway.destination_space_id}`
				: '/tools/attestation-guide'}
			class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
		>
			<ArrowLeft class="rtl:-scale-x-100" size={16} />
			{t('attestation_guide.back', 'Back to attestation guide')}
		</a>
	</div>

	<div class="p-4 space-y-4">
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
			<div class="flex items-start gap-3">
				<div class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary-text)]">
					<Stamp size={20} />
				</div>
				<div class="min-w-0 flex-1">
					<h1 class="text-lg font-semibold text-[var(--color-text)]">{docTypeLabel(pathway.document_type)}</h1>
					<p class="text-sm text-[var(--color-text-subtle)]">
						{pathway.issuing_space_name ?? t('attestation_guide.unknown_issuing', 'Unknown')}
						→
						{pathway.destination_space_name ?? t('attestation_guide.unknown_dest', 'Unknown')}
					</p>
					{#if pathway.apostille_required}
						<span
							class="mt-2 inline-flex items-center gap-1 rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs font-medium text-[var(--color-primary-text)]"
						>
							{t('attestation_guide.apostille_required', 'Apostille required')}
						</span>
					{:else}
						<span
							class="mt-2 inline-flex items-center gap-1 rounded-full bg-[var(--color-muted)]/10 px-2 py-0.5 text-xs font-medium text-[var(--color-muted)]"
						>
							{t('attestation_guide.no_apostille', 'No apostille')}
						</span>
					{/if}
				</div>
			</div>
		</div>

		{#if pathway.community_last_verified}
			<p class="text-xs text-[var(--color-muted)]">
				{t('attestation_guide.community_last_verified', 'Community last verified:')}
				{formatDate(pathway.community_last_verified)}
			</p>
		{/if}
		{#if isStale(pathway.community_last_verified)}
			<p
				class="rounded-[var(--radius)] px-3 py-2 text-xs"
				style="background: color-mix(in srgb, var(--color-warning, #f59e0b) 10%, transparent); color: var(--color-warning, #f59e0b)"
			>
				<AlertTriangle size={14} class="inline align-text-bottom" />
				{t(
					'attestation_guide.stale_warning',
					'This pathway has not been community-verified recently. Details may be outdated — please confirm with official sources.'
				)}
			</p>
		{/if}

		{#if Array.isArray(pathway.pathway_steps) && pathway.pathway_steps.length > 0}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-3 text-sm font-semibold text-[var(--color-text)]">{t('attestation_guide.pathway_steps', 'Pathway steps')}</h2>
				<ol class="space-y-3">
					{#each pathway.pathway_steps as step, i (i)}
						<li class="flex items-start gap-3">
							<span
								class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-xs font-semibold text-[var(--color-primary-text)]"
							>{i + 1}</span>

							<div
								class="min-w-0 flex-1 text-sm text-[var(--color-text)]"
							>
								{#if isStepObject(step)}
									<p class="font-medium">{stepHeadline(step, i)}</p>
									{#each stepFields(step) as field (field.label)}
										<div class="mt-0.5 flex items-start gap-1.5">
											<span class="text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">{field.label}:</span>
											{#if field.url}
												<a
													href={field.url}
													target="_blank"
													rel="noopener noreferrer"
													class="break-all text-[var(--color-primary-text)] hover:underline"
												>
													{field.value}
												</a>
											{:else}
												<span class="text-[var(--color-text-subtle)]">{field.value}</span>
											{/if}
										</div>
									{/each}
								{:else}
									<span class="text-[var(--color-text-subtle)]">{stepValueText(step)}</span>
								{/if}
							</div>
						</li>
					{/each}
				</ol>
			</div>
		{/if}

		<div class="flex flex-wrap items-center gap-3 print:hidden">
			<button
				type="button"
				onclick={() => window.print()}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary-text)]"
			>
				<Download size={16} />
				{t('attestation_guide.download_pdf', 'Export PDF')}
			</button>
			<button
				type="button"
				onclick={openUpdateForm}
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)]"
			>
				<MessageSquarePlus size={16} />
				{t('attestation_guide.add_update', 'Add community update')}
			</button>
		</div>

		{#if updateSubmitted}
			<p class="text-xs print:hidden" style="color: var(--color-success, #16a34a)">
				{t(
					'attestation_guide.update_submitted',
					'Update submitted — it will appear after moderator review.'
				)}
			</p>
		{/if}

		{#if showUpdateForm}
			<div class="space-y-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-4 print:hidden">
				<h3 class="text-sm font-semibold text-[var(--color-text)]">
					{t('attestation_guide.update_title', 'Submit a community update')}
				</h3>
				{#if updateError}
					<p class="text-sm" style="color: var(--color-danger)">{updateError}</p>
				{/if}
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
					<div>
						<label for="upd-step" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
							{t('attestation_guide.step_index', 'Which step?')}
						</label>
						<select
							id="upd-step"
							bind:value={updateStepIndex}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm focus:border-[var(--color-primary)] focus:outline-none"
						>
							<option value="">{t('attestation_guide.step_general', 'General note')}</option>
							{#each (Array.isArray(pathway.pathway_steps) ? pathway.pathway_steps : []) as _s, i (i)}
								<option value={String(i)}>{t('attestation_guide.step_n', 'Step')} {i + 1}</option>
							{/each}
						</select>
					</div>
					<div>
						<label for="upd-fee" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
							{t('attestation_guide.fee_estimate', 'Fee estimate')}
						</label>
						<input
							id="upd-fee"
							type="text"
							bind:value={updateFeeEstimate}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm focus:border-[var(--color-primary)] focus:outline-none"
						/>
					</div>
					<div>
						<label for="upd-turnaround" class="mb-1 block text-xs font-medium text-[var(--color-muted)]">
							{t('attestation_guide.turnaround_days', 'Turnaround (days)')}
						</label>
						<input
							id="upd-turnaround"
							type="number"
							bind:value={updateTurnaroundDays}
							min="0"
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm focus:border-[var(--color-primary)] focus:outline-none"
						/>
					</div>
				</div>
				<BodyField
					id="upd-note"
					label={t('attestation_guide.note', 'Note')}
					body={updateNote}
					onBody={(v) => (updateNote = v)}
					placeholder={t('attestation_guide.note_placeholder', 'Share your experience with this step...')}
					rows={3}
				/>
				<div class="flex items-center gap-2">
					<button
						type="button"
						onclick={submitUpdate}
						disabled={submittingUpdate}
						class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						{submittingUpdate ? t('common.submitting', 'Submitting...') : t('common.submit', 'Submit')}
					</button>
					<button
						type="button"
						onclick={() => (showUpdateForm = false)}
						class="px-4 py-2 text-sm text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
					>
						{t('common.cancel', 'Cancel')}
					</button>
				</div>
			</div>
		{/if}
	</div>
</div>

<style>
	/* §97 PDF export: print renders a clean black-on-white document —
	 * nav/actions/form are hidden via print:hidden on their containers. */
	@media print {
		:global(body) {
			background: #fff !important;
			color: #111 !important;
		}
		:global(header), :global(nav), :global(footer), :global(aside) {
			display: none !important;
		}
	}
</style>
