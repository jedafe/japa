<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	/**
	 * API Keys settings — Section 50 (Developer).
	 *
	 * Create, rotate, and revoke API keys. Scopes are selectable via an
	 * Autocomplete-powered picker populated from GET /developer/scopes
	 * (Rule 16 — no hardcoded scope list); each pick appends a scope chip.
	 *
	 * Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
	 */
	import { Modal } from '$lib/components/ui';
	import { untrack, onMount } from 'svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { createApiKey, createExtensionToken, rotateApiKey, revokeApiKey } from '$lib/api/hub';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';
	import { invalidateAll } from '$app/navigation';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import { Key, X } from 'lucide-svelte';
	import ApiKeyRevealDialog from '../flows/ApiKeyRevealDialog.svelte';

	interface ApiKey {
		id: string;
		name: string;
		key_prefix?: string;
		scopes?: string[];
		rate_limit_tier?: string;
		status?: string;
		notes?: string;
		created_at?: string;
		updated_at?: string;
	}

	interface ApiKeyIPAllowlist {
		id: string;
		api_key_id: string;
		cidr: string;
		description?: string;
		created_at?: string;
	}

	interface ScopeOption {
		value: string;
		label: string;
		description: string;
	}

	let { data }: { data: unknown } = $props();
	let keys = $state<ApiKey[]>(untrack(() => (data as ApiKey[] | null) ?? []));

	let newName = $state('');
	let selectedScopes = $state<string[]>([]);
	let creating = $state(false);
	let connecting = $state(false);
	let installId = $state('');
	let error = $state<string | null>(null);

	let revealOpen = $state(false);
	let revealedKey = $state('');
	let revokeTarget = $state<string | null>(null);

	// Detail panel state — GET /me/api-keys/{id} and
	// GET /me/api-keys/{id}/ip-allowlist.
	let detailOpen = $state(false);
	let detailKey = $state<ApiKey | null>(null);
	let detailLoading = $state(false);
	let detailAllowlist = $state<ApiKeyIPAllowlist[]>([]);

	// Re-seed the list when the server payload changes (invalidateAll re-runs
	// the loader after create/rotate/revoke; the untracked init would
	// otherwise keep the stale array — same fix class as BUG-P014).
	$effect(() => {
		keys = (data as ApiKey[] | null) ?? [];
	});

	// Scope catalog from GET /developer/scopes (Rule 16 — no hardcoded list).
	let scopes = $state<ScopeOption[]>([]);

	onMount(async () => {
		const res = await api.get<ScopeOption[]>('/developer/scopes');
		if (res.data) scopes = res.data;
	});

	function addScope(value: string) {
		if (!selectedScopes.includes(value)) {
			selectedScopes = [...selectedScopes, value];
		}
	}

	function removeScope(value: string) {
		selectedScopes = selectedScopes.filter((s) => s !== value);
	}

	// Autocomplete over the scope catalog, minus scopes already picked —
	// the picker is an "add scope" combobox; chips handle removal.
	const scopeFetcher = $derived(
		staticAutocompleteFetcher(
			scopes
				.filter((s) => !selectedScopes.includes(s.value))
				.map((s) => ({ id: s.value, label: s.label, sublabel: s.description }))
		)
	);

	function scopeLabel(value: string): string {
		return scopes.find((s) => s.value === value)?.label ?? value;
	}

	async function doCreate() {
		if (!newName.trim()) return;
		creating = true;
		error = null;
		const res = await createApiKey({ name: newName.trim(), scopes: selectedScopes });
		creating = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		// Secret shown ONCE — capture before any dismissal
		revealedKey = res.data?.key ?? '';
		revealOpen = true;
		newName = '';
		selectedScopes = [];
		// invalidateAll — the list is SSR-fetched via the backend base URL,
		// so invalidating the browser-relative '/api/v1/...' path matches
		// nothing and the new key never appeared (dead invalidation).
		await invalidateAll();
	}

	// Browser-extension mint (Section 47 v1.3) — scopes are fixed
	// server-side; the minted key is revealed once, like any other key.
	async function doConnectExtension() {
		connecting = true;
		error = null;
		const install_id = installId.trim();
		const res = await createExtensionToken(install_id ? { install_id } : undefined);
		connecting = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		revealedKey = res.data?.key ?? '';
		revealOpen = true;
		await invalidateAll();
	}

	async function doRotate(id: string) {
		error = null;
		const res = await rotateApiKey(id);
		if (res.error) {
			error = res.error.message;
			return;
		}
		revealedKey = res.data?.key ?? '';
		revealOpen = true;
		await invalidateAll();
	}

	async function doRevoke() {
		if (!revokeTarget) return;
		const id = revokeTarget;
		revokeTarget = null;
		const idx = keys.findIndex((k) => k.id === id);
		if (idx < 0) return;
		const removed = keys[idx];
		await optimisticAction({
			apply: () => (keys = keys.filter((k) => k.id !== id)),
			rollback: () => (keys = [...keys.slice(0, idx), removed, ...keys.slice(idx + 1)]),
			call: () => revokeApiKey(id)
		});
	}

	function onRevealClose() {
		revealedKey = '';
	}

	async function openDetail(key: ApiKey) {
		detailOpen = true;
		detailKey = key;
		detailLoading = true;
		detailAllowlist = [];
		try {
			const [keyRes, listRes] = await Promise.all([
				api.get<ApiKey>(`/me/api-keys/${key.id}`),
				api.get<ApiKeyIPAllowlist[]>(`/me/api-keys/${key.id}/ip-allowlist`)
			]);
			if (keyRes.data) detailKey = keyRes.data;
			if (listRes.data) detailAllowlist = listRes.data;
		} finally {
			detailLoading = false;
		}
	}

	function closeDetail() {
		detailOpen = false;
		detailKey = null;
		detailAllowlist = [];
	}
</script>

<section class="max-w-2xl">
	<header class="mb-4">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.api_keys')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.apikeys.desc')}</p>
	</header>

	<form class="mb-6 space-y-3" onsubmit={(e) => { e.preventDefault(); doCreate(); }}>
		<label class="block">
			<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.apikeys.name')}</span>
			<input
				type="text"
				placeholder={t('settings.apikeys.name_placeholder', 'e.g. CI deploy bot')}
				aria-describedby="apikey-name-hint"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				value={newName}
				oninput={(e) => (newName = e.currentTarget.value)}
			/>
			<HintIcon id="apikey-name-hint" text={t('settings.apikeys.name_hint', 'A label so you remember what this key is for.')} />
		</label>

		<!-- Scope multi-select -->
		<div class="block">
			<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.apikeys.scopes')}</span>

			<!-- Selected scope chips -->
			{#if selectedScopes.length > 0}
				<div class="mb-2 flex flex-wrap gap-1.5">
					{#each selectedScopes as scope (scope)}
						<span class="inline-flex items-center gap-1 rounded-full bg-[var(--color-primary)]/10 px-2.5 py-1 text-xs font-medium text-[var(--color-primary)]">
							{scopeLabel(scope)}
							<button
								type="button"
								class="hover:text-[var(--color-danger)]"
								onclick={() => removeScope(scope)}
								aria-label={t('settings.apikeys.remove_scope', 'Remove scope')}
							>
								<X size={12} />
							</button>
						</span>
					{/each}
				</div>
			{/if}

			<!-- Scope picker — Autocomplete combobox; each pick appends a chip
			     above. Options exclude already-selected scopes. -->
			<Autocomplete
				placeholder={scopes.length === 0
					? t('settings.apikeys.scopes_loading', 'Loading scopes...')
					: t('settings.apikeys.select_scopes', 'Select scopes...')}
				ariaLabel={t('settings.apikeys.select_scopes', 'Select scopes')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={scopeFetcher}
				initialFetcher={scopeFetcher}
				selectedId={null}
				onSelect={(item) => addScope(item.id)}
				disabled={creating}
			/>
			<HintIcon text={t('settings.apikeys.scopes_hint', 'The API endpoints this key can access — pick each permission it needs.')} />
		</div>

		{#if error}<p class="text-xs text-[var(--color-danger)]">{error}</p>{/if}
		<button
			type="submit"
			disabled={creating || !newName.trim()}
			class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
		>
			{creating ? t('settings.apikeys.creating') : t('settings.apikeys.create')}
		</button>
	</form>

	<div class="mb-6 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<p class="mb-1 text-sm font-medium text-[var(--color-text)]">{t('settings.apikeys.connect_extension')}</p>
		<input
			type="text"
			bind:value={installId}
			placeholder={t('settings.apikeys.install_id')}
			aria-label={t('settings.apikeys.install_id')}
			aria-describedby="apikey-install-hint"
			class="mb-3 w-full max-w-md rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm text-[var(--color-text)]"
		/>
		<button
			type="button"
			disabled={connecting}
			onclick={doConnectExtension}
			class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
		>
			{connecting ? t('settings.apikeys.connecting') : t('settings.apikeys.connect_extension')}
		</button>
	</div>

	{#if keys.length === 0}
		<EmptyState icon={Key} title={t('settings.apikeys.empty_title')} body={t('settings.apikeys.empty_body')} />
	{:else}
		<ul class="space-y-2">
			{#each keys as key (key.id)}
				<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
					<div class="min-w-0">
						<p class="text-sm font-medium text-[var(--color-text)]">{key.name}</p>
						<p class="text-xs text-[var(--color-muted)]">
							{key.key_prefix ?? '••••'}
							{#if key.scopes?.length}
								{#each key.scopes as s, i (s)}
									{#if i === 0} · {/if}
									<span class="inline-block rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] font-medium">{scopeLabel(s)}</span>
								{/each}
							{/if}
						</p>
					</div>
					<div class="ms-3 flex shrink-0 gap-2">
						<button
							type="button"
							class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
							onclick={() => openDetail(key)}
						>
							{t('settings.apikeys.details')}
						</button>
						<button
							type="button"
							class="text-xs text-[var(--color-muted)] hover:text-[var(--color-text)]"
							onclick={() => doRotate(key.id)}
						>
							{t('settings.apikeys.rotate')}
						</button>
						<button
							type="button"
							class="text-xs text-[var(--color-muted)] hover:text-[var(--color-danger)]"
							onclick={() => (revokeTarget = key.id)}
						>
							{t('settings.apikeys.revoke')}
						</button>
					</div>
				</li>
			{/each}
		</ul>
	{/if}

	<ApiKeyRevealDialog bind:open={revealOpen} apiKey={revealedKey} onClose={onRevealClose} />

	<Modal open={revokeTarget !== null} onOpenChange={(v) => { if (!v) revokeTarget = null; }} title={t('settings.apikeys.revoke_confirm')} description={t('settings.apikeys.revoke_confirm_body')} size="md">
		{#snippet children(close)}
			<div class="flex justify-end gap-2">
				<button type="button" onclick={close} class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)]">
					{t('common.cancel')}
				</button>
				<button
					type="button"
					class="rounded-[var(--radius)] bg-[var(--color-danger)] px-4 py-1.5 text-sm font-medium text-white"
					onclick={doRevoke}
				>
					{t('settings.apikeys.revoke')}
				</button>
			</div>
		{/snippet}
	</Modal>

	<Modal open={detailOpen} onOpenChange={(v) => { if (!v) closeDetail(); }} title={t('settings.apikeys.detail_title')} description={detailKey?.name} size="md">
			{#if detailKey}
					{#if detailLoading}
						<p class="text-sm text-[var(--color-muted)]">{t('settings.apikeys.loading')}</p>
					{:else}
						<dl class="space-y-2 text-sm">
							<div class="flex justify-between">
								<dt class="text-[var(--color-muted)]">{t('settings.apikeys.prefix')}</dt>
								<dd class="text-[var(--color-text)]">{detailKey.key_prefix ?? '••••'}</dd>
							</div>
							<div class="flex justify-between">
								<dt class="text-[var(--color-muted)]">{t('settings.apikeys.status')}</dt>
								<dd class="text-[var(--color-text)]">{detailKey.status ?? '—'}</dd>
							</div>
							<div class="flex justify-between">
								<dt class="text-[var(--color-muted)]">{t('settings.apikeys.rate_limit_tier')}</dt>
								<dd class="text-[var(--color-text)]">{detailKey.rate_limit_tier ?? '—'}</dd>
							</div>
							<div class="flex justify-between">
								<dt class="text-[var(--color-muted)]">{t('settings.apikeys.scopes')}</dt>
								<dd class="text-[var(--color-text)]">{detailKey.scopes?.map(scopeLabel).join(', ') ?? '—'}</dd>
							</div>
							{#if detailKey.created_at}
								<div class="flex justify-between">
									<dt class="text-[var(--color-muted)]">{t('settings.apikeys.created')}</dt>
									<dd class="text-[var(--color-text)]">{new Date(detailKey.created_at).toLocaleString()}</dd>
								</div>
							{/if}
						</dl>

						<h3 class="mb-2 mt-4 text-sm font-medium text-[var(--color-text)]">{t('settings.apikeys.allowlist')}</h3>
						{#if detailAllowlist.length === 0}
							<p class="text-sm text-[var(--color-muted)]">{t('settings.apikeys.allowlist_empty')}</p>
						{:else}
							<ul class="space-y-1 text-sm">
								{#each detailAllowlist as a (a.id)}
									<li class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2">
										<p class="font-mono text-[var(--color-text)]">{a.cidr}</p>
										{#if a.description}
											<p class="text-xs text-[var(--color-muted)]">{a.description}</p>
										{/if}
									</li>
								{/each}
							</ul>
						{/if}
					{/if}
				{:else}
					<p class="text-sm text-[var(--color-muted)]">{t('settings.apikeys.loading')}</p>
				{/if}
	</Modal>
</section>
