<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /join-requests — migrated to the EntityPage scaffold; read-only list with state actions (approve/reject).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, XCircle } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	// GET /admin/join-requests rows carry a composite id (space_id::user_id)
	// and joined_at — there is no request id, message, or requested_at.
	// There is also no DELETE route for join requests: approve/reject are the
	// only mutations.
	type JoinRequest = {
		id: string;
		user_id: string;
		space_id: string;
		status: string;
		joined_at: string;
		username?: string;
		display_name?: string;
		space_name?: string;
	};

	const canModerateJoins = $derived(hasPermission('spaces.moderate'));

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');

	// ── State actions ───── approve/reject call the API directly then reload via ctx.
	async function approveRequest(row: JoinRequest, ctx?: any) {
		pageError = '';
		try {
			const result = await api(`/admin/join-requests/${row.id}/approve`, { method: 'POST', body: {} });
			if (result.ok) {
				ctx?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'Failed to approve request');
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to approve request');
		}
	}

	async function rejectRequest(row: JoinRequest, ctx?: any) {
		pageError = '';
		try {
			const result = await api(`/admin/join-requests/${row.id}/reject`, { method: 'POST', body: {} });
			if (result.ok) {
				ctx?.reload();
			} else {
				pageError = result.error ?? t('common.error', 'Failed to reject request');
			}
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to reject request');
		}
	}

	// `fields` is required by EntityPageConfig; the page is read-only so the form never opens.
	const fields: FieldDef[] = [
		{ key: 'display_name', label: t('joinRequests.user', 'User'), type: 'text' },
		{ key: 'space_name', label: t('joinRequests.space', 'Space'), type: 'text' },
		{ key: 'status', label: t('joinRequests.status', 'Status'), type: 'text' }
	];

	// Row actions: custom array replaces defaults, so View is included explicitly.
	const rowActions = $derived.by<ActionItem[]>(() => {
		const acts: ActionItem[] = [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (row: JoinRequest, ctx?: any) => ctx?.openView(row) }
		];
		if (canModerateJoins) {
			acts.push({
				label: t('joinRequests.approve', 'Approve'),
				icon: CheckCircle,
				onclick: approveRequest,
				show: (row: JoinRequest) => row.status === 'pending'
			});
			acts.push({
				label: t('joinRequests.reject', 'Reject'),
				icon: XCircle,
				variant: 'destructive',
				onclick: rejectRequest,
				show: (row: JoinRequest) => row.status === 'pending'
			});
		}
		return acts;
	});

	const config = $derived<EntityPageConfig>({
		title: t('joinRequests.title', 'Join Requests'),
		description: t('joinRequests.description', 'Review and moderate space join requests.'),
		pageSize: 25,
		readonly: true,
		columns: [
			{
				key: 'display_name',
				label: t('joinRequests.user', 'User'),
				render: (v: string, row: JoinRequest) => v || row.username || 'anonymous'
			},
			{
				key: 'space_name',
				label: t('joinRequests.space', 'Space'),
				render: (v: string, row: JoinRequest) => v || (row.space_id ? row.space_id.substring(0, 8) + '...' : '—')
			},
			{
				key: 'status',
				label: t('joinRequests.status', 'Status'),
				render: (v: string) => {
					const cls = v === 'approved' ? 'bg-success/15 text-success' : v === 'rejected' ? 'bg-danger/15 text-danger' : 'bg-warning/15 text-warning';
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${v}</span>`;
				}
			},
			{
				key: 'joined_at',
				label: t('joinRequests.requested', 'Requested'),
				render: (v: string) => (v ? new Date(v).toLocaleDateString() : '—')
			}
		],
		actions: rowActions,
		fields,
		load: async ({ page, pageSize, q }) => {
			// The backend currently returns every join request without pagination,
			// so we fetch a large window and apply client-side search/pagination.
			const result = await api<JoinRequest[]>(`/admin/join-requests?limit=10000`);
			if (result.ok) {
				let data = result.data ?? [];
				const query = (q ?? '').trim().toLowerCase();
				if (query) {
					data = data.filter((r) =>
						(r.username ?? '').toLowerCase().includes(query) ||
						(r.display_name ?? '').toLowerCase().includes(query) ||
						(r.space_name ?? '').toLowerCase().includes(query)
					);
				}
				const total = data.length;
				const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
				const start = page * pageSize;
				return { data: data.slice(start, start + pageSize), total, totalPages };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load join requests'));
		},
		viewSnippet: JoinRequestView
	});
</script>

<svelte:head>
	<title>{t('joinRequests.title', 'Join Requests')} — {siteName()} Admin</title>
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

<!-- ── View snippet ──────────────────────────────────────────────────────── -->
{#snippet JoinRequestView({ row }: { row: JoinRequest })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('joinRequests.user', 'User')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.display_name || row.username || 'anonymous'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('joinRequests.space', 'Space')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.space_name || (row.space_id ? row.space_id.substring(0, 8) + '...' : '—')}</p>
			</div>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('joinRequests.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('joinRequests.requested', 'Requested')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.joined_at ? new Date(row.joined_at).toLocaleString() : '—'}</p>
		</div>
	</div>
{/snippet}
