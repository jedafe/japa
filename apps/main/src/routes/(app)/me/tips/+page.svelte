<script lang="ts">
	// /me/tips — sent & received tip history (§125).
	// Ref: japaconnect-engineering/sections/125-donations-tips.md
	import { t } from '$lib/i18n';
	import { listTipsSent, listTipsReceived, type Tip } from '$lib/api/tips';
	import { Loader2 } from 'lucide-svelte';

	let tab = $state<'received' | 'sent'>('received');
	let items = $state<Tip[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let page = $state(1);
	const pageSize = 20;

	async function load() {
		loading = true;
		error = null;
		const res = tab === 'received' ? await listTipsReceived(page, pageSize) : await listTipsSent(page, pageSize);
		loading = false;
		if (res.error) {
			error = res.error.message;
			return;
		}
		items = res.data ?? [];
	}

	$effect(() => {
		tab;
		page;
		load();
	});

	function amount(tip: Tip): string {
		if (tip.method === 'points') return `${tip.points_amount ?? 0} pts`;
		if (tip.method === 'credits') return `${tip.credits_amount ?? 0} cr`;
		if (tip.fiat_amount != null) return `${(tip.fiat_amount / 100).toFixed(2)} ${tip.fiat_currency ?? ''}`;
		return '—';
	}
</script>

<svelte:head><title>{t('tips.history_title')}</title></svelte:head>

<div class="mx-auto max-w-3xl px-4 py-8">
	<h1 class="mb-4 text-2xl font-bold">{t('tips.history_title')}</h1>

	<div class="mb-4 flex gap-2">
		<button onclick={() => { tab = 'received'; page = 1; }} class="rounded-lg border px-3 py-1.5 text-sm {tab === 'received' ? 'border-primary bg-primary/5 font-medium' : 'border-border'}">
			{t('tips.received')}
		</button>
		<button onclick={() => { tab = 'sent'; page = 1; }} class="rounded-lg border px-3 py-1.5 text-sm {tab === 'sent' ? 'border-primary bg-primary/5 font-medium' : 'border-border'}">
			{t('tips.sent')}
		</button>
	</div>

	{#if loading}
		<div class="flex justify-center py-10"><Loader2 class="size-6 animate-spin text-muted-foreground" /></div>
	{:else if error}
		<p class="text-sm text-destructive">{error}</p>
	{:else if items.length === 0}
		<p class="py-10 text-center text-sm text-muted-foreground">{t('tips.empty')}</p>
	{:else}
		<ul class="divide-y divide-border rounded-lg border border-border">
			{#each items as tip (tip.id)}
				<li class="flex items-center justify-between px-4 py-3">
					<div class="min-w-0">
						<p class="truncate text-sm font-medium">
							{tab === 'received' ? tip.sender_name || 'Someone' : tip.recipient_name || tip.recipient_type}
						</p>
						{#if tip.message}<p class="truncate text-xs text-muted-foreground">{tip.message}</p>{/if}
					</div>
					<div class="ml-4 shrink-0 text-right">
						<p class="text-sm font-semibold">{amount(tip)}</p>
						<p class="text-xs capitalize text-muted-foreground">{tip.method} · {tip.status}</p>
					</div>
				</li>
			{/each}
		</ul>
		<div class="mt-4 flex items-center justify-between">
			<button onclick={() => { if (page > 1) page -= 1; }} disabled={page === 1}
				class="rounded-lg border border-border px-3 py-1.5 text-sm disabled:opacity-40">
				{t('tips.prev', 'Previous')}
			</button>
			<span class="text-sm text-muted-foreground">{page}</span>
			<button onclick={() => { if (items.length === pageSize) page += 1; }} disabled={items.length < pageSize}
				class="rounded-lg border border-border px-3 py-1.5 text-sm disabled:opacity-40">
				{t('tips.next', 'Next')}
			</button>
		</div>
	{/if}
</div>
