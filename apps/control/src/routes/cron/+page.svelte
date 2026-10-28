<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import AlertBanner from '$lib/components/ui/AlertBanner.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import Modal from '$lib/components/ui/Modal.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { api } from '$lib/api';
	import { t, translations } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import { Play, Pause, PlayCircle, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-svelte';
	import type { DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';

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
	let selectedSchedulePreset = $state('');
	let customScheduleMode = $state(false);

	let deleteModalOpen = $state(false);
	let deletingJob = $state<CronJob | null>(null);
	// H7: Bulk selection state for cron jobs
	let selectedIds = $state(new Set<string>());
	let confirmBulkDeleteOpen = $state(false);
	let pendingBulkIds = $state<string[]>([]);
	// Per-job in-flight guard for trigger/pause/unpause: double-clicks must
	// not double-enqueue or double-toggle.
	let busyJobs = $state(new Set<string>());

	const statusItems = $derived.by(() => {
		translations();
		const seen = new Set<string>();
		for (const j of jobs) {
			seen.add(j.paused ? 'paused' : 'active');
		}
		const items = [{ value: 'all', label: t('cron.all_status', 'All Status') }];
		if (seen.has('active')) items.push({ value: 'active', label: t('cron.active', 'Active') });
		if (seen.has('paused')) items.push({ value: 'paused', label: t('cron.paused', 'Paused') });
		return items;
	});

	$effect(() => {
		if (filterStatus !== 'all' && !statusItems.some((i) => i.value === filterStatus)) {
			filterStatus = 'all';
		}
	});

	interface TaskTypeOption {
		value: string;
		label: string;
		description: string;
		tier: 'critical' | 'default' | 'low';
	}
	let taskTypeOptions = $state<TaskTypeOption[]>([]);

	async function loadTaskTypes() {
		const result = await api<{ value: string; label: string; tier: string; description: string }[]>("/control/task-types");
		if (result.ok && Array.isArray(result.data)) {
			taskTypeOptions = result.data.map((t) => ({
				value: t.value,
				label: t.label,
				description: t.description,
				tier: t.tier as "critical" | "default" | "low",
			}));
		}
	}

	interface SchedulePreset {
		value: string;
		label: string;
		cron: string;
	}

	let schedulePresets = $state<SchedulePreset[]>([]);
	let queueTiers = $state<string[]>(['critical', 'default', 'low']);

	async function loadSchedulePresets() {
		const result = await api<{ presets: SchedulePreset[]; tiers: string[] }>("/control/cron/schedule-presets");
		if (result.ok && result.data) {
			schedulePresets = result.data.presets;
			queueTiers = result.data.tiers;
		}
	}

	const columns = $derived.by<DataTableColumn[]>(() => {
		translations();
		return [
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
					? `<span class="text-muted-foreground text-xs">${t('common.system', 'System')}</span>`
					: `<span class="text-accent-foreground text-xs font-medium">${t('common.custom', 'Custom')}</span>`
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
	});

	function triggerJob(job: CronJob) {
		if (busyJobs.has(job.id)) return;
		busyJobs.add(job.id);
		void (async () => {
			message = '';
			try {
				const result = await api(`/control/cron/jobs/${encodeURIComponent(job.id)}/trigger`, { method: 'POST' });
				if (result.ok) {
					await load(true);
					message = t('cron.trigger_success', 'Job triggered successfully.');
					messageType = 'success';
				} else {
					message = result.error || t('cron.trigger_error', 'Failed to trigger job.');
					messageType = 'error';
				}
			} catch {
				message = t('cron.trigger_error', 'Failed to trigger job.');
				messageType = 'error';
			} finally {
				busyJobs.delete(job.id);
			}
		})();
	}

	function pauseJob(job: CronJob) {
		if (busyJobs.has(job.id)) return;
		busyJobs.add(job.id);
		void (async () => {
			message = '';
			try {
				const result = await api(`/control/cron/jobs/${encodeURIComponent(job.id)}/pause`, { method: 'POST' });
				if (result.ok) {
					await load(true);
					message = t('cron.pause_success', 'Job paused successfully.');
					messageType = 'success';
				} else {
					message = result.error || t('cron.pause_error', 'Failed to pause job.');
					messageType = 'error';
				}
			} catch {
				message = t('cron.pause_error', 'Failed to pause job.');
				messageType = 'error';
			} finally {
				busyJobs.delete(job.id);
			}
		})();
	}

	function unpauseJob(job: CronJob) {
		if (busyJobs.has(job.id)) return;
		busyJobs.add(job.id);
		void (async () => {
			message = '';
			try {
				const result = await api(`/control/cron/jobs/${encodeURIComponent(job.id)}/unpause`, { method: 'POST' });
				if (result.ok) {
					await load(true);
					message = t('cron.unpause_success', 'Job resumed successfully.');
					messageType = 'success';
				} else {
					message = result.error || t('cron.unpause_error', 'Failed to resume job.');
					messageType = 'error';
				}
			} catch {
				message = t('cron.unpause_error', 'Failed to resume job.');
				messageType = 'error';
			} finally {
				busyJobs.delete(job.id);
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
			const match = schedulePresets.find((p) => p.cron === job.spec && p.value !== 'custom');
			if (match) {
				selectedSchedulePreset = match.value;
				customScheduleMode = false;
			} else {
				selectedSchedulePreset = 'custom';
				customScheduleMode = true;
			}
		} else {
			editingJob = null;
			editForm = { type: '', spec: '', payload: '{}' };
			selectedSchedulePreset = '';
			customScheduleMode = false;
		}
		editModalOpen = true;
	}

	function closeEditModal() {
		editModalOpen = false;
		editingJob = null;
		editForm = { type: '', spec: '', payload: '{}' };
		selectedSchedulePreset = '';
		customScheduleMode = false;
	}

	let formValid = $derived(editForm.type.trim() !== '' && editForm.spec.trim() !== '' && /^[\d*/\-,]+(\s[\d*/\-,]+){4}$/.test(editForm.spec.trim()));

	function handleSchedulePresetChange(value: string) {
		selectedSchedulePreset = value;
		if (value === 'custom') {
			customScheduleMode = true;
			editForm.spec = '';
		} else {
			customScheduleMode = false;
			const preset = schedulePresets.find((p) => p.value === value);
			if (preset) editForm.spec = preset.cron;
		}
	}

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
		try {
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
		} catch {
			message = t('cron.create_error', 'Failed to create job.');
			messageType = 'error';
		} finally {
			saving = false;
		}
	}

	function confirmDelete(job: CronJob) {
		deletingJob = job;
		deleteModalOpen = true;
	}

	async function handleDelete() {
		if (!deletingJob) return;
		message = '';
		try {
			const result = await api<{ id: string; status: string }>(`/control/cron/jobs/${deletingJob.id}`, { method: 'DELETE' });
			if (result.ok) {
				await load();
				message = result.data.status === 'paused'
					? t('cron.delete_system_note', 'System job paused (cannot be deleted).')
					: t('cron.delete_success', 'Job deleted successfully.');
				messageType = 'success';
			} else {
				message = result.error || t('cron.delete_error', 'Failed to delete job.');
				messageType = 'error';
			}
		} catch {
			message = t('cron.delete_error', 'Failed to delete job.');
			messageType = 'error';
		} finally {
			deleteModalOpen = false;
			deletingJob = null;
		}
	}

	async function load(quiet = false) {
		if (!quiet) loading = true;
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
		loadTaskTypes();
		loadSchedulePresets();
		const interval = setInterval(() => {
			if (document.visibilityState === 'visible') {
				load(true);
			}
		}, 15_000);
		return () => clearInterval(interval);
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

	// DataTable owns pagination — it receives the full sorted list below.
	// (Parent-side slicing would paginate twice and misorder rows.)

	const rowActions: DataTableRowAction[] = [
		{
			label: t('cron.run_now', 'Run now'),
			icon: Play,
			onClick: (row) => triggerJob(row as unknown as CronJob),
			disabled: (row) => busyJobs.has((row as unknown as CronJob).id)
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
			disabled: (row) => (row as unknown as CronJob).paused || busyJobs.has((row as unknown as CronJob).id)
		},
		{
			label: t('cron.resume', 'Resume'),
			icon: PlayCircle,
			onClick: (row) => unpauseJob(row as unknown as CronJob),
			disabled: (row) => !(row as unknown as CronJob).paused || busyJobs.has((row as unknown as CronJob).id)
		},
		{
			label: t('cron.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onClick: (row) => confirmDelete(row as unknown as CronJob)
		}
	];

	// H7: Bulk actions for cron jobs
	const bulkActions: DataTableBulkAction[] = [
		{
			label: t('cron.pause_selected', 'Pause Selected'),
			icon: Pause,
			onClick: async (ids: string[]) => {
				const results = await Promise.allSettled(
					ids.map((id) => api(`/control/cron/jobs/${encodeURIComponent(id)}/pause`, { method: 'POST' }))
				);
				const failed = results.filter(
					(r) => r.status === 'rejected' || (r.status === 'fulfilled' && !r.value.ok)
				).length;
				selectedIds = new Set();
				await load(true);
				if (failed > 0) {
					message = t('cron.bulk_pause_partial', '{n} job(s) could not be paused.').replace('{n}', String(failed));
					messageType = 'error';
				} else {
					message = t('cron.bulk_pause_success', 'Selected jobs paused.');
					messageType = 'success';
				}
			}
		},
		{
			label: t('cron.resume_selected', 'Resume Selected'),
			icon: PlayCircle,
			onClick: async (ids: string[]) => {
				const results = await Promise.allSettled(
					ids.map((id) => api(`/control/cron/jobs/${encodeURIComponent(id)}/unpause`, { method: 'POST' }))
				);
				const failed = results.filter(
					(r) => r.status === 'rejected' || (r.status === 'fulfilled' && !r.value.ok)
				).length;
				selectedIds = new Set();
				await load(true);
				if (failed > 0) {
					message = t('cron.bulk_resume_partial', '{n} job(s) could not be resumed.').replace('{n}', String(failed));
					messageType = 'error';
				} else {
					message = t('cron.bulk_resume_success', 'Selected jobs resumed.');
					messageType = 'success';
				}
			}
		},
		{
			label: t('cron.delete_selected', 'Delete Selected'),
			icon: Trash2,
			variant: 'destructive',
			onClick: (ids: string[]) => {
				pendingBulkIds = ids;
				confirmBulkDeleteOpen = true;
			}
		}
	];

	async function confirmBulkDelete() {
		const ids = pendingBulkIds;
		const results = await Promise.allSettled(
			ids.map((id) => api(`/control/cron/jobs/${encodeURIComponent(id)}`, { method: 'DELETE' }))
		);
		const failed = results.filter(
			(r) => r.status === 'rejected' || (r.status === 'fulfilled' && !r.value.ok)
		).length;
		selectedIds = new Set();
		pendingBulkIds = [];
		confirmBulkDeleteOpen = false;
		await load(true);
		if (failed > 0) {
			message = t('cron.bulk_delete_partial', '{n} job(s) could not be deleted.').replace('{n}', String(failed));
			messageType = 'error';
		} else {
			message = t('cron.bulk_delete_success', 'Selected jobs deleted.');
			messageType = 'success';
		}
	}

	// --- Autocomplete fetchers (client-side filter of loaded options) ---

	// Job Type fetcher — filters the server-fetched taskTypeOptions list.
	async function fetchTaskTypes(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		return taskTypeOptions
			.filter((o) => o.label.toLowerCase().includes(query) || o.value.toLowerCase().includes(query))
			.map((o) => ({ id: o.value, label: o.label, sublabel: o.tier }));
	}

	async function fetchTaskTypesInitial(): Promise<AutocompleteItem[]> {
		return taskTypeOptions.map((o) => ({ id: o.value, label: o.label, sublabel: o.tier }));
	}

	function getTaskTypeSelectedLabel(value: string): string | null {
		if (!value) return null;
		return taskTypeOptions.find((o) => o.value === value)?.label ?? value;
	}

	// Schedule Preset fetcher — filters the server-fetched schedulePresets list.
	async function fetchSchedulePresets(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		return schedulePresets
			.filter((p) => p.label.toLowerCase().includes(query) || p.value.toLowerCase().includes(query))
			.map((p) => ({ id: p.value, label: p.label, sublabel: p.cron }));
	}

	async function fetchSchedulePresetsInitial(): Promise<AutocompleteItem[]> {
		return schedulePresets.map((p) => ({ id: p.value, label: p.label, sublabel: p.cron }));
	}

	function getSchedulePresetSelectedLabel(value: string): string | null {
		if (!value) return null;
		return schedulePresets.find((p) => p.value === value)?.label ?? value;
	}
</script>

<PageHeader
	title={t('cron.title', 'Cron Jobs')}
	description={t('cron.description', 'Scheduled task management')}
/>

<div class="mb-4 flex flex-wrap items-center gap-3">
	<AutocompleteSelect
		bind:value={filterStatus}
		options={statusItems}
		placeholder={t('cron.all_status', 'All Status')}
		ariaLabel={t('cron.filter_status', 'Filter by status')}
		class="w-44"
	/>
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
	rows={sortedJobs}
	{totalRows}
	bind:page
	bind:pageSize
	bind:sortKey
	bind:sortDirection
	{rowActions}
	{bulkActions}
	{loading}
	emptyMessage={t('cron.no_jobs', 'No cron jobs configured.')}
	selectable={true}
	bind:selectedIds
/>

<Modal bind:open={editModalOpen} title={editingJob ? t('cron.edit_job', 'Edit Job') : t('cron.create_job', 'Create Job')} size="lg">
			<div class="mt-4 space-y-4">
				<div>
					<label for="cron-type" class="text-foreground mb-1 block text-sm font-medium">
						{t('cron.job_type', 'Job Type')}
					</label>
					<Autocomplete
						id="cron-type"
						placeholder={t('cron.select_task_type', 'Select a task type...')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchTaskTypes}
						initialFetcher={fetchTaskTypesInitial}
						selectedId={editForm.type || null}
						selectedLabel={getTaskTypeSelectedLabel(editForm.type)}
						onSelect={(item) => (editForm.type = item.id)}
						onClear={() => (editForm.type = '')}
						inputClass="font-mono"
					/>
					<p class="text-muted-foreground mt-1 text-xs">
						{t('cron.job_type_hint', 'The Asynq task type to execute')}
					</p>
				</div>
				<div>
					<label for="cron-schedule-preset" class="text-foreground mb-1 block text-sm font-medium">
						{t('cron.schedule', 'Schedule')}
					</label>
					<Autocomplete
						id="cron-schedule-preset"
						placeholder={t('cron.select_schedule', 'Select a schedule...')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchSchedulePresets}
						initialFetcher={fetchSchedulePresetsInitial}
						selectedId={selectedSchedulePreset || null}
						selectedLabel={getSchedulePresetSelectedLabel(selectedSchedulePreset)}
						onSelect={(item) => handleSchedulePresetChange(item.id)}
						onClear={() => handleSchedulePresetChange('')}
					/>
					{#if customScheduleMode}
						<input
							id="cron-spec"
							type="text"
							bind:value={editForm.spec}
							class="border-border-input bg-background text-foreground mt-2 w-full rounded-5px border px-3 py-2 font-mono text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
							placeholder="0 */6 * * *"
						/>
						<p class="text-muted-foreground mt-1 text-xs">
							{t('cron.cron_expression', 'Cron expression (e.g. 0 */6 * * *)')}
						</p>
					{:else if editForm.spec}
						<p class="text-muted-foreground mt-1 font-mono text-xs">
							{editForm.spec}
						</p>
					{/if}
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
				<button
					type="button"
					class="border-border-input bg-background hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
					onclick={closeEditModal}
				>
					{t('common.cancel', 'Cancel')}
				</button>
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
</Modal>

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

<!-- H7: Bulk delete confirmation for cron jobs -->
<ConfirmModal
	bind:open={confirmBulkDeleteOpen}
	title={t('cron.bulk_delete_title', 'Delete Selected Jobs')}
	consequence={t('cron.bulk_delete_confirm', 'Are you sure you want to delete the selected cron jobs? This action cannot be undone.')}
	confirmText="DELETE"
	onConfirm={confirmBulkDelete}
	onCancel={() => { confirmBulkDeleteOpen = false; pendingBulkIds = []; }}
/>
