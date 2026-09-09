<script lang="ts">
	// /content/video-claims — admin DataTable for video_channel_claims.
	// Ref: japaconnect-engineering/sections/106-video-content-and-import.md
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { videoClaimsAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { CheckCircle2, XCircle } from 'lucide-svelte';

	type VideoClaim = {
		id: string;
		tenant_id: string;
		claimant_user_id: string;
		claimant_username?: string;
		video_author_user_id: string;
		author_username?: string;
		video_source_id?: string;
		claim_type: string;
		evidence_url?: string;
		notes?: string;
		status: string;
		reviewed_by?: string;
		resolution_note?: string;
		created_at: string;
		resolved_at?: string;
	};

	function mapClaim(raw: any): VideoClaim {
		return { ...raw, id: raw.id?.toString?.() ?? raw.id };
	}

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkLoading = $state(false);

	const fields: FieldDef[] = [
		{
			key: 'status',
			label: t('video_claims.status', 'Status'),
			type: 'select',
			options: [
				{ value: 'approved', label: t('video_claims.approved', 'Approve') },
				{ value: 'rejected', label: t('video_claims.rejected', 'Reject') },
				{ value: 'withdrawn', label: t('video_claims.withdrawn', 'Withdraw') }
			],
			required: true
		},
		{
			key: 'resolution_note',
			label: t('video_claims.resolution_note', 'Resolution Note'),
			type: 'textarea',
			placeholder: t('video_claims.resolution_note_placeholder', 'Optional note for the claimant')
		}
	];

	const config: EntityPageConfig = {
		title: t('video_claims.title', 'Video Claims & Takedowns'),
		description: t('video_claims.description', 'Review channel ownership claims and source takedown requests.'),
		createPermission: 'admin.video_claims.create',
		editPermission: 'admin.video_claims.manage',
		deletePermission: 'admin.video_claims.delete',
		pageSize: 25,
		columns: [
			{ key: 'claimant_username', label: t('video_claims.claimant', 'Claimant'), sortable: false },
			{ key: 'author_username', label: t('video_claims.channel', 'Channel'), sortable: false },
			{
				key: 'claim_type',
				label: t('video_claims.claim_type', 'Type'),
				sortable: true,
				render: (v: string) =>
					`<span class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium ${
						v === 'claim'
							? 'bg-warning/10 text-warning'
							: 'bg-danger/10 text-danger'
					}">${v}</span>`
			},
			{
				key: 'status',
				label: t('video_claims.status', 'Status'),
				sortable: true,
				render: (v: string) =>
					`<span class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium ${
						v === 'approved'
							? 'bg-success/10 text-success'
							: v === 'rejected'
								? 'bg-danger/10 text-danger'
								: 'bg-warning/10 text-warning'
					}">${v}</span>`
			},
			{
				key: 'created_at',
				label: t('common.created', 'Created'),
				sortable: true,
				render: (v: string) => (v ? new Date(v).toLocaleString() : '-')
			},
			{
				key: 'resolved_at',
				label: t('video_claims.resolved', 'Resolved'),
				sortable: false,
				render: (v: string) => (v ? new Date(v).toLocaleString() : '-')
			}
		],
		filters: [
			{
				key: 'status',
				label: t('video_claims.status', 'Status'),
				options: [
					{ value: 'pending', label: t('video_claims.pending', 'Pending') },
					{ value: 'approved', label: t('video_claims.approved', 'Approved') },
					{ value: 'rejected', label: t('video_claims.rejected', 'Rejected') },
					{ value: 'withdrawn', label: t('video_claims.withdrawn', 'Withdrawn') }
				]
			},
			{
				key: 'claim_type',
				label: t('video_claims.claim_type', 'Type'),
				options: [
					{ value: 'claim', label: t('video_claims.type_claim', 'Ownership Claim') },
					{ value: 'takedown', label: t('video_claims.type_takedown', 'Takedown') }
				]
			}
		],
		bulkActions: [
			{
				label: t('video_claims.bulk_approve', 'Approve'),
				icon: CheckCircle2,
				disabled: (_ids: Set<string>, rows?: any[]) =>
					bulkLoading || !rows || rows.length === 0 || rows.every((r) => r?.status === 'approved'),
				onclick: async (ids: Set<string>) => {
					if (bulkLoading) return;
					bulkLoading = true;
					pageError = '';
					try {
						await Promise.all(
							Array.from(ids).map((id) =>
								videoClaimsAdminApi.review(id, { status: 'approved' })
							)
						);
					} catch (e) {
						pageError = e instanceof Error ? e.message : t('video_claims.bulk_approve_error', 'Bulk approve failed');
					} finally {
						bulkLoading = false;
					}
				}
			},
			{
				label: t('video_claims.bulk_reject', 'Reject'),
				icon: XCircle,
				variant: 'destructive',
				disabled: (_ids: Set<string>, rows?: any[]) =>
					bulkLoading || !rows || rows.length === 0 || rows.every((r) => r?.status === 'rejected'),
				onclick: async (ids: Set<string>) => {
					if (bulkLoading) return;
					bulkLoading = true;
					pageError = '';
					try {
						await Promise.all(
							Array.from(ids).map((id) =>
								videoClaimsAdminApi.review(id, { status: 'rejected' })
							)
						);
					} catch (e) {
						pageError = e instanceof Error ? e.message : t('video_claims.bulk_reject_error', 'Bulk reject failed');
					} finally {
						bulkLoading = false;
					}
				}
			}
		],
		fields,
		load: async ({ page, pageSize, q, sort, filter }) => {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (q) params.set('q', q);
			if (filter.status) params.set('status', filter.status);
			if (filter.claim_type) params.set('claim_type', filter.claim_type);
			if (sort) {
				const [sortKey, sortDir] = sort.split(':');
				if (sortKey) params.set('sort', sortKey);
				if (sortDir) params.set('dir', sortDir);
			}
			const result = await videoClaimsAdminApi.list(params.toString());
			if (result.ok) {
				const d = (result.data as { items: any[]; total: number; total_pages: number }) ?? { items: [], total: 0, total_pages: 1 };
				const data = (d.items ?? []).map(mapClaim);
				return { data, total: d.total ?? data.length, totalPages: d.total_pages ?? 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load video claims'));
		},
		update: async (id, body) => {
			const res = await videoClaimsAdminApi.review(id, {
				status: String(body.status ?? ''),
				resolution_note: body.resolution_note ? String(body.resolution_note) : undefined
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: ClaimView
	};
</script>

<svelte:head>
	<title>{t('video_claims.title', 'Video Claims')} — JapaConnect Admin</title>
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

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

{#snippet ClaimView({ row }: { row: VideoClaim })}
	<div class="space-y-4">
		<div>
			<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">
				{row.claim_type === 'takedown'
					? t('video_claims.takedown_request', 'Takedown request')
					: t('video_claims.ownership_claim', 'Ownership claim')}
			</h3>
			<p class="text-sm text-muted capitalize">{row.status}</p>
		</div>
		<dl class="grid grid-cols-2 gap-2 text-sm">
			<div class="col-span-2 sm:col-span-1">
				<dt class="text-muted">{t('video_claims.claimant', 'Claimant')}</dt>
				<dd>{row.claimant_username || row.claimant_user_id}</dd>
			</div>
			<div class="col-span-2 sm:col-span-1">
				<dt class="text-muted">{t('video_claims.channel', 'Channel')}</dt>
				<dd>{row.author_username || row.video_author_user_id}</dd>
			</div>
			<div class="col-span-2 sm:col-span-1">
				<dt class="text-muted">{t('video_claims.source', 'Source')}</dt>
				<dd>{row.video_source_id ?? t('common.none', 'None (channel-wide)')}</dd>
			</div>
			<div class="col-span-2 sm:col-span-1">
				<dt class="text-muted">{t('common.created', 'Created')}</dt>
				<dd>{row.created_at ? new Date(row.created_at).toLocaleString() : '-'}</dd>
			</div>
			{#if row.resolved_at}
				<div class="col-span-2 sm:col-span-1">
					<dt class="text-muted">{t('video_claims.resolved', 'Resolved')}</dt>
					<dd>{new Date(row.resolved_at).toLocaleString()}</dd>
				</div>
			{/if}
		</dl>
		{#if row.evidence_url}
			<div>
				<dt class="text-sm text-muted">{t('video_claims.evidence_url', 'Evidence URL')}</dt>
				<dd><a href={row.evidence_url} target="_blank" rel="noopener noreferrer" class="break-all text-sm underline">{row.evidence_url}</a></dd>
			</div>
		{/if}
		{#if row.notes}
			<div>
				<dt class="text-sm text-muted">{t('video_claims.notes', 'Notes')}</dt>
				<dd class="whitespace-pre-wrap text-sm">{row.notes}</dd>
			</div>
		{/if}
		{#if row.resolution_note}
			<div
				class="rounded p-3 text-sm"
				style="background: color-mix(in srgb, var(--color-success) 8%, transparent); color: var(--color-success)"
			>
				<strong>{t('video_claims.resolution_note', 'Resolution')}:</strong> {row.resolution_note}
			</div>
		{/if}
	</div>
{/snippet}
