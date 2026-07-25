<script lang="ts">
	import { t } from '$lib/i18n';
	import { unlinkAccount } from '$lib/api/hub';
	import { optimisticAction } from '$lib/components/hub/optimisticAction';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { Link2 } from 'lucide-svelte';

	interface ConnectedAccount {
		provider: string;
		connected_at?: string;
	}

	let { data }: { data: unknown } = $props();
	let accounts = $state<ConnectedAccount[]>((data as ConnectedAccount[] | null) ?? []);

	async function doUnlink(provider: string) {
		const idx = accounts.findIndex((a) => a.provider === provider);
		if (idx < 0) return;
		const removed = accounts[idx];
		await optimisticAction({
			apply: () => (accounts = accounts.filter((a) => a.provider !== provider)),
			rollback: () => (accounts = [...accounts.slice(0, idx), removed, ...accounts.slice(idx)]),
			call: () => unlinkAccount(provider)
		});
	}
</script>

<section class="max-w-2xl">
	<header class="mb-4">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.connected')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.connected.desc')}</p>
	</header>

	{#if accounts.length === 0}
		<EmptyState icon={Link2} title={t('settings.connected.empty_title')} body={t('settings.connected.empty_body')} />
	{:else}
		<ul class="space-y-2">
			{#each accounts as acc (acc.provider)}
				<li class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
					<div>
						<p class="text-sm font-medium text-[var(--color-text)]">{acc.provider}</p>
						{#if acc.connected_at}
							<p class="text-xs text-[var(--color-muted)]">{t('settings.connected.since')} {acc.connected_at}</p>
						{/if}
					</div>
					<button
						type="button"
						class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
						onclick={() => doUnlink(acc.provider)}
					>
						{t('settings.connected.unlink')}
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</section>
