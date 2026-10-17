<script lang="ts">
	// /content/video-imports — admin DataTable for video_import_jobs.
	// Ref: japaconnect-engineering/sections/106-video-content-and-import.md
	import { EntityPage, type EntityPageConfig, type EntityPageController } from '$lib/components/entity';
	import type { ActionContext } from '$lib/components/ui/data-table/types';
	import { videoImportJobsAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { siteName } from '$lib/stores/site.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, RefreshCw } from 'lucide-svelte';

	type VideoImportJob = {
		id: string;
		user_id: string;
		owner_username?: string;
		source_type: string;
		source_url?: string;
		query?: string;
		target_author_user_id?: string;
		status: string;
		total_items: number;
		imported_count: number;
		failed_count: number;
		error_log?: any[];
		created_at: string;
		completed_at?: string;
		started_at?: string;
		auto_publish: boolean;
	};

	function mapJob(raw: any): VideoImportJob {
		return { ...raw, id: raw.id?.toString?.() ?? raw.id };
	}

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let retryingId = $state<string | null>(null);
	let bulkLoading = $state(false);

	function errorSummary(job: VideoImportJob): string {
		if (!job.error_log || !Array.isArray(job.error_log) || job.error_log.length === 0) return '';
		const first = job.error_log[0];
		if (typeof first === 'string') return first;
		return first?.message ?? JSON.stringify(first);
	}

	const config: EntityPageConfig = {
		title: t('video_import_jobs.title', 'Video Import Jobs'),
		description: t('video_import_jobs.description', 'Monitor and retry bulk YouTube import jobs.'),
		createPermission: 'admin.video_import_jobs.create',
		editPermission: 'admin.video_import_jobs.manage',
		deletePermission: 'admin.video_import_jobs.delete',
		pageSize: 25,
		columns: [
			{ key: 'source_type', label: t('video_import_jobs.source_type', 'Type'), sortable: true },
			{
				key: 'query',
				label: t('video_import_jobs.query', 'Query / URL'),
				sortable: false,
				render: (_: string, row: VideoImportJob) => (row.query || row.source_url || '-')
			},
			{ key: 'owner_username', label: t('video_import_jobs.owner', 'Owner'), sortable: false },
			{
				key: 'status',
				label: t('video_import_jobs.status', 'Status'),
				sortable: true,
				render: (v: string) =>
					`<span class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium ${
						v === 'completed'
							? 'bg-success/10 text-success'
							: v === 'failed'
								? 'bg-danger/10 text-danger'
								: 'bg-warning/10 text-warning'
					}">${v}</span>`
			},
			{
				key: 'progress',
				label: t('video_import_jobs.progress', 'Progress'),
				sortable: false,
				render: (_: unknown, row: VideoImportJob) =>
					`${row.imported_count ?? 0} / ${row.total_items ?? 0} (${row.failed_count ?? 0} failed)`
			},
			{
				key: 'created_at',
				label: t('common.created', 'Created'),
				sortable: true,
				render: (v: string) => (v ? new Date(v).toLocaleString() : '-')
			}
		],
		filters: [
			{
				key: 'status',
				label: t('video_import_jobs.status', 'Status'),
				options: [
					{ value: 'pending', label: t('video_import_jobs.status_pending', 'Pending') },
					{ value: 'running', label: t('video_import_jobs.status_running', 'Running') },
					{ value: 'completed', label: t('video_import_jobs.status_completed', 'Completed') },
					{ value: 'failed', label: t('video_import_jobs.status_failed', 'Failed') }
				]
			},
			{
				key: 'source_type',
				label: t('video_import_jobs.source_type', 'Type'),
				options: [
					{ value: 'channel', label: t('video_import_jobs.type_channel', 'Channel') },
					{ value: 'playlist', label: t('video_import_jobs.type_playlist', 'Playlist') },
					{ value: 'keyword', label: t('video_import_jobs.type_keyword', 'Keyword') }
				]
			}
		],
		actions: [
			{
				label: t('common.view', 'View'),
				icon: Eye,
				onclick: (row: any, ctx?: ActionContext) => ctx?.openView(row)
			},
			{
				label: t('video_import_jobs.retry', 'Retry'),
				icon: RefreshCw,
				show: (row: any) =>
					hasPermission('admin.video_import_jobs.manage') &&
					row?.status !== 'running' &&
					row?.status !== 'pending',
				onclick: async (row: any, ctx?: ActionContext) => {
					const id = row?.id;
					if (!id || retryingId) return;
					retryingId = id;
					pageError = '';
					try {
						const res = await videoImportJobsAdminApi.retry(id);
						if (!res.ok) {
							pageError = res.error ?? t('common.error', 'Failed to retry job');
						} else {
							ctx?.reload();
						}
					} catch (e) {
						pageError = e instanceof Error ? e.message : t('common.error', 'Failed to retry job');
					} finally {
						retryingId = null;
					}
				}
			}
		],
		bulkActions: [
			{
				label: t('video_import_jobs.bulk_retry', 'Retry'),
				icon: RefreshCw,
				disabled: (_ids: Set<string>, rows?: any[]) =>
					bulkLoading || !rows || rows.length === 0 || rows.every((r) => r?.status === 'running' || r?.status === 'pending'),
				onclick: async (ids: Set<string>) => {
					if (bulkLoading) return;
					bulkLoading = true;
					pageError = '';
					try {
						await Promise.all(
							Array.from(ids).map((id) =>
								videoImportJobsAdminApi.retry(id)
							)
						);
					} catch (e) {
						pageError = e instanceof Error ? e.message : t('video_import_jobs.bulk_retry_error', 'Bulk retry failed');
					} finally {
						bulkLoading = false;
					}
				}
			}
		],
		fields: [],
		load: async ({ page, pageSize, q, sort, filter }) => {
			const params = new URLSearchParams({
				page_size: String(pageSize),
				page_offset: String(page * pageSize)
			});
			if (q) params.set('q', q);
			if (filter.status) params.set('status', filter.status);
			if (filter.source_type) params.set('source_type', filter.source_type);
			if (sort) {
				const [sortKey, sortDir] = sort.split(':');
				if (sortKey) params.set('sort', sortKey);
				if (sortDir) params.set('dir', sortDir);
			}
			const result = await videoImportJobsAdminApi.list(params.toString());
			if (result.ok) {
				const d = (result.data as { items: any[]; total: number; total_pages: number }) ?? { items: [], total: 0, total_pages: 1 };
				const data = (d.items ?? []).map(mapJob);
				return { data, total: d.total ?? data.length, totalPages: d.total_pages ?? 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load import jobs'));
		},
		viewSnippet: JobView
	};
</script>

<svelte:head>
	<title>{t('video_import_jobs.title', 'Video Import Jobs')} — {siteName()} Admin</title>
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

{#snippet JobView({ row }: { row: VideoImportJob })}
	<div class="space-y-4">
		<div>
			<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">{row.query || row.source_url || t('video_import_jobs.untitled', 'Untitled import')}</h3>
			<p class="text-sm text-muted capitalize">{row.source_type}</p>
		</div>
		<dl class="grid grid-cols-2 gap-2 text-sm">
			<div class="col-span-2 sm:col-span-1">
				<dt class="text-muted">{t('video_import_jobs.owner', 'Owner')}</dt>
				<dd>{row.owner_username || row.user_id}</dd>
			</div>
			<div class="col-span-2 sm:col-span-1">
				<dt class="text-muted">{t('video_import_jobs.status', 'Status')}</dt>
				<dd class="capitalize">{row.status}</dd>
			</div>
			<div class="col-span-2 sm:col-span-1">
				<dt class="text-muted">{t('video_import_jobs.progress', 'Progress')}</dt>
				<dd>{row.imported_count ?? 0} / {row.total_items ?? 0} ({row.failed_count ?? 0} failed)</dd>
			</div>
			<div class="col-span-2 sm:col-span-1">
				<dt class="text-muted">{t('video_import_jobs.auto_publish', 'Auto-publish')}</dt>
				<dd>{row.auto_publish ? t('common.yes', 'Yes') : t('common.no', 'No')}</dd>
			</div>
			<div class="col-span-2 sm:col-span-1">
				<dt class="text-muted">{t('common.created', 'Created')}</dt>
				<dd>{row.created_at ? new Date(row.created_at).toLocaleString() : '-'}</dd>
			</div>
			<div class="col-span-2 sm:col-span-1">
				<dt class="text-muted">{t('video_import_jobs.completed', 'Completed')}</dt>
				<dd>{row.completed_at ? new Date(row.completed_at).toLocaleString() : '-'}</dd>
			</div>
		</dl>
		{#if errorSummary(row)}
			<div
				class="rounded p-3 text-sm"
				style="background: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
			>
				<strong>{t('video_import_jobs.last_error', 'Last error')}:</strong> {errorSummary(row)}
			</div>
		{/if}
	</div>
{/snippet}
