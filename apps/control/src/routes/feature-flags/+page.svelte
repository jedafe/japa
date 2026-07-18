<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import DataTable from '$lib/components/DataTable.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { Select, Tabs } from 'bits-ui';
	import { api } from '$lib/api';
	import type { FeatureFlag, DataTableColumn, DataTableRowAction, DataTableBulkAction } from '$lib/api/types';
	import { t } from '$lib/stores/i18n';
	import { escapeHtml } from '$lib/utils';
	import { Flag, RefreshCw, Check, X } from 'lucide-svelte';

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
		const result = await api(`/control/feature-flags/${key}`, {
			method: 'PATCH',
			body: { enabled }
		});
		if (result.ok) {
			flags = flags.map((f) => (f.key === key ? { ...f, enabled } : f));
			message = t('feature_flags.toggle_success', 'Flag toggled').replace('{key}', key).replace('{status}', enabled ? t('feature_flags.enabled', 'enabled') : t('feature_flags.disabled', 'disabled'));
		} else {
			message = result.error || t('feature_flags.toggle_error', 'Failed to toggle flag');
		}
		toggling = null;
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

	let groupItems = $derived([
		{ value: 'all', label: t('feature_flags.all_groups', 'All Groups') },
		...allGroups.map((g) => ({ value: g, label: g }))
	]);

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

	let tabItems = $derived([
		{ value: 'all', label: `${t('common.all', 'All')} (${filtered.length})` },
		{ value: 'enabled', label: `${t('feature_flags.enabled_count', 'Enabled')} (${filteredEnabled.length})` },
		{ value: 'disabled', label: `${t('feature_flags.disabled_count', 'Disabled')} (${filteredDisabled.length})` }
	]);

	let displayedFlags = $derived(
		activeTab === 'all' ? filtered : activeTab === 'enabled' ? filteredEnabled : filteredDisabled
	);

	// M1: shared DataTable spec — columns, per-row actions, bulk actions.
	const columns: DataTableColumn[] = [
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

	const rowActions: DataTableRowAction[] = [
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

	const bulkActions: DataTableBulkAction[] = [
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

	async function enableSelected(ids: string[]) {
		for (const f of flags) {
			if (ids.includes(f.key) && !f.enabled) {
				await toggleFlag(f.key, true);
			}
		}
		load();
	}

	async function confirmBulkDisable() {
		if (pendingBulkDisable) {
			for (const id of pendingBulkDisable) {
				const f = flags.find((x) => x.key === id);
				if (f && f.enabled) await toggleFlag(id, false);
			}
		}
		pendingBulkDisable = null;
		load();
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
