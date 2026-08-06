<script lang="ts">
	// /blog/review-queue — migrated to the EntityPage scaffold; read-only list with Approve/Reject state actions + View-Live external link.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { blogAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, XCircle, ExternalLink } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type ReviewItem = {
		id: string;
		title: string;
		slug: string;
		author_name: string;
		author_id: string;
		status: string;
		submitted_at: string;
		excerpt?: string;
		url?: string;
	};

	const canManage = $derived(hasPermission('blog.review.manage'));

	// ── State-action confirmation modal (reject with optional reason) ────────
	// EntityPage's confirm Modal only covers default delete; reject keeps a page-local Modal with a reason field, confirm calls API + controller.reload().
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<ReviewItem | null>(null);
	let showRejectModal = $state(false);
	let rejectReason = $state('');

	function openReject(item: ReviewItem) {
		actionTarget = item;
		rejectReason = '';
		showRejectModal = true;
	}

	async function confirmReject() {
		if (!actionTarget) return;
		acting = true;
		pageError = '';
		try {
			const result = await blogAdminApi.rejectPost(actionTarget.id, { reason: rejectReason });
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

	async function approvePost(item: ReviewItem) {
		acting = true;
		pageError = '';
		try {
			const result = await blogAdminApi.approvePost(item.id);
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

	function openExternal(item: ReviewItem) {
		// BUG-010: backend ListPendingReviewSummaries doesn't return url; derive from slug so "View Live" always works.
		if (item.slug) {
			window.open(`/blog/${item.slug}`, '_blank');
		}
	}

	// ── Row actions ─────────────────────────────────────────────────────────
	// Custom actions replace defaults; View opens the slide-over, Approve/Reject are state actions, View Live opens the public post.
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row: ReviewItem, ctx) => ctx?.openView(_row) }
		];
		if (canManage) {
			acts.push({
				label: t('app.blog.approve', 'Approve'),
				icon: CheckCircle,
				onclick: approvePost,
				show: () => true
			});
			acts.push({
				label: t('app.blog.reject', 'Reject'),
				icon: XCircle,
				variant: 'destructive',
				onclick: openReject,
				show: () => true
			});
		}
		acts.push({
			label: t('app.blog.view_live', 'View Live'),
			icon: ExternalLink,
			onclick: openExternal,
			show: (row: ReviewItem) => !!row.slug
		});
		return acts;
	});

	// `fields` is required by EntityPageConfig; readonly page so the form is never shown, but kept for shape completeness.
	const fields: FieldDef[] = [
		{ key: 'title', label: t('app.blog.col_title', 'Title'), type: 'text' },
		{ key: 'slug', label: t('app.blog.col_slug', 'Slug'), type: 'text' },
		{ key: 'author_name', label: t('app.blog.col_author', 'Author'), type: 'text' },
		{ key: 'excerpt', label: t('app.blog.detail_excerpt', 'Excerpt'), type: 'textarea' }
	];

	const config = $derived<EntityPageConfig>({
		title: t('app.blog.review_queue_heading', 'Blog Review Queue'),
		description: t('app.blog.review_queue_desc', 'Review and approve pending blog submissions'),
		readonly: true,
		pageSize: 25,
		columns: [
			{ key: 'title', label: t('app.blog.col_title', 'Title'), sortable: true },
			{
				key: 'author_name',
				label: t('app.blog.col_author', 'Author'),
				sortable: true,
				render: (value: string) => value || t('app.blog.unknown', 'Unknown')
			},
			{
				key: 'submitted_at',
				label: t('app.blog.review_submitted_at', 'Submitted'),
				sortable: true,
				render: (value: string) => (value ? new Date(value).toLocaleDateString() : '—')
			},
			{
				key: 'excerpt',
				label: t('app.blog.detail_excerpt', 'Excerpt'),
				render: (value: string) => {
					if (!value) return '—';
					return value.length > 80 ? value.substring(0, 80) + '…' : value;
				}
			}
		],
		actions: rowActions,
		fields,
		load: async () => {
			const result = await blogAdminApi.getReviewQueue();
			if (result.ok) {
				const data = (result.data as ReviewItem[]) ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'An error occurred'));
		},
		viewSnippet: ReviewView
	});
</script>

<svelte:head>
	<title>{t('app.blog.review_queue_page_title', 'Blog Review Queue — JapaConnect Admin')}</title>
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

<!-- ── Reject confirmation modal ─────────────────────────────────────────── -->
<Modal bind:open={showRejectModal} title={t('app.blog.modal_reject_post', 'Reject Post')}>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('app.blog.reject_confirm', 'Are you sure you want to reject "{title}"? The author will be notified.').replace('{title}', actionTarget?.title ?? '')}
		</p>
		<div>
			<label for="reject-reason" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">{t('app.blog.reason_optional', 'Reason (optional)')}</label>
			<textarea
				id="reject-reason"
				class="w-full rounded-lg border p-3 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="3"
				bind:value={rejectReason}
				placeholder={t('app.blog.reject_placeholder', 'Explain why this post was rejected...')}
			></textarea>
		</div>
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showRejectModal = false)}>{t('app.blog.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={confirmReject} disabled={acting}>
			{acting ? t('app.blog.rejecting', 'Rejecting...') : t('app.blog.reject', 'Reject')}
		</Button>
	</div>
</Modal>

<!-- ── View slide-over body ──────────────────────────────────────────────── -->
{#snippet ReviewView({ row }: { row: ReviewItem })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.col_title', 'Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.col_author', 'Author')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.author_name || t('app.blog.unknown', 'Unknown')}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.review_submitted_at', 'Submitted')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.submitted_at ? new Date(row.submitted_at).toLocaleString() : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.col_slug', 'Slug')}</p>
			<p class="mt-1 break-all text-sm font-mono" style="color: var(--color-foreground)">{row.slug || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.blog.detail_excerpt', 'Excerpt')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.excerpt || '—'}</p>
		</div>
	</div>
{/snippet}
