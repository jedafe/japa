<script lang="ts">
	// /mentorship/applications — §102 member mentor applications review queue.
	// Ref: japaconnect-engineering/sections/102-mentorship-matching.md (2026-11-28 amendment)
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '#lib/components/entity/index.js';
	import { mentorshipAdminApiExtended as mentorshipApi } from '#lib/api.js';
	import { Modal } from '#lib/components/ui/modal/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { t } from '#lib/stores/i18n.svelte.js';
	import { hasPermission } from '#lib/stores/permissions.svelte.js';
	import type { ActionItem } from '#lib/components/ui/data-table/index.js';

	// §102 mentor_applications row + the list endpoint's applicant join.
	type Application = {
		id: string;
		user_id: string;
		username?: string;
		display_name?: string;
		status: string;
		motivation?: string;
		mentor_areas?: string[];
		reviewer_user_id?: string;
		reviewed_at?: string;
		decline_reason?: string;
		created_at: string;
		updated_at: string;
	};

	const APPLICATION_STATUSES = ['pending', 'approved', 'declined'] as const;

	// Custom row actions bypass EntityPage's permission gates (they render
	// config.actions verbatim) — the review actions gate themselves per-row.
	const canManage = $derived(hasPermission('mentorship.manage'));

	let controller = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let acting = $state(false);

	// Decline carries an optional reason the member sees on resubmission —
	// it needs a small form, so it gets a modal (approve fires directly).
	let declineTarget = $state<Application | null>(null);
	let declineReason = $state('');
	let showDeclineModal = $state(false);

	async function approve(row: Application) {
		acting = true;
		pageError = '';
		const result = await mentorshipApi.approveApplication(row.id);
		acting = false;
		if (result.ok) {
			controller?.reload();
		} else {
			pageError = result.error ?? t('app.mentorship.applications.error.approve_failed', 'Failed to approve application');
		}
	}

	function openDecline(row: Application) {
		declineTarget = row;
		declineReason = '';
		showDeclineModal = true;
	}

	async function confirmDecline() {
		const row = declineTarget;
		if (!row) return;
		acting = true;
		pageError = '';
		const result = await mentorshipApi.declineApplication(row.id, declineReason.trim() || undefined);
		acting = false;
		if (result.ok) {
			declineTarget = null;
			showDeclineModal = false;
			controller?.reload();
		} else {
			pageError = result.error ?? t('app.mentorship.applications.error.decline_failed', 'Failed to decline application');
		}
	}

	const fields = $derived<FieldDef[]>([
		{
			key: 'status',
			label: t('app.mentorship.columns.status', 'Status'),
			type: 'select',
			options: APPLICATION_STATUSES.map((s) => ({ value: s, label: s }))
		}
	]);

	const config = $derived<EntityPageConfig>({
		title: t('app.mentorship.applications.title', 'Mentor Applications'),
		description: t(
			'app.mentorship.applications.description',
			'Review member applications to become a mentor — approval grants mentor capability'
		),
		// Applications are submitted by members via POST /me/mentor-application —
		// admins only review; no admin create/edit/delete routes exist.
		hideCreate: true,
		bulkDelete: false,
		createPermission: 'mentorship.manage',
		readonly: !canManage,
		pageSize: 15,
		pageSizeOptions: [10, 15, 25, 50],
		columns: [
			{
				key: 'username',
				label: t('app.mentorship.applications.columns.applicant', 'Applicant'),
				render: (_: any, row: Application) =>
					`<span class="font-medium text-foreground">${row.username ? '@' + row.username : row.user_id}</span>${
						row.display_name ? ` <span class="text-muted">${row.display_name}</span>` : ''
					}`
			},
			{
				key: 'status',
				label: t('app.mentorship.columns.status', 'Status'),
				render: (v: string) => `<span class="font-medium">${v}</span>`
			},
			{
				key: 'mentor_areas',
				label: t('app.mentorship.applications.columns.areas', 'Areas'),
				render: (v: string[] | null) =>
					`<span class="text-muted">${v && v.length ? v.join(', ') : '—'}</span>`
			},
			{
				key: 'created_at',
				label: t('app.mentorship.columns.created', 'Created'),
				render: (v: string) => `<span class="text-muted">${new Date(v).toLocaleDateString()}</span>`
			}
		],
		filters: [
			{
				key: 'status',
				label: t('app.mentorship.columns.status', 'Status'),
				options: APPLICATION_STATUSES.map((s) => ({ value: s, label: s }))
			}
		],
		fields,
		actions: [
			{ label: t('common.view', 'View'), onclick: (row, ctx) => ctx?.openView(row) },
			{
				label: t('app.mentorship.applications.actions.approve', 'Approve'),
				show: (row: Application) => canManage && row.status === 'pending',
				onclick: (row: Application) => approve(row)
			},
			{
				label: t('app.mentorship.applications.actions.decline', 'Decline'),
				variant: 'destructive',
				// Pending → decline; approved → revoke the grant (same route).
				show: (row: Application) => canManage && (row.status === 'pending' || row.status === 'approved'),
				onclick: (row: Application) => openDecline(row)
			}
		] as ActionItem[],
		load: async ({ page, pageSize, q, filter }) => {
			// Server-side list contract: status/q/page/page_size, meta.total —
			// ordering is pending-first on the backend (no sort param).
			const size = pageSize || 25;
			const params = new URLSearchParams({
				page: String((page ?? 0) + 1),
				page_size: String(size)
			});
			if (filter?.status) params.set('status', filter.status);
			if (q) params.set('q', q);
			const result = await mentorshipApi.listApplications(params.toString());
			if (result.ok) {
				const data = (result.data?.data as Application[]) ?? [];
				const total = result.data?.meta?.total ?? data.length;
				return {
					data,
					total,
					totalPages: Math.max(1, Math.ceil(total / size))
				};
			}
			throw new Error(
				result.error ?? t('app.mentorship.applications.error.load_failed', 'Failed to load applications')
			);
		},
		viewSnippet: ApplicationView
	});
</script>

<svelte:head>
	<title>{t('app.mentorship.applications.page_title', 'Mentor Applications — {site} Admin')}</title>
</svelte:head>

{#if pageError}
	<div class="mb-4 rounded-lg border border-danger/30 bg-danger/10 p-3 text-sm text-danger">
		{pageError}
	</div>
{/if}
<EntityPage {config} bind:controller />

{#snippet ApplicationView({ row }: { row: Application })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.applications.columns.applicant', 'Applicant')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">
				{row.username ? `@${row.username}` : row.user_id}{row.display_name ? ` — ${row.display_name}` : ''}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.columns.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{row.status}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.applications.field.motivation', 'Motivation')}</p>
			<p class="mt-1 text-sm text-foreground whitespace-pre-wrap">{row.motivation || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.applications.columns.areas', 'Areas')}</p>
			<p class="mt-1 text-sm text-foreground">{row.mentor_areas?.length ? row.mentor_areas.join(', ') : '—'}</p>
		</div>
		{#if row.decline_reason}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.applications.field.decline_reason', 'Decline reason')}</p>
				<p class="mt-1 text-sm text-foreground">{row.decline_reason}</p>
			</div>
		{/if}
		{#if row.reviewed_at}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.applications.field.reviewed', 'Reviewed')}</p>
				<p class="mt-1 text-sm text-foreground">{new Date(row.reviewed_at).toLocaleString()}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.columns.created', 'Created')}</p>
			<p class="mt-1 text-sm text-foreground">{new Date(row.created_at).toLocaleString()}</p>
		</div>
	</div>
{/snippet}

<!-- Decline / revoke reason — the member sees this when resubmitting. -->
<Modal
	bind:open={showDeclineModal}
	title={declineTarget?.status === 'approved'
		? t('app.mentorship.applications.revoke_title', 'Revoke mentor grant')
		: t('app.mentorship.applications.decline_title', 'Decline application')}
>
	{#if declineTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{#if declineTarget.status === 'approved'}
				{t(
					'app.mentorship.applications.revoke_body',
					'This removes the mentors group grant — the member loses mentor capability and their listing is retired.'
				)}
			{:else}
				{t('app.mentorship.applications.decline_body', 'The applicant can edit and resubmit after a decline.')}
			{/if}
		</p>
		<label class="mt-3 block text-sm font-medium" style="color: var(--color-foreground)" for="decline-reason">
			{t('app.mentorship.applications.field.decline_reason', 'Decline reason')}
		</label>
		<textarea
			id="decline-reason"
			bind:value={declineReason}
			rows="3"
			maxlength="500"
			placeholder={t('app.mentorship.applications.decline_reason_placeholder', 'Optional — shown to the applicant')}
			class="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm text-foreground"
		></textarea>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button
			variant="ghost"
			onclick={() => {
				showDeclineModal = false;
				declineTarget = null;
			}}
		>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={confirmDecline} disabled={acting}>
			{acting
				? t('common.working', 'Working…')
				: declineTarget?.status === 'approved'
					? t('app.mentorship.applications.actions.revoke', 'Revoke grant')
					: t('app.mentorship.applications.actions.decline', 'Decline')}
		</Button>
	</div>
</Modal>
