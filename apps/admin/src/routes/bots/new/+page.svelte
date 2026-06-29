<script lang="ts">
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';

	let name = $state('');
	let avatarUrl = $state('');
	let description = $state('');
	let creating = $state(false);
	let errorMessage = $state('');

	async function createBot() {
		if (!name.trim()) return;
		creating = true;
		errorMessage = '';
		try {
			const result = await api('/admin/bots', {
				method: 'POST',
				body: {
					name: name.trim(),
					avatar_url: avatarUrl.trim() || null,
					description: description.trim() || null
				}
			});
			if (result.ok) {
				const bot = result.data as { id: string };
				await goto(`/bots/${bot.id}`);
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = 'Failed to create bot';
		} finally {
			creating = false;
		}
	}
</script>

<svelte:head>
	<title>Create Bot — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">Create Bot</h1>
<p class="mb-6 text-sm" style="color: var(--color-muted)">Set up a new automated bot for the platform</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mx-auto max-w-lg rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
	<div class="space-y-4">
		<div>
			<label for="bot-name" class="mb-1 block text-sm font-medium">Name <span style="color: var(--color-danger)">*</span></label>
			<input
				id="bot-name"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				placeholder="e.g. Welcome Bot"
				bind:value={name}
			/>
		</div>

		<div>
			<label for="bot-avatar" class="mb-1 block text-sm font-medium">Avatar URL</label>
			<input
				id="bot-avatar"
				type="url"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				placeholder="https://example.com/avatar.png"
				bind:value={avatarUrl}
			/>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">Leave empty for default avatar</p>
		</div>

		<div>
			<label for="bot-desc" class="mb-1 block text-sm font-medium">Description</label>
			<textarea
				id="bot-desc"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				rows="3"
				placeholder="What does this bot do?"
				bind:value={description}
			></textarea>
		</div>

		<div class="flex items-center gap-3 pt-2">
			<button
				class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
				style="background-color: var(--color-primary)"
				disabled={creating || !name.trim()}
				onclick={createBot}
			>
				{creating ? 'Creating...' : 'Create Bot'}
			</button>
			<a href="/bots" class="rounded-md px-4 py-2 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10" style="color: var(--color-muted)">
				Cancel
			</a>
		</div>
	</div>
</div>
