<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	/**
	 * Generic report dialog for users and other reportable entities.
	 * Ref: japaconnect-engineering/sections/51-content-reports-flagging.md
	 * Reason options are read from the public system setting
	 * `moderation.report_reasons` so they are never hardcoded (Rule 16) — the
	 * backend validates against the same setting.
	 */
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { reportContent, reportComment, reportSpace, reportUser, reportSource, reportDMMessage, reportForumReply } from '$lib/api/hub';
	import { requireAuthOrRedirect } from '$lib/auth/guest';
	import { Flag, Loader2, AlertCircle } from 'lucide-svelte';
	import { AutocompleteSelect, Modal } from '$lib/components/ui';

	interface Props {
		open?: boolean;
		/** Entity type must match the backend report route. */
		entityType?: 'user' | 'content' | 'comment' | 'space' | 'source' | 'dm_message' | 'forum_reply';
		/** Backend ID (or slug for sources) for the entity being reported. */
		entityId: string;
		/** Human-readable name shown in the dialog title. */
		entityName?: string;
		/** Required for dm_message — the conversation that owns the message. */
		dmConversationId?: string;
		/** Fired after a report is accepted — lets callers persist a
		 *  "reported" state on their affordance. */
		onSubmitted?: () => void;
	}

	let {
		open = $bindable(false),
		entityType = 'user',
		entityId,
		entityName,
		dmConversationId,
		onSubmitted
	}: Props = $props();

	/** Reason codes are admin-configurable via system_settings (public). */
	const reportReasons = $derived<string[]>(
		Array.isArray(page.data.siteSettings?.['moderation.report_reasons'])
			? (page.data.siteSettings['moderation.report_reasons'] as string[])
			: []
	);

	const NOTES_MAX = 5000;
	const MAX_EVIDENCE = 10;

	let reason = $state('');
	let notes = $state('');
	let evidenceInput = $state('');
	let status = $state<'idle' | 'submitting' | 'success' | 'error'>('idle');
	let errorMessage = $state('');

	function humanize(code: string): string {
		return code.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}

	function reset() {
		reason = '';
		notes = '';
		evidenceInput = '';
		status = 'idle';
		errorMessage = '';
	}

	/** Parses the evidence textarea into http(s) URLs, one per line. */
	function parseEvidence(): { urls: string[]; invalid: boolean } {
		const urls = evidenceInput
			.split('\n')
			.map((l) => l.trim())
			.filter(Boolean);
		const invalid = urls.some((u) => !/^https?:\/\/\S+$/i.test(u));
		return { urls: urls.slice(0, MAX_EVIDENCE), invalid };
	}

	async function submit() {
		if (!reason) {
			status = 'error';
			errorMessage = t('report.reason_required', 'Please choose a reason');
			return;
		}
		const trimmedNotes = notes.trim();
		if (reason === 'other' && !trimmedNotes) {
			status = 'error';
			errorMessage = t('report.other_notes_required', 'Please describe the issue in the notes');
			return;
		}
		const { urls, invalid } = parseEvidence();
		if (invalid) {
			status = 'error';
			errorMessage = t('report.evidence_invalid', 'Evidence links must be valid http(s) URLs');
			return;
		}
		if (!(await requireAuthOrRedirect())) return;

		status = 'submitting';
		errorMessage = '';

		const body = {
			reason,
			notes: trimmedNotes || undefined,
			evidence_urls: urls
		};

		let res;
		switch (entityType) {
			case 'content':
				res = await reportContent(entityId, body);
				break;
			case 'comment':
				res = await reportComment(entityId, body);
				break;
			case 'space':
				res = await reportSpace(entityId, body);
				break;
			case 'source':
				res = await reportSource(entityId, body);
				break;
			case 'user':
				res = await reportUser(entityId, body);
				break;
			case 'forum_reply':
				res = await reportForumReply(entityId, body);
				break;
			case 'dm_message':
				if (!dmConversationId) {
					res = { error: { message: t('report.unsupported_entity', 'Reporting is not yet supported for this type') } } as any;
				} else {
					res = await reportDMMessage(dmConversationId, entityId, body);
				}
				break;
			default:
				res = { error: { message: t('report.unsupported_entity', 'Reporting is not yet supported for this type') } } as any;
				break;
		}

		if (res.error) {
			status = 'error';
			errorMessage = res.error.message ?? t('report.submit_error', 'Failed to submit report');
		} else {
			status = 'success';
			onSubmitted?.();
		}
	}

	$effect(() => {
		if (open) {
			reset();
		}
	});
</script>

<Modal bind:open title={t('report.title', 'Report {entity}').replace('{entity}', entityName ?? t('report.this_entity', 'this item')).trim()} icon={Flag} size="md">
	{#snippet children(close)}
		{#if status === 'success'}
				<div class="rounded-[var(--radius)] border border-emerald-500/30 bg-emerald-500/10 p-4 text-sm text-emerald-700 dark:text-emerald-300">
					{t('report.success', 'Report submitted. Our moderation team will review it.')}
				</div>
				<div class="mt-4 flex justify-end">
					<button
						type="button"
						onclick={close}
						class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
					>
						{t('common.close', 'Close')}
					</button>
				</div>
			{:else}
				{#if reportReasons.length === 0}
					<div class="mb-3 rounded-[var(--radius)] border border-amber-500/30 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-300">
						{t('report.no_reasons', 'Report reasons are not configured. Please try again later.')}
					</div>
				{:else}
					<label class="mb-3 block">
						<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
							{t('report.reason_label', 'Reason')}
							<span class="text-[var(--color-error)]">*</span>
						</span>
						<AutocompleteSelect
							bind:value={reason}
							options={[
								{ value: '', label: t('report.select_reason', 'Select a reason') },
								...reportReasons.map((r) => ({ value: r, label: t(`report.reason.${r}`, humanize(r)) }))
							]}
						/>
					</label>
				{/if}

				<label class="mb-3 block">
					<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('report.notes_label', 'Additional notes')}
						{#if reason === 'other'}<span class="text-[var(--color-error)]">*</span>{/if}
					</span>
					<textarea
						bind:value={notes}
						rows={4}
						maxlength={NOTES_MAX}
						placeholder={t('report.notes_placeholder', 'Add any details that may help our moderation team...')}
						aria-describedby="report-notes-hint"
						class="w-full resize-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
					></textarea>
					<HintIcon id="report-notes-hint" text={t('report.notes_hint', 'Include links or details that help our moderation team review faster.')} />
				</label>

				<label class="mb-4 block">
					<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('report.evidence_label', 'Evidence links (optional)')}
					</span>
					<textarea
						bind:value={evidenceInput}
						rows={2}
						placeholder={t('report.evidence_placeholder', 'https://… — one link per line')}
						aria-describedby="report-evidence-hint"
						class="w-full resize-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
					></textarea>
					<HintIcon id="report-evidence-hint" text={t('report.evidence_hint', 'URLs to screenshots, listings or profiles — one per line.')} />
				</label>

				{#if status === 'error'}
					<div class="mb-4 flex items-start gap-2 rounded-[var(--radius)] border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-700 dark:text-red-300">
						<AlertCircle size={16} class="mt-0.5 shrink-0" />
						<span>{errorMessage}</span>
					</div>
				{/if}

				<div class="flex items-center justify-end gap-2">
					<button
						type="button"
						onclick={close}
						class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						{t('common.cancel', 'Cancel')}
					</button>
					<button
						type="button"
						onclick={submit}
						disabled={status === 'submitting' || reportReasons.length === 0}
						class="flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						{#if status === 'submitting'}
							<Loader2 size={14} class="animate-spin" />
						{/if}
						{t('report.submit', 'Submit report')}
					</button>
				</div>
		{/if}
	{/snippet}
</Modal>
