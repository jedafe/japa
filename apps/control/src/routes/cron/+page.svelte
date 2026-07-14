<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import Pagination from '$lib/components/ui/Pagination.svelte';
	import { Tabs, Select, Tooltip } from 'bits-ui';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Clock, RefreshCw, Play, Trash2, Pause, Pencil } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data = {} as PageData } = $props();

	interface CronJob {
		id: string;
		type: string;
		spec: string;
		next_run_at: string | null;
		prev_run_at: string | null;
	}

	// svelte-ignore state_referenced_locally
	let jobs = $state<CronJob[]>(data.jobs ?? []);
	let loading = $state(false);
	let message = $state('');
	let messageType = $state<'success' | 'error' | ''>('');
	let activeTab = $state('all');
	let filterStatus = $state('all');

	// Pagination
	let page = $state(0);
	let pageSize = $state(10);

	// Edit/Create modal state
	let editModalOpen = $state(false);
	let editingJob = $state<CronJob | null>(null);
	let editForm = $state({ type: '', spec: '', payload: '{}' });

	const statusItems = [
		{ value: 'all', label: t('cron.all_status', 'All Status') },
		{ value: 'active', label: t('cron.active', 'Active') },
		{ value: 'paused', label: t('cron.paused', 'Paused') },
	];

	async function load() {
		loading = true;
		message = '';
		const result = await api<CronJob[]>('/control/cron/jobs');
		if (result.ok) {
			jobs = result.data;
		}
		loading = false;
	}

	async function triggerJob(id: string) {
		message = '';
		const result = await api(`/control/cron/jobs/${id}/trigger`, { method: 'POST' });
		if (result.ok) {
			message = t('cron.trigger_success', `Job triggered successfully.`);
			messageType = 'success';
			await load();
		} else {
			message = result.error || t('cron.trigger_error', 'Failed to trigger job.');
			messageType = 'error';
		}
	}

	let filteredJobs = $derived(
		filterStatus !== 'all'
			? jobs.filter((j) => {
					if (filterStatus === 'active') return true;
					if (filterStatus === 'paused') return false;
					return true;
				})
			: jobs
	);

	let totalPages = $derived(Math.max(1, Math.ceil(filteredJobs.length / pageSize)));
	let paginatedJobs = $derived.by(() => {
		const start = page * pageSize;
		return filteredJobs.slice(start, start + pageSize);
	});

	function openEditModal(job?: CronJob) {
		if (job) {
			editingJob = job;
			editForm = { type: job.type, spec: job.spec, payload: '{}' };
		} else {
			editingJob = null;
			editForm = { type: '', spec: '', payload: '{}' };
		}
		editModalOpen = true;
	}

	function closeModal() {
		editModalOpen = false;
		editingJob = null;
		editForm = { type: '', spec: '', payload: '{}' };
	}
</script>

<PageHeader
	title={t('cron.title', 'Cron Jobs')}
	description={t('cron.description', 'Scheduled task management')}
/>

<div class="mb-4 flex flex-wrap items-center gap-3">
	<Select.Root type="single" bind:value={filterStatus} items={statusItems}>
		<Select.Trigger
			class="border-border-input bg-background data-placeholder:text-muted-foreground/50 rounded-9px flex h-10 items-center border px-3 text-sm transition-colors hover:border-border-input-hover"
			aria-label={t('cron.filter_status', 'Filter by status')}
		>
			<Select.Value placeholder={t('cron.all_status', 'All Status')} />
		</Select.Trigger>
		<Select.Portal>
			<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-xl border px-1 py-1" sideOffset={4}>
				<Select.Viewport class="p-1">
					{#each statusItems as item}
						<Select.Item
							class="data-highlighted:bg-accent rounded-9px flex h-9 cursor-pointer items-center px-3 text-sm outline-none"
							value={item.value}
							label={item.label}
						>
							{#snippet children({ selected })}
								{item.label}
								{#if selected}<span class="ml-auto">✓</span>{/if}
							{/snippet}
						</Select.Item>
					{/each}
				</Select.Viewport>
			</Select.Content>
		</Select.Portal>
	</Select.Root>
	<div class="flex-1"></div>
	<button
		class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px flex items-center gap-1.5 border px-3 py-2 text-xs font-medium transition-colors"
		onclick={load}
		disabled={loading}
	>
		<RefreshCw class="size-3 {loading ? 'animate-spin' : ''}" />
		{t('common.refresh', 'Refresh')}
	</button>
</div>

{#if message}
	<div
		class="mb-4 rounded-lg border px-4 py-3 text-sm {messageType === 'success'
			? 'border-green-500/30 bg-green-500/10 text-green-600'
			: messageType === 'error'
				? 'border-destructive/30 bg-destructive/10 text-destructive'
				: ''}"
	>
		{message}
	</div>
{/if}

{#if loading}
	<div class="flex items-center justify-center py-12">
		<RefreshCw class="text-muted-foreground size-6 animate-spin" />
	</div>
{:else if jobs.length === 0}
	<div class="border-border-card bg-background-alt rounded-9px border p-8 text-center">
		<Clock class="text-muted-foreground mx-auto mb-3 size-8" />
		<p class="text-muted-foreground text-sm">{t('cron.no_jobs', 'No cron jobs configured.')}</p>
	</div>
{:else}
	<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
		<table class="w-full text-left text-sm">
			<thead>
				<tr class="border-border-card border-b">
					<th class="text-muted-foreground px-4 py-3 font-medium">{t('cron.job', 'Job')}</th>
					<th class="text-muted-foreground px-4 py-3 font-medium">{t('cron.type', 'Type')}</th>
					<th class="text-muted-foreground px-4 py-3 font-medium">{t('cron.schedule', 'Schedule')}</th>
					<th class="text-muted-foreground px-4 py-3 font-medium">{t('cron.last_run', 'Last Run')}</th>
					<th class="text-muted-foreground px-4 py-3 font-medium">{t('cron.next_run', 'Next Run')}</th>
					<th class="text-muted-foreground px-4 py-3 font-medium">{t('cron.actions', 'Actions')}</th>
				</tr>
			</thead>
			<tbody>
				{#each paginatedJobs as job, index (job.id + '-' + index)}
					<tr class="border-border-card border-t hover:bg-muted/30 transition-colors">
						<td class="px-4 py-3">
							<p class="font-medium">{job.id}</p>
						</td>
						<td class="px-4 py-3">
							<span class="bg-muted text-muted-foreground rounded-5px px-1.5 py-0.5 font-mono text-xs">
								{job.type}
							</span>
						</td>
						<td class="px-4 py-3 font-mono text-xs">{job.spec}</td>
						<td class="text-muted-foreground px-4 py-3 text-xs">
							{job.prev_run_at ? new Date(job.prev_run_at).toLocaleString() : '—'}
						</td>
						<td class="text-muted-foreground px-4 py-3 text-xs">
							{job.next_run_at ? new Date(job.next_run_at).toLocaleString() : '—'}
						</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-1">
								<Tooltip.Root>
									<Tooltip.Trigger>
										<button
											class="text-green-600 hover:bg-green-500/10 rounded-5px p-1.5 transition-colors"
											onclick={() => triggerJob(job.id)}
										>
											<Play class="size-3.5" />
										</button>
									</Tooltip.Trigger>
									<Tooltip.Portal>
										<Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs">
											{t('cron.run_now', 'Run now')}
										</Tooltip.Content>
									</Tooltip.Portal>
								</Tooltip.Root>
								<Tooltip.Root>
									<Tooltip.Trigger>
										<button
											class="text-foreground-alt hover:bg-muted rounded-5px p-1.5 transition-colors"
											onclick={() => openEditModal(job)}
										>
											<Pencil class="size-3.5" />
										</button>
									</Tooltip.Trigger>
									<Tooltip.Portal>
										<Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs">
											{t('cron.edit', 'Edit')}
										</Tooltip.Content>
									</Tooltip.Portal>
								</Tooltip.Root>
								<Tooltip.Root>
									<Tooltip.Trigger>
										<button
											class="text-tertiary hover:bg-tertiary/10 rounded-5px p-1.5 transition-colors"
										>
											<Pause class="size-3.5" />
										</button>
									</Tooltip.Trigger>
									<Tooltip.Portal>
										<Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs">
											{t('cron.pause', 'Pause')}
										</Tooltip.Content>
									</Tooltip.Portal>
								</Tooltip.Root>
								<Tooltip.Root>
									<Tooltip.Trigger>
										<button
											class="text-destructive hover:bg-destructive/10 rounded-5px p-1.5 transition-colors"
										>
											<Trash2 class="size-3.5" />
										</button>
									</Tooltip.Trigger>
									<Tooltip.Portal>
										<Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 rounded-input border px-3 py-2 text-xs">
											{t('cron.delete', 'Delete')}
										</Tooltip.Content>
									</Tooltip.Portal>
								</Tooltip.Root>
							</div>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	<Pagination
		bind:page
		{totalPages}
		totalItems={filteredJobs.length}
		bind:pageSize
	/>
{/if}

{#if editModalOpen}
	<div class="fixed inset-0 z-50 flex items-center justify-center">
		<div class="fixed inset-0 bg-black/50" onclick={closeModal}></div>
		<div class="border-border-card bg-background relative z-50 w-full max-w-lg rounded-xl border p-6 shadow-lg">
			<h2 class="text-foreground mb-4 text-lg font-semibold">
				{editingJob ? t('cron.edit_job', 'Edit Job') : t('cron.create_job', 'Create Job')}
			</h2>
			<div class="space-y-4">
				<div>
					<label class="text-foreground mb-1 block text-sm font-medium">{t('cron.job_type', 'Job Type')}</label>
					<input
						type="text"
						bind:value={editForm.type}
						class="border-border-input bg-background text-foreground w-full rounded-5px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder="email:send"
					/>
				</div>
				<div>
					<label class="text-foreground mb-1 block text-sm font-medium">{t('cron.schedule', 'Schedule')}</label>
					<input
						type="text"
						bind:value={editForm.spec}
						class="border-border-input bg-background text-foreground w-full rounded-5px border px-3 py-2 font-mono text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder="0 */6 * * *"
					/>
					<p class="text-muted-foreground mt-1 text-xs">Cron expression (e.g., "0 */6 * * *" for every 6 hours)</p>
				</div>
				<div>
					<label class="text-foreground mb-1 block text-sm font-medium">{t('cron.payload', 'Payload')} (JSON)</label>
					<textarea
						bind:value={editForm.payload}
						rows="4"
						class="border-border-input bg-background text-foreground w-full rounded-5px border px-3 py-2 font-mono text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder={'{"key": "value"}'}
					></textarea>
				</div>
			</div>
			<div class="mt-6 flex justify-end gap-3">
				<button
					class="border-border-input bg-background hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
					onclick={closeModal}
				>
					{t('common.cancel', 'Cancel')}
				</button>
				<button
					class="bg-foreground text-background rounded-9px px-4 py-2 text-sm font-medium transition-colors hover:opacity-90"
				>
					{editingJob ? t('common.save', 'Save') : t('cron.create', 'Create')}
				</button>
			</div>
		</div>
	</div>
{/if}
