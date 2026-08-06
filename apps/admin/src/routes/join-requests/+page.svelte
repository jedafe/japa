<script lang="ts">
	// /join-requests — migrated to the EntityPage scaffold; read-only list with state actions (approve/reject) + bulk delete.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, CheckCircle, XCircle, Trash2 } from 'lucide-svelte';
	import type { ActionItem } from '$lib/components/ui/data-table';

	type JoinRequest = {
		id: string;
		user_id: string;
		space_id: string;
		status: string;
		message?: string;
		requested_at: string;
		username?: string;
		display_name?: string;
		space_name?: string;
	};

	const canModerateJoins = $derived(hasPermission('spaces.moderate'));

	// ── Bulk delete ───── EntityPage auto-reloads + clears selection after a bulk action handler completes.
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => api(`/admin/join-requests/${id}`, { method: 'DELETE' })));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete join requests');
		} finally {
			bulkDeleting = false;
		}
	}

	// ── State actions ───── approve/reject call the API directly then reload via ctx.
	async function approveRequest(row: JoinRequest, ctx?: any) {
		try {
			await api(`/admin/join-requests/${row.id}/approve`, { method: 'POST', body: {} });
			ctx?.reload();
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to approve request');
		}
	}

	async function rejectRequest(row: JoinRequest, ctx?: any) {
		try {
			await api(`/admin/join-requests/${row.id}/reject`, { method: 'POST', body: {} });
			ctx?.reload();
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to reject request');
		}
	}

	// `fields` is required by EntityPageConfig; the page is read-only so the form never opens.
	const fields: FieldDef[] = [
		{ key: 'display_name', label: t('joinRequests.user', 'User'), type: 'text' },
		{ key: 'space_name', label: t('joinRequests.space', 'Space'), type: 'text' },
		{ key: 'message', label: t('joinRequests.message', 'Message'), type: 'textarea' },
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
				sortable: true,
				render: (v: string, row: JoinRequest) => v || row.username || 'anonymous'
			},
			{
				key: 'space_name',
				label: t('joinRequests.space', 'Space'),
				sortable: true,
				render: (v: string, row: JoinRequest) => v || (row.space_id ? row.space_id.substring(0, 8) + '...' : '—')
			},
			{
				key: 'message',
				label: t('joinRequests.message', 'Message'),
				sortable: false,
				render: (v: string) => v || '—'
			},
			{
				key: 'status',
				label: t('joinRequests.status', 'Status'),
				sortable: true,
				render: (v: string) => {
					const cls = v === 'approved' ? 'bg-success/15 text-success' : v === 'rejected' ? 'bg-danger/15 text-danger' : 'bg-warning/15 text-warning';
					return `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${cls}">${v}</span>`;
				}
			},
			{
				key: 'requested_at',
				label: t('joinRequests.requested', 'Requested'),
				sortable: true,
				render: (v: string) => new Date(v).toLocaleDateString()
			}
		],
		actions: rowActions,
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ q }) => {
			const params = new URLSearchParams();
			if (q) params.set('q', q);
			const qs = params.toString();
			const result = await api<JoinRequest[]>(`/admin/join-requests${qs ? `?${qs}` : ''}`);
			if (result.ok) {
				const data = result.data ?? [];
				return { data, total: data.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load join requests'));
		},
		remove: async (id) => {
			const res = await api(`/admin/join-requests/${id}`, { method: 'DELETE' });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: JoinRequestView
	});
</script>

<svelte:head>
	<title>{t('joinRequests.title', 'Join Requests')} — JapaConnect Admin</title>
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
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('joinRequests.message', 'Message')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.message || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('joinRequests.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.status}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('joinRequests.requested', 'Requested')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{new Date(row.requested_at).toLocaleString()}</p>
		</div>
	</div>
{/snippet}
