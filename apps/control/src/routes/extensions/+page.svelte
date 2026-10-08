<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import Switch from '$lib/components/ui/Switch.svelte';
	import ManageSheet from '$lib/components/extensions/manage-sheet.svelte';
	import { SURFACES, type Extension } from '$lib/components/extensions/types';
	import { api } from '$lib/api';
	import { t, translations } from '$lib/stores/i18n.svelte';
	import {
		Puzzle,
		ToggleLeft,
		ToggleRight,
		RefreshCw,
		AlertTriangle,
		Loader2,
		Package,
		Settings,
		SlidersHorizontal,
		Route
	} from 'lucide-svelte';

	let extensions = $state<Extension[]>([]);
	let loading = $state(true);
	let error = $state('');
	let toggling = $state<string | null>(null);
	let message = $state('');
	let messageType = $state<'success' | 'error' | ''>('');
	let activeCategory = $state('all');

	// Disabling an extension that may have active content (e.g. spaces,
	// posts, or integrations that depend on it) is destructive — surface a
	// confirmation dialog before sending the disable request.
	let confirmDisableOpen = $state(false);
	let pendingDisable = $state<Extension | null>(null);

	let categories = $derived.by(() => {
		translations();
		return [...new Set(extensions.map((e) => e.category))].sort();
	});

	let filteredExtensions = $derived(
		activeCategory === 'all'
			? extensions
			: extensions.filter((e) => e.category === activeCategory)
	);

	let enabledTotal = $derived(extensions.filter((e) => e.enabled).length);
	let disabledTotal = $derived(extensions.length - enabledTotal);

	async function load(quiet = false) {
		if (!quiet) loading = true;
		error = '';
		const result = await api<Extension[]>('/control/extensions');
		if (result.ok) {
			extensions = (result.data ?? []).map((e) => ({
				key: e.key,
				name: e.name,
				description: e.description,
				enabled: !!e.enabled,
				version: e.version,
				setting_key: e.setting_key ?? '',
				settings_category: e.settings_category ?? '',
				category: e.category ?? 'Other',
				rollout_percentage: e.rollout_percentage ?? 100,
				// Task-9 unified rollout fields — phase derives to shipped/planned
				// server-side when the config blob carries no explicit value.
				phase: e.phase ?? (e.enabled ? 'shipped' : 'planned'),
				show_on_roadmap: !!e.show_on_roadmap,
				surfaces: e.surfaces ?? {},
				linked_roadmap_slug: e.linked_roadmap_slug
			}));
		} else {
			error = result.error || t('extensions.load_error', 'Failed to load extensions.');
		}
		loading = false;
	}

	// Safe polling pattern: refresh every 30s while the tab is visible, and
	// clean up the interval on component destroy. Avoids the leak where a
	// one-shot $effect fetch runs without returning a teardown.
	$effect(() => {
		load();
		const interval = setInterval(() => {
			if (document.visibilityState === 'visible') load(true);
		}, 30_000);
		return () => clearInterval(interval);
	});

	// Optimistic toggle: flip the local state immediately so the UI feels
	// instant, then call the enable/disable endpoint. On failure, roll the
	// card back to its previous state and surface the error.
	async function setEnabled(ext: Extension, enabled: boolean) {
		const previous = ext.enabled;
		extensions = extensions.map((e) =>
			e.key === ext.key ? { ...e, enabled } : e
		);
		toggling = ext.key;
		message = '';
		try {
			const path = enabled
				? `/control/extensions/${ext.key}/enable`
				: `/control/extensions/${ext.key}/disable`;
			const result = await api<Extension>(path, { method: 'POST' });
			if (result.ok) {
				// Reconcile with the server response if it returns an enabled field.
				// The enable/disable endpoints return {key, status} without an
				// enabled boolean, so we keep the optimistic state in that case.
				if (result.data && typeof result.data.enabled === 'boolean') {
					extensions = extensions.map((e) =>
						e.key === ext.key
							? {
								...e,
								enabled: result.data!.enabled,
								version: result.data!.version ?? e.version
							}
							: e
					);
				}
				messageType = 'success';
				message = enabled
					? t('extensions.enable_success', '"{name}" enabled.').replace('{name}', ext.name)
					: t('extensions.disable_success', '"{name}" disabled.').replace('{name}', ext.name);
			} else {
				// Rollback on error.
				extensions = extensions.map((e) =>
					e.key === ext.key ? { ...e, enabled: previous } : e
				);
				messageType = 'error';
				message = result.error || t('extensions.toggle_error', 'Failed to toggle extension.');
			}
		} catch {
			extensions = extensions.map((e) =>
				e.key === ext.key ? { ...e, enabled: previous } : e
			);
			messageType = 'error';
			message = t('extensions.toggle_error', 'Failed to toggle extension.');
		} finally {
			toggling = null;
			if (message) {
				setTimeout(() => {
					message = '';
					messageType = '';
				}, 3000);
			}
		}
	}

	function handleToggle(ext: Extension, checked: boolean) {
		if (!checked && ext.enabled) {
			// Disabling may orphan active content that depends on the
			// extension — confirm before proceeding.
			pendingDisable = ext;
			confirmDisableOpen = true;
			return;
		}
		setEnabled(ext, checked);
	}

	function confirmDisable() {
		if (pendingDisable) {
			setEnabled(pendingDisable, false);
		}
		confirmDisableOpen = false;
		pendingDisable = null;
	}

	// Rollout percentage editing — only for feature-flagged extensions.
	// Updates the feature_flags config blob's rollout_percentage via the
	// control panel API. 100 = fully on, 0 = off, 1-99 = gradual rollout.
	let editingRollout = $state<string | null>(null);
	let rolloutInput = $state(100);
	let savingRollout = $state(false);

	function startEditRollout(ext: Extension) {
		editingRollout = ext.key;
		rolloutInput = ext.rollout_percentage ?? 100;
	}

	function cancelEditRollout() {
		editingRollout = null;
		rolloutInput = 100;
	}

	async function saveRollout(ext: Extension) {
		savingRollout = true;
		try {
			const result = await api(`/control/extensions/${ext.key}/rollout`, {
				method: 'PATCH',
				body: { rollout_percentage: rolloutInput }
			});
			if (result.ok) {
				extensions = extensions.map((e) =>
					e.key === ext.key ? { ...e, rollout_percentage: rolloutInput } : e
				);
				messageType = 'success';
				message = t('extensions.rollout_saved', 'Rollout updated to {p}%').replace('{p}', String(rolloutInput));
			} else {
				messageType = 'error';
				message = result.error || t('extensions.rollout_error', 'Failed to update rollout.');
			}
		} catch {
			messageType = 'error';
			message = t('extensions.rollout_error', 'Failed to update rollout.');
		} finally {
			savingRollout = false;
			editingRollout = null;
			if (message) {
				setTimeout(() => { message = ''; messageType = ''; }, 3000);
			}
		}
	}

	// Manage slide-over — edits the flag's full config blob via PATCH
	// /control/extensions/{key}. The card merges the saved patch
	// optimistically (same rollback contract as setEnabled): onRevert restores
	// the pre-save snapshot if the API rejects.
	let manageOpen = $state(false);
	let managingKey = $state<string | null>(null);
	let managingExt = $derived(extensions.find((e) => e.key === managingKey) ?? null);

	function openManage(ext: Extension) {
		managingKey = ext.key;
		manageOpen = true;
	}

	function applyExtPatch(key: string, patch: Partial<Extension>) {
		extensions = extensions.map((e) => (e.key === key ? { ...e, ...patch } : e));
	}

	function handleExtSaved(key: string, warning?: string) {
		messageType = 'success';
		message =
			warning === 'roadmap_disabled'
				? t(
						'extensions.roadmap_disabled_warning',
						'Saved, but the roadmap feature is disabled — the item will not appear on the board until roadmap.enabled is on.'
					)
				: t('extensions.saved', 'Extension updated.');
		setTimeout(() => {
			message = '';
			messageType = '';
		}, 4000);
	}

	// Phase chip — labels/colors follow features.Phases. Unknown/absent
	// phases render with the neutral style.
	const PHASE_FALLBACKS: Record<string, string> = {
		planned: 'Planned',
		in_progress: 'In progress',
		testing: 'Testing',
		shipped: 'Shipped',
		archived: 'Archived'
	};

	function phaseLabel(phase: string | undefined): string {
		const p = phase ?? 'planned';
		return t(`extensions.phase_${p}`, PHASE_FALLBACKS[p] ?? p);
	}

	function phaseChipClass(phase: string | undefined): string {
		switch (phase) {
			case 'testing':
				return 'bg-amber-500/10 text-amber-600';
			case 'in_progress':
				return 'bg-blue-500/10 text-blue-600';
			case 'shipped':
				return 'bg-emerald-500/10 text-emerald-600';
			case 'archived':
				return 'bg-muted text-muted-foreground line-through';
			default:
				return 'bg-muted text-muted-foreground';
		}
	}

	// Surface visibility summary, e.g. "main ✓ · tools hidden · admin ✓".
	// Absent surfaces default to visible per the eval rules.
	function surfaceSummary(ext: Extension): string {
		return SURFACES.map((s) => {
			const state = ext.surfaces?.[s]?.state ?? 'visible';
			if (state === 'visible') return `${s} ✓`;
			return `${s} ${t(`extensions.surface_${state}`, state).toLowerCase()}`;
		}).join(' · ');
	}
</script>

<!-- RBAC CRUD: Extensions management -->

<PageHeader
	title={t('extensions.title', 'Extensions & Tools')}
	description={t('extensions.description', 'Enable or disable platform extensions and tools — click a title to configure its settings')}
/>

<div class="mb-4 flex items-center gap-3">
	<div class="text-muted-foreground flex items-center gap-4 text-sm">
		<span class="flex items-center gap-1.5">
			<ToggleRight class="size-4 text-emerald-500" />
			{t('extensions.enabled_count', 'Enabled')}: <strong class="text-foreground">{enabledTotal}</strong>
		</span>
		<span class="flex items-center gap-1.5">
			<ToggleLeft class="size-4" />
			{t('extensions.disabled_count', 'Disabled')}: <strong class="text-foreground">{disabledTotal}</strong>
		</span>
	</div>
	<div class="flex-1"></div>
	<button
		class="border-border-input bg-background-alt hover:bg-muted text-foreground rounded-9px flex items-center gap-1.5 border px-3 py-1.5 text-xs font-medium transition-colors"
		onclick={() => load()}
	>
		<RefreshCw class="size-3" />
		{t('common.refresh', 'Refresh')}
	</button>
</div>

<!-- Category filter tabs -->
{#if categories.length > 1}
	<div class="bg-muted mb-4 inline-flex flex-wrap gap-1 rounded-9px p-1">
		<button
			class="rounded-7px px-4 py-2 text-sm font-medium transition-all {activeCategory === 'all' ? 'bg-background text-foreground shadow-mini' : 'text-muted-foreground hover:text-foreground'}"
			onclick={() => (activeCategory = 'all')}
		>
			{t('common.all', 'All')} ({extensions.length})
		</button>
		{#each categories as cat}
			<button
				class="rounded-7px px-4 py-2 text-sm font-medium transition-all {activeCategory === cat ? 'bg-background text-foreground shadow-mini' : 'text-muted-foreground hover:text-foreground'}"
				onclick={() => (activeCategory = cat)}
			>
				{cat}
			</button>
		{/each}
	</div>
{/if}

{#if message}
	<div
		class="mb-4 rounded-lg border px-4 py-3 text-sm transition-colors {messageType === 'success' ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-600' : 'bg-destructive/10 border-destructive/20 text-destructive'}"
	>
		{message}
	</div>
{/if}

{#if error}
	<div class="border-border-card bg-background-alt rounded-9px border p-8 text-center">
		<AlertTriangle class="text-destructive mx-auto size-8" />
		<p class="text-destructive mt-3 text-sm">{error}</p>
		<button
			class="border-border-input bg-background-alt hover:bg-muted text-foreground mt-4 rounded-9px border px-4 py-2 text-sm font-medium transition-colors"
			onclick={() => load()}
		>
			{t('common.retry', 'Retry')}
		</button>
	</div>
{:else if loading && extensions.length === 0}
	<!-- Skeleton cards while the initial load is in flight. -->
	<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
		{#each Array(12) as _, i}
			<div
				class="border-border-card bg-background-alt rounded-9px border p-5"
				aria-hidden="true"
			>
				<div class="flex items-center justify-between">
					<div class="bg-muted h-5 w-32 animate-pulse rounded"></div>
					<div class="bg-muted h-5 w-12 animate-pulse rounded-full"></div>
				</div>
				<div class="bg-muted mt-3 h-4 w-full animate-pulse rounded"></div>
				<div class="bg-muted mt-2 h-4 w-2/3 animate-pulse rounded"></div>
				<div class="mt-5 flex items-center justify-between">
					<div class="bg-muted h-4 w-16 animate-pulse rounded"></div>
					<div class="bg-muted h-5 w-9 animate-pulse rounded-full"></div>
				</div>
			</div>
		{/each}
	</div>
{:else if extensions.length === 0}
	<div class="border-border-card bg-background-alt rounded-9px border p-8 text-center">
		<Puzzle class="text-muted-foreground mx-auto size-8" />
		<p class="text-muted-foreground mt-3 text-sm">
			{t('extensions.empty', 'No extensions installed.')}
		</p>
	</div>
{:else}
	<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
		{#each filteredExtensions as ext (ext.key)}
			{@const isToggling = toggling === ext.key}
			<div
				class="border-border-card bg-background-alt rounded-9px flex flex-col border p-5 transition-colors {ext.enabled ? 'border-emerald-500/40' : ''}"
			>
				<div class="flex items-start justify-between gap-3">
					<div class="flex min-w-0 items-center gap-2.5">
						<div
							class="flex size-9 shrink-0 items-center justify-center rounded-lg {ext.enabled ? 'bg-emerald-500/10' : 'bg-muted'}"
						>
							<Package
								class="size-5 {ext.enabled ? 'text-emerald-600' : 'text-muted-foreground'}"
							/>
						</div>
						<div class="min-w-0">
							{#if ext.settings_category}
							<a
								href="/settings?category={ext.settings_category}"
								class="text-foreground hover:text-primary truncate block text-sm font-semibold transition-colors"
								title={t('extensions.configure', 'Configure {name} settings').replace('{name}', ext.name)}
							>
								{ext.name}
							</a>
						{:else}
							<h3 class="text-foreground truncate text-sm font-semibold">{ext.name}</h3>
						{/if}
							<p class="text-muted-foreground font-mono text-xs">{ext.key}</p>
						</div>
					</div>
					<span
						class="bg-muted text-muted-foreground shrink-0 rounded-full px-2 py-0.5 text-xs font-medium"
					>
						v{ext.version}
					</span>
				</div>

				<p class="text-muted-foreground mt-3 line-clamp-3 min-h-[3.5rem] text-sm">
					{ext.description || t('extensions.no_description', 'No description provided.')}
				</p>

				<!-- Phase chip + roadmap badge + surface visibility summary
				     (unified rollout — spec §6.1). -->
				<div class="mt-3 flex flex-wrap items-center gap-1.5">
					<span
						class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium {phaseChipClass(ext.phase)}"
					>
						{phaseLabel(ext.phase)}
					</span>
					{#if ext.linked_roadmap_slug}
						<span
							class="bg-accent text-accent-foreground inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium"
							title="{t('extensions.linked_roadmap', 'Linked roadmap item')}: {ext.linked_roadmap_slug}"
						>
							<Route class="size-3" />
							{ext.linked_roadmap_slug}
						</span>
					{:else if ext.show_on_roadmap}
						<span
							class="bg-accent text-accent-foreground inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium"
						>
							<Route class="size-3" />
							{t('extensions.roadmap_badge', 'Roadmap')}
						</span>
					{/if}
				</div>
				<p
					class="text-muted-foreground mt-2 truncate text-xs"
					title={surfaceSummary(ext)}
				>
					{surfaceSummary(ext)}
				</p>

				<div class="mt-4 flex items-center justify-between border-t border-border-card pt-4">
					<div class="flex items-center gap-2">
						<span
							class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium {ext.enabled ? 'bg-emerald-500/10 text-emerald-600' : 'bg-muted text-muted-foreground'}"
						>
							{#if ext.enabled}
								<ToggleRight class="size-3.5" />
								{t('extensions.status_enabled', 'Enabled')}
							{:else}
								<ToggleLeft class="size-3.5" />
								{t('extensions.status_disabled', 'Disabled')}
							{/if}
						</span>
						{#if ext.settings_category}
							<a
								href="/settings?category={ext.settings_category}"
								class="text-muted-foreground hover:text-primary inline-flex items-center gap-1 text-xs font-medium transition-colors"
							>
								<Settings class="size-3.5" />
								{t('extensions.configure_short', 'Configure')}
							</a>
						{/if}
						<button
							class="text-muted-foreground hover:text-primary inline-flex items-center gap-1 text-xs font-medium transition-colors"
							onclick={() => openManage(ext)}
						>
							<SlidersHorizontal class="size-3.5" />
							{t('extensions.manage', 'Manage')}
						</button>
					</div>

					<div class="flex items-center gap-2">
						{#if isToggling}
							<Loader2 class="text-muted-foreground size-4 animate-spin" />
						{/if}
						<Switch
							id="ext-{ext.key}"
							checked={ext.enabled}
							disabled={isToggling}
							onCheckedChange={(checked) => handleToggle(ext, checked)}
						/>
					</div>
				</div>

				{#if ext.enabled}
					<div class="mt-2 flex items-center gap-2 text-xs">
						{#if editingRollout === ext.key}
							<span class="text-muted-foreground">{t('extensions.rollout', 'Rollout')}:</span>
							<input
								type="number"
								min="0"
								max="100"
								bind:value={rolloutInput}
								class="border-border-input bg-background w-16 rounded border px-2 py-0.5 text-xs"
							/>
							<span class="text-muted-foreground">%</span>
							<button
								class="text-primary hover:text-primary/80 font-medium"
								onclick={() => saveRollout(ext)}
								disabled={savingRollout}
							>
								{savingRollout ? '...' : t('common.save', 'Save')}
							</button>
							<button
								class="text-muted-foreground hover:text-foreground"
								onclick={cancelEditRollout}
							>
								{t('common.cancel', 'Cancel')}
							</button>
						{:else}
							<button
								class="text-muted-foreground hover:text-primary inline-flex items-center gap-1 font-medium transition-colors"
								onclick={() => startEditRollout(ext)}
								title={t('extensions.rollout_edit', 'Edit rollout percentage')}
							>
								{t('extensions.rollout', 'Rollout')}:
								<span class="text-foreground">{ext.rollout_percentage ?? 100}%</span>
							</button>
						{/if}
					</div>
				{/if}
			</div>
		{/each}
	</div>
{/if}

<ConfirmModal
	bind:open={confirmDisableOpen}
	title={t('extensions.disable_title', 'Disable Extension')}
	consequence={pendingDisable
		? t(
				'extensions.disable_warning',
				'Disabling "{name}" may affect active content that depends on it. Are you sure you want to continue?'
			).replace('{name}', pendingDisable.name)
		: ''}
	confirmText={t('extensions.disable_confirm_text', 'DISABLE')}
	onConfirm={confirmDisable}
	onCancel={() => {
		confirmDisableOpen = false;
		pendingDisable = null;
	}}
/>

<ManageSheet
	bind:open={manageOpen}
	ext={managingExt}
	onToggle={handleToggle}
	onOptimistic={applyExtPatch}
	onRevert={applyExtPatch}
	onSaved={handleExtSaved}
/>
