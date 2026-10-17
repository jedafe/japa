<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /roadmap/submissions — pending community submissions triage queue (§113).
	// Readonly EntityPage; Approve/Decline drive the lifecycle state machine
	// (pending → under_review | declined) via PATCH .../status.
	import { EntityPage, type EntityPageConfig, type EntityPageController } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { roadmapAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Check, X } from 'lucide-svelte';
	import type { ActionItem, ActionContext } from '$lib/components/ui/data-table';

	type RoadmapSubmission = {
		id: string;
		title: string;
		slug: string;
		summary: string | null;
		body: string | null;
		category_id: string | null;
		category_name: string | null;
		status: string;
		vote_count: number;
		submitted_by: string | null;
		created_at: string;
	};

	const canModerate = $derived(hasPermission('roadmap.moderate'));

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);
	let actionTarget = $state<RoadmapSubmission | null>(null);
	let showDeclineConfirm = $state(false);

	function openDecline(row: RoadmapSubmission) {
		actionTarget = row;
		showDeclineConfirm = true;
	}

	async function approve(row: RoadmapSubmission) {
		acting = true;
		try {
			// pending → under_review publishes the item on the public board.
			const result = await roadmapAdminApi.setStatus(row.id, 'under_review');
			if (result.ok) {
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'Failed to approve submission');
			}
		} catch {
			pageError = t('common.error', 'Failed to approve submission');
		} finally {
			acting = false;
		}
	}

	async function confirmDecline() {
		if (!actionTarget) return;
		acting = true;
		try {
			const result = await roadmapAdminApi.setStatus(actionTarget.id, 'declined');
			if (result.ok) {
				showDeclineConfirm = false;
				entityController?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'Failed to decline submission');
			}
		} catch {
			pageError = t('common.error', 'Failed to decline submission');
		} finally {
			acting = false;
		}
	}

	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: RoadmapSubmission, ctx?: ActionContext) => ctx?.openView(row) }
		];
		if (canModerate) {
			acts.push({
				label: t('roadmap.approve', 'Approve'),
				icon: Check,
				onclick: approve,
				show: (row: RoadmapSubmission) => row.status === 'pending'
			});
			acts.push({
				label: t('roadmap.decline', 'Decline'),
				icon: X,
				variant: 'destructive',
				onclick: openDecline,
				show: (row: RoadmapSubmission) => row.status === 'pending'
			});
		}
		return acts;
	});

	const config = $derived<EntityPageConfig>({
		title: t('roadmap.submissions', 'Roadmap Submissions'),
		description: t('roadmap.submissions_description', 'Triage community-submitted feature requests.'),
		readonly: true,
		pageSize: 25,
		columns: [
			{
				key: 'title',
				label: t('roadmap.title', 'Title'),
				render: (v: string) =>
					`<span class="font-medium" style="color: var(--color-primary)">${v}</span>`
			},
			{
				key: 'category_name',
				label: t('roadmap.category', 'Category'),
				render: (v: string | null) =>
					v ?? `<span style="color: var(--color-muted)">—</span>`
			},
			{ key: 'vote_count', label: t('roadmap.votes', 'Votes'), sortable: true, sortKey: 'votes' },
			{
				key: 'created_at',
				label: t('common.created', 'Created'),
				sortable: true,
				sortKey: 'recent',
				render: (value: string) => new Date(value).toLocaleDateString()
			}
		],
		actions: rowActions,
		fields: [],
		load: async ({ page, pageSize, q, sort }) => {
			// The backend hardcodes status=pending / include_deleted=false for
			// this endpoint; search is `q`, sort keywords are recent|votes|title.
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (q) params.set('q', q);
			if (sort) {
				const [key] = sort.split(':');
				params.set('sort', key);
			}
			const result = await roadmapAdminApi.submissions(params.toString());
			if (result.ok) {
				const d = result.data as { items: RoadmapSubmission[]; total: number; total_pages: number };
				return { data: d.items ?? [], total: d.total ?? 0, totalPages: d.total_pages ?? 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load submissions'));
		},
		viewSnippet: SubmissionView
	});
</script>

<svelte:head>
	<title>{t('roadmap.submissions', 'Roadmap Submissions')} — {siteName()} Admin</title>
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

<!-- Decline confirm -->
<Modal bind:open={showDeclineConfirm} title={t('roadmap.decline_submission', 'Decline Submission')}>
	{#if actionTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('roadmap.decline_confirm', 'Are you sure you want to decline')} <strong>{actionTarget.title}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			{t('roadmap.decline_hint', 'Declined submissions are hidden from the public board.')}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeclineConfirm = false)} disabled={acting}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={confirmDecline} disabled={acting}>
			{t('roadmap.decline', 'Decline')}
		</Button>
	</div>
</Modal>

<!-- View slide-over body -->
{#snippet SubmissionView({ row }: { row: RoadmapSubmission })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('roadmap.title', 'Title')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.title}</p>
		</div>
		{#if row.summary}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('roadmap.summary', 'Summary')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.summary}</p>
			</div>
		{/if}
		{#if row.body}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('roadmap.body', 'Body')}</p>
				<p class="mt-1 whitespace-pre-wrap text-sm" style="color: var(--color-foreground)">{row.body}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('roadmap.category', 'Category')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.category_name ?? '—'}</p>
		</div>
		{#if row.submitted_by}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('roadmap.submitted_by', 'Submitted By')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.submitted_by}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.created', 'Created')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(row.created_at).toLocaleString()}</p>
		</div>
	</div>
{/snippet}
