<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { ArrowLeft, Loader2, Headphones } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { goto } from '$app/navigation';

	let subject = $state('');
	let description = $state('');
	let category = $state('other');
	let submitting = $state(false);
	let errorMessage = $state('');

	const categories = [
		{ value: 'account', label: 'Account' },
		{ value: 'billing', label: 'Billing' },
		{ value: 'technical', label: 'Technical' },
		{ value: 'content', label: 'Content' },
		{ value: 'feature_request', label: 'Feature Request' },
		{ value: 'other', label: 'Other' }
	];

	async function handleSubmit(e: Event) {
		e.preventDefault();
		if (!subject.trim() || !description.trim()) return;

		submitting = true;
		errorMessage = '';

		try {
			const ticket = await api.createTicket({
				subject: subject.trim(),
				description: description.trim(),
				type_id: category
			});
			goto(`/tickets/${ticket.id}`);
		} catch (err) {
			errorMessage = err instanceof Error ? err.message : 'Failed to create ticket. Please try again.';
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>New Ticket | JapaConnect</title>
	<meta name="description" content="Create a new support ticket." />
</svelte:head>

<main class="mx-auto max-w-2xl px-4 py-8">
	<a
		href="/tickets"
		class="mb-6 inline-flex items-center gap-1 text-sm transition-colors hover:underline"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		Back to Tickets
	</a>

	<header class="mb-8">
		<div class="flex items-center gap-3">
			<div
				class="flex h-10 w-10 items-center justify-center rounded-xl"
				style="background-color: color-mix(in srgb, var(--color-primary) 12%, transparent)"
			>
				<Headphones class="h-5 w-5" style="color: var(--color-primary)" />
			</div>
			<div>
				<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">{t('tickets.new.new_support_ticket')}</h1>
				<p class="text-sm" style="color: var(--color-muted)">{t('tickets.new.describe_your_issue_and_we_ll_help_you_o')}</p>
			</div>
		</div>
	</header>

	{#if errorMessage}
		<div
			class="mb-6 rounded-lg border px-4 py-3 text-sm"
			style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)"
		>
			{errorMessage}
		</div>
	{/if}

	<form onsubmit={handleSubmit} class="space-y-5">
		<div>
			<label for="category" class="mb-1.5 block text-sm font-medium" style="color: var(--color-foreground)">
				Category
			</label>
			<select
				id="category"
				bind:value={category}
				class="w-full rounded-lg border px-3 py-2.5 text-sm outline-none transition-colors focus:ring-2"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
			>
				{#each categories as cat}
					<option value={cat.value}>{cat.label}</option>
				{/each}
			</select>
		</div>

		<div>
			<label for="subject" class="mb-1.5 block text-sm font-medium" style="color: var(--color-foreground)">
				Subject
			</label>
			<input
				id="subject"
				type="text"
				bind:value={subject}
				placeholder="Brief description of your issue"
				required
				class="w-full rounded-lg border px-3 py-2.5 text-sm outline-none transition-colors focus:ring-2"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
			/>
		</div>

		<div>
			<label for="description" class="mb-1.5 block text-sm font-medium" style="color: var(--color-foreground)">
				Description
			</label>
			<textarea
				id="description"
				bind:value={description}
				placeholder="Provide as much detail as possible about your issue..."
				rows="6"
				required
				class="w-full resize-y rounded-lg border px-3 py-2.5 text-sm outline-none transition-colors focus:ring-2"
				style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-foreground)"
			></textarea>
		</div>

		<div class="flex justify-end gap-3">
			<a
				href="/tickets"
				class="rounded-lg px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
				style="color: var(--color-foreground); border: 1px solid var(--color-border)"
			>
				Cancel
			</a>
			<button
				type="submit"
				disabled={submitting || !subject.trim() || !description.trim()}
				class="flex items-center gap-1.5 rounded-lg px-5 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
				style="background-color: var(--color-primary)"
			>
				{#if submitting}
					<Loader2 class="h-4 w-4 animate-spin" />
					Creating...
				{:else}
					Create Ticket
				{/if}
			</button>
		</div>
	</form>
</main>
