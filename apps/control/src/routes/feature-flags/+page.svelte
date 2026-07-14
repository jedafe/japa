<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import { Switch, Select, Tooltip } from 'bits-ui';
	import { api } from '$lib/api';
	import type { FeatureFlag } from '$lib/api/types';
	import { t } from '$lib/stores/i18n.svelte';
	import { Flag, RefreshCw } from 'lucide-svelte';
	import type { PageData } from './$types';

	let { data = {} as PageData } = $props();

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

	// svelte-ignore state_referenced_locally
	let flags = $state<FeatureFlag[]>(normalizeFlags(data.flags));
	let loading = $state(false);
	let filter = $state('');
	let filterGroup = $state('all');
	let toggling = $state<string | null>(null);
	let message = $state('');

	async function load() {
		loading = true;
		const result = await api<FeatureFlag[] | Record<string, unknown>>('/control/feature-flags');
		if (result.ok) {
			flags = normalizeFlags(result.data);
		}
		loading = false;
	}

	async function toggleFlag(key: string, enabled: boolean) {
		toggling = key;
		const result = await api(`/control/feature-flags/${key}`, {
			method: 'PATCH',
			body: { enabled }
		});
		if (result.ok) {
			flags = flags.map((f) => (f.key === key ? { ...f, enabled } : f));
			message = t('feature_flags.toggle_success', `Flag "${key}" ${enabled ? 'enabled' : 'disabled'}.`);
		} else {
			message = result.error || t('feature_flags.toggle_error', 'Failed to toggle flag.');
		}
		toggling = null;
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
</script>

<PageHeader title={t('feature_flags.title', 'Feature Flags')} description={t('feature_flags.description', 'Manage feature flags')} />

<div class="mb-4 flex items-center gap-3">
	<input
		bind:value={filter}
		placeholder="Search flags..."
		class="border-border-input bg-background text-foreground placeholder:text-muted-foreground/50 h-input rounded-9px w-72 border px-3 py-2.5 text-sm transition-colors hover:border-border-input-hover focus:border-border-input-hover focus:shadow-date-field-focus"
	/>
	<Select.Root type="single" bind:value={filterGroup} items={groupItems}>
		<Select.Trigger
			class="border-border-input bg-background data-placeholder:text-muted-foreground/50 rounded-9px flex h-10 items-center border px-3 text-sm transition-colors hover:border-border-input-hover"
			aria-label="Filter by group"
		>
			<Select.Value placeholder="All Groups" />
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
	<span class="text-muted-foreground text-xs">
		{enabledCount} {t('feature_flags.enabled', 'enabled')}, {disabledCount} {t('feature_flags.disabled', 'disabled')}
	</span>
	<div class="flex-1"></div>
	<button class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px border px-3 py-1.5 text-xs font-medium transition-colors" onclick={load}>
		<RefreshCw class="size-3" />
		{t('common.refresh')}
	</button>
</div>

{#if message}
	<div class="bg-accent text-accent-foreground mb-4 rounded-lg border border-accent/20 px-4 py-3 text-sm">
		{message}
	</div>
{/if}

{#if loading}
	<p class="text-muted-foreground py-8 text-center text-sm">{t('common.loading', 'Loading...')}</p>
{:else if filtered.length === 0}
	<div class="border-border-card bg-background-alt rounded-9px border p-8 text-center">
		<Flag class="text-muted-foreground mx-auto size-8" />
		<p class="text-muted-foreground mt-3 text-sm">{filter ? t('feature_flags.no_match', 'No flags match your search.') : t('feature_flags.no_flags', 'No feature flags configured.')}</p>
	</div>
{:else}
	<div class="space-y-6">
		<!-- Enabled flags -->
		{#if filteredEnabled.length > 0}
			<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
				<div class="border-border-card border-b px-4 py-3">
					<h3 class="text-foreground text-sm font-semibold">{t('feature_flags.enabled_count', 'Enabled')} ({filteredEnabled.length})</h3>
				</div>
				<div class="divide-border-card divide-y">
					{#each filteredEnabled as flag (flag.key)}
						<div class="flex items-center gap-4 px-4 py-3">
							<div class="min-w-0 flex-1">
								<div class="flex items-center gap-2">
									<p class="text-foreground font-mono text-sm font-medium">{flag.key}</p>
									<Tooltip.Root>
										<Tooltip.Trigger>
											<span class="text-muted-foreground cursor-help text-xs">ℹ</span>
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
										<span class="bg-accent text-accent-foreground rounded-5px px-1.5 py-0.5 text-[10px]">{group}</span>
									{/each}
								</div>
								<Switch.Root
									checked={flag.enabled}
									onCheckedChange={(checked) => toggleFlag(flag.key, checked)}
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

		<!-- Disabled flags -->
		{#if filteredDisabled.length > 0}
			<div class="border-border-card bg-background-alt shadow-card overflow-hidden rounded-xl">
				<div class="border-border-card border-b px-4 py-3">
					<h3 class="text-muted-foreground text-sm font-semibold">{t('feature_flags.disabled_count', 'Disabled')} ({filteredDisabled.length})</h3>
				</div>
				<div class="divide-border-card divide-y">
					{#each filteredDisabled as flag (flag.key)}
						<div class="flex items-center gap-4 px-4 py-3">
							<div class="min-w-0 flex-1">
								<div class="flex items-center gap-2">
									<p class="text-foreground font-mono text-sm font-medium">{flag.key}</p>
									<Tooltip.Root>
										<Tooltip.Trigger>
											<span class="text-muted-foreground cursor-help text-xs">ℹ</span>
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
										<span class="bg-muted text-muted-foreground rounded-5px px-1.5 py-0.5 text-[10px]">{group}</span>
									{/each}
								</div>
								<Switch.Root
									checked={flag.enabled}
									onCheckedChange={(checked) => toggleFlag(flag.key, checked)}
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
	</div>
{/if}
