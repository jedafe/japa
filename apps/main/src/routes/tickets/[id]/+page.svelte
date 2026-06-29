<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		ArrowLeft,
		Loader2,
		Headphones,
		Send,
		Clock,
		CheckCircle2,
		XCircle,
		AlertCircle,
		ExternalLink
	} from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { api, type Ticket, type TicketComment } from '$lib/api';
	import { onMount } from 'svelte';

	let ticket = $state<Ticket | null>(null);
	let comments = $state<TicketComment[]>([]);
	let loading = $state(true);
	let error = $state('');
	let commentBody = $state('');
	let submittingComment = $state(false);
	const ticketId = $derived(page.params.id);

	const statusColors: Record<string, string> = {
		open: 'var(--color-primary)',
		in_progress: 'var(--color-warning, #f59e0b)',
		resolved: 'var(--color-success, #22c55e)',
		closed: 'var(--color-muted)'
	};

	const categoryLabels: Record<string, string> = {
		account: 'Account',
		billing: 'Billing',
		technical: 'Technical',
		content: 'Content',
		feature_request: 'Feature Request',
		other: 'Other'
	};

	function statusIcon(status: string) {
		switch (status) {
			case 'open': return AlertCircle;
			case 'in_progress': return Clock;
			case 'resolved': return CheckCircle2;
			case 'closed': return XCircle;
			default: return AlertCircle;
		}
	}

	async function loadTicket() {
		if (!ticketId) return;
		loading = true;
		error = '';
		try {
			const result = await api.getTicket(ticketId);
			ticket = result.ticket;
			comments = result.comments;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load ticket.';
		} finally {
			loading = false;
		}
	}

	async function handleSubmitComment(e: Event) {
		e.preventDefault();
		if (!ticket || !commentBody.trim()) return;
		submittingComment = true;
		try {
			const comment = await api.addTicketReply(ticket.id, { body: commentBody.trim() });
			comments = [...comments, comment];
			commentBody = '';
		} catch {
			// ignore
		} finally {
			submittingComment = false;
		}
	}

	function formatDate(dateStr: string): string {
		return new Date(dateStr).toLocaleDateString('en-US', {
			year: 'numeric',
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}

	onMount(loadTicket);
</script>

<svelte:head>
	<title>{ticket?.subject ?? 'Ticket'} | JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-3xl px-4 py-8">
	<a
		href="/tickets"
		class="mb-6 inline-flex items-center gap-1 text-sm transition-colors hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to Tickets
	</a>

	{#if loading}
		<div class="space-y-4">
			<div class="h-8 w-3/4 animate-pulse rounded-lg" style="background-color: var(--color-surface)"></div>
			<div class="h-32 animate-pulse rounded-xl" style="background-color: var(--color-surface)"></div>
		</div>
	{:else if error}
		<div
			class="rounded-lg border px-4 py-3 text-sm"
			style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
		>
			{error}
		</div>
	{:else if ticket}
		{@const SIcon = statusIcon(ticket.status)}
		<!-- Ticket Header -->
		<header class="mb-8">
			<div class="flex items-start justify-between gap-4">
				<div class="flex-1 min-w-0">
					<h1 class="text-xl font-bold leading-snug" style="color: var(--color-foreground)">
						{ticket.subject}
					</h1>
					<div class="mt-2 flex flex-wrap items-center gap-2 text-xs" style="color: var(--color-muted)">
						<span>{formatDate(ticket.created_at)}</span>
						<span>&middot;</span>
						<span>{categoryLabels[ticket.category] ?? ticket.category}</span>
					</div>
				</div>
				<span
					class="inline-flex shrink-0 items-center gap-1 rounded-full px-3 py-1 text-xs font-medium"
					style="background-color: color-mix(in srgb, {statusColors[ticket.status] ?? 'var(--color-muted)'} 12%, transparent); color: {statusColors[ticket.status] ?? 'var(--color-muted)'}"
				>
					<SIcon class="h-3.5 w-3.5" />
					{ticket.status.replace('_', ' ')}
				</span>
			</div>
		</header>

		<!-- Ticket Body -->
		<article
			class="mb-8 rounded-xl border p-5"
			style="border-color: var(--color-border); background-color: var(--color-surface)"
		>
			<div class="flex items-center gap-2 text-xs" style="color: var(--color-muted)">
				<span class="font-medium" style="color: var(--color-foreground)">{ticket.author_name}</span>
				<span>&middot;</span>
				<span>{formatDate(ticket.created_at)}</span>
			</div>
			<div class="prose prose-sm mt-3 max-w-none" style="color: var(--color-foreground)">
				{ticket.description}
			</div>
		</article>

		<!-- Actions -->
		<div class="mb-8 flex gap-3">
			{#if ticket.status !== 'closed'}
				<p class="text-sm" style="color: var(--color-muted)">
					Reply below to continue the conversation.
				</p>
			{/if}
		</div>

		<!-- Comments -->
		<section>
			<h2 class="mb-4 flex items-center gap-2 text-lg font-semibold" style="color: var(--color-foreground)">
				<Headphones class="h-5 w-5" />
				Conversation ({comments.length})
			</h2>

			{#if comments.length === 0}
				<p class="mb-6 text-sm" style="color: var(--color-muted)">
					No messages yet. Start the conversation below.
				</p>
			{:else}
				<div class="mb-8 space-y-4">
					{#each comments as comment (comment.id)}
						<div
							class="rounded-xl border p-4"
							style="border-color: var(--color-border); background-color: var(--color-surface)"
						>
							<div class="flex items-center gap-2 text-xs" style="color: var(--color-muted)">
								<span class="font-medium" style="color: var(--color-foreground)">
									{comment.author_name}
								</span>
								<span>{formatDate(comment.created_at)}</span>
							</div>
							<p class="mt-2 text-sm leading-relaxed" style="color: var(--color-foreground)">
								{comment.body}
							</p>
						</div>
					{/each}
				</div>
			{/if}

			<!-- Comment form -->
			{#if ticket.status !== 'closed'}
				<form onsubmit={handleSubmitComment} class="flex gap-3">
					<textarea
						placeholder="Type your message..."
						bind:value={commentBody}
						rows="2"
						class="flex-1 resize-y rounded-lg border px-3 py-2.5 text-sm outline-none transition-colors focus:ring-2"
						style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
					></textarea>
					<button
						type="submit"
						disabled={submittingComment || !commentBody.trim()}
						class="flex items-center gap-1.5 self-end rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
						style="background-color: var(--color-primary)"
					>
						{#if submittingComment}
							<Loader2 class="h-4 w-4 animate-spin" />
						{:else}
							<Send class="h-4 w-4" />
						{/if}
					</button>
				</form>
			{:else}
				<p class="text-center text-sm" style="color: var(--color-muted)">
					This ticket is closed. Appeal to reopen it.
				</p>
			{/if}
		</section>
	{:else}
		<EmptyState
			icon={Headphones}
			title="Ticket not found"
			message="This ticket may have been deleted or the link is invalid."
		/>
	{/if}
</main>
