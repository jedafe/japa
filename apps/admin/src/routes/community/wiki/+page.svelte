<script lang="ts">
	// /community/wiki — admin HITL review queue for wiki page revisions.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { wikiAdminApi, apiRaw } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, XCircle } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type WikiItem = {
		id: string;
		title: string;
		space_slug: string;
		status: string;
		updated_at: string;
		latest_revision_id?: string;
		excerpt?: string;
		// ADR-088 AI generation columns
		ai_generated?: boolean;
		ai_model?: string;
		hitl_reviewed_by?: string;
		ai_generated_at?: string;
	};

	const canManage = $derived(hasPermission('wiki.review'));

	// ── State-action confirmation modal (reject with optional reason) ────────
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<WikiItem | null>(null);
	let showRejectModal = $state(false);
	let rejectReason = $state('');

	function openReject(item: WikiItem) {
		actionTarget = item;
		rejectReason = '';
		showRejectModal = true;
	}

	async function confirmReject() {
		if (!actionTarget || !actionTarget.latest_revision_id) return;
		acting = true;
		pageError = '';
		try {
			const result = await wikiAdminApi.rejectRevision(actionTarget.id, actionTarget.latest_revision_id, { reason: rejectReason });
			if (result.ok) {
				showRejectModal = false;
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

	async function approveRevision(item: WikiItem) {
		if (!item.latest_revision_id) return;
		acting = true;
		pageError = '';
		try {
			const result = await wikiAdminApi.approveRevision(item.id, item.latest_revision_id);
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
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: WikiItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('app.wiki.approve_revision', 'Approve Revision'),
				icon: CheckCircle,
				onclick: approveRevision,
				show: (row: WikiItem) => !!row.latest_revision_id
			});
			acts.push({
				label: t('app.wiki.reject_revision', 'Reject Revision'),
				icon: XCircle,
				variant: 'destructive',
				onclick: openReject,
				show: (row: WikiItem) => !!row.latest_revision_id
			});
		}
		return acts;
	});

	const fields: FieldDef[] = [
		{ key: 'title', label: t('app.wiki.col_title', 'Title'), type: 'text' },
		{ key: 'space_slug', label: t('app.wiki.col_space', 'Space'), type: 'text' },
		{ key: 'status', label: t('app.wiki.col_status', 'Status'), type: 'text' },
		{ key: 'excerpt', label: t('app.wiki.detail_excerpt', 'Excerpt'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.wiki.review_queue_heading', 'Wiki Review Queue'),
		description: t('app.wiki.review_queue_desc', 'Review and approve pending wiki page revisions'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('app.wiki.col_title', 'Title'), sortable: true },
			{ key: 'space_slug', label: t('app.wiki.col_space', 'Space'), sortable: true },
			{ key: 'status', label: t('app.wiki.col_status', 'Status'), sortable: true },
			{
				key: 'updated_at',
				label: t('app.wiki.col_updated', 'Updated'),
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
			if (params.sort) qs.set('sort', params.sort);
			if (params.filter?.dest) qs.set('dest', params.filter.dest);
			const result = await apiRaw<{ data: WikiItem[]; pagination: { total: number } }>(
				`/admin/wiki${qs.toString() ? `?${qs.toString()}` : ''}`
			);
			if (result.ok) {
				const body = result.data as { data: WikiItem[]; pagination: { total: number } };
				const data = body.data ?? [];
				const total = body.pagination?.total ?? data.length;
				const totalPages = Math.max(1, Math.ceil(total / (params.pageSize ?? 25)));
				return { data, total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: WikiView
	});
</script>

<svelte:head>
	<title>{t('app.wiki.review_queue_page_title', 'Wiki Review Queue — JapaConnect Admin')}</title>
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

<!-- ── Reject revision confirmation modal ─────────────────────────────────── -->
<Modal bind:open={showRejectModal} title={t('app.wiki.modal_reject_revision', 'Reject Revision')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.wiki.reject_confirm', 'Are you sure you want to reject the revision for "{title}"?').replace('{title}', actionTarget?.title ?? '')}
		</p>
		<div>
			<label for="reject-reason" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.wiki.reason_optional', 'Reason (optional)')}</label>
			<textarea
				id="reject-reason"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="3"
				bind:value={rejectReason}
				placeholder={t('app.wiki.reject_placeholder', 'Explain why this revision was rejected...')}
			></textarea>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showRejectModal = false)}>{t('app.wiki.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmReject} disabled={acting}>
			{acting ? t('app.wiki.rejecting', 'Rejecting...') : t('app.wiki.reject', 'Reject')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet WikiView({ row }: { row: WikiItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_title', 'Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_space', 'Space')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.space_slug || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.col_updated', 'Updated')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.updated_at ? new Date(row.updated_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wiki.detail_excerpt', 'Excerpt')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.excerpt || '—'}</p>
		</div>
	</div>
{/snippet}
