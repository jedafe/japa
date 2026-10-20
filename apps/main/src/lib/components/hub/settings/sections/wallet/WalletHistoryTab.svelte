<script lang="ts">
	import { t, i18n } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { Plus } from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import type { LedgerEntry, WalletData } from './types';

	interface Props {
		data: WalletData;
	}

	let { data }: Props = $props();

	type LedgerFilter = 'all' | 'points' | 'credits';
	let filter = $state<LedgerFilter>('all');

	// The merged ledger is the first page of both streams (page_size=50,
	// page_size/page_offset pagination). Further pages append via
	// load-more — each stream pages independently; "more" is inferred
	// from a full page since the endpoints return no total.
	const LEDGER_PAGE = 50;

	interface RawLedgerEntry {
		id: string;
		operation?: string;
		amount?: number;
		reference?: unknown;
		reason?: unknown;
		created_at?: string;
	}

	// Mirrors the loader's mapping (reference falls back to reason).
	function mapEntries(entries: RawLedgerEntry[] | undefined, type: string): LedgerEntry[] {
		return (entries ?? []).map((raw) => {
			const ref = raw?.reference?.toString().trim();
			const reason = raw?.reason?.toString().trim();
			return { id: raw.id, type, operation: raw.operation, amount: raw.amount, description: ref || reason || null, created_at: raw.created_at };
		});
	}

	// Localized headline for known ledger operations — the backend stores
	// English `reference`/`reason` strings, so the operation discriminator
	// (earn/spend/purchase/convert/refund/clawback/admin_adjust/withdraw)
	// drives the primary label and the server text drops to a detail line.
	const WALLET_OPS = new Set(['earn', 'spend', 'purchase', 'convert', 'refund', 'clawback', 'admin_adjust', 'withdraw']);
	function opLabel(entry: LedgerEntry): string | null {
		return entry.operation && WALLET_OPS.has(entry.operation)
			? t(`wallet.op.${entry.operation}`, entry.operation)
			: null;
	}

	let extra = $state<LedgerEntry[]>([]);
	let pointsOffset = $state(0);
	let creditsOffset = $state(0);
	let pointsMore = $state(false);
	let creditsMore = $state(false);
	let moreLoading = $state(false);
	let moreError = $state('');

	// Resync when the loader data is replaced (invalidateAll) — resets the
	// appended pages back to the fresh first page of each stream.
	$effect(() => {
		extra = [];
		pointsOffset = data.pointsLoaded ?? 0;
		creditsOffset = data.creditsLoaded ?? 0;
		pointsMore = (data.pointsLoaded ?? 0) >= LEDGER_PAGE;
		creditsMore = (data.creditsLoaded ?? 0) >= LEDGER_PAGE;
	});

	// Merge the SSR ledger with appended pages, dedupe (entries can straddle
	// a page boundary if new rows land between fetches), sort newest-first.
	const ledger = $derived.by(() => {
		const seen = new Set<string>();
		return [...(data.ledger ?? []), ...extra]
			.filter((e) => {
				const k = `${e.type}:${e.id}`;
				if (seen.has(k)) return false;
				seen.add(k);
				return true;
			})
			.sort((a, b) => new Date(b.created_at ?? 0).getTime() - new Date(a.created_at ?? 0).getTime());
	});
	const filtered = $derived(filter === 'all' ? ledger : ledger.filter((e) => e.type === filter));
	const hasMore = $derived(pointsMore || creditsMore);

	async function loadMore() {
		if (moreLoading || !hasMore) return;
		moreLoading = true;
		moreError = '';
		const results = await Promise.allSettled([
			pointsMore
				? api
						.get<{ entries?: RawLedgerEntry[] }>(
							`/me/wallet/points?page_size=${LEDGER_PAGE}&page_offset=${pointsOffset}`
						)
						.then((res) => {
							if (res.error) throw new Error(res.error.message);
							const batch = res.data?.entries ?? [];
							extra = [...extra, ...mapEntries(batch, 'points')];
							pointsOffset += batch.length;
							pointsMore = batch.length === LEDGER_PAGE;
						})
				: Promise.resolve(),
			creditsMore
				? api
						.get<{ entries?: RawLedgerEntry[] }>(
							`/me/wallet/credits?page_size=${LEDGER_PAGE}&page_offset=${creditsOffset}`
						)
						.then((res) => {
							if (res.error) throw new Error(res.error.message);
							const batch = res.data?.entries ?? [];
							extra = [...extra, ...mapEntries(batch, 'credits')];
							creditsOffset += batch.length;
							creditsMore = batch.length === LEDGER_PAGE;
						})
				: Promise.resolve()
		]);
		if (results.some((r) => r.status === 'rejected')) {
			moreError = t('settings.common.load_error', 'Could not load more entries.');
		}
		moreLoading = false;
	}

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
			{#each filtered as entry (`${entry.type}:${entry.id}`)}
				<li class="flex items-center justify-between rounded-[var(--radius)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm">
					<div class="min-w-0 flex-1">
						<p class="truncate text-[var(--color-text)]">
							{opLabel(entry) ?? entry.description?.trim() ?? (entry.amount !== undefined && entry.amount >= 0 ? t('wallet.earned', 'Earned') : t('wallet.spent', 'Spent'))}
						</p>
						{#if opLabel(entry) && entry.description?.trim()}
							<p class="truncate text-xs text-[var(--color-muted)]">{entry.description}</p>
						{/if}
						<p class="text-xs text-[var(--color-muted)]">{formatDate(entry.created_at)}</p>
					</div>
					<span class="font-medium {entry.amount !== undefined && entry.amount >= 0 ? 'text-[var(--color-success)]' : 'text-[var(--color-danger)]'} [font-variant-numeric:tabular-nums]">
						{entry.amount !== undefined && entry.amount >= 0 ? '+' : ''}{(entry.amount ?? 0).toLocaleString()}
					</span>
				</li>
			{/each}
		</ul>
		{#if moreError}
			<p class="rounded-[var(--radius)] bg-[var(--color-danger)]/10 px-3 py-2 text-sm text-[var(--color-danger)]" role="alert">{moreError}</p>
		{/if}
		{#if hasMore}
			<button
				type="button"
				onclick={loadMore}
				disabled={moreLoading}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-hover)] disabled:opacity-50"
			>
				{moreLoading ? t('settings.common.loading', 'Loading…') : t('settings.common.load_more', 'Load more')}
			</button>
		{/if}
	{/if}
</div>
