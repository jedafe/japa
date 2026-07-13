<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Clock, RefreshCw, Play, Pause } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data = {} as PageData } = $props();

	interface CronJob {
		name: string;
		schedule: string;
		description: string;
		last_run: string | null;
		last_status: 'success' | 'failed' | 'skipped' | null;
		next_run: string | null;
		enabled: boolean;
	}

	const descriptions: Record<string, string> = {
		'cleanup-expired-sessions': 'Remove expired user sessions from the database',
		'sync-meilisearch': 'Reindex content into Meilisearch search engine',
		'cleanup-tmp-uploads': 'Delete temporary uploaded files older than 24 hours',
		'process-webhooks': 'Retry failed webhook deliveries from the queue',
		'sync-user-stats': 'Recalculate user statistics (posts, followers, reputation)',
		'cleanup-soft-deleted': 'Permanently remove soft-deleted records after grace period',
		'generate-analytics': 'Aggregate daily analytics and usage metrics',
		'check-feature-flags': 'Refresh feature flag cache from the database',
		'send-digest-emails': 'Send daily/weekly digest emails to subscribed users',
		'backup-database': 'Create incremental database backup'
	};

	// svelte-ignore state_referenced_locally
	let jobs = $state<CronJob[]>(
		(data.jobs ?? []).map((j: CronJob) => ({
			...j,
			description: descriptions[j.name] ?? j.description
		}))
	);
	let loading = $state(false);
	let message = $state('');

	async function load() {
		loading = true;
		const result = await api<CronJob[]>('/control/cron/jobs');
		if (result.ok) {
			jobs = result.data.map((j) => ({
				...j,
				description: descriptions[j.name] ?? j.description
			}));
		}
		loading = false;
	}

	async function triggerJob(name: string) {
		message = '';
		const result = await api(`/control/cron/jobs/${name}/trigger`, { method: 'POST' });
		if (result.ok) {
			message = `Job "${name}" triggered.`;
			await load();
		} else {
			message = result.error || 'Failed to trigger job.';
		}
	}

	function statusColor(status: CronJob['last_status']): string {
		switch (status) {
			case 'success': return 'var(--color-success)';
			case 'failed': return 'var(--color-danger)';
			case 'skipped': return 'var(--color-warning)';
			default: return 'var(--color-muted)';
		}
	}
</script>

<PageHeader title={t('cron.title')} description={t('cron.description')} />

<div class="mb-4 flex items-center justify-end">
	<button class="btn btn-secondary text-xs" onclick={load}>
		<RefreshCw class="h-3 w-3" />
		{t('common.refresh')}
	</button>
</div>

{#if message}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-primary); background-color: color-mix(in srgb, var(--color-primary) 8%, transparent); color: var(--color-primary)">
		{message}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else if jobs.length === 0}
	<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<Clock class="mx-auto h-8 w-8" style="color: var(--color-muted)" />
		<p class="mt-3 text-sm" style="color: var(--color-muted)">No cron jobs configured.</p>
	</div>
{:else}
	<div class="overflow-hidden rounded-lg border" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<table class="w-full text-left text-sm">
			<thead>
				<tr style="border-bottom: 1px solid var(--color-border)">
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Job</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Schedule</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Last Run</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Status</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Next Run</th>
					<th class="px-4 py-3 font-medium" style="color: var(--color-muted)">Actions</th>
				</tr>
			</thead>
			<tbody>
				{#each jobs as job (job.name)}
					<tr style="border-bottom: 1px solid var(--color-border)">
						<td class="px-4 py-3">
							<div>
								<p class="font-medium">{job.name}</p>
								<p class="text-xs" style="color: var(--color-muted)">{job.description}</p>
							</div>
						</td>
						<td class="px-4 py-3 font-mono text-xs">{job.schedule}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">
							{job.last_run ? new Date(job.last_run).toLocaleString() : '—'}
						</td>
						<td class="px-4 py-3">
							{#if job.last_status}
								<span class="rounded-full px-2 py-0.5 text-xs font-medium" style="background: color-mix(in srgb, {statusColor(job.last_status)} 12%, transparent); color: {statusColor(job.last_status)}">
									{job.last_status}
								</span>
							{:else}
								<span class="text-xs" style="color: var(--color-muted)">—</span>
							{/if}
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">
							{job.next_run ? new Date(job.next_run).toLocaleString() : '—'}
						</td>
						<td class="px-4 py-3">
							<button
								class="flex items-center gap-1 rounded px-2 py-1 text-xs font-medium transition-colors"
								style="color: var(--color-primary)"
								onclick={() => triggerJob(job.name)}
							>
								<Play class="h-3 w-3" />
								Run
							</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
