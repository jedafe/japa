<!-- RBAC CRUD: Backup Configs management -->
<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import AlertBanner from '$lib/components/ui/AlertBanner.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { Dialog, Select, Popover } from 'bits-ui';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import { DatabaseBackup, Plus, Trash2, Play, Power, RefreshCw, ChevronDown, Check, X } from 'lucide-svelte';
	import type { DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';

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

	const schedulePresets: SchedulePreset[] = [
		{ value: 'every_minute', label: 'Every minute', cron: '* * * * *' },
		{ value: 'every_5min', label: 'Every 5 minutes', cron: '*/5 * * * *' },
		{ value: 'every_15min', label: 'Every 15 minutes', cron: '*/15 * * * *' },
		{ value: 'every_30min', label: 'Every 30 minutes', cron: '*/30 * * * *' },
		{ value: 'hourly', label: 'Every hour', cron: '0 * * * *' },
		{ value: 'every_6h', label: 'Every 6 hours', cron: '0 */6 * * *' },
		{ value: 'daily_3am', label: 'Daily at 03:00', cron: '0 3 * * *' },
		{ value: 'daily_midnight', label: 'Daily at 00:00', cron: '0 0 * * *' },
		{ value: 'weekly_sun_2am', label: 'Weekly (Sun 02:00)', cron: '0 2 * * 0' },
		{ value: 'monthly_1st_2am', label: 'Monthly (1st 02:00)', cron: '0 2 1 * *' },
		{ value: 'custom', label: 'Custom cron expression...', cron: '' }
	];

	function onSchedulePresetChange(value: string) {
		selectedSchedulePreset = value;
		if (value === 'custom') {
			customScheduleMode = true;
			createForm.schedule = '';
		} else {
			customScheduleMode = false;
			const preset = schedulePresets.find((p) => p.value === value);
			if (preset) createForm.schedule = preset.cron;
		}
	}

	function toggleBackupTarget(targetKey: string) {
		if (createForm.what_to_backup.includes(targetKey)) {
			createForm.what_to_backup = createForm.what_to_backup.filter((t) => t !== targetKey);
		} else {
			createForm.what_to_backup = [...createForm.what_to_backup, targetKey];
		}
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
			class: 'text-right',
			render: (val) =>
				`<span class="text-foreground tabular-nums text-sm">${val ?? '—'}</span>`
		},
		{
			key: 'enabled',
			label: t('backup_configs.enabled', 'Enabled'),
			render: (val) =>
				val
					? `<span class="bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 rounded-5px px-2 py-0.5 text-xs font-medium">Enabled</span>`
					: `<span class="bg-muted text-muted-foreground rounded-5px px-2 py-0.5 text-xs font-medium">Disabled</span>`
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
			}
		})();
	}

	function toggleEnabled(config: BackupConfig) {
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

	async function load() {
		loading = true;
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
		const interval = setInterval(() => {
			load();
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
		await Promise.allSettled(
			ids.map((id) => api(`/control/backup-configs/${id}`, { method: 'DELETE' }))
		);
		selectedIds = new Set();
		pendingBulkIds = [];
		confirmBulkDeleteOpen = false;
		await load();
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

<Dialog.Root bind:open={createModalOpen}>
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
				{t('backup_configs.create_config', 'Create Backup Config')}
			</Dialog.Title>
			<div class="mt-4 space-y-4">
				<div>
					<label for="bc-name" class="text-foreground mb-1 block text-sm font-medium">
						{t('backup_configs.name', 'Name')}
					</label>
					<input
						id="bc-name"
						type="text"
						bind:value={createForm.name}
						class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
						placeholder="Daily DB Backup"
					/>
				</div>
				<div>
					<label for="bc-schedule" class="text-foreground mb-1 block text-sm font-medium">
						{t('backup_configs.schedule', 'Schedule')}
					</label>
					<Select.Root type="single" value={selectedSchedulePreset} onValueChange={(v) => { if (v) onSchedulePresetChange(v); }}>
						<Select.Trigger
							id="bc-schedule"
							class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
							aria-label={t('backup_configs.schedule', 'Schedule')}
						>
							<Select.Value placeholder={t('backup_configs.schedule', 'Select schedule...')} />
						</Select.Trigger>
						<Select.Portal>
							<Select.Content
								class="border-border-input bg-background shadow-popover z-50 rounded-xl border"
								sideOffset={4}
							>
								<Select.Viewport class="p-1">
									{#each schedulePresets as preset (preset.value)}
										<Select.Item
											class="data-highlighted:bg-accent rounded-9px flex h-9 cursor-pointer items-center px-3 text-sm outline-none"
											value={preset.value}
											label={preset.label}
										>
											{#snippet children({ selected })}
												<span>{preset.label}</span>
												{#if selected}<span class="ml-auto">✓</span>{/if}
											{/snippet}
										</Select.Item>
									{/each}
								</Select.Viewport>
							</Select.Content>
						</Select.Portal>
					</Select.Root>
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
					<label class="text-foreground mb-1 block text-sm font-medium">
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
										aria-label="Remove {target.label}"
									>
										<X class="size-3" />
									</button>
								</span>
							{/each}
						</div>
					{/if}

					<Popover.Root>
						<Popover.Trigger
							class="border-border-input bg-background text-foreground hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus flex w-full items-center justify-between rounded-9px border px-3 py-2 text-sm transition-colors"
							aria-label={t('backup_configs.what_to_backup', 'What to Backup')}
						>
							<span class={createForm.what_to_backup.length === 0 ? 'text-muted-foreground' : ''}>
								{createForm.what_to_backup.length === 0
									? t('backup_configs.select_targets', 'Select what to backup...')
									: t('backup_configs.targets_selected', '{count} selected').replace('{count}', String(createForm.what_to_backup.length))}
							</span>
							<ChevronDown class="text-muted-foreground size-4 shrink-0" />
						</Popover.Trigger>
						<Popover.Portal>
							<Popover.Content
								class="border-border-input bg-background shadow-popover z-50 w-[var(--bits-popover-anchor-width)] rounded-xl border p-0"
								sideOffset={4}
							>
								<div class="max-h-60 overflow-y-auto p-1">
									{#each backupTargets as target (target.key)}
										<button
											type="button"
											class="hover:bg-accent flex w-full items-center gap-2 rounded-9px px-3 py-2 text-left text-sm transition-colors {createForm.what_to_backup.includes(target.key) ? 'bg-accent' : ''}"
											onclick={() => toggleBackupTarget(target.key)}
											title={target.description}
										>
											<span class="flex-1">
												<span class="text-foreground block font-medium">{target.label}</span>
												<span class="text-muted-foreground block text-xs">{target.description}</span>
											</span>
											{#if createForm.what_to_backup.includes(target.key)}
												<Check class="text-foreground size-4 shrink-0" />
											{/if}
										</button>
									{/each}
								</div>
								{#if createForm.what_to_backup.length > 0}
									<div class="border-border-input border-t p-2">
										<button
											type="button"
											class="text-muted-foreground hover:text-foreground w-full text-center text-xs font-medium"
											onclick={() => { createForm.what_to_backup = []; }}
										>
											{t('backup_configs.clear_all', 'Clear all')}
										</button>
									</div>
								{/if}
							</Popover.Content>
						</Popover.Portal>
					</Popover.Root>
					<p class="text-muted-foreground mt-1 text-xs">
						{t('backup_configs.what_to_backup_hint', 'Select which components to include in the backup.')}
					</p>
				</div>
				<div>
					<label for="bc-storage" class="text-foreground mb-1 block text-sm font-medium">
						{t('backup_configs.storage_type', 'Storage Type')}
					</label>
					<Select.Root type="single" value={createForm.storage_type} onValueChange={(v) => { if (v) createForm.storage_type = v as BackupConfig['storage_type']; }}>
						<Select.Trigger
							id="bc-storage"
							class="border-border-input bg-background text-foreground w-full rounded-9px border px-3 py-2 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
							aria-label={t('backup_configs.storage_type', 'Storage Type')}
						>
							<Select.Value placeholder={t('backup_configs.select_storage', 'Select storage type...')} />
						</Select.Trigger>
						<Select.Portal>
							<Select.Content
								class="border-border-input bg-background shadow-popover z-50 rounded-xl border"
								sideOffset={4}
							>
								<Select.Viewport class="p-1">
									{#each storageTypeOptions as option}
										<Select.Item
											class="data-highlighted:bg-accent rounded-9px flex h-9 cursor-pointer items-center px-3 text-sm outline-none"
											value={option.value}
											label={option.label}
										>
											{#snippet children({ selected })}
												<span>{option.label}</span>
												{#if selected}<span class="ml-auto">✓</span>{/if}
											{/snippet}
										</Select.Item>
									{/each}
								</Select.Viewport>
							</Select.Content>
						</Select.Portal>
					</Select.Root>
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
				<Dialog.Close
					class="border-border-input bg-background hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
					onclick={closeCreateModal}
				>
					{t('common.cancel', 'Cancel')}
				</Dialog.Close>
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
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

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
