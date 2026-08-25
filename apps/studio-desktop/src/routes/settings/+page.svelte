<script lang="ts">
	import { Save } from 'lucide-svelte';
</script>

<div class="flex h-full flex-col p-6">
	<header class="mb-6">
		<h1 class="text-2xl font-bold">Settings</h1>
		<p class="text-sm text-muted-foreground">Configure the JapaConnect connection and watched project root.</p>
	</header>

	<div class="max-w-2xl space-y-4 rounded-xl border border-border bg-card p-6">
		<label class="block text-sm font-medium">
			Project Name
			<input type="text" class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm" placeholder="My Japa Project" />
		</label>
		<label class="block text-sm font-medium">
			JapaConnect Base URL
			<input type="url" class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm" placeholder="https://api.japaconnect.com" />
		</label>
		<label class="block text-sm font-medium">
			API Key
			<input type="password" class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm" placeholder="jc_live_..." />
			<p class="mt-1 text-xs text-muted-foreground">Stored locally in the OS keychain.</p>
		</label>
		<label class="block text-sm font-medium">
			Project Root Folder
			<input type="text" class="mt-1 w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm" placeholder="/home/user/japa-studio/project-a" />
			<p class="mt-1 text-xs text-muted-foreground">The folder containing <code class="rounded bg-muted px-1">.claude/CLAUDE.md</code> and <code class="rounded bg-muted px-1">jc-content/</code>.</p>
		</label>
		<div class="flex items-center gap-2 pt-2">
			<input type="checkbox" id="auto-sync" class="h-4 w-4 rounded border-border" />
			<label for="auto-sync" class="text-sm font-medium">Enable auto-sync</label>
		</div>
		<div class="pt-4">
			<button class="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-contrast hover:bg-primary-hover">
				<Save class="h-4 w-4" />
				<span>Save Settings</span>
			</button>
		</div>
	</div>
</div>
