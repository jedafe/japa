<script lang="ts">
	import { Wallet, ArrowRightLeft, History, Loader2, Coins, TrendingUp, TrendingDown } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { getToken } from '$lib/api';
	import { onMount } from 'svelte';

	interface WalletBalance {
		points: number;
		credits: number;
	}

	interface LedgerEntry {
		id: string;
		amount: number;
		balance_after: number;
		type: string;
		description: string;
		created_at: string;
	}

	let balance = $state<WalletBalance>({ points: 0, credits: 0 });
	let ledger = $state<LedgerEntry[]>([]);
	let loading = $state(true);
	let converting = $state(false);
	let errorMessage = $state('');
	let successMessage = $state('');

	function authHeaders(extra?: Record<string, string>): Record<string, string> {
		const h: Record<string, string> = { 'Content-Type': 'application/json', ...extra };
		const token = getToken();
		if (token) h['Authorization'] = `Bearer ${token}`;
		return h;
	}

	onMount(async () => {
		try {
			const [balanceRes, ledgerRes] = await Promise.allSettled([
				fetch('/api/v1/wallet/balance', { headers: authHeaders() }),
				fetch('/api/v1/wallet/ledger?limit=30', { headers: authHeaders() })
			]);

			if (balanceRes.status === 'fulfilled') {
				const body = await balanceRes.value.json();
				if (body.data) balance = body.data;
			}

			if (ledgerRes.status === 'fulfilled') {
				const body = await ledgerRes.value.json();
				if (body.data) ledger = body.data;
			}
		} catch {
			// use defaults
		} finally {
			loading = false;
		}
	});

	function formatNumber(n: number): string {
		return new Intl.NumberFormat('en-US').format(n);
	}

	import { formatTimestampWithTime } from '$lib/utils/timezone';

	function formatDate(dateStr: string): string {
		return formatTimestampWithTime(dateStr);
	}

	function isPositiveEntry(type: string): boolean {
		return type === 'credit' || type === 'earn' || type === 'convert_in';
	}

	function entryColor(type: string): string {
		if (type === 'credit' || type === 'earn' || type === 'convert_in') return 'var(--color-success, #22c55e)';
		return 'var(--color-danger)';
	}

	async function convertPointsToCredits() {
		if (balance.points <= 0) return;
		converting = true;
		errorMessage = '';
		successMessage = '';

		try {
			const res = await fetch('/api/v1/wallet/convert', {
				method: 'POST',
				headers: authHeaders(),
				body: JSON.stringify({ points: balance.points })
			});
			const body = await res.json();
			if (body.data) {
				balance = body.data;
				successMessage = 'Points converted to credits successfully!';
				// Refresh ledger
				const ledgerRes = await fetch('/api/v1/wallet/ledger?limit=30', { headers: authHeaders() });
				const ledgerBody = await ledgerRes.json();
				if (ledgerBody.data) ledger = ledgerBody.data;
			}
		} catch {
			errorMessage = 'Failed to convert points. Please try again.';
		} finally {
			converting = false;
		}
	}
</script>

<svelte:head>
	<title>Wallet | JapaConnect</title>
	<meta name="description" content="Manage your points and credits." />
</svelte:head>

<main class="mx-auto max-w-4xl px-4 py-8">
	<h1 class="mb-8 text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">
		Wallet
	</h1>

	{#if loading}
		<div class="space-y-4">
			<div class="h-40 animate-pulse rounded-xl" style="background-color: var(--color-surface)"></div>
			<div class="h-72 animate-pulse rounded-xl" style="background-color: var(--color-surface)"></div>
		</div>
	{:else}
		<!-- Balance Cards -->
		<div class="mb-8 grid gap-4 sm:grid-cols-2">
			<div
				class="rounded-xl border p-6"
				style="border-color: var(--color-border); background: var(--color-surface)"
			>
				<div class="flex items-center gap-3">
					<div
						class="flex h-12 w-12 items-center justify-center rounded-xl"
						style="background-color: color-mix(in srgb, var(--color-primary) 12%, transparent)"
					>
						<Coins class="h-6 w-6" style="color: var(--color-primary)" />
					</div>
					<div>
						<p class="text-sm" style="color: var(--color-muted)">Points</p>
						<p class="text-2xl font-bold" style="color: var(--color-foreground)">
							{formatNumber(balance.points)}
						</p>
					</div>
				</div>
			</div>

			<div
				class="rounded-xl border p-6"
				style="border-color: var(--color-border); background: var(--color-surface)"
			>
				<div class="flex items-center gap-3">
					<div
						class="flex h-12 w-12 items-center justify-center rounded-xl"
						style="background-color: color-mix(in srgb, var(--color-success, #22c55e) 12%, transparent)"
					>
						<Wallet class="h-6 w-6" style="color: var(--color-success, #22c55e)" />
					</div>
					<div>
						<p class="text-sm" style="color: var(--color-muted)">Credits</p>
						<p class="text-2xl font-bold" style="color: var(--color-foreground)">
							{formatNumber(balance.credits)}
						</p>
					</div>
				</div>
			</div>
		</div>

		<!-- Convert Button -->
		{#if balance.points > 0}
			<div class="mb-8">
				<button
					onclick={convertPointsToCredits}
					disabled={converting}
					class="flex items-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
					style="background-color: var(--color-primary)"
				>
					{#if converting}
						<Loader2 class="h-4 w-4 animate-spin" />
						Converting...
					{:else}
						<ArrowRightLeft class="h-4 w-4" />
						Convert {formatNumber(balance.points)} points to credits
					{/if}
				</button>
				<p class="mt-2 text-xs" style="color: var(--color-muted)">
					Points are converted at a 1:1 ratio to credits.
				</p>
			</div>
		{/if}

		{#if errorMessage}
			<div
				class="mb-4 rounded-lg border px-4 py-3 text-sm"
				style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
			>
				{errorMessage}
			</div>
		{/if}

		{#if successMessage}
			<div
				class="mb-4 rounded-lg border px-4 py-3 text-sm"
				style="border-color: var(--color-success, #22c55e); background-color: color-mix(in srgb, var(--color-success, #22c55e) 8%, transparent); color: var(--color-success, #22c55e)"
			>
				{successMessage}
			</div>
		{/if}

		<!-- Transaction Ledger -->
		<div
			class="rounded-xl border"
			style="border-color: var(--color-border); background: var(--color-surface)"
		>
			<div class="flex items-center gap-2 border-b px-6 py-4" style="border-color: var(--color-border)">
				<History class="h-5 w-5" style="color: var(--color-muted)" />
				<h2 class="text-lg font-semibold" style="color: var(--color-foreground)">
					Recent Activity
				</h2>
			</div>

			{#if ledger.length === 0}
				<EmptyState
					icon={History}
					title="No transactions yet"
					message="Your wallet activity will appear here."
				/>
			{:else}
				<div class="divide-y" style="divide-color: var(--color-border)">
					{#each ledger as entry (entry.id)}
						<div class="flex items-center justify-between px-6 py-4">
							<div class="flex items-center gap-3">
								<div
									class="flex h-9 w-9 items-center justify-center rounded-full"
									style="background-color: color-mix(in srgb, {entryColor(entry.type)} 12%, transparent)"
								>
									{#if isPositiveEntry(entry.type)}
										<TrendingUp class="h-4 w-4" style="color: {entryColor(entry.type)}" />
									{:else}
										<TrendingDown class="h-4 w-4" style="color: {entryColor(entry.type)}" />
									{/if}
								</div>
								<div>
									<p class="text-sm font-medium" style="color: var(--color-foreground)">
										{entry.description}
									</p>
									<p class="mt-0.5 text-xs" style="color: var(--color-muted)">
										{formatDate(entry.created_at)}
									</p>
								</div>
							</div>
							<div class="text-right">
								<p class="text-sm font-semibold" style="color: {entryColor(entry.type)}">
									{entry.amount > 0 ? '+' : ''}{formatNumber(entry.amount)}
								</p>
								<p class="mt-0.5 text-xs" style="color: var(--color-muted)">
									Balance: {formatNumber(entry.balance_after)}
								</p>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</main>
