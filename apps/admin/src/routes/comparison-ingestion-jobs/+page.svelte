<script lang="ts">
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { listIngestionJobs } from '$lib/api/comparison-data-sources';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import type { IngestionJob } from '$lib/types/jce';
	import type { Column } from '$lib/components/ui/data-table';
	import { onMount, untrack } from 'svelte';

	const canView = $derived(hasPermission('jce.ingestion_job.view'));

	let contentID = $state(page.url.searchParams.get('content_id') ?? '');
	let jobs = $state<IngestionJob[]>([]);
	let loading = $state(false);
	let error = $state('');

	function statusClass(status: string): string {
		switch (status) {
			case 'completed':
				return 'bg-success/15 text-success';
			case 'failed':
				return 'bg-destructive/15 text-destructive';
			case 'running':
				return 'bg-primary/15 text-primary';
			case 'cancelled':
				return 'bg-muted/15 text-muted';
			default:
				return 'bg-muted/15 text-muted';
		}
	}

	function formatDate(v: string | null | undefined): string {
		if (!v) return '—';
		try {
			return new Date(v).toLocaleString();
		} catch {
			return v;
		}
	}

	const columns: Column[] = [
		{
			key: 'status',
			label: t('ingestion_jobs.status', 'Status'),
			sortable: true,
			render: (v: string) =>
				`<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium ${statusClass(v)}">${v}</span>`
		},
		{
			key: 'source_kind',
			label: t('ingestion_jobs.source_kind', 'Source kind'),
			render: (v: string | null) =>
				v ? t('data_sources.' + v, v) : '—'
		},
		{
			key: 'refreshed_count',
			label: t('ingestion_jobs.refreshed', 'Refreshed')
		},
		{
			key: 'error',
			label: t('ingestion_jobs.error', 'Error'),
			render: (v: string | null) =>
				v ? `<span class="text-xs" style="color: var(--color-danger)">${v}</span>` : '—'
		},
		{
			key: 'started_at',
			label: t('ingestion_jobs.started_at', 'Started'),
			render: (v: string | null) => `<span class="text-xs" style="color: var(--color-muted)">${formatDate(v)}</span>`
		},
		{
			key: 'completed_at',
			label: t('ingestion_jobs.completed_at', 'Completed'),
			render: (v: string | null) => `<span class="text-xs" style="color: var(--color-muted)">${formatDate(v)}</span>`
		},
		{
			key: 'created_at',
			label: t('ingestion_jobs.created_at', 'Created'),
			render: (v: string) => `<span class="text-xs" style="color: var(--color-muted)">${formatDate(v)}</span>`
		}
	];

	async function load(id: string = contentID) {
		const trimmed = id.trim();
		if (!trimmed) {
			error = t('ingestion_jobs.content_id_required', 'Comparison content ID is required');
			return;
		}
		if (untrack(() => loading)) return;
		loading = true;
		error = '';
		try {
			jobs = await listIngestionJobs(trimmed);
			if (typeof window !== 'undefined') {
				replaceState(`/comparison-ingestion-jobs?content_id=${encodeURIComponent(trimmed)}`, {});
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('common.error', 'Failed to load ingestion jobs');
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		const id = contentID.trim();
		if (id) {
			load(id);
		}
	});

	function handleLoadClick() {
		load();
	}

	function handleInputKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			e.preventDefault();
			load();
		}
	}
</script>

<svelte:head>
	<title>{t('ingestion_jobs.title', 'Comparison refresh jobs')} — {t('common.admin_title_suffix', '{site} Admin')}</title>
</svelte:head>

{#if !canView}
	<div
		class="rounded-lg p-4 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{t('common.permission_denied', 'Permission denied')}
	</div>
{:else}
	<div class="space-y-6">
		<div>
			<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">
				{t('ingestion_jobs.title', 'Comparison refresh jobs')}
			</h1>
			<p class="mt-1 text-sm" style="color: var(--color-muted)">
				{t('ingestion_jobs.description', 'View refresh job history for a comparison')}
			</p>
		</div>

		<div class="flex flex-col gap-2 sm:flex-row sm:items-end sm:gap-3">
			<div class="flex-1">
				<label for="content-id" class="block text-sm font-medium" style="color: var(--color-foreground)">
					{t('ingestion_jobs.content_id', 'Comparison')}
				</label>
				<input
					id="content-id"
					type="text"
					class="mt-1 w-full rounded-lg border p-2 text-sm font-mono"
					style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
					placeholder={t('ingestion_jobs.content_id_placeholder', 'Enter comparison content ID')}
					bind:value={contentID}
					onkeydown={handleInputKeydown}
				/>
			</div>
			<Button onclick={handleLoadClick} disabled={loading}>
				{loading ? t('common.loading', 'Loading…') : t('common.load', 'Load')}
			</Button>
		</div>

		{#if error}
			<div
				class="rounded-lg p-3 text-sm"
				style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
			>
				{error}
			</div>
		{/if}

		{#if contentID.trim() && !loading && jobs.length === 0 && !error}
			<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border)">
				<p class="text-sm" style="color: var(--color-muted)">
					{t('ingestion_jobs.empty', 'No refresh jobs found for this comparison')}
				</p>
			</div>
		{:else if jobs.length > 0}
			<DataTable {columns} data={jobs} pageSize={25} />
		{/if}
	</div>
{/if}
