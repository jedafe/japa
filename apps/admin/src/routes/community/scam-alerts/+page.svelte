<script lang="ts">
	// /community/scam-alerts — admin HITL review queue for Scam Alerts with
	// confirm/debunk/duplicate review + pin/unpin.
	// Ref: japaconnect-engineering/sections/81-scam-alert-board.md
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { scamAlertAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Pin, PinOff, CheckCircle, XCircle, Copy } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type ScamAlertItem = {
		id: string;
		scam_type: string;
		status: string;
		destination_space_name?: string;
		origin_space_name?: string;
		origin_community?: string;
		platform_used?: string;
		approximate_date?: string;
		amount_range?: string;
		description: string;
		reported_to_authorities: boolean;
		moderator_note?: string;
		is_pinned: boolean;
		reviewer_name?: string;
		created_at: string;
		updated_at?: string;
		// ADR-088 AI generation columns
		ai_generated?: boolean;
		ai_model?: string;
		hitl_reviewed_by?: string;
		ai_generated_at?: string;
	};

	const canReview = $derived(hasPermission('scam_alert.review'));
	const canPin = $derived(hasPermission('scam_alert.pin'));

	// ── State ───────────────────────────────────────────────────────────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);

	// Review slide-over state
	let reviewOpen = $state(false);
	let reviewTarget = $state<ScamAlertItem | undefined>(undefined);
	let reviewStatus = $state<'confirmed' | 'debunked' | 'duplicate'>('confirmed');
	let moderatorNote = $state('');
	let reviewSubmitting = $state(false);

	function openReview(item: ScamAlertItem) {
		reviewTarget = item;
		reviewStatus = 'confirmed';
		moderatorNote = item.moderator_note ?? '';
		reviewOpen = true;
	}

	async function submitReview() {
		if (!reviewTarget || reviewSubmitting) return;
		reviewSubmitting = true;
		pageError = '';
		try {
			const result = await scamAlertAdminApi.update(reviewTarget.id, {
				status: reviewStatus,
				moderator_note: moderatorNote.trim()
			});
			if (result.ok) {
				reviewOpen = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			reviewSubmitting = false;
		}
	}

	async function pinAlert(item: ScamAlertItem) {
		acting = true;
		pageError = '';
		try {
			const result = await scamAlertAdminApi.pin(item.id);
			if (result.ok) {
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	async function unpinAlert(item: ScamAlertItem) {
		acting = true;
		pageError = '';
		try {
			const result = await scamAlertAdminApi.unpin(item.id);
			if (result.ok) {
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'An error occurred');
			}
		} catch {
			pageError = t('common.error', 'An error occurred');
		} finally {
			acting = false;
		}
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: ScamAlertItem, ctx) => ctx?.openView(_row) }
		];
		if (canReview) {
			acts.push({
				label: t('app.scam_alert.review', 'Review'),
				icon: CheckCircle,
				onclick: openReview,
				show: (row: ScamAlertItem) => row.status === 'under_review'
			});
		}
		if (canPin) {
			acts.push({
				label: t('app.scam_alert.pin', 'Pin'),
				icon: Pin,
				onclick: pinAlert,
				show: (row: ScamAlertItem) => !row.is_pinned && row.status === 'confirmed'
			});
			acts.push({
				label: t('app.scam_alert.unpin', 'Unpin'),
				icon: PinOff,
				onclick: unpinAlert,
				show: (row: ScamAlertItem) => !!row.is_pinned
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'scam_type', label: t('app.scam_alert.col_scam_type', 'Scam Type'), type: 'text' },
		{ key: 'status', label: t('app.scam_alert.col_status', 'Status'), type: 'text' },
		{ key: 'description', label: t('app.scam_alert.detail_description', 'Description'), type: 'textarea' },
		{ key: 'destination_space_name', label: t('app.scam_alert.col_destination', 'Destination'), type: 'text' },
		{ key: 'moderator_note', label: t('app.scam_alert.col_moderator_note', 'Moderator Note'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.scam_alert.review_queue_heading', 'Scam Alerts Review Queue'),
		description: t('app.scam_alert.review_queue_desc', 'Review and pin scam alerts for the community'),
		readonly: true,
		pageSize: 25,
		columns: [
			{
				key: 'scam_type',
				label: t('app.scam_alert.col_scam_type', 'Scam Type'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'status',
				label: t('app.scam_alert.col_status', 'Status'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'destination_space_name',
				label: t('app.scam_alert.col_destination', 'Destination'),
				sortable: true,
				render: (value: string) => value || '—'
			},
			{
				key: 'created_at',
				label: t('app.scam_alert.col_created_at', 'Created'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async (params) => {
			const qs = new URLSearchParams();
			if (params.page) qs.set('page', String(params.page));
			if (params.pageSize) qs.set('limit', String(params.pageSize));
			if (params.q) qs.set('q', params.q);
			if (params.filter?.dest) qs.set('dest', params.filter.dest);
			const result = await scamAlertAdminApi.listRaw(qs.toString());
			if (result.ok) {
				const data = (result.data.data as ScamAlertItem[]) ?? [];
				const total = result.data.pagination?.total ?? data.length;
				const totalPages = params.pageSize
					? Math.max(1, Math.ceil(total / params.pageSize))
					: 1;
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: ScamAlertView
	});
</script>

<svelte:head>
	<title>{t('app.scam_alert.review_queue_page_title', 'Scam Alerts Review Queue — JapaConnect Admin')}</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── Review slide-over ─────────────────────────────────────────────────── -->
{#if reviewOpen && reviewTarget}
	<div class="fixed inset-0 z-50 flex justify-end">
		<!-- backdrop -->
		<button
			type="button"
			class="absolute inset-0 bg-black/40"
			aria-label={t('common.close', 'Close')}
			onclick={() => (reviewOpen = false)}
		></button>
		<!-- panel -->
		<div class="relative flex h-full w-full max-w-md flex-col bg-[var(--color-background)] shadow-xl">
			<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
				<h2 class="text-sm font-semibold" style="color: var(--color-foreground)">
					{t('app.scam_alert.review_title', 'Review Scam Report')}
				</h2>
				<button
					type="button"
					class="text-[var(--color-muted)] hover:text-[var(--color-foreground)]"
					onclick={() => (reviewOpen = false)}
					aria-label={t('common.close', 'Close')}
				>
					✕
				</button>
			</div>

			<div class="flex-1 space-y-4 overflow-y-auto p-4">
				<!-- Report summary -->
				<div class="space-y-2 rounded-lg border border-[var(--color-border)] p-3 text-sm">
					<div>
						<span class="font-medium" style="color: var(--color-foreground)">{reviewTarget.scam_type}</span>
						<span class="ml-2 text-[var(--color-muted)]">· {reviewTarget.status}</span>
					</div>
					{#if reviewTarget.destination_space_name}
						<div class="text-[var(--color-muted)]">
							{t('app.scam_alert.col_destination', 'Destination')}: {reviewTarget.destination_space_name}
						</div>
					{/if}
					<p class="whitespace-pre-wrap" style="color: var(--color-foreground)">{reviewTarget.description}</p>
				</div>

				<!-- Status selector -->
				<div class="flex flex-col gap-1.5">
					<label for="review-status" class="text-sm font-medium" style="color: var(--color-foreground)">
						{t('app.scam_alert.review_status', 'Decision')} <span class="text-red-500">*</span>
					</label>
					<select
						id="review-status"
						value={reviewStatus}
						onchange={(e) => (reviewStatus = e.currentTarget.value as 'confirmed' | 'debunked' | 'duplicate')}
						class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] px-3 py-2 text-sm"
						style="color: var(--color-foreground)"
					>
						<option value="confirmed">
							{t('app.scam_alert.status_confirmed', 'Confirmed — verified scam')}
						</option>
						<option value="debunked">
							{t('app.scam_alert.status_debunked', 'Debunked — not a scam')}
						</option>
						<option value="duplicate">
							{t('app.scam_alert.status_duplicate', 'Duplicate — already reported')}
						</option>
					</select>
				</div>

				<!-- Moderator note -->
				<div class="flex flex-col gap-1.5">
					<label for="moderator-note" class="text-sm font-medium" style="color: var(--color-foreground)">
						{t('app.scam_alert.col_moderator_note', 'Moderator Note')}
					</label>
					<textarea
						id="moderator-note"
						rows={4}
						value={moderatorNote}
						oninput={(e) => (moderatorNote = e.currentTarget.value)}
						placeholder={t('app.scam_alert.moderator_note_placeholder', 'Required for confirmed reports per §81 legal boundary')}
						class="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-background)] px-3 py-2 text-sm"
						style="color: var(--color-foreground)"
					></textarea>
				</div>
			</div>

			<div class="flex items-center justify-end gap-2 border-t border-[var(--color-border)] px-4 py-3">
				<button
					type="button"
					class="rounded-lg border border-[var(--color-border)] px-4 py-2 text-sm font-medium"
					style="color: var(--color-foreground)"
					onclick={() => (reviewOpen = false)}
				>
					{t('common.cancel', 'Cancel')}
				</button>
				<button
					type="button"
					disabled={reviewSubmitting}
					onclick={submitReview}
					class="rounded-lg bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] disabled:opacity-50"
				>
					{reviewSubmitting
						? t('common.submitting', 'Submitting…')
						: t('app.scam_alert.submit_review', 'Submit Review')}
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet ScamAlertView({ row }: { row: ScamAlertItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.scam_alert.col_scam_type', 'Scam Type')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.scam_type || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.scam_alert.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.scam_alert.col_destination', 'Destination')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.destination_space_name || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.scam_alert.col_created_at', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.scam_alert.detail_pinned', 'Pinned')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_pinned ? t('common.yes', 'Yes') : t('common.no', 'No')}</p>
		</div>
		{#if row.origin_space_name}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.scam_alert.col_origin', 'Origin')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.origin_space_name}</p>
			</div>
		{/if}
		{#if row.platform_used}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.scam_alert.col_platform', 'Platform')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.platform_used}</p>
			</div>
		{/if}
		{#if row.amount_range}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.scam_alert.col_amount', 'Amount Range')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.amount_range}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.scam_alert.detail_description', 'Description')}</p>
			<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.description || '—'}</p>
		</div>
		{#if row.moderator_note}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.scam_alert.col_moderator_note', 'Moderator Note')}</p>
				<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.moderator_note}</p>
			</div>
		{/if}
	</div>
{/snippet}
