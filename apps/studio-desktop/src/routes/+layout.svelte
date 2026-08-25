<script lang="ts">
	import '../app.css';
	import { LayoutDashboard, Sparkles, Settings, RefreshCw } from 'lucide-svelte';

	let { children } = $props();
</script>

<div class="flex h-screen w-screen overflow-hidden bg-background text-foreground">
	<aside class="w-60 border-r border-border bg-surface flex flex-col">
		<div class="p-4 border-b border-border">
			<h1 class="font-semibold text-lg">Japa Studio</h1>
			<p class="text-xs text-muted-foreground">Content desktop client</p>
		</div>
		<nav class="flex-1 p-2 space-y-1">
			<a href="/" class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm hover:bg-accent">
				<LayoutDashboard class="h-4 w-4" />
				<span>Queue</span>
			</a>
			<a href="/prompt" class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm hover:bg-accent">
				<Sparkles class="h-4 w-4" />
				<span>Prompt</span>
			</a>
			<a href="/sync" class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm hover:bg-accent">
				<RefreshCw class="h-4 w-4" />
				<span>Sync Log</span>
			</a>
			<a href="/settings" class="flex items-center gap-3 rounded-lg px-3 py-2 text-sm hover:bg-accent">
				<Settings class="h-4 w-4" />
				<span>Settings</span>
			</a>
		</nav>
	</aside>
	<main class="flex-1 overflow-auto">
		{@render children?.()}
	</main>
</div>
