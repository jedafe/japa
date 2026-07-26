<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { createApiKey, rotateApiKey, revokeApiKey } from '$lib/api/hub';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';
	import { invalidate } from '$app/navigation';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Key } from 'lucide-svelte';
	import ApiKeyRevealDialog from '../flows/ApiKeyRevealDialog.svelte';

	interface ApiKey {
		id: string;
		name: string;
		prefix?: string;
		scopes?: string[];
		created_at?: string;
	}

	let { data }: { data: unknown } = $props();
	let keys = $state<ApiKey[]>(untrack(() => (data as ApiKey[] | null) ?? []));

	let newName = $state('');
	let newScopes = $state('');
	let creating = $state(false);
	let error = $state<string | null>(null);

	let revealOpen = $state(false);
	let revealedKey = $state('');
	let revokeTarget = $state<string | null>(null);

	async function doCreate() {
		if (!newName.trim()) return;
		creating = true;
		error = null;
		const scopes = newScopes.split(',').map((s) => s.trim()).filter(Boolean);
		const res = await createApiKey({ name: newName.trim(), scopes });
		creating = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		// Secret shown ONCE — capture before any dismissal
		revealedKey = res.data?.key ?? '';
		revealOpen = true;
		newName = '';
		newScopes = '';
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
		<label class="block">
			<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.apikeys.scopes')}</span>
			<input
				type="text"
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
				placeholder="read,write"
				value={newScopes}
				oninput={(e) => (newScopes = e.currentTarget.value)}
			/>
		</label>
		{#if error}<p class="text-xs text-[var(--color-danger,#dc2626)]">{error}</p>{/if}
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
							{#if key.scopes?.length} · {key.scopes.join(', ')}{/if}
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
							class="text-xs text-[var(--color-muted)] hover:text-[var(--color-danger,#dc2626)]"
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
						class="rounded-[var(--radius)] bg-[var(--color-danger,#dc2626)] px-4 py-1.5 text-sm font-medium text-white"
						onclick={doRevoke}
					>
						{t('settings.apikeys.revoke')}
					</button>
				</div>
			</Dialog.Content>
		</Dialog.Portal>
	</Dialog.Root>
</section>
