<script lang="ts">
	// /mentorship — §102 Mentorship Matching (ADR-110) connections admin.
	// Ref: japaconnect-engineering/sections/102-mentorship-matching.md
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '#lib/components/entity/index.js';
	import { mentorshipAdminApiExtended as mentorshipApi, serviceDirectoryAdminApi } from '#lib/api.js';
	import { Modal } from '#lib/components/ui/modal/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { t } from '#lib/stores/i18n.svelte.js';
	import { hasPermission } from '#lib/stores/permissions.svelte.js';
	import type { ActionItem } from '#lib/components/ui/data-table/index.js';

	// §102 connection — a mentorship-kind service_booking exposed in the
	// legacy connection vocabulary by the compat layer (000824).
	type Connection = {
		id: string;
		mentor_id: string;
		mentor_username?: string;
		mentee_id: string;
		mentee_username?: string;
		status: string;
		goals?: string | null;
		listing_content_id?: string;
		created_at: string;
		updated_at: string;
	};

	const CONNECTION_STATUSES = ['pending', 'active', 'completed', 'cancelled', 'declined'] as const;

	// Permission derivation — custom row actions bypass EntityPage's
	// permission gates (they render config.actions verbatim), so the
	// manage/destructive actions must gate themselves per-row.
	const canManageConnections = $derived(hasPermission('mentorship.manage'));
	// Unpublish hits PATCH /admin/services/{id}/unpublish which also
	// requires service.verify — show the arm only when both nodes hold.
	const canUnpublishListing = $derived(canManageConnections && hasPermission('service.verify'));

	let connectionController = $state<EntityPageController | undefined>(undefined);
	let connectionPageError = $state('');
	// "Remove bad actor" moderation arm: unpublish the mentor's shadow
	// service listing (the row stays unpublished across profile re-syncs —
	// UpdateMentorshipShadowContent only republishes 'draft' rows).
	let unpublishTarget = $state<Connection | null>(null);
	let unpublishing = $state(false);
	let showUnpublishConfirm = $state(false);

	async function confirmUnpublishListing() {
		const row = unpublishTarget;
		if (!row?.listing_content_id) return;
		unpublishing = true;
		connectionPageError = '';
		const result = await serviceDirectoryAdminApi.unpublish(row.listing_content_id);
		unpublishing = false;
		if (result.ok) {
			unpublishTarget = null;
			connectionController?.reload();
		} else {
			connectionPageError =
				result.error ?? t('app.mentorship.error.unpublish_failed', 'Failed to unpublish listing');
			unpublishTarget = null;
		}
	}

	const connectionFields = $derived<FieldDef[]>([
		{
			key: 'status',
			label: t('app.mentorship.columns.status', 'Status'),
			type: 'select',
			options: CONNECTION_STATUSES.map((s) => ({ value: s, label: s }))
		}
	]);

	const connectionConfig = $derived<EntityPageConfig>({
		title: t('app.mentorship.tabs.connections', 'Connections'),
		description: t(
			'app.mentorship.connections_description',
			'View and override mentorship connections'
		),
		// Connections are created by members via POST /mentorship/request —
		// admins only view and override status (no admin create route).
		hideCreate: true,
		createPermission: 'mentorship.manage',
		editPermission: 'mentorship.manage',
		readonly: !canManageConnections,
		pageSize: 15,
		pageSizeOptions: [10, 15, 25, 50],
		columns: [
			{
				key: 'mentor_username',
				label: t('app.mentorship.columns.mentor', 'Mentor'),
				sortable: true,
				render: (_: any, row: Connection) =>
					`<span class="font-medium text-foreground">${row.mentor_username ? '@' + row.mentor_username : row.mentor_id}</span>`
			},
			{
				key: 'mentee_username',
				label: t('app.mentorship.columns.mentee', 'Mentee'),
				sortable: true,
				render: (_: any, row: Connection) =>
					`<span class="font-medium text-foreground">${row.mentee_username ? '@' + row.mentee_username : row.mentee_id}</span>`
			},
			{
				key: 'status',
				label: t('app.mentorship.columns.status', 'Status'),
				sortable: true,
				render: (v: string) => `<span class="font-medium">${v}</span>`
			},
			{
				key: 'created_at',
				label: t('app.mentorship.columns.created', 'Created'),
				sortable: true,
				render: (v: string) => `<span class="text-muted">${new Date(v).toLocaleDateString()}</span>`
			}
		],
		filters: [
			{
				key: 'status',
				label: t('app.mentorship.columns.status', 'Status'),
				options: CONNECTION_STATUSES.map((s) => ({ value: s, label: s }))
			}
		],
		fields: connectionFields,
		actions: [
			{ label: t('common.view', 'View'), onclick: (row, ctx) => ctx?.openView(row) },
			{
				label: t('common.edit', 'Edit'),
				show: () => canManageConnections,
				onclick: (row, ctx) => ctx?.openEdit(row)
			},
			{
				label: t('app.mentorship.actions.unpublish_listing', 'Unpublish mentor listing'),
				variant: 'destructive',
				show: (row: Connection) => canUnpublishListing && !!row.listing_content_id,
				onclick: (row: Connection) => {
					unpublishTarget = row;
					showUnpublishConfirm = true;
				}
			}
		] as ActionItem[],
		load: async ({ page, pageSize, q, sort, filter }) => {
			// Server-side list contract: the API accepts
			// status/q/sort/page/page_size and returns meta.total — column
			// '<key>:<dir>' maps onto the backend vocabulary
			// (latest|oldest|status|mentor_asc|mentor_desc|mentee_asc|mentee_desc).
			const size = pageSize || 25;
			const params = new URLSearchParams({
				page: String((page ?? 0) + 1),
				page_size: String(size)
			});
			if (filter?.status) params.set('status', filter.status);
			if (q) params.set('q', q);
			if (sort) {
				const [key, dir] = sort.split(':');
				const d = dir === 'asc' ? 'asc' : 'desc';
				const token = {
					status: 'status',
					mentor_username: `mentor_${d}`,
					mentee_username: `mentee_${d}`,
					created_at: d === 'asc' ? 'oldest' : 'latest'
				}[key];
				if (token) params.set('sort', token);
			}
			const result = await mentorshipApi.listConnections(params.toString());
			if (result.ok) {
				const data = (result.data?.data as Connection[]) ?? [];
				const total = result.data?.meta?.total ?? data.length;
				return {
					data,
					total,
					totalPages: Math.max(1, Math.ceil(total / size))
				};
			}
			throw new Error(
				result.error ?? t('app.mentorship.error.load_connections_failed', 'Failed to load connections')
			);
		},
		update: async (id, body) => {
			const result = await mentorshipApi.updateConnection(id, body);
			if (result.ok) return { ok: true };
			return { ok: false, error: result.error ?? 'Failed to update connection' };
		},
		viewSnippet: ConnectionView
	});
</script>

<svelte:head>
	<title>{t('app.mentorship.page_title', 'Mentorship — {site} Admin')}</title>
</svelte:head>

{#if connectionPageError}
	<div class="mb-4 rounded-lg border border-danger/30 bg-danger/10 p-3 text-sm text-danger">
		{connectionPageError}
	</div>
{/if}
<EntityPage config={connectionConfig} bind:controller={connectionController} />

{#snippet ConnectionView({ row }: { row: Connection })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.columns.mentor', 'Mentor')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{row.mentor_username ? `@${row.mentor_username}` : row.mentor_id}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.columns.mentee', 'Mentee')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{row.mentee_username ? `@${row.mentee_username}` : row.mentee_id}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.columns.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{row.status}</p>
		</div>
		{#if row.goals}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.field.goals', 'Goals')}</p>
				<p class="mt-1 text-sm font-medium text-foreground">{row.goals}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider text-muted">{t('app.mentorship.columns.created', 'Created')}</p>
			<p class="mt-1 text-sm font-medium text-foreground">{new Date(row.created_at).toLocaleString()}</p>
		</div>
	</div>
{/snippet}

<!-- Unpublish mentor listing (remove bad actor) — stays unpublished across
     profile re-syncs; the listing re-enters review on next publish. -->
<Modal
	bind:open={showUnpublishConfirm}
	title={t('app.mentorship.unpublish_title', 'Unpublish mentor listing')}
>
	{#if unpublishTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t(
				'app.mentorship.unpublish_confirm',
				'Remove the mentor’s listing from the directory? They will stop receiving new requests immediately.'
			)}
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">
			{t('app.mentorship.unpublish_warning', 'Existing connections are not cancelled — only the public listing is unpublished.')}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button
			variant="ghost"
			onclick={() => {
				showUnpublishConfirm = false;
				unpublishTarget = null;
			}}
		>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={confirmUnpublishListing} disabled={unpublishing}>
			{unpublishing
				? t('common.working', 'Working…')
				: t('app.mentorship.actions.unpublish_listing', 'Unpublish mentor listing')}
		</Button>
	</div>
</Modal>
