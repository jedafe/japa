<script lang="ts">
	// AI Keys settings — BYOK key management: list the caller's attached
	// provider keys (masked), verify, delete, and add new ones. Backed by
	// /me/ai-keys + /ai/byok/providers (loader returns {providers, keys}).
	//
	// Spec: docs/superpowers/specs/2026-10-19-ai-byok-design.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { deleteAIKey, verifyAIKey } from '$lib/api/byok';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';
	import { invalidateAll } from '$app/navigation';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import AddAiKeyDialog from '$lib/ai/AddAiKeyDialog.svelte';
	import type { ByokProviderOption, UserAIKeyView } from '$lib/ai/agentTypes';
	import { KeyRound, Plus, RefreshCw, Trash2 } from 'lucide-svelte';

	interface SectionData {
		providers: ByokProviderOption[];
		keys: UserAIKeyView[];
	}

	let { data }: { data: unknown } = $props();

	const providers = $derived(((data as SectionData | null)?.providers ?? []) as ByokProviderOption[]);
	let keys = $state<UserAIKeyView[]>(untrack(() => (data as SectionData | null)?.keys ?? []));

	// Resync after invalidateAll (same pattern as ConnectedAppsSection —
	// BUG-P014).
	$effect(() => {
		keys = (data as SectionData | null)?.keys ?? [];
	});

	let dialogOpen = $state(false);
	let busyKeyId = $state('');

	async function doVerify(id: string) {
		busyKeyId = id;
		try {
			const res = await verifyAIKey(id);
			const status = res.data?.status;
			if (status) keys = keys.map((k) => (k.id === id ? { ...k, status } : k));
		} finally {
			busyKeyId = '';
		}
	}

	async function doDelete(id: string) {
		const idx = keys.findIndex((k) => k.id === id);
		if (idx < 0) return;
		const removed = keys[idx];
		await optimisticAction({
			apply: () => (keys = keys.filter((k) => k.id !== id)),
			rollback: () => (keys = [...keys.slice(0, idx), removed, ...keys.slice(idx + 1)]),
			call: () => deleteAIKey(id)
		});
	}
</script>

<section class="max-w-2xl">
	<header class="mb-4 flex items-start justify-between gap-4">
		<div>
			<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.ai_provider_keys')}</h1>
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
								{key.provider?.display_name}
								{#if key.status === 'invalid'}
									<span class="ml-1 text-[var(--color-destructive)]">· {t('ai.byok.invalid', 'key rejected')}</span>
								{/if}
							</p>
						</div>
						<div class="flex shrink-0 items-center gap-1">
							<button
								type="button"
								title={t('ai.byok.verify', 'Re-verify')}
								disabled={busyKeyId === key.id}
								onclick={() => doVerify(key.id)}
								class="rounded-[var(--radius)] p-1.5 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] disabled:opacity-40"
							>
								<RefreshCw class="h-4 w-4" />
							</button>
							<button
								type="button"
								title={t('common.delete', 'Delete')}
								onclick={() => doDelete(key.id)}
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
</section>
