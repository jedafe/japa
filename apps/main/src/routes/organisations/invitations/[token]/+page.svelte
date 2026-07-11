<script lang="ts">
	import { Mail, Check, Loader2, X } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';

	let token = $derived($page.params.token ?? '');
	let accepting = $state(false);
	let result = $state<'idle' | 'accepted' | 'failed'>('idle');
	let orgSlug = $state('');

	async function accept() {
		accepting = true;
		try {
			const res = await api.acceptOrganizationInvitation(token);
			orgSlug = res.organization_slug;
			result = 'accepted';
			setTimeout(() => goto(`/organisations/${orgSlug}`), 1500);
		} catch {
			result = 'failed';
		} finally {
			accepting = false;
		}
	}
</script>

<svelte:head>
	<title>Organisation invitation | JapaConnect</title>
</svelte:head>

<main class="mx-auto flex max-w-md flex-col items-center px-4 py-16 text-center">
	<div
		class="flex h-16 w-16 items-center justify-center rounded-2xl text-white"
		style="background: var(--gradient-primary, var(--color-primary))"
	>
		<Mail class="h-8 w-8" />
	</div>

	{#if result === 'idle'}
		<h1 class="mt-6 text-xl font-bold" style="color: var(--color-foreground)">
			You've been invited to an organisation
		</h1>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			Accept this invitation to join and collaborate.
		</p>
		<button
			onclick={accept}
			disabled={accepting}
			class="mt-6 flex items-center gap-2 rounded-xl px-6 py-2.5 text-sm font-semibold text-white transition-opacity hover:opacity-90 disabled:opacity-50"
			style="background: var(--color-primary)"
		>
			{#if accepting}
				<Loader2 class="h-4 w-4 animate-spin" /> Accepting…
			{:else}
				<Check class="h-4 w-4" /> Accept invitation
			{/if}
		</button>
	{:else if result === 'accepted'}
		<div class="mt-6 flex flex-col items-center">
			<Check class="h-8 w-8" style="color: var(--color-primary)" />
			<h1 class="mt-2 text-xl font-bold" style="color: var(--color-foreground)">Welcome aboard!</h1>
			<p class="mt-1 text-sm" style="color: var(--color-muted)">Taking you to the organisation…</p>
		</div>
	{:else}
		<div class="mt-6 flex flex-col items-center">
			<X class="h-8 w-8" style="color: var(--color-destructive, #ef4444)" />
			<h1 class="mt-2 text-xl font-bold" style="color: var(--color-foreground)">
				Invitation invalid
			</h1>
			<p class="mt-1 text-sm" style="color: var(--color-muted)">
				This invitation may have expired or already been used.
			</p>
			<a href="/organisations" class="mt-4 text-sm font-medium hover:underline" style="color: var(--color-primary)">
				Browse organisations
			</a>
		</div>
	{/if}
</main>
