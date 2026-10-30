<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { Bot, RefreshCw, Check, X } from 'lucide-svelte';
	import { api, apiRaw } from '$lib/api';

	// QueueItem matches the backend participation.QueueItem DTO (§76).
	type QueueItem = {
		review_id: string;
		content_id: string;
		persona_name: string;
		content_type: string;
		status: string;
		created_at: string;
	};

	type QueueData = {
		items: QueueItem[];
		total: number;
		page: number;
		page_size: number;
		total_pages: number;
	};

	type Stats = {
		synthetic_actions_today: number;
		global_daily_cap: number;
		active_personas: number;
	};

	const canReview = $derived(hasPermission('participation.review'));

	let queue = $state<QueueItem[]>([]);
	let listPage = $state(0);
	let listPageSize = $state(25);
	let listTotalPages = $state(1);
	let stats = $state<Stats | null>(null);
	let loading = $state(true);
	let error = $state('');
	let acting = $state(false);

	async function load() {
		loading = true;
		error = '';
		try {
			const pageParam = listPage + 1;
			const [queueResult, statsResult] = await Promise.all([
				api<QueueData>(`/admin/participation/queue?page=${pageParam}&page_size=${listPageSize}`),
				api<Stats>('/admin/participation/stats')
			]);
			if (queueResult.ok) {
				const q = queueResult.data;
				queue = q?.items ?? [];
				listTotalPages = q?.total_pages ?? 1;
			} else {
				error = queueResult.error ?? 'Failed to load participation queue';
			}
			if (statsResult.ok) {
				stats = statsResult.data ?? null;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('participation.load_error', 'Failed to load participation queue');
		} finally {
			loading = false;
		}
	}

	// approve/reject reuses PATCH /admin/moderation-queue/:id (§76).
	// The backend ModerateItem handler detects persona_action-sourced
	// content_reviews rows, updates their status, and enqueues
	// persona:dispatch_action on approve (BUG-001).
	async function reviewAction(row: QueueItem, action: 'approve' | 'reject') {
		if (acting) return;
		acting = true;
		try {
			const result = await apiRaw(`/admin/moderation-queue/${row.review_id}`, {
				method: 'PATCH',
				body: { action }
			});
			if (result.ok) {
				await load();
			} else {
				error = result.error ?? `Failed to ${action} action`;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : `Failed to ${action} action`;
		} finally {
			acting = false;
		}
	}

	const rowActions: ActionItem[] = [
		{
			label: t('participation.approve', 'Approve'),
			icon: Check,
			onclick: (row: QueueItem) => reviewAction(row, 'approve'),
			show: () => canReview
		},
		{
			label: t('participation.reject', 'Reject'),
			icon: X,
			onclick: (row: QueueItem) => reviewAction(row, 'reject'),
			variant: 'destructive',
			show: () => canReview
		}
	];

	const columns = $derived([
		{
			key: 'persona_name',
			label: t('participation.persona', 'Persona'),
			sortable: true,
			render: (value: string) => `<span class="text-foreground font-medium">${value ?? '—'}</span>`
		},
		{
			key: 'content_type',
			label: t('participation.content_type', 'Content Type'),
			sortable: true,
			render: (value: string) => `<span class="text-muted-foreground">${value}</span>`
		},
		{
			key: 'status',
			label: t('participation.status', 'Status'),
			sortable: true,
			render: (value: string) => {
				const cls = value === 'pending' ? 'bg-warning/15 text-warning border-warning' :
					value === 'approved' ? 'bg-success/15 text-success border-success' :
					'bg-muted text-muted-foreground border-border';
				return `<span class="inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${cls}">${value}</span>`;
			}
		},
		{
			key: 'created_at',
			label: t('participation.created', 'Created'),
			sortable: true,
			render: (value: string) => `<span class="text-muted-foreground text-xs">${value ? new Date(value).toLocaleString() : '—'}</span>`
		}
	]);

	load();
</script>

<PageHeader
	title={t('participation.title', 'Participation Queue')}
	description={t('participation.description', 'Review queued AI persona actions before they are published.')}
	actions={[
		{ label: t('common.refresh', 'Refresh'), icon: RefreshCw, onclick: () => load() }
	]}
/>

{#if stats}
	<div class="mb-4 grid grid-cols-1 sm:grid-cols-3 gap-4">
		<div class="rounded-lg border border-border p-4">
			<p class="text-xs font-medium text-muted-foreground">{t('participation.active_personas', 'Active Personas')}</p>
			<p class="mt-1 text-2xl font-bold text-foreground">{stats.active_personas}</p>
		</div>
		<div class="rounded-lg border border-border p-4">
			<p class="text-xs font-medium text-muted-foreground">{t('participation.synthetic_today', 'Synthetic Actions Today')}</p>
			<p class="mt-1 text-2xl font-bold text-foreground">{stats.synthetic_actions_today}</p>
		</div>
		<div class="rounded-lg border border-border p-4">
			<p class="text-xs font-medium text-muted-foreground">{t('participation.global_cap', 'Global Daily Cap')}</p>
			<p class="mt-1 text-2xl font-bold text-foreground">{stats.global_daily_cap}</p>
		</div>
	</div>
{/if}

{#if loading}
	<div class="flex items-center gap-2 p-8 text-muted-foreground">
		<Bot class="h-4 w-4 animate-pulse" />
		<span>{t('participation.loading', 'Loading queue...')}</span>
	</div>
{:else if error}
	<div class="rounded-lg border border-danger p-4 text-danger">
		{error}
	</div>
{:else}
	<DataTable
		data={queue}
		{columns}
		actions={rowActions}
		serverSide
		page={listPage}
		totalPages={listTotalPages}
		pageSize={listPageSize}
		onpagechange={(p) => { listPage = p; load(); }}
		onpagesizechange={(s) => { listPageSize = s; listPage = 0; load(); }}
		emptyMessage={t('participation.empty', 'No queued persona actions.')}
	/>
{/if}
