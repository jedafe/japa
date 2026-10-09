<script lang="ts">
	// §22 Appeals — reusable appeal submission control. Mount wherever a
	// rejected/appealable decision is surfaced (service claims, ownership
	// claims, video channel claims, moderation decisions). Submits
	// POST /api/v1/appeals which records the moderation_appeal and opens a
	// linked 'appeal' ticket the user can track from /dashboard/tickets.
	//
	// Ref: japaconnect-engineering/sections/22-admin-panel.md (Appeals)
	import { Modal } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { AlertCircle, CheckCircle2, Loader2, Scale } from 'lucide-svelte';

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
		/** Button label override. */
		label?: string;
		/** Extra classes merged onto the trigger button. */
		class?: string;
	}
	let { entityType, entityId, label, class: cls = '' }: Props = $props();

	let open = $state(false);
	let reason = $state('');
	let submitting = $state(false);
	let error = $state('');
	let done = $state<AppealResult | null>(null);

	async function submit() {
		if (submitting) return;
		if (!reason.trim()) {
			error = t('appeals.reason_required', 'Please explain why the decision should be reconsidered.');
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
		} else if (res.error?.code === 'duplicate_appeal') {
			error = t('appeals.duplicate', 'You already have a pending appeal for this decision.');
		} else {
			error = res.error?.message ?? t('appeals.failed', 'Failed to submit appeal.');
		}
	}
</script>

<button
	type="button"
	onclick={() => {
		done = null;
		error = '';
		open = true;
	}}
	class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] {cls}"
>
	<Scale size={15} aria-hidden="true" />
	{label ?? t('appeals.appeal', 'Appeal')}
</button>

<Modal bind:open title={t('appeals.dialog_title', 'Appeal this decision')} icon={Scale} size="sm">
	{#snippet children()}
		{#if done}
			<div class="flex flex-col items-center gap-3 py-2 text-center">
				<CheckCircle2 size={32} class="text-[var(--color-success)]" aria-hidden="true" />
				<p class="text-sm text-[var(--color-text)]">
					{t('appeals.submitted', 'Appeal submitted — our team will review it.')}
				</p>
				{#if done.ticket_id}
					<a
						href="/dashboard/tickets/{done.ticket_number ? `JC-${done.ticket_number}` : done.ticket_id}"
						class="text-sm text-[var(--color-primary)] hover:underline"
					>
						{t('appeals.track_ticket', 'Track it in your support tickets')}
					</a>
				{/if}
			</div>
		{:else}
			<form
				onsubmit={(e) => {
					e.preventDefault();
					void submit();
				}}
				class="space-y-3"
			>
				<div class="flex flex-col gap-1.5">
					<label for="appeal-reason" class="text-sm font-medium text-[var(--color-text)]">
						{t('appeals.reason_label', 'Why should this decision be reconsidered?')}
					</label>
					<textarea
						id="appeal-reason"
						rows="4"
						bind:value={reason}
						placeholder={t(
							'appeals.reason_placeholder',
							'Explain briefly why the decision was wrong — links or reference details help the review.'
						)}
						class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
					></textarea>
				</div>

				{#if error}
					<p role="alert" class="flex items-center gap-1.5 text-sm text-[var(--color-danger)]">
						<AlertCircle size={14} aria-hidden="true" /> {error}
					</p>
				{/if}

				<div class="flex justify-end">
					<button
						type="submit"
						disabled={submitting}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						{#if submitting}
							<Loader2 size={15} class="motion-safe:animate-spin" aria-hidden="true" />
						{:else}
							<Scale size={15} aria-hidden="true" />
						{/if}
						{t('appeals.submit', 'Submit appeal')}
					</button>
				</div>
			</form>
		{/if}
	{/snippet}
</Modal>
