<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { Headphones, Plus, ChevronRight, Clock, CheckCircle2, XCircle, AlertCircle } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api, type Ticket } from '$lib/api';
	import autoAnimate from '@formkit/auto-animate';
	import { onMount } from 'svelte';

	let { data } = $props();

	let tickets = $derived(data.tickets);
	let loading = $state(false);
	let filter = $state<'all' | 'open' | 'in_progress' | 'resolved' | 'closed'>('all');
	let currentPage = $state(0);
	let totalPages = $state(1);
	let categoryLabels = $state<Record<string, string>>({});

	const filterOptions = [
		{ value: 'all' as const, label: 'All' },
		{ value: 'open' as const, label: 'Open' },
		{ value: 'in_progress' as const, label: 'In Progress' },
		{ value: 'resolved' as const, label: 'Resolved' },
		{ value: 'closed' as const, label: 'Closed' }
	];

	onMount(async () => {
		try {
			const types = await api.listTicketTypes();
			if (Array.isArray(types)) {
				categoryLabels = Object.fromEntries(types.map((t) => [t.slug, t.name]));
			}
		} catch {
			categoryLabels = {
				account: 'Account',
				billing: 'Billing',
				technical: 'Technical',
				content: 'Content',
				feature_request: 'Feature Request',
				other: 'Other'
			};
		}
	});

	async function loadTickets() {
		loading = true;
		try {
			const params: { status?: string; page_size: number; page_offset: number } = {
				page_size: 20,
				page_offset: currentPage * 20
			};
			if (filter !== 'all') params.status = filter;

			const result = await api.listTickets(params);
			tickets = result;
			totalPages = 1;
		} catch {
			tickets = data.tickets;
		} finally {
			loading = false;
		}
	}

	function setFilter(f: 'all' | 'open' | 'in_progress' | 'resolved' | 'closed') {
		filter = f;
		currentPage = 0;
		loadTickets();
	}

	function nextPage() {
		if (currentPage < totalPages - 1) {
			currentPage++;
			loadTickets();
		}
	}

	function prevPage() {
		if (currentPage > 0) {
			currentPage--;
			loadTickets();
		}
	}

	function statusColor(status: string): string {
		switch (status) {
			case 'open': return 'var(--color-primary)';
			case 'in_progress': return 'var(--color-warning, #f59e0b)';
			case 'resolved': return 'var(--color-success, #22c55e)';
			case 'closed': return 'var(--color-muted)';
			default: return 'var(--color-muted)';
		}
	}

	function statusIcon(status: string) {
		switch (status) {
			case 'open': return AlertCircle;
			case 'in_progress': return Clock;
			case 'resolved': return CheckCircle2;
			case 'closed': return XCircle;
			default: return AlertCircle;
		}
	}

	import { formatTimestamp } from '$lib/utils/timezone';

	function formatDate(dateStr: string): string {
		return formatTimestamp(dateStr);
	}
</script>

<svelte:head>
	<title>My Tickets | JapaConnect</title>
	<meta name="description" content="View and manage your support tickets." />
</svelte:head>

<main class="mx-auto max-w-4xl px-4 py-8">
	<header class="mb-8 flex items-start justify-between">
		<div>
			<h1 class="text-2xl font-bold sm:text-3xl" style="color: var(--color-foreground)">{t('tickets.my_tickets')}</h1>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">
				Get help from our support team.
			</p>
		</div>
		<a
			href="/tickets/new"
			class="flex items-center gap-1.5 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90"
			style="background-color: var(--color-primary)"
		>
			<Plus class="h-4 w-4" />
			New Ticket
		</a>
	</header>

	<!-- Filters -->
	<div class="mb-6 flex gap-2 overflow-x-auto pb-2">
		{#each filterOptions as opt}
			<button
				onclick={() => setFilter(opt.value)}
				class="shrink-0 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
				style="background-color: {filter === opt.value ? 'var(--color-primary)' : 'var(--color-surface)'}; color: {filter === opt.value ? 'white' : 'var(--color-foreground)'}; border: 1px solid {filter === opt.value ? 'transparent' : 'var(--color-border)'}"
			>
				{opt.label}
			</button>
		{/each}
	</div>

	{#if loading}
		<div class="space-y-3">
			{#each Array(5) as _}
				<div class="h-20 animate-pulse rounded-xl" style="background-color: var(--color-surface)"></div>
			{/each}
		</div>
	{:else if tickets.length === 0}
		<EmptyState
			icon={Headphones}
			title="No tickets yet"
			message="Need help? Create a support ticket and our team will assist you."
		/>
	{:else}
		<div class="divide-y rounded-xl border" style="border-color: var(--color-border); divide-color: var(--color-border)" use:autoAnimate>
			{#each tickets as ticket (ticket.id)}
				{@const SIcon = statusIcon(ticket.status)}
				<a
					href="/tickets/{ticket.id}"
					class="flex items-center gap-4 p-4 transition-colors hover:bg-black/5 dark:hover:bg-white/5"
				>
					<div class="flex-1 min-w-0">
						<div class="flex items-center gap-2">
							<h3 class="text-sm font-semibold truncate" style="color: var(--color-foreground)">
								{ticket.subject}
							</h3>
							<span
								class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium"
								style="background-color: color-mix(in srgb, {statusColor(ticket.status)} 12%, transparent); color: {statusColor(ticket.status)}"
							>
								<SIcon class="h-3 w-3" />
								{ticket.status.replace('_', ' ')}
							</span>
						</div>
						<div class="mt-1.5 flex items-center gap-2 text-xs" style="color: var(--color-muted)">
							<span>{categoryLabels[ticket.category] ?? ticket.category}</span>
							<span>&middot;</span>
							<span>{formatDate(ticket.created_at)}</span>
						</div>
					</div>
					<ChevronRight class="h-4 w-4 shrink-0" style="color: var(--color-muted)" />
				</a>
			{/each}
		</div>

		{#if totalPages > 1}
			<div class="mt-6 flex items-center justify-between">
				<button
					onclick={prevPage}
					disabled={currentPage === 0}
					class="rounded-lg px-3 py-1.5 text-sm font-medium transition-colors disabled:opacity-40"
					style="color: var(--color-foreground); background-color: var(--color-surface); border: 1px solid var(--color-border)"
				>
					Previous
				</button>
				<span class="text-sm" style="color: var(--color-muted)">
					Page {currentPage + 1} of {totalPages}
				</span>
				<button
					onclick={nextPage}
					disabled={currentPage >= totalPages - 1}
					class="rounded-lg px-3 py-1.5 text-sm font-medium transition-colors disabled:opacity-40"
					style="color: var(--color-foreground); background-color: var(--color-surface); border: 1px solid var(--color-border)"
				>
					Next
				</button>
			</div>
		{/if}
	{/if}
</main>
