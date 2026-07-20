<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import AlertBanner from '$lib/components/ui/AlertBanner.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { Dialog, Select } from 'bits-ui';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import { Play, Pause, PlayCircle, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-svelte';
	import type { DataTableColumn, DataTableRowAction } from '$lib/api/types';

	let {} = $props();

	interface CronJob {
		id: string;
		type: string;
		spec: string;
		source: 'system' | 'custom';
		paused: boolean;
		payload?: string;
		next_run_at: string | null;
		prev_run_at: string | null;
	}

	let jobs = $state<CronJob[]>([]);
	let loading = $state(true);
	let message = $state('');
	let messageType = $state<'success' | 'error' | 'info'>('info');
	let filterStatus = $state('all');

	let page = $state(0);
	let pageSize = $state(10);
	let sortKey = $state('');
	let sortDirection = $state<'asc' | 'desc'>('asc');

	let editModalOpen = $state(false);
	let editingJob = $state<CronJob | null>(null);
	let editForm = $state({ type: '', spec: '', payload: '{}' });
	let saving = $state(false);

	let deleteModalOpen = $state(false);
	let deletingJob = $state<CronJob | null>(null);

	const statusItems = [
		{ value: 'all', label: t('cron.all_status', 'All Status') },
		{ value: 'active', label: t('cron.active', 'Active') },
		{ value: 'paused', label: t('cron.paused', 'Paused') }
	];

	const columns: DataTableColumn[] = [
		{
			key: 'type',
			label: t('cron.type', 'Type'),
			sortable: true,
			render: (val) =>
				`<span class="bg-muted text-muted-foreground rounded-5px px-1.5 py-0.5 font-mono text-xs">${escapeHtml(val) || '—'}</span>`
		},
		{
			key: 'spec',
			label: t('cron.schedule', 'Schedule'),
			sortable: true,
			render: (val) => `<span class="font-mono text-xs">${escapeHtml(val) || '—'}</span>`
		},
		{
			key: 'source',
			label: t('cron.source', 'Source'),
			sortable: true,
			render: (val) =>
				val === 'system'
					? `<span class="text-muted-foreground text-xs">System</span>`
					: `<span class="text-accent-foreground text-xs font-medium">Custom</span>`
		},
		{
			key: 'prev_run_at',
			label: t('cron.last_run', 'Last Run'),
			sortable: true,
			render: (val) =>
				`<span class="text-muted-foreground text-xs">${val ? escapeHtml(new Date(val as string).toLocaleString()) : '—'}</span>`
		},
		{
			key: 'next_run_at',
			label: t('cron.next_run', 'Next Run'),
			sortable: true,
			render: (val) =>
				`<span class="text-muted-foreground text-xs">${val ? escapeHtml(new Date(val as string).toLocaleString()) : '—'}</span>`
		}
	];

	function triggerJob(job: CronJob) {
		void (async () => {
			message = '';
			const result = await api(`/control/cron/jobs/${job.id}/trigger`, { method: 'POST' });
			if (result.ok) {
				await load();
				message = t('cron.trigger_success', 'Job triggered successfully.');
				messageType = 'success';
			} else {
				message = result.error || t('cron.trigger_error', 'Failed to trigger job.');
				messageType = 'error';
			}
		})();
	}

	function pauseJob(job: CronJob) {
		void (async () => {
			message = '';
			const result = await api(`/control/cron/jobs/${job.id}/pause`, { method: 'POST' });
			if (result.ok) {
				await load();
				message = t('cron.pause_success', 'Job paused successfully.');
				messageType = 'success';
			} else {
				message = result.error || t('cron.pause_error', 'Failed to pause job.');
				messageType = 'error';
			}
		})();
	}

	function unpauseJob(job: CronJob) {
		void (async () => {
			message = '';
			const result = await api(`/control/cron/jobs/${job.id}/unpause`, { method: 'POST' });
			if (result.ok) {
				await load();
				message = t('cron.unpause_success', 'Job resumed successfully.');
				messageType = 'success';
			} else {
				message = result.error || t('cron.unpause_error', 'Failed to resume job.');
				messageType = 'error';
			}
		})();
	}

	function openEditModal(job?: CronJob) {
		if (job) {
			editingJob = job;
			editForm = {
				type: job.type,
				spec: job.spec,
				payload: job.payload || '{}'
			};
		} else {
			editingJob = null;
			editForm = { type: '', spec: '', payload: '{}' };
		}
		editModalOpen = true;
	}

	function closeEditModal() {
		editModalOpen = false;
		editingJob = null;
		editForm = { type: '', spec: '', payload: '{}' };
	}

	let formValid = $derived(editForm.type.trim() !== '' && editForm.spec.trim() !== '' && /^(\*\/\d+|\d+\s|\d+\/\d+|[\d,*/-])+/.test(editForm.spec.trim()));

	async function submitJob() {
		if (!formValid) return;
		// Validate payload JSON
		try {
			JSON.parse(editForm.payload);
		} catch {
			message = t('cron.invalid_payload', 'Payload must be valid JSON.');
			messageType = 'error';
			return;
		}

		saving = true;
		message = '';
		const body = { type: editForm.type.trim(), spec: editForm.spec.trim(), payload: editForm.payload };
		if (editingJob) {
			const result = await api(`/control/cron/jobs/${editingJob.id}`, {
				method: 'PATCH',
				body
			});
			if (result.ok) {
				closeEditModal();
				await load();
				message = t('cron.update_success', 'Job updated successfully.');
				messageType = 'success';
			} else {
				message = result.error || t('cron.update_error', 'Failed to update job.');
				messageType = 'error';
			}
		} else {
			const result = await api<CronJob>('/control/cron/jobs', {
				method: 'POST',
				body
			});
			if (result.ok) {
				closeEditModal();
				await load();
				message = t('cron.create_success', 'Job created successfully.');
				messageType = 'success';
			} else {
				message = result.error || t('cron.create_error', 'Failed to create job.');
				messageType = 'error';
			}
		}
		saving = false;
	}

	function confirmDelete(job: CronJob) {
		deletingJob = job;
		deleteModalOpen = true;
	}

	async function handleDelete() {
		if (!deletingJob) return;
		message = '';
		const result = await api<{ id: string; status: string }>(`/control/cron/jobs/${deletingJob.id}`, { method: 'DELETE' });
		if (result.ok) {
			deleteModalOpen = false;
			deletingJob = null;
			await load();
			message = result.data.status === 'paused'
				? t('cron.delete_system_note', 'System job paused (cannot be deleted).')
				: t('cron.delete_success', 'Job deleted successfully.');
			messageType = 'success';
		} else {
			message = result.error || t('cron.delete_error', 'Failed to delete job.');
			messageType = 'error';
			deleteModalOpen = false;
			deletingJob = null;
		}
	}

	async function load() {
		loading = true;
		try {
			const result = await api<CronJob[]>('/control/cron/jobs');
			if (result.ok) {
				jobs = result.data;
			} else {
				message = result.error || t('cron.load_error', 'Failed to load cron jobs.');
				messageType = 'error';
			}
		} catch {
			message = t('cron.load_error', 'Failed to load cron jobs.');
			messageType = 'error';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
	});

	let filteredJobs = $derived(
		filterStatus !== 'all'
			? jobs.filter((j) => {
					if (filterStatus === 'active') return !j.paused;
					if (filterStatus === 'paused') return j.paused;
					return true;
				})
			: jobs
	);

	let sortedJobs = $derived.by(() => {
		const list = [...filteredJobs];
		if (sortKey) {
			list.sort((a, b) => {
				const row = a as unknown as Record<string, unknown>;
				const row2 = b as unknown as Record<string, unknown>;
				const av = row[sortKey];
				const bv = row2[sortKey];
				if (av == null && bv == null) return 0;
				if (av == null) return 1;
				if (bv == null) return -1;
				const cmp = String(av).localeCompare(String(bv));
				return sortDirection === 'asc' ? cmp : -cmp;
			});
		}
		return list;
	});

	let totalRows = $derived(sortedJobs.length);

	let paginatedJobs = $derived.by(() => {
		const start = page * pageSize;
		return sortedJobs.slice(start, start + pageSize);
	});

	const rowActions: DataTableRowAction[] = [
		{
			label: t('cron.run_now', 'Run now'),
			icon: Play,
			onClick: (row) => triggerJob(row as unknown as CronJob)
		},
		{
			label: t('cron.edit', 'Edit'),
			icon: Pencil,
			onClick: (row) => openEditModal(row as unknown as CronJob),
			disabled: (row) => (row as unknown as CronJob).source !== 'custom'
		},
		{
			label: t('cron.pause', 'Pause'),
			icon: Pause,
			onClick: (row) => pauseJob(row as unknown as CronJob),
			disabled: (row) => (row as unknown as CronJob).paused
		},
		{
			label: t('cron.resume', 'Resume'),
			icon: PlayCircle,
			onClick: (row) => unpauseJob(row as unknown as CronJob),
			disabled: (row) => !(row as unknown as CronJob).paused
		},
		{
			label: t('cron.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onClick: (row) => confirmDelete(row as unknown as CronJob)
		}
	];
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
			<Select.Content
				class="border-border-input bg-background shadow-popover z-50 rounded-xl border px-1 py-1"
				sideOffset={4}
			>
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
		class="bg-foreground text-background flex items-center gap-1.5 rounded-9px px-3 py-2 text-xs font-medium transition-colors hover:opacity-90"
		onclick={() => openEditModal()}
	>
		<Plus class="size-3" />
		{t('cron.create', 'Create Job')}
	</button>
	<button
		class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px flex items-center gap-1.5 border px-3 py-2 text-xs font-medium transition-colors"
		onclick={() => { message = ''; load(); }}
		disabled={loading}
	>
		<RefreshCw class="size-3 {loading ? 'animate-spin' : ''}" />
		{t('common.refresh', 'Refresh')}
	</button>
</div>

{#if message}
	<div class="mb-4">
		<AlertBanner variant={messageType} {message} />
	</div>
{/if}

<DataTable
	{columns}
	rows={paginatedJobs}
	{totalRows}
	bind:page
	bind:pageSize
	bind:sortKey
	bind:sortDirection
	{rowActions}
	{loading}
	emptyMessage={t('cron.no_jobs', 'No cron jobs configured.')}
	selectable={false}
/>

<Dialog.Root bind:open={editModalOpen}>
	<Dialog.Portal>
		<Dialog.Overlay
			class="bg-black/50 data-[state=open]:animate-in data-[state=open]:fade-in data-[state=closed]:animate-out data-[state=closed]:fade-out fixed inset-0 z-50"
		/>
		<Dialog.Content
			class="border-border-card bg-background shadow-popover fixed left-1/2 top-1/2 z-50 w-full max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6 focus:outline-none"
			role="dialog"
			aria-modal="true"
			onOpenAutoFocus={(e) => e.preventDefault()}
		>
			<Dialog.Title class="text-foreground text-lg font-semibold">
				{editingJob ? t('cron.edit_job', 'Edit Job') : t('cron.create_job', 'Create Job')}
			</Dialog.Title>
			<div class="mt-4 space-y-4">
				<div>
					<label for="cron-type" class="text-foreground mb-1 block text-sm font-medium">
						{t('cron.job_type', 'Job Type')}
					</label>
					<input
						id="cron-type"
						type="text"
						bind:value={editForm.type}
						class="border-border-input bg-background text-foreground w-full rounded-5px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder="email:send"
					/>
				</div>
				<div>
					<label for="cron-spec" class="text-foreground mb-1 block text-sm font-medium">
						{t('cron.schedule', 'Schedule')}
					</label>
					<input
						id="cron-spec"
						type="text"
						bind:value={editForm.spec}
						class="border-border-input bg-background text-foreground w-full rounded-5px border px-3 py-2 font-mono text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder="0 */6 * * *"
					/>
					<p class="text-muted-foreground mt-1 text-xs">
						{t('cron.cron_expression', 'Cron expression (e.g. 0 */6 * * *)')}
					</p>
				</div>
				<div>
					<label for="cron-payload" class="text-foreground mb-1 block text-sm font-medium">
						{t('cron.payload', 'Payload')} (JSON)
					</label>
					<textarea
						id="cron-payload"
						bind:value={editForm.payload}
						rows="4"
						class="border-border-input bg-background text-foreground w-full rounded-5px border px-3 py-2 font-mono text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder={'{"key": "value"}'}
					></textarea>
				</div>
			</div>
			<div class="mt-6 flex justify-end gap-3">
				<Dialog.Close
					class="border-border-input bg-background hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
					onclick={closeEditModal}
				>
					{t('common.cancel', 'Cancel')}
				</Dialog.Close>
				<button
					class="bg-foreground text-background rounded-9px px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
					onclick={submitJob}
					disabled={saving || !formValid}
				>
					{saving
						? t('common.saving', 'Saving...')
						: editingJob
							? t('common.save', 'Save')
							: t('cron.create', 'Create')}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<ConfirmModal
	bind:open={deleteModalOpen}
	title={t('cron.delete_job', 'Delete Job')}
	consequence={deletingJob
		? deletingJob.source === 'system'
			? t('cron.delete_system_confirm', 'System jobs cannot be deleted. The job will be paused instead.')
			: t('cron.delete_confirm', 'This will permanently delete the cron job. This action cannot be undone.')
		: ''}
	confirmText="DELETE"
	onConfirm={handleDelete}
/>
