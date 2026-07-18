<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { Switch, Select, Tooltip, Tabs } from 'bits-ui';
	import { api } from '$lib/api';
	import type { FeatureFlag } from '$lib/api/types';
	import { t } from '$lib/stores/i18n';
	import { Flag, RefreshCw } from 'lucide-svelte';

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
			{#if displayedFlags.length === 0}
				<div class="border-border-card bg-background-alt rounded-9px border p-8 text-center">
					<Flag class="text-muted-foreground mx-auto size-8" />
					<p class="text-muted-foreground mt-3 text-sm">{filter ? t('feature_flags.no_match', 'No flags match your filter.') : t('feature_flags.no_flags', 'No feature flags configured.')}</p>
				</div>
			{:else}
				<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
					<div class="divide-border-card divide-y">
						{#each displayedFlags as flag (flag.key)}
							<div class="flex items-center gap-4 px-4 py-3">
								<div class="min-w-0 flex-1">
									<div class="flex items-center gap-2">
										<p class="text-foreground font-mono text-sm font-medium">{flag.key}</p>
										<Tooltip.Root>
											<Tooltip.Trigger>
												<span class="text-muted-foreground cursor-help text-xs">{'\u2139'}</span>
											</Tooltip.Trigger>
											<Tooltip.Portal>
												<Tooltip.Content side="top" sideOffset={4} class="border-dark-10 bg-background shadow-popover z-50 w-[280px] rounded-input border p-3 text-xs">
													{flag.description}
												</Tooltip.Content>
											</Tooltip.Portal>
										</Tooltip.Root>
									</div>
									<p class="text-muted-foreground truncate text-xs">{flag.description}</p>
								</div>
								<div class="flex items-center gap-2">
									<div class="flex flex-wrap gap-1">
										{#each flag.allowed_groups as group}
											<span class="{flag.enabled ? 'bg-accent text-accent-foreground' : 'bg-muted text-muted-foreground'} rounded-5px px-1.5 py-0.5 text-[10px]">{group}</span>
										{/each}
									</div>
									<Switch.Root
										checked={flag.enabled}
										onCheckedChange={(checked) => handleFlagToggle(flag, checked)}
										disabled={toggling === flag.key}
										class="bg-muted data-[state=checked]:bg-foreground peer inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full px-[3px] transition-colors disabled:cursor-not-allowed disabled:opacity-50"
									>
										<Switch.Thumb class="bg-background pointer-events-none block size-4 shrink-0 rounded-full transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0" />
									</Switch.Root>
								</div>
							</div>
						{/each}
					</div>
				</div>
			{/if}
		</Tabs.Content>
	</Tabs.Root>
{/if}

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
