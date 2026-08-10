<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Bot, RefreshCw } from 'lucide-svelte';
	import { api } from '$lib/api';

	// QueueItem matches the backend participation.QueueItem DTO (§76).
	type QueueItem = {
		review_id: string;
		content_id: string;
		persona_name: string;
		content_type: string;
		status: string;
		created_at: string;
	};

	type Stats = {
		synthetic_actions_today: number;
		global_daily_cap: number;
		active_personas: number;
	};

	const canView = $derived(hasPermission('participation.view'));

	let queue = $state<QueueItem[]>([]);
	let stats = $state<Stats | null>(null);
	let loading = $state(true);
	let error = $state('');

	async function load() {
		loading = true;
		error = '';
		try {
			const [queueResult, statsResult] = await Promise.all([
				api<{ items: QueueItem[]; total: number }>('/admin/participation/queue'),
				api<Stats>('/admin/participation/stats')
			]);
			if (queueResult.ok) {
				queue = queueResult.data?.items ?? [];
			} else {
				error = queueResult.error;
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
	<div class="mb-4 grid grid-cols-3 gap-4">
		<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
			<p class="text-xs font-medium" style="color: var(--color-muted)">{t('participation.active_personas', 'Active Personas')}</p>
			<p class="mt-1 text-2xl font-bold" style="color: var(--color-foreground)">{stats.active_personas}</p>
		</div>
		<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
			<p class="text-xs font-medium" style="color: var(--color-muted)">{t('participation.synthetic_today', 'Synthetic Actions Today')}</p>
			<p class="mt-1 text-2xl font-bold" style="color: var(--color-foreground)">{stats.synthetic_actions_today}</p>
		</div>
		<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
			<p class="text-xs font-medium" style="color: var(--color-muted)">{t('participation.global_cap', 'Global Daily Cap')}</p>
			<p class="mt-1 text-2xl font-bold" style="color: var(--color-foreground)">{stats.global_daily_cap}</p>
		</div>
	</div>
{/if}

{#if loading}
	<div class="flex items-center gap-2 p-8" style="color: var(--color-muted)">
		<Bot class="h-4 w-4 animate-pulse" />
		<span>{t('participation.loading', 'Loading queue...')}</span>
	</div>
{:else if error}
	<div class="rounded-lg border p-4" style="border-color: var(--color-danger); color: var(--color-danger)">
		{error}
	</div>
{:else}
	<DataTable
		data={queue}
		{columns}
		emptyMessage={t('participation.empty', 'No queued persona actions.')}
	/>
{/if}
