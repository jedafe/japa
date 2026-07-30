<script lang="ts">
	/**
	 * API Keys settings — Section 50 (Developer).
	 *
	 * Create, rotate, and revoke API keys. Scopes are selectable via a
	 * multi-select dropdown populated from GET /developer/scopes (Rule 16 —
	 * no hardcoded scope list).
	 *
	 * Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
	 */
	import { Dialog } from 'bits-ui';
	import { untrack, onMount } from 'svelte';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { createApiKey, rotateApiKey, revokeApiKey } from '$lib/api/hub';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';
	import { invalidate } from '$app/navigation';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Key, ChevronDown, Check, X } from 'lucide-svelte';
	import ApiKeyRevealDialog from '../flows/ApiKeyRevealDialog.svelte';

	interface ApiKey {
		id: string;
		name: string;
		prefix?: string;
		scopes?: string[];
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
	let error = $state<string | null>(null);

	let revealOpen = $state(false);
	let revealedKey = $state('');
	let revokeTarget = $state<string | null>(null);

	// Scope catalog from GET /developer/scopes (Rule 16 — no hardcoded list).
	let scopes = $state<ScopeOption[]>([]);
	let scopeDropdownOpen = $state(false);

	onMount(async () => {
		const res = await api.get<ScopeOption[]>('/developer/scopes');
		if (res.data) scopes = res.data;
	});

	function toggleScope(value: string) {
		if (selectedScopes.includes(value)) {
			selectedScopes = selectedScopes.filter((s) => s !== value);
		} else {
			selectedScopes = [...selectedScopes, value];
		}
	}

	function removeScope(value: string) {
		selectedScopes = selectedScopes.filter((s) => s !== value);
	}

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
		await invalidate('/api/v1/me/api-keys');
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
		await invalidate('/api/v1/me/api-keys');
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
			rollback: () => (keys = [...keys.slice(0, idx), removed, ...keys.slice(idx)]),
			call: () => revokeApiKey(id)
		});
	}

	function onRevealClose() {
		revealedKey = '';
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
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				value={newName}
				oninput={(e) => (newName = e.currentTarget.value)}
			/>
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

			<!-- Dropdown trigger -->
			<div class="relative">
				<button
					type="button"
					onclick={() => (scopeDropdownOpen = !scopeDropdownOpen)}
					class="inline-flex w-full items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					aria-expanded={scopeDropdownOpen}
					aria-label={t('settings.apikeys.select_scopes', 'Select scopes')}
				>
					<span class={selectedScopes.length === 0 ? 'text-[var(--color-muted)]' : ''}>
						{selectedScopes.length === 0
							? t('settings.apikeys.select_scopes', 'Select scopes...')
							: t('settings.apikeys.scopes_selected', '{count} selected').replace('{count}', String(selectedScopes.length))}
					</span>
					<ChevronDown size={16} class="transition-transform {scopeDropdownOpen ? 'rotate-180' : ''}" />
				</button>

				{#if scopeDropdownOpen}
					<!-- Click-outside backdrop -->
					<button
						type="button"
						class="fixed inset-0 z-40 cursor-default"
						aria-hidden="true"
						tabindex="-1"
						onclick={() => (scopeDropdownOpen = false)}
					></button>

					<!-- Scope dropdown panel -->
					<div class="absolute z-50 mt-1 max-h-60 w-full overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] shadow-lg">
						{#if scopes.length === 0}
							<div class="px-3 py-2 text-sm text-[var(--color-muted)]">
								{t('settings.apikeys.scopes_loading', 'Loading scopes...')}
							</div>
						{:else}
							{#each scopes as scope (scope.value)}
								<button
									type="button"
									class="flex w-full items-start gap-2 px-3 py-2 text-left hover:bg-[var(--color-surface-subtle)]"
									onclick={() => toggleScope(scope.value)}
								>
									<span class="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded border border-[var(--color-border)] {selectedScopes.includes(scope.value) ? 'bg-[var(--color-primary)] text-white' : ''}">
										{#if selectedScopes.includes(scope.value)}
											<Check size={12} />
										{/if}
									</span>
									<span class="min-w-0 flex-1">
										<span class="block text-sm font-medium text-[var(--color-text)]">{scope.label}</span>
										<span class="block text-xs text-[var(--color-muted)]">{scope.description}</span>
									</span>
								</button>
							{/each}
						{/if}
					</div>
				{/if}
			</div>
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

	{#if keys.length === 0}
		<EmptyState icon={Key} title={t('settings.apikeys.empty_title')} body={t('settings.apikeys.empty_body')} />
	{:else}
		<ul class="space-y-2">
			{#each keys as key (key.id)}
				<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
					<div class="min-w-0">
						<p class="text-sm font-medium text-[var(--color-text)]">{key.name}</p>
						<p class="text-xs text-[var(--color-muted)]">
							{key.prefix ?? '••••'}
							{#if key.scopes?.length}
								{#each key.scopes as s, i (s)}
									{#if i === 0} · {/if}
									<span class="inline-block rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] font-medium">{scopeLabel(s)}</span>
								{/each}
							{/if}
						</p>
					</div>
					<div class="ml-3 flex shrink-0 gap-2">
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

	<Dialog.Root open={revokeTarget !== null} onOpenChange={(v) => { if (!v) revokeTarget = null; }}>
		<Dialog.Portal>
			<Dialog.Content class="z-50 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6 shadow-lg">
				<Dialog.Title class="mb-2 text-lg font-medium text-[var(--color-text)]">
					{t('settings.apikeys.revoke_confirm')}
				</Dialog.Title>
				<Dialog.Description class="mb-4 text-sm text-[var(--color-muted)]">
					{t('settings.apikeys.revoke_confirm_body')}
				</Dialog.Description>
				<div class="flex justify-end gap-2">
					<Dialog.Close class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)]">
						{t('common.cancel')}
					</Dialog.Close>
					<button
						type="button"
						class="rounded-[var(--radius)] bg-[var(--color-danger)] px-4 py-1.5 text-sm font-medium text-white"
						onclick={doRevoke}
					>
						{t('settings.apikeys.revoke')}
					</button>
				</div>
			</Dialog.Content>
		</Dialog.Portal>
	</Dialog.Root>
</section>
