<script lang="ts">
	import { t, i18n } from '$lib/i18n';
	import { Plus } from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import type { WalletData } from './types';

	interface Props {
		data: WalletData;
	}

	let { data }: Props = $props();

	type LedgerFilter = 'all' | 'points' | 'credits';
	let filter = $state<LedgerFilter>('all');

	const ledger = $derived(data.ledger ?? []);
	const filtered = $derived(filter === 'all' ? ledger : ledger.filter((e) => e.type === filter));

	const filters: { key: LedgerFilter; labelKey: string; default: string }[] = [
		{ key: 'all', labelKey: 'settings.wallet.filter_all', default: 'All' },
		{ key: 'points', labelKey: 'settings.wallet.filter_points', default: 'Points' },
		{ key: 'credits', labelKey: 'settings.wallet.filter_credits', default: 'Credits' }
	];

	function formatDate(iso: string | undefined): string {
		if (!iso) return '—';
		return new Date(iso).toLocaleDateString(i18n.locale);
	}
</script>

<div class="space-y-4">
	<div class="mb-3 flex gap-2">
		{#each filters as f (f.key)}
			<button
				type="button"
				class="rounded-[var(--radius)] px-3 py-1.5 text-sm {filter === f.key
					? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
					: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
				onclick={() => (filter = f.key)}
			>
				{t(f.labelKey, f.default)}
			</button>
		{/each}
	</div>

	{#if filtered.length === 0}
		<EmptyState icon={Plus} title={t('wallet.history.empty', 'No transactions yet')} />
	{:else}
		<ul class="max-h-96 space-y-2 overflow-auto">
			{#each filtered as entry (entry.id)}
				<li class="flex items-center justify-between rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm">
					<div class="min-w-0 flex-1">
						<p class="truncate text-[var(--color-text)]">
							{entry.description?.trim() || (entry.amount !== undefined && entry.amount >= 0 ? t('wallet.earned', 'Earned') : t('wallet.spent', 'Spent'))}
						</p>
						<p class="text-xs text-[var(--color-muted)]">{formatDate(entry.created_at)}</p>
					</div>
					<span class="font-medium {entry.amount !== undefined && entry.amount >= 0 ? 'text-[var(--color-success)]' : 'text-[var(--color-danger)]'} [font-variant-numeric:tabular-nums]">
						{entry.amount !== undefined && entry.amount >= 0 ? '+' : ''}{(entry.amount ?? 0).toLocaleString()}
					</span>
				</li>
			{/each}
		</ul>
	{/if}
</div>
