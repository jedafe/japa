<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
	import ConfirmModal from '$lib/components/ui/ConfirmModal.svelte';
	import { Switch } from 'bits-ui';
	import { api } from '$lib/api';
	import { t, translations } from '$lib/stores/i18n.svelte';
	import {
		Puzzle,
		ToggleLeft,
		ToggleRight,
		RefreshCw,
		AlertTriangle,
		Loader2,
		Package
	} from 'lucide-svelte';

	interface Extension {
		key: string;
		name: string;
		description: string;
		enabled: boolean;
		version: string;
	}

	let extensions = $state<Extension[]>([]);
	let loading = $state(true);
	let error = $state('');
	let toggling = $state<string | null>(null);
	let message = $state('');
	let messageType = $state<'success' | 'error' | ''>('');

	// Disabling an extension that may have active content (e.g. spaces,
	// posts, or integrations that depend on it) is destructive — surface a
	// confirmation dialog before sending the disable request.
	let confirmDisableOpen = $state(false);
	let pendingDisable = $state<Extension | null>(null);

	let enabledCount = $derived(
		translations() || extensions.filter((e) => e.enabled).length
	);
	let enabledTotal = $derived(extensions.filter((e) => e.enabled).length);
	let disabledTotal = $derived(extensions.length - enabledTotal);

	async function load() {
		loading = true;
		error = '';
		const result = await api<Extension[]>('/control/extensions');
		if (result.ok) {
			extensions = (result.data ?? []).map((e) => ({
				key: e.key,
				name: e.name,
				description: e.description,
				enabled: !!e.enabled,
				version: e.version
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
			if (document.visibilityState === 'visible') load();
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
				// Reconcile with the server response if it returns updated fields.
				if (result.data) {
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
</script>

<!-- RBAC CRUD: Extensions management -->

<PageHeader
	title={t('extensions.title', 'Extensions')}
	description={t('extensions.description', 'Manage platform extensions and integrations')}
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
		onclick={load}
	>
		<RefreshCw class="size-3" />
		{t('common.refresh', 'Refresh')}
	</button>
</div>

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
			onclick={load}
		>
			{t('common.retry', 'Retry')}
		</button>
	</div>
{:else if loading && extensions.length === 0}
	<!-- Skeleton cards while the initial load is in flight. -->
	<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
		{#each Array(6) as _, i}
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
		{#each extensions as ext (ext.key)}
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
							<h3 class="text-foreground truncate text-sm font-semibold">{ext.name}</h3>
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

				<div class="mt-4 flex items-center justify-between border-t border-border-card pt-4">
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

					<div class="flex items-center gap-2">
						{#if isToggling}
							<Loader2 class="text-muted-foreground size-4 animate-spin" />
						{/if}
						<Switch.Root
							id="ext-{ext.key}"
							checked={ext.enabled}
							disabled={isToggling}
							onCheckedChange={(checked) => handleToggle(ext, checked)}
							class="bg-muted data-[state=checked]:bg-emerald-500 peer inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full px-[2px] transition-colors disabled:cursor-not-allowed disabled:opacity-50"
						>
							<Switch.Thumb
								class="bg-background pointer-events-none block size-4 shrink-0 rounded-full shadow-lg transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0"
							/>
						</Switch.Root>
					</div>
				</div>
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
