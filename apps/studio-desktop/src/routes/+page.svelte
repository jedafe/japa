<script lang="ts">
	import { onMount } from 'svelte';
	import { Sparkles, Settings, RefreshCw } from 'lucide-svelte';

	let files = $state<{ path: string; title: string; status: string }[]>([]);

	onMount(async () => {
		// TODO: scan jc-content/ via Tauri fs API
		files = [];
	});
</script>

<div class="flex h-full flex-col p-6">
	<header class="mb-6 flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold">Content Queue</h1>
			<p class="text-sm text-muted-foreground">Review, edit, preview, and publish JapaConnect content.</p>
		</div>
		<div class="flex items-center gap-2">
			<button class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent">
				<RefreshCw class="h-4 w-4" />
				<span>Rescan</span>
			</button>
			<a href="/prompt" class="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-contrast hover:bg-primary-hover">
				<Sparkles class="h-4 w-4" />
				<span>New Prompt</span>
			</a>
			<a href="/settings" class="inline-flex items-center gap-2 rounded-lg border border-border bg-surface px-4 py-2 text-sm hover:bg-accent">
				<Settings class="h-4 w-4" />
				<span>Settings</span>
			</a>
		</div>
	</header>

	<div class="rounded-xl border border-border bg-card p-8 text-center">
		<p class="text-muted-foreground">No files detected yet.</p>
		<p class="text-sm text-muted-foreground mt-2">Generate prompts in Claude Code to create <code class="rounded bg-muted px-1">jc-content/[content-type]/[slug].md</code> files, then rescan.</p>
	</div>

	{#if files.length > 0}
		<div class="mt-4 rounded-xl border border-border bg-card">
			<table class="w-full text-sm">
				<thead class="border-b border-border bg-accent">
					<tr>
						<th class="px-4 py-2 text-left font-medium">Title</th>
						<th class="px-4 py-2 text-left font-medium">Type</th>
						<th class="px-4 py-2 text-left font-medium">Status</th>
						<th class="px-4 py-2 text-left font-medium">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each files as file}
						<tr class="border-b border-border last:border-0">
							<td class="px-4 py-2">{file.title}</td>
							<td class="px-4 py-2">{file.status}</td>
							<td class="px-4 py-2"><span class="capitalize">{file.status}</span></td>
							<td class="px-4 py-2"><a href="/editor/{file.path}" class="text-primary hover:underline">Edit</a></td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
