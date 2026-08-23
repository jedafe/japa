<script lang="ts">
	/**
	 * Generic report dialog for users and other followable/reportable entities.
	 * Ref: japaconnect-engineering/sections/51-content-reports-flagging.md
	 * Reason options are read from the public system setting
	 * `moderation.report_reasons` so they are never hardcoded (Rule 16).
	 */
	import { Dialog } from 'bits-ui';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { reportUser } from '$lib/api/hub';
	import { requireAuthOrRedirect } from '$lib/auth/guest';
	import { Flag, X, Loader2, AlertCircle } from 'lucide-svelte';

	interface Props {
		open?: boolean;
		/** Entity type must match the backend report route. */
		entityType?: 'user' | 'content' | 'comment' | 'space';
		/** Backend ID for the entity being reported. */
		entityId: string;
		/** Human-readable name shown in the dialog title. */
		entityName?: string;
	}

	let {
		open = $bindable(false),
		entityType = 'user',
		entityId,
		entityName
	}: Props = $props();

	/** Reason codes are admin-configurable via system_settings (public). */
	const reportReasons = $derived<string[]>(
		Array.isArray(page.data.siteSettings?.['moderation.report_reasons'])
			? (page.data.siteSettings['moderation.report_reasons'] as string[])
			: []
	);

	let reason = $state('');
	let notes = $state('');
	let status = $state<'idle' | 'submitting' | 'success' | 'error'>('idle');
	let errorMessage = $state('');

	function humanize(code: string): string {
		return code.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
	}

	function reset() {
		reason = '';
		notes = '';
		status = 'idle';
		errorMessage = '';
	}

	async function submit() {
		if (!reason) {
			status = 'error';
			errorMessage = t('report.reason_required', 'Please choose a reason');
			return;
		}
		if (!(await requireAuthOrRedirect())) return;

		status = 'submitting';
		errorMessage = '';

		if (entityType !== 'user') {
			status = 'error';
			errorMessage = t('report.unsupported_entity', 'Reporting is not yet supported for this type');
			return;
		}

		const trimmedNotes = notes.trim();
		const res = await reportUser(entityId, {
			reason,
			notes: trimmedNotes || undefined,
			evidence_urls: []
		});

		if (res.error) {
			status = 'error';
			errorMessage = res.error.message ?? t('report.submit_error', 'Failed to submit report');
		} else {
			status = 'success';
		}
	}

	$effect(() => {
		if (open) {
			reset();
		}
	});
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Content
			class="z-50 flex max-h-[80vh] w-full max-w-md flex-col rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5 shadow-lg"
		>
			<div class="mb-4 flex items-center gap-2">
				<Flag size={18} class="text-[var(--color-primary)]" />
				<Dialog.Title class="text-base font-medium text-[var(--color-text)]">
					{t('report.title', `Report ${entityName ?? ''}`.trim())}
				</Dialog.Title>
				<Dialog.Close
					class="ml-auto grid h-7 w-7 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					<X size={16} />
				</Dialog.Close>
			</div>

			{#if status === 'success'}
				<div class="rounded-[var(--radius)] border border-emerald-500/30 bg-emerald-500/10 p-4 text-sm text-emerald-700 dark:text-emerald-300">
					{t('report.success', 'Report submitted. Our moderation team will review it.')}
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
						<select
							bind:value={reason}
							class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
						>
							<option value="">{t('report.select_reason', 'Select a reason')}</option>
							{#each reportReasons as r (r)}
								<option value={r}>
									{t(`report.reason.${r}`, humanize(r))}
								</option>
							{/each}
						</select>
					</label>
				{/if}

				<label class="mb-4 block">
					<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
						{t('report.notes_label', 'Additional notes')}
					</span>
					<textarea
						bind:value={notes}
						rows={4}
						placeholder={t('report.notes_placeholder', 'Add any details that may help our moderation team...')}
						class="w-full resize-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
					></textarea>
				</label>

				{#if status === 'error'}
					<div class="mb-4 flex items-start gap-2 rounded-[var(--radius)] border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-700 dark:text-red-300">
						<AlertCircle size={16} class="mt-0.5 shrink-0" />
						<span>{errorMessage}</span>
					</div>
				{/if}

				<div class="flex items-center justify-end gap-2">
					<Dialog.Close
						class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						{t('common.cancel', 'Cancel')}
					</Dialog.Close>
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
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
