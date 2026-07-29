<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { Select, Tabs, Dialog } from 'bits-ui';
	import { api } from '$lib/api';
	import type { FeatureFlag, DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';
	import { t, translations } from '$lib/stores/i18n.svelte';
	import { escapeHtml } from '$lib/utils';
	import { Flag, RefreshCw, Check, X, Plus } from 'lucide-svelte';

	let {} = $props();

	function normalizeFlags(raw: unknown): FeatureFlag[] {
		if (Array.isArray(raw)) return raw;
		if (raw && typeof raw === 'object') {
			return Object.entries(raw).map(([key, value]) => ({
				key,
				enabled: typeof value === 'boolean' ? value : false,
				description: '',
				allowed_groups: []
			}));
		}
		return [];
	}

	let flags = $state<FeatureFlag[]>([]);
	let loading = $state(true);
	let filter = $state('');
	let filterGroup = $state('all');
	let toggling = $state<string | null>(null);
	let message = $state('');
	let activeTab = $state('all');

	let confirmDisableOpen = $state(false);
	let pendingDisableFlag = $state<FeatureFlag | null>(null);
	let page = $state(0);
	let pageSize = $state(10);
	let confirmBulkDisableOpen = $state(false);
	let pendingBulkDisable = $state<string[] | null>(null);
	// H5: Create feature flag dialog state
	let showCreateModal = $state(false);
	let createKey = $state('');
	let createName = $state('');
	let createDescription = $state('');
	let createEnabled = $state(false);
	let creating = $state(false);

	async function load() {
		loading = true;
		const result = await api<FeatureFlag[] | Record<string, unknown>>('/control/feature-flags');
		if (result.ok) {
			flags = normalizeFlags(result.data);
		}
		loading = false;
	}

	$effect(() => {
		load();
	});

	async function toggleFlag(key: string, enabled: boolean) {
		toggling = key;
		try {
			const result = await api(`/control/feature-flags/${key}`, {
				method: 'PATCH',
				body: { enabled }
			});
			if (result.ok) {
				flags = flags.map((f) => (f.key === key ? { ...f, enabled } : f));
				message = t('feature_flags.toggle_success', 'Flag toggled').replace('{key}', key).replace('{status}', enabled ? t('feature_flags.enabled', 'enabled') : t('feature_flags.disabled', 'disabled'));
				setTimeout(() => { message = ''; }, 3000);
			} else {
				message = result.error || t('feature_flags.toggle_error', 'Failed to toggle flag');
			}
		} catch {
			message = t('feature_flags.toggle_error', 'Failed to toggle flag');
		} finally {
			toggling = null;
		}
	}

	function handleFlagToggle(flag: FeatureFlag, checked: boolean) {
		if (!checked && flag.enabled) {
			pendingDisableFlag = flag;
			confirmDisableOpen = true;
		} else {
			toggleFlag(flag.key, checked);
		}
	}

	function confirmDisable() {
		if (pendingDisableFlag) {
			toggleFlag(pendingDisableFlag.key, false);
		}
		confirmDisableOpen = false;
		pendingDisableFlag = null;
	}

	let allGroups = $derived(
		[...new Set(flags.flatMap((f) => f.allowed_groups ?? []))].sort()
	);

	let groupItems = $derived.by(() => {
		translations();
		return [
		{ value: 'all', label: t('feature_flags.all_groups', 'All Groups') },
		...allGroups.map((g) => ({ value: g, label: g }))
	];

	});

	let filtered = $derived(
		filter || filterGroup !== 'all'
			? flags.filter(
					(f) =>
						(filter
							? f.key.toLowerCase().includes(filter.toLowerCase()) ||
								f.description.toLowerCase().includes(filter.toLowerCase())
							: true) &&
						(filterGroup !== 'all' ? (f.allowed_groups ?? []).includes(filterGroup) : true)
				)
			: flags
	);

	let enabledCount = $derived(flags.filter((f) => f.enabled).length);
	let disabledCount = $derived(flags.length - enabledCount);

	let filteredEnabled = $derived(filtered.filter((f) => f.enabled));
	let filteredDisabled = $derived(filtered.filter((f) => !f.enabled));

	let tabItems = $derived.by(() => {
		translations();
		return [
		{ value: 'all', label: `${t('common.all', 'All')} (${filtered.length})` },
		{ value: 'enabled', label: `${t('feature_flags.enabled_count', 'Enabled')} (${filteredEnabled.length})` },
		{ value: 'disabled', label: `${t('feature_flags.disabled_count', 'Disabled')} (${filteredDisabled.length})` }
	];

	});

	let displayedFlags = $derived(
		activeTab === 'all' ? filtered : activeTab === 'enabled' ? filteredEnabled : filteredDisabled
	);

	// M1: shared DataTable spec — columns, per-row actions, bulk actions.
	let columns = $derived.by<DataTableColumn[]>(() => {
		translations();
		return [
		{
			key: 'key',
			label: t('feature_flags.col_key', 'Key'),
			sortable: true,
			render: (v) => `<span class="font-mono text-xs">${escapeHtml(String(v ?? ''))}</span>`
		},
		{
			key: 'description',
			label: t('feature_flags.col_description', 'Description'),
			render: (v) => `<span class="text-muted-foreground text-xs">${escapeHtml(String(v ?? ''))}</span>`
		},
		{
			key: 'allowed_groups',
			label: t('feature_flags.col_groups', 'Groups'),
			render: (v) => {
				const groups = (Array.isArray(v) ? v : []) as string[];
				if (groups.length === 0) return '<span class="text-muted-foreground text-xs">—</span>';
				return groups
					.map((g) => `<span class="bg-muted text-muted-foreground rounded-5px px-1.5 py-0.5 text-[10px] mr-1">${escapeHtml(g)}</span>`)
					.join('');
			}
		},
		{
			key: 'enabled',
			label: t('feature_flags.col_status', 'Status'),
			sortable: true,
			render: (v) => {
				const on = v === true || v === 'true';
				const cls = on ? 'bg-emerald-500/10 text-emerald-600' : 'bg-muted text-muted-foreground';
				return `<span class="${cls} rounded-full px-2 py-0.5 text-xs font-medium">${
					on ? t('feature_flags.enabled', 'enabled') : t('feature_flags.disabled', 'disabled')
				}</span>`;
			}
		}
	];

	});

	let rowActions = $derived.by<DataTableRowAction[]>(() => {
		translations();
		return [
		{
			label: t('feature_flags.enable', 'Enable'),
			icon: Check,
			onClick: (row) => handleFlagToggle(row as unknown as FeatureFlag, true),
			disabled: (row) => (row as unknown as FeatureFlag).enabled || toggling === (row as unknown as FeatureFlag).key
		},
		{
			label: t('feature_flags.disable', 'Disable'),
			icon: X,
			variant: 'destructive',
			onClick: (row) => handleFlagToggle(row as unknown as FeatureFlag, false),
			disabled: (row) => !(row as unknown as FeatureFlag).enabled || toggling === (row as unknown as FeatureFlag).key
		}
	];

	});

	let bulkActions = $derived.by<DataTableBulkAction[]>(() => {
		translations();
		return [
		{
			label: t('feature_flags.enable_selected', 'Enable Selected'),
			icon: Check,
			onClick: (ids) => enableSelected(ids)
		},
		{
			label: t('feature_flags.disable_selected', 'Disable Selected'),
			icon: X,
			variant: 'destructive',
			onClick: (ids) => {
				pendingBulkDisable = ids;
				confirmBulkDisableOpen = true;
			}
		}
	];

	});

	// H4: Use Promise.all instead of sequential loops for bulk operations.
	async function enableSelected(ids: string[]) {
		const toEnable = flags.filter((f) => ids.includes(f.key) && !f.enabled);
		await Promise.allSettled(toEnable.map((f) => toggleFlag(f.key, true)));
		load();
	}

	async function confirmBulkDisable() {
		if (pendingBulkDisable) {
			const ids = pendingBulkDisable;
			const toDisable = flags.filter((f) => ids.includes(f.key) && f.enabled);
			await Promise.allSettled(toDisable.map((f) => toggleFlag(f.key, false)));
		}
		pendingBulkDisable = null;
		load();
	}

	// H5: Create a new feature flag via POST /control/feature-flags.
	async function createFlag() {
		if (!createKey.trim()) return;
		creating = true;
		try {
			const result = await api('/control/feature-flags', {
				method: 'POST',
				body: {
					key: createKey.trim(),
					name: createName.trim() || createKey.trim(),
					description: createDescription.trim(),
					is_enabled: createEnabled
				}
			});
			if (result.ok) {
				message = t('feature_flags.create_success', 'Feature flag created successfully.');
				showCreateModal = false;
				createKey = '';
				createName = '';
				createDescription = '';
				createEnabled = false;
				await load();
			} else {
				message = result.error || t('feature_flags.create_error', 'Failed to create feature flag.');
			}
		} catch {
			message = t('feature_flags.create_error', 'Failed to create feature flag.');
		} finally {
			creating = false;
		}
	}

</script>

<PageHeader title={t('feature_flags.title', 'Feature Flags')} description={t('feature_flags.description', 'Toggle feature flags across the platform')} />

<div class="mb-4 flex items-center gap-3">
	<input
		bind:value={filter}
		placeholder={t('feature_flags.search_placeholder', 'Search flags...')}
		class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 h-input rounded-9px w-72 border px-3 py-2.5 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
	/>
	<Select.Root type="single" bind:value={filterGroup} items={groupItems}>
		<Select.Trigger
			class="border-border-input bg-background data-placeholder:text-muted-foreground/50 rounded-9px flex h-10 items-center border px-3 text-sm transition-colors hover:border-border-input-hover"
			aria-label={t('feature_flags.all_groups', 'All Groups')}
		>
			<Select.Value placeholder={t('feature_flags.all_groups', 'All Groups')} />
		</Select.Trigger>
		<Select.Portal>
			<Select.Content class="border-border-input bg-background shadow-popover z-50 rounded-xl border px-1 py-1" sideOffset={4}>
				<Select.Viewport class="p-1">
					{#each groupItems as item}
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
	<!-- H5: Create button for feature flags -->
	<button
		class="bg-accent text-accent-foreground hover:bg-accent/90 rounded-9px flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium transition-colors"
		onclick={() => (showCreateModal = true)}
	>
		<Plus class="size-3" />
		{t('feature_flags.create', 'Create Flag')}
	</button>
	<button class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors" onclick={load}>
		<RefreshCw class="size-3" />
		{t('common.refresh', 'Refresh')}
	</button>
</div>

{#if message}
	<div class="bg-accent text-accent-foreground mb-4 rounded-lg border border-accent/20 px-4 py-3 text-sm">
		{message}
	</div>
{/if}

{#if loading}
	<p class="text-muted-foreground py-8 text-center text-sm">{t('common.loading', 'Loading...')}</p>
{:else if flags.length === 0}
	<div class="border-border-card bg-background-alt rounded-9px border p-8 text-center">
		<Flag class="text-muted-foreground mx-auto size-8" />
		<p class="text-muted-foreground mt-3 text-sm">{t('feature_flags.no_flags', 'No feature flags configured.')}</p>
	</div>
{:else}
	<Tabs.Root bind:value={activeTab}>
		<Tabs.List class="bg-muted mb-4 inline-flex gap-1 rounded-9px p-1">
			{#each tabItems as tab}
				<Tabs.Trigger value={tab.value} class="data-[state=active]:bg-background data-[state=active]:shadow-mini rounded-7px px-4 py-2 text-sm font-medium transition-all">
					{tab.label}
				</Tabs.Trigger>
			{/each}
		</Tabs.List>

		<Tabs.Content value={activeTab} class="mt-0">
			<DataTable
				{columns}
				rows={displayedFlags}
				totalRows={displayedFlags.length}
				{loading}
				bind:page
				bind:pageSize
				rowActions={rowActions}
				bulkActions={bulkActions}
				selectable={true}
				getRowId={(row) => (row as FeatureFlag).key}
				emptyMessage={filter ? t('feature_flags.no_match', 'No flags match your filter.') : t('feature_flags.no_flags', 'No feature flags configured.')}
			/>
		</Tabs.Content>
	</Tabs.Root>
{/if}

<ConfirmModal
	bind:open={confirmBulkDisableOpen}
	title={t('feature_flags.disable_title', 'Disable Feature Flag')}
	consequence={pendingBulkDisable && pendingBulkDisable.length > 0
		? t('feature_flags.disable_confirm', 'Are you sure you want to disable "{key}"? This may affect users who depend on this feature.').replace('{key}', `${pendingBulkDisable.length} selected flags`)
		: ''}
	confirmText={t('feature_flags.disable_text', 'DISABLE')}
	onConfirm={confirmBulkDisable}
	onCancel={() => { confirmBulkDisableOpen = false; pendingBulkDisable = null; }}
/>

<ConfirmModal
	bind:open={confirmDisableOpen}
	title={t('feature_flags.disable_title', 'Disable Feature Flag')}
	consequence={pendingDisableFlag
		? t('feature_flags.disable_confirm', 'Are you sure you want to disable "{key}"? This may affect users who depend on this feature.').replace('{key}', pendingDisableFlag.key)
		: ''}
	confirmText={t('feature_flags.disable_text', 'DISABLE')}
	onConfirm={confirmDisable}
	onCancel={() => { confirmDisableOpen = false; pendingDisableFlag = null; }}
/>

<!-- H5: Create Feature Flag Dialog -->
<Dialog.Root bind:open={showCreateModal}>
	<Dialog.Portal>
		<Dialog.Overlay class="bg-black/50 fixed inset-0 z-50" />
		<Dialog.Content
			class="border-border-card bg-background shadow-popover focus:outline-none fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-xl border p-6"
			role="dialog"
			aria-modal="true"
		>
			<Dialog.Title class="text-foreground text-lg font-semibold">
				{t('feature_flags.create_title', 'Create Feature Flag')}
			</Dialog.Title>
			<div class="mt-4 space-y-4">
				<div>
					<label class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider" for="flag-key">
						{t('feature_flags.key', 'Key')} *
					</label>
					<input
						id="flag-key"
						bind:value={createKey}
						placeholder="feature.my_new_flag"
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 rounded-9px w-full border px-3 py-2 text-sm"
					/>
				</div>
				<div>
					<label class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider" for="flag-name">
						{t('feature_flags.name', 'Name')}
					</label>
					<input
						id="flag-name"
						bind:value={createName}
						placeholder={t('feature_flags.name_placeholder', 'My New Flag')}
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 rounded-9px w-full border px-3 py-2 text-sm"
					/>
				</div>
				<div>
					<label class="text-muted-foreground mb-1 block text-xs font-medium uppercase tracking-wider" for="flag-desc">
						{t('feature_flags.description', 'Description')}
					</label>
					<textarea
						id="flag-desc"
						bind:value={createDescription}
						rows={2}
						class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 rounded-9px w-full border px-3 py-2 text-sm"
					></textarea>
				</div>
				<label class="flex items-center gap-2">
					<input type="checkbox" bind:checked={createEnabled} class="size-4" />
					<span class="text-foreground text-sm">{t('feature_flags.enabled_initial', 'Enabled initially')}</span>
				</label>
			</div>
			<div class="mt-6 flex justify-end gap-2">
				<button
					class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
					onclick={() => (showCreateModal = false)}
				>
					{t('common.cancel', 'Cancel')}
				</button>
				<button
					class="bg-foreground text-background rounded-9px px-4 py-2 text-sm font-medium transition-colors hover:opacity-90 disabled:opacity-50"
					disabled={creating || !createKey.trim()}
					onclick={createFlag}
				>
					{creating ? t('common.creating', 'Creating...') : t('feature_flags.create', 'Create Flag')}
				</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
