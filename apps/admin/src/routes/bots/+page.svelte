<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';

	type Bot = {
		id: string;
		name: string;
		avatar_url: string;
		description: string;
		created_at: string;
	};

	let bots = $state<Bot[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<{ items: Bot[] }>(`/admin/bots`);
			if (result.ok) {
				bots = result.data.items ?? [];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = 'Failed to load bots';
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>Bots — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">Bot Management</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">Manage automated bots and their messaging capabilities</p>

<div class="mb-4 flex justify-end">
	<a
		href="/bots/new"
		class="inline-block rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
	>
		Create Bot
	</a>
</div>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Name</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Description</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Created</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each bots as bot}
				<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
					<td class="px-4 py-3">
						<div class="flex items-center gap-3">
							{#if bot.avatar_url}
								<img src={bot.avatar_url} alt="" class="h-8 w-8 rounded-full object-cover" />
							{:else}
								<div class="flex h-8 w-8 items-center justify-center rounded-full bg-black/10 text-xs font-bold dark:bg-white/20">
									{bot.name.charAt(0).toUpperCase()}
								</div>
							{/if}
							<a href="/bots/{bot.id}" class="font-medium hover:underline">{bot.name}</a>
						</div>
					</td>
					<td class="px-4 py-3 text-xs max-w-xs truncate" style="color: var(--color-muted)">{bot.description || '—'}</td>
					<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{bot.created_at}</td>
					<td class="px-4 py-3">
						<a href="/bots/{bot.id}" class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10" style="color: var(--color-info)">
							Edit
						</a>
					</td>
				</tr>
				{:else}
					<tr>
						<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							No bots created yet
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}
