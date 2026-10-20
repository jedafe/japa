<script lang="ts">
	// AI Keys settings — BYOK key management: list the caller's attached
	// provider keys (masked), verify, rename/rotate, and delete. Backed by
	// /me/ai-keys + /ai/byok/providers (loader returns {providers, keys}).
	//
	// Spec: docs/superpowers/specs/2026-10-19-ai-byok-design.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { Modal } from '$lib/components/ui';
	import { deleteAIKey, verifyAIKey } from '$lib/api/byok';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';
	import { invalidateAll } from '$app/navigation';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import AddAiKeyDialog from '$lib/ai/AddAiKeyDialog.svelte';
	import EditAiKeyDialog from '$lib/ai/EditAiKeyDialog.svelte';
	import type { ByokProviderOption, UserAIKeyView } from '$lib/ai/agentTypes';
	import { KeyRound, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-svelte';

	interface SectionData {
		providers: ByokProviderOption[];
		keys: UserAIKeyView[];
	}

	let { data, embedded = false }: { data: unknown; embedded?: boolean } = $props();

	const providers = $derived(((data as SectionData | null)?.providers ?? []) as ByokProviderOption[]);
	let keys = $state<UserAIKeyView[]>(untrack(() => (data as SectionData | null)?.keys ?? []));

	// Resync after invalidateAll (same pattern as ConnectedAppsSection —
	// BUG-P014).
	$effect(() => {
		keys = (data as SectionData | null)?.keys ?? [];
	});

	let dialogOpen = $state(false);
	let busyKeyId = $state('');
	let error = $state<string | null>(null);
	let editTarget = $state<UserAIKeyView | null>(null);
	let deleteTarget = $state<UserAIKeyView | null>(null);

	async function doVerify(id: string) {
		if (busyKeyId) return;
		busyKeyId = id;
		error = null;
		try {
			const res = await verifyAIKey(id);
			if (res.error) {
				// e.g. 502 verification_failed — the provider was unreachable;
				// surface it instead of silently re-enabling the button.
				error = res.error.message;
				return;
			}
			// Refetch so status and last_error come back as server truth.
			await invalidateAll();
		} finally {
			busyKeyId = '';
		}
	}

	async function doDelete() {
		const target = deleteTarget;
		deleteTarget = null;
		if (!target) return;
		const idx = keys.findIndex((k) => k.id === target.id);
		if (idx < 0) return;
		const removed = keys[idx];
		const res = await optimisticAction({
			apply: () => (keys = keys.filter((k) => k.id !== target.id)),
			rollback: () => (keys = [...keys.slice(0, idx), removed, ...keys.slice(idx + 1)]),
			call: () => deleteAIKey(target.id)
		});
		if (!res.ok) {
			// Rollback already restored the row — tell the user it failed.
			error = res.error?.message ?? t('error.500_body', 'An unexpected error occurred. Please try again.');
		}
	}
</script>

<section class="max-w-2xl">
	<header class="mb-4 flex items-start justify-between gap-4">
		<div>
			{#if !embedded}
				<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.ai_provider_keys')}</h1>
			{/if}
			<p class="text-sm text-[var(--color-muted)]">{t('settings.ai_keys.desc')}</p>
		</div>
		<button
			type="button"
			onclick={() => (dialogOpen = true)}
			class="inline-flex shrink-0 items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-foreground)]"
		>
			<Plus class="h-4 w-4" />
			{t('ai.byok.add_key', 'Add API key')}
		</button>
	</header>

	{#if error}<p class="mb-3 text-xs text-[var(--color-danger)]" role="alert">{error}</p>{/if}

	{#if keys.length === 0}
		<EmptyState icon={KeyRound} title={t('settings.ai_keys.empty_title')} body={t('settings.ai_keys.empty_body')} />
	{:else}
		<ul class="space-y-2">
			{#each keys as key (key.id)}
				<li class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
					<div class="flex items-center justify-between gap-3">
						<div class="min-w-0">
							<p class="truncate text-sm font-medium text-[var(--color-text)]">
								{key.label || key.provider?.display_name}
								<span class="ml-2 font-mono text-xs text-[var(--color-muted)]">{key.api_key_masked}</span>
							</p>
							<p class="text-xs text-[var(--color-muted)]">
								{#if key.label}{key.provider?.display_name} · {/if}
								{#if key.status === 'invalid'}
									<span class="text-[var(--color-destructive)]">{t('ai.byok.invalid', 'key rejected')}</span>
								{:else}
									{t('ai.byok.status_active', 'active')}
								{/if}
							</p>
							{#if key.status === 'invalid' && key.last_error}
								<p class="truncate text-xs text-[var(--color-destructive)]" title={key.last_error}>{key.last_error}</p>
							{/if}
						</div>
						<div class="flex shrink-0 items-center gap-1">
							<button
								type="button"
								title={t('ai.byok.verify', 'Re-verify')}
								aria-label={t('ai.byok.verify', 'Re-verify')}
								disabled={busyKeyId === key.id}
								onclick={() => doVerify(key.id)}
								class="rounded-[var(--radius)] p-1.5 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-40"
							>
								<RefreshCw class="h-4 w-4" />
							</button>
							<button
								type="button"
								title={t('common.edit', 'Edit')}
								aria-label={t('common.edit', 'Edit')}
								onclick={() => (editTarget = key)}
								class="rounded-[var(--radius)] p-1.5 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
							>
								<Pencil class="h-4 w-4" />
							</button>
							<button
								type="button"
								title={t('common.delete', 'Delete')}
								aria-label={t('common.delete', 'Delete')}
								onclick={() => (deleteTarget = key)}
								class="rounded-[var(--radius)] p-1.5 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-destructive)]"
							>
								<Trash2 class="h-4 w-4" />
							</button>
						</div>
					</div>
				</li>
			{/each}
		</ul>
	{/if}

	<AddAiKeyDialog bind:open={dialogOpen} {providers} onsaved={() => invalidateAll()} />
	<EditAiKeyDialog
		open={editTarget !== null}
		keyEntry={editTarget}
		onclose={() => (editTarget = null)}
		onsaved={() => invalidateAll()}
	/>

	<Modal
		open={deleteTarget !== null}
		onOpenChange={(v) => { if (!v) deleteTarget = null; }}
		title={t('ai.byok.delete_title', 'Remove API key?')}
		description={t('ai.byok.delete_confirm', 'AI turns using it will stop working.')}
		size="md"
	>
		{#snippet children(close)}
			<div class="flex justify-end gap-2">
				<button type="button" onclick={close} class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)]">
					{t('common.cancel', 'Cancel')}
				</button>
				<button
					type="button"
					class="rounded-[var(--radius)] bg-[var(--color-danger)] px-4 py-1.5 text-sm font-medium text-white"
					onclick={doDelete}
				>
					{t('common.delete', 'Delete')}
				</button>
			</div>
		{/snippet}
	</Modal>
</section>
