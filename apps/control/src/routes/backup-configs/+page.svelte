<!-- RBAC CRUD: Backup Configs management -->
<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import AlertBanner from '$lib/components/ui/AlertBanner.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import Modal from '$lib/components/ui/Modal.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import { DatabaseBackup, Plus, Trash2, Play, Power, RefreshCw, X } from 'lucide-svelte';
	import type { DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';

	let {} = $props();

	interface BackupConfig {
		id: string;
		name: string;
		schedule: string;
		storage_type: 'local' | 's3' | 'gcs' | 'azure';
		destination: string;
		what_to_backup: string[];
		retention_days: number;
		enabled: boolean;
		last_run_at: string | null;
		created_at: string;
	}

	interface BackupTargetDef {
		key: string;
		label: string;
		description: string;
	}

	let backupTargets = $state<BackupTargetDef[]>([]);

	let configs = $state<BackupConfig[]>([]);
	let loading = $state(true);
	let message = $state('');
	let messageType = $state<'success' | 'error' | 'info'>('info');

	let page = $state(0);
	let pageSize = $state(10);
	let sortKey = $state('');
	let sortDirection = $state<'asc' | 'desc'>('asc');

	let createModalOpen = $state(false);
	let createForm = $state({
		name: '',
		schedule: '',
		storage_type: 'local' as BackupConfig['storage_type'],
		destination: '',
		what_to_backup: ['database'] as string[],
		retention_days: 30
	});
	let saving = $state(false);
	let selectedSchedulePreset = $state('daily_3am');
	let customScheduleMode = $state(false);

	interface SchedulePreset {
		value: string;
		label: string;
		cron: string;
	}

	let schedulePresets = $state<SchedulePreset[]>([]);

	async function loadSchedulePresets() {
		const result = await api<{ presets: SchedulePreset[]; tiers: string[] }>("/control/cron/schedule-presets");
		if (result.ok && result.data) {
			schedulePresets = result.data.presets;
		} else {
			// Preset catalog unavailable — fall back to raw cron input so
			// the schedule stays settable instead of a dead end.
			customScheduleMode = true;
			message = result && !result.ok ? result.error : message;
			if (message) messageType = 'error';
		}
	}

	function onSchedulePresetChange(value: string) {
		selectedSchedulePreset = value;
		if (value === 'custom' || value === '') {
			customScheduleMode = true;
			if (value === 'custom') createForm.schedule = '';
		} else {
			customScheduleMode = false;
			const preset = schedulePresets.find((p) => p.value === value);
			if (preset) createForm.schedule = preset.cron;
		}
	}

	function addBackupTarget(targetKey: string) {
		if (!createForm.what_to_backup.includes(targetKey)) {
			createForm.what_to_backup = [...createForm.what_to_backup, targetKey];
		}
	}

	// Autocomplete fetcher for the backup-target picker: offers only the
	// targets not already selected, matched against label and description.
	async function fetchBackupTargetItems(query: string): Promise<AutocompleteItem[]> {
		const q = query.trim().toLowerCase();
		return backupTargets
			.filter((t) => !createForm.what_to_backup.includes(t.key))
			.filter(
				(t) =>
					!q ||
					t.label.toLowerCase().includes(q) ||
					t.description.toLowerCase().includes(q)
			)
			.map((t) => ({ id: t.key, label: t.label, sublabel: t.description }));
	}

	function removeBackupTarget(targetKey: string) {
		createForm.what_to_backup = createForm.what_to_backup.filter((t) => t !== targetKey);
	}

	let selectedTargetLabels = $derived.by(() => {
		return createForm.what_to_backup.map((key) => {
			const def = backupTargets.find((t) => t.key === key);
			return { key, label: def?.label ?? key };
		});
	});

	let deleteModalOpen = $state(false);
	let deletingConfig = $state<BackupConfig | null>(null);

	let selectedIds = $state(new Set<string>());
	let confirmBulkDeleteOpen = $state(false);
	let pendingBulkIds = $state<string[]>([]);
	// Per-row in-flight guard for trigger/toggle: double-clicks must not
	// enqueue duplicate backups or double-toggle.
	let busyConfigs = $state(new Set<string>());

	const storageTypeOptions: { value: BackupConfig['storage_type']; label: string }[] = [
		{ value: 'local', label: 'Local' },
		{ value: 's3', label: 'S3' },
		{ value: 'gcs', label: 'GCS' },
		{ value: 'azure', label: 'Azure' }
	];

	const columns: DataTableColumn[] = [
		{
			key: 'name',
			label: t('backup_configs.name', 'Name'),
			sortable: true,
			render: (val) =>
				`<span class="text-foreground text-sm font-medium">${escapeHtml(val as string) || '—'}</span>`
		},
		{
			key: 'schedule',
			label: t('backup_configs.schedule', 'Schedule'),
			sortable: true,
			render: (val) => `<span class="font-mono text-xs">${escapeHtml(val as string) || '—'}</span>`
		},
		{
			key: 'what_to_backup',
			label: t('backup_configs.what_to_backup', 'What to Backup'),
			render: (val) => {
				const arr = Array.isArray(val) ? val : [];
				if (arr.length === 0) return `<span class="text-muted-foreground text-xs">—</span>`;
				return arr
					.map((e) => `<span class="bg-muted text-muted-foreground rounded-5px px-1.5 py-0.5 font-mono text-xs">${escapeHtml(e)}</span>`)
					.join(' ');
			}
		},
		{
			key: 'storage_type',
			label: t('backup_configs.storage_type', 'Storage'),
			render: (val) => {
				const v = escapeHtml(String(val ?? ''));
				return `<span class="bg-muted text-muted-foreground rounded-5px px-1.5 py-0.5 font-mono text-xs uppercase">${v || '—'}</span>`;
			}
		},
		{
			key: 'destination',
			label: t('backup_configs.destination', 'Destination'),
			render: (val) => {
				const v = escapeHtml(String(val ?? ''));
				return `<span class="text-muted-foreground block max-w-[200px] truncate text-xs" title="${v}">${v || '—'}</span>`;
			}
		},
		{
			key: 'retention_days',
			label: t('backup_configs.retention_days', 'Retention (days)'),
			class: 'text-end',
			render: (val) =>
				`<span class="text-foreground tabular-nums text-sm">${val ?? '—'}</span>`
		},
		{
			key: 'enabled',
			label: t('backup_configs.enabled', 'Enabled'),
			render: (val) =>
				val
					? `<span class="bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 rounded-5px px-2 py-0.5 text-xs font-medium">{t('common.enabled', 'Enabled')}</span>`
					: `<span class="bg-muted text-muted-foreground rounded-5px px-2 py-0.5 text-xs font-medium">{t('common.disabled', 'Disabled')}</span>`
		},
		{
			key: 'last_run_at',
			label: t('backup_configs.last_run', 'Last Run'),
			render: (val) =>
				`<span class="text-muted-foreground text-xs">${val ? escapeHtml(new Date(val as string).toLocaleString()) : '—'}</span>`
		},
		{
			key: 'created_at',
			label: t('backup_configs.created_at', 'Created'),
			sortable: true,
			render: (val) =>
				`<span class="text-muted-foreground text-xs">${val ? escapeHtml(new Date(val as string).toLocaleString()) : '—'}</span>`
		}
	];

	function triggerBackup(config: BackupConfig) {
		if (busyConfigs.has(config.id)) return;
		busyConfigs.add(config.id);
		void (async () => {
			message = '';
			try {
				const result = await api(`/control/backup-configs/${config.id}/trigger`, { method: 'POST' });
				if (result.ok) {
					await load();
					message = t('backup_configs.trigger_success', 'Backup triggered successfully.');
					messageType = 'success';
				} else {
					message = result.error || t('backup_configs.trigger_error', 'Failed to trigger backup.');
					messageType = 'error';
				}
			} catch {
				message = t('backup_configs.trigger_error', 'Failed to trigger backup.');
				messageType = 'error';
			} finally {
				busyConfigs.delete(config.id);
			}
		})();
	}

	function toggleEnabled(config: BackupConfig) {
		if (busyConfigs.has(config.id)) return;
		busyConfigs.add(config.id);
		void (async () => {
			message = '';
			const next = !config.enabled;
			try {
				const result = await api(`/control/backup-configs/${config.id}`, {
					method: 'PATCH',
					body: { enabled: next }
				});
				if (result.ok) {
					await load();
					message = next
						? t('backup_configs.enable_success', 'Backup config enabled.')
						: t('backup_configs.disable_success', 'Backup config disabled.');
					messageType = 'success';
				} else {
					message = result.error || t('backup_configs.toggle_error', 'Failed to update backup config.');
					messageType = 'error';
				}
			} catch {
				message = t('backup_configs.toggle_error', 'Failed to update backup config.');
				messageType = 'error';
			} finally {
				busyConfigs.delete(config.id);
			}
		})();
	}

	function confirmDelete(config: BackupConfig) {
		deletingConfig = config;
		deleteModalOpen = true;
	}

	async function handleDelete() {
		if (!deletingConfig) return;
		message = '';
		try {
			const result = await api(`/control/backup-configs/${deletingConfig.id}`, { method: 'DELETE' });
			if (result.ok) {
				await load();
				message = t('backup_configs.delete_success', 'Backup config deleted successfully.');
				messageType = 'success';
			} else {
				message = result.error || t('backup_configs.delete_error', 'Failed to delete backup config.');
				messageType = 'error';
			}
		} catch {
			message = t('backup_configs.delete_error', 'Failed to delete backup config.');
			messageType = 'error';
		} finally {
			deleteModalOpen = false;
			deletingConfig = null;
		}
	}

	function openCreateModal() {
		createForm = {
			name: '',
			schedule: '0 3 * * *',
			storage_type: 'local',
			destination: '',
			what_to_backup: ['database'],
			retention_days: 30
		};
		selectedSchedulePreset = 'daily_3am';
		customScheduleMode = false;
		createModalOpen = true;
	}

	function closeCreateModal() {
		createModalOpen = false;
		createForm = {
			name: '',
			schedule: '',
			storage_type: 'local',
			destination: '',
			what_to_backup: ['database'],
			retention_days: 30
		};
		selectedSchedulePreset = 'daily_3am';
		customScheduleMode = false;
	}

	let formValid = $derived(
		createForm.name.trim() !== '' &&
		createForm.schedule.trim() !== '' &&
		/^[\d*/\-,]+(\s[\d*/\-,]+){4}$/.test(createForm.schedule.trim()) &&
		createForm.destination.trim() !== '' &&
		createForm.retention_days > 0
	);

	async function submitCreate() {
		if (!formValid) return;
		saving = true;
		message = '';
		try {
			const body = {
				name: createForm.name.trim(),
				schedule: createForm.schedule.trim(),
				storage_type: createForm.storage_type,
				destination: createForm.destination.trim(),
				what_to_backup: createForm.what_to_backup,
				retention_days: createForm.retention_days
			};
			const result = await api<BackupConfig>('/control/backup-configs', {
				method: 'POST',
				body
			});
			if (result.ok) {
				closeCreateModal();
				await load();
				message = t('backup_configs.create_success', 'Backup config created successfully.');
				messageType = 'success';
			} else {
				message = result.error || t('backup_configs.create_error', 'Failed to create backup config.');
				messageType = 'error';
			}
		} catch {
			message = t('backup_configs.create_error', 'Failed to create backup config.');
			messageType = 'error';
		} finally {
			saving = false;
		}
	}

	async function load(quiet = false) {
		if (!quiet) loading = true;
		try {
			const result = await api<{ data: BackupConfig[] }>('/control/backup-configs');
			if (result.ok) {
				configs = Array.isArray(result.data) ? (result.data as BackupConfig[]) : (result.data?.data ?? []);
			} else {
				message = result.error || t('backup_configs.load_error', 'Failed to load backup configs.');
				messageType = 'error';
			}
		} catch {
			message = t('backup_configs.load_error', 'Failed to load backup configs.');
			messageType = 'error';
		} finally {
			loading = false;
		}
	}

	async function loadBackupTargets() {
		const result = await api<BackupTargetDef[]>('/control/backup-targets');
		if (result.ok && Array.isArray(result.data) && result.data.length > 0) {
			backupTargets = result.data;
		}
	}

	$effect(() => {
		load();
		loadBackupTargets();
		loadSchedulePresets();
		const interval = setInterval(() => {
			if (document.visibilityState === 'visible') load(true);
		}, 30000);
		return () => clearInterval(interval);
	});

	let sortedConfigs = $derived.by(() => {
		const list = [...configs];
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

	let totalRows = $derived(sortedConfigs.length);

	let paginatedConfigs = $derived.by(() => {
		const start = page * pageSize;
		return sortedConfigs.slice(start, start + pageSize);
	});

	const rowActions: DataTableRowAction[] = [
		{
			label: t('backup_configs.trigger', 'Trigger Backup'),
			icon: Play,
			onClick: (row) => triggerBackup(row as unknown as BackupConfig)
		},
		{
			label: t('backup_configs.disable', 'Disable'),
			icon: Power,
			onClick: (row) => toggleEnabled(row as unknown as BackupConfig),
			hidden: (row) => !(row as unknown as BackupConfig).enabled
		},
		{
			label: t('backup_configs.enable', 'Enable'),
			icon: Power,
			onClick: (row) => toggleEnabled(row as unknown as BackupConfig),
			hidden: (row) => (row as unknown as BackupConfig).enabled
		},
		{
			label: t('backup_configs.delete', 'Delete'),
			icon: Trash2,
			variant: 'destructive',
			onClick: (row) => confirmDelete(row as unknown as BackupConfig)
		}
	];

	const bulkActions: DataTableBulkAction[] = [
		{
			label: t('backup_configs.delete_selected', 'Delete Selected'),
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
		message = '';
		const results = await Promise.allSettled(
			ids.map((id) => api(`/control/backup-configs/${id}`, { method: 'DELETE' }))
		);
		const failed = results.filter(
			(r) => r.status === 'rejected' || (r.status === 'fulfilled' && !r.value.ok)
		).length;
		selectedIds = new Set();
		pendingBulkIds = [];
		confirmBulkDeleteOpen = false;
		await load();
		if (failed > 0) {
			message = t('backup_configs.bulk_delete_partial', '{n} config(s) could not be deleted.').replace('{n}', String(failed));
			messageType = 'error';
		} else {
			message = t('backup_configs.bulk_delete_success', 'Selected backup configs deleted.');
			messageType = 'success';
		}
	}

	// --- Autocomplete fetchers (client-side filter of loaded options) ---

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
	title={t('backup_configs.title', 'Backup Configs')}
	description={t('backup_configs.description', 'Backup configuration management')}
/>

<div class="mb-4 flex flex-wrap items-center gap-3">
	<div class="flex-1"></div>
	<button
		class="bg-foreground text-background flex items-center gap-1.5 rounded-9px px-3 py-2 text-xs font-medium transition-colors hover:opacity-90"
		onclick={openCreateModal}
	>
		<Plus class="size-3" />
		{t('backup_configs.create', 'Create Config')}
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
	rows={paginatedConfigs}
	{totalRows}
	bind:page
	bind:pageSize
	bind:sortKey
	bind:sortDirection
	{rowActions}
	{bulkActions}
	{loading}
	emptyMessage={t('backup_configs.no_configs', 'No backup configs configured.')}
	selectable={true}
	bind:selectedIds
/>

<Modal bind:open={createModalOpen} title={t('backup_configs.create_config', 'Create Backup Config')} size="lg">
	{#snippet children()}
		<div>
			<div class="space-y-4">
				<div>
					<label for="bc-name" class="text-foreground mb-1 block text-sm font-medium">
						{t('backup_configs.name', 'Name')}
					</label>
					<input
						id="bc-name"
						type="text"
						bind:value={createForm.name}
						class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder={t('backup_configs.name_ph', 'Daily DB Backup')}
					/>
				</div>
				<div>
					<label for="bc-schedule" class="text-foreground mb-1 block text-sm font-medium">
						{t('backup_configs.schedule', 'Schedule')}
					</label>
					<Autocomplete
						id="bc-schedule"
						placeholder={t('backup_configs.schedule', 'Select schedule...')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchSchedulePresets}
						initialFetcher={fetchSchedulePresetsInitial}
						selectedId={selectedSchedulePreset || null}
						selectedLabel={getSchedulePresetSelectedLabel(selectedSchedulePreset)}
						onSelect={(item) => onSchedulePresetChange(item.id)}
						onClear={() => onSchedulePresetChange('')}
					/>
					{#if customScheduleMode}
						<input
							id="bc-schedule-custom"
							type="text"
							bind:value={createForm.schedule}
							class="border-border-input bg-background text-foreground mt-2 w-full rounded-9px border px-3 py-2 font-mono text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
							placeholder="0 3 * * *"
						/>
					{/if}
					<p class="text-muted-foreground mt-1 text-xs">
						{customScheduleMode
							? t('backup_configs.cron_expression', 'Cron expression (e.g. 0 3 * * *)')
							: t('backup_configs.schedule_hint', 'Select a preset or choose custom for a cron expression.')}
					</p>
				</div>
				<div>
					<label for="backup-targets" class="text-foreground mb-1 block text-sm font-medium">
						{t('backup_configs.what_to_backup', 'What to Backup')}
					</label>

					{#if selectedTargetLabels.length > 0}
						<div class="mb-2 flex flex-wrap gap-1.5">
							{#each selectedTargetLabels as target (target.key)}
								<span class="bg-muted text-muted-foreground flex items-center gap-1 rounded-5px px-2 py-0.5 font-mono text-xs">
									{target.label}
									<button
										type="button"
										class="text-muted-foreground hover:text-foreground"
										onclick={() => removeBackupTarget(target.key)}
										aria-label={t('backup_configs.remove_target', 'Remove {label}').replace('{label}', target.label)}
									>
										<X class="size-3" />
									</button>
								</span>
							{/each}
						</div>
					{/if}

					<Autocomplete
						id="backup-targets"
						placeholder={createForm.what_to_backup.length === 0
							? t('backup_configs.select_targets', 'Select what to backup...')
							: t('backup_configs.targets_selected', '{count} selected').replace('{count}', String(createForm.what_to_backup.length))}
						emptyMessage={t('backup_configs.no_targets', 'No backup targets found')}
						ariaLabel={t('backup_configs.what_to_backup', 'What to Backup')}
						fetcher={fetchBackupTargetItems}
						initialFetcher={() => fetchBackupTargetItems('')}
						selectedId={null}
						onSelect={(item) => addBackupTarget(item.id)}
					/>
					<p class="text-muted-foreground mt-1 text-xs">
						{t('backup_configs.what_to_backup_hint', 'Select which components to include in the backup.')}
					</p>
				</div>
				<div>
					<label for="bc-storage" class="text-foreground mb-1 block text-sm font-medium">
						{t('backup_configs.storage_type', 'Storage Type')}
					</label>
					<AutocompleteSelect
						id="bc-storage"
						value={createForm.storage_type}
						onValueChange={(v) => { createForm.storage_type = v as BackupConfig['storage_type']; }}
						options={storageTypeOptions}
						placeholder={t('backup_configs.select_storage', 'Select storage type...')}
						ariaLabel={t('backup_configs.storage_type', 'Storage Type')}
						class="w-full"
					/>
				</div>
				<div>
					<label for="bc-destination" class="text-foreground mb-1 block text-sm font-medium">
						{t('backup_configs.destination', 'Destination')}
					</label>
					<input
						id="bc-destination"
						type="text"
						bind:value={createForm.destination}
						class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 font-mono text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder="/backups or s3://my-bucket/backups"
					/>
					<p class="text-muted-foreground mt-1 text-xs">
						{t('backup_configs.destination_hint', 'Local path or cloud storage URL')}
					</p>
				</div>
				<div>
					<label for="bc-retention" class="text-foreground mb-1 block text-sm font-medium">
						{t('backup_configs.retention_days', 'Retention (days)')}
					</label>
					<input
						id="bc-retention"
						type="number"
						min="1"
						bind:value={createForm.retention_days}
						class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder="30"
					/>
				</div>
			</div>
			<div class="mt-6 flex justify-end gap-3">
				<button
					type="button"
					class="border-border-input bg-background hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
					onclick={closeCreateModal}
				>
					{t('common.cancel', 'Cancel')}
				</button>
				<button
					class="bg-foreground text-background rounded-9px px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
					onclick={submitCreate}
					disabled={saving || !formValid}
				>
					{saving
						? t('common.saving', 'Saving...')
						: t('backup_configs.create', 'Create')}
				</button>
			</div>
		</div>
	{/snippet}
</Modal>

<ConfirmModal
	bind:open={deleteModalOpen}
	title={t('backup_configs.delete_config', 'Delete Backup Config')}
	consequence={deletingConfig
		? t('backup_configs.delete_confirm', 'This will permanently delete the backup config "{name}". This action cannot be undone.').replace('{name}', deletingConfig.name)
		: ''}
	confirmText="DELETE"
	onConfirm={handleDelete}
/>

<ConfirmModal
	bind:open={confirmBulkDeleteOpen}
	title={t('backup_configs.bulk_delete_title', 'Delete Selected Configs')}
	consequence={t('backup_configs.bulk_delete_confirm', 'Are you sure you want to delete the selected backup configs? This action cannot be undone.')}
	confirmText="DELETE"
	onConfirm={confirmBulkDelete}
	onCancel={() => { confirmBulkDeleteOpen = false; pendingBulkIds = []; }}
/>
